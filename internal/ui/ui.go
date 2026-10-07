// Package ui renders ReviewFile models in the terminal. It knows nothing
// about git or diffing: the file view is a full-file view with folded
// unchanged runs, a toggleable classic unified view, and vim-like
// navigation. All fold and hunk regions come from the ReviewFile model.
package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
)

var (
	addedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	deletedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	gutterStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
	foldStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	hunkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	cursorBG     = lipgloss.Color("#2f334d")
)

// contextWindow is how many unchanged lines surround each hunk in the
// classic unified diff view.
const contextWindow = 3

// Model is the review view: one file at a time, with folds for the
// unchanged runs of the full-file view and a toggleable unified view.
// label names the review subject and appears in the hint when there is
// nothing to review.
type Model struct {
	files     []diff.ReviewFile
	label     string
	width     int
	height    int
	fileIdx   int
	cursor    int    // index of the cursor row in rows()
	offset    int    // index of the first visible row
	foldsOpen []bool // per fold of the current file: open?
	unified   bool   // show the classic unified diff instead of the full file
	pending   string // prefix key ("g", "z", "]", "[") waiting for its second key
	count     int    // pending count for j, k, ctrl-d, ctrl-u
}

// New returns a Model showing files of the review subject label.
func New(files []diff.ReviewFile, label string) Model {
	m := Model{files: files, label: label}
	m.resetFile()
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clamp()
	case tea.KeyPressMsg:
		key := msg.String()
		if key == "q" || key == "ctrl+c" {
			return m, tea.Quit
		}
		m = m.handleKey(msg)
	}
	return m, nil
}

// handleKey applies one key press. Prefix keys ("g", "z", "]", "[") wait
// for their second key before acting. A count before a motion repeats it
// ("5j", "3]c") or selects a line ("5G", "5zz"), like in vim.
func (m Model) handleKey(msg tea.KeyPressMsg) Model {
	if len(m.files) == 0 {
		return m
	}
	key := msg.String()
	if m.pending != "" {
		combo := m.pending + key
		m.pending = ""
		n := m.count
		m.count = 0
		switch combo {
		case "gg":
			if n > 0 {
				m.cursor = m.rowOfLine(min(n-1, len(m.files[m.fileIdx].Lines)-1))
			} else {
				m.cursor, m.offset = 0, 0
			}
		case "zz":
			if n > 0 {
				m.cursor = m.rowOfLine(min(n-1, len(m.files[m.fileIdx].Lines)-1))
			}
			m.offset = m.cursor - m.bodyHeight()/2
		case "zt":
			if n > 0 {
				m.cursor = m.rowOfLine(min(n-1, len(m.files[m.fileIdx].Lines)-1))
			}
			m.offset = m.cursor
		case "zo":
			if fi := m.foldAtCursor(); fi >= 0 && !m.foldsOpen[fi] {
				m.foldsOpen[fi] = true
				m.cursor = m.rowOfLine(m.files[m.fileIdx].Folds[fi].Start)
			}
		case "zc":
			if fi := m.foldAtCursor(); fi >= 0 && m.foldsOpen[fi] {
				m.foldsOpen[fi] = false
				m.cursor = m.rowOfFold(fi)
			}
		case "zR":
			old := m.cursorLine()
			for i := range m.foldsOpen {
				m.foldsOpen[i] = true
			}
			m.cursor = m.rowOfLine(old)
		case "zM":
			old := m.cursorLine()
			for i := range m.foldsOpen {
				m.foldsOpen[i] = false
			}
			m.cursor = m.rowOfLine(old)
		case "]c", "[c":
			dir := 1
			if combo == "[c" {
				dir = -1
			}
			for i := 0; i < max(1, n); i++ {
				m.jumpToHunk(dir)
			}
		case "]f", "[f":
			dir := 1
			if combo == "[f" {
				dir = -1
			}
			for i := 0; i < max(1, n); i++ {
				if !m.jumpFile(dir) {
					break
				}
			}
		}
		m.scrollCursorIntoView()
		return m
	}
	switch key {
	case "j", "down":
		m.cursor = min(m.cursor+m.motion(), max(0, len(m.rows())-1))
		m.scrollCursorIntoView()
	case "k", "up":
		m.cursor = max(m.cursor-m.motion(), 0)
		m.scrollCursorIntoView()
	case "ctrl+d":
		half := max(1, m.bodyHeight()/2)
		n := m.motion()
		rows := m.rows()
		m.offset = min(m.offset+n*half, max(0, len(rows)-m.bodyHeight()))
		m.cursor = min(m.cursor+n*half, max(0, len(rows)-1))
	case "ctrl+u":
		half := max(1, m.bodyHeight()/2)
		n := m.motion()
		m.offset = max(m.offset-n*half, 0)
		m.cursor = max(m.cursor-n*half, 0)
	case "G":
		if m.count > 0 {
			m.cursor = m.rowOfLine(min(m.count-1, len(m.files[m.fileIdx].Lines)-1))
			m.count = 0
		} else {
			m.cursor = max(0, len(m.rows())-1)
			m.offset = max(0, len(m.rows())-m.bodyHeight())
		}
	case "u":
		// u toggles the unified view. It deliberately is not vim's undo:
		// lgtm only reads, and the classic diff view is one key away.
		m.unified = !m.unified
		m.cursor, m.offset = 0, 0
	case "g", "z", "]", "[":
		m.pending = key
		return m
	default:
		if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
			m.count = m.count*10 + int(key[0]-'0')
			return m
		}
		m.count = 0
	}
	m.scrollCursorIntoView()
	return m
}

