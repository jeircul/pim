package state

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	configFile     = "config.toml"
	stateFile      = "state.toml"
	maxRecentJusts = 10
)

// Favorite is a saved role+scope+duration combo.
type Favorite struct {
	Label    string `toml:"label"`
	Role     string `toml:"role"`
	Scope    string `toml:"scope"`
	Duration string `toml:"duration"`
	Key      int    `toml:"key"` // 1-9 for dashboard shortcuts
}

// Preferences holds user-editable preferences.
type Preferences struct {
	DefaultDuration string `toml:"default_duration"`
}

// Config is the hand-editable config file (~/.config/pim/config.toml).
type Config struct {
	Preferences Preferences `toml:"preferences"`
	Favorites   []Favorite  `toml:"favorites"`
}

// State is auto-managed runtime state (~/.config/pim/state.toml).
type State struct {
	Version              int      `toml:"version"`
	RecentJustifications []string `toml:"recent_justifications"`
}

// Store manages persistent config and state files.
type Store struct {
	dir    string
	Config Config
	State  State
}

// New opens (or initialises) the store at the given directory.
// Pass an empty string to use the default ~/.config/pim/.
func New(dir string) (*Store, error) {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("locate home dir: %w", err)
		}
		dir = filepath.Join(home, ".config", "pim")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	s := &Store{
		dir: dir,
		Config: Config{
			Preferences: Preferences{DefaultDuration: "1h"},
		},
		State: State{Version: 1},
	}

	_ = s.loadConfig()
	_ = s.loadState()
	return s, nil
}

// SaveConfig persists config.toml.
func (s *Store) SaveConfig() error {
	return s.write(configFile, s.Config)
}

// SaveState persists state.toml.
func (s *Store) SaveState() error {
	return s.write(stateFile, s.State)
}

// AddRecentJustification prepends a justification to the recent list (deduped, max 10).
func (s *Store) AddRecentJustification(j string) {
	if j == "" {
		return
	}
	out := make([]string, 0, maxRecentJusts)
	out = append(out, j)
	for _, existing := range s.State.RecentJustifications {
		if existing == j {
			continue
		}
		out = append(out, existing)
		if len(out) >= maxRecentJusts {
			break
		}
	}
	s.State.RecentJustifications = out
}

// FavoriteByKey returns the favorite assigned to a number key (1-9).
func (s *Store) FavoriteByKey(key int) (Favorite, bool) {
	for _, f := range s.Config.Favorites {
		if f.Key == key {
			return f, true
		}
	}
	return Favorite{}, false
}

// UpsertFavorite adds or replaces a favorite by label.
func (s *Store) UpsertFavorite(f Favorite) {
	for i, existing := range s.Config.Favorites {
		if existing.Label == f.Label {
			s.Config.Favorites[i] = f
			return
		}
	}
	s.Config.Favorites = append(s.Config.Favorites, f)
}

// RemoveFavorite deletes a favorite by label.
func (s *Store) RemoveFavorite(label string) {
	out := s.Config.Favorites[:0]
	for _, f := range s.Config.Favorites {
		if f.Label != label {
			out = append(out, f)
		}
	}
	s.Config.Favorites = out
}

// DefaultDurationMinutes returns the configured default duration in minutes.
func (s *Store) DefaultDurationMinutes() int {
	d := s.Config.Preferences.DefaultDuration
	if d == "" {
		return 60
	}
	return parseDurationMinutes(d)
}

func parseDurationMinutes(s string) int {
	// "XhYm"
	var h, m int
	if n, _ := fmt.Sscanf(s, "%dh%dm", &h, &m); n == 2 {
		return h*60 + m
	}
	// "Xh" — must end with 'h' after digits
	if len(s) >= 2 && s[len(s)-1] == 'h' {
		if n, _ := fmt.Sscanf(s[:len(s)-1], "%d", &h); n == 1 {
			return h * 60
		}
	}
	// "Xm" — must end with 'm' after digits
	if len(s) >= 2 && s[len(s)-1] == 'm' {
		if n, _ := fmt.Sscanf(s[:len(s)-1], "%d", &m); n == 1 {
			return m
		}
	}
	return 60
}

func (s *Store) loadConfig() error {
	_, err := toml.DecodeFile(filepath.Join(s.dir, configFile), &s.Config)
	return err
}

func (s *Store) loadState() error {
	_, err := toml.DecodeFile(filepath.Join(s.dir, stateFile), &s.State)
	return err
}

func (s *Store) write(name string, v any) error {
	path := filepath.Join(s.dir, name)
	tmp := path + ".tmp." + fmt.Sprintf("%d", time.Now().UnixNano())
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	if err := toml.NewEncoder(f).Encode(v); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("encode %s: %w", name, err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close %s: %w", name, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename %s: %w", name, err)
	}
	return nil
}
