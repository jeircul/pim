# Anti-Patterns Reference — pim

Each entry: what to avoid, why, and the correct alternative.

---

## Go fundamentals

### Pointer to interface

```go
// WRONG — almost never correct:
var r io.Reader = &myReader{}
process(&r)

// CORRECT — interfaces are already reference types:
var r io.Reader = &myReader{}
process(r)
```

### Error: log AND return

```go
// WRONG — double-handles the error:
if err != nil {
    log.Println(err)
    return err
}

// CORRECT — return and let the caller decide:
if err != nil {
    return fmt.Errorf("load roles: %w", err)
}
```

### Error: "failed to" prefix

```go
// WRONG:
return fmt.Errorf("failed to decode user: %w", err)

// CORRECT — use a noun phrase:
return fmt.Errorf("decode user: %w", err)
```

### Silently swallowed errors

```go
// WRONG:
roles, _ := client.GetEligibleRoles()

// CORRECT — handle or propagate:
roles, err := client.GetEligibleRoles()
if err != nil {
    return rolesErrMsg{err}
}
```

### Global mutable state

```go
// WRONG:
var currentUser *azure.User  // package-level mutable

// CORRECT — store in AppModel and pass via deps:
type AppModel struct {
    principalID string
    // ...
}
```

### Anonymous functions longer than 5 lines

```go
// WRONG — hard to read, hard to test:
deps := Deps{
    Activate: func(role azure.Role, pid, just string, mins int, scope string) error {
        if role.RoleName == "" {
            return errors.New("empty role name")
        }
        _, err := client.ActivateRole(role, pid, just, mins, scope)
        if err != nil {
            log.Println(err) // also wrong: log+return
            return err
        }
        return nil
    },
}

// CORRECT — extract to a named function or keep the closure minimal:
deps := Deps{
    Activate: func(role azure.Role, pid, just string, mins int, scope string) error {
        _, err := client.ActivateRole(role, pid, just, mins, scope)
        return err
    },
}
```

### Unnecessary `pkg/` directory

```go
// WRONG — this repo uses internal/ only:
pkg/azpim/client.go

// CORRECT:
internal/azure/client.go
```

---

## Bubble Tea v2

### View() returning string (v1 pattern)

```go
// WRONG — v1 API, does not compile with bubbletea/v2:
func (m AppModel) View() string {
    return "hello"
}

// CORRECT — v2 returns tea.View:
func (m AppModel) View() tea.View {
    return tea.NewView("hello")
}
```

**Note**: sub-models (not the root AppModel) still return `string` from their `View()` and are
composed inside the root's `View()`. Only the root wraps into `tea.View`.

### tea.KeyMsg (v1 key type)

```go
// WRONG — v1 API:
case tea.KeyMsg:
    if msg.Type == tea.KeyRunes { ... }

// CORRECT — v2:
case tea.KeyPressMsg:
    if msg.String() == "q" { ... }
```

### Setting AltScreen via tea.EnterAltScreen command

```go
// WRONG — v1 approach:
func (m Model) Init() tea.Cmd {
    return tea.EnterAltScreen
}

// CORRECT — v2 declarative in View():
func (m AppModel) View() tea.View {
    v := tea.NewView(content)
    v.AltScreen = true
    return v
}
```

### Treating theme.Accent as lipgloss.Style

```go
// WRONG — color.Color has no Render method:
theme.Accent.Render("text")

// CORRECT — wrap in a style:
lipgloss.NewStyle().Foreground(theme.Accent).Render("text")
```

### Rebuilding the theme every render cycle

```go
// WRONG — allocates many Style objects per frame:
func (m Model) View() tea.View {
    theme := styles.NewTheme(m.isDark)
    // ...
}

// CORRECT — rebuild only when BackgroundColorMsg is received:
case tea.BackgroundColorMsg:
    m.theme = styles.NewTheme(msg.IsDark())
    return m, nil
```

---

## CLI / framework

### Using Cobra or urfave/cli

```go
// WRONG — this repo explicitly forbids CLI frameworks:
import "github.com/spf13/cobra"

// CORRECT — hand-rolled flag parsing in internal/app/config.go:
flag.StringVar(&cfg.Justification, "justification", "", "activation justification")
flag.Parse()
```

### Logging libraries

```go
// WRONG:
import "go.uber.org/zap"
logger.Info("roles loaded", zap.Int("count", len(roles)))

// CORRECT — no logging in this repo; surface errors up the call stack.
// For user-facing messages use the TUI statusbar or fmt.Fprintln(os.Stderr, ...) sparingly.
```

---

## Testing

### Using testify

```go
// WRONG — testify is not a dependency:
import "github.com/stretchr/testify/assert"
assert.Equal(t, want, got)

// CORRECT — stdlib only:
if got != want {
    t.Errorf("got %v, want %v", got, want)
}
```

### Non-table-driven tests

```go
// WRONG — harder to extend:
func TestParse(t *testing.T) {
    if parseDurationMinutes("2h") != 120 {
        t.Fail()
    }
    if parseDurationMinutes("30m") != 30 {
        t.Fail()
    }
}

// CORRECT — table-driven (see patterns.md):
tests := []struct{ name, input string; want int }{ ... }
```

### Vague t.Errorf messages

```go
// WRONG — no context:
t.Errorf("unexpected result")

// CORRECT — include input and values:
t.Errorf("parseDurationMinutes(%q) = %d, want %d", input, got, want)
```

---

## Azure client

### Calling GetCurrentUser() per render/request

```go
// WRONG — network call on every activation or render:
func (m Model) Init() tea.Cmd {
    return func() tea.Msg {
        user, err := m.client.GetCurrentUser()
        // ...
    }
}

// CORRECT — called once in tui.New() at startup; principalID stored in AppModel:
user, err := a.Client.GetCurrentUser()
principalID := user.ID  // stored once
```

### Accessing client.User directly

```go
// WRONG — Client has no User field:
m.client.User.ID

// CORRECT:
user, err := m.client.GetCurrentUser()
principalID := user.ID
```

---

## TOML / state

### Using JSON or YAML for persistence

```go
// WRONG — project uses TOML exclusively:
import "encoding/json"
json.Marshal(config)

// CORRECT:
import "github.com/BurntSushi/toml"
toml.NewEncoder(f).Encode(config)
```

### Writing state files without atomic rename

```go
// WRONG — partial writes leave corrupt state:
os.WriteFile(path, data, 0o600)

// CORRECT — tmp file + rename (already implemented in state.Store.write):
store.SaveState()   // use the Store methods; don't write files directly
```
