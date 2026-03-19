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

// ScopeTreeDoneMsg is sent when the user confirms a scope selection for one role.
type ScopeTreeDoneMsg struct {
	Role        azure.Role
	TargetScope string // empty = use role's own scope
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
	parent   *scopeNode
}

// ScopeTree is Step 2: lazy-loaded scope tree for MG-scoped roles.
type ScopeTree struct {
	theme    styles.Theme
	keys     styles.KeyMap
	spinner  components.Spinner
	role     azure.Role
	root     *scopeNode
	flat     []*scopeNode // flattened visible list
	cursor   int
	selected string // scope path of selected node
	width    int
	height   int
	// loadSubs fetches subscriptions under a management group ID.
	loadSubs func(mgID string) ([]azure.Subscription, error)
	// loadRGs fetches resource groups under a subscription ID.
	loadRGs func(subID string) ([]azure.ResourceGroup, error)
}

type scopeChildrenMsg struct {
	parentScope string
	subs        []azure.Subscription
	rgs         []azure.ResourceGroup
	err         error
}

// NewScopeTree creates a ScopeTree for the given MG-scoped role.
func NewScopeTree(
	theme styles.Theme,
	keys styles.KeyMap,
	role azure.Role,
	loadSubs func(string) ([]azure.Subscription, error),
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
		loadSubs: loadSubs,
		loadRGs:  loadRGs,
	}
	st.flatten()
	return st
}

// Init starts the spinner and triggers root expansion.
func (m ScopeTree) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Init(),
		m.expandNode(m.root),
	)
}

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
			subs, err := m.loadSubs(mgID)
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
		n.loaded = true
		n.expanded = true
		if msg.err == nil {
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
		}
		m.flatten()

	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.keys.Up), msg.String() == "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keys.Down), msg.String() == "j":
			if m.cursor < len(m.flat)-1 {
				m.cursor++
			}
		case msg.String() == "l", msg.String() == "right":
			// expand
			if m.cursor < len(m.flat) {
				n := m.flat[m.cursor]
				if n.kind != azure.ScopeResourceGroup && !n.expanded {
					cmd := m.expandNode(n)
					m.flatten()
					return m, tea.Batch(m.spinner.Init(), cmd)
				}
			}
		case msg.String() == "h", msg.String() == "left":
			// collapse
			if m.cursor < len(m.flat) {
				n := m.flat[m.cursor]
				if n.expanded {
					n.expanded = false
					m.flatten()
				} else if n.parent != nil {
					// jump to parent
					for i, fn := range m.flat {
						if fn == n.parent {
							m.cursor = i
							break
						}
					}
				}
			}
		case msg.String() == " ":
			if m.cursor < len(m.flat) {
				m.selected = m.flat[m.cursor].scope
			}
		case key.Matches(msg, m.keys.Enter):
			if m.selected != "" {
				scope := m.selected
				role := m.role
				return m, func() tea.Msg { return ScopeTreeDoneMsg{Role: role, TargetScope: scope} }
			}
		case key.Matches(msg, m.keys.Back), msg.String() == "esc":
			// wizard handles Back
		}

	default:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// flatten rebuilds the visible flat list from the tree.
func (m *ScopeTree) flatten() {
	m.flat = m.flat[:0]
	m.flattenNode(m.root)
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

// View renders the scope tree step.
func (m ScopeTree) View() string {
	var sb strings.Builder

	sb.WriteString(m.theme.Title.Render(fmt.Sprintf("Choose scope for %s:", m.role.RoleName)) + "\n\n")

	for i, n := range m.flat {
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
		case n.kind == azure.ScopeResourceGroup:
			prefix = "  "
		case n.expanded:
			prefix = "▾ "
		default:
			prefix = "▸ "
		}

		check := "    "
		if m.selected == n.scope {
			check = m.theme.Active.Render("[x]") + " "
		}

		line := cursor + indent + prefix + check + n.display
		sb.WriteString(line + "\n")
	}

	sb.WriteString("\n")
	if m.selected != "" {
		sb.WriteString(m.theme.Subtle.Render("scope: "+m.selected) + "\n")
	}

	hints := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Enter, m.keys.Back}
	sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints,
		"h/l collapse/expand  space select  → confirm"))

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
