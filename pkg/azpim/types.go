package azpim

import (
	"fmt"
	"strings"
	"time"
)

// User represents an Azure AD user
type User struct {
	ID                string `json:"id"`
	UserPrincipalName string `json:"userPrincipalName"`
	DisplayName       string `json:"displayName"`
}

// Role represents an eligible PIM role
type Role struct {
	Scope            string
	ScopeDisplay     string
	RoleName         string
	RoleDefinitionID string
}

// ActiveAssignment represents an active PIM role assignment
type ActiveAssignment struct {
	Name             string
	Scope            string
	ScopeDisplay     string
	RoleName         string
	RoleDefinitionID string
	EndDateTime      string
}

// IsPermanent reports whether the assignment has no expiry (admin-managed/standing assignment)
func (a ActiveAssignment) IsPermanent() bool {
	return strings.TrimSpace(a.EndDateTime) == ""
}

// ExpiryDisplay returns a human-readable expiry string
func (a ActiveAssignment) ExpiryDisplay() string {
	if a.EndDateTime == "" {
		return "no expiry"
	}
	end, err := time.Parse(time.RFC3339, a.EndDateTime)
	if err != nil {
		return a.EndDateTime
	}
	now := time.Now().UTC()
	diff := end.Sub(now)
	if diff > 0 {
		return fmt.Sprintf("expires in %s", humanizeDuration(diff))
	}
	return fmt.Sprintf("expired %s ago", humanizeDuration(-diff))
}

func humanizeDuration(d time.Duration) string {
	if d < time.Minute {
		return "under a minute"
	}
	segments := []string{}
	if d >= 24*time.Hour {
		days := d / (24 * time.Hour)
		segments = append(segments, fmt.Sprintf("%dd", days))
		d -= days * 24 * time.Hour
	}
	if d >= time.Hour {
		hours := d / time.Hour
		segments = append(segments, fmt.Sprintf("%dh", hours))
		d -= hours * time.Hour
	}
	if d >= time.Minute {
		minutes := d / time.Minute
		segments = append(segments, fmt.Sprintf("%dm", minutes))
	}
	if len(segments) == 0 {
		segments = append(segments, "under a minute")
	}
	return strings.Join(segments, " ")
}

// ScheduleRequest represents a PIM schedule request body
type ScheduleRequest struct {
	Properties ScheduleProperties `json:"properties"`
}

// ScheduleProperties contains the PIM request details
type ScheduleProperties struct {
	PrincipalID      string        `json:"principalId"`
	RoleDefinitionID string        `json:"roleDefinitionId"`
	RequestType      string        `json:"requestType"`
	Justification    string        `json:"justification,omitempty"`
	ScheduleInfo     *ScheduleInfo `json:"scheduleInfo,omitempty"`
}

// ScheduleInfo contains schedule timing information
type ScheduleInfo struct {
	StartDateTime string     `json:"startDateTime"`
	Expiration    Expiration `json:"expiration"`
}

// Expiration defines how long the assignment lasts
type Expiration struct {
	Type     string `json:"type"`
	Duration string `json:"duration"`
}

// ScheduleResponse is the API response from a schedule request
type ScheduleResponse struct {
	Name       string `json:"name"`
	Properties struct {
		Status string `json:"status"`
	} `json:"properties"`
}
