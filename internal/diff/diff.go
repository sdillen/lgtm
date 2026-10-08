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
// folds derived from it. Kind classifies how the file is presented; the
// kind-specific data (sizes, modes, hashes) sits in the dedicated fields.
// old and new keep the contents the view was computed from, so the UI can
// recompute it with whitespace-only changes hidden.
type ReviewFile struct {
	Path    string
	OldPath string // rename source, "" when the file was not renamed
	Kind    FileKind
	Lines   []Line
	Hunks   []Hunk
	Folds   []Fold

	// OldSize and NewSize are the sizes of a binary file, in bytes.
	OldSize, NewSize int64
	// OldMode and NewMode are the file modes of a mode-only change.
	OldMode, NewMode uint32
	// OldSub and NewSub are the commit hashes of a submodule change.
	OldSub, NewSub string

	old, new string
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

// Contents returns the old and the new content the view was computed
// from, verbatim; empty for an added (deleted) file and for mode and
// submodule entries.
func (f ReviewFile) Contents() (old, new string) {
	return f.old, f.new
}

// SameChange reports whether f and g show the same change: the same
// paths, contents, modes and submodule commits. View options such as
// hidden whitespace changes do not count.
func (f ReviewFile) SameChange(g ReviewFile) bool {
	return f.Path == g.Path && f.OldPath == g.OldPath && f.Kind == g.Kind &&
		f.old == g.old && f.new == g.new &&
		f.OldMode == g.OldMode && f.NewMode == g.NewMode &&
		f.OldSub == g.OldSub && f.NewSub == g.NewSub
}

// New computes the ReviewFile for path from the content before and after the
// change. An empty before (after) stands for an added (deleted) file. The
// change is classified: binary files become size-only info entries, generated
// files keep their diff but are marked for the UI.
func New(path, before, after string) ReviewFile {
	return NewWithAttrs(path, before, after, nil)
}

// NewWithAttrs is New with the .gitattributes check: generated reports
// whether .gitattributes marks path as linguist-generated, nil skips it.
func NewWithAttrs(path, before, after string, generated func(string) bool) ReviewFile {
	kind := Classify(path, before, after, generated)
	if kind == KindBinary {
		return ReviewFile{
			Path:    path,
			Kind:    KindBinary,
			OldSize: int64(len(before)),
			NewSize: int64(len(after)),
			old:     before,
			new:     after,
		}
	}
	f := newView(path, before, after, false)
	f.Kind = kind
	f.old, f.new = before, after
	return f
}

// newView computes the line view of a text change. ignoreWS hides
// whitespace-only changes: the diff is computed over lines that are equal
// ignoring all whitespace, and the matched lines show the new text, so the
// reader sees the new file.
func newView(path, before, after string, ignoreWS bool) ReviewFile {
	x, y := splitLines(before), splitLines(after)
	var edits []zdiff.Edit[string]
	if ignoreWS {
		// The lines are compared without whitespace; stripping each
		// line once is far cheaper than stripping both sides of every
		// comparison. The texts come from x and y by position.
		edits = zdiff.Edits(stripAll(x), stripAll(y))
	} else {
		edits = zdiff.Edits(x, y)
	}
	lines := make([]Line, 0, len(edits))
	for _, e := range edits {
		switch e.Op {
		case zdiff.Match:
			lines = append(lines, Line{Kind: Context, Old: e.PosX + 1, New: e.PosY + 1, Text: trimEOL(y[e.PosY])})
		case zdiff.Delete:
			lines = append(lines, Line{Kind: Deleted, Old: e.PosX + 1, Text: trimEOL(x[e.PosX])})
		case zdiff.Insert:
			lines = append(lines, Line{Kind: Added, New: e.PosY + 1, Text: trimEOL(y[e.PosY])})
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

// WithIgnoreWS returns the view of the same change with whitespace-only
// changes hidden when ignore is true, and with them visible when it is
// false. The classification and the rename path are kept. It is what the
// UI shows when :set ignorews changes. Binary, mode and submodule entries
// have no lines to hide and are returned unchanged.
func (f ReviewFile) WithIgnoreWS(ignore bool) ReviewFile {
	if f.Kind != KindDiff && f.Kind != KindGenerated {
		return f
	}
	g := newView(f.Path, f.old, f.new, ignore)
	g.Kind = f.Kind
	g.OldPath = f.OldPath
	g.old, g.new = f.old, f.new
	return g
}

// Deleted reports whether the view shows the whole old version as
// deleted lines: the file is gone.
func (f ReviewFile) Deleted() bool {
	for _, l := range f.Lines {
		if l.New != 0 {
			return false
		}
	}
	return len(f.Lines) > 0
}

// Added reports whether the view shows the whole new version as added
// lines: the file is new. New files carry no background wash, only the
// gutter markers.
func (f ReviewFile) Added() bool {
	for _, l := range f.Lines {
		if l.Old != 0 {
			return false
		}
	}
	return len(f.Lines) > 0
}

// Collapsed reports whether the UI shows the file collapsed by default:
// deleted files, whose whole content is one dimmed block, and generated
// files, whose noise the reviewer can unfold on demand.
func (f ReviewFile) Collapsed() bool {
	return f.Kind == KindGenerated || f.Deleted()
}

// stripAll returns the lines with all whitespace removed: the keys under
// which the whitespace-ignoring diff compares them.
func stripAll(lines []string) []string {
	stripped := make([]string, len(lines))
	for i, l := range lines {
		stripped[i] = stripWS(l)
	}
	return stripped
}

// stripWS removes every whitespace rune from s.
func stripWS(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
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
				del, add, meaningful := changedRanges(lines[d].Text, lines[i].Text)
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
func changedRanges(oldLine, newLine string) (del, add []Word, meaningful bool) {
	old, new := tokenize(oldLine), tokenize(newLine)
	edits := zdiff.Edits(tokenTexts(oldLine, old), tokenTexts(newLine, new))
	oi, ni := 0, 0
	for _, e := range edits {
		switch e.Op {
		case zdiff.Match:
			meaningful = true
			oi++
			ni++
		case zdiff.Delete:
			del = append(del, old[oi])
			oi++
		case zdiff.Insert:
			add = append(add, new[ni])
			ni++
		}
	}
	return del, add, meaningful
}

// tokenize splits a line into tokens: maximal runs of word characters
// (letters, digits, underscore) and each other non-space character
// individually. Whitespace separates tokens and is not part of any token.
// The tokens are byte spans into the line.
func tokenize(line string) []Word {
	// Code averages a token per two to four bytes; one guess spares
	// most of the regrowth on long lines.
	toks := make([]Word, 0, len(line)/3+1)
	for i := 0; i < len(line); {
		r, size := runeAt(line, i)
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		j := i + size
		if wordRune(r) {
			for j < len(line) {
				r2, s2 := runeAt(line, j)
				if !wordRune(r2) {
					break
				}
				j += s2
			}
		}
		toks = append(toks, Word{Start: i, End: j})
		i = j
	}
	return toks
}

// runeAt decodes the rune at line[i], skipping the decoder for ASCII.
func runeAt(line string, i int) (rune, int) {
	if c := line[i]; c < utf8.RuneSelf {
		return rune(c), 1
	}
	return utf8.DecodeRuneInString(line[i:])
}

// tokenTexts are the texts of the tokens of line, the sequence the token
// diff compares.
func tokenTexts(line string, toks []Word) []string {
	texts := make([]string, len(toks))
	for i, t := range toks {
		texts[i] = line[t.Start:t.End]
	}
	return texts
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
