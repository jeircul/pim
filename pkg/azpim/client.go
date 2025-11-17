package azpim

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/google/uuid"
)

// Client handles Azure PIM operations
type Client struct {
	cred       azcore.TokenCredential
	httpClient *http.Client
	armToken   string
	graphToken string
	ctx        context.Context
}

const (
	// APIVersion is the Azure PIM API version
	APIVersion = "2020-10-01"
	// ManagementGroupSubscriptionsAPIVersion is the API version for listing management group subscriptions
	ManagementGroupSubscriptionsAPIVersion = "2023-04-01"
	// EligibleChildResourcesAPIVersion is the API version for eligible child resources
	EligibleChildResourcesAPIVersion = "2020-10-01"
	// ResourceGroupsAPIVersion is the API version for listing resource groups
	ResourceGroupsAPIVersion = "2021-04-01"
	// ARMEndpoint is the Azure Resource Manager endpoint
	ARMEndpoint = "https://management.azure.com"
	// GraphEndpoint is the Microsoft Graph API endpoint
	GraphEndpoint = "https://graph.microsoft.com/v1.0"
	// DefaultTimeout is the default HTTP request timeout
	DefaultTimeout = 30 * time.Second
	// MinMinutes is the minimum activation duration in minutes
	MinMinutes = 30
	// MaxMinutes is the maximum activation duration in minutes
	MaxMinutes = 480
)

// NewClient creates a new PIM client using the best available delegated credential.
func NewClient(ctx context.Context) (*Client, error) {
	tenantID := os.Getenv("AZURE_TENANT_ID")
	var credChain []azcore.TokenCredential

	if cliCred, err := azidentity.NewAzureCLICredential(&azidentity.AzureCLICredentialOptions{TenantID: tenantID}); err == nil {
		credChain = append(credChain, cliCred)
	}

	if psCred, err := azidentity.NewAzurePowerShellCredential(&azidentity.AzurePowerShellCredentialOptions{TenantID: tenantID}); err == nil {
		credChain = append(credChain, psCred)
	}

	if allowDeviceLogin() {
		if deviceCred, err := azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
			TenantID: tenantID,
			UserPrompt: func(ctx context.Context, msg azidentity.DeviceCodeMessage) error {
				fmt.Fprintln(os.Stderr, msg.Message)
				return nil
			},
		}); err == nil {
			credChain = append(credChain, deviceCred)
		}
	}

	if len(credChain) == 0 {
		return nil, fmt.Errorf("no supported Azure login found; sign in with 'az login' or 'Connect-AzAccount'")
	}

	cred, err := azidentity.NewChainedTokenCredential(credChain, nil)
	if err != nil {
		return nil, fmt.Errorf("create credential chain: %w", err)
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
			ID         string `json:"id"`
			Name       string `json:"name"`
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
			Scope:                 item.Properties.Scope,
			ScopeDisplay:          item.Properties.ExpandedProps.Scope.DisplayName,
			RoleName:              item.Properties.ExpandedProps.RoleDefinition.DisplayName,
			RoleDefinitionID:      item.Properties.RoleDefinitionID,
			EligibilityScheduleID: item.ID,
		})
	}

	return roles, nil
}

// ListManagementGroupSubscriptions returns subscriptions under the specified management group
func (c *Client) ListManagementGroupSubscriptions(mgID string) ([]Subscription, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}

	mgID = strings.TrimSpace(mgID)
	if mgID == "" {
		return nil, fmt.Errorf("management group id cannot be empty")
	}

	childSubs, childErr := c.listEligibleChildSubscriptions(mgID)
	if childErr == nil && len(childSubs) > 0 {
		return childSubs, nil
	}

	legacySubs, legacyErr := c.listManagementGroupSubscriptionsLegacy(mgID)
	if legacyErr == nil {
		// Either the new API returned no subscriptions or failed but the legacy call succeeded.
		return legacySubs, nil
	}

	if childErr != nil {
		return nil, fmt.Errorf("eligible child resources: %w; legacy query: %v", childErr, legacyErr)
	}

	return nil, legacyErr
}

