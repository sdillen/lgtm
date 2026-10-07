package ui

import (
	"slices"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// openPicker opens the fuzzy file picker with an empty query.
func (m *Model) openPicker() {
	m.mode = modePicker
	m.pickerQuery = ""
	m.pickerSel = 0
	m.pickerMatches = m.allFileIndexes()
}

// pickerKey handles keys while the picker is open: text edits the fuzzy
// query, ctrl+n/ctrl+p move the selection, enter jumps to the selected
// file, esc cancels.
func (m Model) pickerKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeNormal
	case "enter":
		if len(m.pickerMatches) > 0 {
			m.fileIdx = m.pickerMatches[min(m.pickerSel, len(m.pickerMatches)-1)]
			m.resetFile()
		}
		m.mode = modeNormal
	case "ctrl+n", "down":
		if len(m.pickerMatches) > 0 {
			m.pickerSel = min(m.pickerSel+1, len(m.pickerMatches)-1)
		}
	case "ctrl+p", "up":
		m.pickerSel = max(m.pickerSel-1, 0)
	case "backspace":
		if m.pickerQuery != "" {
			_, size := utf8.DecodeLastRuneInString(m.pickerQuery)
			m.pickerQuery = m.pickerQuery[:len(m.pickerQuery)-size]
			m.updatePickerMatches()
		}
	default:
		if msg.Text != "" {
			m.pickerQuery += msg.Text
			m.updatePickerMatches()
		}
	}
	return m, nil
}

// updatePickerMatches re-runs the fuzzy query and resets the selection.
func (m *Model) updatePickerMatches() {
	type scored struct {
		file, score int
	}
	var list []scored
	for i, f := range m.files {
		if score, ok := fuzzyScore(m.pickerQuery, f.Path); ok {
			list = append(list, scored{i, score})
		}
	}
	slices.SortStableFunc(list, func(a, b scored) int { return a.score - b.score })
	m.pickerMatches = make([]int, len(list))
	for i, s := range list {
		m.pickerMatches[i] = s.file
	}
	m.pickerSel = 0
}

// pickerBox is the fuzzy picker: the query and the matching files in a
// rounded box.
func (m Model) pickerBox() string {
	lines := []string{m.theme.Styles.Status.Render("Pick file: ") + m.pickerQuery}
	for i, fi := range m.pickerMatches {
		prefix := "  "
		if i == m.pickerSel {
			prefix = "> "
		}
		path := m.files[fi].Path
		if m.state.Read(path) {
			path += "  ✓"
		}
		lines = append(lines, prefix+path)
	}
	// Keep the query and the selection visible: cut matches that do
	// not fit and mark the cut.
	rows := max(1, m.bodyHeight()-boxChromeRows-1)
	if len(lines) > rows+1 {
		lines = append(lines[:rows+1], "…")
	}
	return m.box("Files", lines)
}
