package favorites

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/jeircul/pim/internal/state"
	"github.com/jeircul/pim/internal/tui/components"
	"github.com/jeircul/pim/internal/tui/styles"
)

// ActivateMsg is sent when the user triggers a favorite activation.
type ActivateMsg struct{ Favorite state.Favorite }

// DoneMsg is sent when the user exits the favorites screen.
type DoneMsg struct{}

type favStep int

const (
	favStepList favStep = iota
	favStepEdit
	favStepDelete
)

type editField int

const (
	fieldLabel editField = iota
	fieldRole
	fieldScope
	fieldDuration
	fieldKey
	fieldCount // sentinel
)

// Model is the favorites management screen.
type Model struct {
	theme   styles.Theme
	keys    styles.KeyMap
	store   *state.Store
	cursor  int
	step    favStep
	editIdx int // index being edited (-1 = new)
	edit    state.Favorite
	editFld editField
	width   int
	height  int
	saveErr error
}

// New creates a favorites Model.
func New(theme styles.Theme, keys styles.KeyMap, store *state.Store) Model {
	return Model{
		theme:   theme,
		keys:    keys,
		store:   store,
		editIdx: -1,
	}
}

// Init is a no-op.
func (m Model) Init() tea.Cmd { return nil }

// Editing reports whether the model is in a text-input mode that should
// swallow global hotkeys (such as ?, q).
func (m Model) Editing() bool { return m.step == favStepEdit }

// Update handles messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyPressMsg:
		switch m.step {
		case favStepList:
			return m.updateList(msg)
		case favStepEdit:
			return m.updateEdit(msg)
		case favStepDelete:
			return m.updateDelete(msg)
		}
	}
	return m, nil
}

func (m Model) updateList(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	favs := m.store.Favorites()
	switch {
	case key.Matches(msg, m.keys.Up):
		if m.cursor > 0 {
			m.cursor--
		}
	case key.Matches(msg, m.keys.Down):
		if m.cursor < len(favs)-1 {
			m.cursor++
		}
	case key.Matches(msg, m.keys.Enter):
		if len(favs) > 0 {
			fav := favs[m.cursor]
			return m, func() tea.Msg { return ActivateMsg{Favorite: fav} }
		}
	case msg.String() == "n":
		m.editIdx = -1
		m.edit = state.Favorite{}
		m.editFld = fieldLabel
		m.step = favStepEdit
	case msg.String() == "e":
		if len(favs) > 0 {
			m.editIdx = m.cursor
			m.edit = favs[m.cursor]
			m.editFld = fieldLabel
			m.step = favStepEdit
		}
	case msg.String() == "x", msg.String() == "delete":
		if len(favs) > 0 {
			m.step = favStepDelete
		}
	case key.Matches(msg, m.keys.Back), msg.String() == "esc", msg.String() == "q":
		return m, func() tea.Msg { return DoneMsg{} }
	}
	return m, nil
}

func (m Model) updateEdit(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "tab", "down", "j":
		m.editFld = (m.editFld + 1) % fieldCount
	case "shift+tab", "up", "k":
		if m.editFld == 0 {
			m.editFld = fieldCount - 1
		} else {
			m.editFld--
		}
	case "enter":
		m.store.UpsertFavorite(m.edit)
		if err := m.store.SaveConfig(); err != nil {
			m.saveErr = fmt.Errorf("save favorite: %w", err)
		} else {
			m.saveErr = nil
		}
		m.step = favStepList
	case "esc":
		m.step = favStepList
	case "backspace":
		m.edit = m.deleteChar(m.edit, m.editFld)
	default:
		if len(msg.String()) == 1 {
			m.edit = m.appendChar(m.edit, m.editFld, msg.String())
		}
	}
	return m, nil
}

func (m Model) updateDelete(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "y", "enter":
		favs := m.store.Favorites()
		if m.cursor < len(favs) {
			m.store.RemoveFavorite(favs[m.cursor].Label)
			if err := m.store.SaveConfig(); err != nil {
				m.saveErr = fmt.Errorf("delete favorite: %w", err)
			} else {
				m.saveErr = nil
			}
			if m.cursor >= len(m.store.Favorites()) && m.cursor > 0 {
				m.cursor--
			}
		}
		m.step = favStepList
	case "n", "esc":
		m.step = favStepList
	}
	return m, nil
}

