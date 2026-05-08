package headless

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jeircul/pim/internal/app"
	"github.com/jeircul/pim/internal/azure"
)

// searchMock implements ClientAPI for search tests.
type searchMock struct {
	eligibleRoles []azure.Role
	mgSubs        map[string][]azure.Subscription
	mgSubsErr     error
	mgSubsCalls   int
	mgWarnings    map[string][]string
	mgParents     map[string]map[string]string
}

func (m *searchMock) GetCurrentUser(_ context.Context) (*azure.User, error) {
	return &azure.User{}, nil
}
func (m *searchMock) GetActiveAssignments(_ context.Context) ([]azure.ActiveAssignment, error) {
	return nil, nil
}
func (m *searchMock) GetEligibleRoles(_ context.Context) ([]azure.Role, error) {
	return m.eligibleRoles, nil
}
func (m *searchMock) ActivateRole(_ context.Context, _ azure.Role, _, _ string, _ int, _ string) (*azure.ScheduleResponse, error) {
	return nil, nil
}
func (m *searchMock) DeactivateRole(_ context.Context, _ azure.ActiveAssignment, _ string) (*azure.ScheduleResponse, error) {
	return nil, nil
}
func (m *searchMock) ListAllSubscriptionsUnderMG(_ context.Context, mgID string) ([]azure.Subscription, map[string]string, []string, error) {
	m.mgSubsCalls++
	if m.mgSubsErr != nil {
		return nil, nil, nil, m.mgSubsErr
	}
	var warnings []string
	if m.mgWarnings != nil {
		warnings = m.mgWarnings[mgID]
	}
	var parents map[string]string
	if m.mgParents != nil {
		parents = m.mgParents[mgID]
	}
	return m.mgSubs[mgID], parents, warnings, nil
}

func makeApp(query string, output app.OutputFormat) *app.App {
	cfg := app.Config{
		Command:     app.CmdSearch,
		SearchQuery: query,
		Output:      output,
	}
	return &app.App{Config: cfg}
}

func subRole(subScope, subDisplay, roleName string) azure.Role {
	return azure.Role{
		Scope:        subScope,
		ScopeDisplay: subDisplay,
		RoleName:     roleName,
	}
}

func searchMGRole(mgScope, roleName string) azure.Role {
	return azure.Role{
		Scope:    mgScope,
		RoleName: roleName,
	}
}

func rgRole(rgScope, roleName string) azure.Role {
	return azure.Role{
		Scope:    rgScope,
		RoleName: roleName,
	}
}

func TestRunSearchDirectSubRole(t *testing.T) {
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/aaaa-1111", "Sub Alpha", "Reader"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Sub Alpha") {
		t.Errorf("expected Sub Alpha in output, got: %s", out)
	}
	if !strings.Contains(out, "Reader") {
		t.Errorf("expected Reader in output, got: %s", out)
	}
	if strings.Contains(out, "no matching") {
		t.Error("unexpected no-match message")
	}
}

func TestRunSearchDirectSubRoleEmDash(t *testing.T) {
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/bbbb-2222", "Sub Beta", "Contributor"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "—") {
		t.Errorf("expected em dash for empty ManagementGroup in table output, got: %s", out)
	}
}

func TestRunSearchMGRoleExpandsToSubs(t *testing.T) {
	mgScope := "/providers/Microsoft.Management/managementGroups/mg-root"
	mock := &searchMock{
		eligibleRoles: []azure.Role{searchMGRole(mgScope, "Contributor")},
		mgSubs: map[string][]azure.Subscription{
			"mg-root": {
				{ID: "sub-1", DisplayName: "Sub One"},
				{ID: "sub-2", DisplayName: "Sub Two"},
				{ID: "sub-3", DisplayName: "Sub Three"},
			},
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, name := range []string{"Sub One", "Sub Two", "Sub Three"} {
		if !strings.Contains(out, name) {
			t.Errorf("expected %q in output, got: %s", name, out)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "Sub One") || strings.Contains(line, "Sub Two") || strings.Contains(line, "Sub Three") {
			if !strings.Contains(line, "mg-root") {
				t.Errorf("expected mg-root in line %q", line)
			}
		}
	}
}

func TestRunSearchRGRoleExcluded(t *testing.T) {
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			rgRole("/subscriptions/aaa/resourceGroups/rg1", "Reader"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "no matching") {
		t.Errorf("expected no-match message for RG-scoped role, got: %s", buf.String())
	}
}

func TestRunSearchMergesSameSubMultipleRoles(t *testing.T) {
	mgScope := "/providers/Microsoft.Management/managementGroups/mg-x"
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/sub-shared", "Shared Sub", "Owner"),
			searchMGRole(mgScope, "Reader"),
		},
		mgSubs: map[string][]azure.Subscription{
			"mg-x": {{ID: "sub-shared", DisplayName: "Shared Sub"}},
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputJSON), mock, &buf); err != nil {
		t.Fatal(err)
	}
	var hits []SearchHit
	if err := json.Unmarshal(buf.Bytes(), &hits); err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d: %+v", len(hits), hits)
	}
	roles := hits[0].EligibleRoles
	if len(roles) != 2 || roles[0] != "Owner" || roles[1] != "Reader" {
		t.Errorf("EligibleRoles = %v, want [Owner Reader]", roles)
	}
}

