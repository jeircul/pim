package azpim

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
