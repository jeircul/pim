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
	Minutes          int
	ManagementGroups []string
	Subscriptions    []string
	ScopeContains    []string
	Roles            []string
	ResourceGroups   []string
	Yes              bool
}

// ParseArgs parses os.Args[1:] style arguments into a Command.
func ParseArgs(args []string) (Command, error) {
	if len(args) == 0 {
		return Command{Kind: CommandPrompt}, nil
	}

	switch args[0] {
	case "activate", "a":
		return parseActivate(args[1:])
	case "status", "st":
		return Command{Kind: CommandStatus}, nil
	case "deactivate", "deact", "off":
		return Command{Kind: CommandDeactivate}, nil
	case "version", "v":
		return Command{Kind: CommandVersion}, nil
	case "help", "-h", "--help":
		return Command{Kind: CommandHelp}, nil
	default:
		return Command{}, fmt.Errorf("unknown command %q", args[0])
	}
}

func parseActivate(args []string) (Command, error) {
	var cfg ActivateConfig
	var mgFilters, subFilters, scopeFilters, roleFilters, rgFilters stringSliceFlag
	var durationStr string

	fs := flag.NewFlagSet("activate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	fs.StringVar(&cfg.Justification, "j", "", "")
	fs.StringVar(&cfg.Justification, "justification", "", "")
	fs.StringVar(&durationStr, "t", "1h", "")
	fs.StringVar(&durationStr, "time", "1h", "")
	fs.BoolVar(&cfg.Yes, "yes", false, "")
	fs.BoolVar(&cfg.Yes, "y", false, "")
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

	// Parse duration string
	minutes, err := parseDuration(durationStr)
	if err != nil {
		return Command{}, fmt.Errorf("invalid duration: %w", err)
	}
	cfg.Minutes = minutes

	cfg.ManagementGroups = mgFilters.Slice()
	cfg.Subscriptions = subFilters.Slice()
	cfg.ScopeContains = scopeFilters.Slice()
	cfg.Roles = roleFilters.Slice()
	cfg.ResourceGroups = rgFilters.Slice()

	if err := cfg.Validate(); err != nil {
		return Command{}, err
	}

	return Command{Kind: CommandActivate, Activate: cfg}, nil
}

// Validate ensures activation inputs are consistent.
func (c ActivateConfig) Validate() error {
	if c.Minutes < azpim.MinMinutes || c.Minutes > azpim.MaxMinutes {
		return fmt.Errorf("duration must be between %d and %d minutes", azpim.MinMinutes, azpim.MaxMinutes)
	}
	if c.Minutes%30 != 0 {
		return fmt.Errorf("duration must be in 30-minute increments")
	}
	return nil
}

// EnsureDefaults fills in sensible defaults when flags omit optional values.
func (c *ActivateConfig) EnsureDefaults() {
	if c.Minutes == 0 {
		c.Minutes = azpim.MinMinutes
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

// ModeLabel returns a quick description of the activation mode.
func (c ActivateConfig) ModeLabel() string {
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
	fmt.Fprintf(os.Stderr, "  -t, --time            Duration (default '1h')\n")
	fmt.Fprintf(os.Stderr, "                        Formats: '1h', '90m', '1.5h', '1h30m', '3' (hours)\n")
	fmt.Fprintf(os.Stderr, "                        Range: 30m to 8h in 30-minute increments\n")
	fmt.Fprintf(os.Stderr, "  -y, --yes             Skip confirmation prompt (for automation)\n")
	fmt.Fprintf(os.Stderr, "      --mg              Filter roles by management group (repeatable)\n")
	fmt.Fprintf(os.Stderr, "      --sub             Filter roles by subscription (repeatable)\n")
	fmt.Fprintf(os.Stderr, "      --rg              Target resource group hints (repeatable)\n")
	fmt.Fprintf(os.Stderr, "      --role            Filter roles by name (repeatable)\n")
	fmt.Fprintf(os.Stderr, "      --scope           Advanced scope substring filter (repeatable)\n\n")
	fmt.Fprintf(os.Stderr, "Examples:\n")
	fmt.Fprintf(os.Stderr, "  pim activate -j \"Cleanup\" --mg Omnia-Temp-Dev\n")
	fmt.Fprintf(os.Stderr, "  pim activate -j \"Emergency fix\" --sub Q901-Platform-Dev\n")
	fmt.Fprintf(os.Stderr, "  pim activate -j \"Quick task\" -t 30m --yes\n")
	fmt.Fprintf(os.Stderr, "  pim activate -j \"Extended work\" -t 2h30m --role Owner\n")
	fmt.Fprintf(os.Stderr, "\nTips:\n")
	fmt.Fprintf(os.Stderr, "  - Run 'pim' with no arguments for a guided menu\n")
	fmt.Fprintf(os.Stderr, "  - Scope hints (--sub, --rg) auto-drill when specific enough\n")
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

// parseDuration parses duration strings like "1h", "90m", "1.5h", "1h30m"
func parseDuration(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 60, nil // default 1 hour
	}

	// Check if it contains 'h' or 'm' suffix
	hasUnit := strings.ContainsAny(strings.ToLower(s), "hm")

	// If no unit and it's a plain number, interpret as hours for backward compatibility
	if !hasUnit {
		if val := parseAsNumber(s); val > 0 {
			return val * 60, nil
		}
		return 0, fmt.Errorf("invalid duration format")
	}

	// Parse compound duration like "1h30m" or "2.5h" or "90m"
	totalMinutes := 0
	current := ""

	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' || ch == '.' {
			current += string(ch)
		} else if ch == 'h' || ch == 'H' {
			if current == "" {
				return 0, fmt.Errorf("missing number before 'h'")
			}
			val, err := parseFloat(current)
			if err != nil {
				return 0, fmt.Errorf("invalid hours value: %w", err)
			}
			totalMinutes += int(val * 60)
			current = ""
		} else if ch == 'm' || ch == 'M' {
			if current == "" {
				return 0, fmt.Errorf("missing number before 'm'")
			}
			val, err := parseFloat(current)
			if err != nil {
				return 0, fmt.Errorf("invalid minutes value: %w", err)
			}
			totalMinutes += int(val)
			current = ""
		} else if ch == ' ' {
			continue
		} else {
			return 0, fmt.Errorf("invalid character '%c' in duration", ch)
		}
	}

	if current != "" {
		return 0, fmt.Errorf("duration must end with 'h' or 'm'")
	}

	if totalMinutes <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}

	return totalMinutes, nil
}

func parseAsNumber(s string) int {
	var val int
	if _, err := fmt.Sscanf(s, "%d", &val); err == nil {
		return val
	}
	return 0
}

func parseFloat(s string) (float64, error) {
	var val float64
	if _, err := fmt.Sscanf(s, "%f", &val); err != nil {
		return 0, err
	}
	return val, nil
}
