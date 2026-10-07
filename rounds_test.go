package main

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	git "github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
)

// runReview runs lgtm in dir with args and returns the model at a
// fixed terminal size.
func runReview(t *testing.T, dir string, args ...string) tea.Model {
	t.Helper()
	got := captureProgram(t)
	if _, err := run(dir, args); err != nil {
		t.Fatal(err)
	}
	m, _ := got().Update(tea.WindowSizeMsg{Width: 60, Height: 8})
	return m
}

// snapshotFile reads path from the snapshot commit of round n.
func snapshotFile(t *testing.T, dir string, n int, path string) string {
	t.Helper()
	s, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := git.PlainOpen(dir)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := repo.Reference(plumbing.ReferenceName(s.Snapshot(n)), true)
	if err != nil {
		t.Fatalf("snapshot of round %d: %v", n, err)
	}
	c, err := repo.CommitObject(ref.Hash())
	if err != nil {
		t.Fatal(err)
	}
	f, err := c.File(path)
	if err != nil {
		t.Fatal(err)
	}
	content, err := f.Contents()
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// loadState loads the persisted review state of dir.
func loadState(t *testing.T, dir string) *review.State {
	t.Helper()
	s, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRunStartsTheFirstRoundWithASnapshotOfTheWorkingTree(t *testing.T) {
	dir := repoWithRange(t)
	writeFile(t, filepath.Join(dir, "a.txt"), "three\n")

	m := runReview(t, dir)

	if s := ansi.Strip(m.View().Content); !strings.Contains(s, "Runde 1") {
		t.Errorf("status line misses round 1:\n%s", s)
	}
	if n := loadState(t, dir).Round(); n != 1 {
		t.Fatalf("round = %d, want 1", n)
	}
	if got := snapshotFile(t, dir, 1, "a.txt"); got != "three\n" {
		t.Errorf("snapshot a.txt = %q, want the uncommitted %q", got, "three\n")
	}
}

func TestRunResumesTheRoundWhenNothingChanged(t *testing.T) {
	dir := repoWithRange(t)
	runReview(t, dir)

	m := runReview(t, dir)

	if n := loadState(t, dir).Round(); n != 1 {
		t.Errorf("round = %d after a restart without changes, want 1", n)
	}
	if s := ansi.Strip(m.View().Content); !strings.Contains(s, "Runde 1") {
		t.Errorf("status line misses round 1:\n%s", s)
	}
}

func TestRunStartsTheNextRoundAfterChanges(t *testing.T) {
	dir := repoWithRange(t)
	writeFile(t, filepath.Join(dir, "c.txt"), "c\n")
	// Both files fit the screen: they are read once shown.
	pressMain(runReview(t, dir), "]", "f", "q")
	s := loadState(t, dir)
	s.AddComment(review.Comment{Path: "a.txt", Start: 1, End: 1, Quote: "two", Category: review.Must, Text: "fix two"})
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	// The agent answers: a line above the commented one.
	writeFile(t, filepath.Join(dir, "a.txt"), "zero\ntwo\n")
	got := captureProgram(t)
	if _, err := run(dir, nil); err != nil {
		t.Fatal(err)
	}

	s = loadState(t, dir)
	if s.Round() != 2 {
		t.Fatalf("round = %d, want 2", s.Round())
	}
	if got := snapshotFile(t, dir, 2, "a.txt"); got != "zero\ntwo\n" {
		t.Errorf("snapshot of round 2 a.txt = %q", got)
	}
	c := s.Comments()[0]
	if c.Start != 2 || c.Orphaned || !s.Earlier(c) {
		t.Errorf("comment = %+v, want it relocated to line 2 and from an earlier round", c)
	}
	if s.Read("a.txt") {
		t.Error("the changed a.txt is still read")
	}
	if !s.Read("c.txt") {
		t.Error("the unchanged c.txt lost its read status")
	}

	m, _ := got().Update(tea.WindowSizeMsg{Width: 60, Height: 8})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m = pressMain(m, "i")
	screen := ansi.Strip(m.View().Content)
	if !strings.Contains(screen, "a.txt  1/1  +1 −0") || !strings.Contains(screen, "[interdiff]") {
		t.Errorf("interdiff does not show only the change since round 1:\n%s", screen)
	}
}

func TestRunKeepsTheInterdiffWhenResumingALaterRound(t *testing.T) {
	dir := repoWithRange(t)
	runReview(t, dir)
	writeFile(t, filepath.Join(dir, "a.txt"), "zero\ntwo\n")
	runReview(t, dir)

	m := runReview(t, dir)

	if n := loadState(t, dir).Round(); n != 2 {
		t.Fatalf("round = %d, want 2", n)
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	m = pressMain(m, "i")
	if s := ansi.Strip(m.View().Content); !strings.Contains(s, "[interdiff]") {
		t.Errorf("resumed round 2 has no interdiff:\n%s", s)
	}
}

func TestRunSnapshotsTheNewSideOfARange(t *testing.T) {
	dir := repoWithRange(t)

	runReview(t, dir, "main..feature")

	if got := snapshotFile(t, dir, 1, "a.txt"); got != "two\n" {
		t.Errorf("snapshot a.txt = %q, want the feature side %q", got, "two\n")
	}
}
