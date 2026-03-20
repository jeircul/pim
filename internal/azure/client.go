package azure

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

const (
	apiVersion                             = "2020-10-01"
	managementGroupSubscriptionsAPIVersion = "2023-04-01"
	eligibleChildResourcesAPIVersion       = "2020-10-01"
	resourceGroupsAPIVersion               = "2021-04-01"
	armEndpoint                            = "https://management.azure.com"
	graphEndpoint                          = "https://graph.microsoft.com/v1.0"
	httpTimeout                            = 30 * time.Second
)

// Client handles Azure PIM operations.
type Client struct {
	cred       azcore.TokenCredential
	httpClient *http.Client
	armToken   string
	graphToken string
	ctx        context.Context
}

type childResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

// NewClient creates a PIM client using the best available delegated credential.
func NewClient(ctx context.Context) (*Client, error) {
	tenantID := os.Getenv("AZURE_TENANT_ID")
	var chain []azcore.TokenCredential

	if c, err := azidentity.NewAzureCLICredential(&azidentity.AzureCLICredentialOptions{TenantID: tenantID}); err == nil {
		chain = append(chain, c)
	}
	if c, err := azidentity.NewAzurePowerShellCredential(&azidentity.AzurePowerShellCredentialOptions{TenantID: tenantID}); err == nil {
		chain = append(chain, c)
	}
	if allowDeviceLogin() {
		if c, err := azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
			TenantID: tenantID,
			UserPrompt: func(_ context.Context, msg azidentity.DeviceCodeMessage) error {
				fmt.Fprintln(os.Stderr, msg.Message)
				return nil
			},
		}); err == nil {
			chain = append(chain, c)
		}
	}

	if len(chain) == 0 {
		return nil, ErrNoCredential
	}

	cred, err := azidentity.NewChainedTokenCredential(chain, nil)
	if err != nil {
		return nil, fmt.Errorf("create credential chain: %w", err)
	}

	return &Client{
		cred:       cred,
		httpClient: &http.Client{Timeout: httpTimeout},
		ctx:        ctx,
	}, nil
}

func (c *Client) getToken(scope string) (string, error) {
	tok, err := c.cred.GetToken(c.ctx, policy.TokenRequestOptions{Scopes: []string{scope}})
	if err != nil {
		return "", fmt.Errorf("acquire token for %s: %w", scope, err)
	}
	return tok.Token, nil
}

func (c *Client) ensureTokens() error {
	if c.armToken == "" {
		tok, err := c.getToken("https://management.azure.com/.default")
		if err != nil {
			return err
		}
		c.armToken = tok
	}
	if c.graphToken == "" {
		tok, err := c.getToken("https://graph.microsoft.com/.default")
		if err != nil {
			return err
		}
		c.graphToken = tok
	}
	return nil
}

func (c *Client) doRequest(method, reqURL, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(c.ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "pim/2")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errorFromResponse(resp)
	}
	return resp, nil
}

func errorFromResponse(resp *http.Response) error {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var azErr struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &azErr) == nil && azErr.Error.Code != "" {
		return fmt.Errorf("HTTP %d: %s - %s", resp.StatusCode, azErr.Error.Code, azErr.Error.Message)
	}
	return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
}

// GetCurrentUser fetches the current user from Microsoft Graph.
func (c *Client) GetCurrentUser() (*User, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}
	resp, err := c.doRequest(http.MethodGet, graphEndpoint+"/me", c.graphToken, nil)
	if err != nil {
		return nil, fmt.Errorf("get current user: %w", err)
	}
	defer resp.Body.Close()

	var u User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, fmt.Errorf("decode user: %w", err)
	}
	return &u, nil
}

