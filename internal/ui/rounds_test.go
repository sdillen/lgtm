package ui_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/ui"
)

// The second round of a review: the agent changed a.go after round 1
// (b → B), b.go is as it was. full is the review against the base,
// inter the interdiff since the snapshot of round 1.
var (
	roundFull = []diff.ReviewFile{
		diff.New("a.go", "x\n", "x\nb\nB\n"),
		diff.New("b.go", "", "unchanged\n"),
	}
	roundInter = []diff.ReviewFile{
		diff.New("a.go", "x\nb\n", "x\nb\nB\n"),
	}
)

// secondRound returns options of a review in round 2 with the interdiff
// above, persisted into dir.
func secondRound(dir string) ui.Options {
	opts := withState(dir)
	opts.State.StartRound(opts.State.SnapshotRef(1), time.Unix(100, 0))
	opts.State.StartRound(opts.State.SnapshotRef(2), time.Unix(200, 0))
	opts.Round = 2
	opts.Interdiff = func() ([]diff.ReviewFile, error) { return roundInter, nil }
	return opts
}

// interdiffKey presses <leader>i.
func interdiffKey(m tea.Model) tea.Model {
	return press(leader(m), "i")
}

func TestStatusLineShowsTheRound(t *testing.T) {
	m := start(ui.New(roundFull, "main", themeFor, secondRound(t.TempDir())), 60, 10)

	if s := screen(m); !strings.Contains(s, "Runde 2") {
		t.Errorf("status line misses the round:\n%s", s)
	}
}

func TestLeaderITogglesBetweenFullDiffAndInterdiff(t *testing.T) {
	m := start(ui.New(roundFull, "main", themeFor, secondRound(t.TempDir())), 60, 3)

	m = interdiffKey(m)
	assertScreen(t, m, []string{
		"⋯ 2 unveränderte Zeilen ⋯",
		"        3 + B",
		"a.go  1/1  +1 −0  Runde 2  [interdiff]",
	})

	m = interdiffKey(m)
	s := screen(m)
	if !strings.Contains(s, "a.go  1/2  +2 −0") || strings.Contains(s, "[interdiff]") {
		t.Errorf("second <leader>i does not return to the full diff:\n%s", s)
	}
}

func TestInterdiffKeepsTheCurrentFile(t *testing.T) {
	inter := []diff.ReviewFile{roundInter[0], diff.New("b.go", "", "unchanged\n")}
	opts := secondRound(t.TempDir())
	opts.Interdiff = func() ([]diff.ReviewFile, error) { return inter, nil }
	m := start(ui.New(roundFull, "main", themeFor, opts), 60, 10)
	m = press(m, "]", "f")

	m = interdiffKey(m)

	if s := screen(m); !strings.Contains(s, "b.go  2/2") {
		t.Errorf("interdiff does not stay on b.go:\n%s", s)
	}
}

func TestInterdiffNeedsAnEarlierRound(t *testing.T) {
	m := start(ui.New(roundFull, "main", themeFor, withState(t.TempDir())), 60, 10)

	m = interdiffKey(m)

	if s := screen(m); !strings.Contains(s, "kein Interdiff") || strings.Contains(s, "[interdiff]") {
		t.Errorf("<leader>i in the first round gives no hint:\n%s", s)
	}
}

func TestDrawerMarksFilesChangedSinceTheLastRound(t *testing.T) {
	m := start(ui.New(roundFull, "main", themeFor, secondRound(t.TempDir())), 60, 10)

	m = press(leader(m), "d")

	if row := ansiRow(m, "a.go"); !strings.Contains(row, "Δ") {
		t.Errorf("drawer does not mark the changed a.go: %q", row)
	}
	if row := ansiRow(m, "b.go"); strings.Contains(row, "Δ") {
		t.Errorf("drawer marks the unchanged b.go: %q", row)
	}
}

// ansiRow returns the stripped screen row containing s.
func ansiRow(m tea.Model, s string) string {
	for _, row := range strings.Split(screen(m), "\n") {
		if strings.Contains(row, s) {
			return row
		}
	}
	return ""
}