func (m Model) deleteChar(f state.Favorite, fld editField) state.Favorite {
	ptr := m.fieldPtr(&f, fld)
	if len(*ptr) > 0 {
		*ptr = (*ptr)[:len(*ptr)-1]
	}
	return f
}

func (m Model) appendChar(f state.Favorite, fld editField, ch string) state.Favorite {
	if fld == fieldKey {
		if ch >= "0" && ch <= "9" {
			f.Key = int(ch[0] - '0')
		}
		return f
	}
	ptr := m.fieldPtr(&f, fld)
	*ptr += ch
	return f
}

func (m Model) fieldPtr(f *state.Favorite, fld editField) *string {
	switch fld {
	case fieldRole:
		return &f.Role
	case fieldScope:
		return &f.Scope
	case fieldDuration:
		return &f.Duration
	default:
		return &f.Label
	}
}

// View renders the favorites screen.
func (m Model) View() string {
	var sb strings.Builder

	switch m.step {
	case favStepList:
		sb.WriteString(m.theme.Title.Render("Favorites") + "\n\n")
		favs := m.store.Favorites()
		if len(favs) == 0 {
			sb.WriteString(m.theme.Subtle.Render("  no favorites yet — press n to add one") + "\n")
		} else {
			for i, f := range favs {
				cur := "  "
				if i == m.cursor {
					cur = m.theme.TableRowSelected.Render("▸") + " "
				}
				keyTag := "    "
				if f.Key >= 1 && f.Key <= 9 {
					keyTag = m.theme.Tag.Render(fmt.Sprintf("[%d]", f.Key)) + " "
				}
				line := cur + keyTag + m.theme.Bold.Render(padRight(f.Label, 20)) +
					m.theme.Subtle.Render(fmt.Sprintf("  %s  %s  %s", f.Role, f.Scope, f.Duration))
				sb.WriteString(line + "\n")
			}
		}
		sb.WriteString("\n")
		if m.saveErr != nil {
			sb.WriteString(m.theme.DangerText.Render(m.saveErr.Error()) + "\n\n")
		}
		hints := []key.Binding{m.keys.Up, m.keys.Down, m.keys.Enter, m.keys.Back}
		sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints,
			"n new  e edit  x delete  enter activate"))

	case favStepEdit:
		title := "Edit favorite"
		if m.editIdx == -1 {
			title = "New favorite"
		}
		sb.WriteString(m.theme.Title.Render(title) + "\n\n")

		fields := []struct {
			name string
			val  string
			fld  editField
		}{
			{"Label   ", m.edit.Label, fieldLabel},
			{"Role    ", m.edit.Role, fieldRole},
			{"Scope   ", m.edit.Scope, fieldScope},
			{"Duration", m.edit.Duration, fieldDuration},
			{"Key (1-9)", fmt.Sprintf("%d", m.edit.Key), fieldKey},
		}
		for _, f := range fields {
			label := m.theme.Subtle.Render(f.name + ": ")
			val := f.val
			cursor := ""
			if f.fld == m.editFld {
				label = m.theme.TableRowSelected.Render(f.name + ": ")
				cursor = "█"
			}
			sb.WriteString("  " + label + m.theme.Bold.Render(val) + cursor + "\n")
		}
		sb.WriteString("\n")
		hints := []key.Binding{m.keys.Enter, m.keys.Back}
		sb.WriteString(components.RenderStatusBar(m.theme.HelpKey, m.theme.HelpDesc, m.theme.Subtle, hints,
			"tab next field  enter save  esc cancel"))

	case favStepDelete:
		favs := m.store.Favorites()
		name := ""
		if m.cursor < len(favs) {
			name = favs[m.cursor].Label
		}
		sb.WriteString(m.theme.Title.Render("Delete favorite") + "\n\n")
		sb.WriteString(m.theme.Subtle.Render(fmt.Sprintf("Delete %q?  y / n", name)) + "\n")
	}

	return sb.String()
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
