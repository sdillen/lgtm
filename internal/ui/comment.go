package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sdillen/lgtm/internal/diff"
	"github.com/sdillen/lgtm/internal/review"
)

// floatWidth is the preferred inner width of the comment float.
const floatWidth = 56

// gutterWidth is the width of the line number gutter plus the change
// marker as rendered by renderLine; the float and the inline comments
// start after it.
const gutterWidth = 12

// draft is the comment being written in the float. An empty path marks
// a comment on the whole review, empty start/end a comment on the whole
// file, and old an anchor in the old file content (deleted lines).
// editID is the id of the comment being edited, "" for a new comment.
type draft struct {
	path       string
	start, end int // 1-based, inclusive line range; 0 for file/global
	old        bool
	quote      string
	category   review.Category
	text       string
	pos        int  // byte offset of the text cursor
	normal     bool // vim normal mode; the float starts in insert mode
	editID     string
}

// toggleVisual starts or ends the line selection at the cursor line. A
// selection starts on a code line only.
func (m *Model) toggleVisual() {
	if !m.visual && !m.onCodeLine() {
		m.fail("move the cursor to a code line to select")
		return
	}
	m.visual = !m.visual
	m.visualLine = m.cursorLine()
}

// onCodeLine reports whether the cursor is on a line of the file, not on
// a fold marker, a section header or a comment row.
func (m Model) onCodeLine() bool {
	r := m.rows()[m.cursor]
	return r.fold < 0 && r.header == "" && r.note == nil
}

// selection returns the selected range of line indexes, or the cursor
// line when nothing is selected.
func (m Model) selection() (lo, hi int) {
	lo, hi = m.cursorLine(), m.cursorLine()
	if m.visual {
		lo, hi = min(lo, m.visualLine), max(hi, m.visualLine)
	}
	return lo, hi
}

// selected reports whether the line index is inside the visual
// selection.
func (m Model) selected(line int) bool {
	if !m.visual || line < 0 {
		return false
	}
	lo, hi := m.selection()
	return lo <= line && line <= hi
}

// openComment opens the float for the selection or the cursor line. On
// a comment row it edits the comment instead. The anchor is the range
// of lines it covers: new-file lines by default, the old lines when the
// selection covers only deleted lines. The float slides in from the
// bottom edge.
func (m *Model) openComment() {
	if r := m.rows()[m.cursor]; r.note != nil && !m.visual {
		m.openEdit(r.note.comment)
		return
	}
	if !m.visual && !m.onCodeLine() {
		m.fail("move the cursor to a code line to comment")
		return
	}
	f := m.files[m.fileIdx]
	lo, hi := m.selection()
	start, end := 0, 0
	for _, l := range f.Lines[lo : hi+1] {
		if l.New == 0 {
			continue
		}
		if start == 0 {
			start = l.New
		}
		end = l.New
	}
	old := false
	if start == 0 {
		if m.interdiff {
			// The deleted lines of the interdiff number the
			// snapshot, not the old state the anchor refers to.
			m.fail("gelöschte Zeilen im Gesamt-Diff kommentieren (<leader>i)")
			return
		}
		// The selection has no line of the new file: it anchors in
		// the old state, on the deleted lines.
		old = true
		for _, l := range f.Lines[lo : hi+1] {
			if l.Old == 0 {
				continue
			}
			if start == 0 {
				start = l.Old
			}
			end = l.Old
		}
	}
	var quote []string
	for _, l := range f.Lines {
		if old && l.Old >= start && l.Old <= end {
			quote = append(quote, l.Text)
		} else if !old && l.New >= start && l.New <= end {
			quote = append(quote, l.Text)
		}
	}
	m.draft = draft{
		path:     f.Path,
		start:    start,
		end:      end,
		old:      old,
		quote:    strings.Join(quote, "\n"),
		category: review.DefaultCategory,
	}
	m.mode = modeComment
	m.startSlideOverlay(m.bodyHeight())
}