// motion returns the pending count or 1 and resets it.
func (m *Model) motion() int {
	n := m.count
	m.count = 0
	if n == 0 {
		return 1
	}
	return n
}

// jumpToHunk moves the cursor to the next (dir 1) or previous (dir -1)
// hunk: the first hunk after the cursor line, or the last hunk before it,
// so a cursor inside a hunk lands on that hunk's start. In the full-file
// view hunks are the changed lines of the file, in the unified view the
// section headers.
func (m *Model) jumpToHunk(dir int) {
	if m.unified {
		line := m.cursorLine()
		hit := -1
		for _, s := range m.files[m.fileIdx].Unified(contextWindow) {
			if dir > 0 && s.Start > line {
				hit = s.Start
				break
			}
			if dir < 0 && s.Start < line {
				hit = s.Start // keep the last match for backward jumps
			}
		}
		if hit >= 0 {
			m.cursor = m.rowOfHeader(hit)
		}
		return
	}
	line := m.cursorLine()
	hit := -1
	for _, h := range m.files[m.fileIdx].Hunks {
		if dir > 0 && h.Start > line {
			hit = h.Start
			break
		}
		if dir < 0 && h.Start < line {
			hit = h.Start // keep the last match for backward jumps
		}
	}
	if hit >= 0 {
		m.cursor = m.rowOfLine(hit)
	}
}

// jumpFile moves to the next (dir 1) or previous (dir -1) file and reports
// whether the cursor moved.
func (m *Model) jumpFile(dir int) bool {
	next := m.fileIdx + dir
	if next < 0 || next >= len(m.files) {
		return false
	}
	m.fileIdx = next
	m.resetFile()
	return true
}

// rowOfHeader returns the row of the section header starting at the line,
// -1 if there is none.
func (m Model) rowOfHeader(line int) int {
	for i, r := range m.rows() {
		if r.header != "" && r.line == line {
			return i
		}
	}
	return -1
}

// resetFile resets the per-file view state for the current file.
func (m *Model) resetFile() {
	m.cursor, m.offset = 0, 0
	m.pending, m.count = "", 0
	if len(m.files) == 0 {
		m.foldsOpen = nil
		return
	}
	m.foldsOpen = make([]bool, len(m.files[m.fileIdx].Folds))
}

