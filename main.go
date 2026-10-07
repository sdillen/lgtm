// Command lgtm is a terminal UI for reading and commenting on code changes.
package main

import (
	"errors"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/source"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// exitError is the exit code for errors; 0 and 1 are reserved for the
// review outcome.
const exitError = 2

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lgtm:", err)
		os.Exit(exitError)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		return errors.New("usage: lgtm (no arguments supported yet)")
	}
	src, err := source.Open(".")
	if err != nil {
		return err
	}
	changes, err := src.Default()
	if err != nil {
		return err
	}
	files := make([]diff.ReviewFile, len(changes.Files))
	for i, f := range changes.Files {
		files[i] = diff.New(f.Path, f.Old, f.New)
	}
	_, err = tea.NewProgram(ui.New(files)).Run()
	return err
}
