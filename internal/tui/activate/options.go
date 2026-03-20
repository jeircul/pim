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
	{"1h30", 90},
	{"2h", 120},
	{"2h30", 150},
	{"3h", 180},
	{"3h30", 210},
	{"4h", 240},
	{"4h30", 270},
	{"5h", 300},
	{"5h30", 330},
	{"6h", 360},
	{"6h30", 390},
	{"7h", 420},
	{"7h30", 450},
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
		focusJust:     false,
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
		if m.focusJust {
			// Text input mode: all keys type into the justification field except
			// the special keys below. Always return a consumed signal so parent
			// models (wizard, app) know not to intercept this key.
			switch msg.String() {
			case "tab":
				m.focusJust = false
				m.recentCursor = -1
			case "enter":
				if m.justification != "" {
					mins := durationChoices[m.durationIdx].minutes
					j := m.justification
					return m, func() tea.Msg { return OptionsDoneMsg{Minutes: mins, Justification: j} }
				}
			case "up":
				if len(m.recentJusts) > 0 && m.recentCursor < len(m.recentJusts)-1 {
					m.recentCursor++
					m.justification = m.recentJusts[m.recentCursor]
				}
			case "down":
				if m.recentCursor >= 0 {
					m.recentCursor--
					if m.recentCursor < 0 {
						m.justification = ""
					} else {
						m.justification = m.recentJusts[m.recentCursor]
					}
				}
			case "backspace":
				if len(m.justification) > 0 {
					m.justification = m.justification[:len(m.justification)-1]
					m.recentCursor = -1
				}
			default:
				if msg.Text != "" {
					m.justification += msg.Text
					m.recentCursor = -1
				}
			}
			// Return a no-op cmd to signal this key was consumed so wizard/app
			// do not also handle it (e.g. q for back, ? for help).
			return m, func() tea.Msg { return nil }
		}

		// Duration grid mode (focusJust == false).
		switch msg.String() {
		case "tab":
			m.focusJust = true
			m.recentCursor = -1
		case "enter":
			m.focusJust = true
		case "right", "l":
			if m.durationIdx < len(durationChoices)-1 {
				m.durationIdx++
			}
		case "left", "h":
			if m.durationIdx > 0 {
				m.durationIdx--
			}
		case "up":
			if len(m.recentJusts) > 0 {
				m.focusJust = true
				if m.recentCursor < len(m.recentJusts)-1 {
					m.recentCursor++
					m.justification = m.recentJusts[m.recentCursor]
				}
			}
		case "down":
			if m.recentCursor >= 0 {
				m.focusJust = true
				m.recentCursor--
				if m.recentCursor < 0 {
					m.justification = ""
				} else {
					m.justification = m.recentJusts[m.recentCursor]
				}
			}
		}
	}

	return m, nil
}

// View renders the options step.
func (m Options) View() string {
	var sb strings.Builder

	// Duration rows: split choices evenly into two rows
	sb.WriteString(m.theme.Title.Render("Duration:") + "\n")
	perRow := (len(durationChoices) + 1) / 2
	for row := 0; row < 2; row++ {
		start := row * perRow
		end := min(start+perRow, len(durationChoices))
		for i := start; i < end; i++ {
			if i > start {
				sb.WriteString("  ")
			}
			d := durationChoices[i]
			if i == m.durationIdx {
				sb.WriteString(m.theme.TableRowSelected.Render("● " + d.label))
			} else {
				sb.WriteString(m.theme.Subtle.Render("○ " + d.label))
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

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
