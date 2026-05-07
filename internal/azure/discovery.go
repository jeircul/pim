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

	mgs, subs := classifyChildResources(resources)
	return mgs, subs, nil
}

func classifyChildResources(resources []childResource) ([]ManagementGroup, []Subscription) {
	var mgs []ManagementGroup
	var subs []Subscription
	for _, item := range resources {
		lower := strings.ToLower(item.Type)
		switch {
		case strings.Contains(lower, "resourcegroup"):
		case strings.Contains(lower, "managementgroup"):
			mgs = append(mgs, ManagementGroup{ID: item.Name, DisplayName: displayOr(item)})
		case strings.Contains(lower, "subscription"):
			subID := SubscriptionIDFromScope(item.ID)
			if subID != "" {
				subs = append(subs, Subscription{ID: subID, DisplayName: displayOr(item)})
			}
		}
	}
	return mgs, subs
}

// ListAllSubscriptionsUnderMG returns all subscriptions reachable under a
// management group by recursively expanding child management groups.
// The ARM token is acquired once and reused for all requests.
// Inaccessible intermediate nodes are skipped; their paths are returned as
// warnings so callers can surface them without aborting.
func (c *Client) ListAllSubscriptionsUnderMG(ctx context.Context, mgID string) (subs []Subscription, warnings []string, err error) {
	token, err := c.armToken(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("acquire ARM token: %w", err)
	}

	visited := map[string]struct{}{}
	queue := []string{mgID}

	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]

		key := strings.ToLower(id)
		if _, seen := visited[key]; seen {
			continue
		}
		visited[key] = struct{}{}

		mgs, ss, e := c.listManagementGroupChildrenWithToken(ctx, id, token)
		if e != nil {
			if id == mgID {
				return nil, nil, fmt.Errorf("list children of management group %s: %w", id, e)
			}
			warnings = append(warnings, fmt.Sprintf("skip MG %s: %v", id, e))
			continue
		}
		subs = append(subs, ss...)
		for _, child := range mgs {
			queue = append(queue, child.ID)
		}
	}

	return subs, warnings, nil
}

func (c *Client) listManagementGroupChildrenWithToken(ctx context.Context, mgID, token string) ([]ManagementGroup, []Subscription, error) {
	mgID = strings.TrimSpace(mgID)
	if mgID == "" {
		return nil, nil, fmt.Errorf("management group id cannot be empty")
	}
	scope := fmt.Sprintf("/providers/Microsoft.Management/managementGroups/%s", url.PathEscape(mgID))
	resources, err := c.fetchEligibleChildResourcesWithToken(ctx, scope, token)
	if err != nil {
		return nil, nil, err
	}
	mgs, subs := classifyChildResources(resources)
	return mgs, subs, nil
}

func (c *Client) fetchEligibleChildResourcesWithToken(ctx context.Context, scope, token string) ([]childResource, error) {
	reqURL := fmt.Sprintf("%s%s/providers/Microsoft.Authorization/eligibleChildResources?api-version=%s&$getAllChildren=true",
		armEndpoint, scope, eligibleChildResourcesAPIVersion)

	var out []childResource
	for reqURL != "" {
		resp, err := c.doRequest(ctx, http.MethodGet, reqURL, token, nil)
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
