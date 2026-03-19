package dashboard

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/state"
	"github.com/jeircul/pim/internal/tui/components"
	"github.com/jeircul/pim/internal/tui/styles"
)

// LoadMsg is sent when data loading completes.
type LoadMsg struct {
	Active []azure.ActiveAssignment
	Err    error
}

// TickMsg is sent every second to refresh timers.
type TickMsg struct{}

// ActivateMsg is sent when the user triggers an activation (from favorite key or 'a').
type ActivateMsg struct {
	Favorite *state.Favorite // nil = open full wizard
}

// Model is the dashboard screen model.
type Model struct {
	theme    styles.Theme
	keys     styles.KeyMap
	store    *state.Store
	spinner  components.Spinner
	active   []azure.ActiveAssignment
	loading  bool
	err      error
	width    int
	height   int
	cursor   int
	loadFunc func() ([]azure.ActiveAssignment, error)
}

// New creates a new dashboard Model.
func New(theme styles.Theme, keys styles.KeyMap, store *state.Store, loadFunc func() ([]azure.ActiveAssignment, error)) Model {
	return Model{
		theme:    theme,
		keys:     keys,
		store:    store,
		spinner:  components.NewSpinner(theme.Active),
		loading:  true,
		loadFunc: loadFunc,
	}
}

// Init starts the spinner and triggers the initial data load.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Init(),
		func() tea.Msg {
			active, err := m.loadFunc()
			return LoadMsg{Active: active, Err: err}
		},
	)
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case LoadMsg:
		m.loading = false
		m.err = msg.Err
		m.active = msg.Active
		m.cursor = 0
		return m, tickCmd()

	case TickMsg:
		return m, tickCmd()

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.active)-1 {
				m.cursor++
			}

		case key.Matches(msg, m.keys.Activate):
			return m, func() tea.Msg { return ActivateMsg{} }

		case key.Matches(msg, m.keys.Refresh):
			m.loading = true
			return m, tea.Batch(
				m.spinner.Init(),
				func() tea.Msg {
					active, err := m.loadFunc()
					return LoadMsg{Active: active, Err: err}
				},
			)

		default:
			// 1-9 favorite shortcuts
			s := msg.String()
			if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
				n := int(s[0] - '0')
				if fav, ok := m.store.FavoriteByKey(n); ok {
					return m, func() tea.Msg { return ActivateMsg{Favorite: &fav} }
				}
			}
		}

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the dashboard screen.
func (m Model) View() string {
	var sb strings.Builder

	if m.loading {
		sb.WriteString(m.spinner.View() + " loading…\n")
		return sb.String()
	}

	if m.err != nil {
		sb.WriteString(m.theme.Subtle.Render("error: "+m.err.Error()) + "\n\n")
		hints := []key.Binding{m.keys.Activate, m.keys.Refresh, m.keys.Quit}
		sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints, ""))
		return sb.String()
	}

	sb.WriteString(m.theme.Title.Render("Active elevations") + "\n")

	if len(m.active) == 0 {
		sb.WriteString(m.theme.Subtle.Render("  no active elevations") + "\n")
	} else {
		for i, a := range m.active {
			row := renderActiveRow(m.theme, a, i == m.cursor)
			sb.WriteString(row + "\n")
		}
	}

	sb.WriteString("\n")

	favs := m.store.Config.Favorites
	if len(favs) > 0 {
		sb.WriteString(m.theme.Title.Render("Favorites") + "\n")
		for _, f := range favs {
			line := ""
			if f.Key >= 1 && f.Key <= 9 {
				line += m.theme.Tag.Render(fmt.Sprintf("[%d]", f.Key)) + " "
			} else {
				line += "    "
			}
			line += m.theme.Bold.Render(f.Label)
			line += m.theme.Subtle.Render(fmt.Sprintf("  %s  %s  %s", f.Role, f.Scope, f.Duration))
			sb.WriteString(line + "\n")
		}
		sb.WriteString("\n")
	}

	hints := []key.Binding{
		m.keys.Activate,
		m.keys.Status,
		m.keys.Refresh,
		m.keys.Quit,
	}
	sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints, ""))

	return sb.String()
}

// HeaderTitle returns the title for the header bar.
func (m Model) HeaderTitle() string { return "dashboard" }

func renderActiveRow(theme styles.Theme, a azure.ActiveAssignment, selected bool) string {
	expiry := a.ExpiryDisplay()

	var expiryStyle lipgloss.Style
	if a.TimeRemaining() < 15*time.Minute && !a.IsPermanent() {
		expiryStyle = theme.Subtle.Foreground(theme.Danger)
	} else {
		expiryStyle = theme.Active
	}

	name := a.RoleName
	scope := azure.DefaultScopeDisplay(a.Scope, a.ScopeDisplay)

	row := fmt.Sprintf("  %-40s %-30s %s", name, scope, expiryStyle.Render(expiry))
	if selected {
		return theme.TableRowSelected.Render(row)
	}
	return theme.TableRow.Render(row)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(_ time.Time) tea.Msg {
		return TickMsg{}
	})
}
