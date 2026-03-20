package tui

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jeircul/pim/internal/app"
	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/state"
	"github.com/jeircul/pim/internal/tui/activate"
	"github.com/jeircul/pim/internal/tui/components"
	"github.com/jeircul/pim/internal/tui/dashboard"
	"github.com/jeircul/pim/internal/tui/deactivate"
	"github.com/jeircul/pim/internal/tui/favorites"
	"github.com/jeircul/pim/internal/tui/status"
)

// ErrSilent is returned by Run when activation or deactivation completed with
// failures that were already printed to stdout. The caller should exit 1
// without printing the error again.
var ErrSilent = errors.New("silent")

// Screen identifies which screen is active.
type Screen int

const (
	ScreenDashboard Screen = iota
	ScreenStatus
	ScreenActivate
	ScreenDeactivate
	ScreenFavorites
)

// titleOf returns the header subtitle for a screen.
func titleOf(s Screen) string {
	switch s {
	case ScreenStatus:
		return "status"
	case ScreenActivate:
		return "activate"
	case ScreenDeactivate:
		return "deactivate"
	case ScreenFavorites:
		return "favorites"
	default:
		return "pim"
	}
}

// AppModel is the root Bubble Tea model.
type AppModel struct {
	a               *app.App
	theme           Theme
	keys            KeyMap
	screen          Screen
	dashboardModel  dashboard.Model
	statusModel     status.Model
	wizardModel     activate.Wizard
	deactivateModel deactivate.Model
	favoritesModel  favorites.Model
	principalID     string
	width           int
	height          int
	isDark          bool
	showHelp        bool
	exitSummary     string
	exitErr         error
}

// userReadyMsg carries the resolved principal ID from the background user fetch.
type userReadyMsg struct {
	principalID string
	err         error
}

// New creates the root AppModel. User fetch is deferred to Init().
func New(a *app.App) (AppModel, error) {
	keys := DefaultKeyMap
	theme := NewTheme(true)

	dash := dashboard.New(theme, keys, a.Store)
	favs := favorites.New(theme, keys, a.Store)

	return AppModel{
		a:              a,
		theme:          theme,
		keys:           keys,
		screen:         ScreenDashboard,
		dashboardModel: dash,
		favoritesModel: favs,
	}, nil
}

// Init initialises the active screen and starts the background user fetch.
func (m AppModel) Init() tea.Cmd {
	fetchUser := func() tea.Msg {
		user, err := m.a.Client.GetCurrentUser()
		if err != nil {
			return userReadyMsg{err: err}
		}
		return userReadyMsg{principalID: user.ID}
	}
	// For headless commands the actual screen transition is deferred until
	// userReadyMsg arrives (principalID is required for all PIM API calls).
	// For the default dashboard path, start the dashboard immediately.
	if m.a.Config.Command == "" {
		return tea.Batch(fetchUser, m.dashboardModel.Init())
	}
	return fetchUser
}

