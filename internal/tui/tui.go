package tui

import (
	"lab/internal/db"
	gosync "lab/internal/sync"
	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI application with an alternate screen buffer.
func Run(database *db.Database, engine *gosync.Engine) error {
	model := NewModel(database, engine)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
