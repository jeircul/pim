# pim — Azure PIM TUI Manager

Terminal-based Azure Privileged Identity Management role activation. Bubble Tea v2 TUI is the primary interface; `--headless` for CI/scripting.

## Stack

- Go 1.26
- TUI: Bubble Tea v2, Lip Gloss v2, Bubbles v2, Huh v2
- Azure: `azidentity` + `azcore` + raw REST (no ARM SDK for PIM)
- Persistence: TOML via BurntSushi/toml (`~/.config/pim/`)
- Build: Task (`task fmt / test / build / install`), GoReleaser

## Layout

```
internal/app/        CLI flag parsing, app composition, ctx setup
internal/azure/      PIM REST client (client/roles/activation/discovery), errors, scopes, duration
internal/state/      TOML config + state with mutex-guarded Store
internal/headless/   non-TUI execution path for --headless
internal/completion/ shell completion script generators (bash/zsh/fish)
internal/tui/        Bubble Tea screens
  activate/          4-step wizard
  dashboard/         home screen + favorites
  status/            active + eligible roles
  deactivate/        deactivation screen
  favorites/         favorites CRUD
  components/        header, statusbar, spinner, tree, help
  styles/            theme + KeyMap
```

## Workflow

```bash
task fmt && task test && task build   # before every commit (test runs -race)
task install                          # install to ~/.local/bin
```

If `go mod tidy` fails with `x509: certificate signed by unknown authority` (corporate TLS proxy), extract the intercepting CA and set `SSL_CERT_FILE`:

```bash
echo | openssl s_client -connect proxy.golang.org:443 -showcerts 2>/dev/null \
  | awk '/-----BEGIN CERTIFICATE-----/{c++} c==2,/-----END CERTIFICATE-----/' > /tmp/corp-ca.pem
cat /etc/ssl/certs/ca-certificates.crt /tmp/corp-ca.pem > /tmp/ca-bundle.crt
GOTOOLCHAIN=local SSL_CERT_FILE=/tmp/ca-bundle.crt go mod tidy
```

## Rules

- `internal/` only. No `pkg/`, no Cobra/urfave, no testify, no logging libs.
- Early returns; guard clauses over nested `if`.
- Errors: `fmt.Errorf("noun phrase: %w", err)`. No "failed to" prefix.
- Typed errors over string matching: use `errors.As(err, &apiErr)` against `*azure.APIError`.
- `context.Context` is per-call, never stored on structs.
- No inline comments; godoc on exported symbols only.
- Delete obsolete code in the same change. No opportunistic refactoring.

## Azure PIM API quirks

- `linkedRoleEligibilityScheduleId` must be the full ARM resource path, not a bare GUID.
- Inherited MG-level eligibilities are invisible when re-queried at child scopes.
- RG-scope activation returns 403 (chicken-and-egg). The client falls back to subscription scope automatically.
- `roleAssignmentSchedules` GET at RG scope returns 500 when the caller lacks read access. Treated as "not active".
- `GetEligibleRoles` and `GetActiveAssignments` paginate via `nextLink`.

Full API reference: `.agents/skills/golang/references/azure-pim-api.md`.

## Scope and role matching (headless)

- `--role` and `--scope` use exact-first, substring-fallback matching.
- Multiple substring matches with no exact match returns an ambiguity error.
- ARM scope paths take precedence over display-name matching.

## Favorites behaviour

- `Favorite.Complete()` returns true when `role`, `scope`, `duration`, and `justification` are all non-empty.
- Dashboard 1–9 shortcut: if `Complete()` → `startWizard` with `AutoSubmit=true` and `favoritePending=true`; activation result shown as dashboard notice, TUI stays open. If not `Complete()` → error notice, no activation.
- `favoritePending` on `AppModel` distinguishes favorite-triggered activations (return to dashboard) from manual wizard activations (quit with summary).
- Favorites screen `ActivateMsg` always opens the wizard (incomplete favorites stop at the missing step).
- `autoAdvance` in `rolelist.go` uses `scopeFilter` as a tiebreaker when multiple roles share the same name — emits only when exactly one scope match survives.

## Skills

`.agents/skills/golang/` has full Go conventions, Bubble Tea v2 patterns, and the Azure PIM API reference.

- OpenCode: `skill golang` (auto-loads on Go/Azure tasks).
- Other agents: read `.agents/skills/golang/SKILL.md` and `references/`.
