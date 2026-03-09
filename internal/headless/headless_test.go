package headless

import (
	"testing"

	"github.com/jeircul/pim/internal/azure"
)

func TestFilterRoles(t *testing.T) {
	roles := []azure.Role{
		{RoleName: "Contributor", Scope: "/subscriptions/aaa"},
		{RoleName: "Reader", Scope: "/subscriptions/aaa"},
		{RoleName: "Owner", Scope: "/subscriptions/bbb"},
	}

	tests := []struct {
		name         string
		roleFilters  []string
		scopeFilters []string
		wantLen      int
	}{
		{"no filters returns nothing (scope required)", []string{"Reader"}, nil, 0},
		{"role+scope match", []string{"Reader"}, []string{"/subscriptions/aaa"}, 1},
		{"case-insensitive role", []string{"reader"}, []string{"/subscriptions/aaa"}, 1},
		{"substring role match", []string{"ontrib"}, []string{"/subscriptions/aaa"}, 1},
		{"no role match", []string{"NonExistent"}, []string{"/subscriptions/aaa"}, 0},
		{"multiple scopes expand targets", []string{"Reader"}, []string{"/subscriptions/aaa", "/subscriptions/bbb"}, 2},
		{"multiple roles", []string{"Reader", "Owner"}, []string{"/subscriptions/aaa"}, 2}, // cross-product: Reader×/aaa + Owner×/aaa
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterRoles(roles, tc.roleFilters, tc.scopeFilters)
			if len(got) != tc.wantLen {
				t.Errorf("filterRoles() len = %d, want %d", len(got), tc.wantLen)
			}
		})
	}
}

func TestFilterAssignments(t *testing.T) {
	assignments := []azure.ActiveAssignment{
		{RoleName: "Contributor", Scope: "/subscriptions/aaa"},
		{RoleName: "Reader", Scope: "/subscriptions/bbb"},
		{RoleName: "Owner", Scope: "/subscriptions/aaa/resourceGroups/rg1"},
	}

	tests := []struct {
		name         string
		roleFilters  []string
		scopeFilters []string
		wantLen      int
	}{
		{"no filters returns all", nil, nil, 3},
		{"role filter", []string{"Reader"}, nil, 1},
		{"scope filter", nil, []string{"/subscriptions/aaa"}, 2}, // aaa and aaa/rg1 both match substring
		{"role+scope", []string{"Owner"}, []string{"/subscriptions/aaa"}, 1},
		{"no match", []string{"NonExistent"}, nil, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filterAssignments(assignments, tc.roleFilters, tc.scopeFilters)
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
