package ui

import (
	"fmt"
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
	m.drawerTop = 0
	m.scrollDrawerToCursor()
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
			break
		}
	}
	m.scrollDrawerToCursor()
}

// drawerRows is the number of tree rows that fit into the drawer.
func (m Model) drawerRows() int {
	return max(1, m.bodyHeight()-boxChromeRows)
}

// scrollDrawerToCursor moves the drawer window the least needed to show
// the selected row.
func (m *Model) scrollDrawerToCursor() {
	rows := m.drawerRows()
	if m.drawerCursor < m.drawerTop {
		m.drawerTop = m.drawerCursor
	} else if m.drawerCursor >= m.drawerTop+rows {
		m.drawerTop = m.drawerCursor - rows + 1
	}
	m.drawerTop = max(0, min(m.drawerTop, len(m.tree)-rows))
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

// treeLines renders the file tree rows. highlight is the row shown with
// the cursor background, -1 for none. Read files carry a checkmark,
// commented files a dot, files changed since the previous round a Δ;
// with the icons option the status icons of the changes come first.
func (m Model) treeLines(highlight int) []string {
	lines := make([]string, len(m.tree))
	for i, r := range m.tree {
		style := lipgloss.Style{}
		text := strings.Repeat("  ", r.indent) + r.name
		if r.dir {
			style = m.theme.Styles.Fold
			text += "/"
		} else {
			if m.opts.Icons {
				text = m.fileIcon(r.file) + " " + text
			}
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

// fileIcon is the Nerd Font icon of a file row: a plus for added
// files, a minus for deleted ones, an exchange arrow for renames and a
// pencil for modified files. Only shown with the icons option on; the
// glyphs are from the classic Font Awesome set of Nerd Fonts.
func (m Model) fileIcon(fi int) string {
	f := m.files[fi]
	style := m.theme.Styles.Fold
	var icon string
	switch {
	case f.Added():
		icon, style = "\uf055", m.theme.Styles.Added // nf-fa-plus_circle
	case f.Deleted():
		icon, style = "\uf056", m.theme.Styles.Deleted // nf-fa-minus_circle
	case f.OldPath != "":
		icon = "\uf0ec" // nf-fa-exchange: renamed
	default:
		icon = "\uf040" // nf-fa-pencil: modified
	}
	return style.Render(icon)
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

// drawerBox is the file drawer: the tree in a rounded box, scrolled
// with the selection when the tree is taller than the terminal; the
// title then shows the visible range.
func (m Model) drawerBox() string {
	lines := m.treeLines(m.drawerCursor)
	rows := m.drawerRows()
	m.scrollDrawerToCursor()
	if len(lines) <= rows {
		return m.box("Files", lines)
	}
	end := min(m.drawerTop+rows, len(lines))
	title := fmt.Sprintf("Files  %d–%d of %d", m.drawerTop+1, end, len(lines))
	return m.box(title, lines[m.drawerTop:end])
}

// sidebarWidth is the width of the permanent sidebar: the widest tree
// line plus the box chrome, capped so the file view keeps room.
func (m Model) sidebarWidth() int {
	if m.sidebarW > 0 {
		return m.sidebarW
	}
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
