package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keyboard bindings for the TUI.
type KeyMap struct {
	Up, Down, Select, Back, Top, Bottom, FilterRepo, FilterAuthor, FilterReviewer, FilterLabel, FilterDraft, FilterAccepted, CycleRepoNext, CycleRepoPrev, CycleAuthorNext, CycleAuthorPrev, ToggleAuthorNegate, ToggleUnread, Sync, Claude, Next, Prev, Web, Quit key.Binding
}

// Keys holds the default vim-style key bindings.
var Keys = KeyMap{
	Up:             key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
	Down:           key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
	Select:         key.NewBinding(key.WithKeys("l", "enter", "right"), key.WithHelp("l/enter", "select")),
	Back:           key.NewBinding(key.WithKeys("h", "b", "left", "esc"), key.WithHelp("h/b", "back")),
	Top:            key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
	Bottom:         key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
	FilterRepo:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "filter repo")),
	FilterAuthor:   key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "filter author")),
	FilterReviewer: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "filter reviewer")),
	FilterLabel:    key.NewBinding(key.WithKeys("L"), key.WithHelp("L", "filter labels")),
	FilterDraft:    key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "filter draft")),
	FilterAccepted: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "filter accepted")),
	CycleRepoNext:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next repo")),
	CycleRepoPrev:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev repo")),
	CycleAuthorNext:    key.NewBinding(key.WithKeys("]"), key.WithHelp("]", "next author")),
	CycleAuthorPrev:    key.NewBinding(key.WithKeys("["), key.WithHelp("[", "prev author")),
	ToggleAuthorNegate: key.NewBinding(key.WithKeys("!"), key.WithHelp("!", "negate author")),
	ToggleUnread: key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "toggle unread")),
	Sync:         key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "sync")),
	Claude:         key.NewBinding(key.WithKeys("C"), key.WithHelp("C", "claude")),
	Next:         key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next")),
	Prev:         key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev")),
	Web:          key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "web")),
	Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
