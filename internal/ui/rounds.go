package ui

import (
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/review"
)

// loadInterdiff resolves the changes since the previous round. inter
// stays nil without a previous round; an empty interdiff is non-nil.
func (m *Model) loadInterdiff() {
	m.inter, m.sinceRound = nil, nil
	if m.opts.Interdiff == nil {
		return
	}
	inter, err := m.opts.Interdiff()
	if err != nil {
		m.fail("interdiff: " + err.Error())
		return
	}
	m.inter = make([]diff.ReviewFile, len(inter))
	m.sinceRound = make(map[string]bool, len(inter))
	for i, f := range inter {
		m.inter[i] = f.WithIgnoreWS(m.ignoreWS)
		m.sinceRound[f.Path] = true
	}
}

// toggleInterdiff switches between the whole review and the changes
// since the previous round, staying on the current file when the other
// view has it.
func (m *Model) toggleInterdiff() {
	if m.inter == nil {
		m.fail("keine Vorrunde – kein Interdiff")
		return
	}
	path := ""
	if len(m.files) > 0 {
		path = m.files[m.fileIdx].Path
	}
	m.interdiff = !m.interdiff
	m.showFiles(path)
}

// showFiles shows the files of the current view, the interdiff or the
// whole review, on the file with path or the first file. Files keep
// their collapse state.
func (m *Model) showFiles(path string) {
	files := m.full
	if m.interdiff {
		files = m.inter
	}
	collapsed := make(map[string]bool, len(m.files))
	for i, f := range m.files {
		collapsed[f.Path] = m.fileCollapsed[i]
	}
	m.setFiles(files, collapsed)
	m.fileIdx = 0
	if fi, ok := m.pathIndex[path]; ok {
		m.fileIdx = fi
	}
	m.resetFile()
	m.jumps, m.jumpPos = nil, 0
	if len(m.files) > 0 {
		m.jumps = []jumpEntry{{file: m.fileIdx}}
	}
}

// savePosition remembers where the reviewer leaves the review.
func (m *Model) savePosition() {
	if len(m.files) == 0 {
		return
	}
	f := m.files[m.fileIdx]
	p := review.Position{Interdiff: m.interdiff, Path: f.Path, Row: m.cursor - m.offset}
	if len(f.Lines) > 0 {
		l := f.Lines[m.cursorLine()]
		p.Line = l.New
		if l.New == 0 {
			p.Line, p.Old = l.Old, true
		}
	}
	m.state.SetPosition(p)
	m.saveState()
}

// resumePosition puts the cursor where the review was left: the same
// view, file and line at the same screen row. It needs the terminal
// size, so it waits for the first one.
func (m *Model) resumePosition() {
	p := m.state.Position()
	if p.Interdiff && m.inter != nil && !m.interdiff {
		m.interdiff = true
		m.showFiles(p.Path)
	}
	fi, ok := m.pathIndex[p.Path]
	if p.Path == "" || !ok {
		return
	}
	if fi != m.fileIdx {
		m.fileIdx = fi
		m.resetFile()
		m.jumps = []jumpEntry{{file: fi}}
	}
	if p.Line == 0 {
		return
	}
	idx := m.lineIndex(m.files[fi], p.Line, p.Old)
	if idx < 0 {
		return
	}
	m.cursor = m.rowOfLine(idx)
	m.offset = max(0, m.cursor-p.Row)
	m.jumps[0].line = idx
}
