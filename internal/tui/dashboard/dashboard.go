package dashboard

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/jeircul/pim/internal/state"
	"github.com/jeircul/pim/internal/tui/components"
	"github.com/jeircul/pim/internal/tui/styles"
)

// ActivateMsg is sent when the user triggers an activation (from favorite key or 'a').
type ActivateMsg struct {
	Favorite *state.Favorite // nil = open full wizard
}

var logo = []string{
	"█▀█ █ █▀▄▀█",
	"█▀▀ █ █ ▀ █",
	"▀   ▀ ▀   ▀",
}

// Model is the landing screen model. It loads no data on startup.
type Model struct {
	theme  styles.Theme
	keys   styles.KeyMap
	store  *state.Store
	width  int
	height int
}

// New creates a new landing screen Model.
func New(theme styles.Theme, keys styles.KeyMap, store *state.Store) Model {
	return Model{
		theme: theme,
		keys:  keys,
		store: store,
	}
}

// Init is a no-op — the landing screen loads no data.
func (m Model) Init() tea.Cmd { return nil }

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Activate):
			return m, func() tea.Msg { return ActivateMsg{} }

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
	}

	return m, nil
}

// View renders the landing screen.
func (m Model) View() string {
	var sb strings.Builder

	for _, line := range logo {
		sb.WriteString("  " + m.theme.Title.Render(line) + "\n")
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
		m.keys.Deactivate,
		m.keys.Quit,
	}
	sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints, ""))

	return sb.String()
}

// HeaderTitle returns the title for the header bar.
func (m Model) HeaderTitle() string { return "pim" }
