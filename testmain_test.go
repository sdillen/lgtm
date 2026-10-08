package main

import (
	"os"
	"testing"
)

// TestMain points the config directory at an empty temporary one, so
// that the tests run with the defaults and not with the config.toml of
// whoever runs them (a custom leader key breaks the key presses).
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "lgtm-test-config")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_CONFIG_HOME", dir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
