package azpim

import "testing"

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		minutes  int
		expected string
	}{
		{"30 minutes", 30, "PT30M"},
		{"1 hour", 60, "PT1H"},
		{"90 minutes", 90, "PT1H30M"},
		{"2 hours", 120, "PT2H"},
		{"2.5 hours", 150, "PT2H30M"},
		{"8 hours", 480, "PT8H"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.minutes)
			if result != tt.expected {
				t.Errorf("formatDuration(%d) = %q; want %q", tt.minutes, result, tt.expected)
			}
		})
	}
}
