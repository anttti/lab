package tui

import (
	"strings"

	"lab/internal/db"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/key"
)

// filterGroup identifies which filter section is active.
type filterGroup int

const (
	filterGroupRepo filterGroup = iota
	filterGroupUser
	filterGroupLabels
)

// filterDataMsg carries data loaded for the filter overlay.
type filterDataMsg struct {
	repos        []db.Repo
	labels       []string
	selectedRepo string
	selectedUser string
	activeLabels map[string]bool
	err          error
}

// filterModel is the filter overlay sub-model.
type filterModel struct {
	db           *db.Database
	group        filterGroup
	repos        []db.Repo
	labels       []string
	cursor       int
	selectedRepo string // "" means "All repos"
	selectedUser string // "" means "All", "me" means "Only me"
	activeLabels map[string]bool
}

func newFilterModel(root *Model) filterModel {
	return filterModel{
		db:           root.db,
		activeLabels: make(map[string]bool),
	}
}

// load reads repos, labels, and current filter state from the config DB.
func (m *filterModel) load() tea.Cmd {
	database := m.db
	return func() tea.Msg {
		repos, err := database.ListRepos()
		if err != nil {
			return filterDataMsg{err: err}
		}
		labels, err := database.AllLabels()
		if err != nil {
			return filterDataMsg{err: err}
		}

		selectedRepo, _ := database.GetConfig("active_repo_filter")
		selectedUser, _ := database.GetConfig("active_user_filter")
		labelFilter, _ := database.GetConfig("active_label_filters")

		activeLabels := make(map[string]bool)
		if labelFilter != "" {
			for _, l := range strings.Split(labelFilter, ",") {
				l = strings.TrimSpace(l)
				if l != "" {
					activeLabels[l] = true
				}
			}
		}

		return filterDataMsg{
			repos:        repos,
			labels:       labels,
			selectedRepo: selectedRepo,
			selectedUser: selectedUser,
			activeLabels: activeLabels,
		}
	}
}

// saveFilters persists the current filter selections to the config DB.
func (m *filterModel) saveFilters() error {
	if err := m.db.SetConfig("active_repo_filter", m.selectedRepo); err != nil {
		return err
	}
	if err := m.db.SetConfig("active_user_filter", m.selectedUser); err != nil {
		return err
	}

	var labels []string
	for l, active := range m.activeLabels {
		if active {
			labels = append(labels, l)
		}
	}
	return m.db.SetConfig("active_label_filters", strings.Join(labels, ","))
}

// repoItems returns display labels for the repo filter group.
func (m *filterModel) repoItems() []string {
	items := []string{"All repos"}
	for _, r := range m.repos {
		items = append(items, r.Name)
	}
	return items
}

// userItems returns display labels for the user filter group.
func (m *filterModel) userItems() []string {
	return []string{"All", "Only me"}
}

// labelItems returns display labels for the labels filter group.
func (m *filterModel) labelItems() []string {
	items := []string{"No filter"}
	return append(items, m.labels...)
}

// currentGroupLen returns the number of items in the active group.
func (m *filterModel) currentGroupLen() int {
	switch m.group {
	case filterGroupRepo:
		return len(m.repoItems())
	case filterGroupUser:
		return len(m.userItems())
	case filterGroupLabels:
		return len(m.labelItems())
	}
	return 0
}

// update handles input for the filter overlay.
func (m *filterModel) update(msg tea.Msg, root *Model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case filterDataMsg:
		if msg.err == nil {
			m.repos = msg.repos
			m.labels = msg.labels
			m.selectedRepo = msg.selectedRepo
			m.selectedUser = msg.selectedUser
			m.activeLabels = msg.activeLabels
		}
		return root, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, Keys.Quit):
			_ = m.saveFilters()
			root.current = viewMRList
			return root, root.mrList.loadMRs()

		case msg.String() == "tab":
			m.group = (m.group + 1) % 3
			m.cursor = 0

		case msg.String() == "shift+tab":
			m.group = (m.group + 2) % 3
			m.cursor = 0

		case key.Matches(msg, Keys.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, Keys.Down):
			if m.cursor < m.currentGroupLen()-1 {
				m.cursor++
			}

		case key.Matches(msg, Keys.Top):
			m.cursor = 0

		case key.Matches(msg, Keys.Bottom):
			n := m.currentGroupLen()
			if n > 0 {
				m.cursor = n - 1
			}

		case msg.String() == " ", key.Matches(msg, Keys.Select):
			m.toggleSelection()

		case key.Matches(msg, Keys.Back):
			// Save and return to MR list.
			_ = m.saveFilters()
			root.current = viewMRList
			return root, root.mrList.loadMRs()
		}
	}
	return root, nil
}

