package review

import (
	"strings"

	zdiff "znkr.io/diff"
)

// File is one file of the review at the time of a (re)load: its path,
// the path it was renamed from ("" if it was not) and the content of the
// old and the new side. Relocate finds the comment anchors in it.
type File struct {
	Path    string
	OldPath string
	Old     string
	New     string
}

// Relocate moves the line anchors of all comments from the review files
// prev, where they were made, to the reloaded files next: each comment
// is found again by its quote, near the place its lines moved to, also
// when only the indentation of the quoted code changed. Comments follow
// their file through renames; a line comment whose file is gone is
// also searched in the files that newly appeared in the review. A
// comment whose code is gone is marked orphaned and keeps its last
// anchor; it is never dropped. An orphaned comment whose code shows up
// again is anchored anew. Relocate returns the number of orphaned
// comments.
func (s *State) Relocate(prev, next []File) int {
	before := byPath(prev)
	orphans := 0
	for i := range s.CommentList {
		c := &s.CommentList[i]
		if c.Path == "" {
			continue
		}
		c.Orphaned = !relocate(c, before, next)
		if c.Orphaned {
			orphans++
		}
	}
	return orphans
}

// relocate finds the anchor of c, made in the files prev, in the files
// next and moves c there. It reports whether the anchor was found.
func relocate(c *Comment, prev map[string]File, next []File) bool {
	fileComment := c.Start == 0 && c.End == 0
	f, ok := target(c.Path, next)
	if fileComment {
		if ok {
			c.Path = f.Path
		}
		return ok
	}
	p := prev[c.Path]
	candidates := []File{f}
	if !ok {
		// The file may have been renamed without a rename source,
		// e.g. a file new in the review: search the quote in the
		// files that newly appeared and look like it.
		candidates = nil
		for _, n := range next {
			if _, known := prev[n.Path]; !known && similar(side(p, c.Old), side(n, c.Old)) {
				candidates = append(candidates, n)
			}
		}
	}
	for _, f := range candidates {
		before, after := side(p, c.Old), side(f, c.Old)
		start, ok := Locate(before, after, c.Quote, c.Start)
		if !ok {
			continue
		}
		n := c.End - c.Start + 1
		c.Path = f.Path
		c.Start, c.End = start, start+n-1
		c.Quote = strings.Join(splitContent(after)[start-1:start-1+n], "\n")
		return true
	}
	return false
}

// side returns the old or the new content of f.
func side(f File, old bool) string {
	if old {
		return f.Old
	}
	return f.New
}

// target returns the file of next that path became: the file with that
// path, or the file renamed from it.
func target(path string, next []File) (File, bool) {
	for _, f := range next {
		if f.Path == path {
			return f, true
		}
	}
	for _, f := range next {
		if f.OldPath == path {
			return f, true
		}
	}
	return File{}, false
}

// Locate finds quote in after, the content that replaced before, and
// returns the 1-based line it starts on now. line is where the quote
// started in before; among several places with the quote, the one
// nearest to where that line moved wins. A place made of lines that
// were other code in before (e.g. another closing brace) does not
// count: the quoted code itself must have stayed or moved there. When
// the quote is not found verbatim, lines that differ from it only in
// their indentation count as a match. ok is false when after no longer
// contains the quote; the line is then the best guess, where the old
// line moved to by the line diff, so a cursor can stay put.
func Locate(before, after, quote string, line int) (int, bool) {
	old, lines := splitContent(before), splitContent(after)
	want := strings.Split(quote, "\n")
	edits := zdiff.Edits(old, lines)
	expected := moveLine(edits, len(old), line)
	origin := origins(edits)
	// own reports whether the lines from i on are not other code that
	// was already in before.
	own := func(i int) bool {
		for k := range want {
			x, ok := origin[i+k]
			if ok && (x < line-1 || x >= line-1+len(want)) {
				return false
			}
		}
		return true
	}
	for _, eq := range []func(a, b string) bool{exact, sameButIndent} {
		best := -1
		for i := 0; i+len(want) <= len(lines); i++ {
			if equalLines(lines[i:i+len(want)], want, eq) && own(i) &&
				(best < 0 || abs(i+1-expected) < abs(best+1-expected)) {
				best = i
			}
		}
		if best >= 0 {
			return best + 1, true
		}
	}
	return max(1, min(expected, len(lines))), false
}

// exact reports whether two lines are equal.
func exact(a, b string) bool { return a == b }

// sameButIndent reports whether two lines differ at most in their
// leading and trailing whitespace.
func sameButIndent(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}

// moveLine maps the 1-based line of before, which has n lines, to the
// line of after it moved to, following the line diff: an unchanged line
// maps to its new place, a removed one to the place where the removal
// happened. A line beyond before keeps its number.
func moveLine(edits []zdiff.Edit[string], n, line int) int {
	if line < 1 || line > n {
		return line
	}
	y := 0 // the after line the walk has reached, 0-based
	for _, e := range edits {
		switch e.Op {
		case zdiff.Match:
			if e.PosX+1 == line {
				return e.PosY + 1
			}
			y = e.PosY + 1
		case zdiff.Delete:
			if e.PosX+1 == line {
				return y + 1
			}
		case zdiff.Insert:
			y = e.PosY + 1
		}
	}
	return line
}

// origins maps every unchanged line of after to the line of before it
// was, both 0-based.
func origins(edits []zdiff.Edit[string]) map[int]int {
	m := map[int]int{}
	for _, e := range edits {
		if e.Op == zdiff.Match {
			m[e.PosY] = e.PosX
		}
	}
	return m
}

// similar reports whether at least half of the lines of the longer of
// before and after are shared, the threshold of the rename detection:
// after looks like a renamed version of before.
func similar(before, after string) bool {
	old, lines := splitContent(before), splitContent(after)
	longest := max(len(old), len(lines))
	return longest > 0 && 2*len(origins(zdiff.Edits(old, lines))) >= longest
}

// byPath indexes files by path.
func byPath(files []File) map[string]File {
	m := make(map[string]File, len(files))
	for _, f := range files {
		m[f.Path] = f
	}
	return m
}

// splitContent splits file content into its lines, the way the review
// shows them: without terminators and without an empty line after a
// final newline.
func splitContent(content string) []string {
	if content == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(content, "\n"), "\n")
}

// equalLines reports whether a and b hold the same lines under eq.
func equalLines(a, b []string, eq func(a, b string) bool) bool {
	for i := range a {
		if !eq(a[i], b[i]) {
			return false
		}
	}
	return true
}

// abs returns the absolute value of n.
func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
