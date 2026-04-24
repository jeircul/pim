package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/jeircul/pim/internal/tui/styles"
)

// RenderHelp renders a full-screen help overlay listing all keybindings.
func RenderHelp(theme styles.Theme, keys styles.KeyMap, width int) string {
	sections := []struct {
		heading  string
		bindings []key.Binding
	}{
		{
			"Navigation",
			[]key.Binding{
				keys.Dashboard,
				keys.Status,
				keys.Activate,
				keys.Deactivate,
				keys.Favorites,
				keys.Back,
				keys.Quit,
			},
		},
		{
			"Lists",
			[]key.Binding{
				keys.Up,
				keys.Down,
				key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle select")),
				keys.Enter,
				key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
				keys.Refresh,
			},
		},
		{
			"Activation wizard",
			[]key.Binding{
				key.NewBinding(key.WithKeys("→"), key.WithHelp("→", "next step")),
				key.NewBinding(key.WithKeys("←"), key.WithHelp("←", "previous step")),
				key.NewBinding(key.WithKeys("1-9"), key.WithHelp("1–9", "launch favorite")),
			},
		},
		{
			"Favorites",
			[]key.Binding{
				key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "new favorite")),
				key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit favorite")),
				key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "delete favorite")),
			},
		},
	}

	var sb strings.Builder
	sb.WriteString(theme.Title.Render("Keyboard shortcuts") + "\n\n")

	for _, sec := range sections {
		sb.WriteString(theme.Bold.Render(sec.heading) + "\n")
		for _, b := range sec.bindings {
			keyStr := lipgloss.NewStyle().Foreground(theme.Accent).Render(padRight(b.Help().Key, 12))
			sb.WriteString("  " + keyStr + "  " + theme.Subtle.Render(b.Help().Desc) + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString(theme.Subtle.Render(strings.Repeat("─", min(width, 48))))
	sb.WriteString("\n" + theme.Subtle.Render("press ? to close"))
	return sb.String()
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
