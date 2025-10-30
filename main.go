package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/jeircul/pim/internal/cli"
	"github.com/jeircul/pim/pkg/azpim"
)

func main() {
	if err := run(); err != nil {
		if errors.Is(err, azpim.ErrUserCancelled) {
			fmt.Println("\n⚠️  Operation cancelled by user")
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Parse command-line flags
	cfg, showHelp, err := cli.ParseFlags()
	if err != nil {
		return err
	}

	if showHelp {
		return nil
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Initialize PIM client
	client, err := azpim.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("initialize client: %w", err)
	}

	// Get current user
	user, err := client.GetCurrentUser()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}
	fmt.Printf("Authenticated as: %s (%s)\n", user.DisplayName, user.UserPrincipalName)

	// Handle status, deactivation, or activation flow
	if cfg.Status {
		return cli.HandleStatus(ctx, client, user.ID)
	}

	if cfg.Deactivate {
		return cli.HandleDeactivation(ctx, client, user.ID)
	}

	return cli.HandleActivation(ctx, client, user.ID, cfg.Justification, cfg.Hours)
}
