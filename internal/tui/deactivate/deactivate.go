package deactivate

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/tui/components"
	"github.com/jeircul/pim/internal/tui/styles"
)

// DoneMsg is sent when deactivation completes (success or partial failure).
type DoneMsg struct{ Errors []error }

// CancelMsg is sent when the user cancels.
type CancelMsg struct{}

type deactStep int

const (
	stepSelect deactStep = iota
	stepConfirm
)

type deactResultMsg struct {
	idx int
	err error
}

type itemState int

const (
	itemWaiting itemState = iota
	itemRunning
	itemDone
	itemFailed
)

type deactItem struct {
	assignment azure.ActiveAssignment
	selected   bool
	state      itemState
	err        error
}

// Model is the deactivation screen.
type Model struct {
	theme       styles.Theme
	keys        styles.KeyMap
	spinner     components.Spinner
	items       []deactItem
	cursor      int
	step        deactStep
	loading     bool
	err         error
	width       int
	height      int
	loadFunc    func() ([]azure.ActiveAssignment, error)
	deactivate  func(assignment azure.ActiveAssignment, principalID string) error
	principalID string
}

// New creates a deactivation Model.
func New(
	theme styles.Theme,
	keys styles.KeyMap,
	principalID string,
	loadFunc func() ([]azure.ActiveAssignment, error),
	deactivateFunc func(azure.ActiveAssignment, string) error,
) Model {
	return Model{
		theme:       theme,
		keys:        keys,
		spinner:     components.NewSpinner(theme.Active),
		loading:     true,
		loadFunc:    loadFunc,
		deactivate:  deactivateFunc,
		principalID: principalID,
	}
}

type loadMsg struct {
	active []azure.ActiveAssignment
	err    error
}

// Init starts loading active assignments.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Init(),
		func() tea.Msg {
			active, err := m.loadFunc()
			return loadMsg{active: active, err: err}
		},
	)
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case loadMsg:
		m.loading = false
		m.err = msg.err
		m.items = make([]deactItem, len(msg.active))
		for i, a := range msg.active {
			m.items[i] = deactItem{assignment: a}
		}
		m.cursor = 0

	case deactResultMsg:
		m.items[msg.idx].state = itemDone
		if msg.err != nil {
			m.items[msg.idx].state = itemFailed
			m.items[msg.idx].err = msg.err
		}
		if m.allDone() {
			errs := m.collectErrors()
			return m, func() tea.Msg { return DoneMsg{Errors: errs} }
		}

	case tea.KeyPressMsg:
		if m.loading {
			break
		}
		switch m.step {
		case stepSelect:
			return m.updateSelect(msg)
		case stepConfirm:
			return m.updateConfirm(msg)
		}

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) updateSelect(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case msg.String() == " ":
		if len(m.items) > 0 {
			m.items[m.cursor].selected = !m.items[m.cursor].selected
		}
	case key.Matches(msg, m.keys.Enter), msg.String() == "right":
		if m.countSelected() > 0 {
			m.step = stepConfirm
		}
	case key.Matches(msg, m.keys.Back), msg.String() == "esc", msg.String() == "q":
		return m, func() tea.Msg { return CancelMsg{} }
	}
	return m, nil
}

func (m Model) updateConfirm(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Enter):
		cmds := make([]tea.Cmd, 0, m.countSelected())
		for i := range m.items {
			if m.items[i].selected {
				m.items[i].state = itemRunning
				cmds = append(cmds, m.runDeactivation(i))
			}
		}
		return m, tea.Batch(append([]tea.Cmd{m.spinner.Init()}, cmds...)...)
	case key.Matches(msg, m.keys.Back), msg.String() == "esc":
		m.step = stepSelect
	case msg.String() == "q":
		return m, func() tea.Msg { return CancelMsg{} }
	}
	return m, nil
}

func (m Model) runDeactivation(i int) tea.Cmd {
	assignment := m.items[i].assignment
	pid := m.principalID
	fn := m.deactivate
	return func() tea.Msg {
		return deactResultMsg{idx: i, err: fn(assignment, pid)}
	}
}

func (m *Model) allDone() bool {
	for _, it := range m.items {
		if it.selected && (it.state == itemWaiting || it.state == itemRunning) {
			return false
		}
	}
	return true
}

func (m *Model) collectErrors() []error {
	var errs []error
	for _, it := range m.items {
		if it.err != nil {
			errs = append(errs, it.err)
		}
	}
	return errs
}

func (m *Model) countSelected() int {
	n := 0
	for _, it := range m.items {
		if it.selected {
			n++
		}
	}
	return n
}

// View renders the deactivation screen.
func (m Model) View() string {
	var sb strings.Builder

	if m.loading {
		sb.WriteString(m.spinner.View() + " loading active roles…\n")
		return sb.String()
	}
	if m.err != nil {
		sb.WriteString(m.theme.Subtle.Render("error: "+m.err.Error()) + "\n")
		return sb.String()
	}
	if len(m.items) == 0 {
		sb.WriteString(m.theme.Subtle.Render("no active elevations to deactivate") + "\n")
		hints := []key.Binding{m.keys.Back, m.keys.Quit}
		sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints, ""))
		return sb.String()
	}

	switch m.step {
	case stepSelect:
		sb.WriteString(m.theme.Title.Render("Select roles to deactivate:") + "\n\n")
		for i, it := range m.items {
			cur := "  "
			if i == m.cursor {
				cur = m.theme.TableRowSelected.Render("▸") + " "
			}
			check := "☐ "
			if it.selected {
				check = m.theme.Active.Render("☑") + " "
			}
			scope := azure.DefaultScopeDisplay(it.assignment.Scope, it.assignment.ScopeDisplay)
			expiry := m.theme.Subtle.Render(it.assignment.ExpiryDisplay())
			line := cur + check + padRight(it.assignment.RoleName, 30) + " " +
				m.theme.Subtle.Render(padRight(scope, 28)) + " " + expiry
			sb.WriteString(line + "\n")
		}
		n := m.countSelected()
		if n > 0 {
			sb.WriteString("\n" + m.theme.Subtle.Render(fmt.Sprintf("%d selected", n)) + "\n")
		}
		hints := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Enter, m.keys.Back, m.keys.Quit}
		sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints,
			"space toggle  → confirm"))

	case stepConfirm:
		sb.WriteString(m.theme.Title.Render(fmt.Sprintf("Deactivating %d role(s):", m.countSelected())) + "\n\n")
		for _, it := range m.items {
			if !it.selected {
				continue
			}
			scope := azure.DefaultScopeDisplay(it.assignment.Scope, it.assignment.ScopeDisplay)
			var stateStr string
			switch it.state {
			case itemRunning:
				stateStr = m.spinner.View() + " pending"
			case itemDone:
				stateStr = m.theme.Active.Render("✓ done")
			case itemFailed:
				stateStr = m.theme.Subtle.Foreground(m.theme.Danger).Render("✗ failed")
			default:
				stateStr = m.theme.Subtle.Render("waiting")
			}
			sb.WriteString(fmt.Sprintf("  %-30s %-28s %s\n",
				it.assignment.RoleName, scope, stateStr))
		}
		sb.WriteString("\n")
		hints := []key.Binding{m.keys.Enter, m.keys.Back, m.keys.Quit}
		sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints,
			"enter confirm  ← back  q cancel"))
	}

	return sb.String()
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
