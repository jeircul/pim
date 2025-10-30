package azpim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/google/uuid"
)

const (
	// APIVersion is the Azure PIM API version
	APIVersion = "2020-10-01"
	// ARMEndpoint is the Azure Resource Manager endpoint
	ARMEndpoint = "https://management.azure.com"
	// GraphEndpoint is the Microsoft Graph API endpoint
	GraphEndpoint = "https://graph.microsoft.com/v1.0"
	// DefaultTimeout is the default HTTP request timeout
	DefaultTimeout = 30 * time.Second
	// MinHours is the minimum activation duration
	MinHours = 1
	// MaxHours is the maximum activation duration
	MaxHours = 8
)

// Client handles Azure PIM operations
type Client struct {
	cred       azcore.TokenCredential
	httpClient *http.Client
	armToken   string
	graphToken string
	ctx        context.Context
}

// NewClient creates a new PIM client with default Azure credential
func NewClient(ctx context.Context) (*Client, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}

	return &Client{
		cred: cred,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		ctx: ctx,
	}, nil
}

// getToken retrieves an access token for the specified scope
func (c *Client) getToken(scope string) (string, error) {
	token, err := c.cred.GetToken(c.ctx, policy.TokenRequestOptions{
		Scopes: []string{scope},
	})
	if err != nil {
		return "", fmt.Errorf("acquire token for %s: %w", scope, err)
	}
	return token.Token, nil
}

// ensureTokens ensures ARM and Graph tokens are cached
func (c *Client) ensureTokens() error {
	if c.armToken == "" {
		token, err := c.getToken("https://management.azure.com/.default")
		if err != nil {
			return err
		}
		c.armToken = token
	}
	if c.graphToken == "" {
		token, err := c.getToken("https://graph.microsoft.com/.default")
		if err != nil {
			return err
		}
		c.graphToken = token
	}
	return nil
}

// doRequest executes an HTTP request with proper authentication
func (c *Client) doRequest(method, reqURL, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(c.ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "pim-client/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, handleErrorResponse(resp)
	}

	return resp, nil
}

// handleErrorResponse extracts and formats error details from HTTP response
func handleErrorResponse(resp *http.Response) error {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("HTTP %d (failed to read error body: %w)", resp.StatusCode, err)
	}

	// Try to parse Azure error format
	var azureErr struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if json.Unmarshal(body, &azureErr) == nil && azureErr.Error.Code != "" {
		return fmt.Errorf("HTTP %d: %s - %s", resp.StatusCode, azureErr.Error.Code, azureErr.Error.Message)
	}

	// Fallback to raw body
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
}

// GetCurrentUser fetches the current user from Microsoft Graph
func (c *Client) GetCurrentUser() (*User, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}

	resp, err := c.doRequest(http.MethodGet, GraphEndpoint+"/me", c.graphToken, nil)
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}
	defer resp.Body.Close()

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}

	return &user, nil
}

// GetEligibleRoles fetches all eligible PIM roles for the current user
func (c *Client) GetEligibleRoles() ([]Role, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/providers/Microsoft.Authorization/roleEligibilitySchedules?api-version=%s&$filter=asTarget()",
		ARMEndpoint, APIVersion)

	resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
	if err != nil {
		return nil, fmt.Errorf("get eligible roles: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Value []struct {
			Properties struct {
				Scope            string `json:"scope"`
				RoleDefinitionID string `json:"roleDefinitionId"`
				ExpandedProps    struct {
					Scope struct {
						DisplayName string `json:"displayName"`
					} `json:"scope"`
					RoleDefinition struct {
						DisplayName string `json:"displayName"`
					} `json:"roleDefinition"`
				} `json:"expandedProperties"`
			} `json:"properties"`
		} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode roles: %w", err)
	}

	roles := make([]Role, 0, len(result.Value))
	for _, item := range result.Value {
		roles = append(roles, Role{
			Scope:            item.Properties.Scope,
			ScopeDisplay:     item.Properties.ExpandedProps.Scope.DisplayName,
			RoleName:         item.Properties.ExpandedProps.RoleDefinition.DisplayName,
			RoleDefinitionID: item.Properties.RoleDefinitionID,
		})
	}

	return roles, nil
}

