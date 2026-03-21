package tui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("170")).
			Bold(true)

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

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
)
