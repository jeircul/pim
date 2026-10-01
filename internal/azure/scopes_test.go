package azure

import "testing"

func TestBareSubscriptionGUID(t *testing.T) {
	const guid = "00000000-0000-0000-0000-000000000001"
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"bare guid", guid, guid},
		{"slash prefix", "/subscriptions/" + guid, guid},
		{"trailing slash", "/subscriptions/" + guid + "/", guid},
		{"with resource group", "/subscriptions/" + guid + "/resourceGroups/x", ""},
		{"non-guid string", "my-subscription", ""},
		{"empty", "", ""},
		{"uppercase prefix", "/SUBSCRIPTIONS/" + guid, guid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BareSubscriptionGUID(tt.input)
			if got != tt.want {
				t.Errorf("BareSubscriptionGUID(%q) = %q; want %q", tt.input, got, tt.want)
			}
		})
	}
}

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

func TestValidateScope(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		ok    bool
	}{
		{"subscription", "/subscriptions/00000000-0000-0000-0000-000000000000", true},
		{"resource group", "/subscriptions/00000000-0000-0000-0000-000000000000/resourceGroups/my-rg", true},
		{"management group", "/providers/Microsoft.Management/managementGroups/my-mgmt-group", true},
		{"empty", "", false},
		{"no leading slash", "subscriptions/x", false},
		{"userinfo", "@evil.example/resourceGroups/x", false},
		{"at sign mid path", "/subscriptions/x@evil.example", false},
		{"scheme", "/x/https://evil.example", false},
		{"query", "/subscriptions/x?api-version=1", false},
		{"fragment", "/subscriptions/x#frag", false},
		{"backslash", `/subscriptions/x\evil`, false},
		{"dotdot segment", "/subscriptions/x/../../evil", false},
		{"dotdot inside name", "/subscriptions/x/resourceGroups/a..b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateScope(tt.scope)
			if (err == nil) != tt.ok {
				t.Errorf("validateScope(%q) = %v; want ok=%v", tt.scope, err, tt.ok)
			}
		})
	}
}

func TestIsResourceGroupScope(t *testing.T) {
	tests := []struct {
		name  string
		scope string
		want  bool
	}{
		{"resource group", "/subscriptions/x/resourceGroups/my-rg", true},
		{"lowercase keywords", "/subscriptions/x/resourcegroups/my-rg", true},
		{"resource below rg", "/subscriptions/x/resourceGroups/my-rg/providers/Microsoft.Web/sites/a", true},
		{"subscription", "/subscriptions/x", false},
		{"userinfo trick", "@evil.example/resourceGroups/x", false},
		{"no leading slash", "subscriptions/x/resourceGroups/my-rg", false},
		{"rg not at prefix", "/providers/foo/subscriptions/x/resourceGroups/my-rg", false},
		{"empty sub id", "/subscriptions//resourceGroups/my-rg", false},
		{"empty rg name", "/subscriptions/x/resourceGroups/", false},
		{"management group", "/providers/Microsoft.Management/managementGroups/my-mgmt-group", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsResourceGroupScope(tt.scope); got != tt.want {
				t.Errorf("IsResourceGroupScope(%q) = %v; want %v", tt.scope, got, tt.want)
			}
		})
	}
}
