# PIM v2 — Rewrite Plan

## Vision

Lightning-fast Azure PIM manager with a modern terminal UI (Bubble Tea v2) that mirrors the Azure portal activation flow. The TUI is the application — flags accelerate it, `--headless` bypasses it. Built to replace portal clickops entirely.

## Design Principles

1. **The TUI is the app** — Not a wrapper around a CLI. The terminal UI is the primary interface.
2. **Mirror the portal** — Steps match Azure portal: see status, pick roles, pick scope, set duration/justification, confirm.
3. **Flag acceleration** — Flags pre-fill TUI fields and auto-advance. `--headless` for CI/scripting.
4. **Speed over everything** — Parallel API calls, lazy loading, session cache, instant startup.
5. **State persistence** — Remember preferences, recent justifications, favorites. Frequent activations near-instant.
6. **Modern Go** — Go 1.26, iterators, Bubble Tea v2 ecosystem, no legacy patterns.

## Architecture

### Package Layout

```
pim/
├── main.go                     # Entrypoint: parse flags, detect mode, launch TUI or headless
├── go.mod                      # github.com/jeircul/pim, go 1.26
├── Taskfile.yml
├── .goreleaser.yaml
│
├── internal/
│   ├── app/                    # Application core
│   │   ├── app.go              # App struct: holds client, config, state. run() method
│   │   └── config.go           # CLI flag parsing, env vars, config file loading
│   │
│   ├── azure/                  # Azure PIM client (ported from pkg/azpim)
│   │   ├── client.go           # Auth, token caching, HTTP client
│   │   ├── roles.go            # GetEligibleRoles, GetActiveAssignments, IsRoleActive
│   │   ├── activate.go         # ActivateRole, DeactivateRole
│   │   ├── scopes.go           # Scope tree: MGs, subscriptions, resource groups
│   │   ├── types.go            # Domain types (Role, Assignment, Scope, etc.)
│   │   ├── duration.go         # ISO 8601 duration handling
│   │   └── errors.go           # Sentinel errors
│   │
│   ├── tui/                    # Bubble Tea TUI layer
│   │   ├── app.go              # Root tea.Model — screen routing, global keys
│   │   ├── theme.go            # Lip Gloss styles (mono + accent)
│   │   ├── keys.go             # Global keybindings
│   │   │
│   │   ├── dashboard/          # Screen: home dashboard
│   │   │   └── dashboard.go
│   │   │
│   │   ├── activate/           # Screen: activation wizard
│   │   │   ├── wizard.go       # Multi-step wizard coordinator
│   │   │   ├── rolelist.go     # Step 1: role selection
│   │   │   ├── scopetree.go    # Step 2: scope tree navigation
│   │   │   ├── options.go      # Step 3: duration + justification
│   │   │   └── confirm.go      # Step 4: review + submit
│   │   │
│   │   ├── status/             # Screen: status view
│   │   │   └── status.go
│   │   │
│   │   ├── deactivate/         # Screen: deactivation
│   │   │   └── deactivate.go
│   │   │
│   │   ├── favorites/          # Screen: manage favorites
│   │   │   └── favorites.go
│   │   │
│   │   └── components/         # Reusable TUI components
│   │       ├── header.go       # App header bar
│   │       ├── statusbar.go    # Bottom status bar
│   │       ├── spinner.go      # Loading spinner wrapper
│   │       └── tree.go         # Scope tree component
│   │
│   ├── state/                  # Persistent state
│   │   └── store.go            # ~/.config/pim/ — TOML config + state
│   │
│   └── headless/               # Non-TUI execution
│       └── run.go              # Flag-driven activation for --headless
│
├── docs/
│   └── plan.md
│
├── scripts/
│   ├── install.sh
│   └── install.ps1
│
└── .github/
    ├── copilot-instructions.md
    ├── instructions/
    │   └── go.instructions.md
    ├── renovate.json5
    └── workflows/
        ├── test.yml
        └── release.yml
```

### Dependency Graph

```
main.go
  └── internal/app
        ├── internal/azure      (API client)
        ├── internal/tui        (TUI layer)
        ├── internal/headless   (scripting mode)
        └── internal/state      (persistence)

internal/tui
  ├── internal/azure            (data fetching)
  ├── internal/state            (read/write prefs)
  └── charm.land/*              (bubbletea v2, bubbles v2, lipgloss v2, huh v2)

internal/headless
  ├── internal/azure
  └── internal/state
```

## CLI Modes

