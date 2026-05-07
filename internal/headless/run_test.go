package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/jeircul/pim/internal/app"
	"github.com/jeircul/pim/internal/azure"
	"github.com/jeircul/pim/internal/state"
)

type mockClient struct {
	user          *azure.User
	userErr       error
	active        []azure.ActiveAssignment
	activeErr     error
	eligible      []azure.Role
	eligibleErr   error
	activateErr   error
	deactivateErr error
	mgSubs        map[string][]azure.Subscription
	mgSubsErr     error
	mgSubsCalls   int
	mgWarnings    map[string][]string
}

func (m *mockClient) GetCurrentUser(_ context.Context) (*azure.User, error) {
	return m.user, m.userErr
}

func (m *mockClient) GetActiveAssignments(_ context.Context) ([]azure.ActiveAssignment, error) {
	return m.active, m.activeErr
}

func (m *mockClient) GetEligibleRoles(_ context.Context) ([]azure.Role, error) {
	return m.eligible, m.eligibleErr
}

func (m *mockClient) ActivateRole(_ context.Context, role azure.Role, principalID, justification string, minutes int, targetScope string) (*azure.ScheduleResponse, error) {
	if m.activateErr != nil {
		return nil, m.activateErr
	}
	return &azure.ScheduleResponse{}, nil
}

func (m *mockClient) DeactivateRole(_ context.Context, assignment azure.ActiveAssignment, principalID string) (*azure.ScheduleResponse, error) {
	if m.deactivateErr != nil {
		return nil, m.deactivateErr
	}
	return &azure.ScheduleResponse{}, nil
}

func (m *mockClient) ListAllSubscriptionsUnderMG(_ context.Context, mgID string) ([]azure.Subscription, []string, error) {
	m.mgSubsCalls++
	if m.mgSubsErr != nil {
		return nil, nil, m.mgSubsErr
	}
	var warnings []string
	if m.mgWarnings != nil {
		warnings = m.mgWarnings[mgID]
	}
	return m.mgSubs[mgID], warnings, nil
}

func newTestApp(t *testing.T, cfg app.Config) *app.App {
	t.Helper()
	store, err := state.New(t.TempDir())
	if err != nil {
		t.Fatalf("state.New: %v", err)
	}
	return &app.App{Store: store, Config: cfg}
}

func captureOutput(t *testing.T, fn func(w io.Writer) error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	fnErr := fn(w)
	w.Close()
	out, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return string(out), fnErr
}

func TestRunActivate(t *testing.T) {
	user := &azure.User{ID: "uid-1", UserPrincipalName: "user@example.com"}
	eligibleRole := azure.Role{
		RoleName:              "Contributor",
		Scope:                 "/subscriptions/sub-1",
		RoleDefinitionID:      "rd-1",
		EligibilityScheduleID: "/sched/1",
	}

	tests := []struct {
		name    string
		cfg     app.Config
		client  *mockClient
		wantErr string
		wantOut string
	}{
		{
			name: "missing required flags no role",
			cfg: app.Config{
				Command:       app.CmdActivate,
				Scopes:        []string{"/subscriptions/sub-1"},
				TimeStr:       "1h",
				Justification: "need access",
			},
			client:  &mockClient{user: user},
			wantErr: "requires",
		},
		{
			name: "no matching eligible roles",
			cfg: app.Config{
				Command:       app.CmdActivate,
				Roles:         []string{"Owner"},
				Scopes:        []string{"/subscriptions/sub-1"},
				TimeStr:       "1h",
				Justification: "need access",
			},
			client: &mockClient{
				user:     user,
				eligible: []azure.Role{eligibleRole},
			},
			wantErr: "no eligible roles",
		},
		{
			name: "successful activation",
			cfg: app.Config{
				Command:       app.CmdActivate,
				Roles:         []string{"Contributor"},
				Scopes:        []string{"/subscriptions/sub-1"},
				TimeStr:       "1h",
				Justification: "need access",
			},
			client: &mockClient{
				user:     user,
				eligible: []azure.Role{eligibleRole},
			},
			wantOut: "Activated:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp(t, tc.cfg)
			out, err := captureOutput(t, func(w io.Writer) error {
				return runActivate(context.Background(), a, tc.client, user, w)
			})
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantOut != "" && !strings.Contains(out, tc.wantOut) {
				t.Errorf("output %q does not contain %q", out, tc.wantOut)
			}
		})
	}
}

