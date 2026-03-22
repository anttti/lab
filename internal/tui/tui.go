package tui

import (
	"io"

	"lab/internal/db"
	gosync "lab/internal/sync"
	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI application with an alternate screen buffer.
func Run(database *db.Database, engine *gosync.Engine) error {
	// Silence the sync engine's default stdout output inside the TUI.
	engine.SetOutput(io.Discard)
	model := NewModel(database, engine)
	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
