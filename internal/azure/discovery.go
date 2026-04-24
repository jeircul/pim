package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ListManagementGroupSubscriptions returns subscriptions under a management group.
func (c *Client) ListManagementGroupSubscriptions(ctx context.Context, mgID string) ([]Subscription, error) {
	mgID = strings.TrimSpace(mgID)
	if mgID == "" {
		return nil, fmt.Errorf("management group id cannot be empty")
	}

	subs, err := c.listEligibleChildSubscriptions(ctx, mgID)
	if err == nil && len(subs) > 0 {
		return subs, nil
	}

	legacy, legacyErr := c.listMGSubscriptionsLegacy(ctx, mgID)
	if legacyErr == nil {
		return legacy, nil
	}
	if err != nil {
		return nil, fmt.Errorf("eligible child resources: %w; legacy: %v", err, legacyErr)
	}
	return nil, legacyErr
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

func (c *Client) listMGSubscriptionsLegacy(ctx context.Context, mgID string) ([]Subscription, error) {
	tok, err := c.armToken(ctx)
	if err != nil {
		return nil, err
	}
	reqURL := fmt.Sprintf("%s/providers/Microsoft.Management/managementGroups/%s/subscriptions?api-version=%s",
		armEndpoint, url.PathEscape(mgID), managementGroupSubscriptionsAPIVersion)

	var out []Subscription
	for reqURL != "" {
		resp, err := c.doRequest(ctx, http.MethodGet, reqURL, tok, nil)
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
