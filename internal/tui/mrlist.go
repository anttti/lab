package tui

import (
	"fmt"
	"strings"

	"lab/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// filterGroup identifies which filter is being edited.
type filterGroup int

const (
	filterGroupRepo filterGroup = iota
	filterGroupAuthor
	filterGroupLabels
	filterGroupDraft
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
	items          []mrItem
	unreadOnly     bool
	err            error
	repoOptions    []string
	authorOptions  []string
	labelOptions   []string
	selectedRepo   string
	selectedAuthor string
	selectedLabel  string
	selectedDraft  string
}

// mrListModel is the home screen listing all MRs.
type mrListModel struct {
	db     *db.Database
	items  []mrItem
	cursor int
	offset int // first visible item index for scrolling

	// Filter selections.
	selectedRepo   string
	selectedAuthor string
	selectedLabel  string
	selectedDraft  string // "", "drafts", "ready"
	unreadOnly     bool

	// Available filter options (loaded from DB).
	repoOptions   []string
	authorOptions []string
	labelOptions  []string

	// Autocomplete state (nil when not active).
	autocomplete *autocompleteModel
	activeFilter filterGroup
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
		draftFilter, _ := database.GetConfig("active_draft_filter")
		unreadFilter, _ := database.GetConfig("active_unread_filter")

		filter := db.MRFilter{}

		switch draftFilter {
		case "drafts":
			t := true
			filter.Draft = &t
		case "ready":
			f := false
			filter.Draft = &f
		}

		repos, _ := database.ListRepos()

		if repoFilter != "" {
			for _, r := range repos {
				if r.Name == repoFilter {
					id := r.ID
					filter.RepoID = &id
					break
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

		// Build repo name/path maps and option list.
		repoNames := make(map[int64]string, len(repos))
		repoPaths := make(map[int64]string, len(repos))
		repoOptions := make([]string, 0, len(repos))
		for _, r := range repos {
			repoNames[r.ID] = r.Name
			repoPaths[r.ID] = r.Path
			repoOptions = append(repoOptions, r.Name)
		}

		authors, _ := database.AllAuthors()
		labels, _ := database.AllLabels()

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

		return mrsLoadedMsg{
			items:          items,
			unreadOnly:     unreadOnly,
			repoOptions:    repoOptions,
			authorOptions:  authors,
			labelOptions:   labels,
			selectedRepo:   repoFilter,
			selectedAuthor: authorFilter,
			selectedLabel:  labelFilter,
			selectedDraft:  draftFilter,
		}
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

// saveAndReload persists current filter selections and reloads MRs.
func (m *mrListModel) saveAndReload() tea.Cmd {
	return func() tea.Msg {
		_ = m.db.SetConfig("active_repo_filter", m.selectedRepo)
		_ = m.db.SetConfig("active_author_filter", m.selectedAuthor)
		_ = m.db.SetConfig("active_label_filters", m.selectedLabel)
		_ = m.db.SetConfig("active_draft_filter", m.selectedDraft)
		return m.loadMRs()()
	}
}

// applySelection updates the filter selection for the given group.
func (m *mrListModel) applySelection(group filterGroup, value string) {
	switch group {
	case filterGroupRepo:
		if value == "All repos" {
			m.selectedRepo = ""
		} else {
			m.selectedRepo = value
		}
	case filterGroupAuthor:
		if value == "All authors" {
			m.selectedAuthor = ""
		} else {
			m.selectedAuthor = value
		}
	case filterGroupLabels:
		if value == "No filter" {
			m.selectedLabel = ""
		} else {
			m.selectedLabel = value
		}
	case filterGroupDraft:
		switch value {
		case "◇ Draft":
			m.selectedDraft = "drafts"
		case "◆ Ready":
			m.selectedDraft = "ready"
		default:
			m.selectedDraft = ""
		}
	}
}

// openAutocomplete starts the autocomplete for the given filter group.
func (m *mrListModel) openAutocomplete(group filterGroup) {
	var options []string
	var current string
	switch group {
	case filterGroupRepo:
		options = append([]string{"All repos"}, m.repoOptions...)
		current = m.selectedRepo
		if current == "" {
			current = "All repos"
		}
	case filterGroupAuthor:
		options = append([]string{"All authors"}, m.authorOptions...)
		current = m.selectedAuthor
		if current == "" {
			current = "All authors"
		}
	case filterGroupLabels:
		options = append([]string{"No filter"}, m.labelOptions...)
		current = m.selectedLabel
		if current == "" {
			current = "No filter"
		}
	case filterGroupDraft:
		options = []string{"All", "◇ Draft", "◆ Ready"}
		switch m.selectedDraft {
		case "drafts":
			current = "◇ Draft"
		case "ready":
			current = "◆ Ready"
		default:
			current = "All"
		}
	}
	ac := newAutocomplete(options, current)
	m.autocomplete = &ac
	m.activeFilter = group
}

// update handles input for the MR list view.
func (m *mrListModel) update(msg tea.Msg, root *Model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case mrsLoadedMsg:
		if msg.err == nil {
			m.items = msg.items
			m.unreadOnly = msg.unreadOnly
			m.repoOptions = msg.repoOptions
			m.authorOptions = msg.authorOptions
			m.labelOptions = msg.labelOptions
			m.selectedRepo = msg.selectedRepo
			m.selectedAuthor = msg.selectedAuthor
			m.selectedLabel = msg.selectedLabel
			m.selectedDraft = msg.selectedDraft
			if m.cursor >= len(m.items) && len(m.items) > 0 {
				m.cursor = len(m.items) - 1
			}
		}
		return root, nil

	case tea.KeyMsg:
		// If autocomplete is active, route input to it.
		if m.autocomplete != nil {
			if msg.String() == "ctrl+c" {
				return root, tea.Quit
			}
			done, cancelled := m.autocomplete.update(msg)
			if done {
				if !cancelled {
					m.applySelection(m.activeFilter, m.autocomplete.selected())
					m.autocomplete = nil
					return root, m.saveAndReload()
				}
				m.autocomplete = nil
			}
			return root, nil
		}

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

		case key.Matches(msg, Keys.FilterRepo):
			m.openAutocomplete(filterGroupRepo)

		case key.Matches(msg, Keys.FilterAuthor):
			m.openAutocomplete(filterGroupAuthor)

		case key.Matches(msg, Keys.FilterLabel):
			m.openAutocomplete(filterGroupLabels)

		case key.Matches(msg, Keys.FilterDraft):
			m.openAutocomplete(filterGroupDraft)

		case key.Matches(msg, Keys.ToggleUnread):
			return root, m.toggleUnreadFilter()

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

	// Filter bar (3 boxes + optional unread indicator).
	sb.WriteString(m.renderFilterBar(root.width - 2))
	sb.WriteString("\n")

	if m.autocomplete != nil {
		// Show autocomplete dropdown instead of MR list.
		acHeight := root.height - 8 // panel border(2) + help(1) + filter bar(4) + blank(1)
		if acHeight < 3 {
			acHeight = 3
		}
		sb.WriteString(m.autocomplete.view(root.width-2, acHeight))
	} else {
		// Show MR list.
		if len(m.items) == 0 {
			sb.WriteString(dimStyle.Render("No merge requests found."))
			sb.WriteString("\n")
		} else {
			titleWidth := root.width - 48
			if titleWidth < 20 {
				titleWidth = 20
			}

			// Filter bar takes 4 lines (3 box lines + 1 blank).
			visibleRows := root.height - 8
			if visibleRows < 1 {
				visibleRows = 1
			}

			if m.cursor < m.offset {
				m.offset = m.cursor
			}
			if m.cursor >= m.offset+visibleRows {
				m.offset = m.cursor - visibleRows + 1
			}

			end := m.offset + visibleRows
			if end > len(m.items) {
				end = len(m.items)
			}

			for i := m.offset; i < end; i++ {
				item := m.items[i]
				unread := " "
				if item.unreadCount > 0 {
					unread = unreadStyle.Render("●")
				}

				pipeline := pipelineIndicator(item.mr.PipelineStatus)

				unresolvedStr := "   "
				if item.unresolvedCount > 0 {
					unresolvedStr = unresolvedStyle.Render(fmt.Sprintf("%2d↩", item.unresolvedCount))
				}

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
	}

	title := titleStyle.Render("lab") + dimStyle.Render(fmt.Sprintf(" — %d MRs", len(m.items)))
	var help string
	if m.autocomplete != nil {
		help = "type to filter  ↑/↓/ctrl-p/ctrl-n: navigate  enter: select  esc: cancel"
	} else {
		help = "j/k: navigate  l/enter: select  r: repo  a: author  L: labels  d: draft  u: unread  R: sync  q: quit"
	}
	if root.syncing && root.syncStatus != "" {
		help = pipelineRunning.Render("⟳ "+root.syncStatus) + "  " + help
	}
	return renderPanel(title, sb.String(), help, root.width, root.height)
}

// renderFilterBar renders the inline filter boxes.
func (m *mrListModel) renderFilterBar(innerWidth int) string {
	draftBoxWidth := 14 // narrow box for the draft filter
	remainingWidth := innerWidth - draftBoxWidth - 3 // 3 gaps of 1 char each
	boxWidth := remainingWidth / 3
	if boxWidth < 15 {
		boxWidth = 15
	}

	repoVal := "All repos"
	if m.selectedRepo != "" {
		repoVal = m.selectedRepo
	}
	authorVal := "All authors"
	if m.selectedAuthor != "" {
		authorVal = m.selectedAuthor
	}
	labelVal := "No filter"
	if m.selectedLabel != "" {
		labelVal = m.selectedLabel
	}
	draftVal := "All"
	switch m.selectedDraft {
	case "drafts":
		draftVal = "◇ Draft"
	case "ready":
		draftVal = "◆ Ready"
	}

	repoActive := m.autocomplete != nil && m.activeFilter == filterGroupRepo
	authorActive := m.autocomplete != nil && m.activeFilter == filterGroupAuthor
	labelActive := m.autocomplete != nil && m.activeFilter == filterGroupLabels
	draftActive := m.autocomplete != nil && m.activeFilter == filterGroupDraft

	repoLines := filterBoxLines("Repo", repoVal, "r", boxWidth, repoActive)
	authorLines := filterBoxLines("Author", authorVal, "a", boxWidth, authorActive)
	labelLines := filterBoxLines("Labels", labelVal, "L", boxWidth, labelActive)
	draftLines := filterBoxLines("Draft", draftVal, "d", draftBoxWidth, draftActive)

	// Add unread indicator if active.
	unreadIndicator := ""
	if m.unreadOnly {
		unreadIndicator = " " + unreadStyle.Render("● unread")
	}

	var sb strings.Builder
	for i := 0; i < 3; i++ {
		sb.WriteString(repoLines[i])
		sb.WriteString(" ")
		sb.WriteString(authorLines[i])
		sb.WriteString(" ")
		sb.WriteString(labelLines[i])
		sb.WriteString(" ")
		sb.WriteString(draftLines[i])
		if i == 1 {
			sb.WriteString(unreadIndicator)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// filterBoxLines returns the 3 lines of a filter box (top border with title+hotkey, value, bottom border).
func filterBoxLines(title, value, hotkey string, width int, active bool) [3]string {
	bc := lipgloss.RoundedBorder()
	color := borderColor
	if active {
		color = lipgloss.Color("170")
	}
	bStyle := lipgloss.NewStyle().Foreground(color)
	tStyle := lipgloss.NewStyle().Foreground(color).Bold(true)

	innerW := width - 2
	if innerW < 1 {
		innerW = 1
	}

	// Line 0: top border with title and hotkey.
	titleText := " " + title + " (" + hotkey + ") "
	titleVisualW := lipgloss.Width(titleText)
	remaining := innerW - 1 - titleVisualW
	if remaining < 0 {
		remaining = 0
	}
	top := bStyle.Render(bc.TopLeft+bc.Top) + tStyle.Render(titleText) + bStyle.Render(strings.Repeat(bc.Top, remaining)+bc.TopRight)

	// Line 1: value.
	displayValue := truncate(value, innerW-1)
	contentW := lipgloss.Width(displayValue) + 1 // +1 for leading space
	pad := innerW - contentW
	if pad < 0 {
		pad = 0
	}
	mid := bStyle.Render(bc.Left) + " " + displayValue + strings.Repeat(" ", pad) + bStyle.Render(bc.Right)

	// Line 2: bottom border.
	bottom := bStyle.Render(bc.BottomLeft + strings.Repeat(bc.Bottom, innerW) + bc.BottomRight)

	return [3]string{top, mid, bottom}
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
