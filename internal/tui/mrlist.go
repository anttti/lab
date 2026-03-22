package tui

import (
	"fmt"
	"strings"

	"lab/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
)

// mrItem holds display-ready data for a single MR row.
type mrItem struct {
	mr              db.MergeRequest
	repoName        string
	unresolvedCount int
	unreadCount     int
}

// mrsLoadedMsg carries the result of an async MR load.
type mrsLoadedMsg struct {
	items         []mrItem
	activeFilters string
	err           error
}

// mrListModel is the home screen listing all MRs.
type mrListModel struct {
	db            *db.Database
	items         []mrItem
	cursor        int
	activeFilters string
}

func newMRListModel(root *Model) mrListModel {
	return mrListModel{db: root.db}
}

// loadMRs reads filter state from config, queries MRs, and populates items.
func (m *mrListModel) loadMRs() tea.Cmd {
	return func() tea.Msg {
		database := m.db

		// Read filter config values.
		repoFilter, _ := database.GetConfig("active_repo_filter")
		userFilter, _ := database.GetConfig("active_user_filter")
		labelFilter, _ := database.GetConfig("active_label_filters")

		filter := db.MRFilter{}

		if repoFilter != "" {
			// Find repo by name.
			repos, err := database.ListRepos()
			if err == nil {
				for _, r := range repos {
					if r.Name == repoFilter {
						id := r.ID
						filter.RepoID = &id
						break
					}
				}
			}
		}

		if userFilter == "me" {
			me, _ := database.GetConfig("username")
			if me != "" {
				filter.Author = &me
			}
		}

		if labelFilter != "" {
			parts := strings.Split(labelFilter, ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					filter.Labels = append(filter.Labels, p)
				}
			}
		}

		mrs, err := database.ListMRs(filter)
		if err != nil {
			return mrsLoadedMsg{err: err}
		}

		// Build repo name map.
		repos, _ := database.ListRepos()
		repoNames := make(map[int64]string, len(repos))
		for _, r := range repos {
			repoNames[r.ID] = r.Name
		}

		items := make([]mrItem, 0, len(mrs))
		for _, mr := range mrs {
			unresolved, _ := database.UnresolvedCommentCount(mr.ID)
			unread, _ := database.UnreadThreadCount(mr.ID)
			items = append(items, mrItem{
				mr:              mr,
				repoName:        repoNames[mr.RepoID],
				unresolvedCount: unresolved,
				unreadCount:     unread,
			})
		}

		// Build a human-readable filter summary.
		var parts []string
		if repoFilter != "" {
			parts = append(parts, "repo:"+repoFilter)
		}
		if userFilter != "" {
			parts = append(parts, "user:"+userFilter)
		}
		if labelFilter != "" {
			parts = append(parts, "labels:"+labelFilter)
		}

		return mrsLoadedMsg{items: items, activeFilters: strings.Join(parts, "  ")}
	}
}

// cycleRepoFilter advances the repo filter to the next option and reloads MRs.
func (m *mrListModel) cycleRepoFilter() tea.Cmd {
	return func() tea.Msg {
		database := m.db
		repos, err := database.ListRepos()
		if err != nil {
			return mrsLoadedMsg{err: err}
		}

		current, _ := database.GetConfig("active_repo_filter")

		// Build option list: "" (all), then each repo name.
		options := make([]string, 0, len(repos)+1)
		options = append(options, "")
		for _, r := range repos {
			options = append(options, r.Name)
		}

		// Find current index and advance.
		idx := 0
		for i, o := range options {
			if o == current {
				idx = i
				break
			}
		}
		next := options[(idx+1)%len(options)]

		_ = database.SetConfig("active_repo_filter", next)

		// Now load MRs with the updated filter (reuse loadMRs logic inline).
		return m.loadMRs()()
	}
}

