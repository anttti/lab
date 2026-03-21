package tui

import (
	"time"

	"github.com/anttimattila/lab/internal/db"
	gosync "github.com/anttimattila/lab/internal/sync"
	tea "github.com/charmbracelet/bubbletea"
)

// view identifies which sub-model is currently active.
type view int

const (
	viewMRList view = iota
	viewMRDetail
	viewThread
	viewFilter
)

// syncTickMsg is sent on a repeating timer to trigger background sync.
type syncTickMsg struct{}

// bgSyncDoneMsg is sent when the background sync has finished.
type bgSyncDoneMsg struct{}

// Model is the root TUI model. It routes messages and rendering to the
// currently active sub-model.
type Model struct {
	db       *db.Database
	sync     *gosync.Engine
	current  view
	mrList   mrListModel
	mrDetail mrDetailModel
	thread   threadModel
	filter   filterModel
	width    int
	height   int
}

// NewModel creates a new root Model with the given DB and sync engine.
func NewModel(database *db.Database, engine *gosync.Engine) *Model {
	m := &Model{
		db:   database,
		sync: engine,
	}
	m.mrList = newMRListModel(m)
	return m
}

// syncTick returns a command that fires a syncTickMsg after 5 minutes.
func syncTick() tea.Cmd {
	return tea.Tick(5*time.Minute, func(time.Time) tea.Msg {
		return syncTickMsg{}
	})
}

// Init starts the initial data load and the background sync ticker.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(m.mrList.loadMRs(), syncTick())
}

// Update routes messages to the currently active sub-model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case syncTickMsg:
		return m, tea.Batch(
			func() tea.Msg {
				m.sync.SyncAll()
				return bgSyncDoneMsg{}
			},
			syncTick(),
		)

	case bgSyncDoneMsg:
		if m.current == viewMRList {
			return m, m.mrList.loadMRs()
		}
		return m, nil
	}

	switch m.current {
	case viewMRList:
		return m.mrList.update(msg, m)
	case viewMRDetail:
		return m.mrDetail.update(msg, m)
	case viewThread:
		return m.thread.update(msg, m)
	case viewFilter:
		return m.filter.update(msg, m)
	}
	return m, nil
}

// View delegates rendering to the currently active sub-model.
func (m *Model) View() string {
	switch m.current {
	case viewMRList:
		return m.mrList.view(m)
	case viewMRDetail:
		return m.mrDetail.view(m)
	case viewThread:
		return m.thread.view(m)
	case viewFilter:
		return m.filter.view(m)
	}
	return ""
}
