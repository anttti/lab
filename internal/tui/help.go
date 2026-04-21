package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpSection is one titled block of key/description rows.
type helpSection struct {
	title string
	rows  [][2]string // {keys, description}
}

var (
	helpKeyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	helpDescStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	helpHintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
)

// globalHelpSection is shown in every view.
var globalHelpSection = helpSection{"Global", [][2]string{
	{"?", "this help"},
	{"q / ctrl-c", "quit"},
}}

// helpSectionsForView returns the shortcut sections relevant to the given view.
func helpSectionsForView(v view) []helpSection {
	switch v {
	case viewMRList:
		return []helpSection{
			{"Navigation", [][2]string{
				{"j / ↓", "down"},
				{"k / ↑", "up"},
				{"g", "top"},
				{"G", "bottom"},
				{"l / enter / →", "open MR"},
			}},
			{"Filters", [][2]string{
				{"r", "repo filter"},
				{"a", "author filter"},
				{"v", "reviewer filter (incl. unassigned)"},
				{"L", "labels filter"},
				{"d", "cycle draft filter"},
				{"c", "cycle accepted filter"},
				{"u", "toggle unread only"},
				{"tab / shift+tab", "cycle repo"},
				{"] / [", "cycle author"},
				{"!", "negate author filter"},
			}},
			{"Presets", [][2]string{
				{"s", "save preset (then 1-9)"},
				{"1-9", "recall preset slot"},
			}},
			{"Actions", [][2]string{
				{"R", "sync all"},
			}},
			globalHelpSection,
		}

	case viewMRDetail:
		return []helpSection{
			{"Navigation", [][2]string{
				{"j / ↓", "down"},
				{"k / ↑", "up"},
				{"g", "top"},
				{"G", "bottom"},
				{"l / enter / →", "open thread"},
				{"h / b / ← / esc", "back to MR list"},
			}},
			{"Actions", [][2]string{
				{"w", "open in web browser"},
				{"R", "sync this MR"},
			}},
			globalHelpSection,
		}

	case viewThread:
		return []helpSection{
			{"Scroll", [][2]string{
				{"j / ↓", "scroll down"},
				{"k / ↑", "scroll up"},
			}},
			{"Thread", [][2]string{
				{"n", "next thread"},
				{"p", "previous thread"},
				{"h / b / ← / esc", "back to MR detail"},
			}},
			{"Actions", [][2]string{
				{"c", "send thread to claude"},
				{"w", "open comment in web"},
			}},
			globalHelpSection,
		}
	}
	return []helpSection{globalHelpSection}
}

// renderHelp builds the body of the help popup (without outer border) for the
// given view.
func renderHelp(v view) string {
	sections := helpSectionsForView(v)

	keyW := 0
	for _, sec := range sections {
		for _, r := range sec.rows {
			if w := lipgloss.Width(r[0]); w > keyW {
				keyW = w
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Keyboard shortcuts"))
	sb.WriteString(helpHintStyle.Render(" — " + viewName(v)))
	sb.WriteString("\n")

	for _, sec := range sections {
		sb.WriteString("\n")
		sb.WriteString(selectedStyle.Render(sec.title))
		sb.WriteString("\n")
		for _, r := range sec.rows {
			pad := keyW - lipgloss.Width(r[0])
			if pad < 0 {
				pad = 0
			}
			sb.WriteString("  ")
			sb.WriteString(helpKeyStyle.Render(r[0]))
			sb.WriteString(strings.Repeat(" ", pad))
			sb.WriteString("  ")
			sb.WriteString(helpDescStyle.Render(r[1]))
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n")
	sb.WriteString(helpHintStyle.Render("press esc or q to close"))
	return sb.String()
}

// viewName returns a human-readable label for a view.
func viewName(v view) string {
	switch v {
	case viewMRList:
		return "MR list"
	case viewMRDetail:
		return "MR detail"
	case viewThread:
		return "Thread"
	}
	return ""
}

// overlayHelp renders the help popup centered within the terminal for the
// given view.
func overlayHelp(v view, width, height int) string {
	popup := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("170")).
		Padding(1, 2).
		Render(renderHelp(v))

	if width < 4 {
		width = 4
	}
	if height < 4 {
		height = 4
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, popup)
}