// clamp keeps cursor and offset inside the current rows.
func (m *Model) clamp() {
	if len(m.files) == 0 {
		m.cursor, m.offset = 0, 0
		return
	}
	rows := m.rows()
	m.cursor = min(m.cursor, max(0, len(rows)-1))
	m.offset = max(0, min(m.offset, max(0, len(rows)-m.bodyHeight())))
}

// scrollCursorIntoView scrolls the minimum amount so the cursor row is
// visible.
func (m *Model) scrollCursorIntoView() {
	rows := m.rows()
	m.offset = max(0, min(m.offset, max(0, len(rows)-m.bodyHeight())))
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.bodyHeight() {
		m.offset = m.cursor - m.bodyHeight() + 1
	}
	m.clamp()
}

// row is one row of the view: a line of the file, a fold marker or a
// section header.
type row struct {
	line   int    // index into the file's Lines; header rows carry the section start
	fold   int    // fold index of a marker row, -1 otherwise
	header string // rendered section header of a header row, "" otherwise
}

// rows are the rows of the current view mode for the current file.
func (m Model) rows() []row {
	f := m.files[m.fileIdx]
	if m.unified {
		return m.unifiedRows(f)
	}
	return m.fullRows(f)
}

// fullRows are the lines of the full-file view; each closed fold collapses
// to one marker row.
func (m Model) fullRows(f diff.ReviewFile) []row {
	var rows []row
	i, fi := 0, 0
	for i < len(f.Lines) {
		if fi < len(f.Folds) && i == f.Folds[fi].Start {
			if m.foldsOpen[fi] {
				for ; i < f.Folds[fi].End; i++ {
					rows = append(rows, row{line: i, fold: -1})
				}
			} else {
				rows = append(rows, row{line: -1, fold: fi})
				i = f.Folds[fi].End
			}
			fi++
		} else {
			rows = append(rows, row{line: i, fold: -1})
			i++
		}
	}
	return rows
}

// unifiedRows are the hunks of the classic unified diff view: section
// headers plus the hunks expanded by the context window.
func (m Model) unifiedRows(f diff.ReviewFile) []row {
	var rows []row
	for _, s := range f.Unified(contextWindow) {
		rows = append(rows, row{line: s.Start, fold: -1, header: s.Header()})
		for i := s.Start; i < s.End; i++ {
			rows = append(rows, row{line: i, fold: -1})
		}
	}
	return rows
}

// cursorLine returns the file line the cursor is on: the fold's first line
// for marker rows, the section's first line for header rows.
func (m Model) cursorLine() int {
	r := m.rows()[m.cursor]
	if r.fold >= 0 {
		return m.files[m.fileIdx].Folds[r.fold].Start
	}
	return r.line
}

// foldAtCursor returns the fold the cursor points at: the fold of a marker
// row or the open fold whose lines the cursor is on. -1 if there is none.
func (m Model) foldAtCursor() int {
	r := m.rows()[m.cursor]
	if r.fold >= 0 {
		return r.fold
	}
	if r.line < 0 {
		return -1
	}
	f := m.files[m.fileIdx]
	for fi, fold := range f.Folds {
		if m.foldsOpen[fi] && fold.Start <= r.line && r.line < fold.End {
			return fi
		}
	}
	return -1
}

// rowOfLine returns the row showing the file line: its line row, or the
// marker row of the closed fold containing it. In the unified view lines
// outside the sections are not shown; the cursor snaps to the next visible
// line instead.
func (m Model) rowOfLine(line int) int {
	rows := m.rows()
	for i, r := range rows {
		if r.line == line {
			return i
		}
	}
	if fi := m.foldOfLine(line); fi >= 0 {
		if i := m.rowOfFold(fi); i >= 0 {
			return i
		}
	}
	for i, r := range rows {
		if r.line > line {
			return i
		}
	}
	return max(0, len(rows)-1)
}