```
pim                          → TUI: dashboard (shows active roles)
pim activate                 → TUI: activation wizard from step 1
pim activate --role Reader   → TUI: wizard, step 1 pre-filtered to "Reader"
pim activate --role Reader --scope /sub/xxx --time 1h -j "ticket" --yes
                             → TUI: wizard auto-advances to confirm, auto-submits
pim activate ... --headless  → No TUI. Execute and exit. Exit code 0/1.
pim status                   → TUI: status screen
pim status --headless        → Print active roles to stdout (table or JSON)
pim deactivate               → TUI: deactivation screen
```

## TUI Screens

### 1. Dashboard (home)

```
┌─────────────────────────────────────────────┐
│  pim                               v2.0.0   │
├─────────────────────────────────────────────┤
│                                             │
│  Active Elevations (2)                      │
│  ────────────────────                       │
│  ● Reader    sub-prod-001     1h22m left    │
│  ● Owner     rg-dev-sandbox   0h14m left    │
│                                             │
│  Favorites                                  │
│  ─────────                                  │
│  1  Prod reader    Reader / sub-prod-001    │
│  2  Dev owner      Owner / sub-dev-002      │
│                                             │
│  Quick Actions                              │
│  ────────────                               │
│  [a] Activate  [s] Status  [d] Deactivate   │
│  [f] Favorites [q] Quit                     │
│                                             │
├─────────────────────────────────────────────┤
│  ↑/↓ navigate  1-9 quick-activate  ? help  │
└─────────────────────────────────────────────┘
```

- Active elevations with live countdown timers.
- Favorites with number-key shortcuts (1-9) for instant re-activation.
- Quick-action keys for all screens.

### 2. Activation Wizard

**Step 1: Role Selection**
```
┌─────────────────────────────────────────────┐
│  Activate  ◆─○─○─○                  1 / 4  │
├─────────────────────────────────────────────┤
│  Select roles to activate:                  │
│  ⌕ _                                        │
│                                             │
│    ☐  Contributor    mg-platform            │
│  ▸ ☑  Reader         sub-prod-001          │
│    ☐  Owner          sub-dev-002            │
│    ☐  Reader         rg-prod-frontend       │
│    ☑  Network Cont.  sub-network-hub        │
│                                             │
│  2 selected                                 │
├─────────────────────────────────────────────┤
│  ↑/↓ move  space toggle  / filter  → next  │
└─────────────────────────────────────────────┘
```

- Fuzzy search/filter inline.
- Multi-select with space. Already-active roles shown dimmed.
- Right arrow or Enter to proceed.

**Step 2: Scope Tree** (per MG-scoped role only)
```
┌─────────────────────────────────────────────┐
│  Activate  ○─◆─○─○  Reader          2 / 4  │
├─────────────────────────────────────────────┤
│  Choose scope for Reader:                   │
│                                             │
│  ▾ mg-platform                              │
│    ├─ ☑ sub-prod-001                        │
│    ├─ ☐ sub-prod-002                        │
│    └─ ▾ sub-dev-003                         │
│         ├─ ☐ rg-frontend                    │
│         ├─ ☐ rg-backend                     │
│         └─ ☐ rg-infra                       │
│                                             │
│  1 scope selected                           │
├─────────────────────────────────────────────┤
│  h/l collapse/expand  j/k move  space sel   │
└─────────────────────────────────────────────┘
```

- Vim-style tree navigation (h/j/k/l).
- Lazy-loads children on expand (spinner while fetching).
- Skipped if role scope is already subscription/RG level.

**Step 3: Options**
```
┌─────────────────────────────────────────────┐
│  Activate  ○─○─◆─○                  3 / 4  │
├─────────────────────────────────────────────┤
│                                             │
│  Duration:   ● 1h  ○ 2h  ○ 4h  ○ 8h  ○ …  │
│                                             │
│  Justification:                             │
│  ┌─────────────────────────────────────┐    │
│  │ Investigating alert in prod_        │    │
│  └─────────────────────────────────────┘    │
│                                             │
│  Recent:                                    │
│    1. Investigating alert in prod           │
│    2. Sprint deployment                     │
│    3. Routine maintenance                   │
│                                             │
├─────────────────────────────────────────────┤
│  tab next field  ↑/↓ recent  → next        │
└─────────────────────────────────────────────┘
```

- Duration radio buttons (common values) or type custom.
- Justification text input with recent history from state file.

**Step 4: Confirm + Execute**
```
┌─────────────────────────────────────────────┐
│  Activate  ○─○─○─◆                  4 / 4  │
├─────────────────────────────────────────────┤
│                                             │
│  Activating 2 role(s) for 1h:              │
│                                             │
│  Reader         sub-prod-001        ● done  │
│  Network Cont.  sub-network-hub  ◌ pending  │
│                                             │
│  Justification: "Investigating alert"       │
│                                             │
├─────────────────────────────────────────────┤
│  enter confirm  ← back  q cancel            │
└─────────────────────────────────────────────┘
```

