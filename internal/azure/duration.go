package azure

import (
	"fmt"
	"strconv"
	"strings"
)

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
// Input is case-insensitive. Negative values are rejected.
func ParseDurationMinutes(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	s = strings.ToLower(strings.TrimSpace(s))

	// "XhYm" — must end exactly after the 'm'
	if hPart, rest, ok := strings.Cut(s, "h"); ok && strings.HasSuffix(rest, "m") {
		mPart := strings.TrimSuffix(rest, "m")
		if h, err := strconv.Atoi(hPart); err == nil {
			if m, err := strconv.Atoi(mPart); err == nil {
				if h < 0 || m < 0 {
					return 0, fmt.Errorf("negative duration not allowed")
				}
				return ClampMinutes(h*60 + m), nil
			}
		}
	}

	// "Xh" — ends with h, prefix is an integer or float
	if strings.HasSuffix(s, "h") {
		numPart := strings.TrimSuffix(s, "h")
		if h, err := strconv.Atoi(numPart); err == nil {
			if h < 0 {
				return 0, fmt.Errorf("negative duration not allowed")
			}
			return ClampMinutes(h * 60), nil
		}
		if f, err := strconv.ParseFloat(numPart, 64); err == nil {
			if f < 0 {
				return 0, fmt.Errorf("negative duration not allowed")
			}
			return ClampMinutes(int(f * 60)), nil
		}
	}

	// "Ym" — ends with m, prefix is an integer
	if strings.HasSuffix(s, "m") {
		numPart := strings.TrimSuffix(s, "m")
		if m, err := strconv.Atoi(numPart); err == nil {
			if m < 0 {
				return 0, fmt.Errorf("negative duration not allowed")
			}
			return ClampMinutes(m), nil
		}
	}

	return 0, fmt.Errorf("unrecognised duration %q; use 1h, 30m, 1h30m, or 1.5h", s)
}
