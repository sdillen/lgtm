// Package diff turns an old and a new version of a file into a ReviewFile,
// the line sequence of the full-file view. It is purely functional and does
// no I/O.
package diff

import (
	"fmt"
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

// Hunk is a maximal run of changed lines in ReviewFile.Lines.
type Hunk struct {
	Start, End int // Lines[start:end]
}

// Fold is a maximal run of unchanged lines in ReviewFile.Lines. It is the
// region the UI collapses behind a "N unchanged lines" marker.
type Fold struct {
	Start, End int // Lines[start:end]
}

// Count returns the number of lines the fold collapses.
func (f Fold) Count() int { return f.End - f.Start }

// ReviewFile is the full-file view of one changed file: the whole new file
// with deleted lines inline at their original position, plus the hunks and
// folds derived from it.
type ReviewFile struct {
	Path  string
	Lines []Line
	Hunks []Hunk
	Folds []Fold
}

// Section is one hunk of the classic unified diff view: a maximal run of
// changed lines expanded by context lines around it. OldStart/NewStart and
// OldCount/NewCount are the numbers of the classic hunk header.
type Section struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Start, End         int // Lines[start:end]
}

// Header renders the classic hunk header, e.g. "@@ -1,5 +1,5 @@".
func (s Section) Header() string {
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@", s.OldStart, s.OldCount, s.NewStart, s.NewCount)
}

// Stats returns the number of added and deleted lines.
func (f ReviewFile) Stats() (added, deleted int) {
	for _, l := range f.Lines {
		switch l.Kind {
		case Added:
			added++
		case Deleted:
			deleted++
		}
	}
	return added, deleted
}

// New computes the ReviewFile for path from the content before and after the
// change. An empty before (after) stands for an added (deleted) file.
func New(path, before, after string) ReviewFile {
	edits := zdiff.Edits(splitLines(before), splitLines(after))
	lines := make([]Line, 0, len(edits))
	for _, e := range edits {
		switch e.Op {
		case zdiff.Match:
			lines = append(lines, Line{Kind: Context, Old: e.PosX + 1, New: e.PosY + 1, Text: trimEOL(e.X)})
		case zdiff.Delete:
			lines = append(lines, Line{Kind: Deleted, Old: e.PosX + 1, Text: trimEOL(e.X)})
		case zdiff.Insert:
			lines = append(lines, Line{Kind: Added, New: e.PosY + 1, Text: trimEOL(e.Y)})
		}
	}
	return ReviewFile{
		Path:  path,
		Lines: lines,
		Hunks: hunks(lines),
		Folds: folds(lines),
	}
}

// hunks groups the changed lines into maximal runs.
func hunks(lines []Line) []Hunk {
	var hunks []Hunk
	start := -1
	for i, l := range lines {
		if l.Kind == Context {
			if start >= 0 {
				hunks = append(hunks, Hunk{Start: start, End: i})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		hunks = append(hunks, Hunk{Start: start, End: len(lines)})
	}
	return hunks
}

// folds groups the unchanged lines into maximal runs.
func folds(lines []Line) []Fold {
	var folds []Fold
	start := -1
	for i, l := range lines {
		if l.Kind != Context {
			if start >= 0 {
				folds = append(folds, Fold{Start: start, End: i})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		folds = append(folds, Fold{Start: start, End: len(lines)})
	}
	return folds
}

// Unified returns the hunks of the classic unified diff view: each hunk
// expanded by context lines around it, hunks whose context windows touch
// merged into one section.
func (f ReviewFile) Unified(context int) []Section {
	var sections []Section
	for _, h := range f.Hunks {
		start := max(0, h.Start-context)
		end := min(len(f.Lines), h.End+context)
		if n := len(sections); n > 0 && start <= sections[n-1].End {
			sections[n-1].End = end
			continue
		}
		sections = append(sections, Section{
			Start: start,
			End:   end,
		})
	}
	for i := range sections {
		s := &sections[i]
		s.OldStart, s.OldCount, s.NewStart, s.NewCount = header(f.Lines[s.Start:s.End])
	}
	return sections
}

// header computes the classic hunk header numbers from the lines of one
// section. The start numbers are the first old/new line numbers present,
// the counts the numbers of old/new lines in the section. A start of 0 with
// count 0 stands for a section with no lines of that side.
func header(lines []Line) (oldStart, oldCount, newStart, newCount int) {
	for _, l := range lines {
		if l.Old != 0 {
			if oldStart == 0 {
				oldStart = l.Old
			}
			oldCount++
		}
		if l.New != 0 {
			if newStart == 0 {
				newStart = l.New
			}
			newCount++
		}
	}
	return oldStart, oldCount, newStart, newCount
}

// splitLines splits content into lines that keep their terminator, so a
// last line without newline differs from the same line with one.
func splitLines(content string) []string {
	lines := strings.SplitAfter(content, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func trimEOL(line string) string {
	return strings.TrimSuffix(line, "\n")
}
