package activate

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/tui/components"
	"github.com/jeircul/pim/internal/tui/styles"
)

// ScopeTreeDoneMsg is sent when the user confirms scope selections for one role.
type ScopeTreeDoneMsg struct {
	Role   azure.Role
	Scopes []string // one activation will be created per scope
}

// scopeNode is a node in the tree (MG, subscription, or resource group).
type scopeNode struct {
	id       string
	display  string
	kind     azure.ScopeType
	scope    string
	children []*scopeNode
	expanded bool
	loading  bool
	loaded   bool
	loadErr  error
	parent   *scopeNode
}

// ScopeTree is Step 2: lazy-loaded scope tree for MG- or subscription-scoped roles.
type ScopeTree struct {
	theme     styles.Theme
	keys      styles.KeyMap
	spinner   components.Spinner
	role      azure.Role
	root      *scopeNode
	flat      []*scopeNode // flattened visible list
	cursor    int
	viewport  int             // top visible row index
	selected  map[string]bool // set of selected scope paths
	filter    string
	filtering bool
	width     int
	height    int
	// subRoot is true when the tree is rooted at a subscription (not an MG).
	// In this mode the root can be selected directly without expansion.
	subRoot bool
	// loadSubs fetches child management groups and subscriptions under a management group ID.
	loadSubs func(mgID string) ([]azure.ManagementGroup, []azure.Subscription, error)
	// loadRGs fetches resource groups under a subscription ID.
	loadRGs func(subID string) ([]azure.ResourceGroup, error)
}

type scopeChildrenMsg struct {
	parentScope string
	mgs         []azure.ManagementGroup
	subs        []azure.Subscription
	rgs         []azure.ResourceGroup
	err         error
}

// NewScopeTree creates a ScopeTree for the given MG-scoped role.
func NewScopeTree(
	theme styles.Theme,
	keys styles.KeyMap,
	role azure.Role,
	loadSubs func(string) ([]azure.ManagementGroup, []azure.Subscription, error),
	loadRGs func(string) ([]azure.ResourceGroup, error),
) ScopeTree {
	root := &scopeNode{
		id:      azure.ManagementGroupIDFromScope(role.Scope),
		display: azure.DefaultScopeDisplay(role.Scope, role.ScopeDisplay),
		kind:    azure.ScopeManagementGroup,
		scope:   role.Scope,
	}
	st := ScopeTree{
		theme:    theme,
		keys:     keys,
		spinner:  components.NewSpinner(theme.Active),
		role:     role,
		root:     root,
		selected: make(map[string]bool),
		loadSubs: loadSubs,
		loadRGs:  loadRGs,
	}
	st.flatten()
	return st
}

// NewScopeTreeForSub creates a ScopeTree rooted at the subscription for the
// given subscription-scoped role. The user may select the subscription itself
// or expand it to choose a resource group.
func NewScopeTreeForSub(
	theme styles.Theme,
	keys styles.KeyMap,
	role azure.Role,
	loadRGs func(string) ([]azure.ResourceGroup, error),
) ScopeTree {
	root := &scopeNode{
		id:      azure.SubscriptionIDFromScope(role.Scope),
		display: azure.DefaultScopeDisplay(role.Scope, role.ScopeDisplay),
		kind:    azure.ScopeSubscription,
		scope:   role.Scope,
	}
	st := ScopeTree{
		theme:    theme,
		keys:     keys,
		spinner:  components.NewSpinner(theme.Active),
		role:     role,
		root:     root,
		selected: make(map[string]bool),
		subRoot:  true,
		loadRGs:  loadRGs,
	}
	st.flatten()
	return st
}

// Init starts the spinner. Children are loaded lazily when the user presses
// l/right to expand; the MG root itself is always selectable without expansion.
func (m ScopeTree) Init() tea.Cmd {
	return m.spinner.Init()
}

// Editing reports whether the filter text field is active.
func (m ScopeTree) Editing() bool { return m.filtering }

func (m ScopeTree) expandNode(n *scopeNode) tea.Cmd {
	if n.loaded || n.loading {
		return nil
	}
	n.loading = true
	scope := n.scope
	kind := n.kind
	return func() tea.Msg {
		msg := scopeChildrenMsg{parentScope: scope}
		switch kind {
		case azure.ScopeManagementGroup:
			mgID := azure.ManagementGroupIDFromScope(scope)
			mgs, subs, err := m.loadSubs(mgID)
			msg.mgs = mgs
			msg.subs = subs
			msg.err = err
		case azure.ScopeSubscription:
			subID := azure.SubscriptionIDFromScope(scope)
			rgs, err := m.loadRGs(subID)
			msg.rgs = rgs
			msg.err = err
		}
		return msg
	}
}