func TestRunSearchQueryExactGUID(t *testing.T) {
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/guid-aaa", "Sub A", "Reader"),
			subRole("/subscriptions/guid-bbb", "Sub B", "Reader"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("guid-aaa", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Sub A") {
		t.Errorf("expected Sub A, got: %s", out)
	}
	if strings.Contains(out, "Sub B") {
		t.Errorf("unexpected Sub B in output: %s", out)
	}
}

func TestRunSearchQuerySubstringDisplayName(t *testing.T) {
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/sub-1", "alpha-prod", "Reader"),
			subRole("/subscriptions/sub-2", "other-sub", "Reader"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("alpha", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "alpha-prod") {
		t.Errorf("expected alpha-prod, got: %s", out)
	}
	if strings.Contains(out, "other-sub") {
		t.Errorf("unexpected other-sub in output: %s", out)
	}
}

func TestRunSearchExactWinsOverSubstring(t *testing.T) {
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/sub-1", "Prod", "Reader"),
			subRole("/subscriptions/sub-2", "Prod-A", "Reader"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("Prod", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Prod") {
		t.Errorf("expected Prod, got: %s", out)
	}
	if strings.Contains(out, "Prod-A") {
		t.Errorf("unexpected Prod-A in output: %s", out)
	}
}

func TestRunSearchNoMatches(t *testing.T) {
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/sub-1", "Alpha", "Reader"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("nope", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "no matching eligible subscriptions") {
		t.Errorf("expected no-match message, got: %s", buf.String())
	}
}

func TestRunSearchNoMatchesJSON(t *testing.T) {
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/sub-1", "Alpha", "Reader"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("nope", app.OutputJSON), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "[]" {
		t.Errorf("expected [], got: %s", out)
	}
}

func TestRunSearchJSONShape(t *testing.T) {
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/sub-abc", "My Sub", "Owner"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputJSON), mock, &buf); err != nil {
		t.Fatal(err)
	}
	var hits []SearchHit
	if err := json.Unmarshal(buf.Bytes(), &hits); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	h := hits[0]
	if h.SubscriptionID != "sub-abc" {
		t.Errorf("SubscriptionID = %q, want sub-abc", h.SubscriptionID)
	}
	if h.DisplayName != "My Sub" {
		t.Errorf("DisplayName = %q, want My Sub", h.DisplayName)
	}
	if len(h.EligibleRoles) != 1 || h.EligibleRoles[0] != "Owner" {
		t.Errorf("EligibleRoles = %v, want [Owner]", h.EligibleRoles)
	}
}

func TestRunSearchMGCacheReused(t *testing.T) {
	mgScope := "/providers/Microsoft.Management/managementGroups/mg-cache"
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			searchMGRole(mgScope, "Reader"),
			searchMGRole(mgScope, "Contributor"),
		},
		mgSubs: map[string][]azure.Subscription{
			"mg-cache": {{ID: "sub-x", DisplayName: "Sub X"}},
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	if mock.mgSubsCalls != 1 {
		t.Errorf("ListManagementGroupSubscriptions called %d times, want 1", mock.mgSubsCalls)
	}
}

func TestRunSearchMGClientErrorWraps(t *testing.T) {
	mgScope := "/providers/Microsoft.Management/managementGroups/mg-err"
	mock := &searchMock{
		eligibleRoles: []azure.Role{searchMGRole(mgScope, "Reader")},
		mgSubsErr:     errors.New("upstream failure"),
	}
	var buf bytes.Buffer
	err := runSearch(t.Context(), makeApp("", app.OutputTable), mock, &buf)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "list subscriptions under management group") {
		t.Errorf("error = %q, want to contain 'list subscriptions under management group'", err.Error())
	}
}

