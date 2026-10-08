package review_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
)

func TestAFreshStateHasNoRound(t *testing.T) {
	s := review.NewState()
	if got := s.Round(); got != 0 {
		t.Errorf("Round() = %d, want 0", got)
	}
	if got := s.Snapshot(1); got != "" {
		t.Errorf("Snapshot(1) = %q, want none", got)
	}
}

func TestRoundsCountUpAndRememberTheirSnapshot(t *testing.T) {
	s := review.NewState()
	first := s.SnapshotRef(1)
	s.StartRound(first, time.Unix(100, 0))
	second := s.SnapshotRef(2)
	s.StartRound(second, time.Unix(200, 0))

	if got := s.Round(); got != 2 {
		t.Errorf("Round() = %d, want 2", got)
	}
	if got := s.Snapshot(1); got != first {
		t.Errorf("Snapshot(1) = %q, want %q", got, first)
	}
	if got := s.Snapshot(2); got != second {
		t.Errorf("Snapshot(2) = %q, want %q", got, second)
	}
	if got := s.Snapshot(3); got != "" {
		t.Errorf("Snapshot(3) = %q, want none", got)
	}
}

func TestSnapshotRefsLiveUnderRefsLgtmAndDifferPerReview(t *testing.T) {
	a, b := review.NewState(), review.NewState()

	ref := a.SnapshotRef(1)

	if !strings.HasPrefix(ref, "refs/lgtm/") {
		t.Errorf("SnapshotRef(1) = %q, want a ref under refs/lgtm/", ref)
	}
	if ref == a.SnapshotRef(2) {
		t.Errorf("rounds 1 and 2 share the ref %q", ref)
	}
	if ref == b.SnapshotRef(1) {
		t.Errorf("two reviews share the ref %q", ref)
	}
}

func TestRoundsSurviveSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	ref := s.SnapshotRef(1)
	s.StartRound(ref, time.Unix(100, 0))
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	got, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got.Round() != 1 || got.Snapshot(1) != ref {
		t.Errorf("loaded round %d with snapshot %q, want 1 with %q", got.Round(), got.Snapshot(1), ref)
	}
	if got.SnapshotRef(2) != s.SnapshotRef(2) {
		t.Errorf("loaded state names round 2 %q, want %q", got.SnapshotRef(2), s.SnapshotRef(2))
	}
}

func TestCommentsBelongToTheRoundTheyWereMadeIn(t *testing.T) {
	s := review.NewState()
	s.StartRound(s.SnapshotRef(1), time.Unix(100, 0))
	old := s.AddComment(review.Comment{Path: "a.go", Start: 1, End: 1, Text: "old"})
	s.StartRound(s.SnapshotRef(2), time.Unix(200, 0))
	cur := s.AddComment(review.Comment{Path: "a.go", Start: 2, End: 2, Text: "new"})

	if old.Round != 1 || cur.Round != 2 {
		t.Errorf("rounds = %d, %d, want 1, 2", old.Round, cur.Round)
	}
	if !s.Earlier(old) {
		t.Error("the comment of round 1 is not from an earlier round")
	}
	if s.Earlier(cur) {
		t.Error("the comment of round 2 is from an earlier round")
	}
}

func TestCommentsFromBeforeTheFirstRoundJoinIt(t *testing.T) {
	s := review.NewState()
	c := s.AddComment(review.Comment{Path: "a.go", Text: "before rounds"})

	s.StartRound(s.SnapshotRef(1), time.Unix(100, 0))

	got := s.Comments()[0]
	if got.ID != c.ID || got.Round != 1 || s.Earlier(got) {
		t.Errorf("comment = %+v, want it in the current round 1", got)
	}
}

func TestResetChangedForgetsTheReadStatusOfChangedContent(t *testing.T) {
	s := review.NewState()
	s.SetRead("same.go", "same\n")
	s.SetRead("changed.go", "before\n")

	if s.ResetChanged("same.go", "same\n") {
		t.Error("ResetChanged reset an unchanged file")
	}
	if !s.ResetChanged("changed.go", "after\n") {
		t.Error("ResetChanged kept a changed file")
	}
	if s.ResetChanged("unread.go", "x\n") {
		t.Error("ResetChanged reported a file that was never read")
	}

	if !s.Read("same.go") {
		t.Error("same.go lost its read status")
	}
	if s.Read("changed.go") {
		t.Error("changed.go is still read")
	}
}

func TestThePositionSurvivesSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	s := review.NewState()
	want := review.Position{Path: "a.go", Line: 12, Old: true, Row: 3}
	s.SetPosition(want)
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}

	got, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got.Position() != want {
		t.Errorf("Position() = %+v, want %+v", got.Position(), want)
	}
}

func TestAStateOfVersion4LoadsWithoutRounds(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".lgtm"), 0o755); err != nil {
		t.Fatal(err)
	}
	v4 := `{"version":4,"files":{"a.go":{"read":true,"hash":"x"}},"comments":[{"id":"c1","path":"a.go","start":1,"end":1,"quote":"a","category":"must","status":"open","text":"t"}]}`
	if err := os.WriteFile(filepath.Join(dir, ".lgtm", "state.json"), []byte(v4), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := review.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if s.Round() != 0 || !s.Read("a.go") || len(s.Comments()) != 1 {
		t.Fatalf("v4 state loaded as round %d, read %v, %d comments", s.Round(), s.Read("a.go"), len(s.Comments()))
	}
	if ref := s.SnapshotRef(1); !strings.HasPrefix(ref, "refs/lgtm/") || strings.HasPrefix(ref, "refs/lgtm//") {
		t.Errorf("v4 state names its snapshot %q", ref)
	}
	s.StartRound(s.SnapshotRef(1), time.Unix(100, 0))
	if c := s.Comments()[0]; c.Round != 1 || s.Earlier(c) {
		t.Errorf("v4 comment = %+v, want it in round 1", c)
	}
}
