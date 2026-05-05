package azure

import (
	"testing"
)

// TestClassifyChildResourcesBareTypes verifies the real PIM API type shapes (bare strings).
func TestClassifyChildResourcesBareTypes(t *testing.T) {
	resources := []childResource{
		{
			ID:   "/providers/Microsoft.Management/managementGroups/child-mg",
			Name: "child-mg",
			Type: "managementgroup",
			Properties: struct {
				DisplayName string `json:"displayName"`
			}{DisplayName: "Child MG"},
		},
		{
			ID:   "/subscriptions/00000000-0000-0000-0000-000000000001",
			Name: "00000000-0000-0000-0000-000000000001",
			Type: "subscription",
			Properties: struct {
				DisplayName string `json:"displayName"`
			}{DisplayName: "My Sub"},
		},
		{
			ID:   "/subscriptions/00000000-0000-0000-0000-000000000001/resourceGroups/my-rg",
			Name: "my-rg",
			Type: "resourcegroup",
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

// TestClassifyChildResourcesNamespacedTypes verifies namespace-prefixed type strings also work.
func TestClassifyChildResourcesNamespacedTypes(t *testing.T) {
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
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscription, got %d: %+v", len(subs), subs)
	}
}

// TestClassifyChildResourcesRGsSkipped verifies resource groups are always skipped.
func TestClassifyChildResourcesRGsSkipped(t *testing.T) {
	resources := []childResource{
		{ID: "/subscriptions/aaa/resourceGroups/rg1", Name: "rg1", Type: "resourcegroup"},
		{ID: "/subscriptions/aaa/resourceGroups/rg2", Name: "rg2", Type: "Microsoft.Resources/subscriptions/resourceGroups"},
	}
	mgs, subs := classifyChildResources(resources)
	if len(mgs) != 0 || len(subs) != 0 {
		t.Fatalf("expected no results, got mgs=%d subs=%d", len(mgs), len(subs))
	}
}
