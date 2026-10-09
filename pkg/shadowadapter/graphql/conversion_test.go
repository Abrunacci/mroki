package graphql_test

import (
	"net/http"
	"testing"

	"github.com/pedrobarco/mroki/pkg/shadowadapter/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfig_field_forms(t *testing.T) {
	cfg, err := graphql.ParseConfig([]byte(`
endpoint: /graphql
routes:
  - match: GET /bookings/{id}
    query: "query Q($id: ID!) { booking(id: $id) { id guestName checkIn } }"
    variables:
      id: path.id
    response:
      root: data.booking
      fields:
        id: { from: id, as: number }
        guest_name: guestName
        check_in:
          from: checkIn
          as: date
        status: { from: status }
`))
	require.NoError(t, err)

	assert.Equal(t, map[string]graphql.FieldMapping{
		"id":         {From: "id", As: graphql.AsNumber},
		"guest_name": {From: "guestName"},
		"check_in":   {From: "checkIn", As: graphql.AsDate},
		"status":     {From: "status"},
	}, cfg.Routes[0].Response.Fields)
}

func TestParseConfig_field_errors(t *testing.T) {
	config := func(field string) string {
		return `
endpoint: /graphql
routes:
  - match: GET /bookings/{id}
    query: "{ booking { id } }"
    response:
      root: data.booking
      fields:
        id: ` + field + "\n"
	}

	tests := []struct {
		name    string
		field   string
		wantErr string
	}{
		{name: "unsupported type", field: "{ from: id, as: numbr }", wantErr: `response.fields["id"].as: unsupported type "numbr" (supported: number, string, boolean, date)`},
		{name: "unknown key", field: "{ form: id, as: number }", wantErr: `unknown key "form" in field mapping`},
		{name: "missing from", field: "{ as: number }", wantErr: `response.fields["id"]: must not be empty`},
		{name: "list instead of field", field: "[id, number]", wantErr: "a field mapping must be a GraphQL field name or {from: <field>, as: <type>}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := graphql.ParseConfig([]byte(config(tt.field)))
			require.Error(t, err)
			assert.ErrorIs(t, err, graphql.ErrInvalidConfig)
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

// normalizeField maps the REST field "v" to the GraphQL field "v" converted
// with as, and normalizes a shadow body whose "v" is shadowValue (raw JSON).
func normalizeField(t *testing.T, as graphql.ConversionType, shadowValue string) graphql.Normalized {
	t.Helper()
	a, err := graphql.New(graphql.Config{
		Endpoint: "/graphql",
		Routes: []graphql.RouteConfig{{
			Match: "GET /items/{id}",
			Query: "{ item { v } }",
			Response: graphql.ResponseConfig{
				Root:   "data.item",
				Fields: map[string]graphql.FieldMapping{"v": {From: "v", As: as}},
			},
		}},
	})
	require.NoError(t, err)

	got, err := a.Normalize(http.MethodGet, "/items/1", []byte(`{}`), []byte(`{"data":{"item":{"v":`+shadowValue+`}}}`))
	require.NoError(t, err)
	return got
}

func TestAdapter_Normalize_conversions(t *testing.T) {
	tests := []struct {
		as      graphql.ConversionType
		shadow  string
		want    string // normalized shadow value (raw JSON)
		wantErr string // empty when the conversion succeeds
	}{
		// number
		{as: graphql.AsNumber, shadow: `"1042"`, want: `1042`},
		{as: graphql.AsNumber, shadow: `"450.50"`, want: `450.5`},
		{as: graphql.AsNumber, shadow: `"-3e2"`, want: `-300`},
		{as: graphql.AsNumber, shadow: `1042`, want: `1042`},
		{as: graphql.AsNumber, shadow: `"Qm9va2luZzoxMDQ0"`, wantErr: `"Qm9va2luZzoxMDQ0" is not a number`},
		{as: graphql.AsNumber, shadow: `" 1042"`, wantErr: `" 1042" is not a number`},
		{as: graphql.AsNumber, shadow: `"1,042"`, wantErr: `"1,042" is not a number`},
		{as: graphql.AsNumber, shadow: `"0x10"`, wantErr: `"0x10" is not a number`},
		{as: graphql.AsNumber, shadow: `"007"`, wantErr: `"007" is not a number`},
		{as: graphql.AsNumber, shadow: `""`, wantErr: `"" is not a number`},
		{as: graphql.AsNumber, shadow: `true`, wantErr: `true is not a number`},
		// string
		{as: graphql.AsString, shadow: `1042`, want: `"1042"`},
		{as: graphql.AsString, shadow: `450.5`, want: `"450.5"`},
		{as: graphql.AsString, shadow: `false`, want: `"false"`},
		{as: graphql.AsString, shadow: `"abc"`, want: `"abc"`},
		{as: graphql.AsString, shadow: `{"a":1}`, wantErr: `an object cannot be converted to text`},
		// boolean
		{as: graphql.AsBoolean, shadow: `"true"`, want: `true`},
		{as: graphql.AsBoolean, shadow: `"TRUE"`, want: `true`},
		{as: graphql.AsBoolean, shadow: `"1"`, want: `true`},
		{as: graphql.AsBoolean, shadow: `"S"`, want: `true`},
		{as: graphql.AsBoolean, shadow: `"Sí"`, want: `true`},
		{as: graphql.AsBoolean, shadow: `"y"`, want: `true`},
		{as: graphql.AsBoolean, shadow: `"false"`, want: `false`},
		{as: graphql.AsBoolean, shadow: `"0"`, want: `false`},
		{as: graphql.AsBoolean, shadow: `"N"`, want: `false`},
		{as: graphql.AsBoolean, shadow: `1`, want: `true`},
		{as: graphql.AsBoolean, shadow: `0`, want: `false`},
		{as: graphql.AsBoolean, shadow: `true`, want: `true`},
		{as: graphql.AsBoolean, shadow: `"2"`, wantErr: `"2" is not a boolean`},
		{as: graphql.AsBoolean, shadow: `" true"`, wantErr: `" true" is not a boolean`},
		{as: graphql.AsBoolean, shadow: `"verdadero"`, wantErr: `"verdadero" is not a boolean`},
		{as: graphql.AsBoolean, shadow: `2`, wantErr: `2 is not a boolean (only 1 and 0 are)`},
		// date
		{as: graphql.AsDate, shadow: `"2026-10-09"`, want: `"2026-10-09"`},
		{as: graphql.AsDate, shadow: `"2026-10-09T00:00:00Z"`, want: `"2026-10-09"`},
		{as: graphql.AsDate, shadow: `"2026-10-09T00:00:00.000Z"`, want: `"2026-10-09"`},
		{as: graphql.AsDate, shadow: `"2026-10-09T00:00:00"`, want: `"2026-10-09"`},
		{as: graphql.AsDate, shadow: `"2026-10-09T15:30:00Z"`, wantErr: `"2026-10-09T15:30:00Z" is not at midnight`},
		{as: graphql.AsDate, shadow: `"2026-10-09T00:00:00-03:00"`, wantErr: `"2026-10-09T00:00:00-03:00" has offset -03:00, not UTC (Z)`},
		{as: graphql.AsDate, shadow: `"2026-10-09T00:00:00+00:00"`, wantErr: `has offset +00:00, not UTC (Z)`},
		{as: graphql.AsDate, shadow: `"09/10/2026"`, wantErr: `"09/10/2026" is not a date`},
		{as: graphql.AsDate, shadow: `"2026-02-30"`, wantErr: `"2026-02-30" is not a date`},
		{as: graphql.AsDate, shadow: `20261009`, wantErr: `20261009 is not a date`},
	}

	for _, tt := range tests {
		t.Run(string(tt.as)+" "+tt.shadow, func(t *testing.T) {
			got := normalizeField(t, tt.as, tt.shadow)

			require.Len(t, got.Conversions, 1)
			c := got.Conversions[0]
			assert.Equal(t, "v", c.Field)
			assert.Equal(t, tt.as, c.As)

			if tt.wantErr == "" {
				assert.True(t, c.OK(), "unexpected error: %s", c.Error)
				assert.JSONEq(t, `{"v":`+tt.want+`}`, string(got.Shadow))
				return
			}
			assert.False(t, c.OK())
			assert.Contains(t, c.Error, tt.wantErr)
			// A failed conversion keeps the value as received, so the diff shows it.
			assert.JSONEq(t, `{"v":`+tt.shadow+`}`, string(got.Shadow))
		})
	}
}

func TestAdapter_Normalize_conversion_records_original(t *testing.T) {
	got := normalizeField(t, graphql.AsNumber, `"1042"`)
	assert.Equal(t, []graphql.Conversion{{Field: "v", As: graphql.AsNumber, Original: "1042"}}, got.Conversions)
}

func TestAdapter_Normalize_conversion_skips_null(t *testing.T) {
	got := normalizeField(t, graphql.AsNumber, `null`)
	assert.Empty(t, got.Conversions, "null is compared as is, not converted")
	assert.JSONEq(t, `{"v":null}`, string(got.Shadow))
}
