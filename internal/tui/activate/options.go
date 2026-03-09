package activate

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/jeircul/pim/internal/tui/components"
	"github.com/jeircul/pim/internal/tui/styles"
)

// OptionsDoneMsg is sent when the user confirms duration and justification.
type OptionsDoneMsg struct {
	Minutes       int
	Justification string
}

var durationChoices = []struct {
	label   string
	minutes int
}{
	{"30m", 30},
	{"1h", 60},
	{"2h", 120},
	{"4h", 240},
	{"8h", 480},
}

// Options is Step 3: duration selection and justification entry.
type Options struct {
	theme         styles.Theme
	keys          styles.KeyMap
	durationIdx   int
	justification string
	recentJusts   []string
	recentCursor  int  // -1 = not in recent list
	focusJust     bool // true = justification text field has focus
	width         int
	height        int
}

// NewOptions creates an Options model.
func NewOptions(
	theme styles.Theme,
	keys styles.KeyMap,
	defaultMinutes int,
	recentJusts []string,
	justification string, // pre-filled from --justification flag
) Options {
	// pick closest duration choice
	idx := 1 // default 1h
	for i, d := range durationChoices {
		if d.minutes == defaultMinutes {
			idx = i
			break
		}
	}
	return Options{
		theme:         theme,
		keys:          keys,
		durationIdx:   idx,
		justification: justification,
		recentJusts:   recentJusts,
		recentCursor:  -1,
		focusJust:     justification == "",
	}
}

// Init is a no-op; no async work needed.
func (m Options) Init() tea.Cmd { return nil }

// Update handles messages.
func (m Options) Update(msg tea.Msg) (Options, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyPressMsg:
		switch msg.String() {
		case "tab":
			m.focusJust = !m.focusJust
			m.recentCursor = -1

		case "enter":
			if !m.focusJust {
				// advance from duration row
				m.focusJust = true
				return m, nil
			}
			// submit
			if m.justification != "" {
				mins := durationChoices[m.durationIdx].minutes
				j := m.justification
				return m, func() tea.Msg { return OptionsDoneMsg{Minutes: mins, Justification: j} }
			}

		case "right", "l":
			if !m.focusJust && m.durationIdx < len(durationChoices)-1 {
				m.durationIdx++
			}

		case "left", "h":
			if !m.focusJust && m.durationIdx > 0 {
				m.durationIdx--
			}

		case "up", "k":
			if m.focusJust && len(m.recentJusts) > 0 {
				if m.recentCursor < len(m.recentJusts)-1 {
					m.recentCursor++
					m.justification = m.recentJusts[m.recentCursor]
				}
			} else if !m.focusJust {
				m.focusJust = false
			}

		case "down", "j":
			if m.focusJust && m.recentCursor >= 0 {
				m.recentCursor--
				if m.recentCursor < 0 {
					m.justification = ""
				} else {
					m.justification = m.recentJusts[m.recentCursor]
				}
			}

		case "backspace":
			if m.focusJust && len(m.justification) > 0 {
				m.justification = m.justification[:len(m.justification)-1]
				m.recentCursor = -1
			}

		default:
			if m.focusJust && len(msg.String()) == 1 {
				m.justification += msg.String()
				m.recentCursor = -1
			}
		}
	}

	return m, nil
}

// View renders the options step.
func (m Options) View() string {
	var sb strings.Builder

	// Duration row
	sb.WriteString(m.theme.Title.Render("Duration:") + "  ")
	for i, d := range durationChoices {
		if i > 0 {
			sb.WriteString("  ")
		}
		if i == m.durationIdx {
			sb.WriteString(m.theme.TableRowSelected.Render("● " + d.label))
		} else {
			sb.WriteString(m.theme.Subtle.Render("○ " + d.label))
		}
	}
	sb.WriteString("\n\n")

	// Justification field
	justTitle := m.theme.Title.Render("Justification:")
	if m.focusJust {
		justTitle = m.theme.TableRowSelected.Render("Justification:")
	}
	sb.WriteString(justTitle + "\n")

	cursor := ""
	if m.focusJust {
		cursor = "█"
	}
	sb.WriteString(m.theme.Bold.Render(m.justification) + cursor + "\n\n")

	// Recent justifications
	if len(m.recentJusts) > 0 {
		sb.WriteString(m.theme.Subtle.Render("Recent:") + "\n")
		for i, j := range m.recentJusts {
			prefix := m.theme.Subtle.Render("  ")
			if i == m.recentCursor {
				prefix = m.theme.TableRowSelected.Render("▸ ")
			}
			sb.WriteString(prefix + m.theme.Subtle.Render(j) + "\n")
		}
		sb.WriteString("\n")
	}

	hints := []key.Binding{m.keys.Enter, m.keys.Back, m.keys.Quit}
	sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints,
		"tab switch  ←/→ duration  ↑/↓ recent  enter next"))

	return sb.String()
}
