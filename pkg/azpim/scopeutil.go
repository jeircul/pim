package azpim

import "strings"

// IsManagementGroupScope reports whether the scope represents a management group
func IsManagementGroupScope(scope string) bool {
	return strings.HasPrefix(strings.ToLower(scope), "/providers/microsoft.management/managementgroups/")
}

// ManagementGroupIDFromScope extracts the management group ID from a scope path
func ManagementGroupIDFromScope(scope string) string {
	const prefix = "/providers/Microsoft.Management/managementGroups/"
	if len(scope) < len(prefix) {
		return ""
	}
	if !strings.EqualFold(scope[:len(prefix)], prefix) {
		return ""
	}
	remainder := scope[len(prefix):]
	parts := strings.Split(remainder, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// SubscriptionIDFromScope extracts the subscription ID from a scope path
func SubscriptionIDFromScope(scope string) string {
	const prefix = "/subscriptions/"
	if len(scope) < len(prefix) {
		return ""
	}
	if !strings.EqualFold(scope[:len(prefix)], prefix) {
		return ""
	}
	remainder := scope[len(prefix):]
	parts := strings.Split(remainder, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// ResourceGroupNameFromScope extracts the subscription ID and resource group name from a scope path
func ResourceGroupNameFromScope(scope string) (subscriptionID string, resourceGroup string) {
	if !strings.Contains(strings.ToLower(scope), "/resourcegroups/") {
		return "", ""
	}
	subscriptionID = SubscriptionIDFromScope(scope)
	marker := "/resourceGroups/"
	idx := strings.Index(scope, marker)
	if idx == -1 {
		return subscriptionID, ""
	}
	remainder := scope[idx+len(marker):]
	if slash := strings.Index(remainder, "/"); slash != -1 {
		remainder = remainder[:slash]
	}
	return subscriptionID, remainder
}
