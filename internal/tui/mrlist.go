package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode"

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
	filterGroupReviewer
	filterGroupLabels
	filterGroupDraft
	filterGroupAccepted
)

// reviewerUnassignedLabel is the sentinel used in the reviewer autocomplete
// to represent MRs that have no reviewer assigned.
const reviewerUnassignedLabel = "— Unassigned —"

// reviewerAllLabel is the sentinel used to clear the reviewer filter.
const reviewerAllLabel = "All reviewers"

// mrItem holds display-ready data for a single MR row.
type mrItem struct {
	mr              db.MergeRequest
	repoName        string
	repoPath        string
	reviewers       []string
	unresolvedCount int
	unreadCount     int
}

// mrsLoadedMsg carries the result of an async MR load.
type mrsLoadedMsg struct {
	items            []mrItem
	unreadOnly       bool
	err              error
	repoOptions      []string
	authorOptions    []string
	reviewerOptions  []string
	labelOptions     []string
	selectedRepo     string
	selectedAuthor   string
	selectedReviewer string // "" = no filter, "__unassigned__" = unassigned, else username
	selectedLabel    string
	selectedDraft    string
	selectedAccepted string
	authorNegate     bool
}

// reviewerUnassignedSentinel is the value stored in config / model state to
// represent "MRs with no reviewer" (since an empty string means no filter).
const reviewerUnassignedSentinel = "__unassigned__"

