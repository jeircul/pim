package activate

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/tui/components"
	"github.com/jeircul/pim/internal/tui/styles"
)

// RoleListDoneMsg is sent when the user confirms their role selection.
type RoleListDoneMsg struct {
	Selected []azure.Role
}

// RoleList is Step 1 of the activation wizard: filterable single-select role list.
type RoleList struct {
	theme     styles.Theme
	keys      styles.KeyMap
	spinner   components.Spinner
	roles     []azure.Role    // full unfiltered list
	visible   []int           // indices into roles matching filter
	active    map[string]bool // role definition IDs that are currently active
	filter    string
	filtering bool
	cursor    int
	loading   bool
	err       error
	width     int
	height    int
	loadFunc  func() ([]azure.Role, error)
	// roleFilter auto-advances when a single --role flag match is found
	roleFilter []string
}

// NewRoleList creates a RoleList model.
func NewRoleList(
	theme styles.Theme,
	keys styles.KeyMap,
	active []azure.ActiveAssignment,
	roleFilter []string,
	loadFunc func() ([]azure.Role, error),
) RoleList {
	activeIDs := make(map[string]bool, len(active))
	for _, a := range active {
		activeIDs[a.RoleDefinitionID] = true
	}
	return RoleList{
		theme:      theme,
		keys:       keys,
		spinner:    components.NewSpinner(theme.Active),
		active:     activeIDs,
		loading:    true,
		loadFunc:   loadFunc,
		roleFilter: roleFilter,
	}
}

// Init starts the spinner and loads eligible roles.
func (m RoleList) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Init(),
		func() tea.Msg {
			roles, err := m.loadFunc()
			return roleListLoadMsg{roles: roles, err: err}
		},
	)
}

type roleListLoadMsg struct {
	roles []azure.Role
	err   error
}

// Update handles messages.
func (m RoleList) Update(msg tea.Msg) (RoleList, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case roleListLoadMsg:
		m.loading = false
		m.err = msg.err
		m.roles = msg.roles
		m.applyFilter()
		// Auto-advance: if --role flag matches exactly one visible role, select it.
		if cmd := m.autoAdvance(); cmd != nil {
			return m, cmd
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.loading {
			break
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		switch {
		case key.Matches(msg, m.keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down):
			if m.cursor < len(m.visible)-1 {
				m.cursor++
			}
		case msg.String() == "/":
			m.filtering = true
		case msg.String() == "esc":
			if m.filter != "" {
				m.filter = ""
				m.filtering = false
				m.applyFilter()
				m.cursor = 0
			}
		case key.Matches(msg, m.keys.Enter), msg.String() == "right", msg.String() == "l":
			if len(m.visible) > 0 {
				ri := m.visible[m.cursor]
				r := m.roles[ri]
				return m, func() tea.Msg { return RoleListDoneMsg{Selected: []azure.Role{r}} }
			}
		}

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m RoleList) updateFilter(msg tea.KeyPressMsg) (RoleList, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		m.filtering = false
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.applyFilter()
			m.cursor = 0
		}
	case "space":
		m.filter += " "
		m.applyFilter()
		m.cursor = 0
	default:
		if r := msg.String(); len(r) == 1 {
			m.filter += r
			m.applyFilter()
			m.cursor = 0
		}
	}
	return m, nil
}

// applyFilter rebuilds the visible index list based on current filter.
func (m *RoleList) applyFilter() {
	m.visible = m.visible[:0]
	lower := strings.ToLower(m.filter)
	for i, r := range m.roles {
		if lower == "" || strings.Contains(strings.ToLower(r.RoleName), lower) ||
			strings.Contains(strings.ToLower(r.ScopeDisplay), lower) {
			m.visible = append(m.visible, i)
		}
	}
	if m.cursor >= len(m.visible) {
		m.cursor = max(0, len(m.visible)-1)
	}
}

// autoAdvance returns a cmd that immediately selects a role when exactly one
// --role flag match is found in the visible list, skipping manual selection.
func (m *RoleList) autoAdvance() tea.Cmd {
	if len(m.roleFilter) == 0 {
		return nil
	}
	var matches []azure.Role
	for _, ri := range m.visible {
		r := m.roles[ri]
		for _, f := range m.roleFilter {
			if strings.EqualFold(r.RoleName, f) {
				matches = append(matches, r)
				break
			}
		}
	}
	if len(matches) != 1 {
		return nil
	}
	r := matches[0]
	return func() tea.Msg { return RoleListDoneMsg{Selected: []azure.Role{r}} }
}

// View renders the role list step.
func (m RoleList) View() string {
	var sb strings.Builder

	if m.loading {
		sb.WriteString(m.spinner.View() + " loading roles…\n")
		return sb.String()
	}
	if m.err != nil {
		sb.WriteString(m.theme.Subtle.Render("error: "+m.err.Error()) + "\n")
		return sb.String()
	}

	sb.WriteString(m.theme.Title.Render("Select role to activate:") + "\n")

	// Filter bar
	if m.filtering {
		sb.WriteString(m.theme.Subtle.Render("/ ") + m.filter + "█\n\n")
	} else if m.filter != "" {
		sb.WriteString(m.theme.Subtle.Render("/ "+m.filter+"  (esc clear)") + "\n\n")
	} else {
		sb.WriteString(m.theme.Subtle.Render("/ filter") + "\n\n")
	}

	for pos, ri := range m.visible {
		r := m.roles[ri]
		scope := azure.DefaultScopeDisplay(r.Scope, r.ScopeDisplay)
		line := fmt.Sprintf("  %-40s %s", r.RoleName, m.theme.Subtle.Render(scope))
		if m.active[r.RoleDefinitionID] {
			line += " " + m.theme.Subtle.Render("(active)")
		}
		if pos == m.cursor {
			line = m.theme.TableRowSelected.Render("▸") + line[1:]
		}
		sb.WriteString(line + "\n")
	}

	sb.WriteString("\n")
	hints := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Enter, m.keys.Back, m.keys.Quit}
	sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints,
		"/ filter  → activate"))

	return sb.String()
}

func pluralf(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
