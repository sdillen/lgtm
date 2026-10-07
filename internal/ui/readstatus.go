package ui

// toggleRead flips the read status of the current file manually. The
// automatic marking only fires when the file end becomes visible, so
// this toggle is not undone by the same key press.
func (m *Model) toggleRead() {
	f := m.files[m.fileIdx]
	if m.state.Read(f.Path) {
		m.state.SetUnread(f.Path)
	} else {
		m.state.SetRead(f.Path, f.NewContent())
	}
	m.saveState()
}

// maybeMarkRead marks the current file as read when its end became
// visible: the last row of the view is on screen and was not before.
// The manual toggle does not move the view, so a manually unread file
// stays unread until the reviewer scrolls away and back.
func (m *Model) maybeMarkRead() {
	visible := false
	if len(m.files) > 0 {
		visible = m.offset+m.bodyHeight() >= len(m.rows())
	}
	if visible && !m.endVisible {
		m.markRead()
	}
	m.endVisible = visible
}

// markRead sets the read status of the current file.
func (m *Model) markRead() {
	f := m.files[m.fileIdx]
	if m.state.Read(f.Path) {
		return
	}
	m.state.SetRead(f.Path, f.NewContent())
	m.saveState()
}

// saveState persists the review state. Without a directory the state
// stays in memory; failures are reported in the bottom line.
func (m *Model) saveState() {
	if m.dir == "" {
		return
	}
	if err := m.state.Save(m.dir); err != nil {
		m.fail("state: " + err.Error())
	}
}
