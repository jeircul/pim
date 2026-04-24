# Copilot instructions for pim

Terminal-based Azure PIM role activation. Bubble Tea v2 TUI + `--headless` for scripting.

## Stack

Go 1.26 · Bubble Tea v2 / Lip Gloss v2 / Bubbles v2 / Huh v2 · `azidentity` + raw ARM REST · TOML state in `~/.config/pim/`.

## Layout

- `internal/app/` — flag parsing, composition, ctx
- `internal/azure/` — PIM REST split across `client.go`, `roles.go`, `activation.go`, `discovery.go`; helpers in `errors.go`, `scopes.go`, `duration.go`
- `internal/state/` — mutex-guarded TOML Store
- `internal/headless/` — non-TUI command runner
- `internal/tui/` — screens (`activate/`, `dashboard/`, `status/`, `deactivate/`, `favorites/`), `components/`, `styles/`

## Workflow

`task fmt && task test && task build` before every commit. `task test` runs `-race`.

## Rules

- `internal/` only. No `pkg/`, Cobra/urfave, testify, logging libs.
- Early returns; guard clauses over nested `if`.
- Error wrap: `fmt.Errorf("noun phrase: %w", err)`. No "failed to".
- Use `errors.As(err, &apiErr)` against `*azure.APIError`. Never string-match on error text.
- `context.Context` is per-call, never stored on structs. TUI calls derive per-call timeouts from a parent ctx tied to SIGINT/SIGTERM.
- No inline comments inside function bodies; godoc on exported symbols only.
- Delete obsolete code in the same change.
- Prefer table-driven tests using stdlib `testing` only.

## Headless matching policy

`--role` and `--scope` resolve exact match first; fall back to substring; multiple substring matches with no exact returns an ambiguity error. ARM paths beat display names.

## Azure PIM API quirks

- `linkedRoleEligibilityScheduleId` is the full ARM path, not a bare GUID.
- Inherited MG-level eligibilities aren't visible when re-queried at child scopes.
- RG-scope activation returns 403; client retries at subscription scope automatically.
- `roleAssignmentSchedules` GET at RG scope returns 500 without read access. Treated as "not active".
- `GetEligibleRoles` and `GetActiveAssignments` paginate via `nextLink`.

Full reference: `.agents/skills/golang/references/azure-pim-api.md`.

## Bubble Tea v2 gotchas

- `View()` returns `tea.View`, not `string`.
- Key messages are `tea.KeyPressMsg`; the space key arrives as `"space"`, not `" "`.
- Adapt theme on `tea.BackgroundColorMsg` using `lipgloss.LightDark(isDark)`.

See `.agents/skills/golang/references/patterns.md` and `references/anti-patterns.md`.