// Update routes messages to the active screen and handles global keys.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.BackgroundColorMsg:
		m.isDark = msg.IsDark()
		m.theme = NewTheme(m.isDark)
		return m, nil

	case userReadyMsg:
		if msg.err != nil {
			// Non-fatal: show error on dashboard and let user retry.
			return m, nil
		}
		m.principalID = msg.principalID
		// If a headless command was pending, dispatch it now.
		switch m.a.Config.Command {
		case app.CmdActivate:
			return m, m.startWizard(nil)
		case app.CmdDeactivate:
			return m, m.startDeactivate()
		case app.CmdStatus:
			return m, m.startStatus()
		}
		return m, nil

	// Status results.
	case status.CancelMsg:
		m.screen = ScreenDashboard
		return m, nil

	// Activation wizard results.
	case activate.WizardDoneMsg:
		m.exitSummary, m.exitErr = buildActivationSummary(msg.Results)
		return m, tea.Quit

	case activate.WizardCancelMsg:
		m.screen = ScreenDashboard
		return m, nil

	// Deactivation results.
	case deactivate.DoneMsg:
		m.exitSummary, m.exitErr = buildDeactivationSummary(msg.Results)
		return m, tea.Quit

	case deactivate.CancelMsg:
		m.screen = ScreenDashboard
		return m, nil

	// Favorites results.
	case favorites.DoneMsg:
		m.screen = ScreenDashboard
		return m, nil

	case favorites.ActivateMsg:
		fav := msg.Favorite
		return m, m.startWizard(&fav)

	// Dashboard activation shortcut.
	case dashboard.ActivateMsg:
		return m, m.startWizard(msg.Favorite)

	case tea.KeyPressMsg:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		// Navigation shortcuts are only intercepted on the dashboard.
		// All other screens own their key handling, including esc/q for back/cancel.
		if m.screen == ScreenDashboard {
			switch {
			case msg.String() == "s":
				return m, m.startStatus()
			case key.Matches(msg, m.keys.Deactivate):
				return m, m.startDeactivate()
			case msg.String() == "f":
				m.screen = ScreenFavorites
				return m, m.favoritesModel.Init()
			}
		}
	}

	// Delegate to active screen.
	var cmd tea.Cmd
	switch m.screen {
	case ScreenActivate:
		m.wizardModel, cmd = m.wizardModel.Update(msg)
	case ScreenStatus:
		m.statusModel, cmd = m.statusModel.Update(msg)
	case ScreenDeactivate:
		m.deactivateModel, cmd = m.deactivateModel.Update(msg)
	case ScreenFavorites:
		m.favoritesModel, cmd = m.favoritesModel.Update(msg)
	default:
		m.dashboardModel, cmd = m.dashboardModel.Update(msg)
	}
	// Toggle help only if the active screen did not consume the key.
	if cmd == nil {
		if kp, ok := msg.(tea.KeyPressMsg); ok && kp.String() == "?" {
			m.showHelp = !m.showHelp
		}
	}
	return m, cmd
}

// startStatus constructs a fresh status model and switches to that screen.
func (m *AppModel) startStatus() tea.Cmd {
	client := m.a.Client
	m.statusModel = status.New(m.theme, m.keys, func() ([]azure.ActiveAssignment, []azure.Role, error) {
		active, err := client.GetActiveAssignments()
		if err != nil {
			return nil, nil, err
		}
		eligible, err := client.GetEligibleRoles()
		if err != nil {
			return nil, nil, err
		}
		return active, eligible, nil
	})
	m.screen = ScreenStatus
	return m.statusModel.Init()
}

// startWizard builds the Wizard deps and switches to the activate screen.
// fav may be nil (full wizard) or point to a pre-filled favorite.
func (m *AppModel) startWizard(fav *state.Favorite) tea.Cmd {
	if m.principalID == "" {
		m.exitSummary = "error: user identity not yet resolved — please retry\n"
		m.exitErr = errors.New("principal ID unavailable")
		return tea.Quit
	}
	cfg := m.a.Config
	principalID := m.principalID
	client := m.a.Client

	var currentActive []azure.ActiveAssignment

	roleFilter := cfg.Roles
	scopeFilter := cfg.Scopes
	timeStr := cfg.TimeStr

	if fav != nil {
		if fav.Role != "" {
			roleFilter = append([]string{fav.Role}, roleFilter...)
		}
		if fav.Scope != "" && len(scopeFilter) == 0 {
			scopeFilter = []string{fav.Scope}
		}
		if fav.Duration != "" && timeStr == "" {
			timeStr = fav.Duration
		}
	}

	deps := activate.Deps{
		PrincipalID: principalID,
		Active:      currentActive,
		RoleFilter:  roleFilter,
		ScopeFilter: scopeFilter,
		TimeStr:     timeStr,
		Justific:    cfg.Justification,
		AutoSubmit:  cfg.Yes,
		Store:       m.a.Store,
		LoadRoles: func() ([]azure.Role, error) {
			return client.GetEligibleRoles()
		},
		LoadSubs: func(mgID string) ([]azure.Subscription, error) {
			return client.ListManagementGroupSubscriptions(mgID)
		},
		LoadRGs: func(subID string) ([]azure.ResourceGroup, error) {
			return client.ListEligibleResourceGroups(subID)
		},
		Activate: func(role azure.Role, pid, justification string, minutes int, targetScope string) error {
			_, err := client.ActivateRole(role, pid, justification, minutes, targetScope)
			return err
		},
	}

	m.wizardModel = activate.New(m.theme, m.keys, deps)
	m.screen = ScreenActivate
	return m.wizardModel.Init()
}

