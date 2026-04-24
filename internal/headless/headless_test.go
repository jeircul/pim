package headless

import (
	"testing"
	"time"

	"github.com/jeircul/pim/internal/azure"
)

func TestFilterRoles(t *testing.T) {
	roles := []azure.Role{
		{RoleName: "Contributor", Scope: "/subscriptions/aaa", ScopeDisplay: "My-Sub-A"},
		{RoleName: "Reader", Scope: "/subscriptions/aaa", ScopeDisplay: "My-Sub-A"},
		{RoleName: "Owner", Scope: "/subscriptions/bbb", ScopeDisplay: "My-Sub-B"},
	}

	tests := []struct {
		name         string
		roleFilters  []string
		scopeFilters []string
		wantLen      int
		wantErr      bool
	}{
		{"no scope filter activates at eligibility scope", []string{"Reader"}, nil, 1, false},
		{"role+scope ARM path match", []string{"Reader"}, []string{"/subscriptions/aaa"}, 1, false},
		{"case-insensitive role", []string{"reader"}, []string{"/subscriptions/aaa"}, 1, false},
		{"substring role match", []string{"ontrib"}, []string{"/subscriptions/aaa"}, 1, false},
		{"no role match", []string{"NonExistent"}, []string{"/subscriptions/aaa"}, 0, false},
		{"scope not under eligibility scope is rejected", []string{"Reader"}, []string{"/subscriptions/bbb"}, 0, false},
		{"multiple scopes: only valid child included", []string{"Reader"}, []string{"/subscriptions/aaa", "/subscriptions/bbb"}, 1, false},
		{"multiple roles: only scope-matching role included", []string{"Reader", "Owner"}, []string{"/subscriptions/aaa"}, 1, false},
		{"child scope of eligibility is accepted", []string{"Contributor"}, []string{"/subscriptions/aaa/resourceGroups/rg1"}, 1, false},
		{"display name match uses eligibility scope", []string{"Owner"}, []string{"My-Sub-B"}, 1, false},
		{"display name substring match", []string{"Owner"}, []string{"sub-b"}, 1, false},
		{"display name no match", []string{"Owner"}, []string{"sub-a"}, 0, false},
		{"exact role match does not match partial name", []string{"Reader"}, nil, 1, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := filterRoles(roles, tc.roleFilters, tc.scopeFilters)
			if tc.wantErr {
				if err == nil {
					t.Errorf("filterRoles() want error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("filterRoles() unexpected error: %v", err)
				return
			}
			if len(got) != tc.wantLen {
				t.Errorf("filterRoles() len = %d, want %d", len(got), tc.wantLen)
			}
		})
	}
}

func TestFilterRolesAmbiguous(t *testing.T) {
	roles := []azure.Role{
		{RoleName: "User Access Administrator", Scope: "/subscriptions/aaa", ScopeDisplay: "My-Sub-A"},
		{RoleName: "Administrator (Privileged)", Scope: "/subscriptions/aaa", ScopeDisplay: "My-Sub-A"},
	}

	_, err := filterRoles(roles, []string{"admin"}, nil)
	if err == nil {
		t.Fatal("expected ambiguity error for 'admin' matching multiple roles, got nil")
	}
}

func TestFilterRolesSingleSubstringAccepted(t *testing.T) {
	roles := []azure.Role{
		{RoleName: "Contributor", Scope: "/subscriptions/aaa", ScopeDisplay: "My-Sub-A"},
		{RoleName: "Reader", Scope: "/subscriptions/aaa", ScopeDisplay: "My-Sub-A"},
	}

	got, err := filterRoles(roles, []string{"ontrib"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 result, got %d", len(got))
	}
}

func TestFilterRolesExactNotSubstring(t *testing.T) {
	roles := []azure.Role{
		{RoleName: "Reader", Scope: "/subscriptions/aaa", ScopeDisplay: "My-Sub-A"},
		{RoleName: "Reader (privileged)", Scope: "/subscriptions/aaa", ScopeDisplay: "My-Sub-A"},
	}

	got, err := filterRoles(roles, []string{"Reader"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].role.RoleName != "Reader" {
		t.Errorf("exact match 'Reader' should only match 'Reader', got %d results", len(got))
	}
}

func TestFilterScopeAmbiguous(t *testing.T) {
	roles := []azure.Role{
		{RoleName: "Owner", Scope: "/subscriptions/aaa", ScopeDisplay: "prod-east"},
		{RoleName: "Owner", Scope: "/subscriptions/bbb", ScopeDisplay: "prod-west"},
	}

	_, err := filterRoles(roles, []string{"Owner"}, []string{"prod"})
	if err == nil {
		t.Fatal("expected ambiguity error for 'prod' matching 'prod-east' and 'prod-west', got nil")
	}
}

func TestFilterAssignments(t *testing.T) {
	assignments := []azure.ActiveAssignment{
		{RoleName: "Contributor", Scope: "/subscriptions/aaa", ScopeDisplay: "My-Sub-A"},
		{RoleName: "Reader", Scope: "/subscriptions/bbb", ScopeDisplay: "My-Sub-B"},
		{RoleName: "Owner", Scope: "/subscriptions/aaa/resourceGroups/rg1", ScopeDisplay: "rg1"},
	}

	tests := []struct {
		name         string
		roleFilters  []string
		scopeFilters []string
		wantLen      int
		wantErr      bool
	}{
		{"no filters returns all", nil, nil, 3, false},
		{"role filter", []string{"Reader"}, nil, 1, false},
		{"scope ARM path filter", nil, []string{"/subscriptions/aaa"}, 2, false},
		{"role+scope", []string{"Owner"}, []string{"/subscriptions/aaa"}, 1, false},
		{"no match", []string{"NonExistent"}, nil, 0, false},
		{"display name match", nil, []string{"My-Sub-B"}, 1, false},
		{"display name substring match", nil, []string{"sub-a"}, 1, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := filterAssignments(assignments, tc.roleFilters, tc.scopeFilters)
			if tc.wantErr {
				if err == nil {
					t.Errorf("filterAssignments() want error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("filterAssignments() unexpected error: %v", err)
				return
			}
			if len(got) != tc.wantLen {
				t.Errorf("filterAssignments() len = %d, want %d", len(got), tc.wantLen)
			}
		})
	}
}

func TestMatchesAny(t *testing.T) {
	tests := []struct {
		s       string
		filters []string
		want    bool
	}{
		{"Contributor", nil, true},
		{"Contributor", []string{"contributor"}, true},
		{"Contributor", []string{"ontrib"}, true},
		{"Contributor", []string{"Owner"}, false},
		{"Contributor", []string{"Owner", "ontrib"}, true},
		{"", []string{"x"}, false},
	}
	for _, tc := range tests {
		got := matchesAny(tc.s, tc.filters)
		if got != tc.want {
			t.Errorf("matchesAny(%q, %v) = %v, want %v", tc.s, tc.filters, got, tc.want)
		}
	}
}

func TestMatchBest(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		filters []string
		want    bool
		wantErr bool
	}{
		{"empty filters", "Contributor", nil, true, false},
		{"exact match", "Reader", []string{"Reader"}, true, false},
		{"exact case-insensitive", "Reader", []string{"reader"}, true, false},
		{"exact does not match partial (per-candidate)", "Reader (privileged)", []string{"Reader"}, true, false},
		{"single substring accepted", "Contributor", []string{"ontrib"}, true, false},
		{"no match", "Contributor", []string{"Owner"}, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := matchBest(tc.s, tc.filters)
			if tc.wantErr {
				if err == nil {
					t.Errorf("matchBest() want error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("matchBest() unexpected error: %v", err)
				return
			}
			if got != tc.want {
				t.Errorf("matchBest(%q, %v) = %v, want %v", tc.s, tc.filters, got, tc.want)
			}
		})
	}
}

func TestPartitionDeactivatable(t *testing.T) {
	exp := time.Now().Add(time.Hour).Format(time.RFC3339)
	assignments := []azure.ActiveAssignment{
		{RoleName: "direct", MemberType: "Direct", EndDateTime: exp},
		{RoleName: "inherited", MemberType: "Inherited", EndDateTime: exp},
		{RoleName: "permanent-direct", MemberType: "Direct", EndDateTime: ""},
		{RoleName: "permanent-inherited", MemberType: "Inherited", EndDateTime: ""},
	}

	deact, inh, perm := partitionDeactivatable(assignments)

	if len(deact) != 1 || deact[0].RoleName != "direct" {
		t.Errorf("deactivatable = %v, want [direct]", roleNames(deact))
	}
	if len(inh) != 2 {
		t.Errorf("inherited count = %d, want 2 (inherited + permanent-inherited)", len(inh))
	}
	if len(perm) != 1 || perm[0].RoleName != "permanent-direct" {
		t.Errorf("permanent = %v, want [permanent-direct]", roleNames(perm))
	}
}

func roleNames(a []azure.ActiveAssignment) []string {
	out := make([]string, len(a))
	for i, x := range a {
		out[i] = x.RoleName
	}
	return out
}
