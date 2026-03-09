package components

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Spinner wraps bubbles/v2 spinner.
type Spinner struct {
	model spinner.Model
}

// NewSpinner creates a MiniDot spinner with the given style.
func NewSpinner(style lipgloss.Style) Spinner {
	s := spinner.New(
		spinner.WithSpinner(spinner.MiniDot),
		spinner.WithStyle(style),
	)
	return Spinner{model: s}
}

// Init returns the initial tick command.
func (s Spinner) Init() tea.Cmd {
	return s.model.Tick
}

// Update handles tick messages.
func (s Spinner) Update(msg tea.Msg) (Spinner, tea.Cmd) {
	m, cmd := s.model.Update(msg)
	s.model = m
	return s, cmd
}

// View renders the current spinner frame.
func (s Spinner) View() string {
	return s.model.View()
}
