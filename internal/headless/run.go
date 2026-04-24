package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/jeircul/pim/internal/app"
	"github.com/jeircul/pim/internal/azure"
)

// ClientAPI is the subset of azure.Client methods used by headless execution.
type ClientAPI interface {
	GetCurrentUser(ctx context.Context) (*azure.User, error)
	GetActiveAssignments(ctx context.Context) ([]azure.ActiveAssignment, error)
	GetEligibleRoles(ctx context.Context) ([]azure.Role, error)
	ActivateRole(ctx context.Context, role azure.Role, principalID, justification string, minutes int, targetScope string) (*azure.ScheduleResponse, error)
	DeactivateRole(ctx context.Context, assignment azure.ActiveAssignment, principalID string) (*azure.ScheduleResponse, error)
}

var _ ClientAPI = (*azure.Client)(nil)

// Run executes the requested command without a TUI and returns an exit error if any.
func Run(ctx context.Context, a *app.App) error {
	client := a.Client

	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	switch a.Config.Command {
	case app.CmdStatus:
		return runStatus(ctx, a, client, user, os.Stdout)
	case app.CmdDeactivate:
		return runDeactivate(ctx, a, client, user, os.Stdout)
	case app.CmdActivate:
		return runActivate(ctx, a, client, user, os.Stdout)
	default:
		return runStatus(ctx, a, client, user, os.Stdout)
	}
}

func runStatus(ctx context.Context, a *app.App, client ClientAPI, user *azure.User, out io.Writer) error {
	assignments, err := client.GetActiveAssignments(ctx)
	if err != nil {
		return fmt.Errorf("get active assignments: %w", err)
	}

	if a.Config.Output == app.OutputJSON {
		return jsonOut(assignments, out)
	}

	if len(assignments) == 0 {
		fmt.Fprintln(out, "No active PIM elevations.")
		return nil
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ROLE\tSCOPE\tEXPIRES")
	for _, a := range assignments {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", a.RoleName, a.ScopeDisplay, a.ExpiryDisplay())
	}
	return tw.Flush()
}

func runDeactivate(ctx context.Context, a *app.App, client ClientAPI, user *azure.User, out io.Writer) error {
	if len(a.Config.Roles) == 0 && len(a.Config.Scopes) == 0 && !a.Config.Yes {
		return fmt.Errorf("--headless deactivate requires --role or --scope; use --yes to deactivate all")
	}

	assignments, err := client.GetActiveAssignments(ctx)
	if err != nil {
		return fmt.Errorf("get active assignments: %w", err)
	}

	all, inherited, permanent := partitionDeactivatable(assignments)
	for _, inh := range inherited {
		fmt.Fprintf(os.Stderr, "skipping inherited assignment: %s @ %s (cannot self-deactivate group-inherited roles)\n",
			inh.RoleName, inh.ScopeDisplay)
	}
	for _, p := range permanent {
		fmt.Fprintf(os.Stderr, "skipping permanent assignment: %s @ %s (no expiry; not PIM-activated)\n",
			p.RoleName, p.ScopeDisplay)
	}

	targets, err := filterAssignments(all, a.Config.Roles, a.Config.Scopes)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		if len(a.Config.Roles) > 0 || len(a.Config.Scopes) > 0 {
			return fmt.Errorf("no active assignments match --role / --scope filters")
		}
		fmt.Fprintln(out, "No matching active assignments.")
		return nil
	}

	var lastErr error
	for _, assignment := range targets {
		if _, err := client.DeactivateRole(ctx, assignment, user.ID); err != nil {
			fmt.Fprintf(os.Stderr, "deactivate %s@%s: %v\n", assignment.RoleName, assignment.ScopeDisplay, err)
			lastErr = err
			continue
		}
		fmt.Fprintf(out, "Deactivated: %s @ %s\n", assignment.RoleName, assignment.ScopeDisplay)
	}
	return lastErr
}

