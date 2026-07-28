package headless

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/jeircul/pim/internal/app"
	"github.com/jeircul/pim/internal/azure"
)

// SearchHit is a single activatable subscription returned by pim search.
type SearchHit struct {
	SubscriptionID   string   `json:"subscriptionId"`
	DisplayName      string   `json:"displayName"`
	ManagementGroup  string   `json:"managementGroup,omitempty"`
	EligibilityScope string   `json:"eligibilityScope,omitempty"`
	EligibleRoles    []string `json:"eligibleRoles"`
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

	subToMG := map[string]string{}

	hits, subRoleMap, err := buildSearchHits(ctx, client, roles, subToMG, a.Config.MGFilter, errOut)
	if err != nil {
		return err
	}

	q := strings.TrimSpace(a.Config.SearchQuery)
	hits = filterSearchHits(hits, q)
	hits = filterHitsByMG(hits, a.Config.MGFilter)

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

	if a.Config.Output == app.OutputTOML {
		return tomlFromHits(hits, subRoleMap, out)
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
			mg = "(direct)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			h.DisplayName, h.SubscriptionID, mg, strings.Join(h.EligibleRoles, ","))
	}
	return tw.Flush()
}

// mgExpansionWorkers bounds concurrent ListAllSubscriptionsUnderMG calls
// across distinct MGs, matching the per-node concurrency bound in
// discovery.go's BFS walk.
const mgExpansionWorkers = 8

// mgExpansionResult is the outcome of expanding one distinct management
// group, produced by the concurrent phase of buildSearchHits and consumed
// by the serial merge phase.
type mgExpansionResult struct {
	mgID     string
	subs     []azure.Subscription
	parents  map[string]string
	warnings []string
	err      error
}

// buildSearchHits walks all eligible roles and flattens them into a deduplicated
// list of activatable subscriptions. MG-scoped roles are expanded via
// ListAllSubscriptionsUnderMG (cached per MG); distinct MGs are expanded
// concurrently, bounded to mgExpansionWorkers. RG-scoped roles are excluded.
// Roles for the same subscription are merged into one hit. Warnings from MG
// expansion (including errors) are written to errOut; a failing MG is skipped
// rather than aborting the entire search. subToMG provides physical parent MG
// for direct-subscription-scoped roles.
func buildSearchHits(ctx context.Context, client ClientAPI, roles []azure.Role, subToMG map[string]string, mgFilter string, errOut io.Writer) ([]SearchHit, map[string]map[string]azure.Role, error) {
	type acc struct {
		id               string
		display          string
		mg               string
		eligibilityScope string
		roles            map[string]struct{}
	}
	bySub := map[string]*acc{}
	subRoleMap := map[string]map[string]azure.Role{}

	add := func(role azure.Role, subID, display, mg, eligibilityScope string) {
		key := strings.ToLower(subID)
		a, ok := bySub[key]
		if !ok {
			if mg == "" {
				mg = subToMG[key]
			}
			a = &acc{id: subID, display: display, mg: mg, eligibilityScope: eligibilityScope, roles: map[string]struct{}{}}
			bySub[key] = a
		}
		if a.display == "" {
			a.display = display
		}
		if a.mg == "" {
			a.mg = mg
		}
		if a.eligibilityScope == "" {
			a.eligibilityScope = eligibilityScope
		}
		a.roles[role.RoleName] = struct{}{}
		if _, ok := subRoleMap[key]; !ok {
			subRoleMap[key] = map[string]azure.Role{}
		}
		if _, ok := subRoleMap[key][strings.ToLower(role.RoleName)]; !ok {
			subRoleMap[key][strings.ToLower(role.RoleName)] = role
		}
	}

	// First pass: collect distinct MG IDs that pass mgFilter and need expansion.
	var mgToExpand []string
	seenMG := map[string]struct{}{}
	for _, r := range roles {
		if r.ScopeKind() != azure.ScopeManagementGroup {
			continue
		}
		mgID := azure.ManagementGroupIDFromScope(r.Scope)
		if _, ok := seenMG[mgID]; ok {
			continue
		}
		seenMG[mgID] = struct{}{}
		if mgFilter != "" {
			f := strings.ToLower(mgFilter)
			idL := strings.ToLower(mgID)
			if !strings.Contains(idL, f) && !strings.Contains(f, idL) {
				continue
			}
		}
		mgToExpand = append(mgToExpand, mgID)
	}

	// Concurrent phase: expand each distinct MG, bounded by mgExpansionWorkers.
	results := make(map[string]mgExpansionResult, len(mgToExpand))
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		sem = make(chan struct{}, mgExpansionWorkers)
	)
	for _, mgID := range mgToExpand {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			list, parents, warnings, err := client.ListAllSubscriptionsUnderMG(ctx, mgID)
			mu.Lock()
			defer mu.Unlock()
			results[mgID] = mgExpansionResult{mgID: mgID, subs: list, parents: parents, warnings: warnings, err: err}
		}()
	}
	wg.Wait()

	// Serial merge phase: apply expansion results and role-scoped logic in
	// deterministic role order.
	mgCache := map[string][]azure.Subscription{}
	for _, r := range roles {
		switch r.ScopeKind() {
		case azure.ScopeSubscription:
			add(r, azure.SubscriptionIDFromScope(r.Scope), r.ScopeDisplay, "", r.Scope)
		case azure.ScopeManagementGroup:
			mgID := azure.ManagementGroupIDFromScope(r.Scope)
			subs, ok := mgCache[mgID]
			if !ok {
				res, expanded := results[mgID]
				if !expanded {
					// mgFilter excluded this MG from expansion.
					mgCache[mgID] = nil
					continue
				}
				for _, w := range res.warnings {
					fmt.Fprintf(errOut, "warning: %s\n", w)
				}
				if res.err != nil {
					fmt.Fprintf(errOut, "warning: list subscriptions under management group %s: %s\n", mgID, res.err)
					mgCache[mgID] = nil
					continue
				}
				for k, v := range res.parents {
					if _, exists := subToMG[k]; !exists {
						subToMG[k] = v
					}
				}
				subs = res.subs
				mgCache[mgID] = subs
			}
			for _, s := range subs {
				parent := subToMG[strings.ToLower(s.ID)]
				if parent == "" {
					parent = mgID
				}
				add(r, s.ID, s.DisplayName, parent, r.Scope)
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
			SubscriptionID:   a.id,
			DisplayName:      a.display,
			ManagementGroup:  a.mg,
			EligibilityScope: a.eligibilityScope,
			EligibleRoles:    names,
		})
	}
	return out, subRoleMap, nil
}

