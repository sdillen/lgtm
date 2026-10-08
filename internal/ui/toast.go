package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// defaultToastTTL is how long a toast stays in the top right corner
// before it decays.
const defaultToastTTL = 4 * time.Second

// toastMaxWidth caps the width of a toast box; longer text wraps.
const toastMaxWidth = 50

// toastTTL is the lifetime of the toast.
func (m Model) toastTTL() time.Duration {
	if m.opts.ToastTTL > 0 {
		return m.opts.ToastTTL
	}
	return defaultToastTTL
}

// toastTickInterval is how often the toast is checked for decay.
const toastTickInterval = 500 * time.Millisecond

// toastTick schedules the next decay check.
func toastTick() tea.Cmd {
	return tea.Tick(toastTickInterval, func(time.Time) tea.Msg { return toastTickMsg{} })
}

// Update handles the message and stamps a message that was shown by
// it, so the toast decays toastTTL later.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before := m.message
	next, cmd := m.update(msg)
	if nm, ok := next.(Model); ok && nm.message != "" {
		if _, key := msg.(tea.KeyPressMsg); key || nm.message != before {
			nm.messageAt = time.Now()
		}
		return nm, cmd
	}
	return next, cmd
}

// toastBox renders the current message as a framed box, "" if there is
// none.
func (m Model) toastBox() string {
	if m.message == "" || m.approved {
		return ""
	}
	inner := max(1, min(min(toastMaxWidth, m.width-boxChromeCols), lipgloss.Width(m.message)))
	style := m.theme.Styles.Status
	if m.messageErr {
		style = m.theme.Styles.Deleted
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(style.GetForeground()).
		Foreground(style.GetForeground()).
		Padding(0, 1).
		Width(inner + boxChromeCols).
		Render(m.message)
}

// withToast draws the toast over the top right corner of the view.
func (m Model) withToast(view string) string {
	box := m.toastBox()
	if box == "" || m.width < boxChromeCols+1 || m.height < 3 {
		return view
	}
	w, _ := lipgloss.Size(box)
	return lipgloss.NewCompositor(
		lipgloss.NewLayer(view),
		lipgloss.NewLayer(box).X(max(0, m.width-w)).Y(0).Z(2),
	).Render()
}