// toggleSelection applies the current cursor selection in the active group.
func (m *filterModel) toggleSelection() {
	switch m.group {
	case filterGroupRepo:
		items := m.repoItems()
		if m.cursor < len(items) {
			if m.cursor == 0 {
				m.selectedRepo = ""
			} else {
				m.selectedRepo = items[m.cursor]
			}
		}

	case filterGroupUser:
		items := m.userItems()
		if m.cursor < len(items) {
			if m.cursor == 0 {
				m.selectedUser = ""
			} else {
				m.selectedUser = "me"
			}
		}

	case filterGroupLabels:
		items := m.labelItems()
		if m.cursor < len(items) {
			if m.cursor == 0 {
				// "No filter" — clear all.
				m.activeLabels = make(map[string]bool)
			} else {
				label := items[m.cursor]
				m.activeLabels[label] = !m.activeLabels[label]
			}
		}
	}
}

// view renders the filter overlay.
func (m *filterModel) view(root *Model) string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Filters"))
	sb.WriteString("\n\n")

	// Repo group.
	repoActive := m.group == filterGroupRepo
	groupLabel := "  Repo"
	if repoActive {
		groupLabel = "◂ Repo"
	}
	sb.WriteString(selectedStyle.Render(groupLabel))
	sb.WriteString("\n")

	repoItems := m.repoItems()
	repoSelected := make([]bool, len(repoItems))
	if m.selectedRepo == "" {
		repoSelected[0] = true
	} else {
		for i, r := range m.repos {
			if r.Name == m.selectedRepo {
				repoSelected[i+1] = true
			}
		}
	}
	renderList(&sb, repoItems, repoSelected, m.cursor, repoActive)
	sb.WriteString("\n")

	// User group.
	userActive := m.group == filterGroupUser
	groupLabel = "  User"
	if userActive {
		groupLabel = "◂ User"
	}
	sb.WriteString(selectedStyle.Render(groupLabel))
	sb.WriteString("\n")

	userItems := m.userItems()
	userSelected := make([]bool, len(userItems))
	if m.selectedUser == "" {
		userSelected[0] = true
	} else {
		userSelected[1] = true
	}
	renderList(&sb, userItems, userSelected, m.cursor, userActive)
	sb.WriteString("\n")

	// Labels group.
	labelsActive := m.group == filterGroupLabels
	groupLabel = "  Labels"
	if labelsActive {
		groupLabel = "◂ Labels"
	}
	sb.WriteString(selectedStyle.Render(groupLabel))
	sb.WriteString("\n")

	labelItems := m.labelItems()
	labelSelected := make([]bool, len(labelItems))
	// "No filter" is selected if no labels are active.
	if len(m.activeLabels) == 0 {
		labelSelected[0] = true
	}
	for i, l := range m.labels {
		if m.activeLabels[l] {
			labelSelected[i+1] = true
		}
	}
	renderList(&sb, labelItems, labelSelected, m.cursor, labelsActive)

	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("tab/shift-tab: switch group  j/k: navigate  space/enter: toggle  esc: save & back"))

	return sb.String()
}

// renderList renders a selectable list into sb.
func renderList(b *strings.Builder, items []string, selected []bool, cursor int, isActive bool) {
	for i, item := range items {
		var prefix string
		if isActive && i == cursor {
			prefix = "▸ "
		} else {
			prefix = "  "
		}

		var selMark string
		if i < len(selected) && selected[i] {
			selMark = "● "
		} else {
			selMark = "○ "
		}

		line := prefix + selMark + item
		if isActive && i == cursor {
			b.WriteString(selectedStyle.Render(line))
		} else if i < len(selected) && selected[i] {
			b.WriteString(resolvedStyle.Render(line))
		} else {
			b.WriteString(dimStyle.Render(line))
		}
		b.WriteString("\n")
	}
}
