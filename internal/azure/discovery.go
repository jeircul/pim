package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ListManagementGroupSubscriptions returns subscriptions under a management group
// that the caller is PIM-eligible to activate. Uses the eligibleChildResources API
// with getAllChildren=true so nested subscriptions are included. An empty result
// means no eligible child scopes exist, which is valid and not an error.
func (c *Client) ListManagementGroupSubscriptions(ctx context.Context, mgID string) ([]Subscription, error) {
	mgID = strings.TrimSpace(mgID)
	if mgID == "" {
		return nil, fmt.Errorf("management group id cannot be empty")
	}

	subs, err := c.listEligibleChildSubscriptions(ctx, mgID)
	if err != nil {
		return nil, err
	}
	return subs, nil
}

// fetchEligibleChildResources fetches PIM-eligible child resources under any
// ARM scope (management group or subscription). scope must be the full ARM
// scope path, e.g. "/providers/Microsoft.Management/managementGroups/{id}"
// or "/subscriptions/{id}".
func (c *Client) fetchEligibleChildResources(ctx context.Context, scope string) ([]childResource, error) {
	tok, err := c.armToken(ctx)
	if err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/eligibleChildResources?api-version=%s&$getAllChildren=true",
		armEndpoint, scope, eligibleChildResourcesAPIVersion)

	var out []childResource
	for reqURL != "" {
		resp, err := c.doRequest(ctx, http.MethodGet, reqURL, tok, nil)
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

func (c *Client) listEligibleChildSubscriptions(ctx context.Context, mgID string) ([]Subscription, error) {
	scope := fmt.Sprintf("/providers/Microsoft.Management/managementGroups/%s", url.PathEscape(mgID))
	resources, err := c.fetchEligibleChildResources(ctx, scope)
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
func (c *Client) ListEligibleResourceGroups(ctx context.Context, subscriptionID string) ([]ResourceGroup, error) {
	if strings.TrimSpace(subscriptionID) == "" {
		return nil, fmt.Errorf("subscription id cannot be empty")
	}
	scope := "/subscriptions/" + subscriptionID
	resources, err := c.fetchEligibleChildResources(ctx, scope)
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

