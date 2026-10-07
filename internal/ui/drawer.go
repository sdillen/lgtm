package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// openDrawer opens the file drawer on the current file, sliding in
// from the top edge.
func (m *Model) openDrawer() {
	m.mode = modeDrawer
	m.drawerCursor = m.rowOfFile(m.fileIdx)
	m.startSlideOverlay(-m.bodyHeight())
}

// drawerKey handles keys while the drawer is open: j/k move the
// selection over the files, enter jumps to the selected file, esc
// closes the drawer.
func (m Model) drawerKey(msg tea.KeyPressMsg) Model {
	switch msg.String() {
	case "j", "down":
		m.moveDrawer(1)
	case "k", "up":
		m.moveDrawer(-1)
	case "enter":
		m.jumpToDrawerFile()
	case "esc":
		m.mode = modeNormal
	}
	return m
}

// moveDrawer moves the drawer selection to the next (dir 1) or previous
// (dir -1) file, skipping directory headings.
func (m *Model) moveDrawer(dir int) {
	for m.drawerCursor+dir >= 0 && m.drawerCursor+dir < len(m.tree) {
		m.drawerCursor += dir
		if !m.tree[m.drawerCursor].dir {
			return
		}
	}
}

// jumpToDrawerFile opens the file the drawer selection points at. The
// drawer stays open so the reviewer can browse.
func (m *Model) jumpToDrawerFile() {
	r := m.tree[m.drawerCursor]
	if r.dir || (r.file == m.fileIdx && m.context == nil) {
		return
	}
	m.jumpToFile(r.file, 0)
}

// rowOfFile returns the tree row showing the file, -1 if there is none.
func (m Model) rowOfFile(fi int) int {
	for i, r := range m.tree {
		if !r.dir && r.file == fi {
			return i
		}
	}
	return -1
}

func (m Model) treeLines(highlight int) []string {
	lines := make([]string, len(m.tree))
	for i, r := range m.tree {
		style := lipgloss.Style{}
		text := strings.Repeat("  ", r.indent) + r.name
		if r.dir {
			style = m.theme.Styles.Fold
			text += "/"
		} else {
			if m.state.Read(m.files[r.file].Path) {
				text += "  " + m.theme.Styles.Status.Render("✓")
			}
			if m.hasComments(m.files[r.file].Path) {
				text += "  " + m.theme.Styles.Status.Render("●")
			}
			if m.sinceRound[m.files[r.file].Path] {
				text += "  " + m.theme.Styles.Status.Render("Δ")
			}
		}
		if i == highlight {
			style = style.Background(m.theme.Styles.Cursor)
		}
		lines[i] = style.Render(text)
	}
	return lines
}

// hasComments reports whether the file carries at least one comment.
func (m Model) hasComments(path string) bool {
	for _, c := range m.state.Comments() {
		if c.Path == path {
			return true
		}
	}
	return false
}

// drawerBox is the file drawer: the tree in a rounded box, cut to the
// terminal height with a marker when the tree is taller.
func (m Model) drawerBox() string {
	lines := m.treeLines(m.drawerCursor)
	return m.box("Files", m.cutTree(lines))
}

// cutTree cuts tree lines that do not fit above the status line,
// marking the cut like the other overlays.
func (m Model) cutTree(lines []string) []string {
	rows := max(1, m.bodyHeight()-boxChromeRows)
	if len(lines) <= rows {
		return lines
	}
	return append(lines[:max(0, rows-1)], "…")
}

// sidebarWidth is the width of the permanent sidebar: the widest tree
// line plus the box chrome, capped so the file view keeps room.
func (m Model) sidebarWidth() int {
	inner := 0
	for _, l := range m.treeLines(-1) {
		inner = max(inner, ansi.StringWidth(l))
	}
	return min(40, max(14, inner+boxChromeCols))
}

// sidebarBox is the permanent sidebar: the tree in a rounded box, as
// tall as the file area, with the current file highlighted.
func (m Model) sidebarBox() string {
	lines := m.treeLines(m.rowOfFile(m.fileIdx))
	for len(lines) < m.bodyHeight() {
		lines = append(lines, "")
	}
	return m.box("Files", lines)
}