// update handles input for the MR list view.
func (m *mrListModel) update(msg tea.Msg, root *Model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case mrsLoadedMsg:
		if msg.err == nil {
			m.items = msg.items
			m.activeFilters = msg.activeFilters
			if m.cursor >= len(m.items) && len(m.items) > 0 {
				m.cursor = len(m.items) - 1
			}
		}
		return root, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Keys.Quit):
			return root, tea.Quit

		case key.Matches(msg, Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, Keys.Down):
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}

		case key.Matches(msg, Keys.Top):
			m.cursor = 0

		case key.Matches(msg, Keys.Bottom):
			if len(m.items) > 0 {
				m.cursor = len(m.items) - 1
			}

		case key.Matches(msg, Keys.Select):
			if len(m.items) > 0 {
				item := m.items[m.cursor]
				detail := newMRDetailModel(root, item.mr, item.repoName)
				root.mrDetail = detail
				root.current = viewMRDetail
				return root, root.mrDetail.loadThreads()
			}

		case key.Matches(msg, Keys.CycleFilter):
			return root, m.cycleRepoFilter()

		case key.Matches(msg, Keys.Filter):
			filt := newFilterModel(root)
			root.filter = filt
			root.current = viewFilter
			return root, root.filter.load()

		case key.Matches(msg, Keys.Sync):
			return root, m.loadMRs()
		}
	}
	return root, nil
}

// view renders the MR list.
func (m *mrListModel) view(root *Model) string {
	var sb strings.Builder

	// Filter status bar.
	if m.activeFilters != "" {
		sb.WriteString(statusBarStyle.Render("Filters: " + m.activeFilters))
	} else {
		sb.WriteString(statusBarStyle.Render("No filters active"))
	}
	sb.WriteString("\n\n")

	if len(m.items) == 0 {
		sb.WriteString(dimStyle.Render("No merge requests found."))
		sb.WriteString("\n")
	} else {
		// Calculate dynamic title column width based on terminal width.
		// Fixed columns: cursor(2) + repo(12) + " !"(2) + IID(4) + " "(1) + " @"(2) + author(15) + suffix(6) = 44
		// Plus 2 for panel border.
		titleWidth := root.width - 46
		if titleWidth < 20 {
			titleWidth = 20
		}

		for i, item := range m.items {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}

			// Build the row text.
			title := truncate(item.mr.Title, titleWidth)
			row := fmt.Sprintf("%-12s !%-4d %-*s @%-15s",
				truncate(item.repoName, 12),
				item.mr.IID,
				titleWidth,
				title,
				truncate(item.mr.Author, 15),
			)

			// Unread indicator.
			if item.unreadCount > 0 {
				row += " " + unreadStyle.Render("●")
			}

			// Unresolved count.
			if item.unresolvedCount > 0 {
				row += " " + unresolvedStyle.Render(fmt.Sprintf("%d↩", item.unresolvedCount))
			}

			// Pipeline indicator.
			row += " " + pipelineIndicator(item.mr.PipelineStatus)

			if i == m.cursor {
				sb.WriteString(selectedStyle.Render(cursor+row) + "\n")
			} else {
				sb.WriteString(cursor + row + "\n")
			}
		}
	}

	title := titleStyle.Render("lab") + dimStyle.Render(fmt.Sprintf(" — %d MRs", len(m.items)))
	help := "j/k: navigate  l/enter: select  f: filter  o: cycle project  r: sync  q: quit"
	return renderPanel(title, sb.String(), help, root.width, root.height)
}

// pipelineIndicator returns a styled pipeline status symbol.
func pipelineIndicator(status *string) string {
	if status == nil {
		return dimStyle.Render("—")
	}
	switch *status {
	case "success":
		return pipelineSuccess.Render("✓")
	case "failed":
		return pipelineFailed.Render("✗")
	case "running", "pending":
		return pipelineRunning.Render("⟳")
	default:
		return dimStyle.Render("—")
	}
}

// truncate shortens a string to maxLen runes, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}
