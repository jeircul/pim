package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme holds all Lip Gloss v2 styles for the application.
// Constructed once per render cycle using the background-color detection result.
type Theme struct {
	Accent  color.Color
	Muted   color.Color
	Danger  color.Color
	Success color.Color

	Header           lipgloss.Style
	StatusBar        lipgloss.Style
	Title            lipgloss.Style
	Active           lipgloss.Style
	Subtle           lipgloss.Style
	DangerText       lipgloss.Style
	Bold             lipgloss.Style
	Tag              lipgloss.Style
	TableHeader      lipgloss.Style
	TableRow         lipgloss.Style
	TableRowSelected lipgloss.Style
	HelpKey          lipgloss.Style
	HelpDesc         lipgloss.Style
}

// NewTheme constructs a Theme. isDark should come from tea.BackgroundColorMsg.IsDark().
func NewTheme(isDark bool) Theme {
	ld := lipgloss.LightDark(isDark)

	accent := ld(lipgloss.Color("#0066cc"), lipgloss.Color("#00ccff"))
	muted := ld(lipgloss.Color("#666666"), lipgloss.Color("#888888"))
	danger := ld(lipgloss.Color("#cc2200"), lipgloss.Color("#ff4444"))
	success := ld(lipgloss.Color("#007700"), lipgloss.Color("#44ff88"))

	return Theme{
		Accent:  accent,
		Muted:   muted,
		Danger:  danger,
		Success: success,

		Header: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true).
			PaddingLeft(1).
			PaddingRight(1),

		StatusBar: lipgloss.NewStyle().
			Foreground(muted).
			PaddingLeft(1).
			PaddingRight(1),

		Title: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true).
			PaddingBottom(1),

		Active: lipgloss.NewStyle().
			Foreground(success),

		Subtle: lipgloss.NewStyle().
			Foreground(muted),

		DangerText: lipgloss.NewStyle().
			Foreground(danger),

		Bold: lipgloss.NewStyle().
			Bold(true),

		Tag: lipgloss.NewStyle().
			Foreground(accent).
			PaddingLeft(1).
			PaddingRight(1),

		TableHeader: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true),

		TableRow: lipgloss.NewStyle(),

		TableRowSelected: lipgloss.NewStyle().
			Foreground(accent).
			Bold(true),

		HelpKey: lipgloss.NewStyle().
			Foreground(accent),

		HelpDesc: lipgloss.NewStyle().
			Foreground(muted),
	}
}
