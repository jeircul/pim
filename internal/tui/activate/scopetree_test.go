package activate

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/tui/styles"
)

func newTestScopeTree(loadSubs func(string) ([]azure.ManagementGroup, []azure.Subscription, error)) ScopeTree {
	role := azure.Role{
		RoleName:     "Contributor",
		Scope:        "/providers/Microsoft.Management/managementGroups/Omnia",
		ScopeDisplay: "Omnia",
	}
	theme := styles.NewTheme(true)
	keys := styles.DefaultKeyMap
	return NewScopeTree(theme, keys, role, loadSubs, nil)
}

func TestScopeTreeInitDoesNotAutoExpand(t *testing.T) {
	called := false
	st := newTestScopeTree(func(mgID string) ([]azure.ManagementGroup, []azure.Subscription, error) {
		called = true
		return nil, nil, nil
	})

	cmd := st.Init()
	if cmd == nil {
		t.Fatal("Init() returned nil; expected spinner cmd")
	}
	if called {
		t.Error("Init() triggered loadSubs; MG root should not auto-expand")
	}
}

func TestScopeTreeChildrenFlattenedWithCorrectDepth(t *testing.T) {
	st := newTestScopeTree(func(mgID string) ([]azure.ManagementGroup, []azure.Subscription, error) {
		return nil, nil, nil
	})

	msg := scopeChildrenMsg{
		parentScope: st.root.scope,
		mgs: []azure.ManagementGroup{
			{ID: "child-mg", DisplayName: "Child MG"},
		},
		subs: []azure.Subscription{
			{ID: "00000000-0000-0000-0000-000000000001", DisplayName: "My Sub"},
		},
	}
	st, _ = st.Update(msg)

	if len(st.flat) != 3 {
		t.Fatalf("expected 3 flat nodes (root + 1 MG + 1 sub), got %d", len(st.flat))
	}
	if nodeDepth(st.flat[0]) != 0 {
		t.Errorf("flat[0] (root) depth: want 0, got %d", nodeDepth(st.flat[0]))
	}
	if nodeDepth(st.flat[1]) != 1 {
		t.Errorf("flat[1] depth: want 1, got %d", nodeDepth(st.flat[1]))
	}
	if nodeDepth(st.flat[2]) != 1 {
		t.Errorf("flat[2] depth: want 1, got %d", nodeDepth(st.flat[2]))
	}
}

func TestScopeTreeRootSelectableAfterLoadErr(t *testing.T) {
	loadErr := errors.New("HTTP 403 AuthorizationFailed")
	st := newTestScopeTree(func(mgID string) ([]azure.ManagementGroup, []azure.Subscription, error) {
		return nil, nil, loadErr
	})

	msg := scopeChildrenMsg{
		parentScope: st.root.scope,
		err:         loadErr,
	}
	st, _ = st.Update(msg)

	if st.root.loadErr == nil {
		t.Fatal("expected root.loadErr to be set after failed child load")
	}

	if len(st.flat) == 0 {
		t.Fatal("flat list is empty")
	}
	st.cursor = 0
	n := st.flat[0]
	if n != st.root {
		t.Fatal("flat[0] is not root")
	}

	st2, _ := st.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if !st2.selected[st.root.scope] {
		t.Error("root scope not selectable after loadErr")
	}
}

func TestStartNextScopeTreePropagatesDimensions(t *testing.T) {
	theme := styles.NewTheme(true)
	keys := styles.DefaultKeyMap
	role := azure.Role{
		RoleName:     "Contributor",
		Scope:        "/providers/Microsoft.Management/managementGroups/Omnia",
		ScopeDisplay: "Omnia",
	}
	w := Wizard{
		theme:  theme,
		keys:   keys,
		width:  200,
		height: 50,
		deps: Deps{
			LoadSubs: func(mgID string) ([]azure.ManagementGroup, []azure.Subscription, error) {
				return nil, nil, nil
			},
			LoadRGs: func(subID string) ([]azure.ResourceGroup, error) {
				return nil, nil
			},
		},
	}
	w.scopeQueue = []azure.Role{role}

	w, _ = w.startNextScopeTree()

	if w.scopeTree.width != 200 {
		t.Errorf("scopeTree.width: want 200, got %d", w.scopeTree.width)
	}
	if w.scopeTree.height != 50 {
		t.Errorf("scopeTree.height: want 50, got %d", w.scopeTree.height)
	}
}

