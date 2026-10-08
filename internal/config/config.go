// Package config loads the lgtm configuration from
// $XDG_CONFIG_HOME/lgtm/config.toml (default ~/.config/lgtm/config.toml).
// The same directory holds custom themes under themes/.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the lgtm configuration. Unknown keys are ignored so that newer
// config files stay readable.
type Config struct {
	// Theme names the theme to use, e.g. "tokyo-night-day" or a custom
	// theme from the themes/ directory. Empty means automatic
	// light/dark detection.
	Theme string `toml:"theme"`

	// Leader is the leader key in vim notation, e.g. "," or "<space>".
	// Empty means the default <space>.
	Leader string `toml:"leader"`

	// Keys rebinds actions to key sequences in vim notation, e.g.
	// top = "<home>" or down = ["j", "<down>"]. An empty list unbinds
	// the action.
	Keys map[string]KeySeqs `toml:"keys"`

	// SmoothScroll disables the animated scrolling of half-page moves
	// and jumps when set to false; nil means on.
	SmoothScroll *bool `toml:"smooth_scroll"`

	// Icons enables Nerd Font icons in the file list. Without a Nerd
	// Font installed the list stays clean. Default off.
	Icons bool `toml:"icons"`
}

// KeySeqs are the key sequences of one action. In TOML they are a single
// string or an array of strings.
type KeySeqs []string

// UnmarshalTOML accepts a string or an array of strings.
func (k *KeySeqs) UnmarshalTOML(v any) error {
	switch v := v.(type) {
	case string:
		*k = KeySeqs{v}
		return nil
	case []any:
		seqs := make(KeySeqs, 0, len(v))
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				return fmt.Errorf("key binding %v: want a string", e)
			}
			seqs = append(seqs, s)
		}
		*k = seqs
		return nil
	}
	return fmt.Errorf("key binding %v: want a string or an array of strings", v)
}

// Dir returns the lgtm config directory: $XDG_CONFIG_HOME/lgtm, or
// ~/.config/lgtm when XDG_CONFIG_HOME is unset.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("config directory: %w", err)
	}
	return filepath.Join(base, "lgtm"), nil
}

// Load reads the config.toml in dir. A missing file is not an error and
// yields an empty Config; a malformed file is.
func Load(dir string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}
