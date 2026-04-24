package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
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
func (a *App) Connect(_ context.Context) error {
	client, err := azure.NewClient()
	if err != nil {
		return err
	}
	a.Client = client
	return nil
}

// DefaultContext returns a context that cancels on SIGINT/SIGTERM or after 2 minutes.
func DefaultContext() (context.Context, context.CancelFunc) {
	sigCtx, sigCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	timeCtx, timeCancel := context.WithTimeout(sigCtx, 2*time.Minute)
	return timeCtx, func() {
		timeCancel()
		sigCancel()
	}
}
