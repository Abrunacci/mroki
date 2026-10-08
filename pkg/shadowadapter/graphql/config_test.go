package graphql_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pedrobarco/mroki/pkg/shadowadapter/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validConfig = `
endpoint: /graphql
routes:
  - match: GET /bookings/{id}
    query: |
      query GetBooking($id: ID!) {
        booking(id: $id) { id guestName }
      }
    variables:
      id: path.id
    response:
      root: data.booking
      fields:
        id: id
        guest_name: guestName
`

func TestParseConfig_valid(t *testing.T) {
	cfg, err := graphql.ParseConfig([]byte(validConfig))
	require.NoError(t, err)

	assert.Equal(t, "/graphql", cfg.Endpoint)
	require.Len(t, cfg.Routes, 1)
	r := cfg.Routes[0]
	assert.Equal(t, "GET /bookings/{id}", r.Match)
	assert.Contains(t, r.Query, "booking(id: $id)")
	assert.Equal(t, map[string]string{"id": "path.id"}, r.Variables)
	assert.Equal(t, "data.booking", r.Response.Root)
	assert.Equal(t, map[string]string{"id": "id", "guest_name": "guestName"}, r.Response.Fields)
}

func TestParseConfig_invalid(t *testing.T) {
	route := func(match, query, variables, root, fields string) string {
		return "endpoint: /graphql\nroutes:\n" +
			"  - match: " + match + "\n" +
			"    query: " + query + "\n" +
			"    variables: " + variables + "\n" +
			"    response:\n" +
			"      root: " + root + "\n" +
			"      fields: " + fields + "\n"
	}
	okQuery := `"query Q($id: ID!) { booking(id: $id) { id } }"`

	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "malformed yaml",
			yaml:    "endpoint: [",
			wantErr: "invalid graphql mapping config",
		},
		{
			name:    "unknown key",
			yaml:    validConfig + "extra: true\n",
			wantErr: "field extra not found",
		},
		{
			name:    "endpoint without leading slash",
			yaml:    "endpoint: graphql\nroutes: []\n",
			wantErr: `endpoint must start with "/"`,
		},
		{
			name:    "no routes",
			yaml:    "endpoint: /graphql\n",
			wantErr: "at least one route is required",
		},
		{
			name:    "match without method",
			yaml:    route("/bookings/{id}", okQuery, "{id: path.id}", "data.booking", "{id: id}"),
			wantErr: `match must be "METHOD /path"`,
		},
		{
			name:    "empty query",
			yaml:    route("GET /bookings/{id}", `""`, "{id: path.id}", "data.booking", "{id: id}"),
			wantErr: "query must not be empty",
		},
		{
			name:    "mutation rejected",
			yaml:    route("GET /bookings/{id}", `"mutation Cancel($id: ID!) { cancel(id: $id) { id } }"`, "{id: path.id}", "data.cancel", "{id: id}"),
			wantErr: "only queries are supported, got a mutation",
		},
		{
			name:    "subscription rejected",
			yaml:    route("GET /bookings/{id}", `"subscription { bookingChanged { id } }"`, "{}", "data.bookingChanged", "{id: id}"),
			wantErr: "only queries are supported, got a subscription",
		},
		{
			name:    "unsupported variable source",
			yaml:    route("GET /bookings/{id}", okQuery, "{id: query.id}", "data.booking", "{id: id}"),
			wantErr: `unsupported source "query.id"`,
		},
		{
			name:    "variable from unknown path parameter",
			yaml:    route("GET /bookings/{id}", okQuery, "{id: path.bookingId}", "data.booking", "{id: id}"),
			wantErr: "path parameter {bookingId} is not in the route pattern",
		},
		{
			name:    "missing root",
			yaml:    route("GET /bookings/{id}", okQuery, "{id: path.id}", `""`, "{id: id}"),
			wantErr: "response.root: must not be empty",
		},
		{
			name:    "no fields",
			yaml:    route("GET /bookings/{id}", okQuery, "{id: path.id}", "data.booking", "{}"),
			wantErr: "response.fields must map at least one field",
		},
		{
			name:    "array wildcard not supported yet",
			yaml:    route("GET /bookings/{id}", okQuery, "{id: path.id}", "data.booking", "{guests.#.name: guests.#.name}"),
			wantErr: "array wildcards are not supported yet",
		},
		{
			name:    "empty path segment",
			yaml:    route("GET /bookings/{id}", okQuery, "{id: path.id}", "data..booking", "{id: id}"),
			wantErr: "empty segment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := graphql.ParseConfig([]byte(tt.yaml))
			require.Error(t, err)
			assert.ErrorIs(t, err, graphql.ErrInvalidConfig)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestParseConfig_query_operation_detection(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "named query", query: "query Q { a }"},
		{name: "shorthand", query: "{ a }"},
		{name: "leading comment", query: "# mutation in a comment\nquery Q { a }"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := graphql.Config{
				Endpoint: "/graphql",
				Routes: []graphql.RouteConfig{{
					Match:    "GET /a",
					Query:    tt.query,
					Response: graphql.ResponseConfig{Root: "data.a", Fields: map[string]string{"a": "a"}},
				}},
			}
			assert.NoError(t, cfg.Validate())
		})
	}
}

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mapping.yaml")
	require.NoError(t, os.WriteFile(path, []byte(validConfig), 0o600))

	cfg, err := graphql.LoadConfig(path)
	require.NoError(t, err)
	assert.Len(t, cfg.Routes, 1)

	_, err = graphql.LoadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	assert.ErrorContains(t, err, "read graphql mapping config")
}
