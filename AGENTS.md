# pim — Azure PIM TUI Manager

Terminal-based Azure PIM (Privileged Identity Management) role activation.
Bubble Tea v2 TUI is the primary interface; `--headless` for CI/scripting.

## Stack

| | |
|---|---|
| Language | Go 1.26 |
| TUI | Bubble Tea v2, Lip Gloss v2, Bubbles v2, Huh v2 |
| Azure | `azidentity` + `azcore` + raw REST (no ARM SDK for PIM) |
| Persistence | TOML via BurntSushi/toml (`~/.config/pim/`) |
| Build | Task (`task build / test / fmt`), GoReleaser |

## Structure

```
internal/app/       — app core, CLI flag parsing, orchestration
internal/azure/     — Azure PIM REST client, auth, types, scope utilities
internal/tui/       — Bubble Tea screens and components
  activate/         — 4-step wizard: role select → scope tree → options → confirm
  dashboard/        — home screen, live timers, favorites shortcuts
  status/           — active + eligible roles
  deactivate/       — deactivation screen
  favorites/        — CRUD for saved role+scope+duration combos
  components/       — reusable header, statusbar, spinner, tree
internal/headless/  — non-TUI execution path for --headless
internal/state/     — TOML config + state (favorites, recent justifications)
```

## Workflow

```bash
task fmt && task test && task build   # always before commit
task install                          # install to $GOPATH/bin
```

## Rules

- `internal/` only — no `pkg/`, no Cobra/urfave, no testify, no logging libs
- Early returns and guard clauses over nested `if`
- Error wrapping: `fmt.Errorf("noun phrase: %w", err)` — no "failed to" prefix
- No inline comments; godoc on exported symbols only
- Delete obsolete code immediately — no opportunistic refactoring
- Parallel API calls where independent; lazy-load scope tree children

## Azure PIM API — critical domain knowledge

The Azure PIM REST API has undocumented behaviors and hard constraints that are
not obvious from the documentation. **Before modifying any code in `internal/azure/`**,
load the golang skill and read `references/azure-pim-api.md`. Key discoveries:

- `linkedRoleEligibilityScheduleId` must be the **full ARM resource path** — never a bare GUID
- Inherited MG-level eligibilities are invisible when re-querying at child scopes
- RG-scope activation returns HTTP 403 (chicken-and-egg: needs `resourceGroups/read` first);
  the client automatically falls back to subscription scope — see section 8 of the API reference
- `roleAssignmentSchedules` GET at RG scope returns HTTP 500 when lacking read access — this
  is expected and treated as "not active" in `isRoleActiveAt`

## Skills

The `.agents/skills/golang/` skill has the full Go conventions, Bubble Tea v2 patterns,
anti-patterns, and Azure PIM API reference for this project.

- **OpenCode**: load with `skill golang` or automatically on any Go/Azure task
- **VS Code Copilot**: consult `.agents/skills/golang/SKILL.md` and its `references/` files
- **Key references**:
  - `references/patterns.md` — Bubble Tea v2, Azure client, state/config patterns
  - `references/anti-patterns.md` — what not to do (with corrections)
  - `references/azure-pim-api.md` — all PIM endpoints, request bodies, scoped-down activation, RG fallback
