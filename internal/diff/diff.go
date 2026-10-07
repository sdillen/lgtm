// Package diff turns an old and a new version of a file into a ReviewFile,
// the line sequence of the full-file view. It is purely functional and does
// no I/O.
package diff

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	zdiff "znkr.io/diff"
)

// Kind classifies a line of the full-file view.
type Kind int

const (
	Context Kind = iota // unchanged, present in old and new version
	Added               // only present in the new version
	Deleted             // only present in the old version
)

// Word is one changed token within a changed line, for intra-line
// highlighting. Start and End are byte offsets into the line's Text.
type Word struct {
	Start, End int // Text[Start:End]
}

// Line is one line of the full-file view. Old and New are 1-based line
// numbers in the respective version, 0 if the line does not exist there.
type Line struct {
	Kind     Kind
	Old, New int
	Text     string
	// Words are the changed tokens of a paired line, as byte offsets
	// into Text. Context lines, unpaired changed lines and pairs whose
	// token diff has no common token have no Words.
	Words []Word
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

// NewContent reconstructs the new version of the file as the reader sees
// it: the lines of the full-file view that exist in the new version, in
// order, without a trailing newline. Deleted lines are skipped.
func (f ReviewFile) NewContent() string {
	var b strings.Builder
	first := true
	for _, l := range f.Lines {
		if l.New == 0 {
			continue
		}
		if !first {
			b.WriteByte('\n')
		}
		b.WriteString(l.Text)
		first = false
	}
	return b.String()
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
	hunks := hunks(lines)
	pairWords(lines, hunks)
	return ReviewFile{
		Path:  path,
		Lines: lines,
		Hunks: hunks,
		Folds: folds(lines),
	}
}

// pairWords fills the Words of the changed lines. Within each hunk, every
// added line is paired with the next unpaired deleted line before it; the
// lines of a pair get the byte ranges of their changed tokens. A pair
// whose token diff has no common token is not meaningful and gets no
// ranges; lines without a partner get none either.
func pairWords(lines []Line, hunks []Hunk) {
	for _, h := range hunks {
		var pending []int // indexes of unpaired deleted lines
		for i := h.Start; i < h.End; i++ {
			switch lines[i].Kind {
			case Deleted:
				pending = append(pending, i)
			case Added:
				if len(pending) == 0 {
					continue
				}
				d := pending[0]
				pending = pending[1:]
				del, add, meaningful := changedRanges(tokenize(lines[d].Text), tokenize(lines[i].Text))
				if !meaningful {
					continue
				}
				lines[d].Words = del
				lines[i].Words = add
			}
		}
	}
}

// changedRanges computes the byte ranges of the changed tokens of a
// paired line: the deleted tokens of the old line and the inserted tokens
// of the new line. meaningful is false when the token diff has no common
// token, i.e. the pairing does not correspond to an actual edit.
func changedRanges(old, new []token) (del, add []Word, meaningful bool) {
	edits := zdiff.EditsFunc(old, new, func(a, b token) bool { return a.text == b.text })
	oi, ni := 0, 0
	for _, e := range edits {
		switch e.Op {
		case zdiff.Match:
			meaningful = true
			oi++
			ni++
		case zdiff.Delete:
			del = append(del, Word{Start: old[oi].start, End: old[oi].end})
			oi++
		case zdiff.Insert:
			add = append(add, Word{Start: new[ni].start, End: new[ni].end})
			ni++
		}
	}
	return del, add, meaningful
}

// token is one token of a line: its text and byte span in the line.
type token struct {
	text       string
	start, end int
}

// tokenize splits a line into tokens: maximal runs of word characters
// (letters, digits, underscore) and each other non-space character
// individually. Whitespace separates tokens and is not part of any token.
func tokenize(line string) []token {
	var toks []token
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		j := i + size
		if wordRune(r) {
			for j < len(line) {
				r2, s2 := utf8.DecodeRuneInString(line[j:])
				if !wordRune(r2) {
					break
				}
				j += s2
			}
		}
		toks = append(toks, token{text: line[i:j], start: i, end: j})
		i = j
	}
	return toks
}

// wordRune reports whether r is a word character of the tokenizer.
func wordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
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
