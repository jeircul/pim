package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ListManagementGroupChildren returns the direct eligible children of a management group.
// Children may be management groups or subscriptions.
func (c *Client) ListManagementGroupChildren(ctx context.Context, mgID string) ([]ManagementGroup, []Subscription, error) {
	mgID = strings.TrimSpace(mgID)
	if mgID == "" {
		return nil, nil, fmt.Errorf("management group id cannot be empty")
	}

	scope := fmt.Sprintf("/providers/Microsoft.Management/managementGroups/%s", url.PathEscape(mgID))
	resources, err := c.fetchEligibleChildResources(ctx, scope)
	if err != nil {
		return nil, nil, err
	}

	var mgs []ManagementGroup
	var subs []Subscription
	for _, item := range resources {
		lower := strings.ToLower(item.Type)
		switch {
		case strings.HasSuffix(lower, "/resourcegroups"):
			// not a direct child of an MG; ignore
		case strings.HasSuffix(lower, "/managementgroups"):
			mgs = append(mgs, ManagementGroup{ID: item.Name, DisplayName: displayOr(item)})
		case strings.HasSuffix(lower, "/subscriptions"):
			subID := SubscriptionIDFromScope(item.ID)
			if subID != "" {
				subs = append(subs, Subscription{ID: subID, DisplayName: displayOr(item)})
			}
		}
	}
	return mgs, subs, nil
}

// ListManagementGroupSubscriptions returns subscriptions under a management group
// that the caller is PIM-eligible to activate. Uses the eligibleChildResources API
// with getAllChildren=true so nested subscriptions are included. An empty result
// means no eligible child scopes exist, which is valid and not an error.
func (c *Client) ListManagementGroupSubscriptions(ctx context.Context, mgID string) ([]Subscription, error) {
	_, subs, err := c.ListManagementGroupChildren(ctx, mgID)
	return subs, err
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
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/eligibleChildResources?api-version=%s",
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

func displayOr(item childResource) string {
	if item.Properties.DisplayName != "" {
		return item.Properties.DisplayName
	}
	return item.Name
}
