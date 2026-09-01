---
name: bubbletea-wizard
description: "Multi-step Bubble Tea v2 wizard patterns — step enum, sub-model key-consumption arbitration, CLI-flag step-skipping, queue-driven multi-pass steps, back-navigation, parallel async loading, and Silent mode. Use when building or modifying a multi-step TUI wizard with Bubble Tea v2."
---

# Bubble Tea v2 — Multi-Step Wizard Patterns

> For Bubble Tea v2 lifecycle basics (Init/Update/View, KeyPressMsg, tea.View) see the `golang` skill.
> This skill covers wizard-specific composition patterns derived from the pim activation wizard
> (`internal/tui/activate/`).

---

## 1. Wizard structure

### Step enum

```go
type wizardStep int

const (
    stepRoleList wizardStep = iota
    stepScopeTree
    stepOptions
    stepConfirm
)
```

One `iota` constant per logical step. The enum is unexported; only the wizard package needs it.

### Root wizard struct

```go
type Wizard struct {
    theme  styles.Theme
    keys   styles.KeyMap
    deps   Deps
    step   wizardStep
    width  int
    height int

    roleList  RoleList
    scopeTree ScopeTree
    options   Options
    confirm   Confirm

    selectedRoles []azure.Role
    scopeQueue    []azure.Role
    items         []activationItem
    scopeVisited  bool
}
```

One sub-model field per step. Accumulation state (`selectedRoles`, `scopeQueue`, `items`) lives on the root, not on sub-models.

### Deps struct

```go
type Deps struct {
    PrincipalID string
    RoleFilter  []string
    ScopeFilter []string
    TimeStr     string
    Justific    string
    AutoSubmit  bool
    Silent      bool
    Store       *state.Store
    LoadRoles   func() ([]azure.Role, error)
    LoadActive  func() ([]azure.ActiveAssignment, error)
    LoadSubs    func(mgID string) ([]azure.ManagementGroup, []azure.Subscription, error)
    LoadRGs     func(subID string) ([]azure.ResourceGroup, error)
    Activate    func(role azure.Role, principalID, justification string, minutes int, targetScope string) error
}
```

All external I/O is injected as function fields. Sub-packages never import concrete client types.
The caller (app layer) closes over `context.Context` inside each func — the wizard never stores a context.

### New() and Init()

```go
func New(theme styles.Theme, keys styles.KeyMap, deps Deps) Wizard {
    w := Wizard{theme: theme, keys: keys, deps: deps}
    w.roleList = NewRoleList(theme, keys, deps.LoadActive, deps.RoleFilter, deps.ScopeFilter, deps.LoadRoles)
    return w
}

func (w Wizard) Init() tea.Cmd {
    return w.roleList.Init()
}
```

Only the first step's sub-model is constructed in `New()`. Subsequent sub-models are constructed
in their respective `start*` functions when the wizard actually reaches that step.

If the parent `AppModel` may receive `WindowSizeMsg` before constructing the wizard, propagate
the current size immediately after `New()`:

```go
m.wizardModel = activate.New(m.theme, m.keys, deps).WithSize(m.width, m.height)
```

`WithSize` must copy dimensions to the wizard and every sub-model field so newly-created steps
do not render with `height == 0`.

---

## 2. Sub-model key-consumption arbitration (most critical pattern)

**Problem**: `esc` and `q` are both "back/cancel" at the wizard level AND valid characters inside
a filter text field. Without arbitration, pressing `esc` to exit filter mode also cancels the wizard.

**Solution**: delegate to the active sub-model first, then check whether it consumed the key.

```go
var cmd tea.Cmd
var consumed bool
switch w.step {
case stepRoleList:
    prev := w.roleList
    w.roleList, cmd = w.roleList.Update(msg)
    consumed = roleListConsumed(prev, w.roleList, msg)
case stepScopeTree:
    prev := w.scopeTree
    w.scopeTree, cmd = w.scopeTree.Update(msg)
    consumed = scopeTreeConsumed(prev, w.scopeTree, msg)
case stepOptions:
    w.options, cmd = w.options.Update(msg)
case stepConfirm:
    w.confirm, cmd = w.confirm.Update(msg)
}
if cmd != nil || consumed {
    return w, cmd
}

if kp, ok := msg.(tea.KeyPressMsg); ok {
    if kp.String() == "esc" || kp.String() == "q" {
        return w.handleBack()
    }
}
```

The `roleListConsumed` helper:

```go
func roleListConsumed(prev, next RoleList, msg tea.Msg) bool {
    kp, ok := msg.(tea.KeyPressMsg)
    if !ok {
        return false
    }
    if prev.filtering {
        return true
    }
    return false
}
```

When a sub-model was filtering before the key press, treat the key as consumed, including
`enter` and `esc`; otherwise `esc` exits the filter and then bubbles to wizard-level back/cancel.

For `ScopeTree`, `/` opens filter mode and must also be consumed:

