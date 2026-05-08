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
	return runSearchWithErr(ctx, a, client, out, noopWriter{})
}

// runSearchWithErr is the full implementation; errOut receives warnings.
func runSearchWithErr(ctx context.Context, a *app.App, client ClientAPI, out io.Writer, errOut io.Writer) error {
	roles, err := client.GetEligibleRoles(ctx)
	if err != nil {
		return fmt.Errorf("get eligible roles: %w", err)
	}

	// subToMG maps lowercased subscription ID to the MG that directly contains it.
	subToMG := map[string]string{}
	// subsUnderFilter is the set of sub IDs under the --mg filter (when set).
	var subsUnderFilter map[string]struct{}

	if a.Config.MGFilter != "" {
		mgIDs, ok := resolveMGFilter(roles, a.Config.MGFilter)
		if !ok {
			fmt.Fprintf(out, "no eligible roles under management group %q\n", a.Config.MGFilter)
			return nil
		}
		subsUnderFilter = make(map[string]struct{})
		for _, mgID := range mgIDs {
			subs, parents, warnings, err := client.ListAllSubscriptionsUnderMG(ctx, mgID)
			if err != nil {
				return fmt.Errorf("list subscriptions under management group %s: %w", mgID, err)
			}
			for _, w := range warnings {
				fmt.Fprintf(errOut, "warning: %s\n", w)
			}
			for _, s := range subs {
				subsUnderFilter[strings.ToLower(s.ID)] = struct{}{}
			}
			for k, v := range parents {
				subToMG[k] = v
			}
		}
		roles = filterRolesByMG(roles, mgIDs, subsUnderFilter)
		if len(roles) == 0 {
			fmt.Fprintf(out, "no eligible roles under management group %q\n", a.Config.MGFilter)
			return nil
		}
	}

	hits, err := buildSearchHits(ctx, client, roles, subToMG, errOut)
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
		mg := h.ManagementGroup
		if mg == "" {
			mg = "—"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			h.DisplayName, h.SubscriptionID, mg, strings.Join(h.EligibleRoles, ","))
	}
	return tw.Flush()
}

// resolveMGFilter returns the management group ARM IDs that best match filter
// using exact-first, substring-fallback on ARM ID and display name.
// Exact match returns a single-element slice. Substring match returns all matches (deduped).
// Returns (nil, false) when no MG-scoped role matches.
func resolveMGFilter(roles []azure.Role, filter string) (mgIDs []string, ok bool) {
	f := strings.ToLower(filter)
	var exactID string
	seen := map[string]struct{}{}
	var subIDs []string
	for _, r := range roles {
		if r.ScopeKind() != azure.ScopeManagementGroup {
			continue
		}
		id := azure.ManagementGroupIDFromScope(r.Scope)
		idL := strings.ToLower(id)
		dnL := strings.ToLower(r.ScopeDisplay)
		if idL == f || dnL == f {
			exactID = id
			continue
		}
		if exactID == "" && (strings.Contains(idL, f) || strings.Contains(dnL, f)) {
			if _, dup := seen[idL]; !dup {
				seen[idL] = struct{}{}
				subIDs = append(subIDs, id)
			}
		}
	}
	if exactID != "" {
		return []string{exactID}, true
	}
	if len(subIDs) > 0 {
		return subIDs, true
	}
	return nil, false
}

// buildSearchHits walks all eligible roles and flattens them into a deduplicated
// list of activatable subscriptions. MG-scoped roles are expanded via
// ListAllSubscriptionsUnderMG (cached per MG). RG-scoped roles are excluded.
// Roles for the same subscription are merged into one hit. Warnings from MG
// expansion (including errors) are written to errOut; a failing MG is skipped
// rather than aborting the entire search. subToMG provides physical parent MG
// for direct-subscription-scoped roles.
func buildSearchHits(ctx context.Context, client ClientAPI, roles []azure.Role, subToMG map[string]string, errOut io.Writer) ([]SearchHit, error) {
	type acc struct {
		id      string
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
			if mg == "" {
				mg = subToMG[key]
			}
			a = &acc{id: subID, display: display, mg: mg, roles: map[string]struct{}{}}
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
				list, parents, warnings, err := client.ListAllSubscriptionsUnderMG(ctx, mgID)
				for _, w := range warnings {
					fmt.Fprintf(errOut, "warning: %s\n", w)
				}
				if err != nil {
					fmt.Fprintf(errOut, "warning: list subscriptions under management group %s: %s\n", mgID, err)
					mgCache[mgID] = nil
					continue
				}
				for k, v := range parents {
					if _, exists := subToMG[k]; !exists {
						subToMG[k] = v
					}
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
	for _, a := range bySub {
		names := make([]string, 0, len(a.roles))
		for n := range a.roles {
			names = append(names, n)
		}
		sort.Strings(names)
		out = append(out, SearchHit{
			SubscriptionID:  a.id,
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

// filterRolesByMG keeps roles whose eligibility scope matches any of mgIDs,
// or whose subscription physically lives under any of those MGs (via subsUnderFilter).
func filterRolesByMG(roles []azure.Role, mgIDs []string, subsUnderFilter map[string]struct{}) []azure.Role {
	mgSet := make(map[string]struct{}, len(mgIDs))
	for _, id := range mgIDs {
		mgSet[strings.ToLower(id)] = struct{}{}
	}
	var out []azure.Role
	for _, r := range roles {
		switch r.ScopeKind() {
		case azure.ScopeManagementGroup:
			if _, ok := mgSet[strings.ToLower(azure.ManagementGroupIDFromScope(r.Scope))]; ok {
				out = append(out, r)
			}
		case azure.ScopeSubscription:
			key := strings.ToLower(azure.SubscriptionIDFromScope(r.Scope))
			if _, ok := subsUnderFilter[key]; ok {
				out = append(out, r)
			}
		}
	}
	return out
}

// noopWriter discards all writes.
type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }
