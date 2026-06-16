package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ListManagementGroupChildren returns the direct eligible children of a management group.
// Children may be management groups or subscriptions.
func (c *Client) ListManagementGroupChildren(ctx context.Context, mgID string) ([]ManagementGroup, []Subscription, error) {
	mgID = strings.TrimSpace(mgID)
	if mgID == "" {
		return nil, nil, fmt.Errorf("management group id cannot be empty")
	}

	scope := fmt.Sprintf("/providers/Microsoft.Management/managementGroups/%s", url.PathEscape(mgID))
	tok, err := c.armToken(ctx)
	if err != nil {
		return nil, nil, err
	}
	resources, err := c.fetchEligibleChildResourcesWithToken(ctx, scope, tok)
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

// mgNodeTimeoutDefault bounds each management-group node lookup so one stalled
// node does not delay its siblings. Override with PIM_MG_NODE_TIMEOUT (e.g. "45s").
const mgNodeTimeoutDefault = 15 * time.Second

func mgNodeTimeout() time.Duration {
	if v := os.Getenv("PIM_MG_NODE_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return mgNodeTimeoutDefault
}

// ListAllSubscriptionsUnderMG returns all subscriptions reachable under a
// management group by recursively expanding child management groups.
// The ARM token is acquired once and reused for all requests.
// Sibling nodes are expanded concurrently (bounded to 8 workers) so a stalled
// node does not delay its siblings. Inaccessible intermediate nodes are
// skipped; their paths are returned as warnings so callers can surface them
// without aborting.
// parents maps lowercased subscription ID to the MG that directly contained it
// during the BFS walk.
func (c *Client) ListAllSubscriptionsUnderMG(ctx context.Context, mgID string) (subs []Subscription, parents map[string]string, warnings []string, err error) {
	token, err := c.armToken(ctx)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("acquire ARM token: %w", err)
	}

	const workers = 8
	sem := make(chan struct{}, workers)
	nodeTO := mgNodeTimeout()

	parents = map[string]string{}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		visited = map[string]struct{}{}
	)

	var enqueue func(id string, depth int)
	enqueue = func(id string, depth int) {
		key := strings.ToLower(id)
		if _, seen := visited[key]; seen {
			return
		}
		visited[key] = struct{}{}

		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			deadline := time.Now().Add(nodeTO)
			if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
				deadline = d
			}
			nodeCtx, cancel := context.WithDeadline(ctx, deadline)
			mgs, ss, e := c.listManagementGroupChildrenWithToken(nodeCtx, id, token)
			cancel()

			mu.Lock()
			defer mu.Unlock()
			if e != nil {
				if depth == 0 {
					if err == nil {
						err = fmt.Errorf("list children of management group %s: %w", id, e)
					}
					return
				}
				warnings = append(warnings, fmt.Sprintf("skip MG %s: %v", id, e))
				return
			}
			subs = append(subs, ss...)
			for _, s := range ss {
				k := strings.ToLower(s.ID)
				if _, exists := parents[k]; !exists {
					parents[k] = id
				}
			}
			for _, child := range mgs {
				enqueue(child.ID, depth+1)
			}
		}()
	}

	mu.Lock()
	enqueue(mgID, 0)
	mu.Unlock()

	wg.Wait()
	return subs, parents, warnings, err
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

// ListEligibleResourceGroups lists resource groups the caller is eligible to
// manage via PIM under the given subscription. Uses the PIM
// eligibleChildResources API so it works before any RBAC is granted.
func (c *Client) ListEligibleResourceGroups(ctx context.Context, subscriptionID string) ([]ResourceGroup, error) {
	if strings.TrimSpace(subscriptionID) == "" {
		return nil, fmt.Errorf("subscription id cannot be empty")
	}
	scope := "/subscriptions/" + subscriptionID
	tok, err := c.armToken(ctx)
	if err != nil {
		return nil, err
	}
	resources, err := c.fetchEligibleChildResourcesWithToken(ctx, scope, tok)
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