// openCommentFile opens the float for a comment on the whole current
// file.
func (m *Model) openCommentFile() {
	m.draft = draft{path: m.files[m.fileIdx].Path, category: review.DefaultCategory}
	m.mode = modeComment
	m.startSlideOverlay(m.bodyHeight())
}

// openCommentGlobal opens the float for a comment on the whole review.
func (m *Model) openCommentGlobal() {
	m.draft = draft{category: review.DefaultCategory}
	m.mode = modeComment
	m.startSlideOverlay(m.bodyHeight())
}

// openEdit opens the float with the text and category of the existing
// comment; saving replaces them.
func (m *Model) openEdit(c review.Comment) {
	m.draft = draft{
		path:     c.Path,
		start:    c.Start,
		end:      c.End,
		old:      c.Old,
		quote:    c.Quote,
		category: c.Category,
		text:     c.Text,
		pos:      len(c.Text),
		editID:   c.ID,
	}
	m.mode = modeComment
	m.startSlideOverlay(m.bodyHeight())
}

// commentKey edits the draft with minimal vim bindings. It starts in
// insert mode; <Esc> goes to normal mode, a second <Esc> discards the
// draft. <Tab> cycles the category and <C-s> saves, in either mode.
func (m Model) commentKey(msg tea.KeyPressMsg) Model {
	d := &m.draft
	switch msg.String() {
	case "ctrl+s":
		m.saveDraft()
		return m
	case "tab":
		d.category = d.category.Next()
		return m
	case "left":
		d.left()
		return m
	case "right":
		d.right()
		return m
	}
	if !d.normal {
		switch msg.String() {
		case "esc":
			d.normal = true
		case "enter":
			d.insert("\n")
		case "backspace":
			if d.pos > 0 {
				_, size := utf8.DecodeLastRuneInString(d.text[:d.pos])
				d.text = d.text[:d.pos-size] + d.text[d.pos:]
				d.pos -= size
			}
		default:
			d.insert(msg.Text)
		}
		return m
	}
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
		m.message = "comment discarded"
	case "i":
		d.normal = false
	case "a":
		d.right()
		d.normal = false
	case "I":
		d.pos = d.lineStart()
		d.normal = false
	case "A":
		d.pos = d.lineEnd()
		d.normal = false
	case "h":
		d.left()
	case "l":
		d.right()
	case "0":
		d.pos = d.lineStart()
	case "$":
		d.pos = d.lineEnd()
	case "x":
		if d.pos < len(d.text) && d.text[d.pos] != '\n' {
			_, size := utf8.DecodeRuneInString(d.text[d.pos:])
			d.text = d.text[:d.pos] + d.text[d.pos+size:]
		}
	}
	return m
}

// saveDraft stores the draft: a new comment, or the replacement of the
// edited comment, and persists the review. An empty draft is discarded.
func (m *Model) saveDraft() {
	m.mode = modeNormal
	m.visual = false
	text := strings.TrimSpace(m.draft.text)
	if text == "" {
		m.message = "empty comment discarded"
		return
	}
	if m.draft.editID != "" {
		m.state.UpdateComment(m.draft.editID, text, m.draft.category)
		m.message = "comment updated"
	} else {
		m.state.AddComment(review.Comment{
			Path:     m.draft.path,
			Start:    m.draft.start,
			End:      m.draft.end,
			Old:      m.draft.old,
			Quote:    m.draft.quote,
			Category: m.draft.category,
			Text:     text,
		})
	}
	m.loadFileComments()
	m.saveState()
}

// insert inserts s at the text cursor.
func (d *draft) insert(s string) {
	d.text = d.text[:d.pos] + s + d.text[d.pos:]
	d.pos += len(s)
}

// left moves the text cursor one rune left, within its line.
func (d *draft) left() {
	if d.pos > d.lineStart() {
		_, size := utf8.DecodeLastRuneInString(d.text[:d.pos])
		d.pos -= size
	}
}