// GetEligibleRoles fetches all eligible PIM roles for the current user.
func (c *Client) GetEligibleRoles() ([]Role, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("%s/providers/Microsoft.Authorization/roleEligibilitySchedules?api-version=%s&$filter=asTarget()",
		armEndpoint, apiVersion)

	resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
	if err != nil {
		return nil, fmt.Errorf("get eligible roles: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Value []struct {
			ID         string `json:"id"`
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
			ScopeDisplay:          DefaultScopeDisplay(item.Properties.Scope, item.Properties.ExpandedProps.Scope.DisplayName),
			RoleName:              item.Properties.ExpandedProps.RoleDefinition.DisplayName,
			RoleDefinitionID:      item.Properties.RoleDefinitionID,
			EligibilityScheduleID: item.ID,
		})
	}
	return roles, nil
}

// GetActiveAssignments fetches all active PIM assignments for the calling user.
func (c *Client) GetActiveAssignments() ([]ActiveAssignment, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("%s/providers/Microsoft.Authorization/roleAssignmentScheduleInstances?api-version=%s&$filter=asTarget()",
		armEndpoint, apiVersion)

	resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
	if err != nil {
		return nil, fmt.Errorf("get active assignments: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Value []struct {
			Properties struct {
				PrincipalID      string `json:"principalId"`
				Scope            string `json:"scope"`
				RoleDefinitionID string `json:"roleDefinitionId"`
				MemberType       string `json:"memberType"`
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

	out := make([]ActiveAssignment, 0)
	for _, item := range result.Value {
		p := item.Properties
		out = append(out, ActiveAssignment{
			Scope:            p.Scope,
			ScopeDisplay:     DefaultScopeDisplay(p.Scope, p.ExpandedProps.Scope.DisplayName),
			RoleName:         p.ExpandedProps.RoleDefinition.DisplayName,
			RoleDefinitionID: p.RoleDefinitionID,
			EndDateTime:      p.EndDateTime,
			MemberType:       p.MemberType,
		})
	}
	return out, nil
}

// IsRoleActive checks if the given role is currently active at its scope.
func (c *Client) IsRoleActive(role Role, principalID string) (bool, error) {
	return c.isRoleActiveAt(role.Scope, role.RoleDefinitionID, principalID)
}

func (c *Client) isRoleActiveAt(scope, roleDefinitionID, principalID string) (bool, error) {
	if err := c.ensureTokens(); err != nil {
		return false, err
	}
	filter := fmt.Sprintf("principalId eq '%s' and roleDefinitionId eq '%s'", principalID, roleDefinitionID)
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/roleAssignmentSchedules?api-version=%s&$filter=%s",
		armEndpoint, scope, apiVersion, url.QueryEscape(filter))

	resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 500") || strings.Contains(err.Error(), "HTTP 400") {
			return false, nil
		}
		return false, fmt.Errorf("check active status: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Value []any `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode active check: %w", err)
	}
	return len(result.Value) > 0, nil
}

