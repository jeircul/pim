package status

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/tui/components"
	"github.com/jeircul/pim/internal/tui/styles"
)

// CancelMsg is sent when the user navigates back from the status screen.
type CancelMsg struct{}

// LoadMsg carries the result of the status data fetch.
type LoadMsg struct {
	Active   []azure.ActiveAssignment
	Eligible []azure.Role
	Err      error
}

// Model is the status screen.
type Model struct {
	theme    styles.Theme
	keys     styles.KeyMap
	spinner  components.Spinner
	active   []azure.ActiveAssignment
	eligible []azure.Role
	loading  bool
	err      error
	cursor   int
	width    int
	height   int
	loadFunc func() ([]azure.ActiveAssignment, []azure.Role, error)
}

// New creates a status Model.
func New(
	theme styles.Theme,
	keys styles.KeyMap,
	loadFunc func() ([]azure.ActiveAssignment, []azure.Role, error),
) Model {
	return Model{
		theme:    theme,
		keys:     keys,
		spinner:  components.NewSpinner(theme.Active),
		loading:  true,
		loadFunc: loadFunc,
	}
}

// Init starts the spinner and triggers data load.
func (m Model) Init() tea.Cmd {
	if m.loadFunc == nil {
		return nil
	}
	return tea.Batch(
		m.spinner.Init(),
		func() tea.Msg {
			active, eligible, err := m.loadFunc()
			return LoadMsg{Active: active, Eligible: eligible, Err: err}
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
		m.eligible = msg.Eligible
		m.cursor = 0

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Back), msg.String() == "esc", msg.String() == "q":
			return m, func() tea.Msg { return CancelMsg{} }
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			maxCursor := len(m.active) + len(m.eligible) - 1
			if m.cursor < maxCursor {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Refresh):
			m.loading = true
			return m, tea.Batch(
				m.spinner.Init(),
				func() tea.Msg {
					active, eligible, err := m.loadFunc()
					return LoadMsg{Active: active, Eligible: eligible, Err: err}
				},
			)
		}

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the status screen.
func (m Model) View() string {
	var sb strings.Builder

	if m.loading {
		sb.WriteString(m.spinner.View() + " loading…\n")
		return sb.String()
	}

	if m.err != nil {
		sb.WriteString(m.theme.Subtle.Render("error: "+m.err.Error()) + "\n")
		return sb.String()
	}

	row := 0

	sb.WriteString(m.theme.Title.Render("Active") + "\n")
	if len(m.active) == 0 {
		sb.WriteString(m.theme.Subtle.Render("  none") + "\n")
	} else {
		for _, a := range m.active {
			selected := row == m.cursor
			scope := azure.DefaultScopeDisplay(a.Scope, a.ScopeDisplay)
			line := fmt.Sprintf("  %-40s %-30s %s", a.RoleName, scope, m.theme.Active.Render(a.ExpiryDisplay()))
			if selected {
				line = m.theme.TableRowSelected.Render(line)
			}
			sb.WriteString(line + "\n")
			row++
		}
	}

	sb.WriteString("\n")
	sb.WriteString(m.theme.Title.Render("Eligible") + "\n")
	if len(m.eligible) == 0 {
		sb.WriteString(m.theme.Subtle.Render("  none") + "\n")
	} else {
		for _, r := range m.eligible {
			selected := row == m.cursor
			scope := azure.DefaultScopeDisplay(r.Scope, r.ScopeDisplay)
			line := fmt.Sprintf("  %-40s %s", r.RoleName, scope)
			if selected {
				line = m.theme.TableRowSelected.Render(line)
			}
			sb.WriteString(line + "\n")
			row++
		}
	}

	sb.WriteString("\n")
	hints := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Refresh, m.keys.Back, m.keys.Quit}
	sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints, ""))

	return sb.String()
}

// HeaderTitle returns the title for the header bar.
func (m Model) HeaderTitle() string { return "status" }
