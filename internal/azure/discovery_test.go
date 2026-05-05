package azure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteDiscoveryDump verifies that writeDiscoveryDump creates a 0600 JSON file
// containing scope and value when PIM_DEBUG_DISCOVERY=1.
func TestWriteDiscoveryDump(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	resources := []childResource{
		{ID: "/subscriptions/abc", Name: "abc", Type: "subscription"},
	}
	writeDiscoveryDump("/subscriptions/abc", resources)

	entries, err := os.ReadDir(filepath.Join(dir, ".config", "pim"))
	if err != nil {
		t.Fatalf("config dir not created: %v", err)
	}
	var dumpFile string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "eligible-child-resources-") && strings.HasSuffix(e.Name(), ".json") {
			dumpFile = filepath.Join(dir, ".config", "pim", e.Name())
		}
	}
	if dumpFile == "" {
		t.Fatal("no dump file written")
	}

	info, err := os.Stat(dumpFile)
	if err != nil {
		t.Fatalf("stat dump file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected 0600 permissions, got %04o", perm)
	}

	raw, err := os.ReadFile(dumpFile)
	if err != nil {
		t.Fatalf("read dump file: %v", err)
	}
	var dump discoveryDump
	if err := json.Unmarshal(raw, &dump); err != nil {
		t.Fatalf("unmarshal dump: %v", err)
	}
	if dump.Scope != "/subscriptions/abc" {
		t.Errorf("unexpected scope %q", dump.Scope)
	}
	if len(dump.Value) != 1 || dump.Value[0].Name != "abc" {
		t.Errorf("unexpected value %+v", dump.Value)
	}
}

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
