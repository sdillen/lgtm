package review

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strings"
)

// Category is the priority a reviewer gives a comment.
type Category string

// The comment categories.
const (
	Must     Category = "must"
	Should   Category = "should"
	Nit      Category = "nit"
	Question Category = "question"
)

// DefaultCategory is the category of a new comment.
const DefaultCategory = Should

// categoryCycle is the order Next walks through, starting at the default.
var categoryCycle = []Category{Should, Must, Nit, Question}

// Next returns the category after c in the cycle should → must → nit →
// question → should. An unknown category starts the cycle over.
func (c Category) Next() Category {
	i := slices.Index(categoryCycle, c)
	return categoryCycle[(i+1)%len(categoryCycle)]
}

// Status is the life cycle state of a comment.
type Status string

// Open comments are still to be addressed by the agent.
const Open Status = "open"

// Done comments were ticked off by the reviewer; they stay visible in
// the review but are not exported.
const Done Status = "done"

// Comment is a reviewer's note. It is anchored to a line range of a file
// in the new state of the review, to a line range of the old state
// (deleted lines, Old is true), to a whole file (no line range) or to
// the whole review (no path).
type Comment struct {
	// ID is stable for the life of the comment.
	ID string `json:"id"`
	// Path is the file the comment belongs to; "" for a comment on
	// the whole review.
	Path string `json:"path"`
	// Start and End are the 1-based, inclusive line range; 0 for file
	// and global comments. For Old comments they number the old file
	// content, otherwise the new one.
	Start int `json:"start"`
	End   int `json:"end"`
	// Old reports that the anchor lives in the old file content: the
	// commented lines were deleted.
	Old bool `json:"old,omitempty"`
	// Quote is the commented code, so the anchor can be found again.
	Quote    string   `json:"quote"`
	Category Category `json:"category"`
	Status   Status   `json:"status"`
	Text     string   `json:"text"`
	// Orphaned reports that the quoted code was not found again when
	// the review was reloaded: the anchor is the last known one. The
	// comment keeps its status, so an open orphan is still exported.
	Orphaned bool `json:"orphaned,omitempty"`
}

// AddComment stores c as a new open comment with a fresh id and returns
// the stored comment.
func (s *State) AddComment(c Comment) Comment {
	c.ID = newID()
	c.Status = Open
	s.CommentList = append(s.CommentList, c)
	return c
}

// Comments returns all comments in the order they were added.
func (s *State) Comments() []Comment {
	return append([]Comment(nil), s.CommentList...)
}

// OpenComments returns the comments that are still open, in the order
// they were added.
func (s *State) OpenComments() []Comment {
	var open []Comment
	for _, c := range s.CommentList {
		if c.Status == Open {
			open = append(open, c)
		}
	}
	return open
}

// UpdateComment replaces the editable fields of the comment with id: the
// text and the category. The anchor, quote and status are unchanged.
// ok is false when there is no comment with id.
func (s *State) UpdateComment(id, text string, category Category) (Comment, bool) {
	for i := range s.CommentList {
		if s.CommentList[i].ID == id {
			s.CommentList[i].Text = text
			s.CommentList[i].Category = category
			return s.CommentList[i], true
		}
	}
	return Comment{}, false
}

// DeleteComment removes the comment with id and reports whether it
// existed.
func (s *State) DeleteComment(id string) bool {
	for i, c := range s.CommentList {
		if c.ID == id {
			s.CommentList = append(s.CommentList[:i], s.CommentList[i+1:]...)
			return true
		}
	}
	return false
}

// SetCommentStatus sets the status of the comment with id and returns
// the updated comment; ok is false when there is no comment with id.
func (s *State) SetCommentStatus(id string, status Status) (Comment, bool) {
	for i := range s.CommentList {
		if s.CommentList[i].ID == id {
			s.CommentList[i].Status = status
			return s.CommentList[i], true
		}
	}
	return Comment{}, false
}

// Location names the anchor of the comment for review.md, the prompt
// and the float title: "path:start-end" on new lines, "path:line (alt,
// entfernt)" on deleted lines, "path (Datei)" for a whole file and
// "global" for the whole review. An orphaned comment is marked
// "verwaist", e.g. "path:start-end (verwaist)": its code was not found
// again and the lines are the last known ones.
func (c Comment) Location() string {
	if c.Path == "" {
		return "global"
	}
	var notes []string
	loc := c.Path
	switch {
	case c.Start == 0 && c.End == 0:
		notes = append(notes, "Datei")
	case c.Old:
		loc += ":" + lineRange(c.Start, c.End)
		notes = append(notes, "alt, entfernt")
	default:
		loc += ":" + lineRange(c.Start, c.End)
	}
	if c.Orphaned {
		notes = append(notes, "verwaist")
	}
	if len(notes) == 0 {
		return loc
	}
	return loc + " (" + strings.Join(notes, ", ") + ")"
}

// newID returns a random comment id.
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
