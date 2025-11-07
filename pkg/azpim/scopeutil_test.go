package azpim

import "testing"

func TestManagementGroupIDFromScope(t *testing.T) {
	scope := "/providers/Microsoft.Management/managementGroups/root/providers/foo"
	got := ManagementGroupIDFromScope(scope)
	if got != "root" {
		t.Fatalf("expected root, got %q", got)
	}

	if ManagementGroupIDFromScope("/subscriptions/123") != "" {
		t.Fatalf("expected empty string for non management group scope")
	}
}

func TestSubscriptionIDFromScope(t *testing.T) {
	scope := "/subscriptions/12345678-1234-1234-1234-123456789000/resourceGroups/my-rg"
	got := SubscriptionIDFromScope(scope)
	if got != "12345678-1234-1234-1234-123456789000" {
		t.Fatalf("unexpected subscription id: %q", got)
	}

	if SubscriptionIDFromScope("/providers/Microsoft.Management/managementGroups/root") != "" {
		t.Fatalf("expected empty string for management group scope")
	}
}

func TestResourceGroupNameFromScope(t *testing.T) {
	sub, rg := ResourceGroupNameFromScope("/subscriptions/abcd/resourceGroups/test-rg/providers/foo")
	if sub != "abcd" {
		t.Fatalf("expected subscription abcd, got %q", sub)
	}
	if rg != "test-rg" {
		t.Fatalf("expected resource group test-rg, got %q", rg)
	}

	sub, rg = ResourceGroupNameFromScope("/subscriptions/abcd")
	if sub != "" || rg != "" {
		t.Fatalf("expected empty values when resource group missing")
	}
}
