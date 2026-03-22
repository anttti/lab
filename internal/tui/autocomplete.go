package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// autocompleteModel is a searchable dropdown for selecting from a list of options.
type autocompleteModel struct {
	options  []string
	filtered []string
	input    string
	cursor   int
}

// newAutocomplete creates an autocomplete with the cursor on the current value.
func newAutocomplete(options []string, current string) autocompleteModel {
	filtered := make([]string, len(options))
	copy(filtered, options)
	cursor := 0
	for i, opt := range options {
		if opt == current {
			cursor = i
			break
		}
	}
	return autocompleteModel{
		options:  options,
		filtered: filtered,
		cursor:   cursor,
	}
}

func (m *autocompleteModel) applyFilter() {
	if m.input == "" {
		m.filtered = make([]string, len(m.options))
		copy(m.filtered, m.options)
		m.cursor = 0
		return
	}
	lower := strings.ToLower(m.input)
	m.filtered = nil
	for _, opt := range m.options {
		if strings.Contains(strings.ToLower(opt), lower) {
			m.filtered = append(m.filtered, opt)
		}
	}
	if m.cursor >= len(m.filtered) {
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		} else {
			m.cursor = 0
		}
	}
}

func (m *autocompleteModel) selected() string {
	if m.cursor >= 0 && m.cursor < len(m.filtered) {
		return m.filtered[m.cursor]
	}
	return ""
}

// update handles key input. Returns (done, cancelled).
func (m *autocompleteModel) update(msg tea.KeyMsg) (bool, bool) {
	switch msg.String() {
	case "esc":
		return true, true
	case "enter":
		return len(m.filtered) > 0, false
	case "up", "ctrl+p":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "ctrl+n":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case "backspace":
		if len(m.input) > 0 {
			runes := []rune(m.input)
			m.input = string(runes[:len(runes)-1])
			m.applyFilter()
		}
	default:
		r := []rune(msg.String())
		if len(r) == 1 && r[0] >= 32 {
			m.input += msg.String()
			m.applyFilter()
		}
	}
	return false, false
}

func (m *autocompleteModel) view(width, maxRows int) string {
	var sb strings.Builder

	sb.WriteString(selectedStyle.Render("> " + m.input + "█"))
	sb.WriteString("\n")

	visible := maxRows - 1
	if visible < 1 {
		visible = 1
	}

	offset := 0
	if m.cursor >= visible {
		offset = m.cursor - visible + 1
	}
	end := offset + visible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := offset; i < end; i++ {
		if i == m.cursor {
			sb.WriteString(renderSelectedRow("  "+m.filtered[i], width))
		} else {
			sb.WriteString(dimStyle.Render("  " + m.filtered[i]))
		}
		sb.WriteString("\n")
	}

	if len(m.filtered) == 0 {
		sb.WriteString(dimStyle.Render("  No matches"))
		sb.WriteString("\n")
	}

	return sb.String()
}
