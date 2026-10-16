package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"

	"github.com/sdillen/lgtm/internal/diff"
)

// minimapOn reports whether the minimap strip is shown: one column at
// the right edge of the file area, whenever the user wants it. Only a
// terminal too narrow to hold the column next to a code column hides
// it regardless.
func (m Model) minimapOn() bool {
	return m.minimap && m.contentWidth() >= 2
}

// minimapRange is the range [r0, r1) of view rows that strip cell i
// covers, for total rows in a strip of h cells. Files smaller than the
// strip map one cell per row.
func minimapRange(i, total, h int) (r0, r1 int) {
	if total > h {
		return i * total / h, (i + 1) * total / h
	}
	return i, i + 1
}

// minimapMarked returns per strip cell the last view row in its range
// that carries a mark, -1 for none: the row whose mark the cell shows.
// It visits every row of the file, so fileView takes it from the row
// cache; the colours are looked up per frame, so a theme change needs
// no rebuild.
func (m Model) minimapMarked(total int, mark func(int) color.Color) []int {
	h := m.bodyHeight()
	marked := make([]int, h)
	for i := range marked {
		marked[i] = -1
		r0, r1 := minimapRange(i, total, h)
		for j := r0; j < r1 && j < total; j++ {
			if mark(j) != nil {
				marked[i] = j
			}
		}
	}
	return marked
}

// cachedMinimapMarked is minimapMarked for the rows of the current file,
// kept in the row cache until the rows or the strip height change.
func (m Model) cachedMinimapMarked(total int, mark func(int) color.Color) []int {
	c := m.rowsCache
	if c == nil {
		return m.minimapMarked(total, mark)
	}
	if c.marked == nil || len(c.marked) != m.bodyHeight() {
		c.marked = m.minimapMarked(total, mark)
	}
	return c.marked
}

// minimapStrip is the minimap strip: one cell per row of the file
// area. Each cell covers a range of view rows of the whole file and
// carries the cursor background when the visible window intersects the
// range, otherwise the background of a change or a comment in its range
// — the position and the marks of the file at a glance. marked are the rows
// of the marks per cell (see minimapMarked), nil when nothing is
// marked. In wrap mode the rows and the screen lines no longer align,
// so the mapping is approximate.
func (m Model) minimapStrip(total int, marked []int, mark func(int) color.Color) []string {
	h := m.bodyHeight()
	cells := make([]string, h)
	if !m.minimapOn() || h == 0 {
		return cells
	}
	// The strip has a handful of colours; each cell of one colour is
	// rendered once.
	type rendered struct {
		key  [4]uint32
		cell string
	}
	var seen []rendered
	render := func(c color.Color) string {
		r, g, b, a := c.RGBA()
		key := [4]uint32{r, g, b, a}
		for _, s := range seen {
			if s.key == key {
				return s.cell
			}
		}
		cell := lipgloss.NewStyle().Background(c).Render(" ")
		seen = append(seen, rendered{key, cell})
		return cell
	}
	for i := range cells {
		r0, r1 := minimapRange(i, total, h)
		if r0 >= total {
			break
		}
		var bg color.Color
		// The visible window wins over the marks, so a large change
		// block cannot hide the position. A window that covers the
		// whole file marks nothing in particular, so the marks stay.
		if total > h && m.offset < r1 && m.offset+h > r0 {
			bg = m.theme.Styles.Cursor
		}
		if bg == nil && i < len(marked) && marked[i] >= 0 {
			bg = mark(marked[i])
		}
		if bg == nil && m.offset < r1 && m.offset+h > r0 {
			bg = m.theme.Styles.Cursor
		}
		if bg != nil {
			cells[i] = render(bg)
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
