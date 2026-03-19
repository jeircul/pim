# PIM v2 Rewrite — Session Plan

## Branch: `rewrite/v2`

## Completed (commits oldest → newest)

| Commit | Summary |
|--------|---------|
| `ab96d90` | Key routing overhaul — delegation, autoConfirmMsg, scopeVisited, scopeOverride |
| `8bda596` | Checkbox rendering fix, active assignments filter fixed |
| `91af732` | Space key fix (`"space"` not `" "`), TUI context deadline fixed |
| `df95963` | Landing screen with logo, single-select role list, space in justification/filter |
| `865b94f` | ASCII logo, scope radio buttons, 11 duration choices, re-expand bug fix |
| `278d64d` | Scope multi-select `[x]`, duration 30m–8h (16 choices), logo style fix, wizard ScopeTreeDoneMsg handler |
| `d616a5e` | Esc in status view, scopeQueue panic guard, sub-scoped RG tree, defer GetCurrentUser() |

## Architecture Decisions

- **No CLI framework** — hand-rolled flag parsing, no Cobra/urfave
- **Go 1.26**, Charm v2 stack: bubbletea/v2, bubbles/v2, lipgloss/v2
- Root model `View()` returns `tea.View` (v2 API); sub-models return `string`
- Key messages: `tea.KeyPressMsg` (v2), `msg.String()` for matching; space key returns `"space"`
- No inline comments in function bodies; godoc on exported symbols
- No testify — stdlib testing only; table-driven tests
- Run `task fmt && task test` before every commit

## Key Discoveries

### bubbletea v2 `KeyPressMsg.String()` returns `"space"` not `" "`
All handlers checking `msg.String() == " "` were dead code. Fixed across rolelist, scopetree, deactivate, options.

### `theme.Title` has `PaddingBottom(1)`
Defined in `internal/tui/styles/theme.go`. Using it for logo lines or section headers adds blank lines. Use `lipgloss.NewStyle().Foreground(m.theme.Accent).Bold(true)` for the logo, `m.theme.Bold` for section headers.

### Startup was blocking on 3 synchronous network calls
`GetCurrentUser()` in `tui.New()` was acquiring 2 tokens + 1 Graph API call before the TUI rendered. Now deferred to a background `tea.Cmd` via `userReadyMsg` — dashboard renders instantly. `principalID` resolves before any PIM API call needs it.

### Scope tree supports MG → Sub → RG hierarchy
`expandNode()` dispatches `loadSubs` for MG nodes, `loadRGs` for subscription nodes. `scopeChildrenMsg` handler appends children. `NewScopeTreeForSub()` added for subscription-rooted trees (user can select the sub directly or drill into RGs).

## Open Bug: RG Scope Drill-Down Not Working

**User report:** "RG scope drill down still not working" after the `d616a5e` commit.

### Investigation Summary

The code logic traces correctly through all paths:

- `advanceFromRoles()` routes `ScopeSubscription` roles to `startNextScopeTree()`
- `startNextScopeTree()` calls `NewScopeTreeForSub()` with `subRoot: true`
- Right/l key handler condition `m.subRoot` allows root subscription node expansion
- `expandNode()` calls `loadRGs(subID)` for subscription nodes
- `scopeChildrenMsg` handler appends RG children and calls `flatten()`

### Likely Root Causes (investigate in order)

**1. API error silently swallowed** — `scopeChildrenMsg` handler sets `n.loaded = true`
and `n.expanded = true` even when `msg.err != nil`. If `ListSubscriptionResourceGroups`
returns a 403 (e.g. because the user hasn't activated the role yet and lacks Reader at
sub level), the node shows as expanded with zero children and no feedback.

Fix: add an `err` field to `ScopeTree`, set it in the `scopeChildrenMsg` handler, render
it under the node in `View()`.

```go
// in ScopeTree struct
err error

// in scopeChildrenMsg handler
if msg.err != nil {
    n.loading = false
    n.loaded = true  // prevent retry loop
    m.err = msg.err
    m.flatten()
    break
}

// in View()
if m.err != nil {
    sb.WriteString(m.theme.Subtle.Render("  error: "+m.err.Error()) + "\n")
}
```

**2. `selected` map toggle bug** — `m.selected[scope] = !m.selected[scope]` sets the
value to `false` instead of deleting the key. Consequences:
- `len(m.selected)` counts `false` entries → "1 selected" shown even after deselecting
- Enter handler collects ALL keys including deselected ones into `ScopeTreeDoneMsg.Scopes`
  → deselected scopes get activated

Fix in `scopetree.go`:

```go
// space handler — toggle with delete
case msg.String() == "space":
    if m.cursor < len(m.flat) {
        scope := m.flat[m.cursor].scope
        if m.selected[scope] {
            delete(m.selected, scope)
        } else {
            m.selected[scope] = true
        }
    }

// enter handler — filter true values only
for s, ok := range m.selected {
    if ok {
        scopes = append(scopes, s)
    }
}

// selected count — count true values only
count := 0
for _, ok := range m.selected {
    if ok {
        count++
    }
}
```

**3. Possible scope format mismatch** — `findNode` uses exact string equality
(`n.scope == scope`). If the PIM API returns scopes with unexpected casing or trailing
slashes, the lookup fails silently. Add debug logging or surface the error.

### Debug Strategy

Use `tea.LogToFile("debug.log", "")` in `main.go` or add temporary stderr logging in
`expandNode` and the `scopeChildrenMsg` handler to confirm:

1. Whether `expandNode` is called when right is pressed on the sub-root node
2. Whether `loadRGs` returns an error or an empty slice
3. Whether `scopeChildrenMsg` arrives and `findNode` finds the parent

### Files to Change

| File | Change |
|------|--------|
| `internal/tui/activate/scopetree.go` | Fix selected map toggle (delete key), filter in enter handler, fix count, surface API errors |

## Relevant File Map

```
internal/
├── app/
│   └── app.go              DefaultContext() — 2-min timeout, headless only
├── azure/
│   ├── client.go            API client; ListSubscriptionResourceGroups
│   ├── types.go             Role, ActiveAssignment, ScopeKind()
│   └── scopes.go            Scope parsing helpers (SubscriptionIDFromScope etc.)
└── tui/
    ├── app.go               Root model, screen routing, deferred user fetch via userReadyMsg
    ├── theme.go             NewTheme wraps styles.NewTheme
    ├── styles/
    │   ├── theme.go         ⚠️  Title style has PaddingBottom(1) — avoid for logos/headers
    │   └── keys.go          Key bindings
    ├── components/
    │   ├── statusbar.go
    │   ├── help.go
    │   └── spinner.go
    ├── dashboard/
    │   └── dashboard.go     Landing screen (logo + favorites, no API calls)
    ├── activate/
    │   ├── wizard.go        4-step wizard coordinator
    │   ├── rolelist.go      Step 1: role selection (single-select + filter)
    │   ├── scopetree.go     Step 2: scope tree (MG or sub-rooted)  ← OPEN BUG
    │   ├── options.go       Step 3: duration (16 choices, 30m–8h) + justification
    │   └── confirm.go       Step 4: confirmation + parallel activation via tea.Batch
    ├── deactivate/
    │   └── deactivate.go    Deactivation flow
    ├── favorites/
    │   └── favorites.go     Favorites management
    └── status/
        └── status.go        Status view (active + eligible roles)
```