func runActivate(ctx context.Context, a *app.App, client ClientAPI, user *azure.User, out io.Writer) error {
	cfg := a.Config
	if !cfg.HasRoleFilter() {
		return fmt.Errorf("--headless activate requires --role")
	}

	timeStr := cfg.TimeStr
	if timeStr == "" {
		timeStr = a.Store.DefaultDuration()
	}
	if timeStr == "" {
		timeStr = "1h"
	}
	minutes, err := azure.ParseDurationMinutes(timeStr)
	if err != nil {
		return err
	}

	roles, err := client.GetEligibleRoles(ctx)
	if err != nil {
		return fmt.Errorf("get eligible roles: %w", err)
	}

	targets, err := filterRoles(roles, cfg.Roles, cfg.Scopes)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		return fmt.Errorf("no eligible roles match the specified --role / --scope filters")
	}

	var lastErr error
	for _, match := range targets {
		scope := azure.NormalizeScope(match.scope)
		_, err := client.ActivateRole(ctx, match.role, user.ID, cfg.Justification, minutes, scope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "activate %s@%s: %v\n", match.role.RoleName, scope, err)
			lastErr = err
			continue
		}
		fmt.Fprintf(out, "Activated: %s @ %s for %s\n", match.role.RoleName, scope, timeStr)
	}

	if lastErr != nil {
		a.Store.AddRecentJustification(cfg.Justification)
		_ = a.Store.SaveState()
		return lastErr
	}

	a.Store.AddRecentJustification(cfg.Justification)
	return a.Store.SaveState()
}

type roleTarget struct {
	role  azure.Role
	scope string
}

func filterRoles(roles []azure.Role, roleFilters, scopeFilters []string) ([]roleTarget, error) {
	roleNames := make([]string, len(roles))
	for i, r := range roles {
		roleNames[i] = r.RoleName
	}
	roleIdx, err := selectByFilter(roleNames, roleFilters, "--role")
	if err != nil {
		return nil, err
	}

	if len(scopeFilters) == 0 {
		out := make([]roleTarget, 0, len(roleIdx))
		for _, i := range roleIdx {
			out = append(out, roleTarget{role: roles[i], scope: roles[i].Scope})
		}
		return out, nil
	}

	scopeDisplays := make([]string, len(roles))
	for i, r := range roles {
		scopeDisplays[i] = r.ScopeDisplay
	}

	var out []roleTarget
	for _, sf := range scopeFilters {
		armMatches := map[int]struct{}{}
		for _, i := range roleIdx {
			if azure.ScopeIsChildOf(sf, roles[i].Scope) {
				armMatches[i] = struct{}{}
			}
		}
		if len(armMatches) > 0 {
			for i := range armMatches {
				out = append(out, roleTarget{role: roles[i], scope: sf})
			}
			continue
		}

		candidateDisplays := make([]string, len(roleIdx))
		for j, i := range roleIdx {
			candidateDisplays[j] = scopeDisplays[i]
		}
		dispIdx, err := selectByFilter(candidateDisplays, []string{sf}, "--scope")
		if err != nil {
			return nil, err
		}
		for _, j := range dispIdx {
			i := roleIdx[j]
			out = append(out, roleTarget{role: roles[i], scope: roles[i].Scope})
		}
	}
	return out, nil
}

// scopeIsChildOf reports whether child is equal to or a descendant of parent.
// Both are ARM scope paths (case-insensitive prefix match on path segments).
func scopeIsChildOf(child, parent string) bool {
	return azure.ScopeIsChildOf(child, parent)
}

