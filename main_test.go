package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
)

func TestParseNoArgsMeansDefault(t *testing.T) {
	subj, opt, err := parse(nil)

	if err != nil {
		t.Fatal(err)
	}
	if subj.kind != subjectDefault {
		t.Errorf("kind = %v, want subjectDefault", subj.kind)
	}
	if opt.theme != "" {
		t.Errorf("theme = %q, want empty", opt.theme)
	}
}

func TestParseSingleCommit(t *testing.T) {
	subj, _, err := parse([]string{"abc1234"})

	if err != nil {
		t.Fatal(err)
	}
	if subj.kind != subjectCommit || subj.rev != "abc1234" {
		t.Errorf("subj = %+v, want commit abc1234", subj)
	}
}

func TestParseVersionFlag(t *testing.T) {
	_, opt, err := parse([]string{"--version"})

	if err != nil {
		t.Fatal(err)
	}
	if !opt.version {
		t.Error("version = false, want true")
	}
}

func TestVersionFlagPrintsVersionAndCommitAndExitsZero(t *testing.T) {
	stdout, stderr, code := runHelper(t, t.TempDir(), "--version")

	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "lgtm version dev (commit unknown)") {
		t.Errorf("stdout = %q, want version and commit", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestVersionStringUsesLdflagsValues(t *testing.T) {
	oldVersion, oldCommit := version, commit
	version, commit = "v0.1.0", "abc1234"
	t.Cleanup(func() {
		version, commit = oldVersion, oldCommit
	})

	if got := versionString(); got != "lgtm version v0.1.0 (commit abc1234)" {
		t.Errorf("versionString() = %q, want the ldflags values", got)
	}
}

func TestParseRange(t *testing.T) {
	subj, _, err := parse([]string{"main..feature"})

	if err != nil {
		t.Fatal(err)
	}
	if subj.kind != subjectRange || subj.a != "main" || subj.b != "feature" {
		t.Errorf("subj = %+v, want range main..feature", subj)
	}
}

func TestParseRejectsMalformedRanges(t *testing.T) {
	for _, arg := range []string{"..b", "a..", "a..b..c", "a...b", "..."} {
		_, _, err := parse([]string{arg})
		if err == nil || !strings.Contains(err.Error(), "invalid range") {
			t.Errorf("parse(%q) err = %v, want an invalid range error", arg, err)
		}
	}
}

func TestParseRejectsMultipleArguments(t *testing.T) {
	for _, args := range [][]string{{"a", "b"}, {"a..b", "c"}} {
		_, _, err := parse(args)
		if err == nil || !strings.Contains(err.Error(), "usage") {
			t.Errorf("parse(%v) err = %v, want a usage error", args, err)
		}
	}
}

func TestRunRangeShowsTheChanges(t *testing.T) {
	dir := repoWithRange(t)
	got := captureProgram(t)
	if err := run(dir, []string{"main..feature"}); err != nil {
		t.Fatal(err)
	}
	m, _ := got().Update(tea.WindowSizeMsg{Width: 30, Height: 5})
	screen := ansi.Strip(m.View().Content)

	if !strings.Contains(screen, "+ two") || !strings.Contains(screen, "a.txt") {
		t.Errorf("screen:\n%s\nwant the changed line of a.txt", screen)
	}
	if strings.Contains(screen, "keep") {
		t.Errorf("screen:\n%s\ndoes not want the untouched b.txt", screen)
	}
}

func TestRunCommitShowsOnlyThatCommit(t *testing.T) {
	dir := repoWithRange(t)
	got := captureProgram(t)
	head, err := repoHead(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := run(dir, []string{head}); err != nil {
		t.Fatal(err)
	}
	m, _ := got().Update(tea.WindowSizeMsg{Width: 30, Height: 5})
	screen := ansi.Strip(m.View().Content)

	if !strings.Contains(screen, "+ two") || !strings.Contains(screen, "- one") {
		t.Errorf("screen:\n%s\nwant exactly the change of the commit", screen)
	}
	if strings.Contains(screen, "keep") {
		t.Errorf("screen:\n%s\ndoes not want the change of the parent commit", screen)
	}
}

func TestRunWithoutChangesShowsHintNamingTheSubject(t *testing.T) {
	dir := repoWithRange(t)
	got := captureProgram(t)
	if err := run(dir, []string{"main..main"}); err != nil {
		t.Fatal(err)
	}
	m, _ := got().Update(tea.WindowSizeMsg{Width: 30, Height: 5})
	screen := ansi.Strip(m.View().Content)

	if !strings.Contains(screen, "Nothing to review.") ||
		!strings.Contains(screen, "No changes in main..main.") {
		t.Errorf("screen:\n%s\nwant the friendly hint naming main..main", screen)
	}
}

func TestOutsideRepositoryFailsWithClearErrorAndExitCode(t *testing.T) {
	_, stderr, code := runHelper(t, t.TempDir())

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "not inside a git repository") {
		t.Errorf("stderr = %q, want a clear repository error", stderr)
	}
}

func TestInvalidRangeFailsWithClearErrorAndExitCode(t *testing.T) {
	_, stderr, code := runHelper(t, t.TempDir(), "a..b..c")

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, `invalid range "a..b..c"`) {
		t.Errorf("stderr = %q, want an invalid range error", stderr)
	}
}

func TestUnresolvableRefFailsWithClearErrorAndExitCode(t *testing.T) {
	dir := repoWithRange(t)
	_, stderr, code := runHelper(t, dir, "does-not-exist")

	if code != exitError {
		t.Errorf("exit code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, `cannot resolve "does-not-exist"`) {
		t.Errorf("stderr = %q, want an unresolvable ref error", stderr)
	}
}

// captureProgram replaces the TUI runner with a stub that captures the
// model, so run() can be tested without a terminal. The returned getter
// yields the captured model after run() has returned.
func captureProgram(t *testing.T) func() tea.Model {
	t.Helper()
	var got tea.Model
	old := runProgram
	runProgram = func(m tea.Model) (tea.Model, error) {
		got = m
		return m, nil
	}
	t.Cleanup(func() { runProgram = old })
	return func() tea.Model { return got }
}

// runHelper executes the test binary as the real lgtm program: it runs
// run() and exits with the production exit code, so stdout, stderr and
// the exit code are observable.
func runHelper(t *testing.T, dir string, args ...string) (stdout string, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"LGTM_HELPER_PROCESS=1",
		"LGTM_HELPER_ARGS="+strings.Join(args, " "))
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return outBuf.String(), errBuf.String(), ee.ExitCode()
		}
		t.Fatal(err)
	}
	return outBuf.String(), errBuf.String(), 0
}

