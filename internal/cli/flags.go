package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jeircul/pim/pkg/azpim"
)

// Config holds command-line configuration
type Config struct {
	Justification    string
	Hours            int
	Deactivate       bool
	Status           bool
	ShowVersion      bool
	ManagementGroups []string
	Subscriptions    []string
	ScopeContains    []string
	Roles            []string
	ResourceGroups   []string
}

// ParseFlags parses command-line flags and returns configuration
func ParseFlags() (Config, bool, error) {
	cfg := Config{}
	var showHelp bool
	var mgFilters, subFilters, scopeFilters, roleFilters, rgFilters stringSliceFlag

	// Define flags
	flag.StringVar(&cfg.Justification, "j", "", "")
	flag.StringVar(&cfg.Justification, "justification", "", "")

	flag.IntVar(&cfg.Hours, "t", 1, "")
	flag.IntVar(&cfg.Hours, "time", 1, "")

	flag.BoolVar(&cfg.Deactivate, "d", false, "")
	flag.BoolVar(&cfg.Deactivate, "deactivate", false, "")

	flag.BoolVar(&cfg.Status, "s", false, "")
	flag.BoolVar(&cfg.Status, "status", false, "")

	flag.BoolVar(&cfg.ShowVersion, "version", false, "")
	flag.BoolVar(&cfg.ShowVersion, "V", false, "")

	flag.BoolVar(&showHelp, "h", false, "")
	flag.BoolVar(&showHelp, "help", false, "")

	flag.Var(&mgFilters, "management-group", "")
	flag.Var(&mgFilters, "mg", "")
	flag.Var(&subFilters, "subscription", "")
	flag.Var(&subFilters, "sub", "")
	flag.Var(&scopeFilters, "scope-contains", "")
	flag.Var(&roleFilters, "role", "")
	flag.Var(&rgFilters, "resource-group", "")
	flag.Var(&rgFilters, "rg", "")

	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Azure PIM Role Activation Tool\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		fmt.Fprintf(os.Stderr, "  -j, --justification string\n")
		fmt.Fprintf(os.Stderr, "        Justification text for activation (required for activation)\n")
		fmt.Fprintf(os.Stderr, "  -t, --time int\n")
		fmt.Fprintf(os.Stderr, "        Duration in hours, 1-8 (default: 1)\n")
		fmt.Fprintf(os.Stderr, "  -d, --deactivate\n")
		fmt.Fprintf(os.Stderr, "        Deactivate an active role assignment\n")
		fmt.Fprintf(os.Stderr, "  -s, --status\n")
		fmt.Fprintf(os.Stderr, "        Show active role assignments\n")
		fmt.Fprintf(os.Stderr, "      --version, -V\n")
		fmt.Fprintf(os.Stderr, "        Show build version\n")
		fmt.Fprintf(os.Stderr, "      --management-group, --mg value\n")
		fmt.Fprintf(os.Stderr, "        Filter eligible roles by management group display name or ID (repeatable)\n")
		fmt.Fprintf(os.Stderr, "      --subscription, --sub value\n")
		fmt.Fprintf(os.Stderr, "        Filter eligible roles by subscription display name or ID (repeatable)\n")
		fmt.Fprintf(os.Stderr, "      --scope-contains value\n")
		fmt.Fprintf(os.Stderr, "        Filter eligible roles by substring match on scope path or display (repeatable)\n")
		fmt.Fprintf(os.Stderr, "      --role value\n")
		fmt.Fprintf(os.Stderr, "        Filter eligible roles by role name (repeatable)\n")
		fmt.Fprintf(os.Stderr, "      --resource-group, --rg value\n")
		fmt.Fprintf(os.Stderr, "        Filter or target resource groups by name (repeatable)\n")
		fmt.Fprintf(os.Stderr, "  -h, --help\n")
		fmt.Fprintf(os.Stderr, "        Show this help message\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s -j \"Deploy infrastructure\" -t 4\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --justification \"Emergency fix\" --time 2\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s -d\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\n")
	}

	flag.Parse()

	// Show help if requested or no arguments provided
	if showHelp || len(os.Args) == 1 {
		flag.Usage()
		return cfg, true, nil
	}

	// Validate configuration
	if err := ValidateConfig(cfg); err != nil {
		return cfg, false, err
	}

	cfg.ManagementGroups = mgFilters.Slice()
	cfg.Subscriptions = subFilters.Slice()
	cfg.ScopeContains = scopeFilters.Slice()
	cfg.Roles = roleFilters.Slice()
	cfg.ResourceGroups = rgFilters.Slice()

	return cfg, false, nil
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

// HasFilters reports whether any narrowing flags were provided
func (c Config) HasFilters() bool {
	return len(c.ManagementGroups) > 0 || len(c.Subscriptions) > 0 || len(c.ScopeContains) > 0 || len(c.Roles) > 0 || len(c.ResourceGroups) > 0
}

// HasTargetHints reports whether the user supplied filters that can guide scope narrowing
func (c Config) HasTargetHints() bool {
	return len(c.Subscriptions) > 0 || len(c.ScopeContains) > 0 || len(c.ResourceGroups) > 0
}

// ValidateConfig validates the configuration
func ValidateConfig(cfg Config) error {
	if cfg.ShowVersion || cfg.Status || cfg.Deactivate {
		return nil
	}
	if cfg.Justification == "" {
		return fmt.Errorf("-j/--justification required for activation")
	}

	if cfg.Hours < azpim.MinHours || cfg.Hours > azpim.MaxHours {
		return fmt.Errorf("hours must be between %d and %d", azpim.MinHours, azpim.MaxHours)
	}

	return nil
}
