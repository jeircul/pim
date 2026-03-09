package azure

import "fmt"

const (
	minMinutes = 30
	maxMinutes = 480
)

// ClampMinutes clamps minutes to [30, 480] rounded to 30-minute increments.
func ClampMinutes(minutes int) int {
	if minutes < minMinutes {
		return minMinutes
	}
	if minutes > maxMinutes {
		return maxMinutes
	}
	return ((minutes + 15) / 30) * 30
}

// FormatDuration converts minutes to ISO 8601 duration (PT1H30M).
func FormatDuration(minutes int) string {
	hours := minutes / 60
	mins := minutes % 60
	switch {
	case mins == 0:
		return fmt.Sprintf("PT%dH", hours)
	case hours == 0:
		return fmt.Sprintf("PT%dM", mins)
	default:
		return fmt.Sprintf("PT%dH%dM", hours, mins)
	}
}

// ParseDurationMinutes parses a human duration string (1h, 30m, 1h30m, 1.5h) into minutes.
func ParseDurationMinutes(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}

	var hours, mins float64
	var parsed bool

	// Try "XhYm" or "Xh" or "Ym"
	var h, m int
	if n, _ := fmt.Sscanf(s, "%dh%dm", &h, &m); n == 2 {
		return ClampMinutes(h*60 + m), nil
	}
	if n, _ := fmt.Sscanf(s, "%dh", &h); n == 1 && (len(s) == countDigits(h)+1) {
		return ClampMinutes(h * 60), nil
	}
	if n, _ := fmt.Sscanf(s, "%dm", &m); n == 1 {
		return ClampMinutes(m), nil
	}

	// Try "1.5h"
	if n, _ := fmt.Sscanf(s, "%fh", &hours); n == 1 {
		parsed = true
		mins = hours * 60
	}

	if !parsed {
		return 0, fmt.Errorf("unrecognised duration %q; use 1h, 30m, 1h30m, or 1.5h", s)
	}

	return ClampMinutes(int(mins)), nil
}

func countDigits(n int) int {
	if n == 0 {
		return 1
	}
	c := 0
	for n > 0 {
		n /= 10
		c++
	}
	return c
}
