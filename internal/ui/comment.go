package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
)

// floatWidth is the preferred inner width of the comment float.
const floatWidth = 56

// gutterWidth is the width of the line number gutter plus the change
// marker as rendered by renderLine; the float and the inline comments
// start after it.
const gutterWidth = 12

// draft is the comment being written in the float.
type draft struct {
	path       string
	start, end int // 1-based, inclusive line range in the new file
	quote      string
	category   review.Category
	text       string
	pos        int  // byte offset of the text cursor
	normal     bool // vim normal mode; the float starts in insert mode
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

// openComment opens the float for the selection or the cursor line. The
// anchor is the range of new-file lines it covers; a selection of only
// deleted lines has no anchor yet.
func (m *Model) openComment() {
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
	if start == 0 {
		m.fail("deleted lines cannot be commented yet")
		return
	}
	var quote []string
	for _, l := range f.Lines {
		if l.New >= start && l.New <= end {
			quote = append(quote, l.Text)
		}
	}
	m.draft = draft{
		path:     f.Path,
		start:    start,
		end:      end,
		quote:    strings.Join(quote, "\n"),
		category: review.DefaultCategory,
	}
	m.mode = modeComment
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

// saveDraft stores the draft as a new comment and persists the review.
// An empty draft is discarded.
func (m *Model) saveDraft() {
	m.mode = modeNormal
	m.visual = false
	text := strings.TrimSpace(m.draft.text)
	if text == "" {
		m.message = "empty comment discarded"
		return
	}
	m.state.AddComment(review.Comment{
		Path:     m.draft.path,
		Start:    m.draft.start,
		End:      m.draft.end,
		Quote:    m.draft.quote,
		Category: m.draft.category,
		Text:     text,
	})
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

// commentBox renders the float: category and location in the title, the
// text with the cursor and the key hints below.
func (m Model) commentBox() string {
	d := m.draft
	inner := max(1, min(floatWidth, m.width-boxChromeCols-gutterWidth))
	loc := fmt.Sprintf("%s:%d", d.path, d.start)
	if d.end != d.start {
		loc += fmt.Sprintf("-%d", d.end)
	}
	title := m.categoryStyle(d.category).Bold(true).Render(string(d.category)) + "  " + loc

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

// noteRows are the virtual rows of the comments ending on the new-file
// line, attached to the file line index.
func (m Model) noteRows(newLine, line int) []row {
	if newLine == 0 {
		return nil
	}
	// The text wraps after the gutter and the "▎ " bar.
	width := max(1, m.contentWidth()-gutterWidth-2)
	var rows []row
	for _, c := range m.fileComments {
		if c.End != newLine {
			continue
		}
		text := string(c.Category) + ": " + c.Text
		for _, l := range strings.Split(ansi.Wrap(text, width, ""), "\n") {
			rows = append(rows, row{line: line, fold: -1, note: &note{category: c.Category, text: l}})
		}
	}
	return rows
}

// note is one line of a comment shown as a virtual row under the code.
type note struct {
	category review.Category
	text     string
}

// renderNote renders a comment row in the colour of its category,
// indented past the gutter.
func (m Model) renderNote(n *note, cursor bool) string {
	style := m.categoryStyle(n.category)
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

// loadFileComments caches the comments of the current file for the
// virtual rows.
func (m *Model) loadFileComments() {
	m.fileComments = nil
	if len(m.files) == 0 {
		return
	}
	for _, c := range m.state.Comments() {
		if c.Path == m.files[m.fileIdx].Path {
			m.fileComments = append(m.fileComments, c)
		}
	}
}
