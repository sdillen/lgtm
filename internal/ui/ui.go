// Package ui renders ReviewFile models in the terminal. It knows nothing
// about git or diffing: the file view is a full-file view with folded
// unchanged runs, a toggleable classic unified view, and vim-like
// navigation. All fold and hunk regions come from the ReviewFile model.
package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/keymap"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/theme"
)

// contextWindow is how many unchanged lines surround each hunk in the
// classic unified diff view.
const contextWindow = 3

// defaultWhichKeyDelay is how long a prefix waits before which-key shows
// its follow-up keys.
const defaultWhichKeyDelay = 300 * time.Millisecond

// Options configure the review view. The zero value is the default.
type Options struct {
	// Keymap resolves keys to actions; nil means the default bindings.
	Keymap *keymap.Keymap
	// LoadTheme loads a theme by name for :theme; nil disables
	// switching.
	LoadTheme func(name string) (theme.Theme, error)
	// WhichKeyDelay is the wait before which-key appears; 0 means the
	// default of 300ms.
	WhichKeyDelay time.Duration
	// State is the review state the read status lives in; nil means a
	// session-only state.
	State *review.State
	// Dir is the worktree root the state is saved to; empty disables
	// saving.
	Dir string
	// CfgDir is the config directory holding the global prompt
	// template; empty disables it.
	CfgDir string
	// Branch, Base and Round are the prompt template data of the
	// review; Round is 0 until rounds are tracked.
	Branch string
	Base   string
	Round  int
	// Copy copies the prompt to the clipboard; nil means the default
	// OSC 52/tool chain.
	Copy func(string) error
	// ApprovalDelay is the confirmation moment of :lgtm; 0 means the
	// default of 800ms.
	ApprovalDelay time.Duration
}

// mode is what key presses go to.
type mode int

const (
	modeNormal  mode = iota // the review view
	modeHelp                // the ? help overlay
	modeCmdline             // the : command line
	modeDrawer              // the file drawer
	modePicker              // the fuzzy file picker
	modeComment             // the comment float
)

// Model is the review view: one file at a time, with folds for the
// unchanged runs of the full-file view and a toggleable unified view.
// label names the review subject and appears in the hint when there is
// nothing to review. theme provides the semantic styles and the syntax
// highlighter.
type Model struct {
	files     []diff.ReviewFile
	label     string
	theme     theme.Theme
	opts      Options
	width     int
	height    int
	fileIdx   int
	cursor    int    // index of the cursor row in rows()
	offset    int    // index of the first visible row
	foldsOpen []bool // per fold of the current file: open?
	unified   bool   // show the classic unified diff instead of the full file

	mode       mode
	keys       keymap.Dispatcher // the key sequence typed so far
	keyGen     int               // counts key presses, to drop stale which-key timers
	whichKey   bool              // the which-key popup is shown
	helpOffset int               // first visible line of the help
	cmdline    string            // text of the command line
	message    string            // command result in the bottom line, until the next key
	messageErr bool              // message is an error

	// state holds the read status per file; dir is where it is saved,
	// "" disables saving.
	state *review.State
	dir   string

	// tree is the file tree of the drawer and the sidebar, built once;
	// drawerCursor is the selected row while the drawer is open.
	tree         []treeRow
	drawerCursor int
	sidebar      bool // the permanent sidebar is shown

	// pickerQuery is the fuzzy query, pickerMatches the matching file
	// indexes and pickerSel the selected match.
	pickerQuery   string
	pickerMatches []int
	pickerSel     int

	// visual is the line selection of V, started at visualLine (a line
	// index of the current file).
	visual     bool
	visualLine int

	// quitArmed remembers that the quit warning was shown; the next q
	// quits. Any other action disarms it.
	quitArmed bool
	// quitOpen reports that open comments remained when the review
	// ended; it becomes the exit code 1.
	quitOpen bool
	// approved is true after :lgtm; the review ends with exit code 0.
	approved bool

	// draft is the comment being written while the float is open;
	// fileComments caches the comments of the current file.
	draft        draft
	fileComments []review.Comment

	// endVisible tells whether the end of the current file was visible
	// after the last update; the read status is only set when it
	// becomes visible, so the manual toggle is not undone by itself.
	endVisible bool

	// highlights are the syntax segments of the new-file content, one
	// entry per line (indexed by new line number - 1). dimmed holds the
	// dimmed segments of the deleted lines, keyed by line index.
	highlights []theme.Line
	dimmed     map[int]theme.Line
}

