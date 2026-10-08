// Package logging sets up the default charm logger. The TUI owns stdout
// and stderr, so logging is off (discarded) unless LGTM_DEBUG is set; then
// it goes to $XDG_STATE_HOME/lgtm/lgtm.log (default
// ~/.local/state/lgtm/lgtm.log). Modules log through the package-level
// functions of charm.land/log/v2.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"charm.land/log/v2"
)

// EnvDebug enables debug logging when set to anything but "", "0" or "false".
const EnvDebug = "LGTM_DEBUG"

// Path returns the log file path: $XDG_STATE_HOME/lgtm/lgtm.log, or
// ~/.local/state/lgtm/lgtm.log when XDG_STATE_HOME is unset.
func Path() (string, error) {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("state directory: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "lgtm", "lgtm.log"), nil
}

// Setup installs the default logger and returns a function that closes the
// log file. Without LGTM_DEBUG the default logger discards everything, so
// stray log calls can never corrupt the TUI. Call it once, early in main.
func Setup() (func(), error) {
	switch os.Getenv(EnvDebug) {
	case "", "0", "false":
		log.SetDefault(log.NewWithOptions(io.Discard, log.Options{}))
		return func() {}, nil
	}

	path, err := Path()
	if err != nil {
		return func() {}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return func() {}, fmt.Errorf("create log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return func() {}, fmt.Errorf("open log file: %w", err)
	}
	log.SetDefault(log.NewWithOptions(f, log.Options{
		Level:           log.DebugLevel,
		ReportTimestamp: true,
	}))
	log.Debug("logging enabled", "path", path)
	return func() { _ = f.Close() }, nil
}
