package review

import (
	"fmt"
	"time"
)

// Round is one review round: the snapshot ref of the reviewed state and
// when the round started.
type Round struct {
	Snapshot string    `json:"snapshot"`
	Time     time.Time `json:"time"`
}

// Round returns the number of the current round, 1-based; 0 before the
// first round started.
func (s *State) Round() int {
	return len(s.Rounds)
}

// Snapshot returns the snapshot ref of round n, "" if there is no such
// round.
func (s *State) Snapshot(n int) string {
	if n < 1 || n > len(s.Rounds) {
		return ""
	}
	return s.Rounds[n-1].Snapshot
}

// SnapshotRef names the ref the snapshot of round n is kept under:
// refs/lgtm/<review id>/<n>. The id tells the reviews of linked
// worktrees apart, which share their refs.
func (s *State) SnapshotRef(n int) string {
	if s.ID == "" {
		s.ID = newID()
	}
	return fmt.Sprintf("refs/lgtm/%s/%d", s.ID, n)
}

// StartRound starts the next round with the snapshot ref of its
// reviewed state. Comments from before the first round join it.
func (s *State) StartRound(snapshot string, at time.Time) {
	s.Rounds = append(s.Rounds, Round{Snapshot: snapshot, Time: at})
	for i := range s.CommentList {
		if s.CommentList[i].Round == 0 {
			s.CommentList[i].Round = 1
		}
	}
}

// Earlier reports whether c was made in a round before the current one.
func (s *State) Earlier(c Comment) bool {
	return c.Round > 0 && c.Round < s.Round()
}

// ResetChanged forgets the read status of path when the content that
// was read differs from content, and reports whether it did.
func (s *State) ResetChanged(path, content string) bool {
	if !s.Read(path) || s.HashOf(path) == contentHash(content) {
		return false
	}
	s.SetUnread(path)
	return true
}

// Position is where the reviewer left the review, so it can resume
// there: the view (whole review or interdiff), the file, the line under
// the cursor (numbered in the old content of that view when Old is
// set, for a deleted line; 0 for none) and the screen row the cursor
// was on.
type Position struct {
	Interdiff bool   `json:"interdiff,omitempty"`
	Path      string `json:"path"`
	Line      int    `json:"line,omitempty"`
	Old       bool   `json:"old,omitempty"`
	Row       int    `json:"row,omitempty"`
}

// Position returns where the review was left; the zero value if it
// was never left.
func (s *State) Position() Position {
	if s.Pos == nil {
		return Position{}
	}
	return *s.Pos
}

// SetPosition remembers where the review is left.
func (s *State) SetPosition(p Position) {
	s.Pos = &p
}
