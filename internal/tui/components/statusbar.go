package components

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
)

// RenderStatusBar renders the bottom status bar with keybinding hints.
// helpKeyStyle and helpDescStyle are used to style key and description respectively.
func RenderStatusBar(helpKeyStyle, helpDescStyle, subtleStyle lipgloss.Style, hints []key.Binding, msg string) string {
	var parts []string
	for _, b := range hints {
		if !b.Enabled() {
			continue
		}
		h := b.Help()
		parts = append(parts, helpKeyStyle.Render(h.Key)+" "+helpDescStyle.Render(h.Desc))
	}
	bar := strings.Join(parts, "  ")
	if msg != "" {
		bar += "  " + subtleStyle.Render(msg)
	}
	return bar
}
