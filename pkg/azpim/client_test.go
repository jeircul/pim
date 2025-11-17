package azpim

import (
	"testing"
)

func TestClampMinutes(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"Below minimum", 0, MinMinutes},
		{"At minimum", 30, 30},
		{"Round down", 40, 30},
		{"Round up", 50, 60},
		{"Normal value", 120, 120},
		{"At maximum", 480, 480},
		{"Above maximum", 600, MaxMinutes},
		{"Negative value", -5, MinMinutes},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := clampMinutes(tt.input)
			if result != tt.expected {
				t.Errorf("clampMinutes(%d) = %d; want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRoleStruct(t *testing.T) {
	role := Role{
		Scope:            "/subscriptions/test-sub",
		ScopeDisplay:     "Test Subscription",
		RoleName:         "Owner",
		RoleDefinitionID: "/providers/Microsoft.Authorization/roleDefinitions/test-role-id",
	}

	if role.Scope == "" {
		t.Error("Role.Scope should not be empty")
	}
	if role.RoleName != "Owner" {
		t.Errorf("Expected RoleName 'Owner', got '%s'", role.RoleName)
	}
}

func TestUserStruct(t *testing.T) {
	user := User{
		ID:                "test-user-id",
		UserPrincipalName: "test@example.com",
		DisplayName:       "Test User",
	}

	if user.ID == "" {
		t.Error("User.ID should not be empty")
	}
	if user.DisplayName != "Test User" {
		t.Errorf("Expected DisplayName 'Test User', got '%s'", user.DisplayName)
	}
}

func TestActiveAssignmentStruct(t *testing.T) {
	assignment := ActiveAssignment{
		Name:             "test-assignment",
		Scope:            "/subscriptions/test",
		ScopeDisplay:     "Test Scope",
		RoleName:         "Contributor",
		RoleDefinitionID: "/providers/test-role",
	}

	if assignment.Name == "" {
		t.Error("ActiveAssignment.Name should not be empty")
	}
	if assignment.RoleName != "Contributor" {
		t.Errorf("Expected RoleName 'Contributor', got '%s'", assignment.RoleName)
	}
}
