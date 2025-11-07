package cli

import (
	"strings"

	"github.com/jeircul/pim/pkg/azpim"
)

func filterEligibleRoles(roles []azpim.Role, cfg ActivateConfig) []azpim.Role {
	if !cfg.HasFilters() {
		return roles
	}

	filtered := make([]azpim.Role, 0, len(roles))
	for _, role := range roles {
		if !matchesManagementGroup(role, cfg.ManagementGroups) {
			continue
		}
		if !matchesSubscription(role, cfg.Subscriptions) {
			continue
		}
		if !matchesResourceGroup(role, cfg.ResourceGroups) {
			continue
		}
		if !matchesScopeContains(role, cfg.ScopeContains) {
			continue
		}
		if !matchesRoleName(role, cfg.Roles) {
			continue
		}
		filtered = append(filtered, role)
	}
	return filtered
}

func matchesManagementGroup(role azpim.Role, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	mGroupID := strings.ToLower(azpim.ManagementGroupIDFromScope(role.Scope))
	if mGroupID == "" {
		return false
	}
	display := strings.ToLower(role.ScopeDisplay)
	for _, f := range filters {
		needle := strings.ToLower(f)
		if (mGroupID != "" && strings.Contains(mGroupID, needle)) || strings.Contains(display, needle) {
			return true
		}
	}
	return false
}

func matchesSubscription(role azpim.Role, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	if azpim.IsManagementGroupScope(role.Scope) {
		return true
	}
	subID := strings.ToLower(azpim.SubscriptionIDFromScope(role.Scope))
	if subID == "" {
		return false
	}
	display := strings.ToLower(role.ScopeDisplay)
	for _, f := range filters {
		needle := strings.ToLower(f)
		if (subID != "" && strings.Contains(subID, needle)) || strings.Contains(display, needle) {
			return true
		}
	}
	return false
}

func matchesResourceGroup(role azpim.Role, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	if azpim.IsManagementGroupScope(role.Scope) {
		return true
	}
	_, rg := azpim.ResourceGroupNameFromScope(role.Scope)
	if rg == "" {
		return false
	}
	rgLower := strings.ToLower(rg)
	for _, f := range filters {
		needle := strings.ToLower(f)
		if strings.Contains(rgLower, needle) {
			return true
		}
	}
	return false
}

func matchesScopeContains(role azpim.Role, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	scope := strings.ToLower(role.Scope)
	display := strings.ToLower(role.ScopeDisplay)
	for _, f := range filters {
		needle := strings.ToLower(f)
		if strings.Contains(scope, needle) || strings.Contains(display, needle) {
			return true
		}
	}
	return false
}

func matchesRoleName(role azpim.Role, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	name := strings.ToLower(role.RoleName)
	for _, f := range filters {
		needle := strings.ToLower(f)
		if strings.Contains(name, needle) {
			return true
		}
	}
	return false
}