// Update handles messages.
func (m ScopeTree) Update(msg tea.Msg) (ScopeTree, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case scopeChildrenMsg:
		n := m.findNode(msg.parentScope)
		if n == nil {
			break
		}
		n.loading = false
		if msg.err != nil {
			n.loadErr = msg.err
			break
		}
		n.loaded = true
		n.expanded = true
		for _, mg := range msg.mgs {
			n.children = append(n.children, &scopeNode{
				id:      mg.ID,
				display: mg.DisplayName,
				kind:    azure.ScopeManagementGroup,
				scope:   mg.Scope(),
				parent:  n,
			})
		}
		for _, s := range msg.subs {
			n.children = append(n.children, &scopeNode{
				id:      s.ID,
				display: s.DisplayName,
				kind:    azure.ScopeSubscription,
				scope:   s.Scope(),
				parent:  n,
			})
		}
		for _, rg := range msg.rgs {
			n.children = append(n.children, &scopeNode{
				id:      rg.Name,
				display: rg.Name,
				kind:    azure.ScopeResourceGroup,
				scope:   rg.Scope(),
				parent:  n,
			})
		}
		m.flatten()

	case tea.KeyPressMsg:
		if m.filtering {
			return m.updateFilter(msg)
		}
		switch {
		case key.Matches(msg, m.keys.Up), msg.String() == "k":
			if m.cursor > 0 {
				m.cursor--
				m.adjustViewport()
			}
		case key.Matches(msg, m.keys.Down), msg.String() == "j":
			if m.cursor < len(m.flat)-1 {
				m.cursor++
				m.adjustViewport()
			}
		case msg.String() == "/":
			m.filtering = true
		case msg.String() == "esc":
			if m.filter != "" {
				m.filter = ""
				m.flatten()
				m.cursor = 0
				m.viewport = 0
			}
		case msg.String() == "l", msg.String() == "right":
			if m.cursor < len(m.flat) {
				n := m.flat[m.cursor]
				if n.kind == azure.ScopeResourceGroup || n.expanded {
					break
				}
			if n.kind == azure.ScopeManagementGroup || (n.kind == azure.ScopeSubscription && n != m.root) || m.subRoot {
				if n.loadErr != nil {
					n.loadErr = nil
					}
					if n.loaded {
						n.expanded = true
						m.flatten()
						break
					}
					cmd := m.expandNode(n)
					m.flatten()
					return m, tea.Batch(m.spinner.Init(), cmd)
				}
			}
		case msg.String() == "h", msg.String() == "left":
			if m.cursor < len(m.flat) {
				n := m.flat[m.cursor]
				if n.expanded {
					n.expanded = false
					m.flatten()
				} else if n.parent != nil {
					for i, fn := range m.flat {
						if fn == n.parent {
							m.cursor = i
							m.adjustViewport()
							break
						}
					}
				}
			}
		case msg.String() == "space":
			if m.cursor < len(m.flat) {
				n := m.flat[m.cursor]
				if n.loadErr != nil && n != m.root && n.kind != azure.ScopeSubscription {
					break
				}
				scope := n.scope
				if m.selected[scope] {
					delete(m.selected, scope)
				} else {
					m.selected[scope] = true
				}
			}
		case key.Matches(msg, m.keys.Enter):
			if len(m.selected) > 0 {
				scopes := make([]string, 0, len(m.selected))
				for s := range m.selected {
					scopes = append(scopes, s)
				}
				role := m.role
				return m, func() tea.Msg { return ScopeTreeDoneMsg{Role: role, Scopes: scopes} }
			}
		case key.Matches(msg, m.keys.Back):
		}

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *ScopeTree) updateFilter(msg tea.KeyPressMsg) (ScopeTree, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filtering = false
	case "esc":
		m.filter = ""
		m.filtering = false
		m.flatten()
		m.cursor = 0
		m.viewport = 0
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.flatten()
			m.cursor = 0
			m.viewport = 0
		}
	case "space":
		m.filter += " "
		m.flatten()
		m.cursor = 0
		m.viewport = 0
	default:
		if r := msg.String(); len(r) == 1 {
			m.filter += r
			m.flatten()
			m.cursor = 0
			m.viewport = 0
		}
	}
	return *m, nil
}

// flatten rebuilds the visible flat list from the tree, applying filter if set.
func (m *ScopeTree) flatten() {
	m.flat = m.flat[:0]
	if m.filter == "" {
		m.flattenNode(m.root)
		return
	}
	lower := strings.ToLower(m.filter)
	m.flattenFiltered(m.root, lower)
}

