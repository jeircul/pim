package activate

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/state"
	"github.com/jeircul/pim/internal/tui/styles"
)

// WizardDoneMsg is sent to the root model when the wizard has finished.
type WizardDoneMsg struct {
	Errors []error
}

// WizardCancelMsg is sent when the user cancels the wizard.
type WizardCancelMsg struct{}

type wizardStep int

const (
	stepRoleList wizardStep = iota
	stepScopeTree
	stepOptions
	stepConfirm
)

// Deps groups external dependencies injected into the Wizard.
type Deps struct {
	PrincipalID string
	Active      []azure.ActiveAssignment
	RoleFilter  []string // from --role flags
	ScopeFilter []string // from --scope flags
	TimeStr     string   // from --time flag
	Justific    string   // from --justification flag
	AutoSubmit  bool     // from --yes flag
	Store       *state.Store
	LoadRoles   func() ([]azure.Role, error)
	LoadSubs    func(mgID string) ([]azure.Subscription, error)
	LoadRGs     func(subID string) ([]azure.ResourceGroup, error)
	Activate    func(role azure.Role, principalID, justification string, minutes int, targetScope string) error
}

// Wizard is the root model for the 4-step activation flow.
type Wizard struct {
	theme  styles.Theme
	keys   styles.KeyMap
	deps   Deps
	step   wizardStep
	width  int
	height int

	// step models
	roleList  RoleList
	scopeTree ScopeTree
	options   Options
	confirm   Confirm

	// accumulation across steps
	selectedRoles []azure.Role
	scopeQueue    []azure.Role // MG-scoped roles still needing scope selection
	items         []activationItem
}

// New creates a Wizard. Call Init() to start.
func New(theme styles.Theme, keys styles.KeyMap, deps Deps) Wizard {
	w := Wizard{theme: theme, keys: keys, deps: deps}
	w.roleList = NewRoleList(theme, keys, deps.Active, deps.RoleFilter, deps.LoadRoles)
	return w
}

// Init starts the first step (or fast-forwards if flags allow).
func (w Wizard) Init() tea.Cmd {
	return w.roleList.Init()
}

// Update routes messages to the active step.
func (w Wizard) Update(msg tea.Msg) (Wizard, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width
		w.height = msg.Height
		// propagate to active step
		w.roleList.width = msg.Width
		w.roleList.height = msg.Height
		w.scopeTree.width = msg.Width
		w.scopeTree.height = msg.Height
		w.options.width = msg.Width
		w.options.height = msg.Height
		w.confirm.width = msg.Width
		w.confirm.height = msg.Height

	case RoleListDoneMsg:
		w.selectedRoles = msg.Selected
		return w.advanceFromRoles()

	case ScopeTreeDoneMsg:
		w.items = append(w.items, activationItem{
			role:        msg.Role,
			targetScope: msg.TargetScope,
		})
		w.scopeQueue = w.scopeQueue[1:]
		if len(w.scopeQueue) > 0 {
			return w.startNextScopeTree()
		}
		return w.startOptions()

	case OptionsDoneMsg:
		w.deps.Store.AddRecentJustification(msg.Justification)
		_ = w.deps.Store.SaveState()
		return w.startConfirm(msg.Minutes, msg.Justification)

	case ConfirmDoneMsg:
		done := WizardDoneMsg{Errors: msg.Errors}
		return w, func() tea.Msg { return done }

	case tea.KeyPressMsg:
		// Global back/cancel
		if msg.String() == "esc" || msg.String() == "q" {
			switch w.step {
			case stepRoleList:
				return w, func() tea.Msg { return WizardCancelMsg{} }
			case stepScopeTree:
				w.step = stepRoleList
				return w, w.roleList.Init()
			case stepOptions:
				w.step = stepRoleList
				return w, w.roleList.Init()
			case stepConfirm:
				w.step = stepOptions
				return w, w.options.Init()
			}
		}
	}

	// Delegate to active step.
	var cmd tea.Cmd
	switch w.step {
	case stepRoleList:
		w.roleList, cmd = w.roleList.Update(msg)
	case stepScopeTree:
		w.scopeTree, cmd = w.scopeTree.Update(msg)
	case stepOptions:
		w.options, cmd = w.options.Update(msg)
	case stepConfirm:
		w.confirm, cmd = w.confirm.Update(msg)
	}
	return w, cmd
}

