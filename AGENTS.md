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
  internal/headless/   non-TUI execution path (run.go) + pim search subcommand (search.go)
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
- **MG/subscription ARM paths are flat (hard constraint).** `/providers/Microsoft.Management/managementGroups/{id}` and `/subscriptions/{guid}` are structurally unrelated strings — `ScopeIsChildOf` cannot cross this boundary. A subscription is semantically a child of an MG but never a string-path child. Never infer MG→subscription parentage from scope strings alone.
- **`$getAllChildren=true` is headless-only.** Per-node timeout is 15 s (`mgNodeTimeoutDefault` in `discovery.go`). On enterprise tenants this consistently stalls. Never call `ListAllSubscriptionsUnderMG` synchronously in an interactive path (TUI render, dashboard shortcut, favorite activation). Favorites carry pre-resolved `scope` + `schedule_id` instead.
- **`EligibilityScheduleID` is the canonical activation key.** It is globally unique per (principal, role definition, eligibility scope) and is what `ActivateRole` sends in `linkedRoleEligibilityScheduleID`. When known, select roles by exact ID match — never by name/scope heuristics. `pim search --output toml` always emits it; users should never hand-write it.

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
- **Role selection precedence in `autoAdvance`** (`rolelist.go`):
  1. Exact `schedule_id` match — iterates full role list, bypasses all heuristics
  2. Exact `eligibility_scope` match — pre-filters by MG ARM path before narrowing
  3. Scope child-of narrowing (`ScopeIsChildOf` / `ScopeMatches`) — narrows to single match
  4. Single-MG-candidate trust — only when exactly one MG-scoped match and a bare sub GUID filter
  5. Fall through → manual role list (safe default for ambiguous cases)
- `RecentActivation.EligibilityScope` stores the ARM eligibility path at activation time. Re-activation from the Recent screen prefers this over `Scope` so the wizard matches the original role precisely.
- `pim search --output toml` generates paste-ready `[[favorites]]` blocks with `scope`, `eligibility_scope` (MG-inherited roles only), and `schedule_id` pre-filled. Users fill in `duration`, `justification`, and `key` only. Never write `schedule_id` or `eligibility_scope` by hand.
- `Favorite.ScheduleID` is the `EligibilityScheduleID` from the Azure PIM API — the globally-unique key used by `ActivateRole` in `linkedRoleEligibilityScheduleID`. When set, `autoAdvance` selects the matching role by exact ID, bypassing all name/scope heuristics. Falls through to `eligibility_scope` + heuristics when empty (hand-written or pre-`schedule_id` favorites).
- `scope` and `schedule_id` are always both required even when they reference the same subscription: `scope` is the PUT URL target; `schedule_id` identifies the eligibility schedule in the request body.

## pim search

`pim search [query] [--mg filter] [--output table|json|toml]` discovers eligible
subscriptions across all management groups.

**Pipeline:** `GetEligibleRoles` → `buildSearchHits` (MG expansion + `subRoleMap`) →
filter → output formatter. `buildSearchHits` returns `([]SearchHit, subRoleMap, error)`;
the `subRoleMap` carries the exact `azure.Role` that granted each role to each
subscription, captured at expansion time. `tomlFromHits` consumes both.

**`--output toml` contract:** emits one `[[favorites]]` block per (subscription, role)
with `scope` (activation target), `eligibility_scope` (MG-inherited only), and
`schedule_id` (`EligibilityScheduleID`) pre-filled. Users fill in `duration`,
`justification`, and `key` only. Never write `schedule_id` or `eligibility_scope`
by hand.

**MG timeout behaviour:** inaccessible MG nodes are skipped as stderr warnings —
not fatal. Results are partial but correct for the nodes that responded.

See `.agents/skills/golang/references/mg-search.md` for the full design reference.

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
4. Does this text include subscription display names, management group names, or role names from `pim search` terminal output? Those are high-risk — they originate from a live environment and must be replaced with the placeholder vocabulary above before writing.

→ Any "yes" → replace with the placeholder above **before** writing.

**Mechanical backstop:** `task check` runs `scripts/check-secrets.sh` which scans staged content and key doc files. It also runs as a precondition of `task release:dev` and `task release:stable`. Add internal names to `.secrets-blocklist` (gitignored, never committed).
