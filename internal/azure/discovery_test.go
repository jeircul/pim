package azure

import (
	"testing"
)

func TestListManagementGroupChildrenTypeClassification(t *testing.T) {
	resources := []childResource{
		{
			ID:   "/providers/Microsoft.Management/managementGroups/child-mg",
			Name: "child-mg",
			Type: "Microsoft.Management/managementGroups",
			Properties: struct {
				DisplayName string `json:"displayName"`
			}{DisplayName: "Child MG"},
		},
		{
			ID:   "/subscriptions/00000000-0000-0000-0000-000000000001",
			Name: "00000000-0000-0000-0000-000000000001",
			Type: "Microsoft.Resources/subscriptions",
			Properties: struct {
				DisplayName string `json:"displayName"`
			}{DisplayName: "My Sub"},
		},
		{
			ID:   "/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg",
			Name: "my-rg",
			Type: "Microsoft.Resources/subscriptions/resourceGroups",
		},
	}

	mgs, subs := classifyChildResources(resources)

	if len(mgs) != 1 {
		t.Fatalf("expected 1 management group, got %d", len(mgs))
	}
	if mgs[0].DisplayName != "Child MG" {
		t.Errorf("expected display name 'Child MG', got %q", mgs[0].DisplayName)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(subs))
	}
	if subs[0].DisplayName != "My Sub" {
		t.Errorf("expected display name 'My Sub', got %q", subs[0].DisplayName)
	}
}

func TestListManagementGroupChildrenFiltersDeepDescendants(t *testing.T) {
	resources := []childResource{
		// direct child MG (4 segments: /providers/Microsoft.Management/managementGroups/<id>) — keep
		{
			ID:   "/providers/Microsoft.Management/managementGroups/direct-mg",
			Name: "direct-mg",
			Type: "Microsoft.Management/managementGroups",
			Properties: struct {
				DisplayName string `json:"displayName"`
			}{DisplayName: "Direct MG"},
		},
		// nested grandchild MG (would be same 5 segments — MG IDs are always flat)
		// Azure returns nested MGs with the same 5-segment shape; depth is encoded in
		// the hierarchy, not the ID. The segment filter correctly passes all MG IDs.
		// Test that a malformed/unexpected deep path is excluded:
		{
			ID:   "/providers/Microsoft.Management/managementGroups/parent/children/nested-mg",
			Name: "nested-mg",
			Type: "Microsoft.Management/managementGroups",
			Properties: struct {
				DisplayName string `json:"displayName"`
			}{DisplayName: "Nested MG"},
		},
		// direct child subscription (2 segments) — keep
		{
			ID:   "/subscriptions/aaaaaaaa-0000-0000-0000-000000000001",
			Name: "aaaaaaaa-0000-0000-0000-000000000001",
			Type: "Microsoft.Resources/subscriptions",
			Properties: struct {
				DisplayName string `json:"displayName"`
			}{DisplayName: "Direct Sub"},
		},
		// subscription nested under a resource group path (>2 segments) — exclude
		{
			ID:   "/subscriptions/bbbbbbbb-0000-0000-0000-000000000002/resourceGroups/rg1",
			Name: "bbbbbbbb-0000-0000-0000-000000000002",
			Type: "Microsoft.Resources/subscriptions",
			Properties: struct {
				DisplayName string `json:"displayName"`
			}{DisplayName: "Deep Sub"},
		},
	}

	mgs, subs := classifyChildResources(resources)

	if len(mgs) != 1 {
		t.Fatalf("expected 1 management group after depth filter, got %d", len(mgs))
	}
	if mgs[0].DisplayName != "Direct MG" {
		t.Errorf("expected 'Direct MG', got %q", mgs[0].DisplayName)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription after depth filter, got %d", len(subs))
	}
	if subs[0].DisplayName != "Direct Sub" {
		t.Errorf("expected 'Direct Sub', got %q", subs[0].DisplayName)
	}
}

func TestCountSegments(t *testing.T) {
	cases := []struct {
		path string
		want int
	}{
		{"/subscriptions/abc", 2},
		{"/providers/Microsoft.Management/managementGroups/root", 4},
		{"/subscriptions/abc/resourceGroups/rg1", 4},
		{"", 0},
		{"/", 0},
	}
	for _, tc := range cases {
		got := countSegments(tc.path)
		if got != tc.want {
			t.Errorf("countSegments(%q) = %d, want %d", tc.path, got, tc.want)
		}
	}
}
