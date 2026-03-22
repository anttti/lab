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

// threadMarkedReadMsg is sent after marking a thread as read (no-op handler).
type threadMarkedReadMsg struct{}

// markThreadReadCmd returns a command that marks a thread as read in the DB.
func markThreadReadCmd(database *db.Database, mrID int64, discussionID string) tea.Cmd {
	return func() tea.Msg {
		_ = database.MarkThreadRead(mrID, discussionID)
		return threadMarkedReadMsg{}
	}
}

// threadModel shows the full content of a single thread.
type threadModel struct {
	db      *db.Database
	threads []db.Thread
	index   int
	thread  db.Thread
	mr      db.MergeRequest
	repo    string
	scroll  int
	state   threadState
	err     string
}

func newThreadModel(root *Model, threads []db.Thread, index int, mr db.MergeRequest, repo string) (threadModel, tea.Cmd) {
	m := threadModel{
		db:      root.db,
		threads: threads,
		index:   index,
		thread:  threads[index],
		mr:      mr,
		repo:    repo,
		state:   threadViewing,
	}
	return m, markThreadReadCmd(root.db, mr.ID, threads[index].DiscussionID)
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
			case key.Matches(msg, Keys.Quit), key.Matches(msg, Keys.Back):
				root.current = viewMRDetail
				return root, root.mrDetail.loadThreads()

			case key.Matches(msg, Keys.Up):
				if m.scroll > 0 {
					m.scroll--
				}

			case key.Matches(msg, Keys.Down):
				m.scroll++

			case key.Matches(msg, Keys.Next):
				if m.index < len(m.threads)-1 {
					m.index++
					m.thread = m.threads[m.index]
					m.scroll = 0
					return root, markThreadReadCmd(m.db, m.mr.ID, m.thread.DiscussionID)
				}

			case key.Matches(msg, Keys.Prev):
				if m.index > 0 {
					m.index--
					m.thread = m.threads[m.index]
					m.scroll = 0
					return root, markThreadReadCmd(m.db, m.mr.ID, m.thread.DiscussionID)
				}

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

	if m.err != "" {
		sb.WriteString(unresolvedStyle.Render("! "+m.err))
		sb.WriteString("\n\n")
	}

	// Render each comment, applying scroll offset.
	lines := buildThreadLines(m.thread, root.width-2)
	start := m.scroll
	if start > len(lines) {
		start = len(lines)
	}
	for _, line := range lines[start:] {
		sb.WriteString(line)
		sb.WriteString("\n")
	}

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

	title := fmt.Sprintf("Thread %d/%d — %s  (MR !%d: %s)", m.index+1, len(m.threads), location, m.mr.IID, truncate(m.mr.Title, 40))

	var help string
	if m.state == threadClaudeChoice {
		help = "Send as-is (s) or augment in editor (a)? (esc to cancel)"
	} else {
		help = "j/k: scroll  n/p: next/prev thread  c: claude  h/b: back  q: quit"
	}

	return renderPanel(title, sb.String(), help, root.width, root.height)
}

// buildThreadLines converts thread comments into display lines.
func buildThreadLines(thread db.Thread, width int) []string {
	// Body is indented by 2, so wrap width accounts for that.
	wrapWidth := width - 2
	if wrapWidth < 40 {
		wrapWidth = 40
	}

	var lines []string
	for _, c := range thread.Comments {
		// Header: @author (age)
		header := selectedStyle.Render("@"+c.Author) + " " + dimStyle.Render("("+timeAgo(c.CreatedAt)+")")
		lines = append(lines, header)

		// Body, indented and word-wrapped.
		for _, bodyLine := range strings.Split(c.Body, "\n") {
			for _, wrapped := range wordWrap(bodyLine, wrapWidth) {
				lines = append(lines, "  "+wrapped)
			}
		}
		lines = append(lines, "")
	}
	return lines
}

// wordWrap breaks a line into multiple lines at word boundaries to fit within maxWidth.
func wordWrap(s string, maxWidth int) []string {
	if len([]rune(s)) <= maxWidth {
		return []string{s}
	}

	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len([]rune(current))+1+len([]rune(w)) > maxWidth {
			lines = append(lines, current)
			current = w
		} else {
			current += " " + w
		}
	}
	lines = append(lines, current)
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
