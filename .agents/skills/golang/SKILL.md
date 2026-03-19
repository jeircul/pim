---
name: golang
description: >
  Go coding conventions for the pim project — Azure PIM TUI CLI.
  Use when writing, reviewing, or fixing Go code, or writing tests.
tags: [go, bubbletea, azure, tui, pim]
version: "1.0"
---

# Go Conventions — pim

## Project snapshot

- **Module**: `github.com/jeircul/pim` | **Go**: 1.26
- **TUI**: `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2`, `charm.land/huh/v2`
- **Azure**: `azidentity` + `azcore` + raw REST (no ARM SDK for PIM)
- **Persistence**: TOML via `github.com/BurntSushi/toml`
- **Structure**: `internal/` only — no `pkg/`; CLI flags hand-rolled, no Cobra/urfave

## Code style

- Early returns; guard clauses over nested `if`
- No globals — inject deps via params or struct fields
- No inline comments in function bodies; godoc on all exported symbols
- Imports: 3 groups — stdlib / external / `github.com/jeircul/pim/internal/...`
- Single-letter receivers matching type: `c *Client`, `m AppModel`, `s *Store`
- Initialisms stay uppercase: `URL`, `ID`, `PIM`, `API`
- Anonymous funcs: max 5 lines; extract if longer

## Error handling

```go
// Wrap with context noun (no "failed to" prefix):
return fmt.Errorf("decode user: %w", err)

// Handle once — return OR log, never both.
// Lowercase, no trailing period.
```

## Go 1.26 patterns to prefer

- `iter.Seq` / `iter.Seq2` for iterators; range-over-func
- `omitzero` struct tag for JSON/TOML zero-value omission
- `strings.Lines` for line iteration
- `testing.T.Context()` in tests

## Interfaces

```go
// Define in the consumer package, not the implementer.
// Compile-time check:
var _ MyInterface = (*MyType)(nil)
// Never pointer to interface: use T, not *T for interface vars.
```

## Concurrency

- Document goroutine lifetimes; always provide a stop mechanism
- Channel size: 0 (synchronous) or 1 (single-item buffer) only
- Copy slices/maps at API boundaries — do not retain caller's slice

## Bubble Tea v2

> Full patterns in `references/patterns.md`

```go
// View() returns tea.View, not string:
func (m Model) View() tea.View {
    v := tea.NewView(content)
    v.AltScreen = true
    v.WindowTitle = "pim"
    return v
}

// Key messages use KeyPressMsg (v2):
case tea.KeyPressMsg:
    if msg.String() == "q" { ... }

// Adaptive theme from background detection:
case tea.BackgroundColorMsg:
    m.theme = styles.NewTheme(msg.IsDark())

// Lip Gloss v2 — LightDark helper:
ld := lipgloss.LightDark(isDark)
accent := ld(lipgloss.Color("#0066cc"), lipgloss.Color("#00ccff"))
// accent is color.Color — not lipgloss.Style; use as .Foreground(accent)
```

## Azure client

> Full patterns in `references/patterns.md`

```go
// Key signatures — memorise these:
client.GetCurrentUser() (*User, error)
client.GetEligibleRoles() ([]Role, error)
client.GetActiveAssignments(principalID string) ([]ActiveAssignment, error)
client.ActivateRole(role Role, principalID, justification string, minutes int, targetScope string) (*ScheduleResponse, error)
client.DeactivateRole(assignment ActiveAssignment, principalID string) (*ScheduleResponse, error)

// Client has NO User field — call GetCurrentUser() once at startup.
// Always call ensureTokens() (internal) before HTTP; tokens are lazy-fetched.
```

## Testing

```go
// Table-driven; stdlib only (no testify):
tests := []struct {
    name string
    input string
    want string
}{
    {"empty", "", ""},
}
for _, tc := range tests {
    t.Run(tc.name, func(t *testing.T) {
        got := fn(tc.input)
        if got != tc.want {
            t.Errorf("fn(%q) = %q, want %q", tc.input, got, tc.want)
        }
    })
}
```

## Workflow

```
task fmt    # gofmt + goimports
task test   # go test ./...
task build  # go build
```

- Run `task fmt && task test` before every commit
- Diffs must be focused — never touch files unrelated to the task
- Delete obsolete code immediately after replacing it
- No new markdown docs unless explicitly requested

## What NOT to do

See `references/anti-patterns.md` for a full list with corrections.

**TL;DR**: no Cobra/urfave, no `pkg/`, no testify, no logging libs, no `View()` returning string,
no pointer-to-interface, no globals, no silenced errors, no inline comments.
