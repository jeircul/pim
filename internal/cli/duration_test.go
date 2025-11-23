package cli

import "testing"

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
		wantErr  bool
	}{
		{"1 hour", "1h", 60, false},
		{"2 hours", "2h", 120, false},
		{"30 minutes", "30m", 30, false},
		{"90 minutes", "90m", 90, false},
		{"1.5 hours", "1.5h", 90, false},
		{"1 hour 30 minutes", "1h30m", 90, false},
		{"2 hours 30 minutes", "2h30m", 150, false},
		{"plain number 3", "3", 180, false},
		{"plain number 1", "1", 60, false},
		{"8 hours", "8h", 480, false},
		{"uppercase H", "2H", 120, false},
		{"uppercase M", "45M", 45, false},
		{"mixed case", "1H30M", 90, false},
		{"empty string", "", 60, false},
		{"missing number", "h", 0, true},
		{"only letters", "abc", 0, true},
		{"negative", "-1h", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDuration(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseDuration(%q) expected error but got none", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("parseDuration(%q) unexpected error: %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("parseDuration(%q) = %d; want %d", tt.input, result, tt.expected)
			}
		})
	}
}
