// Package tui provides the Bubble Tea TUI for pim.
// Theme and KeyMap types live in the styles sub-package to avoid import cycles.
package tui

import "github.com/jeircul/pim/internal/tui/styles"

// Re-export for callers that only import the parent package.
type Theme = styles.Theme
type KeyMap = styles.KeyMap

// NewTheme constructs a Theme from the background darkness hint.
var NewTheme = styles.NewTheme

// DefaultKeyMap is the application-wide default keybindings.
var DefaultKeyMap = styles.DefaultKeyMap
