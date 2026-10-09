package commands

import (
	"context"
	"testing"

	"github.com/pedrobarco/mroki/internal/domain/traffictesting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateGateHandler_Handle_with_shadow_adapter(t *testing.T) {
	var saved *traffictesting.Gate
	repo := &mockGateRepository{saveFn: func(_ context.Context, g *traffictesting.Gate) error {
		saved = g
		return nil
	}}
	handler := NewCreateGateHandler(repo)

	gate, err := handler.Handle(context.Background(), CreateGateCommand{
		Name:          "bookings",
		LiveURL:       "http://legacy:9001",
		ShadowURL:     "http://graphql:9002",
		ShadowAdapter: &ShadowAdapterProps{Type: "graphql", Config: bookingsMapping},
	})

	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, "graphql", gate.ShadowAdapter.Type())
	assert.Equal(t, bookingsMapping, gate.ShadowAdapter.Config())
}

func TestCreateGateHandler_Handle_invalid_shadow_adapter(t *testing.T) {
	handler := NewCreateGateHandler(&mockGateRepository{})

	_, err := handler.Handle(context.Background(), CreateGateCommand{
		Name:          "bookings",
		LiveURL:       "http://legacy:9001",
		ShadowURL:     "http://graphql:9002",
		ShadowAdapter: &ShadowAdapterProps{Type: "graphql", Config: "endpoint: graphql\n"},
	})

	assert.ErrorIs(t, err, traffictesting.ErrInvalidShadowAdapter)
}

func TestUpdateGateHandler_shadow_adapter(t *testing.T) {
	tests := []struct {
		name     string
		props    *ShadowAdapterProps
		existing bool
		wantSet  bool
		wantErr  error
	}{
		{name: "absent leaves it unchanged", props: nil, existing: true, wantSet: true},
		{name: "set", props: &ShadowAdapterProps{Type: "graphql", Config: bookingsMapping}, wantSet: true},
		{name: "zero props remove it", props: &ShadowAdapterProps{}, existing: true, wantSet: false},
		{name: "invalid is rejected", props: &ShadowAdapterProps{Type: "graphql", Config: "nope"}, existing: true, wantErr: traffictesting.ErrInvalidShadowAdapter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockGateRepo()
			if tt.existing {
				adapter, err := traffictesting.ParseShadowAdapter("graphql", bookingsMapping)
				require.NoError(t, err)
				repo.gate.ShadowAdapter = adapter
			}
			handler := NewUpdateGateHandler(repo, testGlobalRetention)

			gate, err := handler.Handle(context.Background(), UpdateGateCommand{
				ID:            repo.gate.ID.String(),
				ShadowAdapter: tt.props,
			})

			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, repo.updated)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantSet, gate.ShadowAdapter.IsSet())
		})
	}
}
