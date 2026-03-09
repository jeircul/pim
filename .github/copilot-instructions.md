---
description: PIM v2 — Azure PIM TUI manager
applyTo: '**'
---
Tool: Azure PIM role elevation manager with Bubble Tea v2 TUI
Tech: Go 1.26, Bubble Tea v2, Lip Gloss v2, Bubbles v2, Huh v2, Azure SDK
Style: gofmt defaults (tabs), minimal code, no inline comments
Prefer: Idiomatic Go, error wrapping, immutability, iterators, early returns
Never: Verbose logging, new markdown docs, mutex unless required, cobra/cli frameworks

Structure:
- `main.go`: entry, flag parsing, mode detection (TUI vs headless)
- `internal/app/`: application core, config, orchestration
- `internal/azure/`: Azure PIM REST client, auth, types
- `internal/tui/`: Bubble Tea screens and components
- `internal/state/`: TOML-based persistent preferences and state
- `internal/headless/`: non-TUI execution path for --headless

TUI Architecture:
- Elm architecture (Model → Update → View) via Bubble Tea v2
- Root model in internal/tui/app.go routes between screens via messages
- Each screen is its own tea.Model in a sub-package under internal/tui/
- Activation is a 4-step wizard: role select → scope tree → options → confirm
- View() returns tea.View (v2 declarative API), not string
- Styling via Lip Gloss v2: mono + accent theme, adaptive ANSI colors
- Reusable components in internal/tui/components/

Patterns:
- Accept interfaces, return structs
- Error wrapping: `fmt.Errorf("context: %w", err)` — no "failed to" prefix
- Early returns, guard clauses over nesting
- Immutable config structs with validation methods
- Context propagation (2min timeout default)
- Iterators (iter.Seq/iter.Seq2) for collection operations
- Parallel API calls where independent
- Lazy loading for scope tree children
- Session-level caching for eligible roles and active assignments

UX:
- TUI is the app. Always launches unless --headless.
- Flags pre-fill and auto-advance wizard steps. --yes auto-submits.
- Favorites on dashboard with number-key shortcuts (1-9).
- Vim-style navigation in scope tree (h/j/k/l).
- State persisted in ~/.config/pim/ (TOML).

Discipline:
- Delete obsolete code immediately after changes
- Code should be self-documenting; do not add commentary while developing
- Run `task fmt` and `task test` before committing

Commands: activate, status, deactivate (+ TUI dashboard as default)
Tasks: `task build`, `task test`, `task install`
