package review

import (
	"crypto/rand"
	"encoding/hex"
	"slices"
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

// Comment is a reviewer's note anchored to a line range of a file in the
// new state of the review.
type Comment struct {
	// ID is stable for the life of the comment.
	ID string `json:"id"`
	// Path is the file the comment belongs to.
	Path string `json:"path"`
	// Start and End are the 1-based, inclusive line range in the new
	// file content.
	Start int `json:"start"`
	End   int `json:"end"`
	// Quote is the commented code, so the anchor can be found again.
	Quote    string   `json:"quote"`
	Category Category `json:"category"`
	Status   Status   `json:"status"`
	Text     string   `json:"text"`
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

// newID returns a random comment id.
func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