- Summary of all activations.
- After confirmation, live progress (spinner → checkmark/X per role).
- Back to revise, q to cancel.

### 3. Status Screen

Active roles (with time remaining) and eligible roles in tables.
Keys: `a` activate, `d` deactivate, `r` refresh.

### 4. Deactivate Screen

List of active roles. Multi-select, confirm, execute.

### 5. Favorites Screen

Full CRUD for saved role+scope+duration combos. Assign number-key (1-9) shortcuts. Edit labels.

## Data Flow

```
Startup:
  1. Parse flags + load ~/.config/pim/config.toml + state.toml
  2. Create azure.Client (AzureCLI → PowerShell → DeviceCode)
  3. Parallel: authenticate + fetch user | fetch eligible roles
  4. Parallel: fetch active assignments
  5. Launch TUI with pre-fetched data (or headless path)

Activation:
  1. User selects roles (pre-filtered by flags if provided)
  2. MG-scoped roles: lazy-load scope tree on expand
  3. User sets duration + justification
  4. Submit activations (parallel where possible)
  5. Update cache, save justification to recent history

Session cache:
  - Eligible roles: fetched once on startup
  - Active assignments: refreshed on activate/deactivate
  - Scope tree children: cached on first expand
  - Tokens: cached by Azure SDK
```

## State Persistence

Directory: `~/.config/pim/`

**config.toml** — User preferences (hand-editable):
```toml
[preferences]
default_duration = "1h"

[[favorites]]
label = "Prod reader"
role = "Reader"
scope = "/subscriptions/xxx-yyy"
duration = "1h"
key = 1

[[favorites]]
label = "Dev owner"
role = "Owner"
scope = "/subscriptions/zzz-www/resourceGroups/rg-dev"
duration = "2h"
key = 2
```

**state.toml** — Auto-managed (not hand-edited):
```toml
version = 1

recent_justifications = [
  "Investigating alert in prod",
  "Sprint deployment",
  "Routine maintenance",
]
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `charm.land/bubbletea/v2` | TUI framework (Elm architecture) |
| `charm.land/bubbles/v2` | List, table, spinner, help, key components |
| `charm.land/lipgloss/v2` | Styling, layout, borders |
| `charm.land/huh/v2` | Form fields for wizard steps |
| `github.com/azure/azure-sdk-for-go/sdk/azidentity` | Azure credential chain |
| `github.com/azure/azure-sdk-for-go/sdk/azcore` | Token management |
| `github.com/google/uuid` | Request IDs |
| `github.com/BurntSushi/toml` | TOML config/state parsing |

Removed: `lithammer/fuzzysearch` (Bubble Tea list has built-in filtering).

## Theme: Minimal Mono + Accent

```
Background:  terminal default
Text:        terminal default foreground
Accent:      adaptive (blue on light, cyan on dark terminals)
Active:      green
Expired:     dim/gray
Error:       red
Borders:     rounded (lipgloss.RoundedBorder)
Selection:   reverse video
```

ANSI colors only — adapts to any terminal palette via Lip Gloss adaptive colors.

## Implementation Phases

### Phase 1: Foundation
- New package layout (internal/azure, internal/app, internal/tui, internal/state)
- Port pkg/azpim → internal/azure (preserve API logic, modernize patterns)
- main.go with flag parsing and mode detection
- State store (TOML read/write)
- Headless path (port current behavior for --headless)
- Tests for azure client and state store

### Phase 2: TUI Shell + Dashboard
- Root TUI model with screen routing
- Theme/styling constants
- Dashboard screen (active elevations with live timers, favorites 1-9 shortcuts)
- Status screen (active + eligible roles in tables)
- Global keybindings (q, ?, a, s, d, f)

### Phase 3: Activation Wizard
- Step 1: Role selection (filterable list, multi-select)
- Step 2: Scope tree (lazy loading, vim nav, expand/collapse)
- Step 3: Options form (duration radio + justification + recent history)
- Step 4: Confirm + execute (live progress per role)
- Wizard navigation + step indicator
- Flag acceleration (pre-fill, auto-advance, --yes auto-submit)

### Phase 4: Deactivation + Favorites + Polish
- Deactivation screen
- Favorites screen (CRUD, dashboard 1-9 shortcuts)
- Error states (auth failure, network timeout, expired token)
- Resize handling
- Help overlay (?)
- README rewrite

### Phase 5: Automation + Release
- --headless mode with all flags
- --output json for scripting
- Shell completions
- Integration tests (testscript)
- GoReleaser config update
- v2.0.0 release

## Files to Delete

```
pkg/            # Ported to internal/azure
internal/cli/   # Replaced by internal/tui + internal/headless
```
