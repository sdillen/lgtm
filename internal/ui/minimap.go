package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
)

// minimapOn reports whether the minimap strip is shown: one column at
// the right edge of the file area, whenever the user wants it. Only a
// terminal too narrow to hold the column next to a code column hides
// it regardless.
func (m Model) minimapOn() bool {
	return m.minimap && m.contentWidth() >= 2
}

// minimapStrip is the minimap strip: one cell per row of the file
// area. Each cell covers a range of view rows of the whole file and
// carries the cursor background when the visible window intersects the
// range, otherwise the background of a change or a comment in its range
// — the position and the marks of the file at a glance. Files smaller than
// the area map one cell per row; in wrap mode the rows and the screen
// lines no longer align, so the mapping is approximate.
func (m Model) minimapStrip(total int, mark func(int) color.Color) []string {
	h := m.bodyHeight()
	cells := make([]string, h)
	if !m.minimapOn() || h == 0 {
		return cells
	}
	for i := range cells {
		r0, r1 := i, i+1
		if total > h {
			r0 = i * total / h
			r1 = (i + 1) * total / h
		}
		if r0 >= total {
			break
		}
		var bg color.Color
		for j := r0; j < r1 && j < total; j++ {
			if c := mark(j); c != nil {
				bg = c
			}
		}
		// The visible window wins over the marks, so a large change
		// block cannot hide the position. A window that covers the
		// whole file marks nothing in particular, so the marks stay.
		if total > h && m.offset < r1 && m.offset+h > r0 {
			bg = m.theme.Styles.Cursor
		}
		if bg == nil && m.offset < r1 && m.offset+h > r0 {
			bg = m.theme.Styles.Cursor
		}
		if bg != nil {
			cells[i] = lipgloss.NewStyle().Background(bg).Render(" ")
		}
	}
	return cells
}

// minimapMarks returns the background of one view row in the minimap: the
// comment category colour of a comment row, the colour of the +/−
// marker for a changed line (stronger than the line wash, so the mark
// stands out in the narrow strip), nothing for the rest. The marker
// colours are looked up once, not per row.
func (m Model) minimapMarks(rows []row) func(int) color.Color {
	lines := m.files[m.fileIdx].Lines
	added := m.theme.Styles.Added.GetForeground()
	deleted := m.theme.Styles.Deleted.GetForeground()
	return func(j int) color.Color {
		r := rows[j]
		if r.note != nil {
			return m.categoryStyle(r.note.category).GetForeground()
		}
		if r.line < 0 || r.line >= len(lines) {
			return nil
		}
		switch lines[r.line].Kind {
		case diff.Added:
			return added
		case diff.Deleted:
			return deleted
		}
		return nil
	}
}

// minimapCell is the strip cell of one screen row, keeping the column
// width of the strip.
func (m Model) minimapCell(strip []string, i int) string {
	if !m.minimapOn() {
		return ""
	}
	if i < len(strip) && strip[i] != "" {
		return strip[i]
	}
	return " "
}
