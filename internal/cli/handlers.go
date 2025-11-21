package cli

import (
	"context"
	"errors"
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

type activationTarget struct {
	scope   string
	display string
}

var errMultipleResourceGroups = errors.New("multiple resource groups match filters")

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
		role   azpim.Role
		target activationTarget
	}
	activations := make([]roleActivation, 0, len(selected))

	for _, role := range selected {
		targets, err := determineActivationTargets(client, role, cfg)
		if err != nil {
			return fmt.Errorf("determine target scope for %s @ %s: %w", role.RoleName, role.ScopeDisplay, err)
		}
		for _, target := range targets {
			activations = append(activations, roleActivation{role: role, target: target})
		}
	}

	// Show detailed confirmation
	if !cfg.Yes {
		summaries := make([]activationSummary, len(activations))
		for i, act := range activations {
			summaries[i] = activationSummary{
				roleName:     act.role.RoleName,
				scopeDisplay: act.target.display,
			}
		}
		if err := PromptConfirmActivationDetailed(summaries, cfg.Justification, formatMinutes(cfg.Minutes)); err != nil {
			return err
		}
	}

	// Execute activations
	for _, act := range activations {
		resp, err := client.ActivateRole(act.role, principalID, cfg.Justification, cfg.Minutes, act.target.scope)
		if err != nil {
			return fmt.Errorf("activate role %s @ %s: %w", act.role.RoleName, act.target.display, err)
		}
		fmt.Printf("✓ Activation submitted for %s @ %s (%s) (status: %s)\n", act.role.RoleName, act.target.display, formatMinutes(cfg.Minutes), resp.Properties.Status)
	}

	return nil
}

func determineActivationTargets(client *azpim.Client, role azpim.Role, cfg ActivateConfig) ([]activationTarget, error) {
	defaultTarget := activationTarget{scope: role.Scope, display: role.ScopeDisplay}

	if !azpim.IsManagementGroupScope(role.Scope) {
		return []activationTarget{defaultTarget}, nil
	}

	mgID := azpim.ManagementGroupIDFromScope(role.Scope)
	if mgID == "" {
		return []activationTarget{defaultTarget}, nil
	}

	subs, err := client.ListManagementGroupSubscriptions(mgID)
	if err != nil {
		if isAuthorizationError(err) {
			fmt.Printf("⚠️  Unable to list subscriptions for %s (missing management group read permission). Activating entire management group.\n", role.ScopeDisplay)
			return []activationTarget{defaultTarget}, nil
		}
		return nil, fmt.Errorf("list subscriptions for %s: %w", role.ScopeDisplay, err)
	}

	if len(subs) == 0 {
		return []activationTarget{defaultTarget}, nil
	}

	if cfg.HasTargetHints() {
		if len(cfg.Subscriptions) > 0 && len(cfg.ResourceGroups) > 0 {
			targets, err := promptResourceGroupsForHints(client, mgID, subs, cfg)
			if err != nil {
				return nil, err
			}
			if len(targets) > 0 {
				return targets, nil
			}
		}
		target, ok, err := autoSelectManagementGroupScope(client, role, subs, cfg)
		if err != nil {
			return nil, err
		}
		if ok {
			return []activationTarget{target}, nil
		}
	}

	return promptManagementGroupTargets(client, mgID, role, subs, cfg)
}

func autoSelectManagementGroupScope(client *azpim.Client, role azpim.Role, subs []azpim.Subscription, cfg ActivateConfig) (activationTarget, bool, error) {
	var zero activationTarget
	if len(cfg.Subscriptions) == 0 {
		return zero, false, nil
	}

	matches := findSubscriptionsByTokens(subs, cfg.Subscriptions)
	if len(matches) == 0 {
		return zero, false, nil
	}
	if len(matches) > 1 {
		fmt.Printf("⚠️  Multiple subscriptions match filters (%d). You'll be prompted to choose.\n", len(matches))
		return zero, false, nil
	}

	chosen := matches[0]
	if len(cfg.ResourceGroups) > 0 {
		target, ok, err := autoSelectResourceGroup(client, chosen, cfg.ResourceGroups)
		if err != nil {
			if errors.Is(err, errMultipleResourceGroups) {
				return zero, false, nil
			}
			return zero, false, err
		}
		if ok {
			fmt.Printf("🔧 Targeting resource group %s within %s\n", target.display, role.ScopeDisplay)
			return target, true, nil
		}
		return zero, false, nil
	}

	fmt.Printf("🔧 Targeting subscription %s (%s) under %s\n", chosen.DisplayName, chosen.ID, role.ScopeDisplay)
	return activationTarget{scope: chosen.Scope(), display: chosen.DisplayName}, true, nil
}

