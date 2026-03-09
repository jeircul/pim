package app

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Command names.
const (
	CmdActivate   = "activate"
	CmdDeactivate = "deactivate"
	CmdStatus     = "status"
	CmdCompletion = "completion"
)

// OutputFormat controls headless output style.
type OutputFormat string

const (
	OutputTable OutputFormat = "table"
	OutputJSON  OutputFormat = "json"
)

// Config holds all parsed CLI configuration.
type Config struct {
	// Command is the subcommand (activate, deactivate, status, completion, or "" for TUI dashboard).
	Command string

	// TUI mode flags
	Headless bool
	Version  bool

	// Activation flags
	Roles         []string
	Scopes        []string
	TimeStr       string
	Justification string
	Yes           bool

	// Output
	Output OutputFormat

	// Config dir override (empty = default ~/.config/pim)
	ConfigDir string

	// CompletionShell is set when Command == CmdCompletion (bash | zsh | fish).
	CompletionShell string
}

// Parse parses os.Args[1:] into a Config.
func Parse(args []string) (Config, error) {
	cfg := Config{Output: OutputTable}

	if len(args) == 0 {
		return cfg, nil
	}

	// First arg may be a subcommand.
	switch strings.ToLower(args[0]) {
	case CmdActivate, "a":
		cfg.Command = CmdActivate
		args = args[1:]
	case CmdDeactivate, "deact", "off", "d":
		cfg.Command = CmdDeactivate
		args = args[1:]
	case CmdStatus, "st", "s":
		cfg.Command = CmdStatus
		args = args[1:]
	case CmdCompletion:
		cfg.Command = CmdCompletion
		if len(args) > 1 {
			cfg.CompletionShell = strings.ToLower(args[1])
		}
		return cfg, nil
	case "version", "v":
		cfg.Version = true
		return cfg, nil
	case "help", "-h", "--help":
		return cfg, flag.ErrHelp
	}

	fs := flag.NewFlagSet("pim", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var roles, scopes multiFlag
	fs.Var(&roles, "role", "role name filter (repeatable)")
	fs.Var(&scopes, "scope", "scope path (repeatable)")
	fs.StringVar(&cfg.TimeStr, "time", "", "activation duration (e.g. 1h, 30m, 1h30m)")
	fs.StringVar(&cfg.TimeStr, "t", "", "activation duration (shorthand)")
	fs.StringVar(&cfg.Justification, "justification", "", "justification text")
	fs.StringVar(&cfg.Justification, "j", "", "justification (shorthand)")
	fs.BoolVar(&cfg.Yes, "yes", false, "skip confirmation prompt")
	fs.BoolVar(&cfg.Yes, "y", false, "skip confirmation (shorthand)")
	fs.BoolVar(&cfg.Headless, "headless", false, "non-TUI mode; exit with code 0/1")

	var outStr string
	fs.StringVar(&outStr, "output", "table", "output format: table | json")
	fs.StringVar(&outStr, "o", "table", "output format (shorthand)")
	fs.StringVar(&cfg.ConfigDir, "config-dir", "", "override config directory")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}

	cfg.Roles = []string(roles)
	cfg.Scopes = []string(scopes)

	switch strings.ToLower(outStr) {
	case "json":
		cfg.Output = OutputJSON
	default:
		cfg.Output = OutputTable
	}

	return cfg, nil
}

// IsHeadless reports whether the run should skip the TUI entirely.
func (c Config) IsHeadless() bool {
	return c.Headless
}

// HasRoleFilter reports whether role filters were provided.
func (c Config) HasRoleFilter() bool { return len(c.Roles) > 0 }

// HasScopeFilter reports whether scope filters were provided.
func (c Config) HasScopeFilter() bool { return len(c.Scopes) > 0 }

// CanAutoAdvance reports whether enough flags are set to auto-advance the wizard.
func (c Config) CanAutoAdvance() bool {
	return c.HasRoleFilter() && c.HasScopeFilter() && c.TimeStr != "" && c.Justification != ""
}

// multiFlag is a flag.Value that accumulates repeated values.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// PrintHelp writes usage to stderr.
func PrintHelp() {
	fmt.Fprint(os.Stderr, `pim — Azure PIM role elevation manager

Usage:
  pim                          launch TUI dashboard
  pim activate [flags]         activate roles (TUI, flags pre-fill wizard)
  pim deactivate               deactivate roles (TUI)
  pim status                   view active/eligible roles (TUI)
  pim completion <bash|zsh|fish>  print shell completion script
  pim version                  print version

Activation flags:
  --role <name>         pre-filter by role name (repeatable)
  --scope <path>        target scope path (repeatable)
  --time, -t <dur>      duration: 1h, 30m, 1h30m, 1.5h
  --justification, -j   justification text
  --yes, -y             skip confirmation prompt
  --headless            non-TUI mode (for scripting)
  --output, -o          table | json (headless only)
`)
}
