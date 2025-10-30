package cli

import (
	"context"
	"fmt"

	"github.com/jeircul/pim/pkg/azpim"
)

// HandleDeactivation processes the deactivation flow
func HandleDeactivation(ctx context.Context, client *azpim.Client, principalID string) error {
	assignments, err := client.GetActiveAssignments(principalID)
	if err != nil {
		return fmt.Errorf("get active assignments: %w", err)
	}

	if len(assignments) == 0 {
		fmt.Println("No active assignments found.")
		return nil
	}

	fmt.Println("\nActive assignments:")
	chosen, err := PromptSelection(assignments,
		func(i int, a azpim.ActiveAssignment) string {
			return fmt.Sprintf("  %2d) %s @ %s", i, a.RoleName, a.ScopeDisplay)
		},
		"Select assignment to deactivate")
	if err != nil {
		return fmt.Errorf("selection: %w", err)
	}

	resp, err := client.DeactivateRole(chosen, principalID)
	if err != nil {
		return fmt.Errorf("deactivate role: %w", err)
	}

	fmt.Printf("✓ Deactivation successful (status: %s)\n", resp.Properties.Status)
	return nil
}

// HandleStatus shows active assignments with expiry times
func HandleStatus(ctx context.Context, client *azpim.Client, principalID string) error {
	assignments, err := client.GetActiveAssignments(principalID)
	if err != nil {
		return fmt.Errorf("get active assignments: %w", err)
	}

	if len(assignments) == 0 {
		fmt.Println("No active assignments found.")
		return nil
	}

	fmt.Printf("\nActive assignments (%d total):\n", len(assignments))
	for i, a := range assignments {
		fmt.Printf("  %2d) %s @ %s (%s)\n", i+1, a.RoleName, a.ScopeDisplay, a.ExpiryDisplay())
	}
	return nil
}

// HandleActivation processes the activation flow
func HandleActivation(ctx context.Context, client *azpim.Client, principalID, justification string, hours int) error {
	roles, err := client.GetEligibleRoles()
	if err != nil {
		return fmt.Errorf("get eligible roles: %w", err)
	}

	if len(roles) == 0 {
		return fmt.Errorf("no eligible PIM roles found")
	}

	fmt.Println("\nEligible roles:")
	chosen, err := PromptSelection(roles,
		func(i int, r azpim.Role) string {
			return fmt.Sprintf("  %2d) %s @ %s", i, r.RoleName, r.ScopeDisplay)
		},
		"Select role to activate")
	if err != nil {
		return fmt.Errorf("selection: %w", err)
	}

	resp, err := client.ActivateRole(chosen, principalID, justification, hours)
	if err != nil {
		return fmt.Errorf("activate role: %w", err)
	}

	fmt.Printf("✓ Activation successful for %d hour(s) (status: %s)\n", hours, resp.Properties.Status)
	return nil
}
