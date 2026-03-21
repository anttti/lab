package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/anttimattila/lab/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
)

type threadState int

const (
	threadViewing threadState = iota
)

// threadModel shows the full content of a single thread.
type threadModel struct {
	db     *db.Database
	thread db.Thread
	mr     db.MergeRequest
	repo   string
	scroll int
	state  threadState
	err    string
}

func newThreadModel(root *Model, thread db.Thread, mr db.MergeRequest, repo string) threadModel {
	return threadModel{
		db:     root.db,
		thread: thread,
		mr:     mr,
		repo:   repo,
		state:  threadViewing,
	}
}

// update handles input for the thread view.
func (m *threadModel) update(msg tea.Msg, root *Model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Keys.Quit):
			return root, tea.Quit

		case key.Matches(msg, Keys.Up):
			if m.scroll > 0 {
				m.scroll--
			}

		case key.Matches(msg, Keys.Down):
			m.scroll++

		case key.Matches(msg, Keys.Back):
			root.current = viewMRDetail
			return root, root.mrDetail.loadThreads()

		case key.Matches(msg, Keys.Claude):
			m.err = "Claude integration pending"
		}
	}
	return root, nil
}

// view renders the thread view.
func (m *threadModel) view(root *Model) string {
	var sb strings.Builder

	// Title: file:line or "General".
	location := "General"
	if m.thread.FilePath != nil {
		location = *m.thread.FilePath
		if m.thread.NewLine != nil {
			location = fmt.Sprintf("%s:%d", location, *m.thread.NewLine)
		} else if m.thread.OldLine != nil {
			location = fmt.Sprintf("%s:%d", location, *m.thread.OldLine)
		}
	}

	header := fmt.Sprintf("Thread — %s  (MR !%d: %s)", location, m.mr.IID, truncate(m.mr.Title, 40))
	sb.WriteString(titleStyle.Render(header))
	sb.WriteString("\n\n")

	if m.err != "" {
		sb.WriteString(unresolvedStyle.Render("! "+m.err))
		sb.WriteString("\n\n")
	}

	// Render each comment, applying scroll offset.
	lines := buildThreadLines(m.thread)
	start := m.scroll
	if start > len(lines) {
		start = len(lines)
	}
	for _, line := range lines[start:] {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("j/k: scroll  c: claude  h/b: back  q: quit"))

	return sb.String()
}

// buildThreadLines converts thread comments into display lines.
func buildThreadLines(thread db.Thread) []string {
	var lines []string
	for _, c := range thread.Comments {
		// Header: @author (age)
		header := selectedStyle.Render("@"+c.Author) + " " + dimStyle.Render("("+timeAgo(c.CreatedAt)+")")
		lines = append(lines, header)

		// Body, indented.
		for _, bodyLine := range strings.Split(c.Body, "\n") {
			lines = append(lines, "  "+bodyLine)
		}
		lines = append(lines, "")
	}
	return lines
}

// timeAgo returns a human-friendly relative time string.
func timeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}
