package tui

import (
	"strings"
	"time"

	"lab/internal/db"
	gosync "lab/internal/sync"
	tea "github.com/charmbracelet/bubbletea"
)

// view identifies which sub-model is currently active.
type view int

const (
	viewMRList view = iota
	viewMRDetail
	viewThread
)

// syncTickMsg is sent on a repeating timer to trigger background sync.
type syncTickMsg struct{}

// bgSyncDoneMsg is sent when the background sync has finished.
type bgSyncDoneMsg struct{}

// syncProgressMsg carries a progress update from the sync engine.
type syncProgressMsg string

// fgSyncDoneMsg is sent when a foreground (user-triggered) sync finishes.
type fgSyncDoneMsg struct{ err error }

// flashMsg carries a transient message to display in the help bar.
type flashMsg struct{ text string }

// clearFlashMsg signals that the flash message should be cleared.
type clearFlashMsg struct{}

// Model is the root TUI model. It routes messages and rendering to the
// currently active sub-model.
type Model struct {
	db           *db.Database
	sync         *gosync.Engine
	current      view
	mrList       mrListModel
	mrDetail     mrDetailModel
	thread       threadModel
	width        int
	height       int
	syncing      bool
	syncStatus   string
	syncProgress chan string
	showHelp     bool
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

	case tea.KeyMsg:
		if m.showHelp {
			switch msg.String() {
			case "esc", "q", "Q":
				m.showHelp = false
			}
			return m, nil
		}
		if msg.String() == "?" && !m.inTextInput() {
			m.showHelp = true
			return m, nil
		}

	case syncTickMsg:
		// Don't start background sync if a foreground sync is in progress.
		if m.syncing {
			return m, syncTick()
		}
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

	case syncProgressMsg:
		m.syncStatus = string(msg)
		return m, waitForProgress(m.syncProgress)

	case fgSyncDoneMsg:
		m.syncing = false
		m.syncStatus = ""
		m.syncProgress = nil
		return m, m.mrList.loadMRs()
	}

	switch m.current {
	case viewMRList:
		return m.mrList.update(msg, m)
	case viewMRDetail:
		return m.mrDetail.update(msg, m)
	case viewThread:
		return m.thread.update(msg, m)
	}
	return m, nil
}

// startSync begins a foreground sync and returns commands for both the sync
// worker and the progress listener.
func (m *Model) startSync() tea.Cmd {
	ch := make(chan string, 64)
	m.syncing = true
	m.syncStatus = "Starting sync..."
	m.syncProgress = ch
	w := &channelWriter{ch: ch}
	return tea.Batch(
		func() tea.Msg {
			m.sync.SyncAllWithWriter(w)
			close(ch)
			return fgSyncDoneMsg{}
		},
		waitForProgress(ch),
	)
}

// waitForProgress returns a command that reads the next progress message
// from the channel. When the channel is closed it returns a fgSyncDoneMsg
// so the model can clean up.
func waitForProgress(ch chan string) tea.Cmd {
	return func() tea.Msg {
		for {
			msg, ok := <-ch
			if !ok {
				return fgSyncDoneMsg{}
			}
			if msg != "" {
				return syncProgressMsg(msg)
			}
		}
	}
}

// channelWriter is an io.Writer that sends each Write as a string to a channel.
type channelWriter struct {
	ch chan string
}

func (w *channelWriter) Write(p []byte) (int, error) {
	s := strings.TrimSpace(string(p))
	if s != "" {
		select {
		case w.ch <- s:
		default:
			// Drop if channel full to avoid blocking sync.
		}
	}
	return len(p), nil
}

// View delegates rendering to the currently active sub-model.
func (m *Model) View() string {
	var base string
	switch m.current {
	case viewMRList:
		base = m.mrList.view(m)
	case viewMRDetail:
		base = m.mrDetail.view(m)
	case viewThread:
		base = m.thread.view(m)
	}
	if m.showHelp {
		return overlayHelp(m.current, m.width, m.height)
	}
	return base
}

// inTextInput reports whether any sub-model is currently accepting free-form
// text input, so global shortcuts (e.g. "?") shouldn't be hijacked.
func (m *Model) inTextInput() bool {
	return m.current == viewMRList && m.mrList.autocomplete != nil
}