func TestRunDeactivate(t *testing.T) {
	user := &azure.User{ID: "uid-1"}
	activeAssignment := azure.ActiveAssignment{
		RoleName:         "Contributor",
		Scope:            "/subscriptions/sub-1",
		ScopeDisplay:     "My Sub",
		RoleDefinitionID: "rd-1",
		EndDateTime:      "2099-01-01T00:00:00Z",
	}

	tests := []struct {
		name    string
		cfg     app.Config
		client  *mockClient
		wantErr string
		wantOut string
	}{
		{
			name: "no filters no yes flag",
			cfg: app.Config{
				Command: app.CmdDeactivate,
			},
			client:  &mockClient{user: user, active: []azure.ActiveAssignment{activeAssignment}},
			wantErr: "--headless deactivate requires",
		},
		{
			name: "no matching active assignments with filter returns error",
			cfg: app.Config{
				Command: app.CmdDeactivate,
				Roles:   []string{"Owner"},
			},
			client: &mockClient{
				user:   user,
				active: []azure.ActiveAssignment{activeAssignment},
			},
			wantErr: "no active assignments match",
		},
		{
			name: "successful deactivation",
			cfg: app.Config{
				Command: app.CmdDeactivate,
				Roles:   []string{"Contributor"},
			},
			client: &mockClient{
				user:   user,
				active: []azure.ActiveAssignment{activeAssignment},
			},
			wantOut: "Deactivated:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp(t, tc.cfg)
			out, err := captureOutput(t, func(w io.Writer) error {
				return runDeactivate(context.Background(), a, tc.client, user, w)
			})
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("want error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantOut != "" && !strings.Contains(out, tc.wantOut) {
				t.Errorf("output %q does not contain %q", out, tc.wantOut)
			}
		})
	}
}

func TestRunDeactivateYesFlag(t *testing.T) {
	user := &azure.User{ID: "uid-1"}
	activeAssignment := azure.ActiveAssignment{
		RoleName:         "Contributor",
		Scope:            "/subscriptions/sub-1",
		ScopeDisplay:     "My Sub",
		RoleDefinitionID: "rd-1",
		EndDateTime:      "2099-01-01T00:00:00Z",
	}

	a := newTestApp(t, app.Config{
		Command: app.CmdDeactivate,
		Yes:     true,
	})
	client := &mockClient{
		user:   user,
		active: []azure.ActiveAssignment{activeAssignment},
	}

	out, err := captureOutput(t, func(w io.Writer) error {
		return runDeactivate(context.Background(), a, client, user, w)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Deactivated:") {
		t.Errorf("output %q does not contain %q", out, "Deactivated:")
	}
}

func TestRunDeactivateClientError(t *testing.T) {
	user := &azure.User{ID: "uid-1"}
	a := newTestApp(t, app.Config{
		Command: app.CmdDeactivate,
		Roles:   []string{"Contributor"},
	})
	client := &mockClient{
		user:      user,
		activeErr: fmt.Errorf("network error"),
	}

	_, err := captureOutput(t, func(w io.Writer) error {
		return runDeactivate(context.Background(), a, client, user, w)
	})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "get active assignments") {
		t.Errorf("error %q does not contain %q", err.Error(), "get active assignments")
	}
}