// right moves the text cursor one rune right, within its line.
func (d *draft) right() {
	if d.pos < d.lineEnd() {
		_, size := utf8.DecodeRuneInString(d.text[d.pos:])
		d.pos += size
	}
}

// lineStart is the offset of the first byte of the cursor's text line.
func (d *draft) lineStart() int {
	return strings.LastIndexByte(d.text[:d.pos], '\n') + 1
}

// lineEnd is the offset of the newline ending the cursor's text line, or
// the text length on the last line.
func (d *draft) lineEnd() int {
	if i := strings.IndexByte(d.text[d.pos:], '\n'); i >= 0 {
		return d.pos + i
	}
	return len(d.text)
}

// categoryStyle is the theme colour of a comment category.
func (m Model) categoryStyle(c review.Category) lipgloss.Style {
	st := m.theme.Styles
	switch c {
	case review.Must:
		return st.CommentMust
	case review.Nit:
		return st.CommentNit
	case review.Question:
		return st.CommentQuestion
	}
	return st.CommentShould
}

// location names the anchor of the draft for the float title.
func (d draft) location() string {
	return review.Comment{Path: d.path, Start: d.start, End: d.end, Old: d.old}.Location()
}

// commentBox renders the float: category and location in the title, the
// text with the cursor and the key hints below.
func (m Model) commentBox() string {
	d := m.draft
	inner := max(1, min(floatWidth, m.width-boxChromeCols-gutterWidth))
	title := m.categoryStyle(d.category).Bold(true).Render(string(d.category)) + "  " + d.location()

	cursor := lipgloss.NewStyle().Reverse(true)
	var lines []string
	offset := 0
	for _, line := range strings.Split(d.text, "\n") {
		text := line
		if d.pos >= offset && d.pos <= offset+len(line) {
			at := d.pos - offset
			under := " "
			rest := line[at:]
			if rest != "" {
				_, size := utf8.DecodeRuneInString(rest)
				under, rest = rest[:size], rest[size:]
			}
			text = line[:at] + cursor.Render(under) + rest
		}
		offset += len(line) + 1
		lines = append(lines, strings.Split(ansi.Wrap(text, inner, ""), "\n")...)
	}
	mode := "INSERT"
	if d.normal {
		mode = "NORMAL"
	}
	lines = append(lines, "", m.theme.Styles.Fold.Render(mode+"  <Tab> category  <C-s> save  <Esc><Esc> discard"))
	for i, l := range lines {
		if w := ansi.StringWidth(l); w < inner {
			lines[i] = l + strings.Repeat(" ", inner-w)
		}
	}
	return m.box(title, lines)
}

// commentBoxPosition places the float below the cursor row, or above it
// when it does not fit below.
func (m Model) commentBoxPosition(box string) (x, y int) {
	w, h := lipgloss.Size(box)
	row := m.cursor - m.offset
	y = row + 1
	if y+h > m.bodyHeight() {
		y = max(0, row-h)
	}
	return max(0, min(gutterWidth, m.contentWidth()-w)), y
}

// noteRows are the virtual rows of the comments anchored to the line at
// index line of the file fi: new-side comments below the new line they
// end on, deleted-line comments below the old line they end on, and
// file and global comments below the first line of their file (global
// comments only in the first file).
func (m Model) noteRows(fi, line int) []row {
	f := m.files[fi]
	l := f.Lines[line]
	var rows []row
	if l.New != 0 {
		rows = append(rows, m.lineNotes(l.New, false, line)...)
	}
	// The deleted lines of the interdiff number the snapshot, not
	// the old state the deleted-line comments are anchored in.
	if l.Old != 0 && !m.interdiff {
		rows = append(rows, m.lineNotes(l.Old, true, line)...)
	}
	if line == 0 {
		rows = append(rows, m.fileNotes()...)
	}
	return rows
}

// lineNotes are the note rows of the line comments whose anchor ends on
// the given line of the given side. The comments are the cached ones of
// the current file.
func (m Model) lineNotes(line int, old bool, attach int) []row {
	var rows []row
	for _, c := range m.fileComments {
		if !c.Orphaned && c.Old == old && c.End == line {
			rows = append(rows, m.noteRowLines(c, attach)...)
		}
	}
	return rows
}

