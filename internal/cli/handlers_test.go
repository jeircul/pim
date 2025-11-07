package cli

import (
	"testing"

	"github.com/jeircul/pim/pkg/azpim"
)

func TestFilterEligibleRoles(t *testing.T) {
	roles := []azpim.Role{
		{
			Scope:        "/providers/Microsoft.Management/managementGroups/root",
			ScopeDisplay: "Tenant Root Group",
			RoleName:     "Owner",
		},
		{
			Scope:        "/subscriptions/12345678-1234-1234-1234-123456789000",
			ScopeDisplay: "Platform Hub",
			RoleName:     "Contributor",
		},
		{
			Scope:        "/subscriptions/abcd-0000-0000-0000-abcdefabcdef/resourceGroups/core-rg",
			ScopeDisplay: "core-rg",
			RoleName:     "Reader",
		},
	}

	tests := []struct {
		name     string
		cfg      ActivateConfig
		expected int
	}{
		{
			name:     "no filters returns all",
			cfg:      ActivateConfig{},
			expected: len(roles),
		},
		{
			name: "management group filter",
			cfg: ActivateConfig{
				ManagementGroups: []string{"root"},
			},
			expected: 1,
		},
		{
			name: "subscription filter",
			cfg: ActivateConfig{
				Subscriptions: []string{"12345678"},
			},
			expected: 2,
		},
		{
			name: "subscription filter preserves management group role",
			cfg: ActivateConfig{
				Subscriptions: []string{"does-not-match"},
			},
			expected: 1,
		},
		{
			name: "subscription filter matches nested scope",
			cfg: ActivateConfig{
				Subscriptions: []string{"abcd-0000"},
			},
			expected: 2,
		},
		{
			name: "role filter",
			cfg: ActivateConfig{
				Roles: []string{"reader"},
			},
			expected: 1,
		},
		{
			name: "scope contains filter",
			cfg: ActivateConfig{
				ScopeContains: []string{"resourcegroups"},
			},
			expected: 1,
		},
		{
			name: "resource group filter",
			cfg: ActivateConfig{
				ResourceGroups: []string{"core"},
			},
			expected: 2,
		},
		{
			name: "combined filters",
			cfg: ActivateConfig{
				Subscriptions: []string{"abcd"},
				Roles:         []string{"reader"},
			},
			expected: 1,
		},
		{
			name: "filters exclude all",
			cfg: ActivateConfig{
				ManagementGroups: []string{"does-not-exist"},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := filterEligibleRoles(roles, tt.cfg)
			if len(filtered) != tt.expected {
				t.Fatalf("expected %d roles, got %d", tt.expected, len(filtered))
			}
		})
	}
}