func TestRunStatus(t *testing.T) {
	user := &azure.User{ID: "uid-1"}
	activeAssignment := azure.ActiveAssignment{
		RoleName:     "Contributor",
		Scope:        "/subscriptions/sub-1",
		ScopeDisplay: "My Sub",
		EndDateTime:  "",
	}

	tests := []struct {
		name    string
		cfg     app.Config
		client  *mockClient
		wantOut string
	}{
		{
			name: "no active assignments",
			cfg:  app.Config{Command: app.CmdStatus},
			client: &mockClient{
				user:   user,
				active: nil,
			},
			wantOut: "No active PIM",
		},
		{
			name: "json output",
			cfg: app.Config{
				Command: app.CmdStatus,
				Output:  app.OutputJSON,
			},
			client: &mockClient{
				user:   user,
				active: []azure.ActiveAssignment{activeAssignment},
			},
			wantOut: "[",
		},
		{
			name: "table output contains ROLE header",
			cfg: app.Config{
				Command: app.CmdStatus,
				Output:  app.OutputTable,
			},
			client: &mockClient{
				user:   user,
				active: []azure.ActiveAssignment{activeAssignment},
			},
			wantOut: "ROLE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := newTestApp(t, tc.cfg)
			out, err := captureOutput(t, func(w io.Writer) error {
				return runStatus(context.Background(), a, tc.client, user, w)
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantOut != "" && !strings.Contains(out, tc.wantOut) {
				t.Errorf("output %q does not contain %q", out, tc.wantOut)
			}
		})
	}
}

func TestRunStatusJSONValid(t *testing.T) {
	user := &azure.User{ID: "uid-1"}
	a := newTestApp(t, app.Config{Command: app.CmdStatus, Output: app.OutputJSON})
	client := &mockClient{
		user: user,
		active: []azure.ActiveAssignment{
			{RoleName: "Owner", Scope: "/subscriptions/sub-1", ScopeDisplay: "Sub 1"},
		},
	}

	out, err := captureOutput(t, func(w io.Writer) error {
		return runStatus(context.Background(), a, client, user, w)
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result []azure.ActiveAssignment
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Errorf("output is not valid JSON array: %v\noutput: %s", err, out)
	}
	if len(result) != 1 {
		t.Errorf("want 1 element, got %d", len(result))
	}
}

func TestFilterRolesBareGUID(t *testing.T) {
	const guid = "00000000-0000-0000-0000-000000000000"
	roles := []azure.Role{
		{RoleName: "Owner", Scope: "/subscriptions/" + guid, ScopeDisplay: "My Sub"},
		{RoleName: "Owner", Scope: "/subscriptions/other-sub", ScopeDisplay: "Other Sub"},
	}

	targets, err := filterRoles(context.Background(), &mockClient{}, roles, []string{"Owner"}, []string{guid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("want 1 target, got %d", len(targets))
	}
	wantScope := "/subscriptions/" + guid
	if targets[0].scope != wantScope {
		t.Errorf("target scope = %q; want %q", targets[0].scope, wantScope)
	}
	if targets[0].role.Scope != "/subscriptions/"+guid {
		t.Errorf("role scope = %q; want /subscriptions/%s", targets[0].role.Scope, guid)
	}
}

func TestFilterAssignmentsBareGUID(t *testing.T) {
	const guid = "00000000-0000-0000-0000-000000000000"
	assignments := []azure.ActiveAssignment{
		{RoleName: "Owner", Scope: "/subscriptions/" + guid, ScopeDisplay: "My Sub"},
		{RoleName: "Owner", Scope: "/subscriptions/other-sub", ScopeDisplay: "Other Sub"},
	}

	out, err := filterAssignments(assignments, []string{"Owner"}, []string{guid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 assignment, got %d", len(out))
	}
	if out[0].Scope != "/subscriptions/"+guid {
		t.Errorf("assignment scope = %q; want /subscriptions/%s", out[0].Scope, guid)
	}
}

const mgScope = "/providers/Microsoft.Management/managementGroups/mg-root"
const childGUID = "aaaaaaaa-0000-0000-0000-000000000001"

func mgRole() azure.Role {
	return azure.Role{
		RoleName:              "Owner",
		Scope:                 mgScope,
		ScopeDisplay:          "mg-root",
		RoleDefinitionID:      "rd-mg",
		EligibilityScheduleID: "/sched/mg",
	}
}

func TestFilterRolesMGInheritedGUID(t *testing.T) {
	roles := []azure.Role{mgRole()}
	mc := &mockClient{
		mgSubs: map[string][]azure.Subscription{
			"mg-root": {{ID: childGUID}},
		},
	}

	targets, err := filterRoles(context.Background(), mc, roles, []string{"Owner"}, []string{childGUID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("want 1 target, got %d", len(targets))
	}
	if targets[0].scope != "/subscriptions/"+childGUID {
		t.Errorf("scope = %q; want /subscriptions/%s", targets[0].scope, childGUID)
	}
	if targets[0].role.Scope != mgScope {
		t.Errorf("role.Scope = %q; want %q", targets[0].role.Scope, mgScope)
	}
}

func TestFilterRolesMGInheritedGUIDSlashPrefix(t *testing.T) {
	roles := []azure.Role{mgRole()}
	mc := &mockClient{
		mgSubs: map[string][]azure.Subscription{
			"mg-root": {{ID: childGUID}},
		},
	}

	targets, err := filterRoles(context.Background(), mc, roles, []string{"Owner"}, []string{"/subscriptions/" + childGUID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("want 1 target, got %d", len(targets))
	}
	if targets[0].scope != "/subscriptions/"+childGUID {
		t.Errorf("scope = %q; want /subscriptions/%s", targets[0].scope, childGUID)
	}
}

func TestFilterRolesMGInheritedGUIDTrailingSlash(t *testing.T) {
	roles := []azure.Role{mgRole()}
	mc := &mockClient{
		mgSubs: map[string][]azure.Subscription{
			"mg-root": {{ID: childGUID}},
		},
	}

	targets, err := filterRoles(context.Background(), mc, roles, []string{"Owner"}, []string{"/subscriptions/" + childGUID + "/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("want 1 target, got %d", len(targets))
	}
	if targets[0].scope != "/subscriptions/"+childGUID {
		t.Errorf("scope = %q; want /subscriptions/%s", targets[0].scope, childGUID)
	}
}

func TestFilterRolesMGGUIDNotChild(t *testing.T) {
	roles := []azure.Role{mgRole()}
	mc := &mockClient{
		mgSubs: map[string][]azure.Subscription{
			"mg-root": {{ID: "bbbbbbbb-0000-0000-0000-000000000002"}},
		},
	}

	targets, err := filterRoles(context.Background(), mc, roles, []string{"Owner"}, []string{childGUID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 0 {
		t.Errorf("want 0 targets, got %d", len(targets))
	}
}

func TestFilterRolesMGFanoutCachePerMG(t *testing.T) {
	const guid2 = "cccccccc-0000-0000-0000-000000000003"
	roles := []azure.Role{
		mgRole(),
		{
			RoleName:              "Contributor",
			Scope:                 mgScope,
			ScopeDisplay:          "mg-root",
			RoleDefinitionID:      "rd-mg2",
			EligibilityScheduleID: "/sched/mg2",
		},
	}
	mc := &mockClient{
		mgSubs: map[string][]azure.Subscription{
			"mg-root": {{ID: childGUID}, {ID: guid2}},
		},
	}

	_, err := filterRoles(context.Background(), mc, roles, []string{"Owner", "Contributor"}, []string{childGUID, guid2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mc.mgSubsCalls != 1 {
		t.Errorf("mgSubsCalls = %d; want 1", mc.mgSubsCalls)
	}
}

func TestFilterRolesMGFanoutErrorWraps(t *testing.T) {
	roles := []azure.Role{mgRole()}
	mc := &mockClient{
		mgSubsErr: fmt.Errorf("api unavailable"),
	}

	_, err := filterRoles(context.Background(), mc, roles, []string{"Owner"}, []string{childGUID})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "list subscriptions under management group") {
		t.Errorf("error %q does not contain expected prefix", err.Error())
	}
}

func TestFilterRolesDirectSubGUIDNoExtraCall(t *testing.T) {
	const guid = "dddddddd-0000-0000-0000-000000000004"
	roles := []azure.Role{
		{
			RoleName:              "Owner",
			Scope:                 "/subscriptions/" + guid,
			ScopeDisplay:          "Direct Sub",
			RoleDefinitionID:      "rd-direct",
			EligibilityScheduleID: "/sched/direct",
		},
	}
	mc := &mockClient{}

	targets, err := filterRoles(context.Background(), mc, roles, []string{"Owner"}, []string{guid})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("want 1 target, got %d", len(targets))
	}
	if mc.mgSubsCalls != 0 {
		t.Errorf("mgSubsCalls = %d; want 0", mc.mgSubsCalls)
	}
}

func TestFilterRolesMGFanoutSecondScopeDisplayFallback(t *testing.T) {
	// First scope filter: bare GUID that matches an MG child (produces 1 target via MG fan-out).
	// Second scope filter: display-name string that matches a direct-sub role.
	// Bug: old code checked `len(out) > 0` after MG fan-out, so the second filter
	// skipped display-name fallback because out was already non-empty from the first.
	const mgChildGUID = "eeeeeeee-0000-0000-0000-000000000005"
	const directSubScope = "/subscriptions/ffffffff-0000-0000-0000-000000000006"
	const mgScopeLocal = "/providers/Microsoft.Management/managementGroups/mg-local"

	roles := []azure.Role{
		{
			RoleName:              "Owner",
			Scope:                 mgScopeLocal,
			ScopeDisplay:          "mg-local",
			RoleDefinitionID:      "rd-mg",
			EligibilityScheduleID: "/sched/mg",
		},
		{
			RoleName:              "Owner",
			Scope:                 directSubScope,
			ScopeDisplay:          "Direct Production",
			RoleDefinitionID:      "rd-direct",
			EligibilityScheduleID: "/sched/direct",
		},
	}
	mc := &mockClient{
		mgSubs: map[string][]azure.Subscription{
			"mg-local": {{ID: mgChildGUID, DisplayName: "MG Child Sub"}},
		},
	}

	targets, err := filterRoles(context.Background(), mc, roles,
		[]string{"Owner"},
		[]string{mgChildGUID, "Direct Production"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("want 2 targets, got %d: %+v", len(targets), targets)
	}
}
