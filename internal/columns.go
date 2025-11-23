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
	Subtle lipgloss.Style
}

func NewStyles() Styles {
	border := lipgloss.RoundedBorder()
	return Styles{
		Title: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		Box:   lipgloss.NewStyle().Border(border).Padding(0, 1),
		Active: lipgloss.NewStyle().
			Border(border).
			BorderForeground(lipgloss.Color("#FA8072")). // Not pink, its Salmon obviously
			Padding(0, 1),
		Status: lipgloss.NewStyle().Foreground(lipgloss.Color("8")).MarginTop(1),
		Error:  lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
		Subtle: lipgloss.NewStyle().Foreground(lipgloss.Color("243")),
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

	separator func(prev, curr T) (string, bool)
}

func NewListColumn[T any](title string, r renderer[T]) *ListColumn[T] {
	return &ListColumn[T]{title: title, render: r, width: 30, height: 20}
}

func (c *ListColumn[T]) SetSeparator(sep func(prev, curr T) (string, bool)) {
	c.separator = sep
}

func truncateToWidth(text string, width int) string {
	if width <= 0 {
		return ""
	}

	if lipgloss.Width(text) <= width {
		return text
	}

	runes := []rune(text)
	total := 0
	for i, r := range runes {
		rWidth := lipgloss.Width(string(r))
		if total+rWidth > width {
			return string(runes[:i])
		}
		total += rWidth
	}

	return text
}

func buildSeparatorLine(label string, width int) string {
	if width <= 0 {
		return label
	}

	trimmed := strings.TrimSpace(label)
	padded := fmt.Sprintf(" %s ", trimmed)
	remaining := width - lipgloss.Width(padded)
	if remaining <= 0 {
		return truncateToWidth(padded, width)
	}

	left := remaining / 2
	right := remaining - left
	return strings.Repeat("─", left) + padded + strings.Repeat("─", right)
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
	if h > 6 {
		c.height = h - 6
	}
}

func (c *ListColumn[T]) CursorUp() {
	if c.selected > 0 {
		c.selected--
	}
	c.ensureSelectedVisible()
}

func (c *ListColumn[T]) CursorDown() {
	if c.selected < len(c.items)-1 {
		c.selected++
	}
	c.ensureSelectedVisible()
}

func (c *ListColumn[T]) Selected() (T, bool) {
	var zero T
	if len(c.items) == 0 {
		return zero, false
	}
	return c.items[c.selected], true
}

type listRow[T any] struct {
	text        string
	isSeparator bool
	itemIndex   int
}

func (c *ListColumn[T]) buildRows() []listRow[T] {
	rows := make([]listRow[T], 0, len(c.items))
	var prev T

	for i, item := range c.items {
		if c.separator != nil {
			if sepText, ok := c.separator(prev, item); ok {
				rows = append(rows, listRow[T]{text: sepText, isSeparator: true, itemIndex: -1})
			}
		}

		rows = append(rows, listRow[T]{text: c.render(item), itemIndex: i})
		prev = item
	}
	return rows
}

func (c *ListColumn[T]) clampScroll(totalRows int) {
	if c.height <= 0 {
		c.scroll = 0
		return
	}

	maxScroll := totalRows - c.height
	if maxScroll < 0 {
		maxScroll = 0
	}
	if c.scroll > maxScroll {
		c.scroll = maxScroll
	}
	if c.scroll < 0 {
		c.scroll = 0
	}
}

func (c *ListColumn[T]) ensureSelectedVisible() {
	if len(c.items) == 0 {
		c.scroll = 0
		return
	}

	rows := c.buildRows()
	selRow := 0
	for idx, row := range rows {
		if row.isSeparator {
			continue
		}
		if row.itemIndex == c.selected {
			selRow = idx
			break
		}
	}

	if c.height <= 0 {
		c.scroll = selRow
		return
	}

	if selRow < c.scroll {
		c.scroll = selRow
	}
	if selRow >= c.scroll+c.height {
		c.scroll = selRow - c.height + 1
	}

	c.clampScroll(len(rows))
}

func (c *ListColumn[T]) View(styles Styles, focused bool) string {
	box := styles.Box
	if focused {
		box = styles.Active
	}

	titleText := fmt.Sprintf("%s (%d)", c.title, len(c.items))
	if focused {
		titleText = fmt.Sprintf("▶ %s", titleText)
	}
	head := styles.Title.Render(titleText)
	meta := styles.Subtle.Render("Waiting for data…")
	lines := []string{}

	if len(c.items) == 0 {
		lines = append(lines, "(no items)")
	} else {
		rows := c.buildRows()
		c.clampScroll(len(rows))

		start := c.scroll
		end := start + c.height
		if end > len(rows) {
			end = len(rows)
		}

		startItem, endItem := -1, -1

		for i := start; i < end; i++ {
			row := rows[i]
			cursor := "  "
			lineText := row.text

			contentWidth := c.width - lipgloss.Width(cursor)

			if row.isSeparator {
				lineText = buildSeparatorLine(lineText, contentWidth)
				lineText = styles.Subtle.Render(lineText)
			} else {
				if contentWidth > 1 && lipgloss.Width(lineText) > contentWidth {
					lineText = fmt.Sprintf("%s…", truncateToWidth(lineText, contentWidth-1))
				}

				if startItem == -1 {
					startItem = row.itemIndex
				}
				endItem = row.itemIndex

				if row.itemIndex == c.selected {
					cursor = "▸ "
					lineText = lipgloss.NewStyle().
						Foreground(lipgloss.Color("#FA8072")). // Not pink, its Salmon obviously
						Bold(true).
						Render(lineText)
				}
			}

			line := fmt.Sprintf("%s%s", cursor, lineText)
			lines = append(lines, line)
		}

		if startItem == -1 {
			startItem = 0
		}
		if endItem == -1 {
			endItem = startItem
		}

		meta = styles.Subtle.Render(fmt.Sprintf("Showing %d–%d of %d", startItem+1, endItem+1, len(c.items)))
	}

	// Fill remaining lines if fewer than height
	for len(lines) < c.height {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	// IMPORTANT: width = interior content width + 4 (border+padding)
	return box.Width(c.width + 4).Render(head + "\n" + meta + "\n" + content)
}
