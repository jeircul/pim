package favorites

import (
	"testing"

	"github.com/jeircul/pim/internal/state"
)

func TestDeleteCharKeyFieldLeavesLabelIntact(t *testing.T) {
	var m Model
	f := state.Favorite{Label: "prod-owner", Key: 5}

	got := m.deleteChar(f, fieldKey)

	if got.Key != 0 {
		t.Errorf("Key = %d, want 0", got.Key)
	}
	if got.Label != "prod-owner" {
		t.Errorf("Label = %q, want %q", got.Label, "prod-owner")
	}
}

func TestEditFieldRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		fld  editField
		get  func(state.Favorite) string
	}{
		{"label", fieldLabel, func(f state.Favorite) string { return f.Label }},
		{"role", fieldRole, func(f state.Favorite) string { return f.Role }},
		{"scope", fieldScope, func(f state.Favorite) string { return f.Scope }},
		{"duration", fieldDuration, func(f state.Favorite) string { return f.Duration }},
		{"justification", fieldJustification, func(f state.Favorite) string { return f.Justification }},
	}

	var m Model
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := m.appendChar(state.Favorite{}, tc.fld, "a")
			f = m.appendChar(f, tc.fld, "b")
			if got := tc.get(f); got != "ab" {
				t.Fatalf("append: field = %q, want %q", got, "ab")
			}
			f = m.deleteChar(f, tc.fld)
			if got := tc.get(f); got != "a" {
				t.Errorf("delete: field = %q, want %q", got, "a")
			}
		})
	}
}

func TestEveryEditFieldIsReachable(t *testing.T) {
	var m Model
	seen := map[string]editField{}

	for fld := editField(0); fld < fieldCount; fld++ {
		if fld == fieldKey {
			continue
		}
		f := m.appendChar(state.Favorite{}, fld, "z")
		switch {
		case f.Label == "z":
			seen["label"] = fld
		case f.Role == "z":
			seen["role"] = fld
		case f.Scope == "z":
			seen["scope"] = fld
		case f.Duration == "z":
			seen["duration"] = fld
		case f.Justification == "z":
			seen["justification"] = fld
		default:
			t.Errorf("field %d wrote to no Favorite field", fld)
		}
	}

	// Justification is required by Favorite.Complete, so the editor must expose it.
	if _, ok := seen["justification"]; !ok {
		t.Error("no edit field maps to Justification; favorites created in the TUI can never be Complete()")
	}
	if len(seen) != int(fieldCount)-1 {
		t.Errorf("mapped %d distinct fields, want %d (two fields share a target)", len(seen), int(fieldCount)-1)
	}
}
