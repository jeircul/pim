package azure

import (
	"fmt"
	"strings"
	"time"
)

// User represents an Azure AD user.
type User struct {
	ID                string `json:"id"`
	UserPrincipalName string `json:"userPrincipalName"`
	DisplayName       string `json:"displayName"`
}

// Role represents an eligible PIM role.
type Role struct {
	Scope                 string
	ScopeDisplay          string
	RoleName              string
	RoleDefinitionID      string
	EligibilityScheduleID string
}

// ScopeType classifies the scope level.
type ScopeType int

const (
	ScopeManagementGroup ScopeType = iota
	ScopeSubscription
	ScopeResourceGroup
	ScopeUnknown
)

// ScopeKind returns the type of scope for this role.
func (r Role) ScopeKind() ScopeType {
	switch {
	case IsManagementGroupScope(r.Scope):
		return ScopeManagementGroup
	case IsSubscriptionScope(r.Scope):
		return ScopeSubscription
	case IsResourceGroupScope(r.Scope):
		return ScopeResourceGroup
	default:
		return ScopeUnknown
	}
}

// ActiveAssignment represents an active PIM role assignment.
type ActiveAssignment struct {
	Name             string
	Scope            string
	ScopeDisplay     string
	RoleName         string
	RoleDefinitionID string
	EndDateTime      string
}

// IsPermanent reports whether the assignment has no expiry.
func (a ActiveAssignment) IsPermanent() bool {
	return strings.TrimSpace(a.EndDateTime) == ""
}

// TimeRemaining returns the duration remaining until expiry. Zero if permanent or expired.
func (a ActiveAssignment) TimeRemaining() time.Duration {
	if a.EndDateTime == "" {
		return 0
	}
	end, err := time.Parse(time.RFC3339, a.EndDateTime)
	if err != nil {
		return 0
	}
	d := time.Until(end)
	if d < 0 {
		return 0
	}
	return d
}

// ExpiryDisplay returns a short human-readable time-remaining string.
func (a ActiveAssignment) ExpiryDisplay() string {
	if a.EndDateTime == "" {
		return "permanent"
	}
	end, err := time.Parse(time.RFC3339, a.EndDateTime)
	if err != nil {
		return a.EndDateTime
	}
	d := time.Until(end)
	if d <= 0 {
		return "expired"
	}
	return humanizeDuration(d)
}

func humanizeDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	var parts []string
	if d >= 24*time.Hour {
		days := d / (24 * time.Hour)
		parts = append(parts, fmt.Sprintf("%dd", days))
		d -= days * 24 * time.Hour
	}
	if d >= time.Hour {
		h := d / time.Hour
		parts = append(parts, fmt.Sprintf("%dh", h))
		d -= h * time.Hour
	}
	if d >= time.Minute {
		m := d / time.Minute
		parts = append(parts, fmt.Sprintf("%dm", m))
	}
	return strings.Join(parts, " ")
}

// Subscription represents a child subscription.
type Subscription struct {
	ID          string
	DisplayName string
}

// Scope returns the subscription scope path.
func (s Subscription) Scope() string { return "/subscriptions/" + s.ID }

// ResourceGroup represents a resource group inside a subscription.
type ResourceGroup struct {
	SubscriptionID string
	Name           string
	ID             string
}

// Scope returns the resource group scope path.
func (rg ResourceGroup) Scope() string {
	return fmt.Sprintf("/subscriptions/%s/resourceGroups/%s", rg.SubscriptionID, rg.Name)
}

// ScheduleRequest is the PIM activation request body.
type ScheduleRequest struct {
	Properties ScheduleProperties `json:"properties"`
}

// ScheduleProperties contains the PIM request details.
type ScheduleProperties struct {
	PrincipalID                     string        `json:"principalId"`
	RoleDefinitionID                string        `json:"roleDefinitionId"`
	RequestType                     string        `json:"requestType"`
	Justification                   string        `json:"justification,omitempty"`
	LinkedRoleEligibilityScheduleID string        `json:"linkedRoleEligibilityScheduleId,omitempty"`
	ScheduleInfo                    *ScheduleInfo `json:"scheduleInfo,omitempty"`
}

// ScheduleInfo contains schedule timing.
type ScheduleInfo struct {
	StartDateTime string     `json:"startDateTime"`
	Expiration    Expiration `json:"expiration"`
}

// Expiration defines the duration type.
type Expiration struct {
	Type     string `json:"type"`
	Duration string `json:"duration"`
}

// ScheduleResponse is the API response from a schedule request.
type ScheduleResponse struct {
	Name       string `json:"name"`
	Properties struct {
		Status string `json:"status"`
	} `json:"properties"`
}
