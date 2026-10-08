package ui

import (
	"strings"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
)

// highlightBuffer is how many rows beyond the visible area are
// highlighted ahead of the render, so scrolling into view never waits
// for the lexer. One key press moves at most a few rows, so a generous
// margin keeps the lexer ahead of the cursor.
const highlightBuffer = 100

// fileHighlights are the syntax highlights of one file, computed only
// for the lines the view needs. The added and context lines come from
// one continuous token stream over the new-file content, so multi-line
// tokens (block comments, raw strings) stay intact; deleted lines are
// dimmed and lexed per line. Everything is cached, so scrolling never
// lexes a line twice.
type fileHighlights struct {
	h    *theme.Highlighter
	path string

	// content is the new-file text, built from file on first use; the
	// stream lexes it lazily, line by line. file is nil for plain
	// texts (the context view), which pass content directly.
	file    *diff.ReviewFile
	content string
	stream  *theme.Stream
	lines   []theme.Line // highlighted new-file lines, line n at n-1
	newDone int          // new-file lines lexed so far

	dim map[int]theme.Line // dimmed deleted lines by file-line index

	// rendered caches finished diff lines: scrolling by one row changes
	// two lines (cursor in, cursor out), not the whole window. It lives
	// and dies with the highlights, which are rebuilt whenever the file
	// or the theme changes.
	rendered map[lineKey]renderedLine
}

// maxRendered bounds the line cache; it is cleared when full, which
// only happens when the horizontal scroll sweeps over a huge file.
const maxRendered = 8192

// lineKey is everything a rendered diff line depends on besides the
// file and the theme: the line, the cursor state, the horizontal window
// and the identity of the highlighted segments, which changes when the
// lazy highlighter has caught up with the line, and of the word diff,
// which changes with every new diff of the file.
type lineKey struct {
	idx        int
	cursor     bool
	mark       int
	skip, need int
	seg0       *theme.Segment
	word0      *diff.Word
}

// renderedLine is a cached line; text and kind guard against a file
// whose lines changed under the same highlights.
type renderedLine struct {
	text string
	kind diff.Kind
	out  string
}

func (h *fileHighlights) cachedLine(k lineKey, l diff.Line) (string, bool) {
	e, ok := h.rendered[k]
	if !ok || e.kind != l.Kind || e.text != l.Text {
		return "", false
	}
	return e.out, true
}

func (h *fileHighlights) storeLine(k lineKey, l diff.Line, out string) {
	if h.rendered == nil || len(h.rendered) >= maxRendered {
		h.rendered = make(map[lineKey]renderedLine)
	}
	h.rendered[k] = renderedLine{text: l.Text, kind: l.Kind, out: out}
}

// newFileHighlights returns the lazy highlights of a review file.
func newFileHighlights(h *theme.Highlighter, f diff.ReviewFile) *fileHighlights {
	return &fileHighlights{h: h, path: f.Path, file: &f, dim: map[int]theme.Line{}}
}

// newTextHighlights returns the lazy highlights of a plain text.
func newTextHighlights(h *theme.Highlighter, path, content string) *fileHighlights {
	return &fileHighlights{h: h, path: path, content: content, dim: map[int]theme.Line{}}
}

// ensureRows highlights the file lines of the rows in [from, to): the
// visible range plus the prefetch buffer. Rows without a file line
// (fold markers, headers, notes, meta entries) are skipped.
func (c *fileHighlights) ensureRows(rows []row, from, to int) {
	if c == nil || c.file == nil {
		return
	}
	if from < 0 {
		from = 0
	}
	if to > len(rows) {
		to = len(rows)
	}
	for i := from; i < to; i++ {
		r := rows[i]
		if r.note != nil || r.meta != "" || r.collapsed || r.header != "" || r.fold >= 0 || r.line < 0 {
			continue
		}
		c.ensureLine(r.line)
	}
}

// ensureLine highlights the file line at idx: its new-file line from
// the token stream, its deleted line dimmed.
func (c *fileHighlights) ensureLine(idx int) {
	if c.file == nil || idx >= len(c.file.Lines) {
		return
	}
	switch l := c.file.Lines[idx]; l.Kind {
	case diff.Added, diff.Context:
		if l.New > 0 {
			c.ensureNew(l.New)
		}
	case diff.Deleted:
		c.ensureDim(idx)
	}
}

// ensureNew lexes new-file lines up to n (1-based), continuing the
// token stream from where it stopped.
func (c *fileHighlights) ensureNew(n int) {
	if n <= c.newDone {
		return
	}
	if c.stream == nil {
		c.stream = c.h.Stream(c.path, c.newContent())
	}
	for c.newDone < n {
		line, ok := c.stream.Next()
		c.lines = append(c.lines, line)
		c.newDone++
		if !ok {
			break
		}
	}
}

// newContent is the new-file text of the review file, built once.
func (c *fileHighlights) newContent() string {
	if c.file == nil || c.content != "" {
		return c.content
	}
	var sb strings.Builder
	for _, l := range c.file.Lines {
		if l.New != 0 {
			sb.WriteString(l.Text)
			sb.WriteByte('\n')
		}
	}
	c.content = sb.String()
	return c.content
}

// newLine returns the highlighted segments of new-file line n (1-based).
// Lines the lexer produced no segments for come back empty, like lines
// beyond the end of the new-file content.
func (c *fileHighlights) newLine(n int) theme.Line {
	if c == nil {
		return nil
	}
	c.ensureNew(n)
	if n-1 < len(c.lines) {
		return c.lines[n-1]
	}
	return nil
}

// ensureDim lexes the dimmed highlight of the deleted file line idx.
func (c *fileHighlights) ensureDim(idx int) {
	if c == nil || c.file == nil {
		return
	}
	if _, ok := c.dim[idx]; ok {
		return
	}
	l := c.file.Lines[idx]
	d := c.h.HighlightDim(c.path, l.Text)
	if len(d) > 0 {
		c.dim[idx] = d[0]
	} else {
		c.dim[idx] = nil
	}
}

// dimLine returns the dimmed segments of the deleted file line idx.
func (c *fileHighlights) dimLine(idx int) theme.Line {
	if c == nil {
		return nil
	}
	c.ensureDim(idx)
	return c.dim[idx]
}
