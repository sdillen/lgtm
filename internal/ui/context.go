package ui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/keymap"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/search"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
)

// contextFile is a file beyond the review, opened read-only in its new
// state. The view shows every line with a plain line number; there are
// no hunks, folds or comments. cursor and offset live in the model, as
// line indexes.
type contextFile struct {
	path  string
	lines []string
	hl    *fileHighlights
}

// jumpEntry is one stop of the jumplist: a file of the review or a
// context file, a 0-based line and the cursor column at that line.
type jumpEntry struct {
	file int    // index into files, -1 for a context file
	ctx  string // context path when file < 0
	line int
	col  int
}

// openPickerAll opens the fuzzy picker over every file of the repo, so
// any file can be opened read-only in its new state.
func (m *Model) openPickerAll() {
	if m.opts.Search == nil {
		m.fail("repo file listing is not available")
		return
	}
	files, err := m.opts.Search.Files()
	if err != nil {
		m.fail(err.Error())
		return
	}
	m.pickerAll = true
	m.pickerFiles = files
	m.mode = modePicker
	m.pickerQuery = ""
	m.pickerSel = 0
	m.pickerMatches = make([]int, len(files))
	for i := range files {
		m.pickerMatches[i] = i
	}
	m.startSlideOverlay(m.bodyHeight())
}

// pickFile opens the picker entry: a review file or, in the all-files
// picker, a file beyond the review read-only.
func (m *Model) pickFile(i int) {
	if m.pickerAll {
		path := m.pickerFiles[i]
		if fi, ok := m.pathIndex[path]; ok {
			m.jumpToFile(fi, 0)
		} else {
			m.openContextAt(path, 0)
		}
	} else {
		m.jumpToFile(i, 0)
	}
}

// jumpToFile switches to review file fi at the line and records the
// jump.
func (m *Model) jumpToFile(fi int, line int) {
	m.recordJump(jumpEntry{file: fi, line: line})
	m.col = 0
	m.context = nil
	m.fileIdx = fi
	m.resetFile()
	m.cursor = m.rowOfLine(line)
	m.scrollCursorIntoView()
}

// openContextAt opens path read-only at the line and records the jump.
// A review file with that path is switched to instead. ok reports
// whether the file could be shown.
func (m *Model) openContextAt(path string, line int) bool {
	if fi, ok := m.pathIndex[path]; ok {
		m.jumpToFile(fi, line)
		return true
	}
	m.recordJump(jumpEntry{file: -1, ctx: path, line: line})
	if !m.showContext(path, line) {
		return false
	}
	m.scrollCursorIntoView()
	return true
}

// showContext shows path read-only at the line, replacing the review
// file in the view. ok reports whether the file could be shown.
func (m *Model) showContext(path string, line int) bool {
	if m.opts.Search == nil {
		m.fail("repo file listing is not available")
		return false
	}
	content, err := m.opts.Search.Read(path)
	if err != nil {
		m.fail(err.Error())
		return false
	}
	if search.IsBinary(content) {
		m.fail(path + " is a binary file")
		return false
	}
	m.context = &contextFile{
		path:  path,
		lines: search.SplitLines(content),
		hl:    newTextHighlights(m.theme.Highlighter, path, content),
	}
	m.cursor = min(line, len(m.context.lines)-1)
	m.offset, m.col = 0, 0
	m.searchMatches = nil
	return true
}

// syncJump brings the current jumplist entry in line with the position
// the next jump leaves from: the open file, the cursor line and the
// cursor column. Small motions after a jump stay part of the entry,
// like vim remembers where the cursor was before the jump.
func (m *Model) syncJump() {
	if m.jumpPos < 0 || m.jumpPos >= len(m.jumps) {
		return
	}
	cur := &m.jumps[m.jumpPos]
	if m.context != nil {
		cur.file, cur.ctx = -1, m.context.path
	} else {
		cur.file, cur.ctx = m.fileIdx, ""
	}
	cur.line = m.cursorViewLine()
	cur.col = m.col
}

// recordJump appends the position as the new current entry of the
// jumplist, dropping the forward history. The entry carries the
// current cursor column. A jump to the current entry is ignored.
func (m *Model) recordJump(e jumpEntry) {
	e.col = m.col
	m.syncJump()
	if m.jumpPos >= 0 && m.jumps[m.jumpPos] == e {
		return
	}
	m.jumps = append(m.jumps[:m.jumpPos+1], e)
	m.jumpPos = len(m.jumps) - 1
}