func filterAssignments(assignments []azure.ActiveAssignment, roleFilters, scopeFilters []string) ([]azure.ActiveAssignment, error) {
	if len(roleFilters) == 0 && len(scopeFilters) == 0 {
		return assignments, nil
	}

	allowRole := make([]bool, len(assignments))
	if len(roleFilters) == 0 {
		for i := range allowRole {
			allowRole[i] = true
		}
	} else {
		roleNames := make([]string, len(assignments))
		for i, a := range assignments {
			roleNames[i] = a.RoleName
		}
		idx, err := selectByFilter(roleNames, roleFilters, "--role")
		if err != nil {
			return nil, err
		}
		for _, j := range idx {
			allowRole[j] = true
		}
	}

	var out []azure.ActiveAssignment
	for i, a := range assignments {
		if !allowRole[i] {
			continue
		}
		if len(scopeFilters) > 0 {
			scopeMatch := false
			for _, sf := range scopeFilters {
				if azure.ScopeIsChildOf(a.Scope, sf) || azure.ScopeIsChildOf(sf, a.Scope) {
					scopeMatch = true
					break
				}
				displays := []string{a.ScopeDisplay}
				idx, err := selectByFilter(displays, []string{sf}, "--scope")
				if err != nil {
					return nil, err
				}
				if len(idx) > 0 {
					scopeMatch = true
					break
				}
			}
			if !scopeMatch {
				continue
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// selectByFilter returns indices of candidates that match any filter using the
// exact-first, substring-fallback policy. If any candidate exactly matches a
// filter, only exact matches are returned for that filter. If no exact match
// exists and multiple candidates match via substring, an ambiguity error is returned.
func selectByFilter(candidates []string, filters []string, flag string) ([]int, error) {
	if len(filters) == 0 {
		idx := make([]int, len(candidates))
		for i := range candidates {
			idx[i] = i
		}
		return idx, nil
	}

	selected := map[int]struct{}{}
	for _, f := range filters {
		fl := strings.ToLower(f)
		var exactIdx []int
		var subIdx []int
		var subNames []string
		for i, c := range candidates {
			cl := strings.ToLower(c)
			if cl == fl {
				exactIdx = append(exactIdx, i)
			} else if strings.Contains(cl, fl) {
				subIdx = append(subIdx, i)
				subNames = append(subNames, c)
			}
		}
		if len(exactIdx) > 0 {
			for _, i := range exactIdx {
				selected[i] = struct{}{}
			}
			continue
		}
		if len(subIdx) == 0 {
			continue
		}
		if len(subIdx) > 1 {
			return nil, fmt.Errorf("%s '%s' is ambiguous: matched '%s' — use exact name",
				flag, f, strings.Join(subNames, "', '"))
		}
		selected[subIdx[0]] = struct{}{}
	}

	out := make([]int, 0, len(selected))
	for i := range candidates {
		if _, ok := selected[i]; ok {
			out = append(out, i)
		}
	}
	return out, nil
}

// matchesAny reports whether s contains any filter as a substring (case-insensitive).
func matchesAny(s string, filters []string) bool {
	if len(filters) == 0 {
		return true
	}
	sl := strings.ToLower(s)
	for _, f := range filters {
		if strings.Contains(sl, strings.ToLower(f)) {
			return true
		}
	}
	return false
}

// matchBest returns true if s matches any filter using exact-first, substring-fallback policy.
// Returns an ambiguity error if multiple substring matches exist and no exact match was found.
// This is a per-candidate check; for global policy use selectByFilter.
func matchBest(s string, filters []string) (bool, error) {
	if len(filters) == 0 {
		return true, nil
	}
	sl := strings.ToLower(s)
	for _, f := range filters {
		if sl == strings.ToLower(f) {
			return true, nil
		}
	}
	var subHits []string
	for _, f := range filters {
		if strings.Contains(sl, strings.ToLower(f)) {
			subHits = append(subHits, f)
		}
	}
	if len(subHits) == 0 {
		return false, nil
	}
	if len(subHits) > 1 {
		return false, fmt.Errorf("'%s' is ambiguous: matched '%s' — use exact name",
			subHits[0], strings.Join(subHits, "', '"))
	}
	return true, nil
}

// partitionDeactivatable splits assignments into ones safe to deactivate
// versus ones that should be skipped: inherited (group membership, cannot
// self-deactivate) and permanent (no expiry, not PIM-activated).
func partitionDeactivatable(assignments []azure.ActiveAssignment) (deactivatable, inherited, permanent []azure.ActiveAssignment) {
	for _, a := range assignments {
		switch {
		case strings.EqualFold(a.MemberType, "Inherited"):
			inherited = append(inherited, a)
		case a.IsPermanent():
			permanent = append(permanent, a)
		default:
			deactivatable = append(deactivatable, a)
		}
	}
	return deactivatable, inherited, permanent
}

func jsonOut(v any, out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
