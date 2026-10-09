package graphql_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pedrobarco/mroki/pkg/shadowadapter/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBookingsAdapter(t *testing.T) *graphql.Adapter {
	t.Helper()
	cfg := graphql.Config{
		Endpoint: "/graphql",
		Routes: []graphql.RouteConfig{
			{
				Match:     "GET /bookings/{id}",
				Query:     "query GetBooking($id: ID!) { booking(id: $id) { id guestName stay { checkIn } } }",
				Variables: map[string]string{"id": "path.id"},
				Response: graphql.ResponseConfig{
					Root: "data.booking",
					Fields: map[string]graphql.FieldMapping{
						"id":             {From: "id"},
						"guest_name":     {From: "guestName"},
						"dates.check_in": {From: "stay.checkIn"},
					},
				},
			},
			{
				Match: "GET /bookings/featured",
				Query: "{ featuredBooking { id } }",
				Response: graphql.ResponseConfig{
					Root:   "data.featuredBooking",
					Fields: map[string]graphql.FieldMapping{"id": {From: "id"}},
				},
			},
		},
	}
	a, err := graphql.New(cfg)
	require.NoError(t, err)
	return a
}

func TestNew_rejects_conflicting_patterns(t *testing.T) {
	route := graphql.RouteConfig{
		Query:    "{ a }",
		Response: graphql.ResponseConfig{Root: "data.a", Fields: map[string]graphql.FieldMapping{"a": {From: "a"}}},
	}
	r1, r2 := route, route
	r1.Match = "GET /bookings/{id}"
	r2.Match = "GET /bookings/{bookingId}"

	_, err := graphql.New(graphql.Config{Endpoint: "/graphql", Routes: []graphql.RouteConfig{r1, r2}})
	require.Error(t, err)
	assert.ErrorIs(t, err, graphql.ErrInvalidConfig)
	assert.Contains(t, err.Error(), "conflicts with")
}

func TestAdapter_RewriteRequest(t *testing.T) {
	a := newBookingsAdapter(t)

	tests := []struct {
		name     string
		method   string
		target   string
		wantBody string
	}{
		{
			name:     "path parameter becomes a variable",
			method:   http.MethodGet,
			target:   "/bookings/1042?verbose=true",
			wantBody: `{"query":"query GetBooking($id: ID!) { booking(id: $id) { id guestName stay { checkIn } } }","variables":{"id":"1042"}}`,
		},
		{
			name:     "literal segment wins over wildcard",
			method:   http.MethodGet,
			target:   "/bookings/featured",
			wantBody: `{"query":"{ featuredBooking { id } }"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, nil)
			req.Header.Set("Authorization", "Bearer token")
			req.Header.Set("Content-Length", "0")

			got, err := a.RewriteRequest(req, nil)
			require.NoError(t, err)

			assert.Equal(t, http.MethodPost, got.Method)
			assert.Equal(t, "/graphql", got.Path)
			assert.Equal(t, "", got.RawQuery)
			assert.JSONEq(t, tt.wantBody, string(got.Body))
			assert.Equal(t, "application/json", got.Header.Get("Content-Type"))
			assert.Equal(t, "application/json", got.Header.Get("Accept"))
			assert.Equal(t, "Bearer token", got.Header.Get("Authorization"), "client headers are forwarded")
			assert.Empty(t, got.Header.Get("Content-Length"))

			// The original request is left untouched.
			assert.Equal(t, tt.method, req.Method)
			assert.Empty(t, req.Header.Get("Accept"))
		})
	}
}

func TestAdapter_RewriteRequest_no_route(t *testing.T) {
	a := newBookingsAdapter(t)

	for _, tc := range []struct{ method, target string }{
		{http.MethodGet, "/guests/7"},
		{http.MethodPost, "/bookings/1042"},
		{http.MethodHead, "/bookings/1042"},
		{http.MethodGet, "/bookings/1042/extra"},
	} {
		t.Run(tc.method+" "+tc.target, func(t *testing.T) {
			_, err := a.RewriteRequest(httptest.NewRequest(tc.method, tc.target, nil), nil)
			assert.ErrorIs(t, err, graphql.ErrNoRoute)
		})
	}
}

func TestAdapter_Normalize(t *testing.T) {
	a := newBookingsAdapter(t)

	tests := []struct {
		name       string
		live       string
		shadow     string
		wantLive   string
		wantShadow string
	}{
		{
			name:       "renames fields and keeps only mapped ones",
			live:       `{"id":1042,"guest_name":"Ana","dates":{"check_in":"2026-10-09","check_out":"2026-10-12"},"internal":"x"}`,
			shadow:     `{"data":{"booking":{"id":"1042","guestName":"Ana","stay":{"checkIn":"2026-10-09T00:00:00Z"},"extra":1}}}`,
			wantLive:   `{"id":1042,"guest_name":"Ana","dates":{"check_in":"2026-10-09"}}`,
			wantShadow: `{"id":"1042","guest_name":"Ana","dates":{"check_in":"2026-10-09T00:00:00Z"}}`,
		},
		{
			name:       "missing GraphQL field is omitted",
			live:       `{"id":1042,"guest_name":"Ana"}`,
			shadow:     `{"data":{"booking":{"id":"1042"}}}`,
			wantLive:   `{"id":1042,"guest_name":"Ana"}`,
			wantShadow: `{"id":"1042"}`,
		},
		{
			name:       "null root becomes null",
			live:       `{"id":1042}`,
			shadow:     `{"data":{"booking":null},"errors":[{"message":"not found"}]}`,
			wantLive:   `{"id":1042}`,
			wantShadow: `null`,
		},
		{
			name:       "missing root becomes null",
			live:       `{"id":1042}`,
			shadow:     `{"errors":[{"message":"boom"}]}`,
			wantLive:   `{"id":1042}`,
			wantShadow: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := a.Normalize(http.MethodGet, "/bookings/1042", []byte(tt.live), []byte(tt.shadow))
			require.NoError(t, err)
			assert.JSONEq(t, tt.wantLive, string(got.Live))
			assert.JSONEq(t, tt.wantShadow, string(got.Shadow))
			assert.Empty(t, got.Conversions, "no field declares a conversion")
		})
	}
}

func TestAdapter_Normalize_errors(t *testing.T) {
	a := newBookingsAdapter(t)

	_, err := a.Normalize(http.MethodGet, "/guests/7", []byte(`{}`), []byte(`{}`))
	assert.ErrorIs(t, err, graphql.ErrNoRoute)

	_, err = a.Normalize(http.MethodGet, "/bookings/1042", []byte(`not json`), []byte(`{}`))
	assert.ErrorContains(t, err, "live body is not valid JSON")

	_, err = a.Normalize(http.MethodGet, "/bookings/1042", []byte(`{}`), []byte(`<html>`))
	assert.ErrorContains(t, err, "shadow body is not valid JSON")
}
