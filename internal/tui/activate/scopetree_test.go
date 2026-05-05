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
