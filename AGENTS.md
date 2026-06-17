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

## Releases

Two release tiers, both require a clean `task fmt && task test` pass.

| Task | Branch | Result |
|------|--------|--------|
| `task release:dev` | any non-main, non-renovate | GitHub pre-release with auto-computed `vX.Y.Z-dev.N-gSHA` tag |
| `task release:stable` | `main` only, up to date with `origin/main` | Stable GitHub release; prompts for `vX.Y.Z` tag |

### Branch naming (advisory)

| Prefix | Purpose |
|--------|---------|
| `fix/` | Bug fixes |
| `feat/` | New features |
| `chore/` | Maintenance, deps, docs |

### Install commands

```powershell
# Latest stable
irm https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.ps1 | iex

# Latest pre-release (dev)
irm https://raw.githubusercontent.com/jeircul/pim/main/scripts/install.ps1 | iex -Dev
```

### Rules

- Never push a `v*` tag manually — always use `task release:dev` or `task release:stable`.
- `release:stable` requires `main` to be up to date with `origin/main` (fetches before checking).
- `release:dev` is blocked on `main` and `renovate/*` branches.

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
- `PendingRoleAssignmentRequest` (HTTP 400) is treated as success at all scopes — the role is already activating.

Full API reference: `.agents/skills/golang/references/azure-pim-api.md`.

## Scope and role matching (headless)

- `--role` and `--scope` use exact-first, substring-fallback matching.
- Multiple substring matches with no exact match returns an ambiguity error.
- ARM scope paths take precedence over display-name matching.
- Bare subscription GUIDs expand to `/subscriptions/<guid>`; bare non-GUID tokens expand to the matching MG ARM path.

## Favorites behaviour

- `Favorite.Complete()` returns true when `role`, `scope`, `duration`, and `justification` are all non-empty.
- Dashboard 1–9 shortcut: if `Complete()` → `startWizard` with `AutoSubmit=true` and `favoritePending=true`; activation result shown as dashboard notice, TUI stays open. If not `Complete()` → error notice, no activation.
- `favoritePending` on `AppModel` distinguishes favorite-triggered activations (return to dashboard) from manual wizard activations (quit with summary).
- Favorites screen `ActivateMsg` always opens the wizard (incomplete favorites stop at the missing step).
- `autoAdvance` in `rolelist.go` uses `scopeFilter` as a tiebreaker when multiple roles share the same name. When all matches are MG-scoped and the filter is a bare subscription GUID, the first match is trusted (Azure rejects wrong-scope activations with 400/403). `scopeOverride` pins the subscription as `targetScope` when exactly one MG-scoped role is selected.
- `RecentActivation.EligibilityScope` stores the ARM eligibility path at activation time. Re-activation from the Recent screen prefers this over `Scope` so the wizard matches the original role precisely.
- `pim search --output toml` generates paste-ready `[[favorites]]` blocks with `scope = /subscriptions/<guid>`. This is the recommended workflow for building `config.toml`.

## Recent behaviour

- `R` from the dashboard opens the recent activations screen (last 10 successful activations).
- Each entry stores `Role`, `Scope` (resolved target), `EligibilityScope` (ARM eligibility path), `Duration`, `Justification`, `ActivatedAt`.
- Pressing Enter builds a `Favorite` using `EligibilityScope` as `Scope` when non-empty, falling back to `Scope`. This ensures re-activation is as precise as the original.
- Populated from both TUI (`ConfirmDoneMsg`) and headless (`AddRecentActivation` in `run.go`) success paths only — failed activations are never recorded.

## Skills

`.agents/skills/golang/` has full Go conventions, Bubble Tea v2 patterns, and the Azure PIM API reference.

- OpenCode: `skill golang` (auto-loads on Go/Azure tasks).
- Other agents: read `.agents/skills/golang/SKILL.md` and `references/`.

## Public-artifact sanitization (mandatory)

**Scope:** every PR body, commit message, README/CHANGELOG/docs edit, code comment, example block, and test fixture that will be committed, pushed, or posted publicly.

**Hard rule:** Identifiers that originate from user-pasted terminal output, chat history, or a live environment are sensitive by default. Never echo them verbatim into any committed or public artifact.

**Mandatory placeholder vocabulary:**

| Real thing | Placeholder to use |
|---|---|
| Subscription GUID | `00000000-0000-0000-0000-000000000000` |
| Management group / tenant name | `my-mgmt-group` |
| Subscription display name | `my-subscription` |
| Resource group | `my-rg` |
| Role names | Azure built-ins only: `Reader`, `Owner`, `Contributor` |

**Pre-write self-check (run before every commit/PR/doc edit):**
1. Does any value in this text originate from pasted terminal output or a live session?
2. Does it match `[0-9a-f]{8}-([0-9a-f]{4}-){3}[0-9a-f]{12}` and is it **not** an all-zero/repeated-nibble sentinel?
3. Is it a proper noun that is not an Azure built-in role or a well-known public Azure service name?

→ Any "yes" → replace with the placeholder above **before** writing.

**Mechanical backstop:** `task check` runs `scripts/check-secrets.sh` which scans staged content and key doc files. It also runs as a precondition of `task release:dev` and `task release:stable`. Add internal names to `.secrets-blocklist` (gitignored, never committed).
