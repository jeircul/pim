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
	Results []Result
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
	RoleFilter  []string // from --role flags
	ScopeFilter []string // from --scope flags
	TimeStr     string   // from --time flag
	Justific    string   // from --justification flag
	AutoSubmit  bool     // from --yes flag
	Store       *state.Store
	LoadRoles   func() ([]azure.Role, error)
	LoadActive  func() ([]azure.ActiveAssignment, error)
	LoadSubs    func(mgID string) ([]azure.Subscription, error)
	LoadRGs     func(subID string) ([]azure.ResourceGroup, error)
	Activate    func(role azure.Role, principalID, justification string, minutes int, targetScope string) error
}

// autoConfirmMsg triggers auto-submission on the confirm step (--yes flag).
type autoConfirmMsg struct{}

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
	scopeVisited  bool // whether the scope tree step was visited this run
}

// New creates a Wizard. Call Init() to start.
func New(theme styles.Theme, keys styles.KeyMap, deps Deps) Wizard {
	w := Wizard{theme: theme, keys: keys, deps: deps}
	w.roleList = NewRoleList(theme, keys, deps.LoadActive, deps.RoleFilter, deps.LoadRoles)
	return w
}

// Init starts the first step (or fast-forwards if flags allow).
func (w Wizard) Init() tea.Cmd {
	return w.roleList.Init()
}

// Editing reports whether the active step is in a text-input mode.
func (w Wizard) Editing() bool {
	switch w.step {
	case stepRoleList:
		return w.roleList.Editing()
	case stepOptions:
		return w.options.Editing()
	}
	return false
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
		for _, scope := range msg.Scopes {
			w.items = append(w.items, activationItem{
				role:        msg.Role,
				targetScope: scope,
			})
		}
		if len(w.scopeQueue) > 0 {
			w.scopeQueue = w.scopeQueue[1:]
		}
		if len(w.scopeQueue) > 0 {
			return w.startNextScopeTree()
		}
		return w.startOptions()

	case OptionsDoneMsg:
		w.deps.Store.AddRecentJustification(msg.Justification)
		_ = w.deps.Store.SaveState()
		return w.startConfirm(msg.Minutes, msg.Justification)

	case ConfirmDoneMsg:
		done := WizardDoneMsg{Results: msg.Results}
		return w, func() tea.Msg { return done }

	case autoConfirmMsg:
		return w.handleAutoConfirm()
	}

	// Delegate to active step first so sub-models (e.g. filter mode in rolelist)
	// can consume the key before the wizard interprets it as back/cancel.
	var cmd tea.Cmd
	var consumed bool
	switch w.step {
	case stepRoleList:
		prev := w.roleList
		w.roleList, cmd = w.roleList.Update(msg)
		consumed = roleListConsumed(prev, w.roleList, msg)
	case stepScopeTree:
		w.scopeTree, cmd = w.scopeTree.Update(msg)
	case stepOptions:
		w.options, cmd = w.options.Update(msg)
	case stepConfirm:
		w.confirm, cmd = w.confirm.Update(msg)
	}
	if cmd != nil || consumed {
		return w, cmd
	}

	// Sub-model did not consume the key; handle wizard-level back/cancel.
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		if kp.String() == "esc" || kp.String() == "q" {
			return w.handleBack()
		}
	}
	return w, cmd
}

// handleBack navigates one step back or cancels the wizard.
func (w Wizard) handleBack() (Wizard, tea.Cmd) {
	switch w.step {
	case stepRoleList:
		return w, func() tea.Msg { return WizardCancelMsg{} }
	case stepScopeTree:
		w.step = stepRoleList
		return w, w.roleList.Init()
	case stepOptions:
		if w.scopeVisited {
			// Rebuild scope queue and items so the user can re-select scopes.
			w.items = nil
			var treeRoles []azure.Role
			for _, r := range w.selectedRoles {
				switch r.ScopeKind() {
				case azure.ScopeManagementGroup, azure.ScopeSubscription:
					if w.scopeOverride(r) == "" {
						treeRoles = append(treeRoles, r)
					}
				}
			}
			w.scopeQueue = treeRoles
			w.step = stepScopeTree
			return w, w.scopeTree.Init()
		}
		w.step = stepRoleList
		return w, w.roleList.Init()
	case stepConfirm:
		w.step = stepOptions
		return w, w.options.Init()
	}
	return w, nil
}

