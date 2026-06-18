package activate

import (
	"testing"

	"github.com/jeircul/pim/internal/azure"
)

func TestAutoAdvance(t *testing.T) {
	roleA := azure.Role{RoleName: "Contributor", Scope: "/subscriptions/sub-1", ScopeDisplay: "Sub One"}
	roleB := azure.Role{RoleName: "Contributor", Scope: "/subscriptions/sub-2", ScopeDisplay: "Sub Two"}
	roleC := azure.Role{RoleName: "Reader", Scope: "/subscriptions/sub-1", ScopeDisplay: "Sub One"}
	roleD := azure.Role{RoleName: "Contributor", Scope: "/subscriptions/00000000-0000-0000-0000-000000000000", ScopeDisplay: "GUID Sub"}

	tests := []struct {
		name        string
		roles       []azure.Role
		roleFilter  []string
		scopeFilter []string
		scheduleID  string
		wantNil     bool
		wantRole    azure.Role
	}{
		{
			name:    "no roleFilter returns nil",
			roles:   []azure.Role{roleA},
			wantNil: true,
		},
		{
			name:       "roleFilter matches exactly one role emits msg",
			roles:      []azure.Role{roleA, roleC},
			roleFilter: []string{"Contributor"},
			wantNil:    false,
			wantRole:   roleA,
		},
		{
			name:       "roleFilter matches two roles no scopeFilter returns nil",
			roles:      []azure.Role{roleA, roleB},
			roleFilter: []string{"Contributor"},
			wantNil:    true,
		},
		{
			name:        "roleFilter matches two roles scopeFilter matches one by ARM path emits correct role",
			roles:       []azure.Role{roleA, roleB},
			roleFilter:  []string{"Contributor"},
			scopeFilter: []string{"/subscriptions/sub-1"},
			wantNil:     false,
			wantRole:    roleA,
		},
		{
			name:        "roleFilter matches two roles scopeFilter matches one by display name substring emits correct role",
			roles:       []azure.Role{roleA, roleB},
			roleFilter:  []string{"Contributor"},
			scopeFilter: []string{"Sub Two"},
			wantNil:     false,
			wantRole:    roleB,
		},
		{
			name:        "roleFilter matches two roles scopeFilter matches both returns nil",
			roles:       []azure.Role{roleA, roleB},
			roleFilter:  []string{"Contributor"},
			scopeFilter: []string{"sub"},
			wantNil:     true,
		},
		{
			name:        "roleFilter matches two roles scopeFilter matches none returns nil",
			roles:       []azure.Role{roleA, roleB},
			roleFilter:  []string{"Contributor"},
			scopeFilter: []string{"nonexistent"},
			wantNil:     true,
		},
		{
			name:        "roleFilter matches two roles scopeFilter bare GUID emits correct role",
			roles:       []azure.Role{roleA, roleD},
			roleFilter:  []string{"Contributor"},
			scopeFilter: []string{"00000000-0000-0000-0000-000000000000"},
			wantNil:     false,
			wantRole:    roleD,
		},
		{
			name: "two MG-scoped matches with bare sub GUID returns nil — cannot resolve owning MG without API",
			roles: []azure.Role{
				{RoleName: "Contributor", Scope: "/providers/Microsoft.Management/managementGroups/mg-a"},
				{RoleName: "Contributor", Scope: "/providers/Microsoft.Management/managementGroups/mg-b"},
			},
			roleFilter:  []string{"Contributor"},
			scopeFilter: []string{"00000000-0000-0000-0000-000000000000"},
			wantNil:     true,
		},
		{
			name: "one MG-scoped match with bare sub GUID emits the role",
			roles: []azure.Role{
				{RoleName: "Contributor", Scope: "/providers/Microsoft.Management/managementGroups/mg-a"},
				{RoleName: "Reader", Scope: "/providers/Microsoft.Management/managementGroups/mg-b"},
			},
			roleFilter:  []string{"Contributor"},
			scopeFilter: []string{"00000000-0000-0000-0000-000000000000"},
			wantNil:     false,
			wantRole:    azure.Role{RoleName: "Contributor", Scope: "/providers/Microsoft.Management/managementGroups/mg-a"},
		},
		{
			name: "scheduleID exact match selects correct role among same-name same-MG duplicates",
			roles: []azure.Role{
				{RoleName: "Reader", Scope: "/providers/Microsoft.Management/managementGroups/mg-a", EligibilityScheduleID: "/providers/Microsoft.Management/managementGroups/mg-a/providers/Microsoft.Authorization/roleEligibilitySchedules/aaaaaaaa-0000-0000-0000-000000000000"},
				{RoleName: "Reader", Scope: "/providers/Microsoft.Management/managementGroups/mg-a", EligibilityScheduleID: "/providers/Microsoft.Management/managementGroups/mg-a/providers/Microsoft.Authorization/roleEligibilitySchedules/bbbbbbbb-0000-0000-0000-000000000000"},
			},
			roleFilter:  []string{"Reader"},
			scopeFilter: []string{"/subscriptions/00000000-0000-0000-0000-000000000000"},
			scheduleID:  "/providers/Microsoft.Management/managementGroups/mg-a/providers/Microsoft.Authorization/roleEligibilitySchedules/bbbbbbbb-0000-0000-0000-000000000000",
			wantNil:     false,
			wantRole:    azure.Role{RoleName: "Reader", Scope: "/providers/Microsoft.Management/managementGroups/mg-a", EligibilityScheduleID: "/providers/Microsoft.Management/managementGroups/mg-a/providers/Microsoft.Authorization/roleEligibilitySchedules/bbbbbbbb-0000-0000-0000-000000000000"},
		},
		{
			name: "scheduleID not found falls through to heuristic single-match",
			roles: []azure.Role{
				{RoleName: "Reader", Scope: "/providers/Microsoft.Management/managementGroups/mg-a", EligibilityScheduleID: "/providers/Microsoft.Management/managementGroups/mg-a/providers/Microsoft.Authorization/roleEligibilitySchedules/aaaaaaaa-0000-0000-0000-000000000000"},
			},
			roleFilter:  []string{"Reader"},
			scopeFilter: []string{"/subscriptions/00000000-0000-0000-0000-000000000000"},
			scheduleID:  "/providers/Microsoft.Management/managementGroups/mg-a/providers/Microsoft.Authorization/roleEligibilitySchedules/cccccccc-0000-0000-0000-000000000000",
			// scheduleID not found → falls through to roleFilter heuristic → single match emits
			wantNil:  false,
			wantRole: azure.Role{RoleName: "Reader", Scope: "/providers/Microsoft.Management/managementGroups/mg-a"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			visible := make([]int, len(tc.roles))
			for i := range tc.roles {
				visible[i] = i
			}
			m := &RoleList{
				roles:       tc.roles,
				visible:     visible,
				roleFilter:  tc.roleFilter,
				scopeFilter: tc.scopeFilter,
				scheduleID:  tc.scheduleID,
			}
			cmd := m.autoAdvance()
			if tc.wantNil {
				if cmd != nil {
					t.Errorf("expected nil cmd, got non-nil")
				}
				return
			}
			if cmd == nil {
				t.Fatalf("expected non-nil cmd, got nil")
			}
			msg := cmd()
			done, ok := msg.(RoleListDoneMsg)
			if !ok {
				t.Fatalf("expected RoleListDoneMsg, got %T", msg)
			}
			if len(done.Selected) != 1 {
				t.Fatalf("expected 1 selected role, got %d", len(done.Selected))
			}
			got := done.Selected[0]
			if got.RoleName != tc.wantRole.RoleName || got.Scope != tc.wantRole.Scope ||
				(tc.wantRole.EligibilityScheduleID != "" && got.EligibilityScheduleID != tc.wantRole.EligibilityScheduleID) {
				t.Errorf("selected role = {%s %s %s}, want {%s %s %s}",
					got.RoleName, got.Scope, got.EligibilityScheduleID,
					tc.wantRole.RoleName, tc.wantRole.Scope, tc.wantRole.EligibilityScheduleID)
			}
		})
	}
}
