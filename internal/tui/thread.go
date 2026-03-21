package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"lab/internal/claude"
	"lab/internal/db"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type threadState int

const (
	threadViewing      threadState = iota
	threadClaudeChoice             // waiting for s/a/esc
)

// claudeLaunchedMsg is sent after attempting to launch Claude.
type claudeLaunchedMsg struct{ err error }

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
	case claudeLaunchedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
		}
		return root, nil

	case tea.KeyMsg:
		switch m.state {
		case threadViewing:
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
				m.state = threadClaudeChoice
				m.err = ""
			}

		case threadClaudeChoice:
			switch msg.String() {
			case "s":
				// Send as-is.
				thread := m.thread
				repo := m.repo
				prompt := claude.BuildPrompt(&thread, repo)
				return root, func() tea.Msg {
					err := claude.LaunchInNewTerminal(prompt, repo)
					return claudeLaunchedMsg{err: err}
				}

			case "a":
				// Augment in editor first.
				thread := m.thread
				repo := m.repo
				prompt := claude.BuildPrompt(&thread, repo)

				tmpFile, err := claude.WritePromptToTempFile(prompt)
				if err != nil {
					m.err = err.Error()
					m.state = threadViewing
					return root, nil
				}

				editor := os.Getenv("EDITOR")
				if editor == "" {
					editor = "vi"
				}

				editorCmd := exec.Command(editor, tmpFile)
				return root, tea.ExecProcess(editorCmd, func(err error) tea.Msg {
					if err != nil {
						_ = os.Remove(tmpFile)
						return claudeLaunchedMsg{err: fmt.Errorf("editor: %w", err)}
					}

					// Read the edited file.
					data, readErr := os.ReadFile(tmpFile)
					_ = os.Remove(tmpFile)
					if readErr != nil {
						return claudeLaunchedMsg{err: fmt.Errorf("read edited file: %w", readErr)}
					}

					launchErr := claude.LaunchInNewTerminal(string(data), repo)
					return claudeLaunchedMsg{err: launchErr}
				})

			case "esc":
				m.state = threadViewing
			}
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

	if m.state == threadClaudeChoice {
		sb.WriteString(helpStyle.Render("Send as-is (s) or augment in editor (a)? (esc to cancel)"))
	} else {
		sb.WriteString(helpStyle.Render("j/k: scroll  c: claude  h/b: back  q: quit"))
	}

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