// jumpHistory moves back (dir -1) or forward (dir 1) in the jumplist.
func (m *Model) jumpHistory(dir int) {
	next := m.jumpPos + dir
	if next < 0 || next >= len(m.jumps) {
		return
	}
	e := m.jumps[next]
	m.syncJump()
	if e.file >= 0 {
		if e.file >= len(m.files) {
			return // stale entry
		}
		m.context = nil
		m.fileIdx = e.file
		m.resetFile()
		m.cursor = m.rowOfLine(e.line)
	} else if !m.showContext(e.ctx, e.line) {
		return // stale entry; stay put
	}
	m.jumpPos = next
	m.col = e.col
	m.scrollCursorIntoView()
}

// doContext runs an action in a context file. Only navigation, the
// search actions, gr/gd and the jumplist work there; the review
// actions (comments, folds, hunks, read status) are ignored: the file
// is read-only.
func (m Model) doContext(a keymap.Action, count int) Model {
	n := max(1, count)
	last := len(m.context.lines) - 1
	switch a {
	case actDown:
		m.cursor = min(m.cursor+n, last)
	case actUp:
		m.cursor = max(m.cursor-n, 0)
	case actHalfDown:
		half := max(1, m.bodyHeight()/2)
		m.offset = min(m.offset+n*half, max(0, last+1-m.bodyHeight()))
		m.cursor = min(m.cursor+n*half, last)
	case actHalfUp:
		half := max(1, m.bodyHeight()/2)
		m.offset = max(m.offset-n*half, 0)
		m.cursor = max(m.cursor-n*half, 0)
	case actTop:
		if count > 0 {
			m.jumpToViewLine(min(count-1, last))
		} else {
			m.jumpToViewLine(0)
		}
	case actBottom:
		if count > 0 {
			m.jumpToViewLine(min(count-1, last))
		} else {
			m.jumpToViewLine(last)
		}
	case actCenter:
		if count > 0 {
			m.jumpToViewLine(min(count-1, last))
		}
		m.offset = m.cursor - m.bodyHeight()/2
	case actCursorTop:
		if count > 0 {
			m.jumpToViewLine(min(count-1, last))
		}
		m.offset = m.cursor
	case actScrollLeft:
		if !m.wrap {
			m.hscroll = max(0, m.hscroll-m.contentWidth()/2)
		}
	case actScrollRight:
		if !m.wrap {
			m.hscroll += m.contentWidth() / 2
		}
	case actNextFile, actPrevFile:
		dir := 1
		if a == actPrevFile {
			dir = -1
		}
		for range n {
			if !m.jumpFile(dir) {
				break
			}
		}
	}
	m.scrollCursorIntoView()
	return m
}

// openSearch opens the / search line.
func (m *Model) openSearch() {
	m.mode = modeSearch
	m.searchQuery = ""
}

// searchKey edits the search line: text is inserted, <bs> deletes (and
// leaves an empty line), <esc> cancels, <cr> searches.
func (m Model) searchKey(msg tea.KeyPressMsg) Model {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
	case "enter":
		m.mode = modeNormal
		m.runSearch(m.searchQuery)
	case "backspace":
		if m.searchQuery == "" {
			m.mode = modeNormal
			break
		}
		_, size := utf8.DecodeLastRuneInString(m.searchQuery)
		m.searchQuery = m.searchQuery[:len(m.searchQuery)-size]
	default:
		if msg.Text != "" {
			m.searchQuery += msg.Text
		}
	}
	return m
}

// runSearch searches the current view for the pattern and jumps to the
// first match after the cursor, wrapping around.
func (m *Model) runSearch(pattern string) {
	if pattern == "" {
		return
	}
	matches := search.FindInFile(m.viewLines(), pattern)
	m.searchQuery = pattern
	m.searchIdx = 0
	m.searchMatches = matches
	if len(matches) == 0 {
		m.fail("no matches: " + pattern)
		return
	}
	cur := m.cursorViewLine()
	for i, l := range matches {
		if l > cur {
			m.searchIdx = i
			break
		}
	}
	m.jumpToViewLine(matches[m.searchIdx])
}

// searchJump moves to the next (dir 1) or previous (dir -1) match,
// wrapping around.
func (m *Model) searchJump(dir int) {
	if len(m.searchMatches) == 0 {
		m.fail("no previous search")
		return
	}
	n := len(m.searchMatches)
	m.searchIdx = ((m.searchIdx+dir)%n + n) % n
	m.jumpToViewLine(m.searchMatches[m.searchIdx])
}

