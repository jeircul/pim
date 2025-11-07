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
