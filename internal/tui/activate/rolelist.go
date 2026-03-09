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

// RoleList is Step 1 of the activation wizard: filterable multi-select role list.
type RoleList struct {
	theme     styles.Theme
	keys      styles.KeyMap
	spinner   components.Spinner
	roles     []azure.Role    // full unfiltered list
	visible   []int           // indices into roles matching filter
	selected  map[int]bool    // set of indices (into roles) that are checked
	active    map[string]bool // role definition IDs that are currently active
	filter    string
	filtering bool
	cursor    int
	loading   bool
	err       error
	width     int
	height    int
	loadFunc  func() ([]azure.Role, error)
	// roleFilter pre-selects roles matching these names (from --role flags)
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
		selected:   make(map[int]bool),
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
		m.autoSelect()
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
		case msg.String() == " ":
			if len(m.visible) > 0 {
				ri := m.visible[m.cursor]
				m.selected[ri] = !m.selected[ri]
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
			selected := m.collectSelected()
			if len(selected) > 0 {
				return m, func() tea.Msg { return RoleListDoneMsg{Selected: selected} }
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

// autoSelect pre-selects roles matching the roleFilter flag values.
func (m *RoleList) autoSelect() {
	if len(m.roleFilter) == 0 {
		return
	}
	for i, r := range m.roles {
		for _, f := range m.roleFilter {
			if strings.EqualFold(r.RoleName, f) {
				m.selected[i] = true
				break
			}
		}
	}
}

func (m *RoleList) collectSelected() []azure.Role {
	var out []azure.Role
	for i, r := range m.roles {
		if m.selected[i] {
			out = append(out, r)
		}
	}
	return out
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

	sb.WriteString(m.theme.Title.Render("Select roles to activate:") + "\n")

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
		cursor := "  "
		if pos == m.cursor {
			cursor = m.theme.TableRowSelected.Render("▸") + " "
		}
		check := "☐ "
		if m.selected[ri] {
			check = m.theme.Active.Render("☑") + " "
		}
		scope := azure.DefaultScopeDisplay(r.Scope, r.ScopeDisplay)
		line := cursor + check + padRight(r.RoleName, 30) + " " + m.theme.Subtle.Render(scope)
		if m.active[r.RoleDefinitionID] {
			line += " " + m.theme.Subtle.Render("(active)")
		}
		sb.WriteString(line + "\n")
	}

	count := 0
	for _, v := range m.selected {
		if v {
			count++
		}
	}
	sb.WriteString("\n")
	if count > 0 {
		sb.WriteString(m.theme.Subtle.Render(pluralf(count, "selected")) + "\n")
	}

	hints := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Enter, m.keys.Back, m.keys.Quit}
	sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints,
		"space toggle  / filter  → next"))

	return sb.String()
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func pluralf(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
