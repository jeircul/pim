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

	hits, err := buildSearchHits(ctx, client, roles, subToMG, a.Config.MGFilter, errOut)
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
		return tomlFromHits(hits, roles, out)
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

// buildSearchHits walks all eligible roles and flattens them into a deduplicated
// list of activatable subscriptions. MG-scoped roles are expanded via
// ListAllSubscriptionsUnderMG (cached per MG). RG-scoped roles are excluded.
// Roles for the same subscription are merged into one hit. Warnings from MG
// expansion (including errors) are written to errOut; a failing MG is skipped
// rather than aborting the entire search. subToMG provides physical parent MG
// for direct-subscription-scoped roles.
func buildSearchHits(ctx context.Context, client ClientAPI, roles []azure.Role, subToMG map[string]string, mgFilter string, errOut io.Writer) ([]SearchHit, error) {
	type acc struct {
		id               string
		display          string
		mg               string
		eligibilityScope string
		roles            map[string]struct{}
	}
	bySub := map[string]*acc{}
	mgCache := map[string][]azure.Subscription{}

	add := func(subID, display, mg, eligibilityScope, roleName string) {
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
		a.roles[roleName] = struct{}{}
	}

	for _, r := range roles {
		switch r.ScopeKind() {
		case azure.ScopeSubscription:
			add(azure.SubscriptionIDFromScope(r.Scope), r.ScopeDisplay, "", r.Scope, r.RoleName)
		case azure.ScopeManagementGroup:
			mgID := azure.ManagementGroupIDFromScope(r.Scope)
			if mgFilter != "" {
				f := strings.ToLower(mgFilter)
				idL := strings.ToLower(mgID)
				if !strings.Contains(idL, f) && !strings.Contains(f, idL) {
					mgCache[mgID] = nil
					continue
				}
			}
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
				parent := subToMG[strings.ToLower(s.ID)]
				if parent == "" {
					parent = mgID
				}
				add(s.ID, s.DisplayName, parent, r.Scope, r.RoleName)
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
	return out, nil
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
// hits carries the query-filtered subscription set with their eligible role names;
// roles is the full raw eligible list used to recover per-role eligibility scope
// and schedule ID. For each matched subscription, every role in h.EligibleRoles
// is emitted — buildSearchHits already verified that role applies to that subscription.
func tomlFromHits(hits []SearchHit, roles []azure.Role, out io.Writer) error {
	if len(hits) == 0 {
		return nil
	}

	// subDisplay: subID → display name.
	subDisplay := make(map[string]string, len(hits))
	for _, h := range hits {
		subDisplay[strings.ToLower(h.SubscriptionID)] = h.DisplayName
	}

	// subRoles: subID → set of eligible role names (from buildSearchHits expansion).
	type subEntry struct {
		display   string
		subID     string
		mgID      string // direct parent MG ID (may be empty for direct subs)
		eligScope string // h.EligibilityScope
		eligRoles []string
	}
	var entries []subEntry
	for _, h := range hits {
		mgID := strings.ToLower(h.ManagementGroup)
		if mgID == "" && azure.IsManagementGroupScope(h.EligibilityScope) {
			mgID = strings.ToLower(azure.ManagementGroupIDFromScope(h.EligibilityScope))
		}
		entries = append(entries, subEntry{
			display:   h.DisplayName,
			subID:     h.SubscriptionID,
			mgID:      mgID,
			eligScope: h.EligibilityScope,
			eligRoles: h.EligibleRoles,
		})
	}

	// rolesByName: roleName → []Role (all MG-scoped roles with that name, sorted).
	rolesByName := make(map[string][]azure.Role)
	for _, r := range roles {
		rn := strings.ToLower(r.RoleName)
		rolesByName[rn] = append(rolesByName[rn], r)
	}

	seen := map[string]struct{}{}
	var blocks []favBlock

	for _, e := range entries {
		subScope := "/subscriptions/" + e.subID

		for _, roleName := range e.eligRoles {
			blockKey := strings.ToLower(roleName) + "|" + strings.ToLower(subScope)
			if _, ok := seen[blockKey]; ok {
				continue
			}
			seen[blockKey] = struct{}{}

			// Find the best Role object for this (subscription, roleName) pair.
			// Priority: sub-direct > same-MG > any MG-scoped.
			var best *azure.Role
			for i := range rolesByName[strings.ToLower(roleName)] {
				r := &rolesByName[strings.ToLower(roleName)][i]
				switch r.ScopeKind() {
				case azure.ScopeSubscription:
					if strings.EqualFold(azure.SubscriptionIDFromScope(r.Scope), e.subID) {
						best = r
					}
				case azure.ScopeManagementGroup:
					mgID := strings.ToLower(azure.ManagementGroupIDFromScope(r.Scope))
					if best == nil {
						best = r // any MG match as fallback
					}
					if mgID == e.mgID {
						best = r // prefer the MG that directly parents this subscription
						break
					}
				}
				if best != nil && best.ScopeKind() == azure.ScopeSubscription {
					break // sub-direct is always best
				}
			}
			if best == nil {
				continue
			}

			b := favBlock{
				displayName:      e.display,
				role:             roleName,
				eligibilityScope: subScope,
				scheduleID:       best.EligibilityScheduleID,
			}
			if best.ScopeKind() == azure.ScopeManagementGroup {
				b.mgEligibility = best.Scope
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
