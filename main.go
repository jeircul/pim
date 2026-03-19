package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/jeircul/pim/internal/app"
	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/completion"
	"github.com/jeircul/pim/internal/headless"
	"github.com/jeircul/pim/internal/tui"
)

// Version is injected at build time via -ldflags.
var Version = "dev"

func main() {
	if err := run(); err != nil {
		if errors.Is(err, azure.ErrUserCancelled) {
			fmt.Fprintln(os.Stderr, "cancelled")
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := app.Parse(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			app.PrintHelp()
			return nil
		}
		app.PrintHelp()
		return err
	}

	if cfg.Version {
		fmt.Printf("pim %s\n", Version)
		return nil
	}

	if cfg.Command == app.CmdCompletion {
		switch cfg.CompletionShell {
		case "bash":
			completion.Bash(os.Stdout)
		case "zsh":
			completion.Zsh(os.Stdout)
		case "fish":
			completion.Fish(os.Stdout)
		default:
			fmt.Fprintf(os.Stderr, "usage: pim completion <bash|zsh|fish>\n")
			return fmt.Errorf("unknown shell: %q", cfg.CompletionShell)
		}
		return nil
	}

	a, err := app.New(cfg, Version)
	if err != nil {
		return err
	}

	ctx, cancel := app.DefaultContext()
	defer cancel()

	if cfg.IsHeadless() {
		if err := a.Connect(ctx); err != nil {
			return err
		}
		return headless.Run(ctx, a)
	}

	if err := a.Connect(context.Background()); err != nil {
		return err
	}
	return tui.Run(a)
}
