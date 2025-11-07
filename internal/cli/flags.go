package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jeircul/pim/pkg/azpim"
)

// CommandKind represents the top-level action the user requested.
type CommandKind int

const (
	CommandPrompt CommandKind = iota
	CommandHelp
	CommandActivate
	CommandStatus
	CommandDeactivate
	CommandVersion
)

// Command captures the parsed CLI intent.
type Command struct {
	Kind      CommandKind
	HelpTopic string
	Activate  ActivateConfig
}

// ActivateConfig holds activation-specific settings.
type ActivateConfig struct {
	Justification    string
	Hours            int
	ManagementGroups []string
	Subscriptions    []string
	ScopeContains    []string
	Roles            []string
	ResourceGroups   []string
	Auto             bool
	LegacyMode       bool
}

// ParseArgs parses os.Args[1:] style arguments into a Command.
func ParseArgs(args []string) (Command, error) {
	if len(args) == 0 {
		return Command{Kind: CommandPrompt}, nil
	}

	switch args[0] {
	case "activate", "a":
		return parseActivate(args[1:], false)
	case "status", "st":
		return Command{Kind: CommandStatus}, nil
	case "deactivate", "deact", "off":
		return Command{Kind: CommandDeactivate}, nil
	case "version", "v":
		return Command{Kind: CommandVersion}, nil
	case "help", "-h", "--help":
		return Command{Kind: CommandHelp}, nil
	default:
		if strings.HasPrefix(args[0], "-") {
			return parseActivate(args, true)
		}
		return Command{}, fmt.Errorf("unknown command %q", args[0])
	}
}

func parseActivate(args []string, legacy bool) (Command, error) {
	var cfg ActivateConfig
	var mgFilters, subFilters, scopeFilters, roleFilters, rgFilters stringSliceFlag

	fs := flag.NewFlagSet("activate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&cfg.Justification, "j", "", "")
	fs.StringVar(&cfg.Justification, "justification", "", "")
	fs.IntVar(&cfg.Hours, "t", 1, "")
	fs.IntVar(&cfg.Hours, "time", 1, "")
	fs.BoolVar(&cfg.Auto, "auto", false, "")
	fs.Var(&mgFilters, "management-group", "")
	fs.Var(&mgFilters, "mg", "")
	fs.Var(&subFilters, "subscription", "")
	fs.Var(&subFilters, "sub", "")
	fs.Var(&rgFilters, "resource-group", "")
	fs.Var(&rgFilters, "rg", "")
	fs.Var(&scopeFilters, "scope", "")
	fs.Var(&scopeFilters, "scope-contains", "")
	fs.Var(&roleFilters, "role", "")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Command{Kind: CommandHelp, HelpTopic: "activate"}, nil
		}
		return Command{}, err
	}

	cfg.ManagementGroups = mgFilters.Slice()
	cfg.Subscriptions = subFilters.Slice()
	cfg.ScopeContains = scopeFilters.Slice()
	cfg.Roles = roleFilters.Slice()
	cfg.ResourceGroups = rgFilters.Slice()
	cfg.LegacyMode = legacy

	if err := cfg.Validate(); err != nil {
		return Command{}, err
	}

	return Command{Kind: CommandActivate, Activate: cfg}, nil
}

// Validate ensures activation inputs are consistent.
func (c ActivateConfig) Validate() error {
	if c.Hours < azpim.MinHours || c.Hours > azpim.MaxHours {
		return fmt.Errorf("hours must be between %d and %d", azpim.MinHours, azpim.MaxHours)
	}
	return nil
}

// EnsureDefaults fills in sensible defaults when flags omit optional values.
func (c *ActivateConfig) EnsureDefaults() {
	if c.Hours == 0 {
		c.Hours = azpim.MinHours
	}
}

// NeedsJustification reports whether we still need to collect a justification from the user.
func (c ActivateConfig) NeedsJustification() bool {
	return strings.TrimSpace(c.Justification) == ""
}

