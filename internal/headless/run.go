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
	GetCurrentUser() (*azure.User, error)
	GetActiveAssignments() ([]azure.ActiveAssignment, error)
	GetEligibleRoles() ([]azure.Role, error)
	ActivateRole(role azure.Role, principalID, justification string, minutes int, targetScope string) (*azure.ScheduleResponse, error)
	DeactivateRole(assignment azure.ActiveAssignment, principalID string) (*azure.ScheduleResponse, error)
}

var _ ClientAPI = (*azure.Client)(nil)

// Run executes the requested command without a TUI and returns an exit error if any.
func Run(ctx context.Context, a *app.App) error {
	client := a.Client

	user, err := client.GetCurrentUser()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	switch a.Config.Command {
	case app.CmdStatus:
		return runStatus(a, client, user, os.Stdout)
	case app.CmdDeactivate:
		return runDeactivate(a, client, user, os.Stdout)
	case app.CmdActivate:
		return runActivate(a, client, user, os.Stdout)
	default:
		return runStatus(a, client, user, os.Stdout)
	}
}

func runStatus(a *app.App, client ClientAPI, user *azure.User, out io.Writer) error {
	assignments, err := client.GetActiveAssignments()
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

func runDeactivate(a *app.App, client ClientAPI, user *azure.User, out io.Writer) error {
	if len(a.Config.Roles) == 0 && len(a.Config.Scopes) == 0 && !a.Config.Yes {
		return fmt.Errorf("--headless deactivate requires --role or --scope; use --yes to deactivate all")
	}

	assignments, err := client.GetActiveAssignments()
	if err != nil {
		return fmt.Errorf("get active assignments: %w", err)
	}

	targets := filterAssignments(assignments, a.Config.Roles, a.Config.Scopes)
	if len(targets) == 0 {
		fmt.Fprintln(out, "No matching active assignments.")
		return nil
	}

	var lastErr error
	for _, assignment := range targets {
		if _, err := client.DeactivateRole(assignment, user.ID); err != nil {
			fmt.Fprintf(os.Stderr, "deactivate %s@%s: %v\n", assignment.RoleName, assignment.ScopeDisplay, err)
			lastErr = err
			continue
		}
		fmt.Fprintf(out, "Deactivated: %s @ %s\n", assignment.RoleName, assignment.ScopeDisplay)
	}
	return lastErr
}

func runActivate(a *app.App, client ClientAPI, user *azure.User, out io.Writer) error {
	cfg := a.Config
	if !cfg.HasRoleFilter() || !cfg.HasScopeFilter() || cfg.TimeStr == "" || cfg.Justification == "" {
		return fmt.Errorf("--headless activate requires --role, --scope, --time, and --justification")
	}

	minutes, err := azure.ParseDurationMinutes(cfg.TimeStr)
	if err != nil {
		return err
	}

	roles, err := client.GetEligibleRoles()
	if err != nil {
		return fmt.Errorf("get eligible roles: %w", err)
	}

	targets := filterRoles(roles, cfg.Roles, cfg.Scopes)
	if len(targets) == 0 {
		return fmt.Errorf("no eligible roles match the specified --role / --scope filters")
	}

	var lastErr error
	for _, match := range targets {
		_, err := client.ActivateRole(match.role, user.ID, cfg.Justification, minutes, match.scope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "activate %s@%s: %v\n", match.role.RoleName, match.scope, err)
			lastErr = err
			continue
		}
		fmt.Fprintf(out, "Activated: %s @ %s for %s\n", match.role.RoleName, match.scope, cfg.TimeStr)
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

func filterRoles(roles []azure.Role, roleFilters, scopeFilters []string) []roleTarget {
	var out []roleTarget
	for _, r := range roles {
		if !matchesAny(r.RoleName, roleFilters) {
			continue
		}
		for _, sf := range scopeFilters {
			out = append(out, roleTarget{role: r, scope: sf})
		}
	}
	return out
}

func filterAssignments(assignments []azure.ActiveAssignment, roleFilters, scopeFilters []string) []azure.ActiveAssignment {
	if len(roleFilters) == 0 && len(scopeFilters) == 0 {
		return assignments
	}
	out := assignments[:0:0]
	for _, a := range assignments {
		if len(roleFilters) > 0 && !matchesAny(a.RoleName, roleFilters) {
			continue
		}
		if len(scopeFilters) > 0 && !matchesAny(a.Scope, scopeFilters) {
			continue
		}
		out = append(out, a)
	}
	return out
}

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

func jsonOut(v any, out io.Writer) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