// flattenFiltered includes a node if it or any descendant matches the filter.
// Ancestor nodes of matching nodes are always included to preserve tree structure.
func (m *ScopeTree) flattenFiltered(n *scopeNode, lower string) bool {
	selfMatch := strings.Contains(strings.ToLower(n.display), lower) ||
		strings.Contains(strings.ToLower(n.scope), lower)

	childMatch := false
	var matchingChildren []*scopeNode
	for _, c := range n.children {
		if m.flattenFilteredCollect(c, lower) {
			childMatch = true
			matchingChildren = append(matchingChildren, c)
		}
	}

	if !selfMatch && !childMatch {
		return false
	}

	m.flat = append(m.flat, n)
	for _, c := range matchingChildren {
		m.flattenFilteredEmit(c, lower)
	}
	return true
}

// flattenFilteredCollect returns true if n or any descendant matches.
func (m *ScopeTree) flattenFilteredCollect(n *scopeNode, lower string) bool {
	if strings.Contains(strings.ToLower(n.display), lower) ||
		strings.Contains(strings.ToLower(n.scope), lower) {
		return true
	}
	for _, c := range n.children {
		if m.flattenFilteredCollect(c, lower) {
			return true
		}
	}
	return false
}

// flattenFilteredEmit appends n and its matching descendants to flat.
func (m *ScopeTree) flattenFilteredEmit(n *scopeNode, lower string) {
	if !m.flattenFilteredCollect(n, lower) {
		return
	}
	m.flat = append(m.flat, n)
	for _, c := range n.children {
		m.flattenFilteredEmit(c, lower)
	}
}

func (m *ScopeTree) flattenNode(n *scopeNode) {
	m.flat = append(m.flat, n)
	if n.expanded {
		for _, c := range n.children {
			m.flattenNode(c)
		}
	}
}

func (m *ScopeTree) findNode(scope string) *scopeNode {
	return findInTree(m.root, scope)
}

func findInTree(n *scopeNode, scope string) *scopeNode {
	if n.scope == scope {
		return n
	}
	for _, c := range n.children {
		if found := findInTree(c, scope); found != nil {
			return found
		}
	}
	return nil
}

// adjustViewport keeps cursor within the visible window.
func (m *ScopeTree) adjustViewport() {
	visible := m.visibleRows()
	if m.cursor < m.viewport {
		m.viewport = m.cursor
	} else if m.cursor >= m.viewport+visible {
		m.viewport = m.cursor - visible + 1
	}
}

// visibleRows returns the number of rows the viewport can display.
// When height is unknown (0), all flat rows are visible so nothing is clipped.
func (m ScopeTree) visibleRows() int {
	if m.height == 0 {
		return max(1, len(m.flat))
	}
	v := m.height - 6
	if v < 1 {
		return 1
	}
	return v
}

// View renders the scope tree step.
func (m ScopeTree) View() string {
	var sb strings.Builder

	sb.WriteString(m.theme.Title.Render(fmt.Sprintf("Choose scope for %s:", m.role.RoleName)) + "\n")

	if m.filtering {
		sb.WriteString(m.theme.Subtle.Render("Filter: ") + m.filter + "█\n")
	} else if m.filter != "" {
		sb.WriteString(m.theme.Subtle.Render("Filter: "+m.filter+"  (esc clear)") + "\n")
	} else {
		sb.WriteString("\n")
	}

	visibleRows := m.visibleRows()
	start := m.viewport
	end := start + visibleRows
	if end > len(m.flat) {
		end = len(m.flat)
	}

	for i := start; i < end; i++ {
		n := m.flat[i]
		depth := nodeDepth(n)
		indent := strings.Repeat("  ", depth)

		cursor := "  "
		if i == m.cursor {
			cursor = m.theme.TableRowSelected.Render("▸") + " "
		}

		var prefix string
		switch {
		case n.loading:
			prefix = m.spinner.View() + " "
		case n.loadErr != nil:
			prefix = "✗ "
		case n.kind == azure.ScopeResourceGroup:
			prefix = "  "
		case n.expanded:
			prefix = "▾ "
		default:
			prefix = "▸ "
		}

		check := "[ ] "
		if m.selected[n.scope] {
			check = m.theme.Active.Render("[x]") + " "
		}

		line := cursor + indent + prefix + check + n.display
		sb.WriteString(line + "\n")
		if n.loadErr != nil {
			errStyle := lipgloss.NewStyle().Foreground(m.theme.Danger)
			sb.WriteString(indent + "  " + errStyle.Render("  "+n.loadErr.Error()) + "\n")
		}
	}

	sb.WriteString("\n")
	count := len(m.selected)
	if count > 0 {
		sb.WriteString(m.theme.Subtle.Render(fmt.Sprintf("%d selected", count)) + "\n")
	}

	hints := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Enter, m.keys.Back}
	sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints,
		"h/l collapse/expand  space toggle  / filter  → confirm"))

	return sb.String()
}

func nodeDepth(n *scopeNode) int {
	d := 0
	for n.parent != nil {
		d++
		n = n.parent
	}
	return d
}
