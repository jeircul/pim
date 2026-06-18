package state

import (
	"os"
	"testing"
)

func TestStoreRecentJustifications(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.AddRecentJustification("first")
	s.AddRecentJustification("second")
	s.AddRecentJustification("first") // dedup: should move to front

	if len(s.State.RecentJustifications) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(s.State.RecentJustifications))
	}
	if s.State.RecentJustifications[0] != "first" {
		t.Fatalf("expected first entry to be 'first', got %q", s.State.RecentJustifications[0])
	}
}

func TestStoreFavorites(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.UpsertFavorite(Favorite{Label: "prod", Role: "Reader", Scope: "/subscriptions/abc", Duration: "1h", Justification: "test reason", Key: 1})
	s.UpsertFavorite(Favorite{Label: "dev", Role: "Owner", Scope: "/subscriptions/xyz", Duration: "2h", Justification: "test reason", Key: 2})

	f, ok := s.FavoriteByKey(1)
	if !ok || f.Label != "prod" {
		t.Fatalf("expected prod favorite at key 1, got ok=%v label=%q", ok, f.Label)
	}

	// Update
	s.UpsertFavorite(Favorite{Label: "prod", Role: "Contributor", Scope: "/subscriptions/abc", Duration: "1h", Justification: "test reason", Key: 1})
	f, _ = s.FavoriteByKey(1)
	if f.Role != "Contributor" {
		t.Fatalf("expected updated role Contributor, got %q", f.Role)
	}

	s.RemoveFavorite("dev")
	if len(s.Config.Favorites) != 1 {
		t.Fatalf("expected 1 favorite after remove, got %d", len(s.Config.Favorites))
	}
}

func TestStorePersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.AddRecentJustification("saved justification")
	if err := s.SaveState(); err != nil {
		t.Fatal(err)
	}

	// Re-open
	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s2.State.RecentJustifications) == 0 || s2.State.RecentJustifications[0] != "saved justification" {
		t.Fatalf("expected persisted justification, got %v", s2.State.RecentJustifications)
	}
}

func TestStoreDefaultDuration(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.Config.Preferences.DefaultDuration = "2h"
	if s.DefaultDurationMinutes() != 120 {
		t.Fatalf("expected 120 minutes for 2h, got %d", s.DefaultDurationMinutes())
	}

	s.Config.Preferences.DefaultDuration = "30m"
	if s.DefaultDurationMinutes() != 30 {
		t.Fatalf("expected 30 minutes, got %d", s.DefaultDurationMinutes())
	}
}

func TestStoreConfigPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	s.UpsertFavorite(Favorite{Label: "test", Role: "Reader", Scope: "/sub/abc", Duration: "1h", Justification: "test reason", Key: 3})
	if err := s.SaveConfig(); err != nil {
		t.Fatal(err)
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, ok := s2.FavoriteByKey(3)
	if !ok || f.Label != "test" {
		t.Fatalf("expected persisted favorite, got ok=%v label=%q", ok, f.Label)
	}
	if f.Justification != "test reason" {
		t.Fatalf("expected persisted justification %q, got %q", "test reason", f.Justification)
	}
}

func TestFavoriteComplete(t *testing.T) {
	tests := []struct {
		name string
		fav  Favorite
		want bool
	}{
		{
			name: "all fields set",
			fav:  Favorite{Role: "Reader", Scope: "/sub/abc", Duration: "1h", Justification: "reason"},
			want: true,
		},
		{
			name: "missing justification",
			fav:  Favorite{Role: "Reader", Scope: "/sub/abc", Duration: "1h"},
			want: false,
		},
		{
			name: "missing role",
			fav:  Favorite{Scope: "/sub/abc", Duration: "1h", Justification: "reason"},
			want: false,
		},
		{
			name: "all empty",
			fav:  Favorite{},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.fav.Complete()
			if got != tc.want {
				t.Errorf("Complete() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestAddRecentActivation(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	a1 := RecentActivation{Role: "Contributor", Scope: "/subscriptions/aaa", Duration: "1h", Justification: "first"}
	a2 := RecentActivation{Role: "Reader", Scope: "/subscriptions/bbb", Duration: "2h", Justification: "second"}
	s.AddRecentActivation(a1)
	s.AddRecentActivation(a2)
	s.AddRecentActivation(RecentActivation{Role: "contributor", Scope: "/subscriptions/aaa", Duration: "1H", Justification: "updated"})

	acts := s.RecentActivations()
	if len(acts) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(acts))
	}
	if acts[0].Role != "contributor" || acts[0].Justification != "updated" {
		t.Fatalf("expected deduped entry at front, got %+v", acts[0])
	}
	if acts[1].Role != "Reader" {
		t.Fatalf("expected Reader second, got %+v", acts[1])
	}
}

func TestRecentActivationPersistence(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	a := RecentActivation{Role: "Owner", Scope: "/subscriptions/ccc", Duration: "8h", Justification: "persist test"}
	s.AddRecentActivation(a)
	if err := s.SaveState(); err != nil {
		t.Fatal(err)
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	acts := s2.RecentActivations()
	if len(acts) == 0 || acts[0].Role != "Owner" || acts[0].Justification != "persist test" {
		t.Fatalf("expected persisted activation, got %v", acts)
	}
}

func TestRecentActivationEligibilityScope(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := RecentActivation{
		Role:             "Contributor",
		Scope:            "/subscriptions/00000000-0000-0000-0000-000000000000",
		EligibilityScope: "/providers/Microsoft.Management/managementGroups/my-mgmt-group",
		Duration:         "1h",
		Justification:    "test",
	}
	s.AddRecentActivation(a)
	acts := s.RecentActivations()
	if acts[0].EligibilityScope != "/providers/Microsoft.Management/managementGroups/my-mgmt-group" {
		t.Errorf("expected eligibility scope, got %q", acts[0].EligibilityScope)
	}
	b := RecentActivation{
		Role:          "Reader",
		Scope:         "/subscriptions/00000000-0000-0000-0000-000000000001",
		Duration:      "1h",
		Justification: "test",
	}
	s.AddRecentActivation(b)
	acts = s.RecentActivations()
	if acts[0].EligibilityScope != "" {
		t.Errorf("expected empty eligibility scope for old entry, got %q", acts[0].EligibilityScope)
	}
}