func autoSelectResourceGroup(client *azpim.Client, subscription azpim.Subscription, hints []string) (activationTarget, bool, error) {
	var zero activationTarget
	if len(hints) == 0 {
		return zero, false, nil
	}

	groups, err := client.ListSubscriptionResourceGroups(subscription.ID)
	if err != nil {
		if isAuthorizationError(err) {
			fmt.Printf("⚠️  Unable to list resource groups for %s (insufficient permission). Activating entire subscription scope.\n", subscription.DisplayName)
			return activationTarget{scope: subscription.Scope(), display: subscription.DisplayName}, true, nil
		}
		return zero, false, fmt.Errorf("list resource groups for %s: %w", subscription.DisplayName, err)
	}

	matches := findResourceGroupsByTokens(groups, hints)
	if len(matches) == 0 {
		return zero, false, nil
	}
	if len(matches) > 1 {
		fmt.Printf("⚠️  Multiple resource groups match filters (%d). You'll be prompted to choose.\n", len(matches))
		return zero, false, errMultipleResourceGroups
	}

	chosen := matches[0]
	label := fmt.Sprintf("%s/%s", subscription.DisplayName, chosen.Name)
	return activationTarget{scope: chosen.Scope(), display: label}, true, nil
}

func promptManagementGroupTargets(client *azpim.Client, mgID string, role azpim.Role, subs []azpim.Subscription, cfg ActivateConfig) ([]activationTarget, error) {
	options := []scopeOption{
		{Label: fmt.Sprintf("Activate entire management group (%s)", role.ScopeDisplay), Kind: scopeOptionManagementGroup},
		{Label: "Scope to subscription(s)", Kind: scopeOptionSubscription},
		{Label: "Scope to resource group(s)", Kind: scopeOptionResourceGroup},
	}

	choice, err := PromptSelection(options,
		func(i int, opt scopeOption) string {
			return fmt.Sprintf("  %2d) %s", i, opt.Label)
		},
		"Choose scope option")
	if err != nil {
		return nil, fmt.Errorf("scope option: %w", err)
	}

	switch choice.Kind {
	case scopeOptionManagementGroup:
		return []activationTarget{{scope: role.Scope, display: role.ScopeDisplay}}, nil
	case scopeOptionSubscription:
		selectedSubs, err := promptSubscriptions(subs, cfg)
		if err != nil {
			return nil, err
		}
		targets := make([]activationTarget, 0, len(selectedSubs))
		for _, sub := range selectedSubs {
			targets = append(targets, activationTarget{scope: sub.Scope(), display: sub.DisplayName})
		}
		return targets, nil
	case scopeOptionResourceGroup:
		return promptResourceGroupsForHints(client, mgID, subs, cfg)
	default:
		return []activationTarget{{scope: role.Scope, display: role.ScopeDisplay}}, nil
	}
}

func promptResourceGroupsForHints(client *azpim.Client, mgID string, subs []azpim.Subscription, cfg ActivateConfig) ([]activationTarget, error) {
	if len(cfg.ResourceGroups) == 0 {
		return nil, nil
	}
	selectedSubs := subs
	if len(cfg.Subscriptions) > 0 {
		selected := findSubscriptionsByTokens(subs, cfg.Subscriptions)
		if len(selected) > 0 {
			selectedSubs = selected
		}
	}
	var targets []activationTarget
	for idx, sub := range selectedSubs {
		if idx > 0 {
			fmt.Println()
		}
		subTargets, err := promptResourceGroups(client, mgID, sub, cfg)
		if err != nil {
			return nil, err
		}
		targets = append(targets, subTargets...)
	}
	return targets, nil
}

