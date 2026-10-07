// Package ui renders ReviewFile models in the terminal. It knows nothing
// about git or diffing.
package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"gitea.fit.fraunhofer.de/ste05096/lgtm/internal/diff"
)

var (
	addedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#9ece6a"))
	deletedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#f7768e"))
	gutterStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#565f89"))
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#7aa2f7"))
)

// Model is the full-file view of the first review file. label names the
// review subject and appears in the hint when there is nothing to review.
type Model struct {
	files         []diff.ReviewFile
	label         string
	width, height int
	offset        int // index of the first visible line
}

// New returns a Model showing files of the review subject label.
func New(files []diff.ReviewFile, label string) Model {
	return Model{files: files, label: label}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "j", "down":
			m.offset++
		case "k", "up":
			m.offset--
		}
	}
	m.offset = max(0, min(m.offset, len(m.lines())-m.bodyHeight()))
	return m, nil
}

func (m Model) View() tea.View {
	var content string
	if len(m.files) == 0 {
		content = m.emptyView()
	} else {
		content = m.fileView()
	}
	v := tea.NewView(content)
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
	lines := make([]string, len(hint))
	for i, line := range hint {
		lines[i] = ansi.Truncate(line, m.width, "")
	}
	return strings.Join(lines, "\n")
}

func (m Model) fileView() string {
	var b strings.Builder
	lines := m.lines()
	for i := range m.bodyHeight() {
		if j := m.offset + i; j < len(lines) {
			b.WriteString(ansi.Truncate(renderLine(lines[j]), m.width, ""))
		}
		b.WriteByte('\n')
	}
	b.WriteString(ansi.Truncate(statusStyle.Render(m.files[0].Path), m.width, ""))
	return b.String()
}

// lines are the lines of the shown file.
func (m Model) lines() []diff.Line {
	if len(m.files) == 0 {
		return nil
	}
	return m.files[0].Lines
}

// bodyHeight is the number of file lines that fit above the status line.
func (m Model) bodyHeight() int {
	return max(0, m.height-1)
}

func renderLine(l diff.Line) string {
	text := strings.ReplaceAll(l.Text, "\t", "    ")
	gutter := gutterStyle.Render(fmt.Sprintf("%4s %4s", lineNum(l.Old), lineNum(l.New)))
	switch l.Kind {
	case diff.Added:
		return gutter + addedStyle.Render(" + "+text)
	case diff.Deleted:
		return gutter + deletedStyle.Render(" - "+text)
	default:
		return gutter + "   " + text
	}
}

func lineNum(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}