// whichKeyMsg fires when a prefix has waited long enough for which-key.
// gen is the key press it belongs to.
type whichKeyMsg struct{ gen int }

// approveMsg ends the program after the :lgtm confirmation moment.
type approveMsg struct{}

// New returns a Model showing files of the review subject label with the
// given theme.
func New(files []diff.ReviewFile, label string, th theme.Theme, opts Options) Model {
	if opts.Keymap == nil {
		km, err := Keymap(keymap.Overrides{})
		if err != nil {
			panic(fmt.Sprintf("default keymap: %v", err))
		}
		opts.Keymap = km
	}
	if opts.WhichKeyDelay == 0 {
		opts.WhichKeyDelay = defaultWhichKeyDelay
	}
	if opts.State == nil {
		opts.State = review.NewState()
	}
	m := Model{
		files: files,
		label: label,
		theme: th,
		opts:  opts,
		keys:  opts.Keymap.Dispatcher(),
		state: opts.State,
		dir:   opts.Dir,
	}
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.Path
	}
	m.tree = fileTree(paths)
	m.drawerCursor = m.firstFileRow()
	m.pickerMatches = m.allFileIndexes()
	m.resetFile()
	return m
}

// firstFileRow is the tree row of the first file, -1 if there is none.
func (m Model) firstFileRow() int {
	for i, r := range m.tree {
		if !r.dir {
			return i
		}
	}
	return -1
}

// allFileIndexes lists every file index, for the picker before any query.
func (m Model) allFileIndexes() []int {
	all := make([]int, len(m.files))
	for i := range m.files {
		all[i] = i
	}
	return all
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clamp()
		m.clampHelp()
		m.maybeMarkRead()
	case whichKeyMsg:
		if msg.gen == m.keyGen && m.keys.InPrefix() {
			m.whichKey = true
		}
	case approveMsg:
		return m, tea.Quit
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.quitOpen = len(m.state.OpenComments()) > 0
			return m, tea.Quit
		}
		m.keyGen++
		m.whichKey = false
		m.message, m.messageErr = "", false
		switch m.mode {
		case modeCmdline:
			return m.cmdlineKey(msg)
		case modeDrawer:
			m = m.drawerKey(msg)
			m.maybeMarkRead()
			return m, nil
		case modePicker:
			m, cmd := m.pickerKey(msg)
			m.maybeMarkRead()
			return m, cmd
		case modeComment:
			return m.commentKey(msg), nil
		}
		m, cmd := m.dispatch(msg.String())
		m.maybeMarkRead()
		return m, cmd
	}
	return m, nil
}

// dispatch feeds a key through the keymap and runs the bound action. A
// prefix starts the which-key timer.
func (m Model) dispatch(key string) (Model, tea.Cmd) {
	var r keymap.Result
	m.keys, r = m.keys.Press(key)
	switch r.Status {
	case keymap.Pending:
		if m.keys.InPrefix() {
			gen := m.keyGen
			return m, tea.Tick(m.opts.WhichKeyDelay, func(time.Time) tea.Msg {
				return whichKeyMsg{gen: gen}
			})
		}
	case keymap.Matched:
		if m.mode == modeHelp {
			return m.helpAction(r.Action, r.Count), nil
		}
		return m.do(r.Action, r.Count)
	case keymap.Unbound:
		switch {
		case m.mode == modeHelp && key == "esc":
			m.mode = modeNormal
		case key == "esc":
			m.visual = false
		}
	}
	return m, nil
}

