package activate

import (
	"errors"
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