func (c *Client) listEligibleChildSubscriptions(mgID string) ([]Subscription, error) {
	reqURL := fmt.Sprintf("%s/providers/Microsoft.Management/managementGroups/%s/providers/Microsoft.Authorization/eligibleChildResources?api-version=%s&getAllChildren=true",
		ARMEndpoint, url.PathEscape(mgID), EligibleChildResourcesAPIVersion)

	subs := []Subscription{}
	for reqURL != "" {
		resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
		if err != nil {
			return nil, fmt.Errorf("list eligible child resources for management group %s: %w", mgID, err)
		}
		var result struct {
			Value []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
				Type string `json:"type"`
			} `json:"value"`
			NextLink string `json:"nextLink"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode eligible child resources response: %w", err)
		}
		resp.Body.Close()
		for _, item := range result.Value {
			if !strings.EqualFold(item.Type, "subscription") {
				continue
			}
			subID := SubscriptionIDFromScope(item.ID)
			if subID == "" {
				continue
			}
			subs = append(subs, Subscription{
				ID:          subID,
				DisplayName: item.Name,
			})
		}
		reqURL = result.NextLink
	}

	return subs, nil
}

func (c *Client) listManagementGroupSubscriptionsLegacy(mgID string) ([]Subscription, error) {
	reqURL := fmt.Sprintf("%s/providers/Microsoft.Management/managementGroups/%s/subscriptions?api-version=%s",
		ARMEndpoint, url.PathEscape(mgID), ManagementGroupSubscriptionsAPIVersion)

	subs := []Subscription{}
	for reqURL != "" {
		resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
		if err != nil {
			return nil, fmt.Errorf("list subscriptions for management group %s: %w", mgID, err)
		}
		var result struct {
			Value []struct {
				ID         string `json:"id"`
				Name       string `json:"name"`
				Properties struct {
					DisplayName string `json:"displayName"`
				} `json:"properties"`
			} `json:"value"`
			NextLink string `json:"nextLink"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode management group subscriptions: %w", err)
		}
		resp.Body.Close()
		for _, item := range result.Value {
			display := item.Properties.DisplayName
			if strings.TrimSpace(display) == "" {
				display = item.Name
			}
			subs = append(subs, Subscription{
				ID:          item.Name,
				DisplayName: display,
			})
		}
		reqURL = result.NextLink
	}

	return subs, nil
}

// ListSubscriptionResourceGroups lists resource groups for the given subscription ID
func (c *Client) ListSubscriptionResourceGroups(subscriptionID string) ([]ResourceGroup, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}

	if strings.TrimSpace(subscriptionID) == "" {
		return nil, fmt.Errorf("subscription id cannot be empty")
	}

	reqURL := fmt.Sprintf("%s/subscriptions/%s/resourceGroups?api-version=%s",
		ARMEndpoint, subscriptionID, ResourceGroupsAPIVersion)

	groups := []ResourceGroup{}
	for reqURL != "" {
		resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
		if err != nil {
			return nil, fmt.Errorf("list resource groups for subscription %s: %w", subscriptionID, err)
		}
		var result struct {
			Value []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"value"`
			NextLink string `json:"nextLink"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode resource groups: %w", err)
		}
		resp.Body.Close()
		for _, item := range result.Value {
			groups = append(groups, ResourceGroup{
				SubscriptionID: subscriptionID,
				Name:           item.Name,
				ID:             item.ID,
			})
		}
		reqURL = result.NextLink
	}

	return groups, nil
}

// GetActiveAssignments fetches all active PIM assignments for the user
func (c *Client) GetActiveAssignments(principalID string) ([]ActiveAssignment, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}

	reqURL := fmt.Sprintf("%s/providers/Microsoft.Authorization/roleAssignmentScheduleInstances?api-version=%s&$filter=asTarget()",
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
				StartDateTime    string `json:"startDateTime"`
				EndDateTime      string `json:"endDateTime"`
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
		return nil, fmt.Errorf("decode active assignments: %w", err)
	}

	assignments := make([]ActiveAssignment, 0)
	for _, item := range result.Value {
		if item.Properties.PrincipalID == principalID {
			assignments = append(assignments, ActiveAssignment{
				Name:             item.Name,
				Scope:            item.Properties.Scope,
				ScopeDisplay:     defaultScopeDisplay(item.Properties.Scope, item.Properties.ExpandedProps.Scope.DisplayName),
				RoleName:         item.Properties.ExpandedProps.RoleDefinition.DisplayName,
				RoleDefinitionID: item.Properties.RoleDefinitionID,
				EndDateTime:      resolveEndTime(item.Properties.EndDateTime, item.Properties.StartDateTime),
			})
		}
	}

	return assignments, nil
}