// favBlock is a single paste-ready favorite entry for --output toml.
type favBlock struct {
	displayName      string
	role             string
	eligibilityScope string // /subscriptions/<guid> — activation target
	mgEligibility    string // MG ARM path — for config eligibility_scope field; empty for sub-direct roles
	scheduleID       string
}

// tomlFromHits emits one [[favorites]] block per (subscription, role) pair.
// subRoleMap carries the exact azure.Role that granted each role to each
// subscription — built by buildSearchHits at expansion time. This eliminates
// any need to reconstruct the granting Role from MG IDs.
func tomlFromHits(hits []SearchHit, subRoleMap map[string]map[string]azure.Role, out io.Writer) error {
	if len(hits) == 0 {
		return nil
	}

	seen := map[string]struct{}{}
	var blocks []favBlock

	for _, h := range hits {
		subKey := strings.ToLower(h.SubscriptionID)
		subScope := "/subscriptions/" + h.SubscriptionID
		roleMap := subRoleMap[subKey]

		for _, roleName := range h.EligibleRoles {
			blockKey := strings.ToLower(roleName) + "|" + strings.ToLower(subScope)
			if _, ok := seen[blockKey]; ok {
				continue
			}
			role, ok := roleMap[strings.ToLower(roleName)]
			if !ok {
				continue
			}
			seen[blockKey] = struct{}{}

			b := favBlock{
				displayName:      h.DisplayName,
				role:             roleName,
				eligibilityScope: subScope,
				scheduleID:       role.EligibilityScheduleID,
			}
			if role.ScopeKind() == azure.ScopeManagementGroup {
				b.mgEligibility = role.Scope
			}
			blocks = append(blocks, b)
		}
	}

	sort.Slice(blocks, func(i, j int) bool {
		if blocks[i].displayName != blocks[j].displayName {
			return blocks[i].displayName < blocks[j].displayName
		}
		return blocks[i].role < blocks[j].role
	})
	return tomlOut(blocks, out)
}

// tomlOut writes paste-ready [[favorites]] TOML blocks to out.
func tomlOut(blocks []favBlock, out io.Writer) error {
	for i, b := range blocks {
		if i > 0 {
			fmt.Fprintln(out)
		}
		fmt.Fprintf(out, "[[favorites]]\n")
		fmt.Fprintf(out, "label         = %q\n", b.role+" @ "+b.displayName)
		fmt.Fprintf(out, "role          = %q\n", b.role)
		fmt.Fprintf(out, "scope         = %q\n", b.eligibilityScope)
		if b.mgEligibility != "" {
			fmt.Fprintf(out, "eligibility_scope = %q\n", b.mgEligibility)
		}
		if b.scheduleID != "" {
			fmt.Fprintf(out, "schedule_id   = %q\n", b.scheduleID)
		}
		fmt.Fprintf(out, "duration      = \"1h\"\n")
		fmt.Fprintf(out, "justification = \"\"\n")
		fmt.Fprintf(out, "key           = 0\n")
	}
	return nil
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

// filterHitsByMG applies exact-first / substring-fallback on ManagementGroup.
// Empty filter returns all hits unchanged. Hits with empty ManagementGroup
// never match a non-empty filter.
func filterHitsByMG(hits []SearchHit, filter string) []SearchHit {
	if filter == "" {
		return hits
	}
	f := strings.ToLower(filter)
	var exact, sub []SearchHit
	for _, h := range hits {
		mg := strings.ToLower(h.ManagementGroup)
		if mg == "" {
			continue
		}
		switch {
		case mg == f:
			exact = append(exact, h)
		case strings.Contains(mg, f):
			sub = append(sub, h)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return sub
}

// noopWriter discards all writes.
type noopWriter struct{}

func (noopWriter) Write(p []byte) (int, error) { return len(p), nil }
