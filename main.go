// Command lgtm is a terminal UI for reading and commenting on code changes.
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/config"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/export"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/keymap"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/search"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/source"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// Exit codes: 0 after :lgtm or without open comments, 1 with open
// comments, 2 and above for errors.
const (
	exitOK          = 0
	exitOpenComment = 1
	exitError       = 2
)

// version and commit are set at build time with -ldflags, e.g.
//
//	go build -ldflags "-X main.version=v0.1.0 -X main.commit=$(git rev-parse --short HEAD)"
//
// Without ldflags the binary reports itself as an unversioned dev build.
var (
	version = "dev"
	commit  = "unknown"
)

// versionString describes the build for --version.
func versionString() string {
	return fmt.Sprintf("lgtm version %s (commit %s)", version, commit)
}

// usageString is the text of --help.
func usageString() string {
	return `lgtm - terminal UI for reviewing code changes

Usage:
  lgtm [flags] [<commit> | <a>..<b>]   review the changes (default: since the default branch)
  lgtm clear [-y]                      delete the review state of this repository

Flags:
  --theme <name>      theme name, overrides the config
  --print-template    print the built-in prompt template and exit
  -y, --yes           with clear: do not ask for confirmation
  --version           print the version and exit
  -h, --help          print this help and exit

clear removes .lgtm/ (state and review.md) and the snapshot refs under
refs/lgtm/. Without a terminal it requires --yes.

Exit codes:
  0   approved (:lgtm) or no open comments
  1   quit with open comments
  2   error
`
}

// stdin and stdinIsTerminal are variables so tests can answer the
// confirmation of clear.
var (
	stdin           io.Reader = os.Stdin
	stdinIsTerminal           = func() bool { return term.IsTerminal(os.Stdin.Fd()) }
)

// runClear deletes the review state of the repository containing dir: the
// .lgtm directory and the snapshot refs of the review. Without yes it asks
// first, and without a terminal it refuses rather than deleting silently.
func runClear(dir string, yes bool) (int, error) {
	src, err := source.Open(dir)
	if err != nil {
		return exitError, err
	}
	root := src.WorktreeDir()
	if root == "" {
		return exitError, errors.New("repository has no working tree")
	}
	state, err := review.Load(root)
	if err != nil {
		return exitError, err
	}
	if _, err := os.Stat(filepath.Join(root, ".lgtm")); errors.Is(err, fs.ErrNotExist) {
		fmt.Println("nothing to clear")
		return exitOK, nil
	}
	if !yes {
		if !stdinIsTerminal() {
			return exitError, errors.New("refusing to clear without a terminal; use --yes")
		}
		fmt.Printf("Delete review state (%d open comments, %d rounds)? [y/N] ", len(state.OpenComments()), state.Round())
		answer, _ := bufio.NewReader(stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Println("aborted")
			return exitOK, nil
		}
	}
	if state.ID != "" {
		if err := src.DeleteSnapshots("refs/lgtm/" + state.ID + "/"); err != nil {
			return exitError, err
		}
	}
	if _, err := review.Clear(root); err != nil {
		return exitError, err
	}
	fmt.Println("review state cleared")
	return exitOK, nil
}

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
	help          bool   // --help, -h: print the usage and exit
	clear         bool   // clear: delete the review state
	yes           bool   // --yes, -y: clear without asking
	theme         string // --theme: theme name, overrides the config
	version       bool   // --version: print version and exit
	printTemplate bool   // --print-template: print the built-in prompt template
}

func main() {
	code, err := run(".", os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "lgtm:", err)
		os.Exit(exitError)
	}
	os.Exit(code)
}

