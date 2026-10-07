package ui

import (
	"strings"
	"time"
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

// execute runs a command line: :q, :q!, :lgtm, :theme [name] and :set
// [option...].
func (m Model) execute(line string) (Model, tea.Cmd) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return m, nil
	}
	m.quitArmed = false
	switch name, args := fields[0], fields[1:]; name {
	case "q", "quit":
		return m, m.quitAction()
	case "q!", "quit!":
		m.quitOpen = len(m.state.OpenComments()) > 0
		return m, tea.Quit
	case "lgtm":
		return m, m.approve()
	case "theme":
		m.setTheme(args)
	case "set":
		m.set(args)
	default:
		m.fail("unknown command: " + name)
	}
	return m, nil
}

// defaultApprovalDelay is how long :lgtm shows its confirmation before
// quitting.
const defaultApprovalDelay = 800 * time.Millisecond

// approve ends the review approved: a short confirmation moment in the
// status line, then quit with exit code 0.
func (m *Model) approve() tea.Cmd {
	m.approved = true
	m.message = "lgtm – approved"
	delay := m.opts.ApprovalDelay
	if delay == 0 {
		delay = defaultApprovalDelay
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return approveMsg{} })
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
			m.hl = newFileHighlights(m.theme.Highlighter, m.files[m.fileIdx])
		}
	}
}

// set changes or reports options like vim's :set. "name" switches an
// option on, "noname" off, "name!" and "invname" toggle it, "name?"
// reports it; without arguments all options are reported.
func (m *Model) set(args []string) {
	if len(args) == 0 {
		m.message = m.optionStates()
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
		switch name {
		case "unified":
			switch op {
			case "on":
				m.setUnified(true)
			case "off":
				m.setUnified(false)
			case "toggle":
				m.setUnified(!m.unified)
			case "query":
				report = append(report, m.optionState("unified"))
			}
		case "sidebar":
			switch op {
			case "on":
				m.sidebar = true
			case "off":
				m.sidebar = false
			case "toggle":
				m.sidebar = !m.sidebar
			case "query":
				report = append(report, m.optionState("sidebar"))
			}
		case "wrap":
			switch op {
			case "on":
				m.wrap = true
				m.hscroll = 0
			case "off":
				m.wrap = false
			case "toggle":
				m.wrap = !m.wrap
				if m.wrap {
					m.hscroll = 0
				}
			case "query":
				report = append(report, m.optionState("wrap"))
			}
		case "ignorews":
			switch op {
			case "on":
				m.setIgnoreWS(true)
			case "off":
				m.setIgnoreWS(false)
			case "toggle":
				m.setIgnoreWS(!m.ignoreWS)
			case "query":
				report = append(report, m.optionState("ignorews"))
			}
		default:
			m.fail("unknown option: " + arg)
			return
		}
	}
	m.message = strings.Join(report, "  ")
}

// setIgnoreWS recomputes every file's view with whitespace-only changes
// hidden or visible.
func (m *Model) setIgnoreWS(on bool) {
	if on == m.ignoreWS {
		return
	}
	m.ignoreWS = on
	for i := range m.files {
		m.files[i] = m.files[i].WithIgnoreWS(on)
	}
	if m.context == nil && len(m.files) > 0 {
		m.resetFile()
	}
}

// optionState reports one option in vim style, e.g. "nounified".
func (m Model) optionState(name string) string {
	on := false
	switch name {
	case "unified":
		on = m.unified
	case "sidebar":
		on = m.sidebar
	case "wrap":
		on = m.wrap
	case "ignorews":
		on = m.ignoreWS
	}
	if on {
		return name
	}
	return "no" + name
}

// optionStates reports all options in vim style, e.g. "nounified
// nosidebar nowrap noignorews".
func (m Model) optionStates() string {
	return m.optionState("unified") + " " + m.optionState("sidebar") +
		" " + m.optionState("wrap") + " " + m.optionState("ignorews")
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
	case m.mode == modeSearch:
		return "/" + m.searchQuery
	case m.messageErr:
		return m.theme.Styles.Deleted.Render(m.message)
	case m.message != "":
		return m.message
	}
	return ""
}