```go
func scopeTreeConsumed(prev, next ScopeTree, msg tea.Msg) bool {
    kp, ok := msg.(tea.KeyPressMsg)
    if !ok {
        return false
    }
    if prev.filtering {
        return true
    }
    return kp.String() == "/" && next.filtering
}
```

For sub-models with a text input that returns a no-op cmd when focused (e.g. `Options`), the
`cmd != nil` branch handles it — the options step returns `func() tea.Msg { return nil }` while
`focusJust` is true, which is non-nil and short-circuits wizard-level key handling.

---

## 3. WindowSizeMsg fan-out

Propagate to **all** sub-model fields, not just the active step:

```go
case tea.WindowSizeMsg:
    w = w.WithSize(msg.Width, msg.Height)
```

If you only update the active step, navigating back to a previously-rendered step will use stale
dimensions. Direct field assignment is fine because sub-models are value types on the wizard struct.

When a later step constructs a fresh sub-model (for example `startNextScopeTree`), copy the
wizard's current `width/height` into that new sub-model before returning its `Init()` command.
Scope-tree viewports must also handle `height == 0` defensively by rendering all current rows,
not one row, because the size event may have arrived before the step existed.

---

## 4. CLI-flag step-skipping (fast-forward)

Check flags inside `start*` functions, not in `Update`. Each `start*` function is the single
decision point for whether its step is shown.

```go
func (w Wizard) startOptions() (Wizard, tea.Cmd) {
    // ... construct w.options ...
    w.step = stepOptions

    if w.deps.TimeStr != "" && w.deps.Justific != "" {
        mins, err := azure.ParseDurationMinutes(w.deps.TimeStr)
        if err == nil {
            return w.startConfirm(mins, w.deps.Justific)
        }
    }

    return w, w.options.Init()
}
```

```go
func (w Wizard) startConfirm(minutes int, justification string) (Wizard, tea.Cmd) {
    // ... construct w.confirm ...
    w.step = stepConfirm

    if w.deps.AutoSubmit {
        return w, func() tea.Msg { return autoConfirmMsg{} }
    }

    return w, w.confirm.Init()
}
```

`autoConfirmMsg` is a typed message, not an inline function call. This keeps `Update` as the
single source of truth — `handleAutoConfirm` runs inside the normal message loop.

```go
type autoConfirmMsg struct{}

func (w Wizard) handleAutoConfirm() (Wizard, tea.Cmd) {
    if w.step != stepConfirm {
        return w, nil
    }
    w.confirm.submitted = true
    cmds := make([]tea.Cmd, len(w.confirm.items))
    for i := range w.confirm.items {
        w.confirm.items[i].status = statusRunning
        cmds[i] = w.confirm.runActivation(i)
    }
    return w, tea.Batch(append([]tea.Cmd{w.confirm.spinner.Init()}, cmds...)...)
}
```

---

## 5. Queue-driven multi-pass step

When one logical step must run N times (e.g. scope selection for N roles), use a queue:

```go
scopeQueue []azure.Role
```

On `ScopeTreeDoneMsg`, accumulate results, pop the queue, and either start the next tree or advance:

```go
case ScopeTreeDoneMsg:
    for _, scope := range msg.Scopes {
        w.items = append(w.items, activationItem{
            role:        msg.Role,
            targetScope: scope,
        })
    }
    if len(w.scopeQueue) > 0 {
        w.scopeQueue = w.scopeQueue[1:]
    }
    if len(w.scopeQueue) > 0 {
        return w.startNextScopeTree()
    }
    return w.startOptions()
```

`startNextScopeTree` always reads `w.scopeQueue[0]` — the queue is the authoritative cursor.

---

## 6. Back-navigation that rebuilds state

Going back from `stepOptions` to `stepScopeTree` must re-derive `scopeQueue` from `selectedRoles`.
Do not cache the queue from the forward pass — it was consumed.

```go
case stepOptions:
    if w.scopeVisited {
        w.items = nil
        var treeRoles []azure.Role
        for _, r := range w.selectedRoles {
            switch r.ScopeKind() {
            case azure.ScopeManagementGroup, azure.ScopeSubscription:
                if w.scopeOverride(r) == "" {
                    treeRoles = append(treeRoles, r)
                }
            }
        }
        w.scopeQueue = treeRoles
        w.step = stepScopeTree
        return w, w.scopeTree.Init()
    }
    w.step = stepRoleList
    return w, w.roleList.Init()
```

`selectedRoles` is the canonical state. `scopeQueue` and `items` are always derived from it.
`scopeVisited` tracks whether the scope tree was shown this run so back-nav routes correctly.

---

## 7. Silent mode

`Deps.Silent` suppresses rendering during programmatic activation (e.g. dashboard favorite shortcut
with `AutoSubmit=true`). The role list flashes briefly otherwise.

```go
func (w Wizard) View() string {
    if w.deps.Silent && w.step == stepRoleList {
        return ""
    }
    // ... normal render ...
}
```

**`Silent` vs `AutoSubmit`**:

