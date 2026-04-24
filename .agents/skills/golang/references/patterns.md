# Patterns Reference — pim

Detailed patterns for the three main domains: Bubble Tea v2, Azure client, and state/config.

---

## Bubble Tea v2 lifecycle

```
Init() tea.Cmd
  └─ returns initial command (e.g. fetch data, start spinner)

Update(tea.Msg) (tea.Model, tea.Cmd)
  └─ never mutates in place — return updated copy + next command

View() tea.View
  └─ pure; no side effects; called after every Update
```

### View construction

```go
func (m Model) View() tea.View {
    var sb strings.Builder
    // ... render content ...
    v := tea.NewView(sb.String())
    v.AltScreen = true        // request alt screen (root model only)
    v.WindowTitle = "pim"     // terminal title (root model only)
    return v
}
```

Sub-models (dashboard, activate, etc.) return a `string` from their `View()` because
only the root `AppModel.View()` wraps into `tea.View`. Sub-model signatures:

```go
func (m Model) View() string { ... }
```

### Message routing

```go
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.WindowSizeMsg:
        m.width, m.height = msg.Width, msg.Height

    case tea.BackgroundColorMsg:          // fired once on startup
        m.theme = styles.NewTheme(msg.IsDark())
        return m, nil

    case tea.KeyPressMsg:                 // v2: NOT tea.KeyMsg
        switch msg.String() {
        case "q", "ctrl+c":
            return m, tea.Quit
        case "?":
            m.showHelp = !m.showHelp
            return m, nil
        }
    }
    // delegate to active sub-model
    var cmd tea.Cmd
    m.activeModel, cmd = m.activeModel.Update(msg)
    return m, cmd
}
```

### Async commands

```go
// Wrap blocking calls in tea.Cmd:
func loadRoles(client *azure.Client) tea.Cmd {
    return func() tea.Msg {
        roles, err := client.GetEligibleRoles()
        if err != nil {
            return errMsg{err}
        }
        return rolesLoadedMsg{roles}
    }
}

// Define result message types in the same package:
type rolesLoadedMsg struct{ roles []azure.Role }
type errMsg struct{ err error }
```

### Screen-to-screen navigation via messages

```go
// Done/cancel messages in sub-packages signal the root model:
type DoneMsg struct{}
type CancelMsg struct{}

// Root model switches screen on receipt:
case activate.WizardDoneMsg:
    m.screen = ScreenDashboard
    return m, m.dashboardModel.Init()
```

### Huh v2 forms inside a model

```go
type OptionsModel struct {
    form *huh.Form
}

func (m OptionsModel) Update(msg tea.Msg) (OptionsModel, tea.Cmd) {
    form, cmd := m.form.Update(msg)
    m.form = form.(*huh.Form)
    if m.form.State == huh.StateCompleted {
        // read values and advance
    }
    return m, cmd
}
```

---

## Lip Gloss v2 theme

```go
// Constructing a theme (NewTheme in internal/tui/styles/theme.go):
func NewTheme(isDark bool) Theme {
    ld := lipgloss.LightDark(isDark)   // returns func(light, dark color.Color) color.Color

    accent := ld(lipgloss.Color("#0066cc"), lipgloss.Color("#00ccff"))
    // accent is color.Color — NOT lipgloss.Style

    return Theme{
        Accent: accent,
        Header: lipgloss.NewStyle().Foreground(accent).Bold(true),
        // ...
    }
}

// Rendering with a theme color:
lipgloss.NewStyle().Foreground(theme.Accent).Render("text")  // correct
theme.Accent.Render("text")                                   // WRONG — color.Color has no Render
```

---

## Azure client patterns

### Initialisation

```go
// In app.go — Connect() wires everything:
client, err := azure.NewClient(ctx)
// NewClient chains: AzureCLI → AzurePowerShell → DeviceCode (if PIM_ALLOW_DEVICE_LOGIN=1)

// Get principal ID once at startup:
user, err := client.GetCurrentUser()
principalID := user.ID
// Store principalID in AppModel — do not call GetCurrentUser again per render
```

### Passing API functions as dependencies (avoid coupling sub-packages to azure.Client)

```go
// In activate/wizard.go — Deps struct holds func fields:
type Deps struct {
    PrincipalID string
    LoadRoles   func() ([]azure.Role, error)
    LoadSubs    func(mgID string) ([]azure.Subscription, error)
    Activate    func(role azure.Role, pid, just string, mins int, scope string) error
    // ...
}

// Root model wires them with closures:
deps := activate.Deps{
    LoadRoles: func() ([]azure.Role, error) {
        return client.GetEligibleRoles()
    },
    Activate: func(role azure.Role, pid, just string, mins int, scope string) error {
        _, err := client.ActivateRole(role, pid, just, mins, scope)
        return err
    },
}
```

### Error handling for HTTP responses

```go
// errorFromResponse (in internal/azure/client.go) parses Azure error envelopes:
// {"error":{"code":"...","message":"..."}}
// Returns: "HTTP 403: AuthorizationFailed - ..."

// Sentinel errors (in internal/azure/errors.go):
azure.ErrNoCredential  // no credential sources available
azure.ErrNotEligible   // role not in eligible list
```

---

## State / config patterns

```go
// Open store (uses ~/.config/pim/ by default):
store, err := state.New("")

// Read config (hand-editable):
dur := store.Config.Preferences.DefaultDuration  // e.g. "1h"
favs := store.Config.Favorites

// Read/write state (auto-managed):
store.AddRecentJustification("break-glass access")
store.SaveState()

// Favorites by number key:
fav, ok := store.FavoriteByKey(1)  // key 1-9

// Atomic write — store uses tmp-file + rename internally
```

### TOML struct tags

```go
type Favorite struct {
    Label    string `toml:"label"`
    Role     string `toml:"role"`
    Scope    string `toml:"scope"`
    Duration string `toml:"duration"`
    Key      int    `toml:"key"`
}
// Zero-value fields are omitted by BurntSushi/toml automatically for pointers/slices.
// For scalars, use omitzero (Go 1.26) when zero should be omitted.
```

---

## Table-driven tests

```go
func TestParseDuration(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  int
    }{
        {"hours only", "2h", 120},
        {"minutes only", "30m", 30},
        {"combined", "1h30m", 90},
        {"empty fallback", "", 60},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got := parseDurationMinutes(tc.input)
            if got != tc.want {
                t.Errorf("parseDurationMinutes(%q) = %d, want %d", tc.input, got, tc.want)
            }
        })
    }
}
```

### Testing TUI Update()

```go
func TestModelUpdate_Quit(t *testing.T) {
    m := New(fakeDeps())
    _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyRune, Rune: 'q'})
    if cmd == nil {
        t.Fatal("expected quit command, got nil")
    }
}
```

---

## Completion scripts

Shell completions live in `internal/completion/completion.go`. Generated at release time by
`.goreleaser.yaml` — do not invoke completion logic in normal TUI flow.

Flag names are the source of truth: keep `internal/app/config.go` and completion scripts in sync.