// searchWord searches the word under the cursor: forward (dir 1) like
// *, backward (dir -1) like #. The search continues with n/N.
func (m *Model) searchWord(dir int) {
	if !m.canView() {
		return
	}
	line := m.viewLine(m.cursorViewLine())
	word, _, _ := search.WordAt(line, m.col)
	if word == "" {
		m.fail("no word under the cursor")
		return
	}
	matches := search.FindWordInFile(m.viewLines(), word)
	m.searchQuery = word
	m.searchIdx = 0
	m.searchMatches = matches
	if len(matches) == 0 {
		m.fail("no matches: " + word)
		return
	}
	cur := m.cursorViewLine()
	if dir > 0 {
		for i, l := range matches {
			if l > cur {
				m.searchIdx = i
				break
			}
		}
	} else {
		m.searchIdx = len(matches) - 1
		for i, l := range matches {
			if l < cur {
				m.searchIdx = i
			}
		}
	}
	m.jumpToViewLine(matches[m.searchIdx])
}

// jumpToViewLine moves the cursor to the view line and records the
// jump.
func (m *Model) jumpToViewLine(line int) {
	if m.context != nil {
		m.recordJump(jumpEntry{file: -1, ctx: m.context.path, line: line})
		m.cursor = line
	} else {
		m.recordJump(jumpEntry{file: m.fileIdx, line: line})
		m.cursor = m.rowOfLine(line)
	}
	m.scrollCursorIntoView()
}

// grep searches the word under the cursor in the whole repository and
// shows the hits in a result list.
func (m *Model) grep() {
	if !m.canView() {
		return
	}
	if m.opts.Search == nil {
		m.fail("repo search is not available")
		return
	}
	word, _, _ := search.WordAt(m.viewLine(m.cursorViewLine()), m.col)
	if word == "" {
		m.fail("no word under the cursor")
		return
	}
	hits, err := m.opts.Search.FindWord(word)
	if err != nil {
		m.fail(err.Error())
		return
	}
	m.openResults("gr: "+word, hits)
}

// gotoDef jumps to the heuristic definition of the word under the
// cursor: directly when it is unique, via the result list when several
// lines define the symbol.
func (m *Model) gotoDef() {
	if !m.canView() {
		return
	}
	if m.opts.Search == nil {
		m.fail("repo search is not available")
		return
	}
	word, _, _ := search.WordAt(m.viewLine(m.cursorViewLine()), m.col)
	if word == "" {
		m.fail("no word under the cursor")
		return
	}
	hits, err := m.opts.Search.FindDefinitions(word)
	if err != nil {
		m.fail(err.Error())
		return
	}
	switch len(hits) {
	case 0:
		m.fail("no definition of " + word + " found")
	case 1:
		m.jumpToHit(hits[0])
	default:
		m.openResults("gd: "+word, hits)
	}
}

// openResults shows the hits of gr or gd as a navigable list, sliding
// in from the bottom edge.
func (m *Model) openResults(title string, hits []search.Hit) {
	if len(hits) == 0 {
		return
	}
	m.resultsTitle = title
	m.results = hits
	m.resultSel = 0
	m.mode = modeResults
	m.startSlideOverlay(m.bodyHeight())
}

// resultsKey handles keys while the result list is open: j/k move the
// selection, enter jumps to the hit, esc closes the list.
func (m Model) resultsKey(msg tea.KeyPressMsg) Model {
	switch msg.String() {
	case "j", "down", "ctrl+n":
		m.resultSel = min(m.resultSel+1, len(m.results)-1)
	case "k", "up", "ctrl+p":
		m.resultSel = max(m.resultSel-1, 0)
	case "enter":
		m.jumpToHit(m.results[m.resultSel])
	case "esc":
		m.mode = modeNormal
	}
	return m
}

// jumpToHit opens the file of the hit at its line and records the
// jump: the review file when the path is part of the review, a context
// file otherwise.
func (m *Model) jumpToHit(h search.Hit) {
	if fi, ok := m.pathIndex[h.Path]; ok {
		m.jumpToFile(fi, h.Line-1)
		return
	}
	m.openContextAt(h.Path, h.Line-1)
}

