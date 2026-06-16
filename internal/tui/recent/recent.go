package recent

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/jeircul/pim/internal/state"
	"github.com/jeircul/pim/internal/tui/components"
	"github.com/jeircul/pim/internal/tui/styles"
)

// ActivateMsg is sent when the user selects a recent activation to re-run.
type ActivateMsg struct{ Favorite state.Favorite }

// DoneMsg is sent when the user exits the recent screen.
type DoneMsg struct{}

// Model is the recent activations screen.
type Model struct {
	theme  styles.Theme
	keys   styles.KeyMap
	store  *state.Store
	cursor int
	width  int
	height int
}

// New creates a recent activations Model.
func New(theme styles.Theme, keys styles.KeyMap, store *state.Store) Model {
	return Model{theme: theme, keys: keys, store: store}
}

// Init is a no-op.
func (m Model) Init() tea.Cmd { return nil }

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyPressMsg:
		acts := m.store.RecentActivations()
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(acts)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keys.Enter):
			if len(acts) > 0 {
				a := acts[m.cursor]
				fav := state.Favorite{
					Role:          a.Role,
					Scope:         a.Scope,
					Duration:      a.Duration,
					Justification: a.Justification,
				}
				return m, func() tea.Msg { return ActivateMsg{Favorite: fav} }
			}
		case key.Matches(msg, m.keys.Back), key.Matches(msg, m.keys.Quit):
			return m, func() tea.Msg { return DoneMsg{} }
		}
	}
	return m, nil
}

// View renders the recent activations screen.
func (m Model) View() string {
	var sb strings.Builder

	sb.WriteString(m.theme.Title.Render("Recent Activations") + "\n\n")

	acts := m.store.RecentActivations()
	if len(acts) == 0 {
		sb.WriteString(m.theme.Subtle.Render("No recent activations.") + "\n")
	} else {
		for i, a := range acts {
			cursor := "  "
			if i == m.cursor {
				cursor = m.theme.Tag.Render("▶ ")
			}
			age := formatAge(a.ActivatedAt)
			scopeStr := a.ScopeDisplay
			if scopeStr == "" {
				scopeStr = a.Scope
			}
			header := fmt.Sprintf("%s  %s  %s  %s", a.Role, scopeStr, a.Duration, age)
			sb.WriteString(cursor + m.theme.Bold.Render(header) + "\n")
			if a.Justification != "" {
				sb.WriteString("    " + m.theme.Subtle.Render(a.Justification) + "\n")
			}
		}
	}

	sb.WriteString("\n")
	hints := []key.Binding{m.keys.Enter, m.keys.Up, m.keys.Down, m.keys.Back}
	sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints, ""))
	return sb.String()
}

// formatAge returns a human-readable relative time string.
func formatAge(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
