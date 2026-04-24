package styles

import "charm.land/bubbles/v2/key"

// KeyMap defines all global keybindings.
type KeyMap struct {
	Quit       key.Binding
	Help       key.Binding
	Back       key.Binding
	Activate   key.Binding
	Deactivate key.Binding
	Status     key.Binding
	Dashboard  key.Binding
	Favorites  key.Binding
	Up         key.Binding
	Down       key.Binding
	Enter      key.Binding
	Refresh    key.Binding
}

// DefaultKeyMap returns the application-wide default keybindings.
var DefaultKeyMap = KeyMap{
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c", "q"),
		key.WithHelp("q", "quit"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "back"),
	),
	Activate: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "activate"),
	),
	Deactivate: key.NewBinding(
		key.WithKeys("D"),
		key.WithHelp("D", "deactivate"),
	),
	Status: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "status"),
	),
	Dashboard: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "dashboard"),
	),
	Favorites: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "favorites"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("↑/k", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("↓/j", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "select"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh"),
	),
}
