package azure

import (
	"regexp"
	"strings"
)

// IsManagementGroupScope reports whether the scope is a management group.
func IsManagementGroupScope(scope string) bool {
	return strings.HasPrefix(strings.ToLower(scope), "/providers/microsoft.management/managementgroups/")
}

// IsSubscriptionScope reports whether the scope is a subscription (not RG).
func IsSubscriptionScope(scope string) bool {
	lower := strings.ToLower(scope)
	return strings.HasPrefix(lower, "/subscriptions/") && !strings.Contains(lower, "/resourcegroups/")
}

// IsResourceGroupScope reports whether the scope is a resource group.
func IsResourceGroupScope(scope string) bool {
	return strings.Contains(strings.ToLower(scope), "/resourcegroups/")
}

// ManagementGroupIDFromScope extracts the management group ID from a scope path.
func ManagementGroupIDFromScope(scope string) string {
	const prefix = "/providers/Microsoft.Management/managementGroups/"
	if len(scope) < len(prefix) {
		return ""
	}
	if !strings.EqualFold(scope[:len(prefix)], prefix) {
		return ""
	}
	remainder := scope[len(prefix):]
	if slash := strings.Index(remainder, "/"); slash != -1 {
		return remainder[:slash]
	}
	return remainder
}

// SubscriptionIDFromScope extracts the subscription ID from a scope path.
func SubscriptionIDFromScope(scope string) string {
	const prefix = "/subscriptions/"
	if len(scope) < len(prefix) {
		return ""
	}
	if !strings.EqualFold(scope[:len(prefix)], prefix) {
		return ""
	}
	remainder := scope[len(prefix):]
	if slash := strings.Index(remainder, "/"); slash != -1 {
		return remainder[:slash]
	}
	return remainder
}

// ResourceGroupNameFromScope extracts the subscription ID and resource group name from a scope path.
func ResourceGroupNameFromScope(scope string) (subscriptionID, resourceGroup string) {
	if !strings.Contains(strings.ToLower(scope), "/resourcegroups/") {
		return "", ""
	}
	subscriptionID = SubscriptionIDFromScope(scope)
	const marker = "/resourceGroups/"
	idx := strings.Index(scope, marker)
	if idx == -1 {
		lower := strings.ToLower(scope)
		idx = strings.Index(lower, strings.ToLower(marker))
		if idx == -1 {
			return subscriptionID, ""
		}
	}
	remainder := scope[idx+len(marker):]
	if slash := strings.Index(remainder, "/"); slash != -1 {
		remainder = remainder[:slash]
	}
	return subscriptionID, remainder
}

// DefaultScopeDisplay returns a human-readable name for a scope path.
func DefaultScopeDisplay(scope, display string) string {
	if strings.TrimSpace(display) != "" {
		return display
	}
	switch {
	case IsResourceGroupScope(scope):
		_, rg := ResourceGroupNameFromScope(scope)
		if rg != "" {
			return rg
		}
	case IsSubscriptionScope(scope):
		id := SubscriptionIDFromScope(scope)
		if id != "" {
			return id
		}
	case IsManagementGroupScope(scope):
		id := ManagementGroupIDFromScope(scope)
		if id != "" {
			return id
		}
	}
	return scope
}

// ScopeIsChildOf reports whether child is equal to or a descendant of parent.
// Both are ARM scope paths (case-insensitive, segment-boundary match).
func ScopeIsChildOf(child, parent string) bool {
	c := strings.ToLower(strings.TrimRight(child, "/"))
	p := strings.ToLower(strings.TrimRight(parent, "/"))
	return c == p || strings.HasPrefix(c, p+"/")
}

// ScopeMatches reports whether filter matches a role's scope, using ARM child-path
// check first, then case-insensitive substring match on scopeDisplay and scope.
func ScopeMatches(filter, scope, scopeDisplay string) bool {
	f := strings.TrimSpace(filter)
	if f == "" {
		return false
	}
	if ScopeIsChildOf(f, scope) {
		return true
	}
	lower := strings.ToLower(f)
	return strings.Contains(strings.ToLower(scopeDisplay), lower) ||
		strings.Contains(strings.ToLower(scope), lower)
}

var reSegmentKeyword = regexp.MustCompile(`(?i)/subscriptions/|/resourcegroups/|/providers/microsoft\.management/managementgroups/`)

// NormalizeScope lowercases known ARM segment keywords while preserving IDs/names verbatim.
func NormalizeScope(s string) string {
	return reSegmentKeyword.ReplaceAllStringFunc(s, strings.ToLower)
}
