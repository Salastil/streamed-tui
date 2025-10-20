package internal

import (
	"fmt"

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
}

func NewStyles() Styles {
	border := lipgloss.RoundedBorder()
	return Styles{
		Title:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		Box:    lipgloss.NewStyle().Border(border).Padding(0, 1).MarginRight(1),
		Active: lipgloss.NewStyle().Border(border).BorderForeground(lipgloss.Color("10")).Padding(0, 1).MarginRight(1),
		Status: lipgloss.NewStyle().Foreground(lipgloss.Color("8")).MarginTop(1),
	}
}

// ────────────────────────────────
// GENERIC LIST COLUMN
// ────────────────────────────────

type renderer[T any] func(T) string

type ListColumn[T any] struct {
	title    string
	items    []T
	selected int
	width    int
	render   renderer[T]
}

func NewListColumn[T any](title string, r renderer[T]) *ListColumn[T] {
	return &ListColumn[T]{title: title, render: r, width: 30}
}

func (c *ListColumn[T]) SetItems(items []T) { c.items = items; c.selected = 0 }
func (c *ListColumn[T]) SetTitle(title string) { c.title = title }
func (c *ListColumn[T]) SetWidth(w int) { if w > 20 { c.width = w - 2 } }
func (c *ListColumn[T]) CursorUp() { if c.selected > 0 { c.selected-- } }
func (c *ListColumn[T]) CursorDown() { if c.selected < len(c.items)-1 { c.selected++ } }

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
	content := ""
	for i, it := range c.items {
		cursor := "  "
		if i == c.selected {
			cursor = "▸ "
		}
		line := fmt.Sprintf("%s%s", cursor, c.render(it))
		if len(line) > c.width && c.width > 3 {
			line = line[:c.width-3] + "…"
		}
		content += line + "\n"
	}
	return box.Width(c.width + 2).Render(head + "\n" + content)
}
