package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keyboard bindings for the TUI.
type KeyMap struct {
	Up, Down, Select, Back, Top, Bottom, Filter, Sync, Claude, Next, Prev, Quit key.Binding
}

// Keys holds the default vim-style key bindings.
var Keys = KeyMap{
	Up:     key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/↑", "up")),
	Down:   key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/↓", "down")),
	Select: key.NewBinding(key.WithKeys("l", "enter", "right"), key.WithHelp("l/enter", "select")),
	Back:   key.NewBinding(key.WithKeys("h", "b", "left", "esc"), key.WithHelp("h/b", "back")),
	Top:    key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
	Bottom: key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
	Filter: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "filter")),
	Sync:   key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "sync")),
	Claude: key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "claude")),
	Next:   key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "next")),
	Prev:   key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "prev")),
	Quit:   key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}
