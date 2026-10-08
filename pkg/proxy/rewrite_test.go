package proxy_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pedrobarco/mroki/pkg/proxy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type receivedRequest struct {
	method      string
	path        string
	rawQuery    string
	contentType string
	mode        string
	body        string
}

func recordRequest(r *http.Request) receivedRequest {
	b, _ := io.ReadAll(r.Body)
	return receivedRequest{
		method:      r.Method,
		path:        r.URL.Path,
		rawQuery:    r.URL.RawQuery,
		contentType: r.Header.Get("Content-Type"),
		mode:        r.Header.Get(proxy.ShadowHeader),
		body:        string(b),
	}
}

func TestProxy_ServeHTTP_rewrites_shadow_request(t *testing.T) {
	var live atomic.Value
	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		live.Store(recordRequest(r))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"source":"live"}`))
	}))
	defer liveServer.Close()

	shadowReceived := make(chan receivedRequest, 1)
	shadowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shadowReceived <- recordRequest(r)
		w.WriteHeader(http.StatusOK)
	}))
	defer shadowServer.Close()

	captured := make(chan proxy.ProxyRequest, 1)
	liveURL, _ := url.Parse(liveServer.URL)
	shadowURL, _ := url.Parse(shadowServer.URL)
	p := proxy.NewProxy(liveURL, shadowURL,
		proxy.WithShadowRequestRewriter(func(r *http.Request, body []byte) (proxy.ShadowRequest, error) {
			h := r.Header.Clone()
			h.Set("Content-Type", "application/json")
			return proxy.ShadowRequest{
				Method: http.MethodPost,
				Path:   "/graphql",
				Header: h,
				Body:   []byte(`{"query":"{ ping }"}`),
			}, nil
		}),
		proxy.WithCallbackFn(func(req proxy.ProxyRequest, _, _ proxy.ProxyResponse) error {
			captured <- req
			return nil
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/bookings/42?expand=guest", nil)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"source":"live"}`, rec.Body.String())

	select {
	case got := <-shadowReceived:
		assert.Equal(t, http.MethodPost, got.method)
		assert.Equal(t, "/graphql", got.path)
		assert.Equal(t, "", got.rawQuery, "rewritten query string replaces the original")
		assert.Equal(t, "application/json", got.contentType)
		assert.Equal(t, proxy.ShadowHeaderValue, got.mode, "identification header survives the rewrite")
		assert.Equal(t, `{"query":"{ ping }"}`, got.body)
	case <-time.After(time.Second):
		t.Fatal("shadow service was not called within timeout")
	}

	gotLive, ok := live.Load().(receivedRequest)
	require.True(t, ok)
	assert.Equal(t, http.MethodGet, gotLive.method, "live request must not be rewritten")
	assert.Equal(t, "/bookings/42", gotLive.path)
	assert.Equal(t, "expand=guest", gotLive.rawQuery)
	assert.Equal(t, "", gotLive.mode)

	select {
	case req := <-captured:
		// Stored request data describes the original request, not the rewrite.
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "/bookings/42", req.Path)
		assert.Equal(t, "", req.Headers.Get("Content-Type"))
		assert.Equal(t, proxy.ShadowHeaderValue, req.Headers.Get(proxy.ShadowHeader))
	case <-time.After(time.Second):
		t.Fatal("callback was not called within timeout")
	}
}

func TestProxy_ServeHTTP_skips_shadow_when_rewriter_panics(t *testing.T) {
	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer liveServer.Close()

	var shadowCalls atomic.Int32
	shadowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shadowCalls.Add(1)
	}))
	defer shadowServer.Close()

	liveURL, _ := url.Parse(liveServer.URL)
	shadowURL, _ := url.Parse(shadowServer.URL)
	p := proxy.NewProxy(liveURL, shadowURL,
		proxy.WithShadowRequestRewriter(func(*http.Request, []byte) (proxy.ShadowRequest, error) {
			panic("boom")
		}),
	)

	rec := httptest.NewRecorder()
	require.NotPanics(t, func() { p.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil)) })
	assert.Equal(t, http.StatusOK, rec.Code, "live traffic is unaffected")

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), shadowCalls.Load())
}

func TestProxy_ServeHTTP_skips_shadow_when_rewrite_fails(t *testing.T) {
	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write(b)
	}))
	defer liveServer.Close()

	var shadowCalls atomic.Int32
	shadowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shadowCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer shadowServer.Close()

	var callbackCalls atomic.Int32
	liveURL, _ := url.Parse(liveServer.URL)
	shadowURL, _ := url.Parse(shadowServer.URL)
	p := proxy.NewProxy(liveURL, shadowURL,
		proxy.WithShadowRequestRewriter(func(*http.Request, []byte) (proxy.ShadowRequest, error) {
			return proxy.ShadowRequest{}, errors.New("no route")
		}),
		proxy.WithCallbackFn(func(proxy.ProxyRequest, proxy.ProxyResponse, proxy.ProxyResponse) error {
			callbackCalls.Add(1)
			return nil
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/unmapped", strings.NewReader(`{"echo":true}`))
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.JSONEq(t, `{"echo":true}`, rec.Body.String(), "live still receives the buffered body")

	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(0), shadowCalls.Load(), "shadow must be skipped")
	assert.Equal(t, int32(0), callbackCalls.Load(), "no comparison runs")
}
