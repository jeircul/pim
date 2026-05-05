package azure

import (
	"strings"
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

	var mgs []ManagementGroup
	var subs []Subscription
	for _, item := range resources {
		lower := strings.ToLower(item.Type)
		switch {
		case strings.HasSuffix(lower, "/resourcegroups"):
			// not a direct child of an MG; ignore
		case strings.HasSuffix(lower, "/managementgroups"):
			mgs = append(mgs, ManagementGroup{ID: item.Name, DisplayName: displayOr(item)})
		case strings.HasSuffix(lower, "/subscriptions"):
			subID := SubscriptionIDFromScope(item.ID)
			if subID != "" {
				subs = append(subs, Subscription{ID: subID, DisplayName: displayOr(item)})
			}
		}
	}

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
