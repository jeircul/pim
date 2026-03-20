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

// Result holds the outcome of a single activation or deactivation.
type Result struct {
	RoleName string
	Scope    string
	Err      error
}

// ConfirmDoneMsg is sent when all activations have completed (success or partial failure).
type ConfirmDoneMsg struct {
	Results []Result
}

// activationItem tracks one activation's state.
type activationItem struct {
	role        azure.Role
	targetScope string // may differ from role.Scope for MG-scoped
	status      itemStatus
	err         error
}

type itemStatus int

const (
	statusPending itemStatus = iota
	statusRunning
	statusDone
	statusFailed
)

type activationResultMsg struct {
	idx int
	err error
}

// Confirm is Step 4: shows a summary and executes activations.
type Confirm struct {
	theme         styles.Theme
	keys          styles.KeyMap
	spinner       components.Spinner
	items         []activationItem
	minutes       int
	justification string
	principalID   string
	submitted     bool
	width         int
	height        int
	activateFunc  func(role azure.Role, principalID, justification string, minutes int, targetScope string) error
}

// NewConfirm creates a Confirm model.
func NewConfirm(
	theme styles.Theme,
	keys styles.KeyMap,
	items []activationItem,
	minutes int,
	justification string,
	principalID string,
	activateFunc func(azure.Role, string, string, int, string) error,
) Confirm {
	return Confirm{
		theme:         theme,
		keys:          keys,
		spinner:       components.NewSpinner(theme.Active),
		items:         items,
		minutes:       minutes,
		justification: justification,
		principalID:   principalID,
		activateFunc:  activateFunc,
	}
}

// Init is a no-op; submission begins on Enter.
func (m Confirm) Init() tea.Cmd { return nil }

// Update handles messages.
func (m Confirm) Update(msg tea.Msg) (Confirm, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case activationResultMsg:
		m.items[msg.idx].err = msg.err
		if msg.err != nil {
			m.items[msg.idx].status = statusFailed
		} else {
			m.items[msg.idx].status = statusDone
		}
		if m.allDone() {
			results := m.collectResults()
			return m, func() tea.Msg { return ConfirmDoneMsg{Results: results} }
		}

	case tea.KeyPressMsg:
		if m.submitted {
			break
		}
		switch {
		case key.Matches(msg, m.keys.Enter):
			m.submitted = true
			cmds := make([]tea.Cmd, len(m.items))
			for i := range m.items {
				m.items[i].status = statusRunning
				cmds[i] = m.runActivation(i)
			}
			return m, tea.Batch(append([]tea.Cmd{m.spinner.Init()}, cmds...)...)

		case key.Matches(msg, m.keys.Back), msg.String() == "esc":
			// wizard handles Back via message

		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		}

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Confirm) runActivation(i int) tea.Cmd {
	item := m.items[i]
	principalID := m.principalID
	justification := m.justification
	minutes := m.minutes
	fn := m.activateFunc
	return func() tea.Msg {
		err := fn(item.role, principalID, justification, minutes, item.targetScope)
		return activationResultMsg{idx: i, err: err}
	}
}

func (m *Confirm) allDone() bool {
	for _, it := range m.items {
		if it.status == statusPending || it.status == statusRunning {
			return false
		}
	}
	return true
}

func (m *Confirm) collectResults() []Result {
	results := make([]Result, 0, len(m.items))
	for _, it := range m.items {
		scope := it.targetScope
		if scope == "" {
			scope = it.role.Scope
		}
		results = append(results, Result{
			RoleName: it.role.RoleName,
			Scope:    scope,
			Err:      it.err,
		})
	}
	return results
}

// View renders the confirm step.
func (m Confirm) View() string {
	var sb strings.Builder

	dur := azure.FormatDuration(m.minutes)
	sb.WriteString(m.theme.Title.Render(fmt.Sprintf("Activating %s for %s:", pluralf(len(m.items), "role"), dur)) + "\n\n")

	for _, it := range m.items {
		scope := azure.DefaultScopeDisplay(it.targetScope, "")
		if scope == "" {
			scope = azure.DefaultScopeDisplay(it.role.Scope, it.role.ScopeDisplay)
		}

		var statusStr string
		switch it.status {
		case statusRunning:
			statusStr = m.spinner.View() + " pending"
		case statusDone:
			statusStr = m.theme.Active.Render("✓ done")
		case statusFailed:
			msg := "failed"
			if it.err != nil {
				msg = "failed: " + it.err.Error()
			}
			statusStr = m.theme.Subtle.Foreground(m.theme.Danger).Render(msg)
		default:
			statusStr = m.theme.Subtle.Render("waiting")
		}

		line := fmt.Sprintf("  %-30s %-30s %s", it.role.RoleName, scope, statusStr)
		sb.WriteString(line + "\n")
	}

	sb.WriteString("\n")
	sb.WriteString(m.theme.Subtle.Render(fmt.Sprintf("Justification: %q", m.justification)) + "\n\n")

	if !m.submitted {
		hints := []key.Binding{m.keys.Enter, m.keys.Back, m.keys.Quit}
		sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints,
			"enter confirm  ← back  q cancel"))
	}

	return sb.String()
}