func TestFilterSearchHitsEmptyQuery(t *testing.T) {
	hits := []SearchHit{
		{SubscriptionID: "a", DisplayName: "Alpha"},
		{SubscriptionID: "b", DisplayName: "Beta"},
	}
	got := filterSearchHits(hits, "")
	if len(got) != 2 {
		t.Errorf("expected 2 hits, got %d", len(got))
	}
}

func TestRunSearchJSONPreservesGUIDCase(t *testing.T) {
	const mixedCaseID = "AAAAAAAA-0000-0000-0000-000000000001"
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/"+mixedCaseID, "Case Sub", "Reader"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputJSON), mock, &buf); err != nil {
		t.Fatal(err)
	}
	var hits []SearchHit
	if err := json.Unmarshal(buf.Bytes(), &hits); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].SubscriptionID != mixedCaseID {
		t.Errorf("SubscriptionID = %q, want %q (original case must be preserved)", hits[0].SubscriptionID, mixedCaseID)
	}
}

func TestFilterRolesByMGExact(t *testing.T) {
	roles := []azure.Role{
		{Scope: "/providers/Microsoft.Management/managementGroups/example-mg", ScopeDisplay: "example-mg", RoleName: "Owner"},
		{Scope: "/providers/Microsoft.Management/managementGroups/Other", ScopeDisplay: "Other", RoleName: "Reader"},
	}
	got := filterRolesByMG(roles, []string{"example-mg"}, nil)
	if len(got) != 1 || got[0].ScopeDisplay != "example-mg" {
		t.Errorf("filterRolesByMG exact: got %+v", got)
	}
}

func TestFilterRolesByMGSubstring(t *testing.T) {
	roles := []azure.Role{
		{Scope: "/providers/Microsoft.Management/managementGroups/example-mg-prod", ScopeDisplay: "example-mg-prod", RoleName: "Owner"},
		{Scope: "/providers/Microsoft.Management/managementGroups/other", ScopeDisplay: "Other", RoleName: "Reader"},
	}
	// resolveMGFilter finds "example-mg-prod" via substring; filterRolesByMG uses exact mgID match.
	got := filterRolesByMG(roles, []string{"example-mg-prod"}, nil)
	if len(got) != 1 || got[0].ScopeDisplay != "example-mg-prod" {
		t.Errorf("filterRolesByMG substring: got %+v", got)
	}
}

func TestFilterRolesByMGNoMatch(t *testing.T) {
	roles := []azure.Role{
		{Scope: "/providers/Microsoft.Management/managementGroups/alpha", ScopeDisplay: "Alpha", RoleName: "Owner"},
	}
	got := filterRolesByMG(roles, []string{"zzz"}, nil)
	if len(got) != 0 {
		t.Errorf("filterRolesByMG no match: expected empty, got %+v", got)
	}
}

func makeAppMG(mgFilter string) *app.App {
	cfg := app.Config{
		Command:  app.CmdSearch,
		MGFilter: mgFilter,
		Output:   app.OutputTable,
	}
	return &app.App{Config: cfg}
}

func TestRunSearchMGFilter(t *testing.T) {
	omniaScope := "/providers/Microsoft.Management/managementGroups/example-mg"
	otherScope := "/providers/Microsoft.Management/managementGroups/Other"
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			searchMGRole(omniaScope, "Owner"),
			searchMGRole(otherScope, "Reader"),
		},
		mgSubs: map[string][]azure.Subscription{
			"example-mg": {{ID: "sub-example", DisplayName: "example-sub"}},
			"Other":      {{ID: "sub-other", DisplayName: "Other Sub"}},
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeAppMG("example-mg"), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "example-sub") {
		t.Errorf("expected example-sub in output, got: %s", out)
	}
	if strings.Contains(out, "Other Sub") {
		t.Errorf("unexpected Other Sub in output: %s", out)
	}
}

