package app

import (
	"context"
	"fmt"
	"time"

	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/state"
)

// App wires together the azure client, state store, and execution mode.
type App struct {
	Client  *azure.Client
	Store   *state.Store
	Config  Config
	Version string
}

// New creates an App from the given config. Does not authenticate yet.
func New(cfg Config, version string) (*App, error) {
	store, err := state.New(cfg.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("open state store: %w", err)
	}
	return &App{Store: store, Config: cfg, Version: version}, nil
}

// Connect creates the Azure client and validates credentials.
func (a *App) Connect(ctx context.Context) error {
	client, err := azure.NewClient(ctx)
	if err != nil {
		return err
	}
	a.Client = client
	return nil
}

// DefaultContext returns a 2-minute background context.
func DefaultContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 2*time.Minute)
}
