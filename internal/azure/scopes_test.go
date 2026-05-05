package azure

import "testing"

func TestExpandScopeFilter(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantExp     string
		wantChanged bool
	}{
		{
			name:        "bare subscription GUID expands",
			input:       "00000000-0000-0000-0000-000000000000",
			wantExp:     "/subscriptions/00000000-0000-0000-0000-000000000000",
			wantChanged: true,
		},
		{
			name:        "bare MG name expands",
			input:       "example-mg",
			wantExp:     "/providers/Microsoft.Management/managementGroups/example-mg",
			wantChanged: true,
		},
		{
			name:        "ARM subscription path unchanged",
			input:       "/subscriptions/00000000-0000-0000-0000-000000000000",
			wantExp:     "/subscriptions/00000000-0000-0000-0000-000000000000",
			wantChanged: false,
		},
		{
			name:        "ARM MG path unchanged",
			input:       "/providers/Microsoft.Management/managementGroups/root",
			wantExp:     "/providers/Microsoft.Management/managementGroups/root",
			wantChanged: false,
		},
		{
			name:        "empty string unchanged",
			input:       "",
			wantExp:     "",
			wantChanged: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := ExpandScopeFilter(tt.input)
			if got != tt.wantExp {
				t.Errorf("ExpandScopeFilter(%q) expanded = %q; want %q", tt.input, got, tt.wantExp)
			}
			if changed != tt.wantChanged {
				t.Errorf("ExpandScopeFilter(%q) wasExpanded = %v; want %v", tt.input, changed, tt.wantChanged)
			}
		})
	}
}

func TestScopeMatchesBareGUID(t *testing.T) {
	scope := "/subscriptions/00000000-0000-0000-0000-000000000000"
	display := "My Subscription"

	if !ScopeMatches("00000000-0000-0000-0000-000000000000", scope, display) {
		t.Error("ScopeMatches: bare GUID should match subscription scope")
	}
	if !ScopeMatches(scope, scope, display) {
		t.Error("ScopeMatches: ARM path should match itself")
	}
	if ScopeMatches("00000000-0000-0000-0000-000000000000", "/subscriptions/other-guid", "Other") {
		t.Error("ScopeMatches: bare GUID should not match different subscription")
	}
}

func TestScopeMatchesBareMGName(t *testing.T) {
	scope := "/providers/Microsoft.Management/managementGroups/example-mg"
	display := "example-mg"

	if !ScopeMatches("example-mg", scope, display) {
		t.Error("ScopeMatches: bare MG name should match MG scope")
	}
	if ScopeMatches("OtherMG", scope, display) {
		t.Error("ScopeMatches: bare MG name should not match different MG scope")
	}
}
