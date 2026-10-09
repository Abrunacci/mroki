package commands

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/pedrobarco/mroki/internal/domain/traffictesting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const bookingsMapping = `endpoint: /graphql
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

// bookingsMappingWithConversions is bookingsMapping with the id compared as a
// number and the check-in date compared as a date.
const bookingsMappingWithConversions = `endpoint: /graphql
routes:
  - match: GET /bookings/{id}
    query: |
      query GetBooking($id: ID!) { booking(id: $id) { id guestName checkIn } }
    variables:
      id: path.id
    response:
      root: data.booking
      fields:
        id: { from: id, as: number }
        guest_name: guestName
        check_in: { from: checkIn, as: date }
`

func gateRepoWithAdapter(t *testing.T) (*mockGateRepoForRequest, traffictesting.ShadowAdapter) {
	t.Helper()
	return gateRepoWithMapping(t, bookingsMapping)
}

func gateRepoWithMapping(t *testing.T, mapping string) (*mockGateRepoForRequest, traffictesting.ShadowAdapter) {
	t.Helper()
	adapter, err := traffictesting.ParseShadowAdapter("graphql", mapping)
	require.NoError(t, err)
	return &mockGateRepoForRequest{
		getByIDFn: func(_ context.Context, id traffictesting.GateID) (*traffictesting.Gate, error) {
			name, _ := traffictesting.ParseGateName("bookings")
			live, _ := traffictesting.ParseGateURL("http://live.example.com")
			shadow, _ := traffictesting.ParseGateURL("http://shadow.example.com")
			return traffictesting.NewGate(name, live, shadow,
				traffictesting.WithGateID(id),
				traffictesting.WithGateShadowAdapter(adapter),
			)
		},
	}, adapter
}

func bookingCommand(gateID traffictesting.GateID, path, liveBody, shadowBody string, liveStatus int) CreateRequestCommand {
	now := time.Now()
	return CreateRequestCommand{
		GateID:    gateID.String(),
		Method:    "GET",
		Path:      path,
		Headers:   map[string][]string{},
		CreatedAt: now,
		LiveResponse: CreateRequestResponseProps{
			StatusCode: liveStatus,
			Headers:    http.Header{"Content-Type": []string{"application/json"}, "X-Legacy": []string{"1"}},
			Body:       b64(liveBody),
			CreatedAt:  now,
		},
		ShadowResponse: CreateRequestResponseProps{
			StatusCode: 200,
			Headers:    http.Header{"Content-Type": []string{"application/graphql-response+json"}},
			Body:       b64(shadowBody),
			CreatedAt:  now,
		},
	}
}

func TestCreateRequestHandler_Handle_shadow_adapter_normalizes_and_compares_bodies_only(t *testing.T) {
	var saved *traffictesting.Request
	repo := &mockRequestRepository{saveFn: func(_ context.Context, r *traffictesting.Request) error {
		saved = r
		return nil
	}}
	gateRepo, adapter := gateRepoWithAdapter(t)
	handler := NewCreateRequestHandler(repo, gateRepo)

	cmd := bookingCommand(traffictesting.NewGateID(), "/bookings/1042",
		`{"id":1042,"guest_name":"Ana","check_in":"2026-10-09","legacy_code":"BK-1042"}`,
		`{"data":{"booking":{"id":"1042","guestName":"Ana","checkIn":"2026-10-09T00:00:00Z"}}}`,
		200)

	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.NotNil(t, saved)

	// Only the two real differences: no protocol noise (field names, headers, status).
	paths := make([]string, 0, len(saved.Diff.Content))
	for _, op := range saved.Diff.Content {
		paths = append(paths, op.Path)
	}
	assert.ElementsMatch(t, []string{"/body/id", "/body/check_in"}, paths)

	// What was compared is what is stored: both bodies in the REST shape.
	assert.JSONEq(t, `{"id":1042,"guest_name":"Ana","check_in":"2026-10-09"}`, string(saved.LiveResponse.Body))
	assert.JSONEq(t, `{"id":"1042","guest_name":"Ana","check_in":"2026-10-09T00:00:00Z"}`, string(saved.ShadowResponse.Body))

	// The diff records the mapping version it was computed with.
	assert.Equal(t, traffictesting.ShadowAdapterSnapshot{Type: "graphql", Version: adapter.Version()}, saved.Diff.ShadowAdapter)
}

func TestCreateRequestHandler_Handle_shadow_adapter_status_not_compared(t *testing.T) {
	var saved *traffictesting.Request
	repo := &mockRequestRepository{saveFn: func(_ context.Context, r *traffictesting.Request) error {
		saved = r
		return nil
	}}
	gateRepo, _ := gateRepoWithAdapter(t)
	handler := NewCreateRequestHandler(repo, gateRepo)

	// REST 404 vs GraphQL 200 with a null booking.
	cmd := bookingCommand(traffictesting.NewGateID(), "/bookings/9999",
		`{"error":"booking not found"}`, `{"data":{"booking":null}}`, 404)

	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)
	require.NotNil(t, saved)

	for _, op := range saved.Diff.Content {
		assert.NotEqual(t, "/statusCode", op.Path, "status codes are not compared")
		assert.NotContains(t, op.Path, "/headers", "headers are not compared")
	}
	assert.True(t, saved.Diff.HasContent(), "the body difference is still reported")
	assert.Equal(t, 404, saved.LiveResponse.StatusCode.Int(), "status codes are still stored")
	assert.JSONEq(t, `null`, string(saved.ShadowResponse.Body), "a null booking is stored as JSON null")
}

func TestCreateRequestHandler_Handle_without_shadow_adapter_has_no_snapshot(t *testing.T) {
	var saved *traffictesting.Request
	repo := &mockRequestRepository{saveFn: func(_ context.Context, r *traffictesting.Request) error {
		saved = r
		return nil
	}}
	handler := NewCreateRequestHandler(repo, &mockGateRepoForRequest{})

	cmd := bookingCommand(traffictesting.NewGateID(), "/bookings/1042", `{"id":1}`, `{"id":1}`, 200)

	_, err := handler.Handle(context.Background(), cmd)
	require.NoError(t, err)
	assert.True(t, saved.Diff.ShadowAdapter.IsZero())
}

func TestCreateRequestHandler_Handle_shadow_adapter_records_conversions(t *testing.T) {
	tests := []struct {
		name            string
		shadowBody      string
		wantPaths       []string
		wantShadow      string
		wantConversions []traffictesting.FieldConversion
	}{
		{
			name:       "declared conversions accept the type differences",
			shadowBody: `{"data":{"booking":{"id":"1042","guestName":"Ana","checkIn":"2026-10-09T00:00:00Z"}}}`,
			wantPaths:  []string{},
			wantShadow: `{"id":1042,"guest_name":"Ana","check_in":"2026-10-09"}`,
			wantConversions: []traffictesting.FieldConversion{
				{Field: "check_in", As: "date", Original: []byte(`"2026-10-09T00:00:00Z"`)},
				{Field: "id", As: "number", Original: []byte(`"1042"`)},
			},
		},
		{
			name:       "a value that cannot be converted is a difference",
			shadowBody: `{"data":{"booking":{"id":"Qm9va2luZzoxMDQy","guestName":"Ana","checkIn":"2026-10-09T00:00:00-03:00"}}}`,
			wantPaths:  []string{"/body/id", "/body/check_in"},
			wantShadow: `{"id":"Qm9va2luZzoxMDQy","guest_name":"Ana","check_in":"2026-10-09T00:00:00-03:00"}`,
			wantConversions: []traffictesting.FieldConversion{
				{Field: "check_in", As: "date", Original: []byte(`"2026-10-09T00:00:00-03:00"`), Error: `"2026-10-09T00:00:00-03:00" has offset -03:00, not UTC (Z)`},
				{Field: "id", As: "number", Original: []byte(`"Qm9va2luZzoxMDQy"`), Error: `"Qm9va2luZzoxMDQy" is not a number`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var saved *traffictesting.Request
			repo := &mockRequestRepository{saveFn: func(_ context.Context, r *traffictesting.Request) error {
				saved = r
				return nil
			}}
			gateRepo, _ := gateRepoWithMapping(t, bookingsMappingWithConversions)
			handler := NewCreateRequestHandler(repo, gateRepo)

			cmd := bookingCommand(traffictesting.NewGateID(), "/bookings/1042",
				`{"id":1042,"guest_name":"Ana","check_in":"2026-10-09"}`, tt.shadowBody, 200)

			_, err := handler.Handle(context.Background(), cmd)
			require.NoError(t, err)
			require.NotNil(t, saved)

			paths := make([]string, 0, len(saved.Diff.Content))
			for _, op := range saved.Diff.Content {
				paths = append(paths, op.Path)
			}
			assert.ElementsMatch(t, tt.wantPaths, paths)
			assert.JSONEq(t, tt.wantShadow, string(saved.ShadowResponse.Body), "the compared (converted) value is stored")
			assert.Equal(t, tt.wantConversions, saved.Diff.Conversions)
		})
	}
}
