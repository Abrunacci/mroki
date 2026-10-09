package commands

import (
	"context"
	"fmt"

	"github.com/pedrobarco/mroki/internal/domain/traffictesting"
)

// CreateGateCommand represents the intent to create a new gate
type CreateGateCommand struct {
	Name      string
	LiveURL   string
	ShadowURL string
	// ShadowAdapter is optional (nil means requests are compared as is).
	ShadowAdapter *ShadowAdapterProps
}

// ShadowAdapterProps holds a gate's shadow adapter: its type (e.g. "graphql")
// and configuration document (e.g. the YAML mapping).
type ShadowAdapterProps struct {
	Type   string
	Config string
}

// CreateGateHandler handles the CreateGate command
type CreateGateHandler struct {
	repo traffictesting.GateRepository
}

// NewCreateGateHandler creates a new CreateGateHandler
func NewCreateGateHandler(repo traffictesting.GateRepository) *CreateGateHandler {
	return &CreateGateHandler{repo: repo}
}

// Handle executes the CreateGate command
func (h *CreateGateHandler) Handle(ctx context.Context, cmd CreateGateCommand) (*traffictesting.Gate, error) {
	// Parse and validate name (domain validation)
	name, err := traffictesting.ParseGateName(cmd.Name)
	if err != nil {
		return nil, fmt.Errorf("invalid name: %w", err)
	}

	// Parse and validate URLs (domain validation)
	liveURL, err := traffictesting.ParseGateURL(cmd.LiveURL)
	if err != nil {
		return nil, fmt.Errorf("invalid live URL: %w", err)
	}

	shadowURL, err := traffictesting.ParseGateURL(cmd.ShadowURL)
	if err != nil {
		return nil, fmt.Errorf("invalid shadow URL: %w", err)
	}

	// Parse and validate the optional shadow adapter (domain validation)
	adapter := traffictesting.NoShadowAdapter()
	if cmd.ShadowAdapter != nil {
		adapter, err = traffictesting.ParseShadowAdapter(cmd.ShadowAdapter.Type, cmd.ShadowAdapter.Config)
		if err != nil {
			return nil, err
		}
	}

	// Create domain aggregate
	gate, err := traffictesting.NewGate(name, liveURL, shadowURL, traffictesting.WithGateShadowAdapter(adapter))
	if err != nil {
		return nil, fmt.Errorf("failed to create gate: %w", err)
	}

	// Persist (transaction boundary)
	if err := h.repo.Save(ctx, gate); err != nil {
		return nil, fmt.Errorf("failed to save gate: %w", err)
	}

	return gate, nil
}