// GetActiveAssignments fetches all active PIM assignments for the user
func (c *Client) GetActiveAssignments(principalID string) ([]ActiveAssignment, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/providers/Microsoft.Authorization/roleAssignmentSchedules?api-version=%s&$filter=asTarget()",
		ARMEndpoint, APIVersion)

	resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
	if err != nil {
		return nil, fmt.Errorf("get active assignments: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Value []struct {
			Name       string `json:"name"`
			Properties struct {
				PrincipalID      string `json:"principalId"`
				Scope            string `json:"scope"`
				RoleDefinitionID string `json:"roleDefinitionId"`
				ScheduleInfo     struct {
					EndDateTime string `json:"endDateTime"`
				} `json:"scheduleInfo"`
				ExpandedProps struct {
					Scope struct {
						DisplayName string `json:"displayName"`
					} `json:"scope"`
					RoleDefinition struct {
						DisplayName string `json:"displayName"`
					} `json:"roleDefinition"`
				} `json:"expandedProperties"`
			} `json:"properties"`
		} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode active assignments: %w", err)
	}

	assignments := make([]ActiveAssignment, 0)
	for _, item := range result.Value {
		if item.Properties.PrincipalID == principalID {
			assignments = append(assignments, ActiveAssignment{
				Name:             item.Name,
				Scope:            item.Properties.Scope,
				ScopeDisplay:     item.Properties.ExpandedProps.Scope.DisplayName,
				RoleName:         item.Properties.ExpandedProps.RoleDefinition.DisplayName,
				RoleDefinitionID: item.Properties.RoleDefinitionID,
				EndDateTime:      item.Properties.ScheduleInfo.EndDateTime,
			})
		}
	}

	return assignments, nil
}

// IsRoleActive checks if a specific role is currently active
func (c *Client) IsRoleActive(role Role, principalID string) (bool, error) {
	if err := c.ensureTokens(); err != nil {
		return false, err
	}

	filter := fmt.Sprintf("principalId eq '%s' and roleDefinitionId eq '%s'", principalID, role.RoleDefinitionID)
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/roleAssignmentSchedules?api-version=%s&$filter=%s",
		ARMEndpoint, role.Scope, APIVersion, url.QueryEscape(filter))

	resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
	if err != nil {
		return false, fmt.Errorf("check active status: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Value []interface{} `json:"value"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode active check: %w", err)
	}

	return len(result.Value) > 0, nil
}

// ActivateRole submits a role activation or extension request
func (c *Client) ActivateRole(role Role, principalID, justification string, hours int) (*ScheduleResponse, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}

	// Validate and clamp hours
	hours = clampHours(hours)

	// Check if already active to determine request type
	active, err := c.IsRoleActive(role, principalID)
	if err != nil {
		return nil, err
	}

	requestType := "SelfActivate"
	if active {
		requestType = "SelfExtend"
	}

	scheduleReq := ScheduleRequest{
		Properties: ScheduleProperties{
			PrincipalID:      principalID,
			RoleDefinitionID: role.RoleDefinitionID,
			RequestType:      requestType,
			Justification:    justification,
			ScheduleInfo: &ScheduleInfo{
				StartDateTime: time.Now().UTC().Format(time.RFC3339),
				Expiration: Expiration{
					Type:     "AfterDuration",
					Duration: fmt.Sprintf("PT%dH", hours),
				},
			},
		},
	}

	body, err := json.Marshal(scheduleReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	requestID := uuid.New().String()
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/%s?api-version=%s",
		ARMEndpoint, role.Scope, requestID, APIVersion)

	resp, err := c.doRequest(http.MethodPut, reqURL, c.armToken, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("submit activation: %w", err)
	}
	defer resp.Body.Close()

	var scheduleResp ScheduleResponse
	if err := json.NewDecoder(resp.Body).Decode(&scheduleResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &scheduleResp, nil
}

// DeactivateRole submits a role deactivation request
func (c *Client) DeactivateRole(assignment ActiveAssignment, principalID string) (*ScheduleResponse, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}

	scheduleReq := ScheduleRequest{
		Properties: ScheduleProperties{
			PrincipalID:      principalID,
			RoleDefinitionID: assignment.RoleDefinitionID,
			RequestType:      "SelfDeactivate",
		},
	}

	body, err := json.Marshal(scheduleReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	requestID := uuid.New().String()
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/%s?api-version=%s",
		ARMEndpoint, assignment.Scope, requestID, APIVersion)

	resp, err := c.doRequest(http.MethodPut, reqURL, c.armToken, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("submit deactivation: %w", err)
	}
	defer resp.Body.Close()

	var scheduleResp ScheduleResponse
	if err := json.NewDecoder(resp.Body).Decode(&scheduleResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &scheduleResp, nil
}

// clampHours ensures hours is within valid range
func clampHours(hours int) int {
	if hours < MinHours {
		return MinHours
	}
	if hours > MaxHours {
		return MaxHours
	}
	return hours
}
