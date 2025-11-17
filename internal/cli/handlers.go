package cli

import (
	"context"
	"fmt"
	"strings"

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
func HandleActivation(ctx context.Context, client *azpim.Client, principalID string, cfg ActivateConfig) error {
	cfg.EnsureDefaults()

	if cfg.NeedsJustification() {
		fmt.Println("A justification is required before we can submit an activation request.")
		justification, err := PromptJustification(cfg.Justification)
		if err != nil {
			return err
		}
		cfg.Justification = justification
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	printActivationSummary(cfg)

	roles, err := client.GetEligibleRoles()
	if err != nil {
		return fmt.Errorf("get eligible roles: %w", err)
	}

	roles = filterEligibleRoles(roles, cfg)

	if len(roles) == 0 {
		if cfg.HasFilters() {
			return fmt.Errorf("no eligible PIM roles matched the provided filters")
		}
		return fmt.Errorf("no eligible PIM roles found")
	}

	var selected []azpim.Role
	if len(roles) == 1 && cfg.HasFilters() {
		selected = roles
		fmt.Printf("\nEligible role matched filters: %s @ %s\n", roles[0].RoleName, roles[0].ScopeDisplay)
	} else {
		fmt.Printf("\nEligible roles (%d):\n", len(roles))
		selected, err = PromptMultiSelection(roles,
			func(i int, r azpim.Role) string {
				return fmt.Sprintf("  %2d) %s @ %s", i, r.RoleName, r.ScopeDisplay)
			},
			func(r azpim.Role) string {
				return fmt.Sprintf("%s %s %s", r.RoleName, r.ScopeDisplay, r.Scope)
			},
			"Select role(s) to activate",
		)
		if err != nil {
			return fmt.Errorf("selection: %w", err)
		}
	}

	// Determine final scopes for all selected roles before confirming
	type roleActivation struct {
		role          azpim.Role
		targetScope   string
		targetDisplay string
	}
	activations := make([]roleActivation, 0, len(selected))

	for _, role := range selected {
		targetScope, targetDisplay, err := determineActivationScope(client, role, cfg)
		if err != nil {
			return fmt.Errorf("determine target scope for %s @ %s: %w", role.RoleName, role.ScopeDisplay, err)
		}
		activations = append(activations, roleActivation{
			role:          role,
			targetScope:   targetScope,
			targetDisplay: targetDisplay,
		})
	}

	// Show detailed confirmation
	if !cfg.Yes {
		summaries := make([]activationSummary, len(activations))
		for i, act := range activations {
			summaries[i] = activationSummary{
				roleName:     act.role.RoleName,
				scopeDisplay: act.targetDisplay,
			}
		}
		if err := PromptConfirmActivationDetailed(summaries, cfg.Justification, formatMinutes(cfg.Minutes)); err != nil {
			return err
		}
	}

	// Execute activations
	for _, act := range activations {
		resp, err := client.ActivateRole(act.role, principalID, cfg.Justification, cfg.Minutes, act.targetScope)
		if err != nil {
			return fmt.Errorf("activate role %s @ %s: %w", act.role.RoleName, act.targetDisplay, err)
		}
		fmt.Printf("✓ Activation submitted for %s @ %s (%s) (status: %s)\n", act.role.RoleName, act.targetDisplay, formatMinutes(cfg.Minutes), resp.Properties.Status)
	}

	return nil
}

func determineActivationScope(client *azpim.Client, role azpim.Role, cfg ActivateConfig) (string, string, error) {
	defaultScope := role.Scope
	defaultDisplay := role.ScopeDisplay

	if !azpim.IsManagementGroupScope(role.Scope) {
		return defaultScope, defaultDisplay, nil
	}

	mgID := azpim.ManagementGroupIDFromScope(role.Scope)
	if mgID == "" {
		return defaultScope, defaultDisplay, nil
	}

	subs, err := client.ListManagementGroupSubscriptions(mgID)
	if err != nil {
		if isAuthorizationError(err) {
			fmt.Printf("⚠️  Unable to list subscriptions for %s (missing management group read permission). Activating entire management group.\n", role.ScopeDisplay)
			return defaultScope, defaultDisplay, nil
		}
		return "", "", fmt.Errorf("list subscriptions for %s: %w", role.ScopeDisplay, err)
	}

	if len(subs) == 0 {
		return defaultScope, defaultDisplay, nil
	}

	// Attempt headless selection using provided hints
	if cfg.HasTargetHints() {
		targetScope, targetDisplay, ok, err := autoSelectManagementGroupScope(client, role, subs, cfg)
		if err != nil {
			return "", "", err
		}
		if ok {
			return targetScope, targetDisplay, nil
		}
	}

	return promptManagementGroupScope(client, role, subs, cfg)
}

func autoSelectManagementGroupScope(client *azpim.Client, role azpim.Role, subs []azpim.Subscription, cfg ActivateConfig) (string, string, bool, error) {
	if len(cfg.Subscriptions) == 0 {
		return "", "", false, nil
	}

	matches := findSubscriptionsByTokens(subs, cfg.Subscriptions)
	if len(matches) == 0 {
		return "", "", false, nil
	}
	if len(matches) > 1 {
		// ambiguous, fall back to interactive selection
		fmt.Printf("⚠️  Multiple subscriptions match filters (%d). You'll be prompted to choose.\n", len(matches))
		return "", "", false, nil
	}

	chosen := matches[0]
	// If resource-group hints provided, attempt to resolve further
	if len(cfg.ResourceGroups) > 0 {
		rgScope, rgDisplay, ok, err := autoSelectResourceGroup(client, chosen, cfg.ResourceGroups)
		if err != nil {
			return "", "", false, err
		}
		if ok {
			fmt.Printf("🔧 Targeting resource group %s within %s\n", rgDisplay, role.ScopeDisplay)
			return rgScope, rgDisplay, true, nil
		}
	}

	fmt.Printf("🔧 Targeting subscription %s (%s) under %s\n", chosen.DisplayName, chosen.ID, role.ScopeDisplay)
	return chosen.Scope(), chosen.DisplayName, true, nil
}

func autoSelectResourceGroup(client *azpim.Client, subscription azpim.Subscription, hints []string) (string, string, bool, error) {
	if len(hints) == 0 {
		return "", "", false, nil
	}

	groups, err := client.ListSubscriptionResourceGroups(subscription.ID)
	if err != nil {
		if isAuthorizationError(err) {
			fmt.Printf("⚠️  Unable to list resource groups for %s (insufficient permission). Activating entire subscription scope.\n", subscription.DisplayName)
			return subscription.Scope(), subscription.DisplayName, true, nil
		}
		return "", "", false, fmt.Errorf("list resource groups for %s: %w", subscription.DisplayName, err)
	}

	matches := findResourceGroupsByTokens(groups, hints)
	if len(matches) == 0 {
		return "", "", false, nil
	}
	if len(matches) > 1 {
		fmt.Printf("⚠️  Multiple resource groups match filters (%d). You'll be prompted to choose.\n", len(matches))
		return "", "", false, nil
	}

	chosen := matches[0]
	label := fmt.Sprintf("%s/%s", subscription.DisplayName, chosen.Name)
	return chosen.Scope(), label, true, nil
}

func promptManagementGroupScope(client *azpim.Client, role azpim.Role, subs []azpim.Subscription, cfg ActivateConfig) (string, string, error) {
	options := []scopeOption{
		{Label: fmt.Sprintf("Activate entire management group (%s)", role.ScopeDisplay), Kind: scopeOptionManagementGroup},
		{Label: "Scope to a subscription", Kind: scopeOptionSubscription},
		{Label: "Scope to a resource group", Kind: scopeOptionResourceGroup},
	}

	choice, err := PromptSelection(options,
		func(i int, opt scopeOption) string {
			return fmt.Sprintf("  %2d) %s", i, opt.Label)
		},
		"Choose scope option")
	if err != nil {
		return "", "", fmt.Errorf("scope option: %w", err)
	}

	switch choice.Kind {
	case scopeOptionManagementGroup:
		return role.Scope, role.ScopeDisplay, nil
	case scopeOptionSubscription:
		selectedSub, err := promptSubscription(subs, cfg)
		if err != nil {
			return "", "", err
		}
		return selectedSub.Scope(), selectedSub.DisplayName, nil
	case scopeOptionResourceGroup:
		selectedSub, err := promptSubscription(subs, cfg)
		if err != nil {
			return "", "", err
		}
		scope, label, err := promptResourceGroup(client, selectedSub, cfg)
		if err != nil {
			return "", "", err
		}
		return scope, label, nil
	default:
		return role.Scope, role.ScopeDisplay, nil
	}
}

func promptSubscription(subs []azpim.Subscription, cfg ActivateConfig) (azpim.Subscription, error) {
	display := func(i int, s azpim.Subscription) string {
		return fmt.Sprintf("  %2d) %s (%s)", i, s.DisplayName, s.ID)
	}
	key := func(s azpim.Subscription) string {
		return fmt.Sprintf("%s %s", s.DisplayName, s.ID)
	}

	if len(cfg.Subscriptions) > 0 {
		fmt.Printf("Filters hint at subscriptions matching %s\n", strings.Join(cfg.Subscriptions, ", "))
	}

	return PromptSingleSelection(subs, display, key, "Select subscription scope")
}

func promptResourceGroup(client *azpim.Client, subscription azpim.Subscription, cfg ActivateConfig) (string, string, error) {
	fmt.Printf("\nFetching resource groups for %s (%s)...\n", subscription.DisplayName, subscription.ID)
	groups, err := client.ListSubscriptionResourceGroups(subscription.ID)
	if err != nil {
		if isAuthorizationError(err) {
			fmt.Printf("⚠️  Unable to list resource groups for %s (insufficient permission). Activating entire subscription scope instead.\n", subscription.DisplayName)
			return subscription.Scope(), subscription.DisplayName, nil
		}
		return "", "", err
	}

	if len(groups) == 0 {
		return subscription.Scope(), subscription.DisplayName, nil
	}

	if len(cfg.ResourceGroups) > 0 {
		fmt.Printf("Filters hint at resource groups matching %s\n", strings.Join(cfg.ResourceGroups, ", "))
	}

	display := func(i int, rg azpim.ResourceGroup) string {
		return fmt.Sprintf("  %2d) %s", i, rg.Name)
	}
	key := func(rg azpim.ResourceGroup) string {
		return fmt.Sprintf("%s %s", rg.Name, rg.ID)
	}

	chosen, err := PromptSingleSelection(groups, display, key, fmt.Sprintf("Select resource group within %s", subscription.DisplayName))
	if err != nil {
		return "", "", err
	}

	label := fmt.Sprintf("%s/%s", subscription.DisplayName, chosen.Name)
	return chosen.Scope(), label, nil
}

func findSubscriptionsByTokens(subs []azpim.Subscription, tokens []string) []azpim.Subscription {
	if len(tokens) == 0 {
		return nil
	}

	matches := make([]azpim.Subscription, 0)
	for _, sub := range subs {
		idLower := strings.ToLower(sub.ID)
		nameLower := strings.ToLower(sub.DisplayName)
		for _, token := range tokens {
			needle := strings.ToLower(token)
			if needle == "" {
				continue
			}
			if strings.Contains(idLower, needle) || strings.Contains(nameLower, needle) {
				matches = append(matches, sub)
				break
			}
		}
	}
	return matches
}

func findResourceGroupsByTokens(groups []azpim.ResourceGroup, tokens []string) []azpim.ResourceGroup {
	if len(tokens) == 0 {
		return nil
	}

	matches := make([]azpim.ResourceGroup, 0)
	for _, rg := range groups {
		nameLower := strings.ToLower(rg.Name)
		for _, token := range tokens {
			needle := strings.ToLower(token)
			if needle == "" {
				continue
			}
			if strings.Contains(nameLower, needle) {
				matches = append(matches, rg)
				break
			}
		}
	}
	return matches
}

type scopeOption struct {
	Label string
	Kind  scopeOptionKind
}

type scopeOptionKind string

const (
	scopeOptionManagementGroup scopeOptionKind = "management-group"
	scopeOptionSubscription    scopeOptionKind = "subscription"
	scopeOptionResourceGroup   scopeOptionKind = "resource-group"
)

func isAuthorizationError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "authorizationfailed") || strings.Contains(msg, "http 403") || strings.Contains(msg, "status code 403")
}