// HasFilters reports whether any filtering hints were supplied.
func (c ActivateConfig) HasFilters() bool {
	return len(c.ManagementGroups) > 0 || len(c.Subscriptions) > 0 || len(c.ScopeContains) > 0 || len(c.Roles) > 0 || len(c.ResourceGroups) > 0
}

// HasTargetHints reports whether we have enough hints to narrow scope automatically.
func (c ActivateConfig) HasTargetHints() bool {
	return len(c.Subscriptions) > 0 || len(c.ScopeContains) > 0 || len(c.ResourceGroups) > 0
}

// AutoScopeEnabled signals whether hints should be applied without prompting.
func (c ActivateConfig) AutoScopeEnabled() bool {
	return c.Auto && c.HasTargetHints()
}

// ModeLabel returns a quick description of the activation mode.
func (c ActivateConfig) ModeLabel() string {
	if c.AutoScopeEnabled() {
		return "auto (apply hints without prompts)"
	}
	return "interactive (guided prompts)"
}

func PrintHelp(topic string) {
	switch topic {
	case "activate":
		printActivateHelp()
	default:
		printGlobalHelp()
	}
}

func printGlobalHelp() {
	fmt.Fprintf(os.Stderr, "Azure PIM helper\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "  pim                Start in guided mode\n")
	fmt.Fprintf(os.Stderr, "  pim activate       Activate roles via flags (or mix with prompts)\n")
	fmt.Fprintf(os.Stderr, "  pim status         View your current activations\n")
	fmt.Fprintf(os.Stderr, "  pim deactivate     Turn off an activation\n")
	fmt.Fprintf(os.Stderr, "  pim version        Show the CLI version\n")
	fmt.Fprintf(os.Stderr, "\nRun 'pim help activate' for flag-based activation options.\n")
}

func printActivateHelp() {
	fmt.Fprintf(os.Stderr, "Activate roles:\n")
	fmt.Fprintf(os.Stderr, "  pim activate -j \"Routine maintenance\" [flags]\n\n")
	fmt.Fprintf(os.Stderr, "Required:\n")
	fmt.Fprintf(os.Stderr, "  -j, --justification   Reason for the activation (prompted if omitted)\n\n")
	fmt.Fprintf(os.Stderr, "Optional:\n")
	fmt.Fprintf(os.Stderr, "  -t, --time            Hours (1-8, default 1)\n")
	fmt.Fprintf(os.Stderr, "      --mg              Filter roles by management group (repeatable)\n")
	fmt.Fprintf(os.Stderr, "      --sub             Filter roles by subscription (repeatable)\n")
	fmt.Fprintf(os.Stderr, "      --rg              Target resource group hints (repeatable)\n")
	fmt.Fprintf(os.Stderr, "      --role            Filter roles by name (repeatable)\n")
	fmt.Fprintf(os.Stderr, "      --scope           Advanced scope substring filter (repeatable)\n")
	fmt.Fprintf(os.Stderr, "      --auto            Apply hints automatically without extra prompts\n\n")
	fmt.Fprintf(os.Stderr, "Examples:\n")
	fmt.Fprintf(os.Stderr, "  pim activate -j \"Cleanup\" --mg Omnia-Temp-Dev\n")
	fmt.Fprintf(os.Stderr, "  pim activate -j \"Emergency fix\" --sub Q901-Platform-Dev --auto\n")
	fmt.Fprintf(os.Stderr, "\nTips:\n")
	fmt.Fprintf(os.Stderr, "  - Run 'pim' with no arguments for a guided menu\n")
	fmt.Fprintf(os.Stderr, "  - Combine flags with --auto to skip interactive scope prompts\n")
}

type stringSliceFlag []string

func (s *stringSliceFlag) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	if value == "" {
		return nil
	}
	*s = append(*s, value)
	return nil
}

func (s stringSliceFlag) Slice() []string {
	return append([]string(nil), s...)
}