func promptSubscriptions(subs []azpim.Subscription, cfg ActivateConfig) ([]azpim.Subscription, error) {
	display := func(i int, s azpim.Subscription) string {
		return fmt.Sprintf("  %2d) %s (%s)", i, s.DisplayName, s.ID)
	}
	key := func(s azpim.Subscription) string {
		return fmt.Sprintf("%s %s", s.DisplayName, s.ID)
	}

	if len(cfg.Subscriptions) > 0 {
		fmt.Printf("Filters hint at subscriptions matching %s\n", strings.Join(cfg.Subscriptions, ", "))
	}

	return PromptMultiSelection(subs, display, key, "Select subscription scope(s)")
}

func promptResourceGroups(client *azpim.Client, mgID string, subscription azpim.Subscription, cfg ActivateConfig) ([]activationTarget, error) {
	fmt.Printf("\nFetching resource groups for %s (%s)...\n", subscription.DisplayName, subscription.ID)
	groups, err := client.ListSubscriptionResourceGroups(subscription.ID)
	if err != nil {
		if isAuthorizationError(err) {
			fmt.Printf("⚠️  Unable to list resource groups for %s (insufficient permission). Activating entire subscription scope instead.\n", subscription.DisplayName)
			return []activationTarget{{scope: subscription.Scope(), display: subscription.DisplayName}}, nil
		}
		return nil, err
	}

	if len(groups) == 0 && mgID != "" {
		mgGroups, mgErr := client.ListManagementGroupResourceGroups(mgID)
		if mgErr == nil {
			groups = filterResourceGroupsForSubscription(mgGroups, subscription.ID)
		}
	}

	view := groups
	if len(cfg.ResourceGroups) > 0 {
		fmt.Printf("Filters hint at resource groups matching %s\n", strings.Join(cfg.ResourceGroups, ", "))
		matching := findResourceGroupsByTokens(groups, cfg.ResourceGroups)
		if len(matching) > 0 {
			view = matching
			fmt.Printf("Showing %d resource group(s) matching filters.\n", len(view))
		} else if len(groups) == 0 {
			synthetic := resourceGroupsFromHints(subscription, cfg.ResourceGroups)
			if len(synthetic) > 0 {
				fmt.Printf("⚠️  Azure API returned no resource groups; using provided filters.\n")
				view = synthetic
			}
		} else {
			fmt.Printf("⚠️  No resource groups matched filters (%s). Showing all available groups.\n", strings.Join(cfg.ResourceGroups, ", "))
		}
	}

	if len(view) == 0 {
		fmt.Printf("⚠️  No resource groups available under %s. Activating entire subscription scope.\n", subscription.DisplayName)
		return []activationTarget{{scope: subscription.Scope(), display: subscription.DisplayName}}, nil
	}

	display := func(i int, rg azpim.ResourceGroup) string {
		return fmt.Sprintf("  %2d) %s", i, rg.Name)
	}
	key := func(rg azpim.ResourceGroup) string {
		return fmt.Sprintf("%s %s", rg.Name, rg.ID)
	}

	chosen, err := PromptMultiSelection(view, display, key, fmt.Sprintf("Select resource group(s) within %s", subscription.DisplayName))
	if err != nil {
		return nil, err
	}

	targets := make([]activationTarget, 0, len(chosen))
	for _, rg := range chosen {
		label := fmt.Sprintf("%s/%s", subscription.DisplayName, rg.Name)
		targets = append(targets, activationTarget{scope: rg.Scope(), display: label})
	}
	return targets, nil
}

func resourceGroupsFromHints(subscription azpim.Subscription, hints []string) []azpim.ResourceGroup {
	if len(hints) == 0 {
		return nil
	}
	groups := make([]azpim.ResourceGroup, 0, len(hints))
	seen := make(map[string]struct{}, len(hints))
	for _, hint := range hints {
		name := strings.TrimSpace(hint)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		groups = append(groups, azpim.ResourceGroup{
			SubscriptionID: subscription.ID,
			Name:           name,
			ID:             fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", subscription.ID, name),
		})
	}
	return groups
}

func filterResourceGroupsForSubscription(groups []azpim.ResourceGroup, subscriptionID string) []azpim.ResourceGroup {
	if len(groups) == 0 {
		return nil
	}
	filtered := make([]azpim.ResourceGroup, 0, len(groups))
	for _, group := range groups {
		if strings.EqualFold(group.SubscriptionID, subscriptionID) {
			filtered = append(filtered, group)
		}
	}
	return filtered
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
