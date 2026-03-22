package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("170")).
			Bold(true)

	// selectedRowStyle highlights the focused list row with a background color (Lazygit-style).
	selectedRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("24")).
				Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("248")).
			Background(lipgloss.Color("238")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("248"))

	pipelineSuccess = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	pipelineFailed = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))

	pipelineRunning = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	unresolvedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))

	resolvedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("42"))

	unreadStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	previewStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	borderColor = lipgloss.Color("246")
)

// selectedBg is the raw ANSI escape for the selection background color.
const selectedBg = "\x1b[48;5;24m"

// renderSelectedRow renders a row with the selection background spanning the
// full inner panel width. It replaces any ANSI full-reset sequences inside the
// row so that nested styled segments (colored indicators etc.) don't break the
// background.
func renderSelectedRow(row string, innerWidth int) string {
	w := lipgloss.Width(row)
	pad := innerWidth - w
	if pad > 0 {
		row += strings.Repeat(" ", pad)
	}
	// Re-apply background after every full SGR reset emitted by nested styles.
	row = strings.ReplaceAll(row, "\x1b[0m", "\x1b[0m"+selectedBg)
	return selectedBg + "\x1b[1m" + row + "\x1b[0m"
}

// renderPanel draws a bordered panel with an optional title and help bar,
// filling the given width and height. The content is placed inside the border
// and clipped/padded to fit exactly.
func renderPanel(title, content, help string, width, height int) string {
	if width < 4 {
		width = 4
	}
	if height < 4 {
		height = 4
	}

	// Inner dimensions (border takes 2 cols and 2 rows).
	innerW := width - 2
	innerH := height - 2

	// Reserve one line for the help bar below the panel.
	if help != "" {
		innerH--
	}
	if innerH < 1 {
		innerH = 1
	}

	// Pad or clip content lines to fill the panel body.
	lines := strings.Split(content, "\n")
	// Remove trailing empty line if present (common from trailing \n).
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for len(lines) < innerH {
		lines = append(lines, "")
	}
	if len(lines) > innerH {
		lines = lines[:innerH]
	}

	// Build the box manually for title support.
	bc := lipgloss.RoundedBorder()
	bStyle := lipgloss.NewStyle().Foreground(borderColor)

	// Top border with title.
	var top string
	if title != "" {
		titleStr := " " + title + " "
		remaining := innerW - 1 - lipgloss.Width(titleStr)
		if remaining < 0 {
			remaining = 0
		}
		top = bStyle.Render(bc.TopLeft+bc.Top) + titleStr + bStyle.Render(strings.Repeat(bc.Top, remaining)+bc.TopRight)
	} else {
		top = bStyle.Render(bc.TopLeft + strings.Repeat(bc.Top, innerW) + bc.TopRight)
	}

	// Content lines with side borders.
	var body strings.Builder
	body.WriteString(top)
	body.WriteString("\n")
	for _, line := range lines {
		// Pad line to inner width accounting for ANSI codes.
		lineWidth := lipgloss.Width(line)
		pad := innerW - lineWidth
		if pad < 0 {
			pad = 0
		}
		body.WriteString(bStyle.Render(bc.Left))
		body.WriteString(line)
		body.WriteString(strings.Repeat(" ", pad))
		body.WriteString(bStyle.Render(bc.Right))
		body.WriteString("\n")
	}

	// Bottom border.
	bottom := bStyle.Render(bc.BottomLeft + strings.Repeat(bc.Bottom, innerW) + bc.BottomRight)
	body.WriteString(bottom)

	if help != "" {
		body.WriteString("\n")
		body.WriteString(helpStyle.Render(" " + help))
	}

	return body.String()
}