// run resolves the review, starts the TUI and returns the process exit
// code of the finished review.
func run(dir string, args []string) (int, error) {
	subj, opt, err := parse(args)
	if err != nil {
		return exitError, err
	}
	if opt.help {
		fmt.Print(usageString())
		return exitOK, nil
	}
	if opt.version {
		fmt.Println(versionString())
		return exitOK, nil
	}
	if opt.printTemplate {
		fmt.Print(export.DefaultTemplate)
		return exitOK, nil
	}
	if opt.clear {
		return runClear(dir, opt.yes)
	}
	cfgDir, err := config.Dir()
	if err != nil {
		return exitError, err
	}
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return exitError, err
	}
	th, err := resolveTheme(opt.theme, cfg, cfgDir, backgroundKind)
	if err != nil {
		return exitError, err
	}
	km, err := ui.Keymap(keyOverrides(cfg))
	if err != nil {
		return exitError, fmt.Errorf("config: %w", err)
	}
	// The animations (smooth scroll, overlay slide-in) are on by
	// default; the config turns them off. The start screen is part of
	// the startup.
	animate := true
	if cfg.SmoothScroll != nil {
		animate = *cfg.SmoothScroll
	}
	src, err := source.Open(dir)
	if err != nil {
		return exitError, err
	}
	// The background change check and the UI's own git access (R, the
	// repo search) may overlap; the lock keeps go-git single-threaded.
	repo := &lockedSource{src: src}
	changes, err := repo.resolve(subj)
	if err != nil {
		return exitError, err
	}
	files := reviewFiles(changes)
	reload := func() ([]diff.ReviewFile, error) {
		changes, err := repo.resolve(subj)
		if err != nil {
			return nil, err
		}
		return reviewFiles(changes), nil
	}
	// The watcher only feeds the change hint; without it (e.g. the
	// inotify limit is reached) the review still runs and R works.
	var changesCh <-chan struct{}
	if w, err := src.Watch(); err == nil {
		defer func() { _ = w.Close() }()
		changesCh = w.Changes()
	}

	// The review state persists into .lgtm/state.json at the worktree
	// root; .lgtm/ is excluded from git so it is never committed.
	var state *review.State
	stateDir := src.WorktreeDir()
	if stateDir != "" {
		state, err = review.Load(stateDir)
		if err != nil {
			return exitError, err
		}
		if err := review.EnsureExcluded(stateDir); err != nil {
			return exitError, err
		}
	}
	// The rounds snapshot the reviewed state under refs/lgtm/ and
	// compare a new round with the previous one; they need the state.
	var interdiff func() ([]diff.ReviewFile, error)
	var snapshot func() error
	round := 0
	if state != nil {
		r := rounds{repo: repo, subj: subj, state: state}
		if err := r.begin(files); err != nil {
			return exitError, err
		}
		if err := state.Save(stateDir); err != nil {
			return exitError, err
		}
		interdiff, snapshot, round = r.interdiff(), r.snapshot, state.Round()
	}
	profile := termProfile()
	final, err := runProgram(ui.New(files, subjectLabel(subj, changes), th, ui.Options{
		Keymap: km,
		LoadTheme: func(name string) (theme.Theme, error) {
			return theme.Load(name, cfgDir, profile)
		},
		State:  state,
		Dir:    stateDir,
		CfgDir: cfgDir,
		Branch: src.HeadBranch(),
		Base:   changes.Base,
		Round:  round,
		// The repo search and the all-files picker ask the source
		// for the working tree.
		Search:      search.New(repo),
		Changes:     changesCh,
		Reload:      reload,
		Interdiff:   interdiff,
		Snapshot:    snapshot,
		StartScreen: true,
		Icons:       cfg.Icons,
		Animate:     &animate,
	}))
	if err != nil {
		return exitError, err
	}
	return final.(ui.Model).ExitCode(), nil
}

// lockedSource serialises the git access of the source: the UI reads
// files and resolves the review on key presses, the change check
// resolves it in the background.
type lockedSource struct {
	mu  sync.Mutex
	src *source.Source
}

// resolve resolves the review subject.
func (l *lockedSource) resolve(subj subject) (source.Changes, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch subj.kind {
	case subjectRange:
		return l.src.Range(subj.a, subj.b)
	case subjectCommit:
		return l.src.Commit(subj.rev)
	}
	return l.src.Default()
}