// moveCol moves the cursor column by one rune: left (dir -1) or right
// (dir 1), keeping the column inside the line.
func (m *Model) moveCol(dir int) {
	if !m.canView() {
		return
	}
	line := m.viewLine(m.cursorViewLine())
	switch dir {
	case -1:
		if m.col > 0 {
			_, size := utf8.DecodeLastRuneInString(line[:m.col])
			m.col -= size
		}
	case 1:
		if m.col < len(line) {
			_, size := utf8.DecodeRuneInString(line[m.col:])
			m.col += size
		}
	}
	m.scrollToCol()
}

// scrollToCol scrolls horizontally so the cursor column stays visible.
// With wrap on there is no horizontal scroll.
func (m *Model) scrollToCol() {
	if m.wrap {
		return
	}
	line := m.viewLine(m.cursorViewLine())
	cell := ansi.StringWidth(strings.ReplaceAll(line[:m.col], "\t", "    "))
	if cell < m.hscroll {
		m.hscroll = cell
	}
	if cell >= m.hscroll+m.contentWidth() {
		m.hscroll = cell - m.contentWidth() + 1
	}
}

// canView reports whether a file is open to search or move in: the
// review file or a context file.
func (m Model) canView() bool {
	return m.context != nil || len(m.files) > 0
}

// viewLines are the lines of the current view: the review file's lines
// as displayed, or the context file's lines. Info entries have none.
func (m Model) viewLines() []string {
	if m.context != nil {
		return m.context.lines
	}
	if len(m.files) == 0 {
		return nil
	}
	f := m.files[m.fileIdx]
	lines := make([]string, len(f.Lines))
	for i, l := range f.Lines {
		lines[i] = l.Text
	}
	return lines
}

// viewLine is the text of the view line: the review file's line as
// displayed, or the context file's line. Info entries have no lines
// and yield empty text.
func (m Model) viewLine(i int) string {
	if m.context != nil {
		return m.context.lines[i]
	}
	f := m.files[m.fileIdx]
	if i < 0 || i >= len(f.Lines) {
		return ""
	}
	return f.Lines[i].Text
}

// cursorViewLine is the 0-based line index of the cursor in the current
// view: the file line for review rows, the line index for context rows.
func (m Model) cursorViewLine() int {
	if m.context != nil {
		return m.cursor
	}
	return m.cursorLine()
}

// viewRows is the number of rows of the current view: the rows of the
// review file or the lines of the context file.
func (m Model) viewRows() int {
	if m.context != nil {
		return len(m.context.lines)
	}
	return len(m.rows())
}

func (m Model) contextView() string {
	width := m.contentWidth()
	m.context.hl.ensureNew(m.offset + m.bodyHeight() + highlightBuffer)
	var b strings.Builder
	if m.wrap {
		m.renderWrapped(&b, width, len(m.context.lines), func(j int) string {
			return m.renderContextLine(j, j == m.cursor)
		})
	} else {
		for i := range m.bodyHeight() {
			if j := m.offset + i; j < len(m.context.lines) {
				row := ansi.TruncateLeft(m.renderContextLine(j, j == m.cursor), m.hscroll, "")
				b.WriteString(ansi.Truncate(row, width, ""))
			}
			b.WriteByte('\n')
		}
	}
	bottom := m.bottomLine()
	if bottom == "" {
		bottom = m.contextStatusLine()
	}
	b.WriteString(ansi.Truncate(bottom, width, ""))
	return b.String()
}

// contextStatusLine shows the context file path, the ctx marker and the
// cursor position, plus the keys of a pending sequence.
func (m Model) contextStatusLine() string {
	parts := []string{
		m.context.path,
		"ctx",
		fmt.Sprintf("%d/%d", m.cursor+1, len(m.context.lines)),
	}
	if typed := m.keys.Typed(); typed != "" {
		parts = append(parts, typed)
	}
	return m.theme.Styles.Status.Render(strings.Join(parts, "  "))
}

// renderContextLine renders one line of a context file: the line number
// and the syntax-highlighted body, without the diff gutter and markers.
func (m Model) renderContextLine(idx int, cursor bool) string {
	st := m.theme.Styles
	gutter := st.Gutter
	if cursor {
		gutter = gutter.Background(st.Cursor)
	}
	segs := m.context.hl.newLine(idx + 1)
	if len(segs) == 0 && m.context.lines[idx] != "" {
		segs = []theme.Segment{{Text: strings.ReplaceAll(m.context.lines[idx], "\t", "    ")}}
	}
	num := fmt.Sprintf("%4d ", idx+1)
	var b strings.Builder
	m.renderBody(&b, segs, len(num), cursor)
	return gutter.Render(num) + b.String()
}
