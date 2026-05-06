package headless

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/jeircul/pim/internal/app"
	"github.com/jeircul/pim/internal/azure"
)

// SearchHit is a single activatable subscription returned by pim search.
type SearchHit struct {
	SubscriptionID  string   `json:"subscriptionId"`
	DisplayName     string   `json:"displayName"`
	ManagementGroup string   `json:"managementGroup,omitempty"`
	EligibleRoles   []string `json:"eligibleRoles"`
}

// runSearch lists PIM-eligible subscriptions, optionally filtered by query.
func runSearch(ctx context.Context, a *app.App, client ClientAPI, out io.Writer) error {
	roles, err := client.GetEligibleRoles(ctx)
	if err != nil {
		return fmt.Errorf("get eligible roles: %w", err)
	}

	hits, err := buildSearchHits(ctx, client, roles)
	if err != nil {
		return err
	}

	q := strings.TrimSpace(a.Config.SearchQuery)
	hits = filterSearchHits(hits, q)

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].DisplayName != hits[j].DisplayName {
			return hits[i].DisplayName < hits[j].DisplayName
		}
		return hits[i].SubscriptionID < hits[j].SubscriptionID
	})

	if a.Config.Output == app.OutputJSON {
		if hits == nil {
			hits = []SearchHit{}
		}
		return jsonOut(hits, out)
	}

	if len(hits) == 0 {
		fmt.Fprintln(out, "no matching eligible subscriptions")
		return nil
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SUBSCRIPTION\tGUID\tMANAGEMENT GROUP\tELIGIBLE ROLES")
	for _, h := range hits {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			h.DisplayName, h.SubscriptionID, h.ManagementGroup, strings.Join(h.EligibleRoles, ","))
	}
	return tw.Flush()
}

// buildSearchHits walks all eligible roles and flattens them into a deduplicated
// list of activatable subscriptions. MG-scoped roles are expanded via
// ListManagementGroupSubscriptions (cached per MG). RG-scoped roles are excluded.
// Roles for the same subscription are merged into one hit.
func buildSearchHits(ctx context.Context, client ClientAPI, roles []azure.Role) ([]SearchHit, error) {
	type acc struct {
		display string
		mg      string
		roles   map[string]struct{}
	}
	bySub := map[string]*acc{}
	mgCache := map[string][]azure.Subscription{}

	add := func(subID, display, mg, roleName string) {
		key := strings.ToLower(subID)
		a, ok := bySub[key]
		if !ok {
			a = &acc{display: display, mg: mg, roles: map[string]struct{}{}}
			bySub[key] = a
		}
		if a.display == "" {
			a.display = display
		}
		if a.mg == "" {
			a.mg = mg
		}
		a.roles[roleName] = struct{}{}
	}

	for _, r := range roles {
		switch r.ScopeKind() {
		case azure.ScopeSubscription:
			add(azure.SubscriptionIDFromScope(r.Scope), r.ScopeDisplay, "", r.RoleName)
		case azure.ScopeManagementGroup:
			mgID := azure.ManagementGroupIDFromScope(r.Scope)
			subs, ok := mgCache[mgID]
			if !ok {
				list, err := client.ListManagementGroupSubscriptions(ctx, mgID)
				if err != nil {
					return nil, fmt.Errorf("list subscriptions under management group %s: %w", mgID, err)
				}
				subs = list
				mgCache[mgID] = subs
			}
			for _, s := range subs {
				add(s.ID, s.DisplayName, mgID, r.RoleName)
			}
		}
	}

	out := make([]SearchHit, 0, len(bySub))
	for id, a := range bySub {
		names := make([]string, 0, len(a.roles))
		for n := range a.roles {
			names = append(names, n)
		}
		sort.Strings(names)
		out = append(out, SearchHit{
			SubscriptionID:  id,
			DisplayName:     a.display,
			ManagementGroup: a.mg,
			EligibleRoles:   names,
		})
	}
	return out, nil
}

// filterSearchHits applies exact-first / substring-fallback on SubscriptionID
// and DisplayName. Empty query returns all hits unchanged.
func filterSearchHits(hits []SearchHit, query string) []SearchHit {
	if query == "" {
		return hits
	}
	q := strings.ToLower(query)
	var exact, sub []SearchHit
	for _, h := range hits {
		id := strings.ToLower(h.SubscriptionID)
		dn := strings.ToLower(h.DisplayName)
		switch {
		case id == q || dn == q:
			exact = append(exact, h)
		case strings.Contains(id, q) || strings.Contains(dn, q):
			sub = append(sub, h)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return sub
}