// fileNotes are the note rows of the file comments and the orphaned
// comments of the current file and, in the first file, of the global
// comments. They attach to the first line of the file.
func (m Model) fileNotes() []row {
	var rows []row
	path := m.files[m.fileIdx].Path
	for _, c := range m.fileComments {
		if c.Path == path && (c.Orphaned || c.Start == 0 && c.End == 0) {
			rows = append(rows, m.noteRowLines(c, 0)...)
		}
	}
	if m.fileIdx == 0 {
		for _, c := range m.globalComments {
			rows = append(rows, m.noteRowLines(c, 0)...)
		}
	}
	return rows
}

// noteRowLines are the wrapped text rows of one comment, attached to the
// file line index line.
func (m Model) noteRowLines(c review.Comment, line int) []row {
	// The text wraps after the gutter and the "▎ " bar.
	width := max(1, m.contentWidth()-gutterWidth-2)
	text := string(c.Category) + ": " + c.Text
	if m.state.Earlier(c) {
		text = fmt.Sprintf("Runde %d · ", c.Round) + text
	}
	if c.Status == review.Done {
		text = "✓ " + text
	}
	if c.Orphaned {
		// The code is gone; the last known anchor tells where the
		// comment was.
		text = c.Location() + " – " + text
	}
	var rows []row
	for _, l := range strings.Split(ansi.Wrap(text, width, ""), "\n") {
		rows = append(rows, row{line: line, fold: -1, note: &note{category: c.Category, text: l, comment: c}})
	}
	return rows
}

// note is one line of a comment shown as a virtual row under the code.
// comment is the whole comment the line belongs to, for yc.
type note struct {
	category review.Category
	text     string
	comment  review.Comment
}

// renderNote renders a comment row in the colour of its category,
// indented past the gutter. Done comments are dimmed and struck
// through; open comments of earlier rounds keep their colour.
func (m Model) renderNote(n *note, cursor bool) string {
	style := m.categoryStyle(n.category)
	if n.comment.Status == review.Done {
		style = m.theme.Styles.Fold.Strikethrough(true)
	}
	pad := lipgloss.NewStyle()
	if cursor {
		style = style.Background(m.theme.Styles.Cursor)
		pad = pad.Background(m.theme.Styles.Cursor)
	}
	return pad.Render(strings.Repeat(" ", gutterWidth)) + style.Render("▎ "+n.text)
}

// commentCount is the number of comments in the review.
func (m Model) commentCount() string {
	n := len(m.state.Comments())
	if n == 1 {
		return "1 comment"
	}
	return fmt.Sprintf("%d comments", n)
}

// loadFileComments caches the comments of the current file and the
// global comments for the virtual rows. Orphaned comments of files that
// left the review count as global ones, so they stay visible.
func (m *Model) loadFileComments() {
	m.invalidateRows()
	m.fileComments = nil
	m.globalComments = nil
	if len(m.files) == 0 {
		return
	}
	path := m.files[m.fileIdx].Path
	for _, c := range m.state.Comments() {
		if c.Path == path {
			m.fileComments = append(m.fileComments, c)
		}
		if c.Path == "" || c.Orphaned && m.fileIdxOf(c.Path) < 0 {
			m.globalComments = append(m.globalComments, c)
		}
	}
}

// deleteComment deletes the comment under the cursor after confirmation:
// the first dd only asks, other keys disarm the confirmation.
func (m *Model) deleteComment() {
	c, ok := m.commentAtCursor()
	if !ok {
		m.deleteArmed = false
		m.fail("move the cursor to a comment to delete")
		return
	}
	if !m.deleteArmed {
		m.deleteArmed = true
		m.message = "delete this comment? press dd again to confirm"
		return
	}
	m.deleteArmed = false
	m.state.DeleteComment(c.ID)
	m.loadFileComments()
	m.saveState()
	m.message = "comment deleted"
}