// TestHelperProcess is not a test: it is the lgtm program inside the test
// binary, started by runHelper.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("LGTM_HELPER_PROCESS") != "1" {
		return
	}
	args := strings.Fields(os.Getenv("LGTM_HELPER_ARGS"))
	if err := run(".", args); err != nil {
		fmt.Fprintln(os.Stderr, "lgtm:", err)
		os.Exit(exitError)
	}
}

// repoWithRange creates a repository on disk with a main and a feature
// branch: base adds a.txt = "one" and b.txt = "keep", feature changed
// a.txt to "two" and left b.txt alone.
func repoWithRange(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false,
		git.WithDefaultBranch(plumbing.NewBranchReferenceName("main")))
	if err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(dir+"/"+name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := func(msg string) {
		t.Helper()
		w, err := repo.Worktree()
		if err != nil {
			t.Fatal(err)
		}
		if err := w.AddWithOptions(&git.AddOptions{All: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Commit(msg, &git.CommitOptions{
			Author: &object.Signature{Name: "t", Email: "t@example.com", When: time.Unix(0, 0)},
		}); err != nil {
			t.Fatal(err)
		}
	}
	write("a.txt", "one\n")
	write("b.txt", "keep\n")
	commit("base")
	w, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName("feature"),
		Create: true,
		Keep:   true,
	}); err != nil {
		t.Fatal(err)
	}
	write("a.txt", "two\n")
	commit("feature work")
	return dir
}

func repoHead(dir string) (string, error) {
	repo, err := git.PlainOpen(dir)
	if err != nil {
		return "", err
	}
	ref, err := repo.Head()
	if err != nil {
		return "", err
	}
	return ref.Hash().String(), nil
}

// TestRunPersistsTheReadStatus pins the acceptance criterion
// "Persistenz des Gelesen-Status": running lgtm in a repository creates
// .git/info/exclude with the .lgtm/ entry, and once the end of a file
// is visible the read status is written to .lgtm/state.json.
func TestRunPersistsTheReadStatus(t *testing.T) {
	dir := repoWithRange(t)
	got := captureProgram(t)
	if err := run(dir, []string{"main..feature"}); err != nil {
		t.Fatal(err)
	}
	m, _ := got().Update(tea.WindowSizeMsg{Width: 30, Height: 5})

	exclude, err := os.ReadFile(filepath.Join(dir, ".git", "info", "exclude"))
	if err != nil {
		t.Fatalf(".git/info/exclude: %v", err)
	}
	if !strings.Contains(string(exclude), ".lgtm/") {
		t.Errorf(".git/info/exclude lacks the .lgtm/ entry:\n%s", exclude)
	}

	// a.txt is one line and fits the screen: its end is visible from
	// the start, so the read status is persisted right away.
	data, err := os.ReadFile(filepath.Join(dir, ".lgtm", "state.json"))
	if err != nil {
		t.Fatalf("state.json after the end became visible: %v", err)
	}
	if !strings.Contains(string(data), "a.txt") || !strings.Contains(string(data), "true") {
		t.Errorf("state.json lacks the read a.txt:\n%s", data)
	}
	_ = m
}
