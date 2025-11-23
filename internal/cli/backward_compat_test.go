package cli

import "testing"

func TestParseDurationBackwardCompatibility(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"plain 1", "1", 60},
		{"plain 2", "2", 120},
		{"plain 8", "8", 480},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDuration(tt.input)
			if err != nil {
				t.Errorf("parseDuration(%q) unexpected error: %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("parseDuration(%q) = %d minutes; want %d minutes", tt.input, result, tt.expected)
			}
		})
	}
}
