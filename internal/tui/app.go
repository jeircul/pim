package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
	ctx             context.Context
	cancel          context.CancelFunc
	theme           Theme
	keys            KeyMap
	screen          Screen
	dashboardModel  dashboard.Model
	statusModel     status.Model
	wizardModel     activate.Wizard
	deactivateModel deactivate.Model
	favoritesModel  favorites.Model
	principalID     string
	userReady       bool
	favoritePending bool
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
func New(a *app.App, ctx context.Context, cancel context.CancelFunc) (AppModel, error) {
	keys := DefaultKeyMap
	theme := NewTheme(true)

	dash := dashboard.New(theme, keys, a.Store)
	favs := favorites.New(theme, keys, a.Store)

	return AppModel{
		a:              a,
		ctx:            ctx,
		cancel:         cancel,
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
		callCtx, callCancel := context.WithTimeout(m.ctx, 30*time.Second)
		defer callCancel()
		user, err := m.a.Client.GetCurrentUser(callCtx)
		if err != nil {
			return userReadyMsg{err: err}
		}
		return userReadyMsg{principalID: user.ID}
	}
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
			m.dashboardModel.SetAuthErr(fmt.Sprintf("auth: %v", msg.err))
			return m, nil
		}
		m.principalID = msg.principalID
		m.userReady = true
		m.dashboardModel.SetReady()
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

	case status.CancelMsg:
		m.screen = ScreenDashboard
		return m, nil

	case activate.WizardDoneMsg:
		if m.favoritePending {
			m.favoritePending = false
			summary, err := buildActivationSummary(msg.Results)
			notice := strings.TrimRight(summary, "\n")
			m.dashboardModel.SetNotice(notice, err != nil)
			m.screen = ScreenDashboard
			return m, nil
		}
		m.exitSummary, m.exitErr = buildActivationSummary(msg.Results)
		return m, tea.Quit

	case activate.WizardCancelMsg:
		m.screen = ScreenDashboard
		return m, nil

	case deactivate.DoneMsg:
		m.exitSummary, m.exitErr = buildDeactivationSummary(msg.Results)
		return m, tea.Quit

	case deactivate.CancelMsg:
		m.screen = ScreenDashboard
		return m, nil

	case favorites.DoneMsg:
		m.screen = ScreenDashboard
		return m, nil

	case favorites.ActivateMsg:
		fav := msg.Favorite
		return m, m.startWizard(&fav)

	case dashboard.ActivateMsg:
		if !m.userReady {
			return m, nil
		}
		return m, m.startWizard(msg.Favorite)

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.cancel()
			return m, tea.Quit
		}
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		// Navigation shortcuts are only intercepted on the dashboard.
		// All other screens own their key handling, including esc/q for back/cancel.
		if m.screen == ScreenDashboard {
			switch {
			case key.Matches(msg, m.keys.Status):
				return m, m.startStatus()
			case key.Matches(msg, m.keys.Deactivate):
				if !m.userReady {
					return m, nil
				}
				return m, m.startDeactivate()
			case key.Matches(msg, m.keys.Favorites):
				m.screen = ScreenFavorites
				return m, m.favoritesModel.Init()
			}
		}
	}

	var cmd tea.Cmd
	editing := false
	switch m.screen {
	case ScreenActivate:
		m.wizardModel, cmd = m.wizardModel.Update(msg)
		editing = m.wizardModel.Editing()
	case ScreenStatus:
		m.statusModel, cmd = m.statusModel.Update(msg)
	case ScreenDeactivate:
		m.deactivateModel, cmd = m.deactivateModel.Update(msg)
	case ScreenFavorites:
		m.favoritesModel, cmd = m.favoritesModel.Update(msg)
		editing = m.favoritesModel.Editing()
	default:
		m.dashboardModel, cmd = m.dashboardModel.Update(msg)
	}
	if !editing {
		if kp, ok := msg.(tea.KeyPressMsg); ok && key.Matches(kp, m.keys.Help) {
			m.showHelp = !m.showHelp
		}
	}
	return m, cmd
} // startStatus constructs a fresh status model and switches to that screen.
func (m *AppModel) startStatus() tea.Cmd {
	client := m.a.Client
	ctx := m.ctx
	m.statusModel = status.New(m.theme, m.keys, func() ([]azure.ActiveAssignment, []azure.Role, error) {
		callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel()
		active, err := client.GetActiveAssignments(callCtx)
		if err != nil {
			return nil, nil, err
		}
		callCtx2, callCancel2 := context.WithTimeout(ctx, 30*time.Second)
		defer callCancel2()
		eligible, err := client.GetEligibleRoles(callCtx2)
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
	ctx := m.ctx

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
		RoleFilter:  roleFilter,
		ScopeFilter: scopeFilter,
		TimeStr:     timeStr,
		Justific:    cfg.Justification,
		AutoSubmit:  cfg.Yes,
		Store:       m.a.Store,
		LoadRoles: func() ([]azure.Role, error) {
			callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
			defer callCancel()
			return client.GetEligibleRoles(callCtx)
		},
		LoadActive: func() ([]azure.ActiveAssignment, error) {
			callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
			defer callCancel()
			return client.GetActiveAssignments(callCtx)
		},
		LoadSubs: func(mgID string) ([]azure.Subscription, error) {
			callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
			defer callCancel()
			return client.ListManagementGroupSubscriptions(callCtx, mgID)
		},
		LoadRGs: func(subID string) ([]azure.ResourceGroup, error) {
			callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
			defer callCancel()
			return client.ListEligibleResourceGroups(callCtx, subID)
		},
		Activate: func(role azure.Role, pid, justification string, minutes int, targetScope string) error {
			callCtx, callCancel := context.WithTimeout(ctx, 60*time.Second)
			defer callCancel()
			_, err := client.ActivateRole(callCtx, role, pid, justification, minutes, targetScope)
			return err
		},
	}

	if fav != nil && fav.Justification != "" && deps.Justific == "" {
		deps.Justific = fav.Justification
	}
	if fav != nil && fav.Complete() {
		deps.AutoSubmit = true
		m.favoritePending = true
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
	ctx := m.ctx

	m.deactivateModel = deactivate.New(
		m.theme,
		m.keys,
		principalID,
		func() ([]azure.ActiveAssignment, error) {
			callCtx, callCancel := context.WithTimeout(ctx, 30*time.Second)
			defer callCancel()
			return client.GetActiveAssignments(callCtx)
		},
		func(assignment azure.ActiveAssignment, pid string) error {
			callCtx, callCancel := context.WithTimeout(ctx, 60*time.Second)
			defer callCancel()
			_, err := client.DeactivateRole(callCtx, assignment, pid)
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
		body = components.RenderHelp(m.theme, m.keys, m.width)
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
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	model, err := New(a, ctx, cancel)
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