// startDeactivate constructs the deactivation model and switches to that screen.
func (m *AppModel) startDeactivate() tea.Cmd {
	if m.principalID == "" {
		m.exitSummary = "error: user identity not yet resolved — please retry\n"
		m.exitErr = errors.New("principal ID unavailable")
		return tea.Quit
	}
	principalID := m.principalID
	client := m.a.Client

	m.deactivateModel = deactivate.New(
		m.theme,
		m.keys,
		principalID,
		func() ([]azure.ActiveAssignment, error) {
			return client.GetActiveAssignments()
		},
		func(assignment azure.ActiveAssignment, pid string) error {
			_, err := client.DeactivateRole(assignment, pid)
			return err
		},
	)
	m.screen = ScreenDeactivate
	return m.deactivateModel.Init()
}

// View renders the active screen inside a header/footer frame.
func (m AppModel) View() tea.View {
	if m.width == 0 {
		return tea.NewView("")
	}

	var sb strings.Builder

	sb.WriteString(components.RenderHeader(m.theme.Header, m.theme.Subtle, titleOf(m.screen), m.a.Version, m.width))
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(m.theme.Muted).Render(strings.Repeat("─", m.width)))
	sb.WriteString("\n")

	bodyHeight := m.height - 3
	if bodyHeight < 1 {
		bodyHeight = 1
	}

	var body string
	if m.showHelp {
		body = components.RenderHelp(m.theme, m.width)
	} else {
		switch m.screen {
		case ScreenActivate:
			body = m.wizardModel.View()
		case ScreenStatus:
			body = m.statusModel.View()
		case ScreenDeactivate:
			body = m.deactivateModel.View()
		case ScreenFavorites:
			body = m.favoritesModel.View()
		default:
			body = m.dashboardModel.View()
		}
	}

	lines := strings.Split(body, "\n")
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	sb.WriteString(strings.Join(lines[:bodyHeight], "\n"))

	v := tea.NewView(sb.String())
	v.AltScreen = true
	v.WindowTitle = "pim"
	return v
}

// Run starts the Bubble Tea program.
func Run(a *app.App) error {
	model, err := New(a)
	if err != nil {
		return err
	}
	p := tea.NewProgram(model)
	final, err := p.Run()
	if err != nil {
		return err
	}
	if m, ok := final.(AppModel); ok && m.exitSummary != "" {
		fmt.Print(m.exitSummary)
		if m.exitErr != nil {
			return ErrSilent
		}
		return nil
	}
	return nil
}

// buildActivationSummary formats per-item activation results for stdout.
// Returns the summary string and a combined error if any item failed.
func buildActivationSummary(results []activate.Result) (string, error) {
	var sb strings.Builder
	var errs []error
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(&sb, "failed:  %s @ %s — %v\n", r.RoleName, r.Scope, r.Err)
			errs = append(errs, r.Err)
		} else {
			fmt.Fprintf(&sb, "activated: %s @ %s\n", r.RoleName, r.Scope)
		}
	}
	return sb.String(), errors.Join(errs...)
}

// buildDeactivationSummary formats per-item deactivation results for stdout.
func buildDeactivationSummary(results []deactivate.Result) (string, error) {
	var sb strings.Builder
	var errs []error
	for _, r := range results {
		if r.Err != nil {
			fmt.Fprintf(&sb, "failed:      %s @ %s — %v\n", r.RoleName, r.Scope, r.Err)
			errs = append(errs, r.Err)
		} else {
			fmt.Fprintf(&sb, "deactivated: %s @ %s\n", r.RoleName, r.Scope)
		}
	}
	return sb.String(), errors.Join(errs...)
}
