package azure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// GetCurrentUser fetches the current user from Microsoft Graph.
func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	tok, err := c.graphToken(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.doRequest(ctx, http.MethodGet, graphEndpoint+"/me", tok, nil)
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

// GetEligibleRoles fetches all eligible PIM roles for the current user, following pagination.
func (c *Client) GetEligibleRoles(ctx context.Context) ([]Role, error) {
	tok, err := c.armToken(ctx)
	if err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("%s/providers/Microsoft.Authorization/roleEligibilitySchedules?api-version=%s&$filter=asTarget()",
		armEndpoint, apiVersion)

	var roles []Role
	for reqURL != "" {
		resp, err := c.doRequest(ctx, http.MethodGet, reqURL, tok, nil)
		if err != nil {
			return nil, fmt.Errorf("get eligible roles: %w", err)
		}

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
			NextLink string `json:"nextLink"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode roles: %w", err)
		}
		resp.Body.Close()

		for _, item := range result.Value {
			roles = append(roles, Role{
				Scope:                 item.Properties.Scope,
				ScopeDisplay:          DefaultScopeDisplay(item.Properties.Scope, item.Properties.ExpandedProps.Scope.DisplayName),
				RoleName:              item.Properties.ExpandedProps.RoleDefinition.DisplayName,
				RoleDefinitionID:      item.Properties.RoleDefinitionID,
				EligibilityScheduleID: item.ID,
			})
		}
		reqURL = result.NextLink
	}
	return roles, nil
}

// GetActiveAssignments fetches all active PIM assignments for the calling user, following pagination.
func (c *Client) GetActiveAssignments(ctx context.Context) ([]ActiveAssignment, error) {
	tok, err := c.armToken(ctx)
	if err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("%s/providers/Microsoft.Authorization/roleAssignmentScheduleInstances?api-version=%s&$filter=asTarget()",
		armEndpoint, apiVersion)

	var out []ActiveAssignment
	for reqURL != "" {
		resp, err := c.doRequest(ctx, http.MethodGet, reqURL, tok, nil)
		if err != nil {
			return nil, fmt.Errorf("get active assignments: %w", err)
		}

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
			NextLink string `json:"nextLink"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("decode active assignments: %w", err)
		}
		resp.Body.Close()

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
		reqURL = result.NextLink
	}
	return out, nil
}

// IsRoleActive checks if the given role is currently active at its scope.
func (c *Client) IsRoleActive(ctx context.Context, role Role, principalID string) (bool, error) {
	return c.isRoleActiveAt(ctx, role.Scope, role.RoleDefinitionID, principalID)
}

func (c *Client) isRoleActiveAt(ctx context.Context, scope, roleDefinitionID, principalID string) (bool, error) {
	tok, err := c.armToken(ctx)
	if err != nil {
		return false, err
	}
	filter := fmt.Sprintf("principalId eq '%s' and roleDefinitionId eq '%s'", principalID, roleDefinitionID)
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/roleAssignmentSchedules?api-version=%s&$filter=%s",
		armEndpoint, scope, apiVersion, url.QueryEscape(filter))

	resp, err := c.doRequest(ctx, http.MethodGet, reqURL, tok, nil)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 500 {
			return false, nil
		}
		return false, fmt.Errorf("check active status: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("read active check response: %w", err)
	}
	if len(body) == 0 {
		return false, nil
	}

	var result struct {
		Value []any `json:"value"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false, fmt.Errorf("decode active check: %w", err)
	}
	return len(result.Value) > 0, nil
}
