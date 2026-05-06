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
func (m *searchMock) ListManagementGroupSubscriptions(_ context.Context, mgID string) ([]azure.Subscription, error) {
	m.mgSubsCalls++
	if m.mgSubsErr != nil {
		return nil, m.mgSubsErr
	}
	return m.mgSubs[mgID], nil
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
			subRole("/subscriptions/sub-1", "q901-prod", "Reader"),
			subRole("/subscriptions/sub-2", "other-sub", "Reader"),
		},
	}
	var buf bytes.Buffer
	if err := runSearch(t.Context(), makeApp("q901", app.OutputTable), mock, &buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "q901-prod") {
		t.Errorf("expected q901-prod, got: %s", out)
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