| Flag | Set by | Effect |
|------|--------|--------|
| `AutoSubmit` | `--yes` CLI flag OR favorite shortcut | Skips confirm step interaction |
| `Silent` | Favorite shortcut only | Suppresses role-list render |

`--yes` from the CLI should still show the UI (user is watching). Favorite shortcut activates
in the background — both `AutoSubmit` and `Silent` are set together.

---

## 8. Editing() propagation

The root `AppModel` calls `wizard.Editing()` to gate global hotkeys (e.g. `?` for help overlay).
The wizard delegates to the active sub-model:

```go
func (w Wizard) Editing() bool {
    switch w.step {
    case stepRoleList:
        return w.roleList.Editing()
    case stepScopeTree:
        return w.scopeTree.Editing()
    case stepOptions:
        return w.options.Editing()
    }
    return false
}
```

Sub-models implement `Editing() bool`:

```go
// RoleList
func (m RoleList) Editing() bool { return m.filtering }

// ScopeTree
func (m ScopeTree) Editing() bool { return m.filtering }

// Options
func (m Options) Editing() bool { return m.focusJust }
```

Steps without text input (Confirm) are not listed in the switch — they return `false` by default.

---

## 9. Exit routing with sentinel flag

`favoritePending bool` on `AppModel` distinguishes two entry points with different exit semantics:

| Entry point | `favoritePending` | `WizardDoneMsg` behaviour |
|-------------|-------------------|--------------------------|
| Dashboard 1–9 shortcut | `true` | Return to dashboard with notice |
| Manual `activate` command | `false` | Quit with summary |

```go
// In startWizard (app layer):
if autoSubmit {
    deps.AutoSubmit = true
    deps.Silent = true
    m.favoritePending = true
}
```

```go
// In AppModel.Update:
case activate.WizardDoneMsg:
    if m.favoritePending {
        m.favoritePending = false
        summary, err := buildActivationSummary(msg.Results)
        notice := strings.TrimRight(summary, "\n")
        m.dashboardModel.SetNotice(notice, err != nil)
        m.screen = ScreenDashboard
        return m, nil
    }
    m.exitSummary, m.exitErr = buildActivationSummary(msg.Results)
    return m, tea.Quit

case activate.WizardCancelMsg:
    if m.favoritePending {
        m.dashboardModel.SetNotice("activation cancelled — verify role/scope in favorites (f)", true)
    }
    m.favoritePending = false
    m.screen = ScreenDashboard
    return m, nil
```

**Clear `favoritePending` on both `WizardDoneMsg` and `WizardCancelMsg`.** If you only clear it
on done, a cancelled favorite activation leaves the flag set and the next manual activation
incorrectly returns to the dashboard instead of quitting.

---

## autoAdvance — programmatic step-skip with resolution ladder

`autoAdvance` (`internal/tui/activate/rolelist.go`) fires automatically after roles
load when `roleFilter` or `scheduleID` is set. It resolves which role to select
without user interaction.

**Resolution order (highest priority first):**

1. **Exact `schedule_id`** — if `m.scheduleID != ""`, scans `m.roles` (full list,
   not just visible) for `EqualFold(r.EligibilityScheduleID, m.scheduleID)`. Emits
   that role and bypasses all further logic. Zero heuristics.

2. **Exact `eligibility_scope`** — if `m.eligibilityScope != ""`, pre-filters matches
   by `EqualFold(r.Scope, m.eligibilityScope)`. If exactly 1 survives, emits it.

3. **Scope child-of narrowing** — `ScopeMatches` / `ScopeIsChildOf` against
   `scopeFilter`. If exactly 1 match survives narrowing, emits it.

4. **Single-MG-candidate trust** — if the scope filter is a bare subscription GUID
   and exactly 1 of the name-matches is MG-scoped: trust it. `scopeOverride`
   in `wizard.go` then pins the subscription as `targetScope`. Count the MG-scoped
   matches; do not test the total match count, which is already >1 at this point.

5. **Fall through → `return nil`** — the safe default. The role list renders for
   manual selection. This is correct for ambiguous cases (2+ same-named MG roles,
   no `schedule_id` set).

**Key rule:** `return nil` from `autoAdvance` is the safe default, not a failure.
Never pick `matches[0]` when multiple MG-scoped candidates exist without a
discriminating key (`schedule_id` or `eligibility_scope`).

**Coupling rule:** `AutoSubmit=true` from a background trigger (dashboard 1–9 shortcut,
recent re-activation) must always also set `Silent=true`. `Silent` suppresses the role
list render while `autoAdvance` resolves asynchronously, preventing a flash/blank-screen.
`--yes` (user-facing) sets `AutoSubmit` without `Silent` — the user is watching.

**Non-determinism guard:** `autoAdvance` selection depends on `m.roles` order.
`GetEligibleRoles` sorts by `(Scope, RoleName)` before returning — this sort is
load-bearing. Do not remove it or add pre-selection logic that assumes arbitrary order.

---

## Reference

Detailed code examples with line citations: `references/wizard-patterns.md`
