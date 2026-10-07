package ui

import (
	"slices"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// openPicker opens the fuzzy file picker with an empty query.
func (m *Model) openPicker() {
	m.pickerAll = false
	m.pickerFiles = nil
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
		m.pickerAll = false
		m.pickerFiles = nil
	case "enter":
		if len(m.pickerMatches) > 0 {
			m.pickFile(m.pickerMatches[min(m.pickerSel, len(m.pickerMatches)-1)])
		}
		m.mode = modeNormal
		m.pickerAll = false
		m.pickerFiles = nil
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

// updatePickerMatches re-runs the fuzzy query over the picker's file
// list and resets the selection.
func (m *Model) updatePickerMatches() {
	paths := m.pickerFiles
	if !m.pickerAll {
		paths = make([]string, len(m.files))
		for i, f := range m.files {
			paths[i] = f.Path
		}
	}
	type scored struct {
		file, score int
	}
	var list []scored
	for i, path := range paths {
		if score, ok := fuzzyScore(m.pickerQuery, path); ok {
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

// pickerPath is the path of the picker entry.
func (m Model) pickerPath(i int) string {
	if m.pickerAll {
		return m.pickerFiles[i]
	}
	return m.files[i].Path
}

// pickerBox is the fuzzy picker: the query and the matching files in a
// rounded box. The all-files picker marks the files of the review.
func (m Model) pickerBox() string {
	lines := []string{m.theme.Styles.Status.Render("Pick file: ") + m.pickerQuery}
	for i, fi := range m.pickerMatches {
		prefix := "  "
		if i == m.pickerSel {
			prefix = "> "
		}
		path := m.pickerPath(fi)
		switch {
		case !m.pickerAll && m.state.Read(path):
			path += "  ✓"
		case m.pickerAll:
			if _, ok := m.pathIndex[path]; ok {
				path += "  ●"
			}
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