func TestCommentsOfEarlierRoundsAreDimmedAndCanBeTickedOff(t *testing.T) {
	dir := t.TempDir()
	opts := withState(dir)
	opts.State.StartRound(opts.State.SnapshotRef(1), time.Unix(100, 0))
	opts.State.AddComment(review.Comment{Path: "a.go", Start: 2, End: 2, Quote: "b", Category: review.Must, Text: "fix b"})
	opts.State.StartRound(opts.State.SnapshotRef(2), time.Unix(200, 0))
	opts.Round = 2
	m := start(ui.New(roundFull, "main", themeFor, opts), 60, 10)

	row := rowContaining(m, "fix b")
	if !strings.Contains(row, "Runde 1") {
		t.Errorf("comment of round 1 does not name its round: %q", row)
	}
	// The dim fold colour #51597d replaces the must colour.
	if !strings.Contains(row, "38;2;81;89;125") {
		t.Errorf("comment of round 1 is not dimmed: %q", row)
	}

	press(m, "j", "j", "x")

	got := readState(t, dir).Comments()
	if len(got) != 1 || got[0].Status != review.Done {
		t.Fatalf("comments = %+v, want the round 1 comment done", got)
	}
	md, err := os.ReadFile(filepath.Join(dir, ".lgtm", "review.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(md), "fix b") {
		t.Errorf("review.md still exports the ticked-off comment:\n%s", md)
	}
}

func TestXOnADoneCommentReopensIt(t *testing.T) {
	dir := t.TempDir()
	m := start(ui.New([]diff.ReviewFile{file}, "main", themeFor, withState(dir)), 60, 12)
	m = press(m, "j", "j", "c")
	m = typeText(m, "fix this")
	m = pressMsg(m, ctrl("s"))

	press(m, "j", "x", "x")

	got := readState(t, dir).Comments()
	if len(got) != 1 || got[0].Status != review.Open {
		t.Fatalf("comment status = %+v, want open again", got)
	}
}

func TestInterdiffShowsCommentsOnNewLinesOnly(t *testing.T) {
	opts := secondRound(t.TempDir())
	opts.State.AddComment(review.Comment{Path: "a.go", Start: 2, End: 2, Quote: "b", Text: "new side"})
	m := start(ui.New(roundFull, "main", themeFor, opts), 60, 10)

	m = interdiffKey(m)

	if s := screen(m); !strings.Contains(s, "new side") {
		t.Errorf("interdiff hides the comment on a new line:\n%s", s)
	}
}

func TestInterdiffDoesNotCommentDeletedLines(t *testing.T) {
	opts := secondRound(t.TempDir())
	opts.Interdiff = func() ([]diff.ReviewFile, error) {
		return []diff.ReviewFile{diff.New("a.go", "x\ngone\n", "x\nb\nB\n")}, nil
	}
	m := start(ui.New(roundFull, "main", themeFor, opts), 60, 10)
	m = interdiffKey(m)

	m = press(m, "j", "c")

	if s := screen(m); !strings.Contains(s, "Gesamt-Diff") {
		t.Errorf("c on a line deleted since the last round gives no hint:\n%s", s)
	}
}

func TestReloadTakesANewSnapshotAndRefreshesTheInterdiff(t *testing.T) {
	opts := secondRound(t.TempDir())
	snapshots := 0
	opts.Snapshot = func() error { snapshots++; return nil }
	next := diff.New("a.go", "x\nb\n", "x\nb\nB\nC\n")
	opts.Interdiff = func() ([]diff.ReviewFile, error) {
		if snapshots > 0 {
			return []diff.ReviewFile{next}, nil
		}
		return roundInter, nil
	}
	opts.Reload = func() ([]diff.ReviewFile, error) {
		return []diff.ReviewFile{diff.New("a.go", "x\n", "x\nb\nB\nC\n"), roundFull[1]}, nil
	}
	m := start(ui.New(roundFull, "main", themeFor, opts), 60, 10)
	m = interdiffKey(m)

	m = press(m, "R")

	if snapshots != 1 {
		t.Errorf("R took %d snapshots, want 1", snapshots)
	}
	// The whole review would show b as added, too.
	if s := screen(m); !strings.Contains(s, "+ C") || strings.Contains(s, "+ b") {
		t.Errorf("R does not show the new interdiff:\n%s", s)
	}
}

func TestTheReviewResumesWhereItWasLeft(t *testing.T) {
	dir := t.TempDir()
	opts := withState(dir)
	m := start(ui.New([]diff.ReviewFile{file, big}, "main", themeFor, opts), 60, 6)
	m = press(m, "]", "f", "]", "c", "]", "c")
	before := screen(m)
	m.Update(key("q"))

	again := withState(dir)
	again.State = readState(t, dir)
	m = start(ui.New([]diff.ReviewFile{file, big}, "main", themeFor, again), 60, 6)

	if s := screen(m); s != before {
		t.Errorf("restart does not resume where the review was left:\n%s\nwant:\n%s", s, before)
	}
}

func TestTheReviewResumesInTheInterdiff(t *testing.T) {
	dir := t.TempDir()
	opts := secondRound(dir)
	m := start(ui.New(roundFull, "main", themeFor, opts), 60, 3)
	m = interdiffKey(m)
	before := screen(m)
	m.Update(key("q"))

	again := secondRound(dir)
	again.State = readState(t, dir)
	m = start(ui.New(roundFull, "main", themeFor, again), 60, 3)

	if s := screen(m); s != before {
		t.Errorf("restart does not resume in the interdiff:\n%s\nwant:\n%s", s, before)
	}
}

func TestInterdiffToggleKeepsExpandedFiles(t *testing.T) {
	generated := diff.NewWithAttrs("gen.pb.go", "", "a\nb\n", nil)
	full := []diff.ReviewFile{generated}
	opts := secondRound(t.TempDir())
	opts.Interdiff = func() ([]diff.ReviewFile, error) { return full, nil }
	m := start(ui.New(full, "main", themeFor, opts), 60, 10)
	m = press(m, "z", "o")

	m = interdiffKey(m)

	if s := screen(m); strings.Contains(s, "generierte Datei") {
		t.Errorf("toggling collapses the expanded file again:\n%s", s)
	}
}
