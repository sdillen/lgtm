package ui

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// cmdlineKey edits the command line: text is inserted, <bs> deletes (and
// leaves an empty line), <esc> cancels, <cr> runs the command.
func (m Model) cmdlineKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
	case "enter":
		m.mode = modeNormal
		return m.execute(m.cmdline)
	case "backspace":
		if m.cmdline == "" {
			m.mode = modeNormal
			break
		}
		_, size := utf8.DecodeLastRuneInString(m.cmdline)
		m.cmdline = m.cmdline[:len(m.cmdline)-size]
	default:
		if msg.Text != "" {
			m.cmdline += msg.Text
		}
	}
	return m, nil
}

// execute runs a command line: :q, :theme [name] and :set [option...].
func (m Model) execute(line string) (Model, tea.Cmd) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return m, nil
	}
	switch name, args := fields[0], fields[1:]; name {
	case "q", "q!", "quit", "quit!":
		return m, tea.Quit
	case "theme":
		m.setTheme(args)
	case "set":
		m.set(args)
	default:
		m.fail("unknown command: " + name)
	}
	return m, nil
}

// setTheme switches to the named theme, or names the current one.
func (m *Model) setTheme(args []string) {
	switch {
	case len(args) == 0:
		m.message = "theme: " + m.theme.Name
	case len(args) > 1:
		m.fail("usage: :theme [name]")
	case m.opts.LoadTheme == nil:
		m.fail("theme switching is not available")
	default:
		th, err := m.opts.LoadTheme(args[0])
		if err != nil {
			m.fail(err.Error())
			return
		}
		m.theme = th
		if len(m.files) > 0 {
			m.highlights, m.dimmed = highlightFile(m.theme.Highlighter, m.files[m.fileIdx])
		}
	}
}

// set changes or reports options like vim's :set. "name" switches an
// option on, "noname" off, "name!" and "invname" toggle it, "name?"
// reports it; without arguments all options are reported.
func (m *Model) set(args []string) {
	if len(args) == 0 {
		m.message = m.optionState()
		return
	}
	var report []string
	for _, arg := range args {
		name, op := arg, "on"
		switch {
		case strings.HasSuffix(arg, "?"):
			name, op = strings.TrimSuffix(arg, "?"), "query"
		case strings.HasSuffix(arg, "!"):
			name, op = strings.TrimSuffix(arg, "!"), "toggle"
		case strings.HasPrefix(arg, "inv"):
			name, op = strings.TrimPrefix(arg, "inv"), "toggle"
		case strings.HasPrefix(arg, "no"):
			name, op = strings.TrimPrefix(arg, "no"), "off"
		}
		if name != "unified" {
			m.fail("unknown option: " + arg)
			return
		}
		switch op {
		case "on":
			m.setUnified(true)
		case "off":
			m.setUnified(false)
		case "toggle":
			m.setUnified(!m.unified)
		case "query":
			report = append(report, m.optionState())
		}
	}
	m.message = strings.Join(report, "  ")
}

// optionState reports the options in vim style, e.g. "nounified".
func (m Model) optionState() string {
	if m.unified {
		return "unified"
	}
	return "nounified"
}

// fail shows an error message in the bottom line.
func (m *Model) fail(msg string) {
	m.message, m.messageErr = msg, true
}

// bottomLine is the command line while typing, otherwise the last command
// result; "" if there is neither.
func (m Model) bottomLine() string {
	switch {
	case m.mode == modeCmdline:
		return ":" + m.cmdline
	case m.messageErr:
		return m.theme.Styles.Deleted.Render(m.message)
	case m.message != "":
		return m.message
	}
	return ""
}
