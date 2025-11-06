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

	assignments = filterTemporary(assignments)

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

func filterTemporary(assignments []azpim.ActiveAssignment) []azpim.ActiveAssignment {
	if len(assignments) == 0 {
		return assignments
	}
	filtered := make([]azpim.ActiveAssignment, 0, len(assignments))
	for _, a := range assignments {
		if !a.IsPermanent() {
			filtered = append(filtered, a)
		}
	}
	return filtered
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

	var temporary []azpim.ActiveAssignment
	var permanent []azpim.ActiveAssignment
	for _, a := range assignments {
		if a.IsPermanent() {
			permanent = append(permanent, a)
		} else {
			temporary = append(temporary, a)
		}
	}

	index := 1
	if len(temporary) > 0 {
		fmt.Printf("\nTemporary elevations (%d):\n", len(temporary))
		for _, a := range temporary {
			fmt.Printf("  %2d) %s @ %s (%s)\n", index, a.RoleName, a.ScopeDisplay, a.ExpiryDisplay())
			index++
		}
	}

	if len(permanent) > 0 {
		fmt.Printf("\nPermanent assignments (%d):\n", len(permanent))
		for _, a := range permanent {
			fmt.Printf("  %2d) %s @ %s (no expiry – admin managed)\n", index, a.RoleName, a.ScopeDisplay)
			index++
		}
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