// rowOfFold returns the marker row of the fold, -1 if it is open.
func (m Model) rowOfFold(fi int) int {
	for i, r := range m.rows() {
		if r.fold == fi {
			return i
		}
	}
	return -1
}

// foldOfLine returns the index of the fold containing the line, -1 if none.
func (m Model) foldOfLine(line int) int {
	for fi, fold := range m.files[m.fileIdx].Folds {
		if fold.Start <= line && line < fold.End {
			return fi
		}
	}
	return -1
}

func (m Model) View() tea.View {
	var content string
	if len(m.files) == 0 {
		content = m.emptyView()
	} else {
		content = m.fileView()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// emptyView is the friendly hint shown instead of an empty review.
func (m Model) emptyView() string {
	hint := []string{"Nothing to review."}
	if m.label != "" {
		hint = append(hint, "", "No changes in "+m.label+".")
	}
	hint = append(hint, "", "Press q to quit.")
	if h := max(0, m.height); len(hint) > h {
		hint = hint[:h]
	}
	lines := make([]string, len(hint))
	for i, line := range hint {
		lines[i] = ansi.Truncate(line, m.width, "")
	}
	return strings.Join(lines, "\n")
}

func (m Model) fileView() string {
	var b strings.Builder
	rows := m.rows()
	for i := range m.bodyHeight() {
		if j := m.offset + i; j < len(rows) {
			b.WriteString(ansi.Truncate(m.renderRow(rows[j], j == m.cursor), m.width, ""))
		}
		b.WriteByte('\n')
	}
	b.WriteString(ansi.Truncate(m.statusLine(), m.width, ""))
	return b.String()
}

// statusLine shows file path, position in the file list and the +/−
// statistics, plus the unified marker when the classic view is active.
func (m Model) statusLine() string {
	f := m.files[m.fileIdx]
	added, deleted := f.Stats()
	parts := []string{
		f.Path,
		fmt.Sprintf("%d/%d", m.fileIdx+1, len(m.files)),
		fmt.Sprintf("+%d −%d", added, deleted),
	}
	if m.unified {
		parts = append(parts, "[unified]")
	}
	return statusStyle.Render(strings.Join(parts, "  "))
}

// renderRow renders one row of the current file, highlighting the cursor.
func (m Model) renderRow(r row, cursor bool) string {
	if r.header != "" {
		style := hunkStyle
		if cursor {
			style = style.Background(cursorBG)
		}
		return style.Render(r.header)
	}
	if r.fold >= 0 {
		style := foldStyle
		if cursor {
			style = style.Background(cursorBG)
		}
		return style.Render(foldText(m.files[m.fileIdx].Folds[r.fold].Count()))
	}
	return renderLine(m.files[m.fileIdx].Lines[r.line], cursor)
}

// foldText is the marker of a collapsed unchanged run.
func foldText(n int) string {
	if n == 1 {
		return "⋯ 1 unveränderte Zeile ⋯"
	}
	return fmt.Sprintf("⋯ %d unveränderte Zeilen ⋯", n)
}

// bodyHeight is the number of file rows that fit above the status line.
func (m Model) bodyHeight() int {
	return max(0, m.height-1)
}

func renderLine(l diff.Line, cursor bool) string {
	text := strings.ReplaceAll(l.Text, "\t", "    ")
	gutter := gutterStyle
	if cursor {
		gutter = gutter.Background(cursorBG)
	}
	marker, body := "   ", text
	switch l.Kind {
	case diff.Added:
		marker = " + "
		style := addedStyle
		if cursor {
			style = style.Background(cursorBG)
		}
		body = style.Render(text)
	case diff.Deleted:
		marker = " - "
		style := deletedStyle
		if cursor {
			style = style.Background(cursorBG)
		}
		body = style.Render(text)
	}
	return gutter.Render(fmt.Sprintf("%4s %4s", lineNum(l.Old), lineNum(l.New))) + marker + body
}

func lineNum(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}
