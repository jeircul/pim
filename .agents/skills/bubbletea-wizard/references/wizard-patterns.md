# Wizard Patterns — Code Reference

Source: `internal/tui/activate/` in the pim project.

---

## 1. `roleListConsumed` — key arbitration helper

`wizard.go:222`

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

**Why `prev` not `next`**: the decision must be based on the state *before* the sub-model
processed the key. If `prev.filtering` is true, the sub-model was in filter mode when the key
arrived — it consumed it. After `enter`/`esc`, `next.filtering` will be false, but the wizard
must still treat the key as consumed so `esc` does not bubble into wizard back/cancel.

Called at `wizard.go:151`:

```go
case stepRoleList:
    prev := w.roleList
    w.roleList, cmd = w.roleList.Update(msg)
    consumed = roleListConsumed(prev, w.roleList, msg)
case stepScopeTree:
    prev := w.scopeTree
    w.scopeTree, cmd = w.scopeTree.Update(msg)
    consumed = scopeTreeConsumed(prev, w.scopeTree, msg)
```

---

## 2. `WindowSizeMsg` fan-out

`wizard.go:98`

```go
case tea.WindowSizeMsg:
    w = w.WithSize(msg.Width, msg.Height)
```

All four sub-model fields are updated regardless of `w.step`. Direct field assignment works
because sub-models are value types embedded in the wizard struct.

When the parent app constructs the wizard after the initial terminal resize, call
`activate.New(...).WithSize(m.width, m.height)`. When a later step constructs a fresh sub-model
inside `startNextScopeTree`, copy `w.width/w.height` into that new scope tree before `Init()`.

---

## 3. `autoConfirmMsg` typed-message pattern

`wizard.go:48` — type declaration:

```go
type autoConfirmMsg struct{}
```

`wizard.go:362` — emitted from `startConfirm`:

```go
if w.deps.AutoSubmit {
    return w, func() tea.Msg { return autoConfirmMsg{} }
}
```

`wizard.go:139` — handled in `Update`:

```go
case autoConfirmMsg:
    return w.handleAutoConfirm()
```

`wizard.go:207` — handler:

```go
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

The guard `if w.step != stepConfirm` is defensive — `autoConfirmMsg` is only emitted from
`startConfirm`, so `w.step` will always be `stepConfirm` when it arrives. The guard prevents
a stale message from a previous wizard run causing a panic on an empty `w.confirm.items`.

---

## 4. `scopeQueue` pop-and-advance

`wizard.go:115`

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

`startNextScopeTree` at `wizard.go:312`:

```go
func (w Wizard) startNextScopeTree() (Wizard, tea.Cmd) {
    role := w.scopeQueue[0]
    if role.ScopeKind() == azure.ScopeSubscription {
        w.scopeTree = NewScopeTreeForSub(w.theme, w.keys, role, w.deps.LoadRGs)
    } else {
        w.scopeTree = NewScopeTree(w.theme, w.keys, role, w.deps.LoadSubs, w.deps.LoadRGs)
    }
    w.scopeTree.width = w.width
    w.scopeTree.height = w.height
    w.step = stepScopeTree
    w.scopeVisited = true
    return w, w.scopeTree.Init()
}
```

The queue is a slice; pop is `w.scopeQueue = w.scopeQueue[1:]`. No index cursor needed.

---

## 5. `handleBack` re-derivation

`wizard.go:173`

```go
func (w Wizard) handleBack() (Wizard, tea.Cmd) {
    switch w.step {
    case stepRoleList:
        return w, func() tea.Msg { return WizardCancelMsg{} }
    case stepScopeTree:
        w.step = stepRoleList
        return w, w.roleList.Init()
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
    case stepConfirm:
        w.step = stepOptions
        return w, w.options.Init()
    }
    return w, nil
}
```

`w.items = nil` clears accumulated scope selections before re-entering the scope tree.
`w.scopeQueue` is rebuilt from `w.selectedRoles` — the queue was consumed during the forward
pass and cannot be reused.

---

## 6. `Silent` vs `AutoSubmit` distinction

`app.go:344` — favorite shortcut sets both:

```go
if autoSubmit {
    deps.AutoSubmit = true
    deps.Silent = true
    m.favoritePending = true
}
```

`app.go:311` — `--yes` CLI flag sets only `AutoSubmit`:

```go
AutoSubmit: cfg.Yes,
```

`wizard.go:237` — `Silent` check in `View()`:

```go
if w.deps.Silent && w.step == stepRoleList {
    return ""
}
```

`wizard.go:362` — `AutoSubmit` check in `startConfirm`:

```go
if w.deps.AutoSubmit {
    return w, func() tea.Msg { return autoConfirmMsg{} }
}
```

`Silent` only suppresses the role-list render. `AutoSubmit` skips the confirm interaction.
A `--yes` CLI invocation shows the role list (user is watching) but skips the confirm prompt.
A favorite shortcut does neither (role list is invisible, confirm is auto-submitted).

---

## 7. `Editing()` chain

`app.go` (root model, abbreviated):

```go
if kp, ok := msg.(tea.KeyPressMsg); ok {
    if kp.String() == "?" && !m.wizardModel.Editing() {
        m.showHelp = !m.showHelp
    }
}
```

`wizard.go:85`:

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

`rolelist.go:239`:

```go
func (m RoleList) Editing() bool { return m.filtering }
```

`scopetree.go`:

```go
func (m ScopeTree) Editing() bool { return m.filtering }
```

`options.go:172`:

```go
func (m Options) Editing() bool { return m.focusJust }
```

Steps without text input (`Confirm`) are absent from the wizard switch — they return `false` by default.

---

## 8. `favoritePending` sentinel — both clear points

`app.go:161` — `WizardDoneMsg`:

```go
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
```

`app.go:173` — `WizardCancelMsg`:

```go
case activate.WizardCancelMsg:
    if m.favoritePending {
        m.dashboardModel.SetNotice("activation cancelled — verify role/scope in favorites (f)", true)
    }
    m.favoritePending = false
    m.screen = ScreenDashboard
    return m, nil
```

Both branches clear `m.favoritePending = false`. The cancel branch clears unconditionally
(the `if` only gates the notice message, not the clear). If the cancel branch only cleared
inside the `if`, a non-favorite cancel would leave `favoritePending` in whatever state it was —
harmless today but fragile.

---

## 9. Parallel async loading in `RoleList.Init()`

`rolelist.go:65`

```go
func (m RoleList) Init() tea.Cmd {
    return tea.Batch(
        m.spinner.Init(),
        func() tea.Msg {
            var (
                roles    []azure.Role
                active   []azure.ActiveAssignment
                rolesErr error
            )
            done := make(chan struct{}, 2)
            go func() {
                roles, rolesErr = m.loadFunc()
                done <- struct{}{}
            }()
            go func() {
                if m.loadActiveFn != nil {
                    active, _ = m.loadActiveFn()
                }
                done <- struct{}{}
            }()
            <-done
            <-done
            return roleListLoadMsg{roles: roles, active: active, err: rolesErr}
        },
    )
}
```

Both fetches run concurrently inside a single `tea.Cmd`. The cmd returns one `roleListLoadMsg`
carrying both results. Active-assignment errors are silently ignored (`_`) — the active indicator
is cosmetic; a missing active set degrades gracefully.
