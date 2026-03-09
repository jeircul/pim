pkg/
tests/
# GitHub Copilot Instructions — PIM v2

Guidelines for the PIM v2 Azure PIM TUI manager.

## Context

- **Project**: Azure PIM elevation TUI (`pim`)
- **Language**: Go 1.26
- **TUI**: Bubble Tea v2 (`charm.land/bubbletea/v2`), Bubbles v2, Lip Gloss v2, Huh v2
- **Azure**: azidentity, azcore, raw REST for PIM API
- **Config**: TOML (`github.com/BurntSushi/toml`)
- **Structure**:
  - `main.go`: entrypoint, flag parsing, mode detection
  - `internal/app`: application core, config
  - `internal/azure`: PIM client, auth, types
  - `internal/tui`: Bubble Tea screens and components
  - `internal/tui/dashboard`: home screen — active roles + favorites (1-9 shortcuts)
  - `internal/tui/activate`: 4-step wizard (roles → scopes → options → confirm)
  - `internal/tui/status`: role status view
  - `internal/tui/deactivate`: deactivation screen
  - `internal/tui/favorites`: favorites management
  - `internal/tui/components`: reusable header, statusbar, spinner, tree
  - `internal/state`: TOML-based persistence (`~/.config/pim/`)
  - `internal/headless`: non-TUI execution for `--headless`

## Go Patterns

- Use Go 1.26 features: iterators (`iter.Seq`/`iter.Seq2`), range-over-func, `omitzero`, `strings.Lines`
- Accept interfaces, return structs
- Wrap errors: `fmt.Errorf("action: %w", err)` — no "failed to" prefix
- Handle errors once: return OR log, never both
- Early returns, guard clauses over nesting
- No globals; inject dependencies via params/structs
- Verify interfaces at compile time: `var _ Interface = (*Type)(nil)`
- Copy slices/maps at API boundaries
- Channel size: 0 or 1

## Bubble Tea v2 Patterns

- Each screen is a `tea.Model` in its own sub-package
- Root model in `internal/tui/app.go` routes between screens via messages
- Use `tea.Cmd` for async operations (API calls, timers)
- `View()` returns `tea.View` (v2 declarative API) — not a string
- Key handling: `tea.KeyPressMsg` (v2) — not `tea.KeyMsg`
- Mouse: `tea.MouseClickMsg`, `tea.MouseReleaseMsg` etc (v2 interface-based)
- `Style` is a value type in Lip Gloss v2 — copy by assignment
- Embed `huh.Form` in tea.Model for wizard form steps

## Testing

- Table-driven tests with Go's `testing` package
- Use `testing.T.Context()` for context-scoped test code
- Azure client tests in `internal/azure/`
- TUI logic: test `Update()` with synthetic messages
- State store tests in `internal/state/`
- Integration tests via `testscript`

## Avoid

- Cobra, urfave/cli, or any CLI framework
- Logging libraries
- HTTP servers
- Global mutable state
- Silently swallowing errors
- New markdown docs unless explicitly requested
- Anonymous functions longer than 5 lines
- `pkg/` directory (everything is `internal/`)

## Workflow

- Run `task fmt` and `task test` before committing
- Keep diffs focused; avoid touching unrelated files
- Delete obsolete code immediately after replacing it
