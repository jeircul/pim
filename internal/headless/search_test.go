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

// perMGErrorMock wraps searchMock and returns per-MG errors from ListAllSubscriptionsUnderMG.
type perMGErrorMock struct {
	base   *searchMock
	errMGs map[string]error
}

func (m *perMGErrorMock) GetCurrentUser(ctx context.Context) (*azure.User, error) {
	return m.base.GetCurrentUser(ctx)
}
func (m *perMGErrorMock) GetActiveAssignments(ctx context.Context) ([]azure.ActiveAssignment, error) {
	return m.base.GetActiveAssignments(ctx)
}
func (m *perMGErrorMock) GetEligibleRoles(ctx context.Context) ([]azure.Role, error) {
	return m.base.GetEligibleRoles(ctx)
}
func (m *perMGErrorMock) ActivateRole(ctx context.Context, r azure.Role, a, b string, d int, j string) (*azure.ScheduleResponse, error) {
	return m.base.ActivateRole(ctx, r, a, b, d, j)
}
func (m *perMGErrorMock) DeactivateRole(ctx context.Context, a azure.ActiveAssignment, s string) (*azure.ScheduleResponse, error) {
	return m.base.DeactivateRole(ctx, a, s)
}
func (m *perMGErrorMock) ListAllSubscriptionsUnderMG(ctx context.Context, mgID string) ([]azure.Subscription, map[string]string, []string, error) {
	if err, ok := m.errMGs[mgID]; ok {
		return nil, nil, nil, err
	}
	return m.base.ListAllSubscriptionsUnderMG(ctx, mgID)
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

func TestRunSearchDirectSubRoleDirect(t *testing.T) {
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
	if !strings.Contains(out, "(direct)") {
		t.Errorf("expected (direct) for empty ManagementGroup in table output, got: %s", out)
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
	var stdout, stderr bytes.Buffer
	a := &app.App{Config: app.Config{Command: app.CmdSearch, Output: app.OutputTable}}
	if err := runSearchWithErr(t.Context(), a, mock, &stdout, &stderr); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "list subscriptions under management group") {
		t.Errorf("expected warning on stderr, got: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "upstream failure") {
		t.Errorf("expected upstream failure in stderr, got: %s", stderr.String())
	}
}

func TestBuildSearchHitsMGExpansionError(t *testing.T) {
	errScope := "/providers/Microsoft.Management/managementGroups/example-mg-err"
	okScope := "/providers/Microsoft.Management/managementGroups/example-mg-ok"

	callCount := 0
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			searchMGRole(errScope, "Reader"),
			searchMGRole(okScope, "Contributor"),
		},
		mgSubs: map[string][]azure.Subscription{
			"example-mg-ok": {{ID: "sub-ok", DisplayName: "Sub OK"}},
		},
	}
	origListFn := mock.ListAllSubscriptionsUnderMG
	_ = origListFn
	_ = callCount

	errMock := &perMGErrorMock{
		base:   mock,
		errMGs: map[string]error{"example-mg-err": errors.New("timeout expanding MG")},
	}

	var stdout, stderr bytes.Buffer
	a := &app.App{Config: app.Config{Command: app.CmdSearch, Output: app.OutputTable}}
	if err := runSearchWithErr(t.Context(), a, errMock, &stdout, &stderr); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(stderr.String(), "example-mg-err") {
		t.Errorf("expected example-mg-err in stderr warning, got: %s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "timeout expanding MG") {
		t.Errorf("expected error text in stderr, got: %s", stderr.String())
	}
	if !strings.Contains(stdout.String(), "sub-ok") && !strings.Contains(stdout.String(), "Sub OK") {
		t.Errorf("expected sub-ok results in stdout, got: %s", stdout.String())
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

func makeAppMG(mgFilter string) *app.App {
	cfg := app.Config{
		Command:  app.CmdSearch,
		MGFilter: mgFilter,
		Output:   app.OutputTable,
	}
	return &app.App{Config: cfg}
}

func TestRunSearchMGFilter(t *testing.T) {
	mgScope := "/providers/Microsoft.Management/managementGroups/example-mg"
	otherScope := "/providers/Microsoft.Management/managementGroups/Other"
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			searchMGRole(mgScope, "Owner"),
			searchMGRole(otherScope, "Reader"),
		},
		mgSubs: map[string][]azure.Subscription{
			"example-mg": {{ID: "sub-example", DisplayName: "example-sub"}},
			"Other":      {{ID: "sub-other", DisplayName: "Other Sub"}},
		},
		mgParents: map[string]map[string]string{
			"example-mg": {"sub-example": "example-mg"},
			"Other":      {"sub-other": "Other"},
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
	mgScope := "/providers/Microsoft.Management/managementGroups/mg-a"
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			searchMGRole(mgScope, "Reader"),
			subRole("/subscriptions/sub-1", "Sub One", "Owner"),
		},
		mgSubs: map[string][]azure.Subscription{
			"mg-a": {{ID: "sub-1", DisplayName: "Sub One"}},
		},
		mgParents: map[string]map[string]string{
			"mg-a": {"sub-1": "mg-a"},
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
		mgParents: map[string]map[string]string{
			"example-mg-a": {"sub-a": "example-mg-a"},
			"example-mg-b": {"sub-b": "example-mg-b"},
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

func TestBuildSearchHitsMGColumnUsesDirectParent(t *testing.T) {
	mgScope := "/providers/Microsoft.Management/managementGroups/parent-mg"
	mock := &searchMock{
		eligibleRoles: []azure.Role{searchMGRole(mgScope, "Reader")},
		mgSubs: map[string][]azure.Subscription{
			"parent-mg": {{ID: "sub-1", DisplayName: "Sub One"}},
		},
		mgParents: map[string]map[string]string{
			"parent-mg": {"sub-1": "child-mg-a"},
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
	if hits[0].ManagementGroup != "child-mg-a" {
		t.Errorf("ManagementGroup = %q, want child-mg-a", hits[0].ManagementGroup)
	}
}

func TestFilterHitsByMGEmptyFilter(t *testing.T) {
	hits := []SearchHit{
		{SubscriptionID: "sub-1", ManagementGroup: "prod"},
		{SubscriptionID: "sub-2", ManagementGroup: ""},
	}
	got := filterHitsByMG(hits, "")
	if len(got) != 2 {
		t.Errorf("expected 2 hits, got %d", len(got))
	}
}

func TestFilterHitsByMGExact(t *testing.T) {
	hits := []SearchHit{
		{SubscriptionID: "sub-1", ManagementGroup: "prod"},
		{SubscriptionID: "sub-2", ManagementGroup: "prod-extra"},
	}
	got := filterHitsByMG(hits, "prod")
	if len(got) != 1 || got[0].ManagementGroup != "prod" {
		t.Errorf("expected exactly {MG:prod}, got %+v", got)
	}
}

func TestFilterHitsByMGSubstring(t *testing.T) {
	hits := []SearchHit{
		{SubscriptionID: "sub-1", ManagementGroup: "child-mg-a"},
		{SubscriptionID: "sub-2", ManagementGroup: "other"},
	}
	got := filterHitsByMG(hits, "child-mg")
	if len(got) != 1 || got[0].ManagementGroup != "child-mg-a" {
		t.Errorf("expected exactly {MG:child-mg-a}, got %+v", got)
	}
}

func TestFilterHitsByMGNoMatch(t *testing.T) {
	hits := []SearchHit{
		{SubscriptionID: "sub-1", ManagementGroup: "prod"},
	}
	got := filterHitsByMG(hits, "zzz")
	if len(got) != 0 {
		t.Errorf("expected empty, got %+v", got)
	}
}

func TestFilterHitsByMGEmptyMGSkipped(t *testing.T) {
	hits := []SearchHit{
		{SubscriptionID: "sub-1", ManagementGroup: ""},
	}
	got := filterHitsByMG(hits, "prod")
	if len(got) != 0 {
		t.Errorf("expected empty (empty MG skipped), got %+v", got)
	}
}

func TestRunSearchMGFilterNoMatch(t *testing.T) {
	mgScope := "/providers/Microsoft.Management/managementGroups/example-mg"
	mock := &searchMock{
		eligibleRoles: []azure.Role{searchMGRole(mgScope, "Reader")},
		mgSubs: map[string][]azure.Subscription{
			"example-mg": {{ID: "sub-1", DisplayName: "Sub One"}},
		},
		mgParents: map[string]map[string]string{
			"example-mg": {"sub-1": "example-mg"},
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeAppMG("nonexistent-mg"), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "no matching eligible subscriptions") {
		t.Errorf("expected no-match message, got: %s", out)
	}
	if strings.Contains(out, "no eligible roles under management group") {
		t.Errorf("unexpected old message in output: %s", out)
	}
}

// perMGCallMock records which MG IDs were passed to ListAllSubscriptionsUnderMG.
type perMGCallMock struct {
	subs  map[string][]azure.Subscription
	calls map[string]int
}

func (m *perMGCallMock) GetCurrentUser(_ context.Context) (*azure.User, error) {
	return &azure.User{}, nil
}
func (m *perMGCallMock) GetActiveAssignments(_ context.Context) ([]azure.ActiveAssignment, error) {
	return nil, nil
}
func (m *perMGCallMock) GetEligibleRoles(_ context.Context) ([]azure.Role, error) {
	return nil, nil
}
func (m *perMGCallMock) ActivateRole(_ context.Context, _ azure.Role, _, _ string, _ int, _ string) (*azure.ScheduleResponse, error) {
	return nil, nil
}
func (m *perMGCallMock) DeactivateRole(_ context.Context, _ azure.ActiveAssignment, _ string) (*azure.ScheduleResponse, error) {
	return nil, nil
}
func (m *perMGCallMock) ListAllSubscriptionsUnderMG(_ context.Context, mgID string) ([]azure.Subscription, map[string]string, []string, error) {
	m.calls[mgID]++
	parents := map[string]string{}
	for _, s := range m.subs[mgID] {
		parents[strings.ToLower(s.ID)] = mgID
	}
	return m.subs[mgID], parents, nil, nil
}

func TestRunSearchDirectSubRoleShowsDirect(t *testing.T) {
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			subRole("/subscriptions/sub-direct", "Direct Sub", "Reader"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "(direct)") {
		t.Errorf("expected (direct) in MG column, got: %s", buf.String())
	}
}

func TestRunSearchMGRoleNoDirectLabel(t *testing.T) {
	mgScope := "/providers/Microsoft.Management/managementGroups/mg-label"
	mock := &searchMock{
		eligibleRoles: []azure.Role{searchMGRole(mgScope, "Reader")},
		mgSubs: map[string][]azure.Subscription{
			"mg-label": {{ID: "sub-mg", DisplayName: "MG Sub"}},
		},
		mgParents: map[string]map[string]string{
			"mg-label": {"sub-mg": "mg-label"},
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "(direct)") {
		t.Errorf("unexpected (direct) label for MG-scoped role, got: %s", buf.String())
	}
}

func TestBuildSearchHitsMGFilterSkipsUnrelatedMG(t *testing.T) {
	scopeA := "/providers/Microsoft.Management/managementGroups/example-mg-a"
	scopeB := "/providers/Microsoft.Management/managementGroups/example-mg-b"

	mock := &perMGCallMock{
		subs: map[string][]azure.Subscription{
			"example-mg-a": {{ID: "sub-a", DisplayName: "Sub A"}},
			"example-mg-b": {{ID: "sub-b", DisplayName: "Sub B"}},
		},
		calls: map[string]int{},
	}

	roles := []azure.Role{
		searchMGRole(scopeA, "Reader"),
		searchMGRole(scopeB, "Reader"),
	}

	hits, err := buildSearchHits(t.Context(), mock, roles, map[string]string{}, "example-mg-b", noopWriter{})
	if err != nil {
		t.Fatal(err)
	}

	if mock.calls["example-mg-a"] != 0 {
		t.Errorf("ListAllSubscriptionsUnderMG called for example-mg-a %d times, want 0", mock.calls["example-mg-a"])
	}
	if mock.calls["example-mg-b"] != 1 {
		t.Errorf("ListAllSubscriptionsUnderMG called for example-mg-b %d times, want 1", mock.calls["example-mg-b"])
	}

	var subIDs []string
	for _, h := range hits {
		subIDs = append(subIDs, h.SubscriptionID)
	}
	for _, id := range subIDs {
		if id == "sub-a" {
			t.Errorf("sub-a should not appear in hits (example-mg-a was skipped)")
		}
	}
	found := false
	for _, id := range subIDs {
		if id == "sub-b" {
			found = true
		}
	}
	if !found {
		t.Errorf("sub-b not found in hits: %v", subIDs)
	}
}

func TestRunSearchTOMLOutput(t *testing.T) {
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			{
				RoleName:     "Contributor",
				Scope:        "/subscriptions/00000000-0000-0000-0000-000000000000",
				ScopeDisplay: "my-subscription",
			},
			{
				RoleName:     "Reader",
				Scope:        "/providers/Microsoft.Management/managementGroups/my-mgmt-group",
				ScopeDisplay: "My MG",
			},
		},
		// The MG contains my-subscription, so Reader must appear in TOML output.
		mgSubs: map[string][]azure.Subscription{
			"my-mgmt-group": {
				{ID: "00000000-0000-0000-0000-000000000000", DisplayName: "my-subscription"},
			},
		},
	}
	var buf strings.Builder
	if err := runSearch(t.Context(), makeApp("", app.OutputTOML), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	// Sub-direct role: scope = subscription ARM path.
	if !strings.Contains(out, `scope         = "/subscriptions/00000000-0000-0000-0000-000000000000"`) {
		t.Errorf("expected sub ARM path in TOML output, got:\n%s", out)
	}
	// MG role: scope = MG ARM path for deterministic activation.
	if !strings.Contains(out, `scope         = "/providers/Microsoft.Management/managementGroups/my-mgmt-group"`) {
		t.Errorf("expected MG ARM path in TOML output, got:\n%s", out)
	}
	// MG role label uses the subscription display name, not the MG display name.
	if !strings.Contains(out, `label         = "Reader @ my-subscription"`) {
		t.Errorf("expected subscription-name label for MG role, got:\n%s", out)
	}
	// Regression guard: exactly two [[favorites]] blocks.
	if got := strings.Count(out, "[[favorites]]"); got != 2 {
		t.Errorf("expected 2 [[favorites]] blocks, got %d:\n%s", got, out)
	}
}

func TestTomlFromHitsParentChildMG(t *testing.T) {
	// Sub is physically under child-mg-a, but the eligibility is granted at the
	// ancestor parent-mg. The role loop looks up by parent-mg's ID, while the
	// hit's ManagementGroup is child-mg-a — they must still resolve to MG roles.
	// Regression guard: MG-inherited roles must not be dropped from TOML output
	// when the physical parent MG differs from the eligibility MG.
	mgScope := "/providers/Microsoft.Management/managementGroups/parent-mg"
	mock := &searchMock{
		eligibleRoles: []azure.Role{
			searchMGRole(mgScope, "Reader"),
			searchMGRole(mgScope, "Contributor"),
		},
		mgSubs: map[string][]azure.Subscription{
			"parent-mg": {
				{ID: "00000000-0000-0000-0000-000000000000", DisplayName: "my-subscription"},
			},
		},
		mgParents: map[string]map[string]string{
			"parent-mg": {"00000000-0000-0000-0000-000000000000": "child-mg-a"},
		},
	}
	var buf strings.Builder
	if err := runSearch(t.Context(), makeApp("", app.OutputTOML), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	if got := strings.Count(out, "[[favorites]]"); got != 2 {
		t.Errorf("expected 2 [[favorites]] blocks (MG roles must survive child-MG parent), got %d:\n%s", got, out)
	}
	for _, role := range []string{"Reader", "Contributor"} {
		if !strings.Contains(out, `role          = "`+role+`"`) {
			t.Errorf("expected role %q in TOML output:\n%s", role, out)
		}
	}
	if !strings.Contains(out, `scope         = "`+mgScope+`"`) {
		t.Errorf("expected MG ARM path scope in TOML output:\n%s", out)
	}
	if !strings.Contains(out, `label         = "Reader @ my-subscription"`) {
		t.Errorf("expected subscription-name label for MG role:\n%s", out)
	}
}
