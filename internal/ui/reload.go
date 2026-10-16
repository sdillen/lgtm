package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sdillen/lgtm/internal/diff"
	"github.com/sdillen/lgtm/internal/review"
)

// changedMsg reports that the watcher saw the source change.
type changedMsg struct{}

// checkedMsg carries the review files as they are after a change, or
// the error resolving them. gen is the load of the files the check
// compares against; a check that started before R is stale.
type checkedMsg struct {
	files []diff.ReviewFile
	err   error
	gen   int
}

// waitForChange waits for the next watcher signal; nil when the review
// is not watched.
func (m Model) waitForChange() tea.Cmd {
	ch := m.opts.Changes
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		if _, ok := <-ch; !ok {
			return nil
		}
		return changedMsg{}
	}
}

// checkChanges resolves the review again in the background, so the hint
// can tell how many files differ from the shown ones. The view itself
// never changes on its own.
func (m Model) checkChanges() tea.Cmd {
	reload := m.opts.Reload
	if reload == nil {
		return m.waitForChange()
	}
	gen := m.loadGen
	return func() tea.Msg {
		files, err := reload()
		return checkedMsg{files: files, err: err, gen: gen}
	}
}

// takeCheck updates the change hint from a background check and waits
// for the next signal.
func (m Model) takeCheck(msg checkedMsg) (Model, tea.Cmd) {
	if msg.err == nil && msg.gen == m.loadGen {
		m.changed = changedFiles(m.full, msg.files)
	}
	return m, m.waitForChange()
}

// changedFiles counts the paths whose change differs between the shown
// files and the files of the source now: changed, new or gone.
func changedFiles(shown, now []diff.ReviewFile) int {
	byPath := make(map[string]diff.ReviewFile, len(shown))
	for _, f := range shown {
		byPath[f.Path] = f
	}
	n := 0
	for _, f := range now {
		g, ok := byPath[f.Path]
		if !ok || !f.SameChange(g) {
			n++
		}
		delete(byPath, f.Path)
	}
	return n + len(byPath)
}

// changeHint is the status line hint about changes not shown yet, ""
// when there are none.
func (m Model) changeHint() string {
	switch m.changed {
	case 0:
		return ""
	case 1:
		return "↻ 1 Datei geändert – R neu laden"
	}
	return fmt.Sprintf("↻ %d Dateien geändert – R neu laden", m.changed)
}

// reload resolves the review again and shows its new state, the whole
// review or the interdiff. The comments are relocated to their quoted
// code, the snapshot of the round becomes the shown state, and the
// cursor stays on the code line it was on, at the same height on
// screen.
func (m *Model) reload() {
	if m.opts.Reload == nil {
		m.fail("reload is not available")
		return
	}
	next, err := m.opts.Reload()
	if err != nil {
		m.fail("reload: " + err.Error())
		return
	}
	if m.ignoreWS {
		for i := range next {
			next[i] = next[i].WithIgnoreWS(true)
		}
	}
	// The anchors live in the whole review: deleted lines number the
	// old state, which the interdiff does not show.
	orphans := m.state.Relocate(relocationFiles(m.full), relocationFiles(next))
	m.saveState()
	var snapErr error
	if m.opts.Snapshot != nil {
		snapErr = m.opts.Snapshot()
	}

	at := m.place()
	collapsed := make(map[string]bool, len(m.files))
	for i, f := range m.files {
		collapsed[f.Path] = m.fileCollapsed[i]
	}
	m.startFlash(m.files)
	m.full = next
	m.loadInterdiff()
	if m.interdiff && m.inter != nil {
		next = m.inter
	} else {
		m.interdiff = false
		next = m.full
	}
	m.setFiles(next, collapsed)
	m.changed = 0
	m.loadGen++
	m.restore(at)
	m.dropIdleFlash()

	m.message = "neu geladen"
	switch {
	case orphans == 1:
		m.message += " – 1 Kommentar verwaist"
	case orphans > 1:
		m.message += fmt.Sprintf(" – %d Kommentare verwaist", orphans)
	}
	if snapErr != nil {
		m.fail("snapshot: " + snapErr.Error())
	}
}

// place is where the reviewer is before a reload: the file, the line
// under the cursor with the content of its side, and the screen row of
// the cursor. A context file is remembered by its path and content.
type place struct {
	file      int
	path      string
	screenRow int

	// hasLine tells whether the cursor was on the lines of a file;
	// line is that line, side the content it numbers (old for a
	// deleted line), onLine false when the cursor was on a marker.
	hasLine bool
	line    diff.Line
	side    string
	onLine  bool

	context string // path of the open context file, "" for none
	ctxText string
	ctxLine int
}

