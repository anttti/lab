package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"lab/internal/claude"
	"lab/internal/db"
	"lab/internal/git"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	db       *db.Database
	threads  []db.Thread
	index    int
	thread   db.Thread
	mr       db.MergeRequest
	repoName string
	repo     string
	scroll   int
	state    threadState
	err      string
}

func newThreadModel(root *Model, threads []db.Thread, index int, mr db.MergeRequest, repoName, repo string) (threadModel, tea.Cmd) {
	m := threadModel{
		db:       root.db,
		threads:  threads,
		index:    index,
		thread:   threads[index],
		mr:       mr,
		repoName: repoName,
		repo:     repo,
		state:    threadViewing,
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
				// Send as-is: suspend TUI, run claude inline.
				thread := m.thread
				repo := m.repo

				if err := ensureBranch(repo, m.mr.SourceBranch); err != nil {
					m.err = err.Error()
					m.state = threadViewing
					return root, nil
				}

				prompt := claude.BuildPrompt(&thread, repo)

				cmd, err := claude.ClaudeCmd(prompt, repo)
				if err != nil {
					m.err = err.Error()
					m.state = threadViewing
					return root, nil
				}

				m.state = threadViewing
				return root, tea.ExecProcess(cmd, func(err error) tea.Msg {
					return claudeLaunchedMsg{err: err}
				})

			case "a":
				// Augment in editor, then run claude inline.
				thread := m.thread
				repo := m.repo

				if err := ensureBranch(repo, m.mr.SourceBranch); err != nil {
					m.err = err.Error()
					m.state = threadViewing
					return root, nil
				}

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

					data, readErr := os.ReadFile(tmpFile)
					_ = os.Remove(tmpFile)
					if readErr != nil {
						return claudeLaunchedMsg{err: fmt.Errorf("read edited file: %w", readErr)}
					}

					cmd, cmdErr := claude.ClaudeCmd(string(data), repo)
					if cmdErr != nil {
						return claudeLaunchedMsg{err: cmdErr}
					}

					// Run claude synchronously; we're already outside the TUI.
					runErr := cmd.Run()
					return claudeLaunchedMsg{err: runErr}
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

	// Title area: MR info and repo/branch.
	mrTitle := titleStyle.Render(fmt.Sprintf("!%d", m.mr.IID)) + " " +
		lipgloss.NewStyle().Bold(true).Render(m.mr.Title)
	sb.WriteString(mrTitle)
	sb.WriteString("\n")
	repoInfo := dimStyle.Render(m.repoName) + " " +
		selectedStyle.Render(m.mr.SourceBranch) +
		dimStyle.Render(" → ") +
		dimStyle.Render(m.mr.TargetBranch)
	sb.WriteString(repoInfo)
	sb.WriteString("\n\n")

	if m.err != "" {
		sb.WriteString(unresolvedStyle.Render("! " + m.err))
		sb.WriteString("\n\n")
	}

	// Render code context if available.
	if m.thread.DiffHunk != "" {
		targetLine := 0
		if m.thread.NewLine != nil {
			targetLine = *m.thread.NewLine
		} else if m.thread.OldLine != nil {
			targetLine = *m.thread.OldLine
		}
		for _, line := range formatDiffHunk(m.thread.DiffHunk, targetLine, root.width-4) {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
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

	// Panel title: file:line or "General".
	location := "General"
	if m.thread.FilePath != nil {
		location = *m.thread.FilePath
		if m.thread.NewLine != nil {
			location = fmt.Sprintf("%s:%d", location, *m.thread.NewLine)
		} else if m.thread.OldLine != nil {
			location = fmt.Sprintf("%s:%d", location, *m.thread.OldLine)
		}
	}

	title := fmt.Sprintf("Thread %d/%d — %s", m.index+1, len(m.threads), location)

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
		// Header: @author (age) — one space indent for border alignment.
		header := " " + selectedStyle.Render("@"+c.Author) + " " + dimStyle.Render("("+timeAgo(c.CreatedAt)+")")
		lines = append(lines, header)

		// Body, indented and word-wrapped.
		for _, bodyLine := range strings.Split(c.Body, "\n") {
			for _, wrapped := range wordWrap(bodyLine, wrapWidth) {
				lines = append(lines, "   "+wrapped)
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

// ensureBranch checks that the repo is clean and switches to the given branch
// if not already on it. Returns an error if the repo has uncommitted changes.
func ensureBranch(repoPath, branch string) error {
	if branch == "" {
		return nil
	}

	if err := git.IsClean(repoPath); err != nil {
		return fmt.Errorf("cannot switch to branch %s: %w — commit or stash your changes first", branch, err)
	}

	current, err := git.CurrentBranch(repoPath)
	if err != nil {
		return err
	}
	if current == branch {
		return nil
	}

	return git.Checkout(repoPath, branch)
}

// formatDiffHunk formats a code snippet into styled display lines.
// The snippet format is "linenum\tcontent\n" per line. The target line
// (matching the thread's line number) is highlighted.
func formatDiffHunk(hunk string, targetLine int, maxWidth int) []string {
	raw := strings.Split(strings.TrimRight(hunk, "\n"), "\n")
	var lines []string
	for _, line := range raw {
		if line == "" {
			continue
		}
		// Parse "linenum\tcontent" format.
		parts := strings.SplitN(line, "\t", 2)
		lineNum := parts[0]
		content := ""
		if len(parts) > 1 {
			content = parts[1]
		}

		num := 0
		fmt.Sscanf(lineNum, "%d", &num)

		display := fmt.Sprintf(" %4s │ %s", lineNum, content)
		if len([]rune(display)) > maxWidth {
			display = string([]rune(display)[:maxWidth])
		}

		if num == targetLine {
			lines = append(lines, selectedStyle.Render(display))
		} else {
			lines = append(lines, diffContextStyle.Render(display))
		}
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