// do runs an action of the review view. count is the count typed before
// the keys, 0 if none: motions repeat, line jumps go to line count.
func (m Model) do(a keymap.Action, count int) (Model, tea.Cmd) {
	// Any action except q cancels the quit warning, so the warning is
	// for the next q only.
	if a != actQuit {
		m.quitArmed = false
	}
	switch a {
	case actQuit:
		return m, m.quitAction()
	case actCopyPrompt:
		m.copyPrompt()
		return m, nil
	case actCopyComment:
		m.copyComment()
		return m, nil
	case actHelp:
		m.mode, m.helpOffset = modeHelp, 0
		return m, nil
	case actCmdline:
		m.mode, m.cmdline = modeCmdline, ""
		return m, nil
	case actThemeCmd:
		m.mode, m.cmdline = modeCmdline, "theme "
		return m, nil
	case actDrawer:
		if len(m.files) > 0 {
			m.openDrawer()
		}
		return m, nil
	case actPicker:
		if len(m.files) > 0 {
			m.openPicker()
		}
		return m, nil
	case actToggleRead:
		if len(m.files) > 0 {
			m.toggleRead()
		}
		return m, nil
	}
	if len(m.files) == 0 {
		return m, nil
	}
	n := max(1, count)
	lastLine := len(m.files[m.fileIdx].Lines) - 1
	switch a {
	case actDown:
		m.cursor = min(m.cursor+n, max(0, len(m.rows())-1))
	case actUp:
		m.cursor = max(m.cursor-n, 0)
	case actHalfDown:
		half := max(1, m.bodyHeight()/2)
		rows := m.rows()
		m.offset = min(m.offset+n*half, max(0, len(rows)-m.bodyHeight()))
		m.cursor = min(m.cursor+n*half, max(0, len(rows)-1))
	case actHalfUp:
		half := max(1, m.bodyHeight()/2)
		m.offset = max(m.offset-n*half, 0)
		m.cursor = max(m.cursor-n*half, 0)
	case actTop:
		if count > 0 {
			m.cursor = m.rowOfLine(min(count-1, lastLine))
		} else {
			m.cursor, m.offset = 0, 0
		}
	case actBottom:
		if count > 0 {
			m.cursor = m.rowOfLine(min(count-1, lastLine))
		} else {
			m.cursor = max(0, len(m.rows())-1)
			m.offset = max(0, len(m.rows())-m.bodyHeight())
		}
	case actCenter:
		if count > 0 {
			m.cursor = m.rowOfLine(min(count-1, lastLine))
		}
		m.offset = m.cursor - m.bodyHeight()/2
	case actCursorTop:
		if count > 0 {
			m.cursor = m.rowOfLine(min(count-1, lastLine))
		}
		m.offset = m.cursor
	case actFoldOpen:
		if fi := m.foldAtCursor(); fi >= 0 && !m.foldsOpen[fi] {
			m.foldsOpen[fi] = true
			m.cursor = m.rowOfLine(m.files[m.fileIdx].Folds[fi].Start)
		}
	case actFoldClose:
		if fi := m.foldAtCursor(); fi >= 0 && m.foldsOpen[fi] {
			m.foldsOpen[fi] = false
			m.cursor = m.rowOfFold(fi)
		}
	case actFoldsOpenAll, actFoldsCloseAll:
		old := m.cursorLine()
		for i := range m.foldsOpen {
			m.foldsOpen[i] = a == actFoldsOpenAll
		}
		m.cursor = m.rowOfLine(old)
	case actNextHunk, actPrevHunk:
		dir := 1
		if a == actPrevHunk {
			dir = -1
		}
		for range n {
			m.jumpToHunk(dir)
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
	case actUnified:
		m.setUnified(!m.unified)
	case actVisual:
		m.toggleVisual()
	case actComment:
		m.openComment()
	}
	m.scrollCursorIntoView()
	return m, nil
}

// quitAction ends the review: with open comments the first q only warns
// and the second q quits, with the exit code reporting the open
// comments.
func (m *Model) quitAction() tea.Cmd {
	n := len(m.state.OpenComments())
	if n > 0 && !m.quitArmed {
		m.quitArmed = true
		m.message = fmt.Sprintf("%d offene Kommentare – Y kopieren? Nochmal q beendet", n)
		return nil
	}
	m.quitOpen = n > 0
	return tea.Quit
}

// ExitCode is the process exit code of the finished review: 0 when the
// review was approved with :lgtm or no open comments remained, 1 when
// open comments remained, so scripts and agents can react.
func (m Model) ExitCode() int {
	if m.approved {
		return 0
	}
	if m.quitOpen {
		return 1
	}
	return 0
}

// setUnified switches between the full-file and the unified view.
func (m *Model) setUnified(on bool) {
	if on == m.unified {
		return
	}
	m.unified = on
	m.cursor, m.offset = 0, 0
	m.visual = false
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

// resetFile resets the per-file view state for the current file and
// highlights it once.
func (m *Model) resetFile() {
	m.cursor, m.offset = 0, 0
	m.endVisible = false
	m.visual = false
	m.loadFileComments()
	if len(m.files) == 0 {
		m.foldsOpen = nil
		m.highlights, m.dimmed = nil, nil
		return
	}
	f := m.files[m.fileIdx]
	m.foldsOpen = make([]bool, len(f.Folds))
	m.highlights, m.dimmed = highlightFile(m.theme.Highlighter, f)
}

// highlightFile highlights a ReviewFile once: the added and context lines
// come from the whole new-file token stream (so multi-line tokens stay
// intact), the deleted lines are dimmed and lexed per line.
func highlightFile(h *theme.Highlighter, f diff.ReviewFile) ([]theme.Line, map[int]theme.Line) {
	var sb strings.Builder
	for _, l := range f.Lines {
		if l.New != 0 {
			sb.WriteString(l.Text)
			sb.WriteByte('\n')
		}
	}
	lines := h.Highlight(f.Path, sb.String())
	dimmed := make(map[int]theme.Line)
	for i, l := range f.Lines {
		if l.Kind != diff.Deleted {
			continue
		}
		if d := h.HighlightDim(f.Path, l.Text); len(d) > 0 {
			dimmed[i] = d[0]
		}
	}
	return lines, dimmed
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

// row is one row of the view: a line of the file, a fold marker, a
// section header or a line of a comment.
type row struct {
	line   int    // index into the file's Lines; header rows carry the section start, comment rows their anchor line
	fold   int    // fold index of a marker row, -1 otherwise
	header string // rendered section header of a header row, "" otherwise
	note   *note  // comment line of a comment row, nil otherwise
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
					rows = append(rows, m.noteRows(f.Lines[i].New, i)...)
				}
			} else {
				// The comments inside a closed fold stay visible
				// below its marker.
				rows = append(rows, row{line: -1, fold: fi})
				for ; i < f.Folds[fi].End; i++ {
					rows = append(rows, m.noteRows(f.Lines[i].New, i)...)
				}
			}
			fi++
		} else {
			rows = append(rows, row{line: i, fold: -1})
			rows = append(rows, m.noteRows(f.Lines[i].New, i)...)
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
			rows = append(rows, m.noteRows(f.Lines[i].New, i)...)
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
	if r.note != nil {
		// A comment row belongs to the fold of its line, open or
		// closed.
		return m.foldOfLine(r.line)
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
		if r.line == line && r.note == nil {
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
	v := tea.NewView(m.withOverlay(content))
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
	if bottom := m.bottomLine(); bottom != "" && m.height > 0 {
		for len(hint) < m.height {
			hint = append(hint, "")
		}
		hint[m.height-1] = bottom
	}
	lines := make([]string, len(hint))
	for i, line := range hint {
		lines[i] = ansi.Truncate(line, m.width, "")
	}
	return strings.Join(lines, "\n")
}

// fileView renders the file rows and the bottom line, with the sidebar
// beside them when it is on.
func (m Model) fileView() string {
	width := m.contentWidth()
	var b strings.Builder
	rows := m.rows()
	for i := range m.bodyHeight() {
		if j := m.offset + i; j < len(rows) {
			r := rows[j]
			hl := j == m.cursor || (r.note == nil && r.header == "" && m.selected(r.line))
			b.WriteString(ansi.Truncate(m.renderRow(r, hl), width, ""))
		}
		b.WriteByte('\n')
	}
	bottom := m.bottomLine()
	if bottom == "" {
		bottom = m.statusLine()
	}
	b.WriteString(ansi.Truncate(bottom, width, ""))
	if !m.sidebar {
		return b.String()
	}

	// The sidebar box spans the file area; the bottom line spans the
	// whole terminal below it.
	side := strings.Split(m.sidebarBox(), "\n")
	content := strings.Split(b.String(), "\n")
	var out strings.Builder
	for i := 0; i < m.bodyHeight(); i++ {
		if i < len(side) {
			out.WriteString(side[i])
		}
		out.WriteByte(' ')
		if i < len(content) {
			out.WriteString(content[i])
		}
		out.WriteByte('\n')
	}
	if len(content) > m.bodyHeight() {
		out.WriteString(content[m.bodyHeight()])
	}
	return out.String()
}

// contentWidth is the width of the file view: the terminal width, minus
// the sidebar when it is on.
func (m Model) contentWidth() int {
	if !m.sidebar {
		return m.width
	}
	return max(1, m.width-m.sidebarWidth()-1)
}

// statusLine shows file path, position in the file list and the +/−
// statistics, plus the unified marker when the classic view is active and
// the keys of a pending sequence.
func (m Model) statusLine() string {
	f := m.files[m.fileIdx]
	added, deleted := f.Stats()
	parts := []string{
		f.Path,
		fmt.Sprintf("%d/%d", m.fileIdx+1, len(m.files)),
		fmt.Sprintf("+%d −%d", added, deleted),
	}
	if len(m.state.Comments()) > 0 {
		parts = append(parts, m.commentCount())
	}
	if m.unified {
		parts = append(parts, "[unified]")
	}
	if m.visual {
		parts = append(parts, "-- VISUAL --")
	}
	if typed := m.keys.Typed(); typed != "" {
		parts = append(parts, typed)
	}
	return m.theme.Styles.Status.Render(strings.Join(parts, "  "))
}

// renderRow renders one row of the current file, highlighting the cursor.
func (m Model) renderRow(r row, cursor bool) string {
	if r.note != nil {
		return m.renderNote(r.note, cursor)
	}
	if r.header != "" {
		style := m.theme.Styles.Hunk
		if cursor {
			style = style.Background(m.theme.Styles.Cursor)
		}
		return style.Render(r.header)
	}
	if r.fold >= 0 {
		style := m.theme.Styles.Fold
		if cursor {
			style = style.Background(m.theme.Styles.Cursor)
		}
		return style.Render(foldText(m.files[m.fileIdx].Folds[r.fold].Count()))
	}
	return m.renderLine(m.files[m.fileIdx].Lines[r.line], r.line, cursor)
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

// renderLine renders one line of the current file: gutter, marker and the
// syntax-highlighted body. Added and context lines come from the file's
// highlight cache, deleted lines are the dimmed per-line highlight. The
// wash backgrounds of changed lines and the cursor background are applied
// on top of the token styles.
func (m Model) renderLine(l diff.Line, idx int, cursor bool) string {
	st := m.theme.Styles
	gutter := st.Gutter
	if cursor {
		gutter = gutter.Background(st.Cursor)
	}

	marker, markerStyle, wash := "   ", lipgloss.Style{}, color.Color(nil)
	switch l.Kind {
	case diff.Added:
		marker, markerStyle, wash = " + ", st.Added, st.AddedBG
	case diff.Deleted:
		marker, markerStyle, wash = " - ", st.Deleted, st.DeletedBG
	}
	if wash != nil {
		markerStyle = markerStyle.Background(wash)
	}
	if cursor {
		markerStyle = markerStyle.Background(st.Cursor)
	}

	segs := washSegments(m.segments(l, idx), wash)
	if len(l.Words) > 0 {
		bg := st.WordAddedBG
		if l.Kind == diff.Deleted {
			bg = st.WordDeletedBG
		}
		segs = wordSegments(segs, l.Text, l.Words, bg)
	}

	var b strings.Builder
	for _, s := range segs {
		style := s.Style
		if cursor {
			style = style.Background(st.Cursor)
		}
		b.WriteString(style.Render(s.Text))
	}
	return gutter.Render(fmt.Sprintf("%4s %4s", lineNum(l.Old), lineNum(l.New))) +
		markerStyle.Render(marker) + b.String()
}

// washSegments applies the background wash of a changed line to the
// syntax segments, without touching the highlight cache the segments came
// from.
func washSegments(segs []theme.Segment, wash color.Color) []theme.Segment {
	if wash == nil {
		return segs
	}
	washed := make([]theme.Segment, len(segs))
	for i, s := range segs {
		washed[i] = theme.Segment{Text: s.Text, Style: s.Style.Background(wash)}
	}
	return washed
}

// wordSegments splits the syntax segments of a line at the intra-line word
// range boundaries and applies the word highlight background to the
// changed pieces. The ranges are byte offsets into the raw line text; the
// segments carry the tab-expanded text, so the ranges are translated to
// expanded offsets first.
func wordSegments(segs []theme.Segment, raw string, words []diff.Word, bg color.Color) []theme.Segment {
	if len(words) == 0 {
		return segs
	}
	exp := make([]diff.Word, len(words))
	for i, w := range words {
		exp[i] = diff.Word{
			Start: w.Start + 3*strings.Count(raw[:w.Start], "\t"),
			End:   w.End + 3*strings.Count(raw[:w.End], "\t"),
		}
	}
	var out []theme.Segment
	pos := 0 // expanded offset of the current segment's start
	for _, s := range segs {
		start, end := pos, pos+len(s.Text)
		pos = end
		prev := start
		for _, w := range exp {
			if w.End <= start || w.Start >= end {
				continue
			}
			if w.Start > prev {
				out = append(out, theme.Segment{Text: s.Text[prev-start : w.Start-start], Style: s.Style})
				prev = w.Start
			}
			next := min(w.End, end)
			out = append(out, theme.Segment{Text: s.Text[prev-start : next-start], Style: s.Style.Background(bg)})
			prev = next
		}
		if prev < end {
			out = append(out, theme.Segment{Text: s.Text[prev-start:], Style: s.Style})
		}
	}
	return out
}

// segments returns the syntax segments of a line: from the file highlight
// cache for added and context lines, from the dimmed cache for deleted
// lines. Lines without a highlight fall back to plain text.
func (m Model) segments(l diff.Line, idx int) []theme.Segment {
	var segs []theme.Segment
	switch l.Kind {
	case diff.Added, diff.Context:
		if l.New > 0 && l.New-1 < len(m.highlights) {
			segs = m.highlights[l.New-1]
		}
	case diff.Deleted:
		segs = m.dimmed[idx]
	}
	if len(segs) == 0 && l.Text != "" {
		segs = []theme.Segment{{Text: strings.ReplaceAll(l.Text, "\t", "    ")}}
	}
	return segs
}

func lineNum(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}