// markDone marks the comment under the cursor as done, or a done one
// as open again; done comments stay visible but are no longer exported.
func (m *Model) markDone() {
	c, ok := m.commentAtCursor()
	if !ok {
		m.fail("move the cursor to a comment to mark as done")
		return
	}
	status, msg := review.Done, "comment marked as done"
	if c.Status == review.Done {
		status, msg = review.Open, "comment open again"
	}
	m.state.SetCommentStatus(c.ID, status)
	m.loadFileComments()
	m.saveState()
	m.message = msg
}

// commentPos is the position of a comment in the review: the file index
// and the file line its note row is attached to.
type commentPos struct {
	c    review.Comment
	file int
	line int
}

// commentPositions lists every comment with a place in the current
// review, in file order: global comments at the top of the first file,
// file comments at the top of their file, line comments below the line
// they end on. Orphaned comments sit at the top of their file, or of the
// first file when their file left the review. Comments whose file or
// line is gone otherwise are left out.
func (m Model) commentPositions() []commentPos {
	var pos []commentPos
	for _, c := range m.state.Comments() {
		fi := 0
		if c.Path != "" {
			fi = m.fileIdxOf(c.Path)
		}
		if fi < 0 && c.Orphaned && len(m.files) > 0 {
			fi = 0
		}
		if fi < 0 {
			continue // the file is no longer part of the review
		}
		if c.Old && m.interdiff {
			continue // deleted-line comments are not shown in the interdiff
		}
		line := 0
		if !c.Orphaned && (c.Start != 0 || c.End != 0) {
			line = m.lineIndex(m.files[fi], c.End, c.Old)
			if line < 0 {
				continue // the anchored line is no longer in the file
			}
		}
		pos = append(pos, commentPos{c: c, file: fi, line: line})
	}
	slices.SortStableFunc(pos, func(a, b commentPos) int {
		if c := cmp.Compare(a.file, b.file); c != 0 {
			return c
		}
		return cmp.Compare(a.line, b.line)
	})
	return pos
}

// fileIdxOf returns the index of the file with the path, -1 if the
// review does not contain it.
func (m Model) fileIdxOf(path string) int {
	for i, f := range m.files {
		if f.Path == path {
			return i
		}
	}
	return -1
}

// lineIndex returns the index of the line with the given number on the
// given side (old for deleted lines), -1 if there is none.
func (m Model) lineIndex(f diff.ReviewFile, n int, old bool) int {
	for i, l := range f.Lines {
		if (old && l.Old == n) || (!old && l.New == n) {
			return i
		}
	}
	return -1
}

// jumpToComment moves the cursor to the next (dir 1) or previous
// (dir -1) comment, across files, and lands on its first note row.
func (m *Model) jumpToComment(dir int) {
	pos := m.commentPositions()
	if len(pos) == 0 {
		return
	}
	cur := commentPos{file: m.fileIdx, line: m.cursorLine()}
	hit := -1
	if dir > 0 {
		for i, p := range pos {
			if p.file > cur.file || p.file == cur.file && p.line > cur.line {
				hit = i
				break
			}
		}
	} else {
		for i, p := range pos {
			if p.file < cur.file || p.file == cur.file && p.line < cur.line {
				hit = i // keep the last match for backward jumps
			}
		}
	}
	if hit < 0 {
		return
	}
	p := pos[hit]
	if p.file != m.fileIdx {
		m.fileIdx = p.file
		m.resetFile()
	}
	if row := m.rowOfComment(p.c); row >= 0 {
		m.cursor = row
		m.scrollCursorIntoView()
	}
	m.visual = false
}

// rowOfComment returns the row of the first note line of the comment,
// -1 if the comment has no note row in the current view.
func (m Model) rowOfComment(c review.Comment) int {
	for i, r := range m.rows() {
		if r.note != nil && r.note.comment.ID == c.ID {
			return i
		}
	}
	return -1
}