// place captures where the reviewer is.
func (m Model) place() place {
	p := place{file: m.fileIdx, screenRow: m.cursor - m.offset}
	if m.context != nil {
		p.context = m.context.path
		p.ctxText = strings.Join(m.context.lines, "\n")
		p.ctxLine = m.cursor
	}
	if len(m.files) == 0 {
		return p
	}
	f := m.files[m.fileIdx]
	p.path = f.Path
	if m.context != nil || len(f.Lines) == 0 {
		return p
	}
	r := m.rows()[m.cursor]
	p.hasLine = true
	p.line = f.Lines[m.cursorLine()]
	p.side = sideContent(f, p.line.New == 0)
	p.onLine = r.fold < 0 && r.header == "" && !r.collapsed && r.meta == ""
	return p
}

// restore puts the reviewer back to the place after a reload: the same
// file (also when it was renamed) and the cursor on the same code, found
// like a comment anchor, at the same height on screen. The jumplist
// starts over.
func (m *Model) restore(p place) {
	m.fileIdx = 0
	if fi := m.fileIdxAfterReload(p.path); fi >= 0 {
		m.fileIdx = fi
	} else if len(m.files) > 0 {
		m.fileIdx = min(p.file, len(m.files)-1)
	}
	m.resetFile()
	m.jumps, m.jumpPos = nil, 0
	if len(m.files) > 0 {
		m.jumps = []jumpEntry{{file: m.fileIdx}}
	}
	if p.context != "" {
		m.restoreContext(p)
		return
	}
	if !p.hasLine || len(m.files) == 0 || len(m.files[m.fileIdx].Lines) == 0 {
		return
	}
	f := m.files[m.fileIdx]
	old := p.line.New == 0
	num := p.line.New
	if old {
		num = p.line.Old
	}
	n, _ := review.Locate(p.side, sideContent(f, old), p.line.Text, num)
	idx := max(0, m.lineIndex(f, n, old))
	if fi := m.foldOfLine(idx); fi >= 0 && p.onLine {
		m.foldsOpen[fi] = true
	}
	m.invalidateRows()
	m.cursor = m.rowOfLine(idx)
	m.offset = max(0, m.cursor-p.screenRow)
	m.jumps[0].line = idx
	m.scrollCursorIntoView()
}

// restoreContext opens the context file again in its new state, with
// the cursor on the same code.
func (m *Model) restoreContext(p place) {
	if m.opts.Search == nil {
		m.context = nil
		return
	}
	content, err := m.opts.Search.Read(p.context)
	if err != nil {
		m.context = nil
		m.fail(err.Error())
		return
	}
	before := strings.Split(p.ctxText, "\n")
	text := ""
	if p.ctxLine < len(before) {
		text = before[p.ctxLine]
	}
	n, _ := review.Locate(p.ctxText, content, text, p.ctxLine+1)
	if !m.showContext(p.context, n-1) {
		m.context = nil
		return
	}
	m.offset = max(0, m.cursor-p.screenRow)
	m.jumps = []jumpEntry{{file: -1, ctx: p.context, line: m.cursor}}
	m.scrollCursorIntoView()
}

// fileIdxAfterReload returns the index of the file that path became:
// the file with that path or the file renamed from it; -1 if none.
func (m Model) fileIdxAfterReload(path string) int {
	if fi, ok := m.pathIndex[path]; ok {
		return fi
	}
	for i, f := range m.files {
		if f.OldPath == path && path != "" {
			return i
		}
	}
	return -1
}

// sideContent returns the old or the new content of the file.
func sideContent(f diff.ReviewFile, old bool) string {
	o, n := f.Contents()
	if old {
		return o
	}
	return n
}

// relocationFiles converts review files for the anchor relocation of
// the review module.
func relocationFiles(files []diff.ReviewFile) []review.File {
	out := make([]review.File, len(files))
	for i, f := range files {
		old, new := f.Contents()
		out[i] = review.File{Path: f.Path, OldPath: f.OldPath, Old: old, New: new}
	}
	return out
}

// orphanCount is the number of orphaned comments in the review.
func (m Model) orphanCount() int {
	n := 0
	for _, c := range m.state.Comments() {
		if c.Orphaned {
			n++
		}
	}
	return n
}
