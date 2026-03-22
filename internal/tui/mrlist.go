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
	repoPath        string
	unresolvedCount int
	unreadCount     int
}

// mrsLoadedMsg carries the result of an async MR load.
type mrsLoadedMsg struct {
	items         []mrItem
	activeFilters string
	unreadOnly    bool
	err           error
}

// mrListModel is the home screen listing all MRs.
type mrListModel struct {
	db            *db.Database
	items         []mrItem
	cursor        int
	offset        int // first visible item index for scrolling
	activeFilters string
	unreadOnly    bool
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
		authorFilter, _ := database.GetConfig("active_author_filter")
		labelFilter, _ := database.GetConfig("active_label_filters")
		unreadFilter, _ := database.GetConfig("active_unread_filter")

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

		if authorFilter != "" {
			filter.Author = &authorFilter
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

		// Build repo name/path maps.
		repos, _ := database.ListRepos()
		repoNames := make(map[int64]string, len(repos))
		repoPaths := make(map[int64]string, len(repos))
		for _, r := range repos {
			repoNames[r.ID] = r.Name
			repoPaths[r.ID] = r.Path
		}

		items := make([]mrItem, 0, len(mrs))
		for _, mr := range mrs {
			unresolved, _ := database.UnresolvedCommentCount(mr.ID)
			unread, _ := database.UnreadThreadCount(mr.ID)
			items = append(items, mrItem{
				mr:              mr,
				repoName:        repoNames[mr.RepoID],
				repoPath:        repoPaths[mr.RepoID],
				unresolvedCount: unresolved,
				unreadCount:     unread,
			})
		}

		// Filter by unread if active.
		unreadOnly := unreadFilter == "true"
		if unreadOnly {
			filtered := items[:0]
			for _, item := range items {
				if item.unreadCount > 0 {
					filtered = append(filtered, item)
				}
			}
			items = filtered
		}

		// Build a human-readable filter summary.
		var parts []string
		if repoFilter != "" {
			parts = append(parts, "repo:"+repoFilter)
		}
		if authorFilter != "" {
			parts = append(parts, "author:"+authorFilter)
		}
		if labelFilter != "" {
			parts = append(parts, "labels:"+labelFilter)
		}
		if unreadOnly {
			parts = append(parts, "unread")
		}

		return mrsLoadedMsg{items: items, activeFilters: strings.Join(parts, "  "), unreadOnly: unreadOnly}
	}
}

// toggleUnreadFilter toggles the unread-only filter and reloads MRs.
func (m *mrListModel) toggleUnreadFilter() tea.Cmd {
	return func() tea.Msg {
		current, _ := m.db.GetConfig("active_unread_filter")
		next := "true"
		if current == "true" {
			next = ""
		}
		_ = m.db.SetConfig("active_unread_filter", next)
		return m.loadMRs()()
	}
}

// cycleRepoFilter cycles the repo filter by delta (+1 forward, -1 backward) and reloads MRs.
func (m *mrListModel) cycleRepoFilter(delta int) tea.Cmd {
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
		next := options[(idx+delta+len(options))%len(options)]

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
			m.unreadOnly = msg.unreadOnly
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
				detail := newMRDetailModel(root, item.mr, item.repoName, item.repoPath)
				root.mrDetail = detail
				root.current = viewMRDetail
				return root, root.mrDetail.loadThreads()
			}

		case key.Matches(msg, Keys.CycleFilter):
			return root, m.cycleRepoFilter(1)

		case key.Matches(msg, Keys.CycleFilterBack):
			return root, m.cycleRepoFilter(-1)

		case key.Matches(msg, Keys.ToggleUnread):
			return root, m.toggleUnreadFilter()

		case key.Matches(msg, Keys.Filter):
			filt := newFilterModel(root)
			root.filter = filt
			root.current = viewFilter
			return root, root.filter.load()

		case key.Matches(msg, Keys.Sync):
			if !root.syncing {
				return root, root.startSync()
			}
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
		// Fixed columns: " "(1) + repo(12) + " !"(2) + IID(4) + " "(1) + unread(1) + " "(1)
		//   + pipeline(1) + "  "(2) + " @"(2) + author(15) + " "(1) + unresolved(3) + border(2) = 48
		titleWidth := root.width - 48
		if titleWidth < 20 {
			titleWidth = 20
		}

		// Calculate visible rows: panel border (2) + help bar (1) + filter bar + blank line (2).
		visibleRows := root.height - 5
		if visibleRows < 1 {
			visibleRows = 1
		}

		// Adjust offset so cursor is always visible.
		if m.cursor < m.offset {
			m.offset = m.cursor
		}
		if m.cursor >= m.offset+visibleRows {
			m.offset = m.cursor - visibleRows + 1
		}

		// Determine visible slice.
		end := m.offset + visibleRows
		if end > len(m.items) {
			end = len(m.items)
		}

		for i := m.offset; i < end; i++ {
			item := m.items[i]
			// Unread indicator (fixed width: 1 visual char).
			unread := " "
			if item.unreadCount > 0 {
				unread = unreadStyle.Render("●")
			}

			// Pipeline indicator (fixed width: 1 visual char).
			pipeline := pipelineIndicator(item.mr.PipelineStatus)

			// Unresolved count (fixed width: 3 visual chars, right-aligned).
			unresolvedStr := "   "
			if item.unresolvedCount > 0 {
				unresolvedStr = unresolvedStyle.Render(fmt.Sprintf("%2d↩", item.unresolvedCount))
			}

			// Build the row: repo, MR ID, unread, pipeline, title, author, comment count.
			title := truncate(item.mr.Title, titleWidth)
			prefix := fmt.Sprintf(" %-12s !%-4d ", truncate(item.repoName, 12), item.mr.IID)
			titleAuthor := fmt.Sprintf("%-*s @%-15s ", titleWidth, title, truncate(item.mr.Author, 15))

			row := prefix + unread + " " + pipeline + "  " + titleAuthor + unresolvedStr
			if i == m.cursor {
				sb.WriteString(renderSelectedRow(row, root.width-2) + "\n")
			} else {
				sb.WriteString(row + "\n")
			}
		}
	}

	title := titleStyle.Render("lab") + dimStyle.Render(fmt.Sprintf(" — %d MRs", len(m.items)))
	help := "j/k: navigate  l/enter: select  f: filter  tab: cycle project  u: unread  r: sync  q: quit"
	if root.syncing && root.syncStatus != "" {
		help = pipelineRunning.Render("⟳ "+root.syncStatus) + "  " + help
	}
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