// IsRoleActive checks if a specific role is currently active at its default scope
func (c *Client) IsRoleActive(role Role, principalID string) (bool, error) {
	return c.isRoleActive(role.Scope, role.RoleDefinitionID, principalID)
}

func (c *Client) isRoleActive(scope, roleDefinitionID, principalID string) (bool, error) {
	if err := c.ensureTokens(); err != nil {
		return false, err
	}

	filter := fmt.Sprintf("principalId eq '%s' and roleDefinitionId eq '%s'", principalID, roleDefinitionID)
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/roleAssignmentSchedules?api-version=%s&$filter=%s",
		ARMEndpoint, scope, APIVersion, url.QueryEscape(filter))

	resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
	if err != nil {
		if isRetryableError(err) {
			return false, nil
		}
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

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "http 500")
}

// ActivateRole submits a role activation or extension request at the specified scope (defaults to role.Scope)
func (c *Client) ActivateRole(role Role, principalID, justification string, minutes int, targetScope string) (*ScheduleResponse, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}

	// Validate and clamp minutes
	minutes = clampMinutes(minutes)

	scopePath := role.Scope
	if strings.TrimSpace(targetScope) != "" {
		scopePath = targetScope
	}

	// Check if already active to determine request type
	active, err := c.isRoleActive(scopePath, role.RoleDefinitionID, principalID)
	if err != nil {
		return nil, err
	}

	requestType := "SelfActivate"
	if active {
		requestType = "SelfExtend"
	}

	scheduleReq := ScheduleRequest{
		Properties: ScheduleProperties{
			PrincipalID:                     principalID,
			RoleDefinitionID:                role.RoleDefinitionID,
			RequestType:                     requestType,
			Justification:                   justification,
			LinkedRoleEligibilityScheduleID: role.EligibilityScheduleID,
			ScheduleInfo: &ScheduleInfo{
				StartDateTime: time.Now().UTC().Format(time.RFC3339),
				Expiration: Expiration{
					Type:     "AfterDuration",
					Duration: formatDuration(minutes),
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
		ARMEndpoint, scopePath, requestID, APIVersion)

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

// clampMinutes ensures minutes is within valid range and rounds to 30-minute increments
func clampMinutes(minutes int) int {
	if minutes < MinMinutes {
		return MinMinutes
	}
	if minutes > MaxMinutes {
		return MaxMinutes
	}
	// Round to nearest 30 minutes
	return ((minutes + 15) / 30) * 30
}

// formatDuration converts minutes to ISO 8601 duration format (PT1H30M)
func formatDuration(minutes int) string {
	hours := minutes / 60
	mins := minutes % 60
	if mins == 0 {
		return fmt.Sprintf("PT%dH", hours)
	}
	if hours == 0 {
		return fmt.Sprintf("PT%dM", mins)
	}
	return fmt.Sprintf("PT%dH%dM", hours, mins)
}

func allowDeviceLogin() bool {
	val := strings.ToLower(os.Getenv("PIM_ALLOW_DEVICE_LOGIN"))
	return val == "1" || val == "true" || val == "yes"
}

func defaultScopeDisplay(scope, display string) string {
	if strings.TrimSpace(display) != "" {
		return display
	}
	switch {
	case strings.HasPrefix(scope, "/subscriptions/"):
		parts := strings.Split(scope, "/")
		if len(parts) >= 3 {
			return parts[2]
		}
	case strings.HasPrefix(scope, "/providers/Microsoft.Management/managementGroups/"):
		parts := strings.Split(scope, "/")
		if len(parts) >= 5 {
			return parts[4]
		}
	case strings.Contains(scope, "/resourceGroups/"):
		idx := strings.Index(scope, "/resourceGroups/")
		if idx != -1 {
			remainder := scope[idx+len("/resourceGroups/"):]
			if slash := strings.Index(remainder, "/"); slash != -1 {
				remainder = remainder[:slash]
			}
			return remainder
		}
	}
	return scope
}

func resolveEndTime(end, start string) string {
	if end != "" {
		return end
	}
	return ""
}
