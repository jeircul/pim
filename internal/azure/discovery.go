package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
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
// management group by recursively expanding child management groups in parallel.
// Inaccessible intermediate nodes are skipped; their paths are returned as
// warnings so callers can surface them without aborting.
func (c *Client) ListAllSubscriptionsUnderMG(ctx context.Context, mgID string) (subs []Subscription, warnings []string, err error) {
	if _, err = c.armToken(ctx); err != nil {
		return nil, nil, fmt.Errorf("acquire ARM token: %w", err)
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 4)
		visited = map[string]struct{}{}
	)

	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		defer wg.Done()
		sem <- struct{}{}
		defer func() { <-sem }()

		mu.Lock()
		key := strings.ToLower(id)
		if _, seen := visited[key]; seen {
			mu.Unlock()
			return
		}
		visited[key] = struct{}{}
		mu.Unlock()

		mgs, ss, e := c.ListManagementGroupChildren(ctx, id)
		if e != nil {
			if depth > 0 {
				mu.Lock()
				warnings = append(warnings, fmt.Sprintf("skip MG %s: %v", id, e))
				mu.Unlock()
				return
			}
			mu.Lock()
			if err == nil {
				err = fmt.Errorf("list children of management group %s: %w", id, e)
			}
			mu.Unlock()
			return
		}

		mu.Lock()
		subs = append(subs, ss...)
		mu.Unlock()

		for _, child := range mgs {
			wg.Add(1)
			go walk(child.ID, depth+1)
		}
	}

	wg.Add(1)
	go walk(mgID, 0)
	wg.Wait()

	return subs, warnings, err
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