// Files lists the files of the working tree, for the repo search.
func (l *lockedSource) Files() ([]string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.src.Files()
}

// Read reads a file of the working tree, for the repo search.
func (l *lockedSource) Read(path string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.src.Read(path)
}

// reviewFiles turns the changes of the review subject into the
// ReviewFile models of the UI: renames are detected, every change is
// classified (binary, mode-only, submodule, generated), and generated
// files are sorted to the end of the file list, where their collapsed
// default does not get in the way. The file list stays sorted by path.
func reviewFiles(changes source.Changes) []diff.ReviewFile {
	attrs := diff.ParseGitAttributes(changes.GitAttributes)
	files := make([]diff.File, len(changes.Files))
	for i, f := range changes.Files {
		files[i] = diff.File{
			Path:      f.Path,
			Status:    toDiffStatus(f.Status),
			Old:       f.Old,
			New:       f.New,
			Submodule: f.Submodule,
			OldMode:   f.OldMode,
			NewMode:   f.NewMode,
		}
	}
	files = diff.Renames(files)

	out := make([]diff.ReviewFile, 0, len(files))
	for _, f := range files {
		var rf diff.ReviewFile
		switch {
		case f.Submodule:
			rf = diff.Submodule(f.Path, f.Old, f.New)
		case f.OldMode != 0 && f.NewMode != 0 && f.OldMode != f.NewMode:
			rf = diff.Mode(f.Path, f.OldMode, f.NewMode)
		default:
			rf = diff.NewWithAttrs(f.Path, f.Old, f.New, attrs.Generated)
		}
		rf.OldPath = f.OldPath
		out = append(out, rf)
	}
	// The rename detection pairs added with deleted files; the result
	// is sorted again so the file list keeps its path order.
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Path < out[j].Path
	})
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Kind != diff.KindGenerated && out[j].Kind == diff.KindGenerated
	})
	return out
}

// toDiffStatus converts the status of the source module. The explicit
// switch keeps the two enumerations decoupled.
func toDiffStatus(s source.Status) diff.Status {
	switch s {
	case source.Added:
		return diff.StatusAdded
	case source.Deleted:
		return diff.StatusDeleted
	default:
		return diff.StatusModified
	}
}

// keyOverrides turns the key settings of the config into keymap
// overrides.
func keyOverrides(cfg config.Config) keymap.Overrides {
	ov := keymap.Overrides{Leader: cfg.Leader}
	if len(cfg.Keys) > 0 {
		ov.Keys = make(map[keymap.Action][]string, len(cfg.Keys))
		for action, seqs := range cfg.Keys {
			ov.Keys[keymap.Action(action)] = seqs
		}
	}
	return ov
}

// termProfile detects the colour profile of the terminal.
func termProfile() colorprofile.Profile {
	return colorprofile.Detect(os.Stderr, os.Environ())
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
func resolveTheme(flagName string, cfg config.Config, cfgDir string, detect func() (theme.Kind, bool)) (theme.Theme, error) {
	name := flagName
	if name == "" {
		name = cfg.Theme
	}
	profile := termProfile()
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
		case arg == "--version":
			opt.version = true
		case arg == "--help" || arg == "-h":
			opt.help = true
		case arg == "--yes" || arg == "-y":
			opt.yes = true
		case arg == "--print-template":
			opt.printTemplate = true
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
	if len(positional) > 0 && positional[0] == "clear" {
		if len(positional) > 1 {
			return subject{}, opt, errors.New("clear takes no arguments")
		}
		opt.clear = true
		return subject{kind: subjectDefault}, opt, nil
	}
	if opt.yes {
		return subject{}, opt, errors.New("--yes only applies to clear")
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
		return subject{}, errors.New("usage: lgtm [flags] [<commit> | <a>..<b>] (see lgtm --help)")
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
