package cli

import (
	"fmt"

	"github.com/jeircul/pim/pkg/azpim"
)

type menuOption struct {
	Label string
	Kind  CommandKind
}

// PromptCommand displays the top-level menu for users who prefer a guided flow.
func PromptCommand() (Command, error) {
	options := []menuOption{
		{Label: "Activate eligible role(s)", Kind: CommandActivate},
		{Label: "View my active assignments", Kind: CommandStatus},
		{Label: "Deactivate an assignment", Kind: CommandDeactivate},
		{Label: "Show version", Kind: CommandVersion},
		{Label: "Help / flag reference", Kind: CommandHelp},
	}

	choice, err := PromptSelection(options,
		func(i int, opt menuOption) string {
			return fmt.Sprintf("  %2d) %s", i, opt.Label)
		},
		"Choose an action",
	)
	if err != nil {
		return Command{}, err
	}

	switch choice.Kind {
	case CommandActivate:
		cfg, err := promptActivateInteractively()
		if err != nil {
			return Command{}, err
		}
		return Command{Kind: CommandActivate, Activate: cfg}, nil
	case CommandHelp:
		return Command{Kind: CommandHelp}, nil
	default:
		return Command{Kind: choice.Kind}, nil
	}
}

func promptActivateInteractively() (ActivateConfig, error) {
	fmt.Println("\n--- Activate eligible role(s) ---")
	fmt.Println("You can always press 'q' to cancel any prompt.")

	justification, err := PromptJustification("")
	if err != nil {
		return ActivateConfig{}, err
	}

	hours, err := PromptHours(azpim.MinHours)
	if err != nil {
		return ActivateConfig{}, err
	}

	cfg := ActivateConfig{Justification: justification, Hours: hours}

	addFilters, err := PromptYesNo("Add filters (management group, subscription, etc.)?", false)
	if err != nil {
		return ActivateConfig{}, err
	}

	if addFilters {
		if cfg.ManagementGroups, err = PromptCSV("Management group filter(s)", nil); err != nil {
			return ActivateConfig{}, err
		}
		if cfg.Subscriptions, err = PromptCSV("Subscription filter(s)", nil); err != nil {
			return ActivateConfig{}, err
		}
		if cfg.ResourceGroups, err = PromptCSV("Resource group hint(s)", nil); err != nil {
			return ActivateConfig{}, err
		}
		if cfg.Roles, err = PromptCSV("Role name filter(s)", nil); err != nil {
			return ActivateConfig{}, err
		}
		if cfg.ScopeContains, err = PromptCSV("Scope contains filter(s)", nil); err != nil {
			return ActivateConfig{}, err
		}
		if cfg.HasTargetHints() {
			applyAuto, autoErr := PromptYesNo("Automatically apply these hints without extra prompts?", false)
			if autoErr != nil {
				return ActivateConfig{}, autoErr
			}
			cfg.Auto = applyAuto
		}
	}

	fmt.Println("\nTip: next time you can run 'pim activate --help' to see the equivalent flags.")
	return cfg, nil
}