func TestRunSearchWarningsSentToStderr(t *testing.T) {
	mgScope := "/providers/Microsoft.Management/managementGroups/mg-warn"
	mock := &searchMock{
		eligibleRoles: []azure.Role{searchMGRole(mgScope, "Reader")},
		mgSubs: map[string][]azure.Subscription{
			"mg-warn": {{ID: "sub-w", DisplayName: "Warn Sub"}},
		},
		mgWarnings: map[string][]string{
			"mg-warn": {"skip MG child-inaccessible: 403"},
		},
	}
	var stdout, stderr bytes.Buffer
	a := &app.App{Config: app.Config{Command: app.CmdSearch, Output: app.OutputTable}}
	if err := runSearchWithErr(t.Context(), a, mock, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stdout.String(), "warning") {
		t.Errorf("warning appeared on stdout: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning") {
		t.Errorf("expected warning on stderr, got: %s", stderr.String())
	}
}

func TestRunSearchDirectSubGetsMGFromParents(t *testing.T) {
	// MG-scoped role walks mg-a and finds sub-1; direct-sub role for sub-2
	// whose physical parent mg-a is injected via mgParents.
	mgScope := "/providers/Microsoft.Management/managementGroups/mg-a"
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			searchMGRole(mgScope, "Reader"),
			subRole("/subscriptions/sub-2", "Sub Two", "Owner"),
		},
		mgSubs: map[string][]azure.Subscription{
			"mg-a": {{ID: "sub-1", DisplayName: "Sub One"}},
		},
		mgParents: map[string]map[string]string{
			"mg-a": {"sub-2": "mg-a"},
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputJSON), mock, &buf); err != nil {
		t.Fatal(err)
	}
	var hits []SearchHit
	if err := json.Unmarshal(buf.Bytes(), &hits); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}
	var sub2Hit *SearchHit
	for i := range hits {
		if hits[i].SubscriptionID == "sub-2" {
			sub2Hit = &hits[i]
		}
	}
	if sub2Hit == nil {
		t.Fatal("sub-2 not found in hits")
	}
	if sub2Hit.ManagementGroup != "mg-a" {
		t.Errorf("sub-2 ManagementGroup = %q, want mg-a", sub2Hit.ManagementGroup)
	}
}

func TestRunSearchDirectSubMGBlankWhenNoWalk(t *testing.T) {
	// Only direct-sub roles, no MG-scoped roles, no --mg filter.
	// ManagementGroup must be empty (no BFS walk to populate subToMG).
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/sub-only", "Only Sub", "Reader"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputJSON), mock, &buf); err != nil {
		t.Fatal(err)
	}
	var hits []SearchHit
	if err := json.Unmarshal(buf.Bytes(), &hits); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].ManagementGroup != "" {
		t.Errorf("ManagementGroup = %q, want empty", hits[0].ManagementGroup)
	}
}

func TestRunSearchMGFilterIncludesDirectSub(t *testing.T) {
	// --mg mg-a filter; one direct-sub role for sub-1 which is in subsUnderFilter.
	mgScope := "/providers/Microsoft.Management/managementGroups/mg-a"
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			searchMGRole(mgScope, "Reader"),
			subRole("/subscriptions/sub-1", "Sub One", "Owner"),
		},
		mgSubs: map[string][]azure.Subscription{
			"mg-a": {{ID: "sub-1", DisplayName: "Sub One"}},
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeAppMG("mg-a"), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Sub One") {
		t.Errorf("expected Sub One in output, got: %s", out)
	}
}

func TestFilterRolesByMGMultiple(t *testing.T) {
	roles := []azure.Role{
		{Scope: "/providers/Microsoft.Management/managementGroups/example-mg-a", RoleName: "Owner"},
		{Scope: "/providers/Microsoft.Management/managementGroups/example-mg-b", RoleName: "Reader"},
		{Scope: "/providers/Microsoft.Management/managementGroups/unrelated", RoleName: "Contributor"},
	}
	got := filterRolesByMG(roles, []string{"example-mg-a", "example-mg-b"}, nil)
	if len(got) != 2 {
		t.Fatalf("expected 2 roles, got %d: %+v", len(got), got)
	}
	ids := map[string]struct{}{}
	for _, r := range got {
		ids[azure.ManagementGroupIDFromScope(r.Scope)] = struct{}{}
	}
	for _, want := range []string{"example-mg-a", "example-mg-b"} {
		if _, ok := ids[want]; !ok {
			t.Errorf("expected mgID %q in result", want)
		}
	}
}

func TestRunSearchMGFilterUnion(t *testing.T) {
	scopeA := "/providers/Microsoft.Management/managementGroups/example-mg-a"
	scopeB := "/providers/Microsoft.Management/managementGroups/example-mg-b"
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			searchMGRole(scopeA, "Reader"),
			searchMGRole(scopeB, "Reader"),
		},
		mgSubs: map[string][]azure.Subscription{
			"example-mg-a": {{ID: "sub-a", DisplayName: "Sub A"}},
			"example-mg-b": {{ID: "sub-b", DisplayName: "Sub B"}},
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeAppMG("example-mg"), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Sub A") {
		t.Errorf("expected Sub A in output, got: %s", out)
	}
	if !strings.Contains(out, "Sub B") {
		t.Errorf("expected Sub B in output, got: %s", out)
	}
}
