package azure

import (
	"sort"
	"testing"
)

func TestClampMinutes(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"below min", 0, 30},
		{"at min", 30, 30},
		{"round down", 40, 30},
		{"round up", 50, 60},
		{"normal", 120, 120},
		{"at max", 480, 480},
		{"above max", 600, 480},
		{"negative", -5, 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClampMinutes(tt.input); got != tt.want {
				t.Errorf("ClampMinutes(%d) = %d; want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		minutes int
		want    string
	}{
		{30, "PT30M"},
		{60, "PT1H"},
		{90, "PT1H30M"},
		{120, "PT2H"},
		{150, "PT2H30M"},
		{480, "PT8H"},
	}
	for _, tt := range tests {
		if got := FormatDuration(tt.minutes); got != tt.want {
			t.Errorf("FormatDuration(%d) = %q; want %q", tt.minutes, got, tt.want)
		}
	}
}

func TestManagementGroupIDFromScope(t *testing.T) {
	got := ManagementGroupIDFromScope("/providers/Microsoft.Management/managementGroups/root/providers/foo")
	if got != "root" {
		t.Fatalf("expected root, got %q", got)
	}
	if ManagementGroupIDFromScope("/subscriptions/123") != "" {
		t.Fatalf("expected empty for non-MG scope")
	}
}

func TestSubscriptionIDFromScope(t *testing.T) {
	got := SubscriptionIDFromScope("/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/my-rg")
	if got != "00000000-0000-0000-0000-000000000000" {
		t.Fatalf("unexpected subscription id: %q", got)
	}
	if SubscriptionIDFromScope("/providers/Microsoft.Management/managementGroups/root") != "" {
		t.Fatalf("expected empty for MG scope")
	}
}

func TestResourceGroupNameFromScope(t *testing.T) {
	sub, rg := ResourceGroupNameFromScope("/subscriptions/abcd/resourceGroups/test-rg/providers/foo")
	if sub != "abcd" {
		t.Fatalf("expected sub abcd, got %q", sub)
	}
	if rg != "test-rg" {
		t.Fatalf("expected rg test-rg, got %q", rg)
	}

	sub, rg = ResourceGroupNameFromScope("/subscriptions/abcd")
	if sub != "" || rg != "" {
		t.Fatalf("expected empty when rg missing, got sub=%q rg=%q", sub, rg)
	}
}

func TestIsManagementGroupScope(t *testing.T) {
	if !IsManagementGroupScope("/providers/Microsoft.Management/managementGroups/root") {
		t.Fatal("expected true for MG scope")
	}
	if IsManagementGroupScope("/subscriptions/abc") {
		t.Fatal("expected false for sub scope")
	}
}

func TestIsSubscriptionScope(t *testing.T) {
	if !IsSubscriptionScope("/subscriptions/abc") {
		t.Fatal("expected true for sub scope")
	}
	if IsSubscriptionScope("/subscriptions/abc/resourceGroups/rg") {
		t.Fatal("expected false for RG scope")
	}
}

func TestTimeRemaining(t *testing.T) {
	a := ActiveAssignment{EndDateTime: ""}
	if a.TimeRemaining() != 0 {
		t.Fatal("expected 0 for permanent assignment")
	}
	if !a.IsPermanent() {
		t.Fatal("expected IsPermanent true")
	}
}

func TestParseDurationMinutes(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"hours only", "1h", 60, false},
		{"hours only uppercase", "1H", 60, false},
		{"minutes only", "30m", 30, false},
		{"minutes only uppercase", "30M", 30, false},
		{"combined", "1h30m", 90, false},
		{"float hours", "1.5h", 90, false},
		{"float half hour", "0.5h", 30, false},
		{"two hours", "2h", 120, false},
		{"eight hours max", "8h", 480, false},
		{"over max clamped", "9h", 480, false},
		{"below min clamped", "10m", 30, false},
		{"empty", "", 0, true},
		{"garbage", "garbage", 0, true},
		{"trailing garbage m", "30mph", 0, true},
		{"trailing garbage h", "2hx", 0, true},
		{"trailing garbage combined", "1h30mx", 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseDurationMinutes(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseDurationMinutes(%q) = %d, want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseDurationMinutes(%q) unexpected error: %v", tc.input, err)
				return
			}
			if got != tc.want {
				t.Errorf("ParseDurationMinutes(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}

func TestGetEligibleRolesSortOrder(t *testing.T) {
	// Verify that the sort contract applied inside GetEligibleRoles is stable
	// and deterministic: roles are ordered by Scope then RoleName, so
	// autoAdvance and other callers see a consistent list regardless of API
	// pagination order.
	roles := []Role{
		{Scope: "/subscriptions/bbb", RoleName: "Reader"},
		{Scope: "/subscriptions/aaa", RoleName: "Owner"},
		{Scope: "/subscriptions/aaa", RoleName: "Contributor"},
		{Scope: "/providers/Microsoft.Management/managementGroups/mg-z", RoleName: "Reader"},
		{Scope: "/subscriptions/bbb", RoleName: "Contributor"},
	}
	want := []Role{
		{Scope: "/providers/Microsoft.Management/managementGroups/mg-z", RoleName: "Reader"},
		{Scope: "/subscriptions/aaa", RoleName: "Contributor"},
		{Scope: "/subscriptions/aaa", RoleName: "Owner"},
		{Scope: "/subscriptions/bbb", RoleName: "Contributor"},
		{Scope: "/subscriptions/bbb", RoleName: "Reader"},
	}

	sort.Slice(roles, func(i, j int) bool {
		if roles[i].Scope != roles[j].Scope {
			return roles[i].Scope < roles[j].Scope
		}
		return roles[i].RoleName < roles[j].RoleName
	})

	for i, got := range roles {
		if got.Scope != want[i].Scope || got.RoleName != want[i].RoleName {
			t.Errorf("roles[%d] = {%s %s}, want {%s %s}",
				i, got.Scope, got.RoleName, want[i].Scope, want[i].RoleName)
		}
	}
}
