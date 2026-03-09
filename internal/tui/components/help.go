package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/jeircul/pim/internal/tui/styles"
)

// RenderHelp renders a full-screen help overlay listing all keybindings.
func RenderHelp(theme styles.Theme, width int) string {
	sections := []struct {
		heading  string
		bindings []key.Binding
	}{
		{
			"Navigation",
			[]key.Binding{
				key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "dashboard")),
				key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "status")),
				key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "activate")),
				key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "deactivate")),
				key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "favorites")),
				key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back / dashboard")),
				key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
			},
		},
		{
			"Lists",
			[]key.Binding{
				key.NewBinding(key.WithKeys("↑/k"), key.WithHelp("↑/k", "up")),
				key.NewBinding(key.WithKeys("↓/j"), key.WithHelp("↓/j", "down")),
				key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle select")),
				key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
				key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
				key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