func printActivationSummary(cfg ActivateConfig) {
	fmt.Println("\nActivation overview:")
	fmt.Printf("  Justification : %s\n", cfg.Justification)
	fmt.Printf("  Duration      : %s\n", formatMinutes(cfg.Minutes))
	fmt.Printf("  Mode          : %s\n", cfg.ModeLabel())
	printFilterSummary(cfg)
	fmt.Println()
}

func printFilterSummary(cfg ActivateConfig) {
	if !cfg.HasFilters() {
		fmt.Println("  Filters       : none (all eligible roles will be shown)")
		return
	}
	fmt.Println("  Filters       :")
	printFilterGroup("    management group", cfg.ManagementGroups)
	printFilterGroup("    subscription", cfg.Subscriptions)
	printFilterGroup("    resource group", cfg.ResourceGroups)
	printFilterGroup("    role", cfg.Roles)
	printFilterGroup("    scope contains", cfg.ScopeContains)
}

func printFilterGroup(label string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Printf("%s: %s\n", label, strings.Join(values, ", "))
}

// formatMinutes formats minutes as human-readable duration
func formatMinutes(minutes int) string {
	hours := minutes / 60
	mins := minutes % 60
	if mins == 0 {
		return fmt.Sprintf("%d hour(s)", hours)
	}
	if hours == 0 {
		return fmt.Sprintf("%d minute(s)", mins)
	}
	return fmt.Sprintf("%d hour(s) %d minute(s)", hours, mins)
}
