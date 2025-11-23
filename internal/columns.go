package internal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ────────────────────────────────
// STYLES
// ────────────────────────────────

type Styles struct {
	Title  lipgloss.Style
	Box    lipgloss.Style
	Active lipgloss.Style
	Status lipgloss.Style
	Error  lipgloss.Style // NEW: for red bold error lines
}

func NewStyles() Styles {
	border := lipgloss.RoundedBorder()
	return Styles{
		Title: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		Box:   lipgloss.NewStyle().Border(border).Padding(0, 1).MarginRight(1),
		Active: lipgloss.NewStyle().
			Border(border).
			BorderForeground(lipgloss.Color("#FA8072")). // Not pink, its Salmon obviously
			Padding(0, 1).
			MarginRight(1),
		Status: lipgloss.NewStyle().Foreground(lipgloss.Color("8")).MarginTop(1),
	}
}

// ────────────────────────────────
// GENERIC LIST COLUMN (SCROLLABLE)
// ────────────────────────────────

type renderer[T any] func(T) string

type ListColumn[T any] struct {
	title    string
	items    []T
	selected int
	scroll   int
	width    int
	height   int
	render   renderer[T]
}

func NewListColumn[T any](title string, r renderer[T]) *ListColumn[T] {
	return &ListColumn[T]{title: title, render: r, width: 30, height: 20}
}

func (c *ListColumn[T]) SetItems(items []T) {
	c.items = items
	c.selected = 0
	c.scroll = 0
}

func (c *ListColumn[T]) SetTitle(title string) { c.title = title }

func (c *ListColumn[T]) SetWidth(w int) {
	// w is the total width the app wants to allocate to the box.
	// Subtract 4 for border (2) + padding (2) to get interior content width.
	if w < 4 {
		c.width = 0
		return
	}
	c.width = w - 4
}

func (c *ListColumn[T]) SetHeight(h int) {
	if h > 5 {
		c.height = h - 5
	}
}

func (c *ListColumn[T]) CursorUp() {
	if c.selected > 0 {
		c.selected--
	}
	if c.selected < c.scroll {
		c.scroll = c.selected
	}
}

func (c *ListColumn[T]) CursorDown() {
	if c.selected < len(c.items)-1 {
		c.selected++
	}
	if c.selected >= c.scroll+c.height {
		c.scroll = c.selected - c.height + 1
	}
}

func (c *ListColumn[T]) Selected() (T, bool) {
	var zero T
	if len(c.items) == 0 {
		return zero, false
	}
	return c.items[c.selected], true
}

func (c *ListColumn[T]) View(styles Styles, focused bool) string {
	box := styles.Box
	if focused {
		box = styles.Active
	}

	head := styles.Title.Render(c.title)
	lines := []string{}

	if len(c.items) == 0 {
		lines = append(lines, "(no items)")
	} else {
		start := c.scroll
		end := start + c.height
		if end > len(c.items) {
			end = len(c.items)
		}
		for i := start; i < end; i++ {
			cursor := "  "
			lineText := c.render(c.items[i])
			if i == c.selected {
				cursor = "▸ "
				lineText = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FA8072")). // Not pink, its Salmon obviously
					Bold(true).
					Render(lineText)
			}
			line := fmt.Sprintf("%s%s", cursor, lineText)
			if len(line) > c.width && c.width > 3 {
				line = line[:c.width-3] + "…"
			}
			lines = append(lines, line)
		}
	}

	// Fill remaining lines if fewer than height
	for len(lines) < c.height {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	// IMPORTANT: width = interior content width + 4 (border+padding)
	return box.Width(c.width + 4).Render(head + "\n" + content)
}
