package components

import (
	"fmt"

	"charm.land/lipgloss/v2"
)

// RenderHeader renders the top header bar.
// width is the current terminal width.
func RenderHeader(headerStyle, subtleStyle lipgloss.Style, title, version string, width int) string {
	left := headerStyle.Render(fmt.Sprintf("pim %s", version))
	right := subtleStyle.Render(title)

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + fmt.Sprintf("%*s", gap, "") + right
}