// ActivateRole submits an activation or extension request.
func (c *Client) ActivateRole(role Role, principalID, justification string, minutes int, targetScope string) (*ScheduleResponse, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}
	minutes = ClampMinutes(minutes)

	scopePath := role.Scope
	if strings.TrimSpace(targetScope) != "" {
		scopePath = targetScope
	}

	active, err := c.isRoleActiveAt(scopePath, role.RoleDefinitionID, principalID)
	if err != nil {
		return nil, err
	}
	requestType := "SelfActivate"
	if active {
		requestType = "SelfExtend"
	}

	// Omit the eligibility schedule link when activating at a narrower scope
	// than the eligibility (e.g. sub-scoped eligibility, RG-scoped activation).
	// Azure rejects a subscription-scoped schedule ID on an RG-scoped request.
	linkedID := role.EligibilityScheduleID
	if targetScope != "" && targetScope != role.Scope {
		linkedID = ""
	}

	req := ScheduleRequest{
		Properties: ScheduleProperties{
			PrincipalID:                     principalID,
			RoleDefinitionID:                role.RoleDefinitionID,
			RequestType:                     requestType,
			Justification:                   justification,
			LinkedRoleEligibilityScheduleID: linkedID,
			ScheduleInfo: &ScheduleInfo{
				StartDateTime: time.Now().UTC().Format(time.RFC3339),
				Expiration: Expiration{
					Type:     "AfterDuration",
					Duration: FormatDuration(minutes),
				},
			},
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	requestID := uuid.New().String()
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/%s?api-version=%s",
		armEndpoint, scopePath, requestID, apiVersion)

	resp, err := c.doRequest(http.MethodPut, reqURL, c.armToken, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("submit activation: %w", err)
	}
	defer resp.Body.Close()

	var schedResp ScheduleResponse
	if err := json.NewDecoder(resp.Body).Decode(&schedResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &schedResp, nil
}

// DeactivateRole submits a role deactivation request.
func (c *Client) DeactivateRole(assignment ActiveAssignment, principalID string) (*ScheduleResponse, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}

	req := ScheduleRequest{
		Properties: ScheduleProperties{
			PrincipalID:      principalID,
			RoleDefinitionID: assignment.RoleDefinitionID,
			RequestType:      "SelfDeactivate",
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	requestID := uuid.New().String()
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/roleAssignmentScheduleRequests/%s?api-version=%s",
		armEndpoint, assignment.Scope, requestID, apiVersion)

	resp, err := c.doRequest(http.MethodPut, reqURL, c.armToken, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("submit deactivation: %w", err)
	}
	defer resp.Body.Close()

	var schedResp ScheduleResponse
	if err := json.NewDecoder(resp.Body).Decode(&schedResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &schedResp, nil
}

// ListManagementGroupSubscriptions returns subscriptions under a management group.
func (c *Client) ListManagementGroupSubscriptions(mgID string) ([]Subscription, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}
	mgID = strings.TrimSpace(mgID)
	if mgID == "" {
		return nil, fmt.Errorf("management group id cannot be empty")
	}

	subs, err := c.listEligibleChildSubscriptions(mgID)
	if err == nil && len(subs) > 0 {
		return subs, nil
	}

	legacy, legacyErr := c.listMGSubscriptionsLegacy(mgID)
	if legacyErr == nil {
		return legacy, nil
	}
	if err != nil {
		return nil, fmt.Errorf("eligible child resources: %w; legacy: %v", err, legacyErr)
	}
	return nil, legacyErr
}

// ListManagementGroupResourceGroups returns resource groups under a management group.
func (c *Client) ListManagementGroupResourceGroups(mgID string) ([]ResourceGroup, error) {
	resources, err := c.fetchEligibleChildResources(mgID)
	if err != nil {
		return nil, err
	}
	out := make([]ResourceGroup, 0, len(resources))
	for _, item := range resources {
		if !strings.Contains(strings.ToLower(item.Type), "resourcegroup") {
			continue
		}
		subID, name := ResourceGroupNameFromScope(item.ID)
		if subID == "" || name == "" {
			continue
		}
		out = append(out, ResourceGroup{SubscriptionID: subID, Name: name, ID: item.ID})
	}
	return out, nil
}

// ListSubscriptionResourceGroups lists resource groups for a subscription.
func (c *Client) ListSubscriptionResourceGroups(subscriptionID string) ([]ResourceGroup, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(subscriptionID) == "" {
		return nil, fmt.Errorf("subscription id cannot be empty")
	}

	reqURL := fmt.Sprintf("%s/subscriptions/%s/resourceGroups?api-version=%s",
		armEndpoint, subscriptionID, resourceGroupsAPIVersion)

	var out []ResourceGroup
	for reqURL != "" {
		resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
		if err != nil {
			return nil, fmt.Errorf("list resource groups for %s: %w", subscriptionID, err)
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
			out = append(out, ResourceGroup{SubscriptionID: subscriptionID, Name: item.Name, ID: item.ID})
		}
		reqURL = result.NextLink
	}
	return out, nil
}

// fetchEligibleChildResources fetches PIM-eligible child resources under any
// ARM scope (management group or subscription). scope must be the full ARM
// scope path, e.g. "/providers/Microsoft.Management/managementGroups/{id}"
// or "/subscriptions/{id}".
func (c *Client) fetchEligibleChildResources(scope string) ([]childResource, error) {
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/eligibleChildResources?api-version=%s",
		armEndpoint, scope, eligibleChildResourcesAPIVersion)

	var out []childResource
	for reqURL != "" {
		resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
		if err != nil {
			return nil, fmt.Errorf("eligible child resources for %s: %w", scope, err)
		}
		var result struct {
			Value    []childResource `json:"value"`
			NextLink string          `json:"nextLink"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode child resources: %w", err)
		}
		resp.Body.Close()
		out = append(out, result.Value...)
		reqURL = result.NextLink
	}
	return out, nil
}

func (c *Client) listEligibleChildSubscriptions(mgID string) ([]Subscription, error) {
	scope := fmt.Sprintf("/providers/Microsoft.Management/managementGroups/%s", url.PathEscape(mgID))
	resources, err := c.fetchEligibleChildResources(scope)
	if err != nil {
		return nil, err
	}
	out := make([]Subscription, 0, len(resources))
	for _, item := range resources {
		if !strings.Contains(strings.ToLower(item.Type), "subscription") {
			continue
		}
		subID := SubscriptionIDFromScope(item.ID)
		if subID == "" {
			continue
		}
		out = append(out, Subscription{ID: subID, DisplayName: item.Name})
	}
	return out, nil
}

// ListEligibleResourceGroups lists resource groups the caller is eligible to
// manage via PIM under the given subscription. Uses the PIM
// eligibleChildResources API so it works before any RBAC is granted.
func (c *Client) ListEligibleResourceGroups(subscriptionID string) ([]ResourceGroup, error) {
	if err := c.ensureTokens(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(subscriptionID) == "" {
		return nil, fmt.Errorf("subscription id cannot be empty")
	}
	scope := "/subscriptions/" + subscriptionID
	resources, err := c.fetchEligibleChildResources(scope)
	if err != nil {
		return nil, err
	}
	out := make([]ResourceGroup, 0, len(resources))
	for _, item := range resources {
		if !strings.Contains(strings.ToLower(item.Type), "resourcegroup") {
			continue
		}
		_, name := ResourceGroupNameFromScope(item.ID)
		if name == "" {
			continue
		}
		out = append(out, ResourceGroup{SubscriptionID: subscriptionID, Name: name, ID: item.ID})
	}
	return out, nil
}

func (c *Client) listMGSubscriptionsLegacy(mgID string) ([]Subscription, error) {
	reqURL := fmt.Sprintf("%s/providers/Microsoft.Management/managementGroups/%s/subscriptions?api-version=%s",
		armEndpoint, url.PathEscape(mgID), managementGroupSubscriptionsAPIVersion)

	var out []Subscription
	for reqURL != "" {
		resp, err := c.doRequest(http.MethodGet, reqURL, c.armToken, nil)
		if err != nil {
			return nil, fmt.Errorf("list subscriptions for %s: %w", mgID, err)
		}
		var result struct {
			Value []struct {
				Name       string `json:"name"`
				Properties struct {
					DisplayName string `json:"displayName"`
				} `json:"properties"`
			} `json:"value"`
			NextLink string `json:"nextLink"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode mg subscriptions: %w", err)
		}
		resp.Body.Close()
		for _, item := range result.Value {
			display := item.Properties.DisplayName
			if strings.TrimSpace(display) == "" {
				display = item.Name
			}
			out = append(out, Subscription{ID: item.Name, DisplayName: display})
		}
		reqURL = result.NextLink
	}
	return out, nil
}

func allowDeviceLogin() bool {
	v := strings.ToLower(os.Getenv("PIM_ALLOW_DEVICE_LOGIN"))
	return v == "1" || v == "true" || v == "yes"
}
