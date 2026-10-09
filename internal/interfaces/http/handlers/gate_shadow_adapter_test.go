package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pedrobarco/mroki/internal/application/commands"
	"github.com/pedrobarco/mroki/internal/domain/traffictesting"
	"github.com/pedrobarco/mroki/pkg/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const handlerTestMapping = "endpoint: /graphql\nroutes:\n  - match: GET /bookings/{id}\n    query: |\n      query Q($id: ID!) { booking(id: $id) { id } }\n    variables:\n      id: path.id\n    response:\n      root: data.booking\n      fields:\n        id: id\n"

func createGateBody(t *testing.T, adapter any) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"name":           "bookings",
		"live_url":       "http://legacy:9001",
		"shadow_url":     "http://graphql:9002",
		"shadow_adapter": adapter,
	})
	require.NoError(t, err)
	return string(b)
}

func TestCreateGate_WithShadowAdapter(t *testing.T) {
	var saved *traffictesting.Gate
	repo := &mockGateRepository{saveFunc: func(_ context.Context, g *traffictesting.Gate) error {
		saved = g
		return nil
	}}
	body := createGateBody(t, map[string]string{"type": "graphql", "config": handlerTestMapping})
	req := httptest.NewRequest(http.MethodPost, "/gates", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	err := CreateGate(commands.NewCreateGateHandler(repo))(rec, req)

	require.NoError(t, err)
	assert.Equal(t, http.StatusCreated, rec.Code)
	require.NotNil(t, saved)
	var response dto.Response[dto.Gate]
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&response))
	require.NotNil(t, response.Data.ShadowAdapter)
	assert.Equal(t, "graphql", response.Data.ShadowAdapter.Type)
	assert.Equal(t, handlerTestMapping, response.Data.ShadowAdapter.Config)
	assert.Equal(t, saved.ShadowAdapter.Version(), response.Data.ShadowAdapter.Version)
}

func TestCreateGate_WithoutShadowAdapter_IsNull(t *testing.T) {
	repo := &mockGateRepository{saveFunc: func(_ context.Context, _ *traffictesting.Gate) error { return nil }}
	body := `{"name":"plain","live_url":"http://live.example.com","shadow_url":"http://shadow.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/gates", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	require.NoError(t, CreateGate(commands.NewCreateGateHandler(repo))(rec, req))

	var raw map[string]any
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&raw))
	data := raw["data"].(map[string]any)
	assert.Contains(t, data, "shadow_adapter")
	assert.Nil(t, data["shadow_adapter"])
}

func TestCreateGate_InvalidShadowAdapter(t *testing.T) {
	repo := &mockGateRepository{saveFunc: func(_ context.Context, _ *traffictesting.Gate) error { return nil }}
	body := createGateBody(t, map[string]string{"type": "graphql", "config": "endpoint: graphql\n"})
	req := httptest.NewRequest(http.MethodPost, "/gates", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	err := CreateGate(commands.NewCreateGateHandler(repo))(rec, req)

	apiErr, ok := err.(*dto.APIError)
	require.True(t, ok, "expected APIError, got %T", err)
	assert.Equal(t, http.StatusBadRequest, apiErr.Status)
	assert.Equal(t, "Invalid Shadow Adapter", apiErr.Title)
}

func TestUpdateGate_ShadowAdapter(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantSet    bool
	}{
		{name: "set", body: `{"shadow_adapter":{"type":"graphql","config":` + jsonString(handlerTestMapping) + `}}`, wantStatus: http.StatusOK, wantSet: true},
		{name: "null removes it", body: `{"shadow_adapter":null}`, wantStatus: http.StatusOK, wantSet: false},
		{name: "missing type", body: `{"shadow_adapter":{"config":"x"}}`, wantStatus: http.StatusBadRequest},
		{name: "invalid config", body: `{"shadow_adapter":{"type":"graphql","config":"nope"}}`, wantStatus: http.StatusBadRequest},
		{name: "wrong JSON type", body: `{"shadow_adapter":"graphql"}`, wantStatus: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existingGate, repo, updated := newRetentionGate(t)
			handler := commands.NewUpdateGateHandler(repo, 720*time.Hour)

			rec, err := patchGate(t, handler, existingGate.ID.String(), tt.body)

			if tt.wantStatus != http.StatusOK {
				apiErr, ok := err.(*dto.APIError)
				require.True(t, ok, "expected APIError, got %T", err)
				assert.Equal(t, tt.wantStatus, apiErr.Status)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, rec.Code)
			require.NotNil(t, *updated)
			assert.Equal(t, tt.wantSet, (*updated).ShadowAdapter.IsSet())
		})
	}
}

func TestUpdateGate_ShadowAdapter_AbsentLeavesUnchanged(t *testing.T) {
	existingGate, repo, updated := newRetentionGate(t)
	adapter, err := traffictesting.ParseShadowAdapter("graphql", handlerTestMapping)
	require.NoError(t, err)
	existingGate.ShadowAdapter = adapter
	handler := commands.NewUpdateGateHandler(repo, 720*time.Hour)

	_, err = patchGate(t, handler, existingGate.ID.String(), `{"name":"renamed"}`)

	require.NoError(t, err)
	assert.Equal(t, adapter.Version(), (*updated).ShadowAdapter.Version())
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestToFullRequestResponseDTO_DiffShadowAdapter(t *testing.T) {
	req := &traffictesting.Request{
		Diff: traffictesting.Diff{ShadowAdapter: traffictesting.ShadowAdapterSnapshot{Type: "graphql", Version: "0123456789ab"}},
	}
	assert.Equal(t, &dto.DiffShadowAdapter{Type: "graphql", Version: "0123456789ab"}, toFullRequestResponseDTO(req).Diff.ShadowAdapter)
	assert.Equal(t, &dto.DiffShadowAdapter{Type: "graphql", Version: "0123456789ab"}, toRequestResponseDTO(req).ShadowAdapter)

	req.Diff.ShadowAdapter = traffictesting.ShadowAdapterSnapshot{}
	assert.Nil(t, toFullRequestResponseDTO(req).Diff.ShadowAdapter)
	assert.Nil(t, toRequestResponseDTO(req).ShadowAdapter)
}
