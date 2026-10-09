package traffictesting_test

import (
	"testing"

	"github.com/pedrobarco/mroki/internal/domain/traffictesting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validGraphQLMapping = `endpoint: /graphql
routes:
  - match: GET /bookings/{id}
    query: |
      query GetBooking($id: ID!) { booking(id: $id) { id guestName } }
    variables:
      id: path.id
    response:
      root: data.booking
      fields:
        id: id
        guest_name: guestName
`

func TestParseShadowAdapter_graphql_valid(t *testing.T) {
	a, err := traffictesting.ParseShadowAdapter("graphql", validGraphQLMapping)

	require.NoError(t, err)
	assert.True(t, a.IsSet())
	assert.Equal(t, "graphql", a.Type())
	assert.Equal(t, validGraphQLMapping, a.Config())
	assert.Len(t, a.Version(), 12)
	assert.Equal(t, traffictesting.ShadowAdapterSnapshot{Type: "graphql", Version: a.Version()}, a.Snapshot())
}

func TestParseShadowAdapter_invalid(t *testing.T) {
	tests := []struct {
		name   string
		typ    string
		config string
	}{
		{name: "missing type", typ: "", config: validGraphQLMapping},
		{name: "unsupported type", typ: "soap", config: validGraphQLMapping},
		{name: "invalid yaml", typ: "graphql", config: "endpoint: [unclosed"},
		{name: "unknown key", typ: "graphql", config: "endpoint: /graphql\nroutez: []\n"},
		{name: "no routes", typ: "graphql", config: "endpoint: /graphql\n"},
		{name: "conflicting routes", typ: "graphql", config: validGraphQLMapping + `  - match: GET /bookings/{bookingId}
    query: query Q($id: ID!) { booking(id: $id) { id } }
    variables:
      id: path.bookingId
    response:
      root: data.booking
      fields:
        id: id
`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := traffictesting.ParseShadowAdapter(tt.typ, tt.config)

			require.Error(t, err)
			assert.ErrorIs(t, err, traffictesting.ErrInvalidShadowAdapter)
			assert.False(t, a.IsSet())
		})
	}
}

func TestShadowAdapter_Version(t *testing.T) {
	a, err := traffictesting.ParseShadowAdapter("graphql", validGraphQLMapping)
	require.NoError(t, err)
	same, err := traffictesting.ParseShadowAdapter("graphql", validGraphQLMapping)
	require.NoError(t, err)
	changed, err := traffictesting.ParseShadowAdapter("graphql", validGraphQLMapping+"# edited\n")
	require.NoError(t, err)

	assert.Equal(t, a.Version(), same.Version(), "same mapping, same version")
	assert.NotEqual(t, a.Version(), changed.Version(), "any edit changes the version")
}

func TestNoShadowAdapter(t *testing.T) {
	a := traffictesting.NoShadowAdapter()

	assert.False(t, a.IsSet())
	assert.Empty(t, a.Version())
	assert.True(t, a.Snapshot().IsZero())
}
