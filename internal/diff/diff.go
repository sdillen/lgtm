// Package diff turns an old and a new version of a file into a ReviewFile,
// the line sequence of the full-file view. It is purely functional and does
// no I/O.
package diff

import (
	"strings"

	zdiff "znkr.io/diff"
)

// Kind classifies a line of the full-file view.
type Kind int

const (
	Context Kind = iota // unchanged, present in old and new version
	Added               // only present in the new version
	Deleted             // only present in the old version
)

// Line is one line of the full-file view. Old and New are 1-based line
// numbers in the respective version, 0 if the line does not exist there.
type Line struct {
	Kind     Kind
	Old, New int
	Text     string
}

// ReviewFile is the full-file view of one changed file: the whole new file
// with deleted lines inline at their original position.
type ReviewFile struct {
	Path  string
	Lines []Line
}

// New computes the ReviewFile for path from its old and new content. An
// empty old (new) content stands for an added (deleted) file.
func New(path, old, new string) ReviewFile {
	edits := zdiff.Edits(splitLines(old), splitLines(new))
	lines := make([]Line, 0, len(edits))
	for _, e := range edits {
		switch e.Op {
		case zdiff.Match:
			lines = append(lines, Line{Kind: Context, Old: e.PosX + 1, New: e.PosY + 1, Text: e.X})
		case zdiff.Delete:
			lines = append(lines, Line{Kind: Deleted, Old: e.PosX + 1, Text: e.X})
		case zdiff.Insert:
			lines = append(lines, Line{Kind: Added, New: e.PosY + 1, Text: e.Y})
		}
	}
	return ReviewFile{Path: path, Lines: lines}
}

// splitLines splits content into lines without their terminators. A final
// line terminator does not start another line.
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}