// View renders the current step with a wizard header.
func (w Wizard) View() string {
	var sb strings.Builder
	sb.WriteString(w.renderStepIndicator() + "\n\n")

	switch w.step {
	case stepRoleList:
		sb.WriteString(w.roleList.View())
	case stepScopeTree:
		sb.WriteString(w.scopeTree.View())
	case stepOptions:
		sb.WriteString(w.options.View())
	case stepConfirm:
		sb.WriteString(w.confirm.View())
	}

	return sb.String()
}

// HeaderTitle returns "activate" for the header bar.
func (w Wizard) HeaderTitle() string { return "activate" }

// advanceFromRoles decides whether to show scope trees or skip straight to options.
func (w Wizard) advanceFromRoles() (Wizard, tea.Cmd) {
	// Partition into MG-scoped (need scope tree) and direct (sub/RG scope).
	var mgRoles []azure.Role
	for _, r := range w.selectedRoles {
		if r.ScopeKind() == azure.ScopeManagementGroup {
			// Check if --scope flag overrides
			if w.scopeOverride(r) != "" {
				w.items = append(w.items, activationItem{role: r, targetScope: w.scopeOverride(r)})
			} else {
				mgRoles = append(mgRoles, r)
			}
		} else {
			w.items = append(w.items, activationItem{role: r})
		}
	}

	if len(mgRoles) > 0 {
		w.scopeQueue = mgRoles
		return w.startNextScopeTree()
	}
	return w.startOptions()
}

func (w Wizard) scopeOverride(r azure.Role) string {
	for _, s := range w.deps.ScopeFilter {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	_ = r
	return ""
}

func (w Wizard) startNextScopeTree() (Wizard, tea.Cmd) {
	role := w.scopeQueue[0]
	w.scopeTree = NewScopeTree(w.theme, w.keys, role, w.deps.LoadSubs, w.deps.LoadRGs)
	w.step = stepScopeTree
	return w, w.scopeTree.Init()
}

func (w Wizard) startOptions() (Wizard, tea.Cmd) {
	defaultMinutes := w.deps.Store.DefaultDurationMinutes()
	if w.deps.TimeStr != "" {
		if m, err := azure.ParseDurationMinutes(w.deps.TimeStr); err == nil {
			defaultMinutes = m
		}
	}
	w.options = NewOptions(
		w.theme, w.keys,
		defaultMinutes,
		w.deps.Store.State.RecentJustifications,
		w.deps.Justific,
	)
	w.step = stepOptions

	// Flag acceleration: if --time and --justification both provided, skip options step.
	if w.deps.TimeStr != "" && w.deps.Justific != "" {
		mins, err := azure.ParseDurationMinutes(w.deps.TimeStr)
		if err == nil {
			return w.startConfirm(mins, w.deps.Justific)
		}
	}

	return w, w.options.Init()
}

func (w Wizard) startConfirm(minutes int, justification string) (Wizard, tea.Cmd) {
	w.confirm = NewConfirm(
		w.theme, w.keys,
		w.items,
		minutes,
		justification,
		w.deps.PrincipalID,
		w.deps.Activate,
	)
	w.step = stepConfirm

	// --yes: auto-submit immediately.
	if w.deps.AutoSubmit {
		return w, func() tea.Msg {
			return tea.KeyPressMsg{} // trigger Enter in Confirm via direct cmd
		}
	}

	return w, w.confirm.Init()
}

func (w Wizard) renderStepIndicator() string {
	steps := []struct {
		label string
		s     wizardStep
	}{
		{"roles", stepRoleList},
		{"scope", stepScopeTree},
		{"options", stepOptions},
		{"confirm", stepConfirm},
	}

	// Count visible steps (scope may be skipped)
	var parts []string
	for i, st := range steps {
		n := i + 1
		label := fmt.Sprintf("%d. %s", n, st.label)
		if st.s == w.step {
			parts = append(parts, w.theme.TableRowSelected.Render(label))
		} else if int(st.s) < int(w.step) {
			parts = append(parts, w.theme.Subtle.Render(label))
		} else {
			parts = append(parts, w.theme.Subtle.Render(label))
		}
	}
	return strings.Join(parts, w.theme.Subtle.Render("  ›  "))
}
