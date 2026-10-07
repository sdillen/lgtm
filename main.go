// Command lgtm is a terminal UI for reading and commenting on code changes.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/source"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// exitError is the exit code for errors; 0 and 1 are reserved for the
// review outcome.
const exitError = 2

// runProgram runs the TUI. It is a variable so tests can capture the model
// instead of opening a terminal.
var runProgram = func(m tea.Model) (tea.Model, error) {
	return tea.NewProgram(m).Run()
}

// subjectKind tells which review subject the command line asked for.
type subjectKind int

const (
	subjectDefault subjectKind = iota // everything since the default branch
	subjectRange                      // a..b
	subjectCommit                     // a single commit
)

// subject is a parsed review subject.
type subject struct {
	kind subjectKind
	a, b string // range sides
	rev  string // single commit
}

func main() {
	if err := run(".", os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lgtm:", err)
		os.Exit(exitError)
	}
}

func run(dir string, args []string) error {
	subj, err := parse(args)
	if err != nil {
		return err
	}
	src, err := source.Open(dir)
	if err != nil {
		return err
	}
	var changes source.Changes
	switch subj.kind {
	case subjectDefault:
		changes, err = src.Default()
	case subjectRange:
		changes, err = src.Range(subj.a, subj.b)
	case subjectCommit:
		changes, err = src.Commit(subj.rev)
	}
	if err != nil {
		return err
	}
	files := make([]diff.ReviewFile, len(changes.Files))
	for i, f := range changes.Files {
		files[i] = diff.New(f.Path, f.Old, f.New)
	}
	// The default dark theme until the theme selection lands.
	th := theme.Default(theme.KindDark)
	_, err = runProgram(ui.New(files, subjectLabel(subj, changes), th))
	return err
}

// parse turns the command line into a review subject. Exactly one
// argument is allowed: a single commit or a range a..b.
func parse(args []string) (subject, error) {
	switch {
	case len(args) == 0:
		return subject{kind: subjectDefault}, nil
	case len(args) == 1:
		arg := args[0]
		if a, b, ok := strings.Cut(arg, ".."); ok {
			if a == "" || b == "" || strings.Contains(b, "..") || strings.HasPrefix(b, ".") {
				return subject{}, fmt.Errorf("invalid range %q: expected <a>..<b>", arg)
			}
			return subject{kind: subjectRange, a: a, b: b}, nil
		}
		return subject{kind: subjectCommit, rev: arg}, nil
	default:
		return subject{}, errors.New("usage: lgtm [<commit> | <a>..<b>]")
	}
}

// subjectLabel names the review subject for the UI, e.g. in the hint shown
// when there is nothing to review.
func subjectLabel(subj subject, changes source.Changes) string {
	switch subj.kind {
	case subjectDefault:
		return changes.Base
	case subjectRange:
		return subj.a + ".." + subj.b
	case subjectCommit:
		return subj.rev
	}
	return ""
}