// mrListModel is the home screen listing all MRs.
type mrListModel struct {
	db     *db.Database
	items  []mrItem
	cursor int
	offset int // first visible item index for scrolling

	// Filter selections.
	selectedRepo     string
	selectedAuthor   string
	selectedReviewer string // "" = none, reviewerUnassignedSentinel = unassigned, else username
	selectedLabel    string
	selectedDraft    string // "", "drafts", "ready"
	selectedAccepted string // "", "accepted", "not_accepted"
	authorNegate     bool   // true = exclude selected author
	unreadOnly       bool

	// Available filter options (loaded from DB).
	repoOptions     []string
	authorOptions   []string
	reviewerOptions []string
	labelOptions    []string

	// Autocomplete state (nil when not active).
	autocomplete *autocompleteModel
	activeFilter filterGroup

	// Transient flash message.
	flash string

	// Save mode: waiting for slot number input.
	saveMode bool
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
		authorNegateStr, _ := database.GetConfig("active_author_negate")
		reviewerFilter, _ := database.GetConfig("active_reviewer_filter")
		labelFilter, _ := database.GetConfig("active_label_filters")
		draftFilter, _ := database.GetConfig("active_draft_filter")
		acceptedFilter, _ := database.GetConfig("active_accepted_filter")
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

		switch acceptedFilter {
		case "accepted":
			t := true
			filter.Approved = &t
		case "not_accepted":
			f := false
			filter.Approved = &f
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

		authorNegate := authorNegateStr == "true"
		if authorFilter != "" {
			filter.Author = &authorFilter
			filter.AuthorNegate = authorNegate
		} else {
			authorNegate = false // clear negation when no author selected
		}

		switch reviewerFilter {
		case "":
			// no filter
		case reviewerUnassignedSentinel:
			empty := ""
			filter.Reviewer = &empty
		default:
			r := reviewerFilter
			filter.Reviewer = &r
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
		reviewers, _ := database.AllReviewers()
		labels, _ := database.AllLabels()

		items := make([]mrItem, 0, len(mrs))
		for _, mr := range mrs {
			unresolved, _ := database.UnresolvedCommentCount(mr.ID)
			unread, _ := database.UnreadThreadCount(mr.ID)
			mrReviewers, _ := database.GetMRReviewers(mr.ID)
			reviewerNames := make([]string, len(mrReviewers))
			for i, r := range mrReviewers {
				reviewerNames[i] = r.Username
			}
			items = append(items, mrItem{
				mr:              mr,
				repoName:        repoNames[mr.RepoID],
				repoPath:        repoPaths[mr.RepoID],
				reviewers:       reviewerNames,
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
			items:            items,
			unreadOnly:       unreadOnly,
			repoOptions:      repoOptions,
			authorOptions:    authors,
			reviewerOptions:  reviewers,
			labelOptions:     labels,
			selectedRepo:     repoFilter,
			selectedAuthor:   authorFilter,
			authorNegate:     authorNegate,
			selectedReviewer: reviewerFilter,
			selectedLabel:    labelFilter,
			selectedDraft:    draftFilter,
			selectedAccepted: acceptedFilter,
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
		negateVal := ""
		if m.authorNegate {
			negateVal = "true"
		}
		_ = m.db.SetConfig("active_author_negate", negateVal)
		_ = m.db.SetConfig("active_reviewer_filter", m.selectedReviewer)
		_ = m.db.SetConfig("active_label_filters", m.selectedLabel)
		_ = m.db.SetConfig("active_draft_filter", m.selectedDraft)
		_ = m.db.SetConfig("active_accepted_filter", m.selectedAccepted)
		return m.loadMRs()()
	}
}

// filterConfigKeys lists the config keys that make up the complete filter state.
var filterConfigKeys = []string{
	"active_repo_filter",
	"active_author_filter",
	"active_author_negate",
	"active_reviewer_filter",
	"active_label_filters",
	"active_draft_filter",
	"active_accepted_filter",
	"active_unread_filter",
}

// saveFavorite saves current filter state to the given slot (1-9).
func (m *mrListModel) saveFavorite(slot int) tea.Cmd {
	return func() tea.Msg {
		prefix := fmt.Sprintf("favorite_%d_", slot)
		for _, k := range filterConfigKeys {
			val, _ := m.db.GetConfig(k)
			// Strip "active_" prefix and prepend favorite prefix.
			suffix := strings.TrimPrefix(k, "active_")
			_ = m.db.SetConfig(prefix+suffix, val)
		}
		return flashMsg{text: fmt.Sprintf("Favorite %d saved", slot)}
	}
}

// recallFavorite restores filter state from the given slot (1-9).
func (m *mrListModel) recallFavorite(slot int) tea.Cmd {
	return func() tea.Msg {
		prefix := fmt.Sprintf("favorite_%d_", slot)
		saved, err := m.db.GetConfigByPrefix(prefix)
		if err != nil || len(saved) == 0 {
			return flashMsg{text: fmt.Sprintf("Favorite %d is empty", slot)}
		}
		for _, k := range filterConfigKeys {
			suffix := strings.TrimPrefix(k, "active_")
			val := saved[prefix+suffix]
			_ = m.db.SetConfig(k, val)
		}
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
			m.authorNegate = false
		} else {
			m.selectedAuthor = value
		}
	case filterGroupReviewer:
		switch value {
		case reviewerAllLabel:
			m.selectedReviewer = ""
		case reviewerUnassignedLabel:
			m.selectedReviewer = reviewerUnassignedSentinel
		default:
			m.selectedReviewer = value
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
	case filterGroupAccepted:
		switch value {
		case "✓ Accepted":
			m.selectedAccepted = "accepted"
		case "— Not accepted":
			m.selectedAccepted = "not_accepted"
		default:
			m.selectedAccepted = ""
		}
	}
}

// cycleFilter cycles the given filter group by delta (+1 or -1) without opening autocomplete.
func (m *mrListModel) cycleFilter(group filterGroup, delta int) {
	var options []string
	var current string
	var allLabel string

	switch group {
	case filterGroupRepo:
		options = m.repoOptions
		current = m.selectedRepo
		allLabel = "All repos"
	case filterGroupAuthor:
		options = m.authorOptions
		current = m.selectedAuthor
		allLabel = "All authors"
	case filterGroupDraft:
		states := []string{"", "drafts", "ready"}
		idx := 0
		for i, s := range states {
			if s == m.selectedDraft {
				idx = i
				break
			}
		}
		idx = (idx + delta + len(states)) % len(states)
		m.selectedDraft = states[idx]
		return
	case filterGroupAccepted:
		states := []string{"", "accepted", "not_accepted"}
		idx := 0
		for i, s := range states {
			if s == m.selectedAccepted {
				idx = i
				break
			}
		}
		idx = (idx + delta + len(states)) % len(states)
		m.selectedAccepted = states[idx]
		return
	default:
		return
	}

	// Build full list: "All" sentinel + actual options.
	full := append([]string{""}, options...)
	idx := 0
	for i, o := range full {
		if o == current {
			idx = i
			break
		}
	}
	idx += delta
	if idx < 0 {
		idx = len(full) - 1
	} else if idx >= len(full) {
		idx = 0
	}
	value := full[idx]
	if value == "" {
		value = allLabel
		if group == filterGroupAuthor {
			m.authorNegate = false
		}
	}
	m.applySelection(group, value)
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
	case filterGroupReviewer:
		options = append([]string{reviewerAllLabel, reviewerUnassignedLabel}, m.reviewerOptions...)
		switch m.selectedReviewer {
		case "":
			current = reviewerAllLabel
		case reviewerUnassignedSentinel:
			current = reviewerUnassignedLabel
		default:
			current = m.selectedReviewer
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
	case filterGroupAccepted:
		options = []string{"All", "✓ Accepted", "— Not accepted"}
		switch m.selectedAccepted {
		case "accepted":
			current = "✓ Accepted"
		case "not_accepted":
			current = "— Not accepted"
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
	case flashMsg:
		m.flash = msg.text
		return root, tea.Tick(2*time.Second, func(time.Time) tea.Msg {
			return clearFlashMsg{}
		})

	case clearFlashMsg:
		m.flash = ""
		return root, nil

	case mrsLoadedMsg:
		if msg.err == nil {
			m.items = msg.items
			m.unreadOnly = msg.unreadOnly
			m.repoOptions = msg.repoOptions
			m.authorOptions = msg.authorOptions
			m.reviewerOptions = msg.reviewerOptions
			m.labelOptions = msg.labelOptions
			m.selectedRepo = msg.selectedRepo
			m.selectedAuthor = msg.selectedAuthor
			m.authorNegate = msg.authorNegate
			m.selectedReviewer = msg.selectedReviewer
			m.selectedLabel = msg.selectedLabel
			m.selectedDraft = msg.selectedDraft
			m.selectedAccepted = msg.selectedAccepted
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

		// Save mode: waiting for slot number.
		if m.saveMode {
			m.saveMode = false
			m.flash = ""
			if k := msg.String(); len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
				slot := int(k[0] - '0')
				return root, m.saveFavorite(slot)
			}
			// Any other key cancels save mode.
			return root, nil
		}

		// Recall favorite (1..9).
		if k := msg.String(); len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			slot := int(k[0] - '0')
			return root, m.recallFavorite(slot)
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

		case key.Matches(msg, Keys.FilterReviewer):
			m.openAutocomplete(filterGroupReviewer)

		case key.Matches(msg, Keys.FilterLabel):
			m.openAutocomplete(filterGroupLabels)

		case key.Matches(msg, Keys.FilterDraft):
			m.cycleFilter(filterGroupDraft, 1)
			return root, m.saveAndReload()

		case key.Matches(msg, Keys.FilterAccepted):
			m.cycleFilter(filterGroupAccepted, 1)
			return root, m.saveAndReload()

		case key.Matches(msg, Keys.CycleRepoNext):
			m.cycleFilter(filterGroupRepo, 1)
			return root, m.saveAndReload()

		case key.Matches(msg, Keys.CycleRepoPrev):
			m.cycleFilter(filterGroupRepo, -1)
			return root, m.saveAndReload()

		case key.Matches(msg, Keys.CycleAuthorNext):
			m.cycleFilter(filterGroupAuthor, 1)
			return root, m.saveAndReload()

		case key.Matches(msg, Keys.CycleAuthorPrev):
			m.cycleFilter(filterGroupAuthor, -1)
			return root, m.saveAndReload()

		case key.Matches(msg, Keys.ToggleAuthorNegate):
			if m.selectedAuthor == "" {
				return root, func() tea.Msg {
					return flashMsg{text: "Select an author first (a) to negate"}
				}
			}
			m.authorNegate = !m.authorNegate
			return root, m.saveAndReload()

		case key.Matches(msg, Keys.ToggleUnread):
			return root, m.toggleUnreadFilter()

		case key.Matches(msg, Keys.Sync):
			if !root.syncing {
				return root, root.startSync()
			}

		case msg.String() == "s":
			m.saveMode = true
			m.flash = "Save to slot 1-9..."
			return root, nil
		}
	}
	return root, nil
}

// view renders the MR list.
func (m *mrListModel) view(root *Model) string {
	var sb strings.Builder

	// Filter bar (3 boxes + optional unread indicator).
	sb.WriteString(m.renderFilterBar(root.width - 2))

	if m.autocomplete != nil {
		// Show autocomplete dropdown instead of MR list.
		acHeight := root.height - 7 // panel border(2) + help(1) + filter bar(4)
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
			titleWidth := root.width - 60
			if titleWidth < 20 {
				titleWidth = 20
			}

			// Filter bar takes 3 box lines.
			visibleRows := root.height - 7
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

				approval := approvalIndicator(item.mr.Approved)
				pipeline := pipelineIndicator(item.mr.PipelineStatus)

				unresolvedStr := "   "
				if item.unresolvedCount > 0 {
					unresolvedStr = unresolvedStyle.Render(fmt.Sprintf("%2d↩", item.unresolvedCount))
				}

				title := truncate(item.mr.Title, titleWidth)
				prefix := fmt.Sprintf(" %-12s !%-4d ", truncate(item.repoName, 12), item.mr.IID)
				titleAuthor := fmt.Sprintf("%-*s @%-12s → ", titleWidth, title, truncate(item.mr.Author, 12))
				reviewerCell := formatReviewerCell(item.reviewers, 5)

				row := prefix + unread + " " + approval + " " + pipeline + "  " + titleAuthor + reviewerCell + " " + unresolvedStr
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
		help = "j/k: navigate  l/enter: select  r: repo  a: author  v: reviewer  L: labels  d: draft  c: accepted  u: unread  1-9/s: presets  R: sync  ?: help  q: quit"
	}
	if m.flash != "" {
		help = pipelineRunning.Render("★ "+m.flash) + "  " + help
	} else if root.syncing && root.syncStatus != "" {
		help = pipelineRunning.Render("⟳ "+root.syncStatus) + "  " + help
	}
	return renderPanel(title, sb.String(), help, root.width, root.height)
}

// renderFilterBar renders the inline filter boxes.
func (m *mrListModel) renderFilterBar(innerWidth int) string {
	narrowBoxWidth := 14                                // narrow box for draft and accepted filters
	remainingWidth := innerWidth - narrowBoxWidth*2 - 5 // 4 wide + 2 narrow = 6 boxes, 5 gaps
	boxWidth := remainingWidth / 4
	if boxWidth < 14 {
		boxWidth = 14
	}

	repoVal := "All repos"
	if m.selectedRepo != "" {
		repoVal = m.selectedRepo
	}
	authorVal := "All authors"
	if m.selectedAuthor != "" {
		authorVal = m.selectedAuthor
	}
	reviewerVal := "All"
	switch m.selectedReviewer {
	case "":
		reviewerVal = "All"
	case reviewerUnassignedSentinel:
		reviewerVal = "— Unassigned"
	default:
		reviewerVal = m.selectedReviewer
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
	acceptedVal := "All"
	switch m.selectedAccepted {
	case "accepted":
		acceptedVal = "✓ Accepted"
	case "not_accepted":
		acceptedVal = "— Not accepted"
	}

	repoActive := m.autocomplete != nil && m.activeFilter == filterGroupRepo
	authorActive := m.autocomplete != nil && m.activeFilter == filterGroupAuthor
	reviewerActive := m.autocomplete != nil && m.activeFilter == filterGroupReviewer
	labelActive := m.autocomplete != nil && m.activeFilter == filterGroupLabels
	draftActive := m.autocomplete != nil && m.activeFilter == filterGroupDraft
	acceptedActive := m.autocomplete != nil && m.activeFilter == filterGroupAccepted

	repoLines := filterBoxLines("Repo", repoVal, "r", boxWidth, repoActive)
	authorTitle := "Author"
	if m.authorNegate {
		authorTitle = "!Author"
	}
	authorLines := filterBoxLines(authorTitle, authorVal, "a", boxWidth, authorActive)
	reviewerLines := filterBoxLines("Reviewer", reviewerVal, "v", boxWidth, reviewerActive)
	labelLines := filterBoxLines("Labels", labelVal, "L", boxWidth, labelActive)
	draftLines := filterBoxLines("Draft", draftVal, "d", narrowBoxWidth, draftActive)
	acceptedLines := filterBoxLines("Acc.", acceptedVal, "c", narrowBoxWidth, acceptedActive)

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
		sb.WriteString(reviewerLines[i])
		sb.WriteString(" ")
		sb.WriteString(labelLines[i])
		sb.WriteString(" ")
		sb.WriteString(draftLines[i])
		sb.WriteString(" ")
		sb.WriteString(acceptedLines[i])
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

// approvalIndicator returns a styled approval status symbol.
func approvalIndicator(approved bool) string {
	if approved {
		return approvalApproved.Render("✓")
	}
	return dimStyle.Render("—")
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

// formatReviewerCell renders the reviewer column for an MR row, padded to
// visibleWidth visible columns. No reviewers shows a dim em-dash; otherwise
// the first reviewer's initials (with "+N" dim suffix for additional
// reviewers).
func formatReviewerCell(reviewers []string, visibleWidth int) string {
	var raw, styled string
	switch len(reviewers) {
	case 0:
		raw = "—"
		styled = dimStyle.Render(raw)
	case 1:
		raw = initials(reviewers[0])
		styled = raw
	default:
		ini := initials(reviewers[0])
		suffix := fmt.Sprintf(" +%d", len(reviewers)-1)
		raw = ini + suffix
		styled = ini + dimStyle.Render(suffix)
	}
	pad := visibleWidth - len([]rune(raw))
	if pad < 0 {
		pad = 0
	}
	return styled + strings.Repeat(" ", pad)
}

// initials returns up to 2 uppercase initials for a username. Segments are
// delimited by ".", "-", "_" or whitespace. Falls back to the first 1-2
// letters of the username if no separators are present.
func initials(username string) string {
	if username == "" {
		return "?"
	}
	parts := strings.FieldsFunc(username, func(r rune) bool {
		return r == '.' || r == '-' || r == '_' || r == ' '
	})
	var out []rune
	for _, p := range parts {
		if p == "" {
			continue
		}
		out = append(out, unicode.ToUpper([]rune(p)[0]))
		if len(out) >= 2 {
			break
		}
	}
	if len(out) == 0 {
		// No separators at all; take up to 2 leading letters.
		for _, r := range username {
			if unicode.IsLetter(r) {
				out = append(out, unicode.ToUpper(r))
				if len(out) >= 2 {
					break
				}
			}
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}

// truncate shortens a string to maxLen runes, appending "…" if truncated.
func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen-1]) + "…"
}
