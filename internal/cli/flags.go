package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/jeircul/pim/pkg/azpim"
)

// Config holds command-line configuration
type Config struct {
	Justification string
	Hours         int
	Deactivate    bool
	Status        bool
}

// ParseFlags parses command-line flags and returns configuration
func ParseFlags() (Config, bool, error) {
	cfg := Config{}
	var showHelp bool

	// Define flags
	flag.StringVar(&cfg.Justification, "j", "", "")
	flag.StringVar(&cfg.Justification, "justification", "", "")

	flag.IntVar(&cfg.Hours, "t", 1, "")
	flag.IntVar(&cfg.Hours, "time", 1, "")

	flag.BoolVar(&cfg.Deactivate, "d", false, "")
	flag.BoolVar(&cfg.Deactivate, "deactivate", false, "")

	flag.BoolVar(&cfg.Status, "s", false, "")
	flag.BoolVar(&cfg.Status, "status", false, "")

	flag.BoolVar(&showHelp, "h", false, "")
	flag.BoolVar(&showHelp, "help", false, "")

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

	return cfg, false, nil
}

// ValidateConfig validates the configuration
func ValidateConfig(cfg Config) error {
	if cfg.Status || cfg.Deactivate {
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
