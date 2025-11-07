package cli

import "testing"

func TestTryParseSelections(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		limit    int
		wantOK   bool
		wantErr  bool
		expected []int
	}{
		{
			name:     "simple sequence",
			input:    "1,2,3",
			limit:    5,
			wantOK:   true,
			expected: []int{1, 2, 3},
		},
		{
			name:     "duplicates removed",
			input:    "1, 2, 2",
			limit:    5,
			wantOK:   true,
			expected: []int{1, 2},
		},
		{
			name:    "non numeric falls back to search",
			input:   "1,a",
			limit:   5,
			wantOK:  false,
			wantErr: false,
		},
		{
			name:    "out of range",
			input:   "7",
			limit:   5,
			wantOK:  true,
			wantErr: true,
		},
		{
			name:    "empty component",
			input:   "1, ",
			limit:   5,
			wantOK:  true,
			wantErr: true,
		},
		{
			name:    "blank input",
			input:   "",
			limit:   5,
			wantOK:  false,
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := tryParseSelections(tc.input, tc.limit)
			if ok != tc.wantOK {
				t.Fatalf("want ok=%v, got %v", tc.wantOK, ok)
			}
			if tc.wantErr && err == nil {
				t.Fatalf("expected error but got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.expected != nil {
				if len(got) != len(tc.expected) {
					t.Fatalf("expected %d items, got %d", len(tc.expected), len(got))
				}
				for i, v := range tc.expected {
					if got[i] != v {
						t.Fatalf("expected %d at position %d, got %d", v, i, got[i])
					}
				}
			}
		})
	}
}

func TestFilterViewBySubstring(t *testing.T) {
	items := []viewItem[string]{
		{idx: 0, value: "S001"},
		{idx: 1, value: "S002"},
		{idx: 2, value: "S101"},
		{idx: 3, value: "A200"},
	}
	keys := []string{"s001-alpha", "s002-beta", "s101-gamma", "a200"}

	filtered, total := filterViewBySubstring(items, keys, "s00", 10)
	if total != 2 {
		t.Fatalf("expected total=2, got %d", total)
	}
	if len(filtered) != 2 {
		t.Fatalf("expected 2 filtered items, got %d", len(filtered))
	}
	if filtered[0].value != "S001" || filtered[1].value != "S002" {
		t.Fatalf("unexpected order: %#v", filtered)
	}

	filtered, total = filterViewBySubstring(items, keys, "s1", 1)
	if total != 1 {
		t.Fatalf("expected total=1, got %d", total)
	}
	if len(filtered) != 1 || filtered[0].value != "S101" {
		t.Fatalf("expected S101, got %#v", filtered)
	}

	filtered, total = filterViewBySubstring(items, keys, "missing", 10)
	if total != 0 || len(filtered) != 0 {
		t.Fatalf("expected no matches, got total=%d filtered=%d", total, len(filtered))
	}
}