// handleAutoConfirm executes the --yes submission on the confirm step.
func (w Wizard) handleAutoConfirm() (Wizard, tea.Cmd) {
	if w.step != stepConfirm {
		return w, nil
	}
	w.confirm.submitted = true
	cmds := make([]tea.Cmd, len(w.confirm.items))
	for i := range w.confirm.items {
		w.confirm.items[i].status = statusRunning
		cmds[i] = w.confirm.runActivation(i)
	}
	return w, tea.Batch(append([]tea.Cmd{w.confirm.spinner.Init()}, cmds...)...)
}

// roleListConsumed reports whether the rolelist consumed the message.
// It returns true when the list was in filter mode (the key typed into the filter).
func roleListConsumed(prev, next RoleList, msg tea.Msg) bool {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return false
	}
	// If the list was filtering, it consumed everything except enter/esc that exit filter.
	if prev.filtering {
		s := kp.String()
		return s != "enter" && s != "esc"
	}
	return false
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

// advanceFromRoles decides whether to show scope trees or skip straight to options.
func (w Wizard) advanceFromRoles() (Wizard, tea.Cmd) {
	// Partition into roles that need scope selection and those that don't.
	// MG-scoped roles: show full MG tree so user can drill to sub/RG.
	// Sub-scoped roles: show sub-rooted tree so user can optionally drill to RG.
	// RG-scoped roles: already at leaf scope — go straight to options.
	var treeRoles []azure.Role
	for _, r := range w.selectedRoles {
		switch r.ScopeKind() {
		case azure.ScopeManagementGroup:
			if w.scopeOverride(r) != "" {
				w.items = append(w.items, activationItem{role: r, targetScope: w.scopeOverride(r)})
			} else {
				treeRoles = append(treeRoles, r)
			}
		case azure.ScopeSubscription:
			if w.scopeOverride(r) != "" {
				w.items = append(w.items, activationItem{role: r, targetScope: w.scopeOverride(r)})
			} else {
				treeRoles = append(treeRoles, r)
			}
		default:
			w.items = append(w.items, activationItem{role: r})
		}
	}

	if len(treeRoles) > 0 {
		w.scopeQueue = treeRoles
		return w.startNextScopeTree()
	}
	return w.startOptions()
}

// scopeOverride returns the target scope to use when a --scope filter matches
// the role's eligibility scope. Returns the filter value when it is a valid
// ARM child path, or the role's own scope when the filter matches by display
// name substring. Returns "" when no filter matches.
func (w Wizard) scopeOverride(r azure.Role) string {
	for _, s := range w.deps.ScopeFilter {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if azure.ScopeIsChildOf(s, r.Scope) {
			return s
		}
		lower := strings.ToLower(s)
		if strings.Contains(strings.ToLower(r.ScopeDisplay), lower) ||
			strings.Contains(strings.ToLower(r.Scope), lower) {
			return r.Scope
		}
	}
	return ""
}

func (w Wizard) startNextScopeTree() (Wizard, tea.Cmd) {
	role := w.scopeQueue[0]
	if role.ScopeKind() == azure.ScopeSubscription {
		w.scopeTree = NewScopeTreeForSub(w.theme, w.keys, role, w.deps.LoadRGs)
	} else {
		w.scopeTree = NewScopeTree(w.theme, w.keys, role, w.deps.LoadSubs, w.deps.LoadRGs)
	}
	w.step = stepScopeTree
	w.scopeVisited = true
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
		w.deps.Store.RecentJustifications(),
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

	// --yes: auto-submit immediately via a typed message so Confirm.Update handles it.
	if w.deps.AutoSubmit {
		return w, func() tea.Msg { return autoConfirmMsg{} }
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

	var parts []string
	for i, st := range steps {
		n := i + 1
		var label string
		switch {
		case st.s == w.step:
			label = w.theme.TableRowSelected.Render(fmt.Sprintf("%d. %s", n, st.label))
		case int(st.s) < int(w.step):
			label = w.theme.Subtle.Render(fmt.Sprintf("✓ %s", st.label))
		default:
			label = w.theme.Subtle.Render(fmt.Sprintf("%d. %s", n, st.label))
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, w.theme.Subtle.Render("  ›  "))
}
