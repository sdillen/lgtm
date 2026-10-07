// Command lgtm is a terminal UI for reading and commenting on code changes.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/config"
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

// options are the command line flags.
type options struct {
	theme string // --theme: theme name, overrides the config
}

func main() {
	if err := run(".", os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lgtm:", err)
		os.Exit(exitError)
	}
}

func run(dir string, args []string) error {
	subj, opt, err := parse(args)
	if err != nil {
		return err
	}
	cfgDir, err := config.Dir()
	if err != nil {
		return err
	}
	th, err := resolveTheme(opt.theme, cfgDir, backgroundKind)
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
	_, err = runProgram(ui.New(files, subjectLabel(subj, changes), th))
	return err
}

// backgroundKind asks the terminal for its background colour. It is a
// variable so tests can substitute the terminal query.
var backgroundKind = func() (theme.Kind, bool) {
	return theme.BackgroundKind(os.Stdin, os.Stdout)
}

// resolveTheme picks the theme: the --theme flag wins, then the config
// file, then automatic light/dark detection. The theme is resolved for the
// terminal's colour profile, so terminals without truecolor fall back to
// the 256 colour palette.
func resolveTheme(flagName, cfgDir string, detect func() (theme.Kind, bool)) (theme.Theme, error) {
	name := flagName
	if name == "" {
		cfg, err := config.Load(cfgDir)
		if err != nil {
			return theme.Theme{}, err
		}
		name = cfg.Theme
	}
	profile := colorprofile.Detect(os.Stderr, os.Environ())
	if name != "" {
		return theme.Load(name, cfgDir, profile)
	}
	kind, _ := detect()
	return theme.Default(kind).ForProfile(profile), nil
}

// parse turns the command line into a review subject and options. Flags
// may appear before or after the subject; -- ends the flags. Exactly one
// positional argument is allowed: a single commit or a range a..b.
func parse(args []string) (subject, options, error) {
	var opt options
	var positional []string
	rest := args
	for len(rest) > 0 {
		arg := rest[0]
		rest = rest[1:]
		switch {
		case arg == "--":
			positional = append(positional, rest...)
			rest = nil
		case strings.HasPrefix(arg, "--theme="):
			opt.theme = strings.TrimPrefix(arg, "--theme=")
		case arg == "--theme":
			if len(rest) == 0 {
				return subject{}, opt, errors.New("flag needs an argument: --theme")
			}
			opt.theme, rest = rest[0], rest[1:]
		case strings.HasPrefix(arg, "-") && arg != "-":
			return subject{}, opt, fmt.Errorf("flag provided but not defined: %s", arg)
		default:
			positional = append(positional, arg)
		}
	}
	subj, err := parseSubject(positional)
	return subj, opt, err
}

// parseSubject validates the positional arguments: exactly one, either a
// range a..b or a single commit.
func parseSubject(args []string) (subject, error) {
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
		return subject{}, errors.New("usage: lgtm [--theme <name>] [<commit> | <a>..<b>]")
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