func TestWizardWithSizePropagates(t *testing.T) {
	theme := styles.NewTheme(true)
	keys := styles.DefaultKeyMap
	deps := Deps{
		LoadRoles:  func() ([]azure.Role, error) { return nil, nil },
		LoadActive: func() ([]azure.ActiveAssignment, error) { return nil, nil },
	}
	w := New(theme, keys, deps).WithSize(120, 40)

	if w.width != 120 || w.height != 40 {
		t.Errorf("wizard dimensions: want 120x40, got %dx%d", w.width, w.height)
	}
	if w.roleList.width != 120 || w.roleList.height != 40 {
		t.Errorf("roleList dimensions: want 120x40, got %dx%d", w.roleList.width, w.roleList.height)
	}
	if w.scopeTree.width != 120 || w.scopeTree.height != 40 {
		t.Errorf("scopeTree dimensions: want 120x40, got %dx%d", w.scopeTree.width, w.scopeTree.height)
	}
	if w.options.width != 120 || w.options.height != 40 {
		t.Errorf("options dimensions: want 120x40, got %dx%d", w.options.width, w.options.height)
	}
	if w.confirm.width != 120 || w.confirm.height != 40 {
		t.Errorf("confirm dimensions: want 120x40, got %dx%d", w.confirm.width, w.confirm.height)
	}
}

func TestScopeTreeZeroHeightRendersAllRows(t *testing.T) {
	st := newTestScopeTree(func(mgID string) ([]azure.ManagementGroup, []azure.Subscription, error) {
		return nil, nil, nil
	})

	msg := scopeChildrenMsg{
		parentScope: st.root.scope,
		mgs: []azure.ManagementGroup{
			{ID: "child-mg-1", DisplayName: "Child MG 1"},
			{ID: "child-mg-2", DisplayName: "Child MG 2"},
		},
	}
	st, _ = st.Update(msg)

	if st.height != 0 {
		t.Fatalf("precondition: height should be 0, got %d", st.height)
	}
	if len(st.flat) != 3 {
		t.Fatalf("expected 3 flat nodes, got %d", len(st.flat))
	}

	visible := st.visibleRows()
	if visible < len(st.flat) {
		t.Errorf("visibleRows() = %d with height=0, want >= %d (all rows)", visible, len(st.flat))
	}
	view := st.View()
	if !strings.Contains(view, "Child MG 1") || !strings.Contains(view, "Child MG 2") {
		t.Errorf("View() with height=0 did not render expanded children:\n%s", view)
	}
}

func TestScopeTreeFilterEscDoesNotTriggerWizardBack(t *testing.T) {
	theme := styles.NewTheme(true)
	keys := styles.DefaultKeyMap
	role := azure.Role{
		RoleName:     "Contributor",
		Scope:        "/providers/Microsoft.Management/managementGroups/Omnia",
		ScopeDisplay: "Omnia",
	}
	w := Wizard{
		theme:  theme,
		keys:   keys,
		width:  120,
		height: 40,
		step:   stepScopeTree,
		deps: Deps{
			LoadSubs: func(mgID string) ([]azure.ManagementGroup, []azure.Subscription, error) {
				return nil, nil, nil
			},
			LoadRGs: func(subID string) ([]azure.ResourceGroup, error) {
				return nil, nil
			},
		},
	}
	w.scopeTree = NewScopeTree(theme, keys, role, w.deps.LoadSubs, w.deps.LoadRGs)
	w.scopeTree.width = 120
	w.scopeTree.height = 40

	w, _ = w.Update(tea.KeyPressMsg{Text: "/"})
	if !w.scopeTree.filtering {
		t.Fatal("expected filtering=true after '/'")
	}

	var cmd tea.Cmd
	w, cmd = w.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if w.scopeTree.filtering {
		t.Error("expected filtering=false after esc")
	}
	if w.step != stepScopeTree {
		t.Errorf("expected step=stepScopeTree after esc in filter, got %d", w.step)
	}
	_ = cmd
}
