package handlers

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pedrobarco/mroki/internal/domain/traffictesting"
	"github.com/pedrobarco/mroki/pkg/client"
	"github.com/pedrobarco/mroki/pkg/shadowadapter/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bookingsMapping = `
endpoint: /graphql
routes:
  - match: GET /bookings/{id}
    query: |
      query GetBooking($id: ID!) { booking(id: $id) { id guestName checkIn } }
    variables:
      id: path.id
    response:
      root: data.booking
      fields:
        id: id
        guest_name: guestName
        check_in: checkIn
`

// TestProxy_graphql_adapter_end_to_end runs the standalone proxy against a fake
// REST live service and a fake GraphQL shadow service, and checks that the
// translated query reaches shadow and that only real differences are logged.
func TestProxy_graphql_adapter_end_to_end(t *testing.T) {
	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Legacy", "true")
		_, _ = w.Write([]byte(`{"id":1042,"guest_name":"Ana","check_in":"2026-10-09","internal_code":"A7"}`))
	}))
	t.Cleanup(liveServer.Close)

	type graphQLRequest struct {
		path      string
		query     string
		variables map[string]any
	}
	shadowRequests := make(chan graphQLRequest, 1)
	shadowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		shadowRequests <- graphQLRequest{path: r.Method + " " + r.URL.Path, query: body.Query, variables: body.Variables}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"booking":{"id":"1042","guestName":"Ana","checkIn":"2026-10-09"}}}`))
	}))
	t.Cleanup(shadowServer.Close)

	gqlCfg, err := graphql.ParseConfig([]byte(bookingsMapping))
	require.NoError(t, err)
	adapter, err := graphql.New(gqlCfg)
	require.NoError(t, err)

	liveURL, _ := url.Parse(liveServer.URL)
	shadowURL, _ := url.Parse(shadowServer.URL)
	logs := &syncBuffer{}
	handler := Proxy(ProxyConfig{
		Live:          liveURL,
		Shadow:        shadowURL,
		LiveTimeout:   5 * time.Second,
		ShadowTimeout: 5 * time.Second,
		Logger:        slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		Redactor:      traffictesting.NewRedactor(traffictesting.DefaultRedactedFields().AllFields()),
		ShadowAdapter: adapter,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/bookings/1042", nil))

	// The client always gets the live REST response.
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"guest_name":"Ana"`)

	select {
	case got := <-shadowRequests:
		assert.Equal(t, "POST /graphql", got.path)
		assert.Contains(t, got.query, "booking(id: $id)")
		assert.Equal(t, map[string]any{"id": "1042"}, got.variables)
	case <-time.After(time.Second):
		t.Fatal("shadow GraphQL service was not called")
	}

	// Only the id type differs: the unmapped live field, the different headers
	// and the GraphQL envelope must not show up.
	assert.Eventually(t, func() bool {
		return strings.Contains(logs.String(), "response diff detected")
	}, time.Second, 10*time.Millisecond, "expected the standalone callback to log a diff")
	out := logs.String()
	assert.Contains(t, out, "changes=1")
	assert.Contains(t, out, `replace /body/id: \"1042\"`)
	assert.NotContains(t, out, "internal_code")
	assert.NotContains(t, out, "/headers")
	assert.NotContains(t, out, "/statusCode")
}

func TestProxy_graphql_adapter_skips_unmapped_routes(t *testing.T) {
	gqlCfg, err := graphql.ParseConfig([]byte(bookingsMapping))
	require.NoError(t, err)
	adapter, err := graphql.New(gqlCfg)
	require.NoError(t, err)

	handler, hits := newWiringHarness(t, ProxyConfig{ShadowAdapter: adapter})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/guests/7", nil))

	assert.Equal(t, http.StatusOK, rec.Code, "live traffic is unaffected")
	assertNotShadowed(t, hits)
}

// TestProxy_graphql_adapter_api_mode checks that in API mode the proxy rewrites
// the shadow request with the gate's mapping and forwards the original REST
// request plus the raw GraphQL response to mroki-api, which normalizes it.
func TestProxy_graphql_adapter_api_mode(t *testing.T) {
	liveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":1042,"guest_name":"Ana"}`))
	}))
	t.Cleanup(liveServer.Close)

	shadowPaths := make(chan string, 1)
	shadowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		shadowPaths <- r.Method + " " + r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"booking":{"id":"1042","guestName":"Ana"}}}`))
	}))
	t.Cleanup(shadowServer.Close)

	captured := make(chan client.CapturedRequest, 1)
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c client.CapturedRequest
		if err := json.NewDecoder(r.Body).Decode(&c); err == nil {
			captured <- c
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(apiServer.Close)

	gqlCfg, err := graphql.ParseConfig([]byte(bookingsMapping))
	require.NoError(t, err)
	adapter, err := graphql.New(gqlCfg)
	require.NoError(t, err)

	liveURL, _ := url.Parse(liveServer.URL)
	shadowURL, _ := url.Parse(shadowServer.URL)
	apiURL, _ := url.Parse(apiServer.URL)
	handler := Proxy(ProxyConfig{
		Live:          liveURL,
		Shadow:        shadowURL,
		LiveTimeout:   5 * time.Second,
		ShadowTimeout: 5 * time.Second,
		APIClient:     client.NewMrokiClient(apiURL, "gate-test"),
		ShadowAdapter: adapter,
	})

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/bookings/1042", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	select {
	case got := <-shadowPaths:
		assert.Equal(t, "POST /graphql", got)
	case <-time.After(time.Second):
		t.Fatal("shadow GraphQL service was not called")
	}

	select {
	case got := <-captured:
		assert.Equal(t, "GET", got.Method)
		assert.Equal(t, "/bookings/1042", got.Path, "the API gets the original REST request")
		assert.Nil(t, got.Diff, "the diff is computed by the API")
		shadowBody, err := base64.StdEncoding.DecodeString(got.ShadowResponse.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{"data":{"booking":{"id":"1042","guestName":"Ana"}}}`, string(shadowBody), "the API gets the raw GraphQL response")
	case <-time.After(time.Second):
		t.Fatal("the API was not called")
	}
}
