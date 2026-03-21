package tui

import (
	"fmt"
	"strings"

	"lab/internal/db"
	gosync "lab/internal/sync"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
)

// threadsLoadedMsg carries threads loaded from the DB.
type threadsLoadedMsg struct {
	threads []db.Thread
	err     error
}

// syncDoneMsg signals that an MR sync has completed.
type syncDoneMsg struct {
	err error
}

// mrDetailModel shows threads for a specific MR.
type mrDetailModel struct {
	db       *db.Database
	sync     *gosync.Engine
	mr       db.MergeRequest
	repoName string
	threads  []db.Thread
	cursor   int
	syncing  bool
}

func newMRDetailModel(root *Model, mr db.MergeRequest, repoName string) mrDetailModel {
	return mrDetailModel{
		db:       root.db,
		sync:     root.sync,
		mr:       mr,
		repoName: repoName,
	}
}

// loadThreads loads threads from DB and sorts unresolved first.
func (m *mrDetailModel) loadThreads() tea.Cmd {
	mrID := m.mr.ID
	database := m.db
	return func() tea.Msg {
		threads, err := database.ListThreads(mrID)
		if err != nil {
			return threadsLoadedMsg{err: err}
		}
		// Sort: unresolved first, resolved last.
		var unresolved, resolved []db.Thread
		for _, t := range threads {
			if t.Resolved {
				resolved = append(resolved, t)
			} else {
				unresolved = append(unresolved, t)
			}
		}
		sorted := append(unresolved, resolved...)
		return threadsLoadedMsg{threads: sorted}
	}
}

// syncMR syncs discussions for this specific MR.
func (m *mrDetailModel) syncMR() tea.Cmd {
	engine := m.sync
	database := m.db
	mr := m.mr
	return func() tea.Msg {
		repo, err := database.GetRepo(mr.RepoID)
		if err != nil {
			return syncDoneMsg{err: err}
		}
		err = engine.SyncMR(repo, mr.IID)
		return syncDoneMsg{err: err}
	}
}

// update handles input for the MR detail view.
func (m *mrDetailModel) update(msg tea.Msg, root *Model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case threadsLoadedMsg:
		if msg.err == nil {
			m.threads = msg.threads
			if m.cursor >= len(m.threads) && len(m.threads) > 0 {
				m.cursor = len(m.threads) - 1
			}
		}
		return root, nil

	case syncDoneMsg:
		m.syncing = false
		return root, m.loadThreads()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Keys.Quit):
			return root, tea.Quit

		case key.Matches(msg, Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, Keys.Down):
			if m.cursor < len(m.threads)-1 {
				m.cursor++
			}

		case key.Matches(msg, Keys.Top):
			m.cursor = 0

		case key.Matches(msg, Keys.Bottom):
			if len(m.threads) > 0 {
				m.cursor = len(m.threads) - 1
			}

		case key.Matches(msg, Keys.Select):
			if len(m.threads) > 0 {
				tv := newThreadModel(root, m.threads, m.cursor, m.mr, m.repoName)
				root.thread = tv
				root.current = viewThread
			}

		case key.Matches(msg, Keys.Sync):
			if !m.syncing {
				m.syncing = true
				return root, m.syncMR()
			}

		case key.Matches(msg, Keys.Back):
			root.current = viewMRList
			return root, root.mrList.loadMRs()
		}
	}
	return root, nil
}

// view renders the MR detail screen.
func (m *mrDetailModel) view(root *Model) string {
	var sb strings.Builder

	// Title.
	header := fmt.Sprintf("%s  !%d  %s", m.repoName, m.mr.IID, m.mr.Title)
	sb.WriteString(titleStyle.Render(header))
	sb.WriteString("\n\n")

	if m.syncing {
		sb.WriteString(pipelineRunning.Render("Syncing..."))
		sb.WriteString("\n\n")
	}

	if len(m.threads) == 0 {
		sb.WriteString(dimStyle.Render("No threads found."))
		sb.WriteString("\n")
	} else {
		for i, thread := range m.threads {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			// File location label.
			location := "General"
			if thread.FilePath != nil {
				location = *thread.FilePath
				if thread.NewLine != nil {
					location = fmt.Sprintf("%s:%d", location, *thread.NewLine)
				} else if thread.OldLine != nil {
					location = fmt.Sprintf("%s:%d", location, *thread.OldLine)
				}
			}

			noteCount := len(thread.Comments)

			// Resolved indicator.
			var resolvedLabel string
			if thread.Resolved {
				resolvedLabel = resolvedStyle.Render(" ✓")
			} else {
				resolvedLabel = unresolvedStyle.Render(" !")
			}

			// First comment preview.
			preview := ""
			if len(thread.Comments) > 0 {
				preview = truncate(thread.Comments[0].Body, 50)
			}

			row := fmt.Sprintf("%-30s  %d notes%s  %s",
				truncate(location, 30),
				noteCount,
				resolvedLabel,
				dimStyle.Render(preview),
			)

			if i == m.cursor {
				sb.WriteString(selectedStyle.Render(cursor+row) + "\n")
			} else {
				sb.WriteString(cursor + row + "\n")
			}
		}
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("j/k: navigate  l/enter: view thread  r: sync  h/b: back  q: quit"))

	return sb.String()
}
