package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sdillen/lgtm/internal/keymap"
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
	{":q", "quit (warns with open comments)"},
	{":q!", "quit without the warning"},
	{":lgtm", "approve, clear the review and exit 0"},
	{":theme [name]", "switch the theme, or show the current one"},
	{":set [no]unified", "set an option (name! toggles, name? shows)"},
	{":set [no]sidebar", "keep the file tree visible"},
	{":set [no]minimap", "show the minimap strip"},
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

// resultsBox renders the gr/gd result list: the hits as
// "path:line  text", the selection marked.
func (m Model) resultsBox() string {
	lines := make([]string, len(m.results))
	for i, h := range m.results {
		prefix := "  "
		if i == m.resultSel {
			prefix = "> "
		}
		lines[i] = prefix + fmt.Sprintf("%s:%d  %s", h.Path, h.Line, h.Text)
	}
	rows := max(1, m.bodyHeight()-boxChromeRows-1)
	if len(lines) > rows+1 {
		lines = append(lines[:rows+1], "…")
	}
	return m.box(m.resultsTitle, lines)
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

// overlayBox is the box of the open overlay and its final position:
// the start screen, the approval moment and the help centred, the
// drawer at the top left, the comment float at the cursor, which-key
// and the picker at the bottom right. ok is false when no overlay is
// open.
func (m Model) overlayBox() (box string, x, y int, ok bool) {
	switch {
	case m.startScreen:
		box = m.startBox()
		w, h := lipgloss.Size(box)
		return box, max(0, (m.width-w)/2), max(0, (m.bodyHeight()-h)/2), true
	case m.approved:
		box = m.lgtmBox()
		w, h := lipgloss.Size(box)
		return box, max(0, (m.width-w)/2), max(0, (m.bodyHeight()-h)/2), true
	case m.mode == modeHelp:
		box = m.helpBox()
		w, h := lipgloss.Size(box)
		return box, max(0, (m.width-w)/2), max(0, (m.bodyHeight()-h)/2), true
	case m.mode == modeDrawer:
		box = m.drawerBox()
		_, h := lipgloss.Size(box)
		return box, 0, max(0, (m.bodyHeight()-h)/2), true
	case m.mode == modePicker:
		box = m.pickerBox()
		w, h := lipgloss.Size(box)
		return box, max(0, (m.width-w)/2), max(0, (m.bodyHeight()-h)/2), true
	case m.mode == modeResults:
		box = m.resultsBox()
		w, h := lipgloss.Size(box)
		return box, max(0, (m.width-w)/2), max(0, (m.bodyHeight()-h)/2), true
	case m.mode == modeComment:
		box = m.commentBox()
		x, y = m.commentBoxPosition(box)
		return box, x, y, true
	case m.whichKey:
		box = m.whichKeyBox()
		w, h := lipgloss.Size(box)
		return box, max(0, m.width-w), max(0, m.bodyHeight()-h), true
	}
	return "", 0, 0, false
}

// withOverlay draws the open overlay over the rendered view, dimming
// the view behind it: the start screen, the approval moment and the
// help centred, which-key and the picker at the bottom right, the
// drawer at the top left.
func (m Model) withOverlay(view string) string {
	if m.bodyHeight() < boxChromeRows+1 || m.width < boxChromeCols+1 {
		return m.withToast(view)
	}
	box, x, y, ok := m.overlayBox()
	if !ok {
		return m.withToast(view)
	}
	// The scrim darkens the view behind the overlay, so the box reads
	// as a layer above the code.
	view = dimView(view)
	return m.withToast(lipgloss.NewCompositor(
		lipgloss.NewLayer(view),
		lipgloss.NewLayer(box).X(x).Y(m.overlayY(y)).Z(1),
	).Render())
}

// dimSGR matches the truecolour SGR sequences of the rendered view.
var dimSGR = regexp.MustCompile(`\x1b\[(38|48);2;(\d+);(\d+);(\d+)m`)

// dimView dims every colour of the rendered view to 55%, the scrim
// behind an open overlay. The text and the layout stay untouched, so
// the box drawn over it keeps its full brightness. 256-colour and
// indexed sequences keep their colour: on terminals without truecolor
// the scrim is a no-op.
func dimView(view string) string {
	return dimSGR.ReplaceAllStringFunc(view, func(s string) string {
		g := dimSGR.FindStringSubmatch(s)
		r, _ := strconv.Atoi(g[2])
		gr, _ := strconv.Atoi(g[3])
		b, _ := strconv.Atoi(g[4])
		return fmt.Sprintf("\x1b[%s;2;%d;%d;%dm", g[1], r*55/100, gr*55/100, b*55/100)
	})
}
