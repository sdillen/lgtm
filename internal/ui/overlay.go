package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/keymap"
)

// boxChrome is the rows and columns a box adds around its lines: the
// rounded border, the padding and the title row.
const (
	boxChromeRows = 3
	boxChromeCols = 4
)

// helpAction runs an action while the help is open: motions scroll it,
// help and quit close it. Everything else is ignored.
func (m Model) helpAction(a keymap.Action, count int) Model {
	n := max(1, count)
	switch a {
	case actDown:
		m.helpOffset += n
	case actUp:
		m.helpOffset -= n
	case actHalfDown:
		m.helpOffset += n * max(1, m.helpRows()/2)
	case actHalfUp:
		m.helpOffset -= n * max(1, m.helpRows()/2)
	case actTop:
		m.helpOffset = 0
	case actBottom:
		m.helpOffset = len(m.helpLines())
	case actHelp, actQuit:
		m.mode = modeNormal
	}
	m.clampHelp()
	return m
}

// helpRows is the number of help lines that fit into the box.
func (m Model) helpRows() int {
	return max(0, m.bodyHeight()-boxChromeRows)
}

// clampHelp keeps the help scrolled inside its lines.
func (m *Model) clampHelp() {
	m.helpOffset = max(0, min(m.helpOffset, len(m.helpLines())-m.helpRows()))
}

// commands are the command line commands, listed in the help after the
// key bindings.
var commands = []struct{ usage, desc string }{
	{":q", "quit"},
	{":theme [name]", "switch the theme, or show the current one"},
	{":set [no]unified", "set an option (name! toggles, name? shows)"},
	{":set [no]sidebar", "keep the file tree visible"},
}

// helpLines are all lines of the full help: every action with all its
// keys as currently bound, then the commands. They are padded to the
// widest line so the box keeps its size while scrolling.
func (m Model) helpLines() []string {
	entries := m.opts.Keymap.Help()
	var keys, descs []string
	for _, e := range entries {
		k := strings.Join(e.Keys, ", ")
		if k == "" {
			k = "–"
		}
		keys, descs = append(keys, k), append(descs, e.Desc)
	}
	for _, c := range commands {
		keys, descs = append(keys, c.usage), append(descs, c.desc)
	}
	lines := m.columns(keys, descs)
	w := 0
	for _, l := range lines {
		w = max(w, ansi.StringWidth(l))
	}
	for i, l := range lines {
		lines[i] = l + strings.Repeat(" ", w-ansi.StringWidth(l))
	}
	return lines
}

// helpBox renders the visible part of the full help.
func (m Model) helpBox() string {
	lines := m.helpLines()
	end := min(len(lines), m.helpOffset+m.helpRows())
	title := "Keys"
	if end-m.helpOffset < len(lines) {
		title = fmt.Sprintf("Keys  %d–%d of %d", m.helpOffset+1, end, len(lines))
	}
	return m.box(title, lines[m.helpOffset:end])
}

// whichKeyBox renders the follow-up keys of the typed prefix.
func (m Model) whichKeyBox() string {
	next := m.keys.Next()
	keys := make([]string, len(next))
	descs := make([]string, len(next))
	for i, e := range next {
		keys[i], descs[i] = e.Key, e.Desc
		if e.Group {
			descs[i] = "+" + e.Desc
		}
	}
	lines := m.columns(keys, descs)
	if rows := m.bodyHeight() - boxChromeRows; len(lines) > rows {
		lines = append(lines[:max(0, rows-1)], "…")
	}
	title := m.keys.Typed()
	if g := m.keys.Group(); g != "" {
		title += "  " + g
	}
	return m.box(title, lines)
}

// columns lays out keys and descriptions as two aligned columns.
func (m Model) columns(keys, descs []string) []string {
	w := 0
	for _, k := range keys {
		w = max(w, ansi.StringWidth(k))
	}
	lines := make([]string, len(keys))
	for i, k := range keys {
		pad := strings.Repeat(" ", w-ansi.StringWidth(k))
		lines[i] = m.theme.Styles.Status.Render(k) + pad + "  " + descs[i]
	}
	return lines
}

// box frames a title and lines in a rounded border, cut to the terminal
// width.
func (m Model) box(title string, lines []string) string {
	inner := max(0, m.width-boxChromeCols)
	body := make([]string, 0, len(lines)+1)
	body = append(body, ansi.Truncate(m.theme.Styles.Status.Bold(true).Render(title), inner, "…"))
	for _, l := range lines {
		body = append(body, ansi.Truncate(l, inner, "…"))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Styles.Status.GetForeground()).
		Padding(0, 1).
		Render(strings.Join(body, "\n"))
}

// withOverlay draws the open overlay over the rendered view: the help
// centred, which-key and the picker at the bottom right, the drawer at
// the top left.
func (m Model) withOverlay(view string) string {
	if m.bodyHeight() < boxChromeRows+1 || m.width < boxChromeCols+1 {
		return view
	}
	var box string
	var x, y int
	switch {
	case m.mode == modeHelp:
		box = m.helpBox()
		w, h := lipgloss.Size(box)
		x, y = max(0, (m.width-w)/2), max(0, (m.bodyHeight()-h)/2)
	case m.mode == modeDrawer:
		box = m.drawerBox()
		_, h := lipgloss.Size(box)
		x, y = 0, max(0, (m.bodyHeight()-h)/2)
	case m.mode == modePicker:
		box = m.pickerBox()
		w, h := lipgloss.Size(box)
		x, y = max(0, (m.width-w)/2), max(0, (m.bodyHeight()-h)/2)
	case m.mode == modeComment:
		box = m.commentBox()
		x, y = m.commentBoxPosition(box)
	case m.whichKey:
		box = m.whichKeyBox()
		w, h := lipgloss.Size(box)
		x, y = max(0, m.width-w), max(0, m.bodyHeight()-h)
	default:
		return view
	}
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(view),
		lipgloss.NewLayer(box).X(x).Y(y).Z(1),
	).Render()
}
