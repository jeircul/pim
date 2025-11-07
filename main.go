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

// Version is set at build time.
var Version = "dev"

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
	cmd, err := cli.ParseArgs(os.Args[1:])
	if err != nil {
		return err
	}

	if cmd.Kind == cli.CommandPrompt {
		cmd, err = cli.PromptCommand()
		if err != nil {
			return err
		}
	}

	switch cmd.Kind {
	case cli.CommandHelp:
		cli.PrintHelp(cmd.HelpTopic)
		return nil
	case cli.CommandVersion:
		fmt.Printf("pim %s\n", Version)
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
	switch cmd.Kind {
	case cli.CommandStatus:
		return cli.HandleStatus(ctx, client, user.ID)
	case cli.CommandDeactivate:
		return cli.HandleDeactivation(ctx, client, user.ID)
	case cli.CommandActivate:
		return cli.HandleActivation(ctx, client, user.ID, cmd.Activate)
	case cli.CommandPrompt:
		return fmt.Errorf("no command selected")
	default:
		return fmt.Errorf("unsupported command")
	}
}
