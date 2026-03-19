package tui

import (
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
}

// New creates the root AppModel. Fetches current user synchronously.
func New(a *app.App) (AppModel, error) {
	keys := DefaultKeyMap
	theme := NewTheme(true)

	user, err := a.Client.GetCurrentUser()
	if err != nil {
		return AppModel{}, err
	}
	principalID := user.ID

	dash := dashboard.New(theme, keys, a.Store)

	stat := status.New(theme, keys, func() ([]azure.ActiveAssignment, []azure.Role, error) {
		active, err := a.Client.GetActiveAssignments(principalID)
		if err != nil {
			return nil, nil, err
		}
		eligible, err := a.Client.GetEligibleRoles()
		if err != nil {
			return nil, nil, err
		}
		return active, eligible, nil
	})

	favs := favorites.New(theme, keys, a.Store)

	return AppModel{
		a:              a,
		theme:          theme,
		keys:           keys,
		screen:         ScreenDashboard,
		dashboardModel: dash,
		statusModel:    stat,
		favoritesModel: favs,
		principalID:    principalID,
	}, nil
}

// Init initialises the active screen based on the invoked command.
func (m AppModel) Init() tea.Cmd {
	switch m.a.Config.Command {
	case app.CmdActivate:
		return m.startWizard(nil)
	case app.CmdDeactivate:
		return m.startDeactivate()
	case app.CmdStatus:
		m.screen = ScreenStatus
		return m.statusModel.Init()
	}
	return m.dashboardModel.Init()
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

	// Activation wizard results.
	case activate.WizardDoneMsg:
		m.screen = ScreenDashboard
		return m, m.dashboardModel.Init()

	case activate.WizardCancelMsg:
		m.screen = ScreenDashboard
		return m, nil

	// Deactivation results.
	case deactivate.DoneMsg:
		m.screen = ScreenDashboard
		return m, m.dashboardModel.Init()

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
		if msg.String() == "?" {
			m.showHelp = !m.showHelp
			return m, nil
		}
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		// Navigation shortcuts are only intercepted on the dashboard.
		// All other screens own their key handling, including esc/q for back/cancel.
		if m.screen == ScreenDashboard {
			switch {
			case msg.String() == "s":
				m.screen = ScreenStatus
				return m, m.statusModel.Init()
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
	return m, cmd
}

// startWizard builds the Wizard deps and switches to the activate screen.
// fav may be nil (full wizard) or point to a pre-filled favorite.
func (m *AppModel) startWizard(fav *state.Favorite) tea.Cmd {
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
			return client.ListSubscriptionResourceGroups(subID)
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
	principalID := m.principalID
	client := m.a.Client

	m.deactivateModel = deactivate.New(
		m.theme,
		m.keys,
		principalID,
		func() ([]azure.ActiveAssignment, error) {
			return client.GetActiveAssignments(principalID)
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
	_, err = p.Run()
	return err
}
