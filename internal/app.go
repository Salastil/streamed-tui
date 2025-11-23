package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ────────────────────────────────
// KEYMAP
// ────────────────────────────────

type keyMap struct {
	Up, Down, Left, Right key.Binding
	Enter, Quit, Refresh  key.Binding
	OpenBrowser, OpenMPV  key.Binding
	Help                  key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:          key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:        key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:        key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "focus left")),
		Right:       key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "focus right")),
		Enter:       key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		OpenBrowser: key.NewBinding(key.WithKeys("o"), key.WithHelp("o", "open in browser")),
		OpenMPV:     key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "open in mpv")),
		Quit:        key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Refresh:     key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		Help:        key.NewBinding(key.WithKeys("f1", "?"), key.WithHelp("F1/?", "toggle help")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Left, k.Right, k.Enter, k.OpenBrowser, k.OpenMPV, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Enter, k.OpenBrowser, k.OpenMPV, k.Refresh, k.Help, k.Quit},
	}
}

// ────────────────────────────────
// TYPES & CONSTANTS
// ────────────────────────────────

type (
	sportsLoadedMsg  []Sport
	matchesLoadedMsg struct {
		Matches []Match
		Title   string
	}
	streamsLoadedMsg []Stream
	errorMsg         error
	launchStreamMsg  struct{ URL string }
	debugLogMsg      string
)

type focusCol int
type viewMode int

const (
	focusSports focusCol = iota
	focusMatches
	focusStreams
)

const (
	viewMain viewMode = iota
	viewHelp
)

// ────────────────────────────────
// MODEL
// ────────────────────────────────

type Model struct {
	apiClient   *Client
	styles      Styles
	keys        keyMap
	help        help.Model
	focus       focusCol
	lastError   error
	currentView viewMode

	sports  *ListColumn[Sport]
	matches *ListColumn[Match]
	streams *ListColumn[Stream]

	status        string
	debugLines    []string
	TerminalWidth int
}

// ────────────────────────────────
// ENTRY POINT
// ────────────────────────────────

func Run(debug bool) error {
	p := tea.NewProgram(New(debug), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func New(debug bool) Model {
	base := BaseURLFromEnv()
	client := NewClient(base, 15*time.Second)
	styles := NewStyles()

	m := Model{
		apiClient:   client,
		styles:      styles,
		keys:        defaultKeys(),
		help:        help.New(),
		focus:       focusSports,
		currentView: viewMain,
		debugLines:  []string{},
	}

	if debug {
		m.debugLines = append(m.debugLines, "(debug logging enabled)")
	}

	m.sports = NewListColumn[Sport]("Sports", func(s Sport) string { return s.Name })
	m.matches = NewListColumn[Match]("Popular Matches", func(mt Match) string {
		when := time.UnixMilli(mt.Date).Local().Format("Jan 2 15:04")
		title := mt.Title
		if mt.Teams != nil && mt.Teams.Home != nil && mt.Teams.Away != nil {
			title = fmt.Sprintf("%s vs %s", mt.Teams.Home.Name, mt.Teams.Away.Name)
		}
		return fmt.Sprintf("%s  %s  (%s)", when, title, mt.Category)
	})
	m.streams = NewListColumn[Stream]("Streams", func(st Stream) string {
		quality := "SD"
		if st.HD {
			quality = "HD"
		}
		return fmt.Sprintf("#%d %s (%s) – %s", st.StreamNo, st.Language, quality, st.Source)
	})

	m.status = fmt.Sprintf("Using API %s | Loading sports and matches…", base)
	return m
}

// ────────────────────────────────
// VIEW MANAGEMENT
// ────────────────────────────────

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchSports(), m.fetchPopularMatches())
}

func (m Model) View() string {
	switch m.currentView {
	case viewHelp:
		return m.renderHelpPanel()
	default:
		return m.renderMainView()
	}
}

func (m Model) renderMainView() string {
	cols := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.sports.View(m.styles, m.focus == focusSports),
		m.matches.View(m.styles, m.focus == focusMatches),
		m.streams.View(m.styles, m.focus == focusStreams),
	)
	debugPane := m.renderDebugPane()
	status := m.renderStatusLine()
	return lipgloss.JoinVertical(lipgloss.Left, cols, debugPane, status, m.help.View(m.keys))
}

func (m Model) renderStatusLine() string {
	focusLabel := m.currentFocusLabel()
	statusText := fmt.Sprintf("%s  | Focus: %s (←/→)", m.status, focusLabel)
	if m.lastError != nil {
		return m.styles.Error.Render(fmt.Sprintf("⚠️  %v  | Focus: %s (Esc to dismiss)", m.lastError, focusLabel))
	}
	return m.styles.Status.Render(statusText)
}

func (m Model) currentFocusLabel() string {
	switch m.focus {
	case focusSports:
		return "Sports"
	case focusMatches:
		return "Matches"
	case focusStreams:
		return "Streams"
	default:
		return "Unknown"
	}
}

func (m Model) renderHelpPanel() string {
	header := m.styles.Title.Render("Keybindings Help")
	bindings := [][]string{
		{"↑/↓ or k/j", "Navigate list"},
		{"←/→ or h/l", "Move focus between columns"},
		{"Enter", "Select / Open"},
		{"O", "Open in browser"},
		{"P", "Open in mpv"},
		{"R", "Refresh"},
		{"Q", "Quit"},
		{"F1 / ?", "Toggle this help"},
		{"Esc", "Return to main view"},
	}

	var sb strings.Builder
	sb.WriteString(header + "\n\n")
	for _, b := range bindings {
		sb.WriteString(fmt.Sprintf("%-18s %s\n", b[0], b[1]))
	}
	sb.WriteString("\nPress Esc to return.")

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FA8072")).
		Padding(1, 2).
		Width(int(float64(m.TerminalWidth) * 0.95)).
		Render(sb.String())

	return panel
}

func (m Model) renderDebugPane() string {
	header := m.styles.Title.Render("Debug log")
	visibleLines := 4
	if len(m.debugLines) == 0 {
		m.debugLines = append(m.debugLines, "(debug log empty)")
	}
	start := len(m.debugLines) - visibleLines
	if start < 0 {
		start = 0
	}
	lines := m.debugLines[start:]
	for len(lines) < visibleLines {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")
	width := m.TerminalWidth
	if width == 0 {
		width = 80
	}

	debugWidth := int(float64(width) * 0.95)
	if debugWidth <= 0 {
		debugWidth = width
	}

	return lipgloss.NewStyle().
		Width(debugWidth).
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Render(header + "\n" + content)
}

// ────────────────────────────────
// UPDATE LOOP
// ────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case debugLogMsg:
		m.debugLines = append(m.debugLines, string(msg))
		if len(m.debugLines) > 200 {
			m.debugLines = m.debugLines[len(m.debugLines)-200:]
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.TerminalWidth = msg.Width
		debugPaneHeight := 7
		statusHeight := 1
		helpHeight := 2
		reservedHeight := debugPaneHeight + statusHeight + helpHeight
		usableHeight := msg.Height - reservedHeight
		if usableHeight < 5 {
			usableHeight = 5
		}
		totalAvailableWidth := int(float64(msg.Width) * 0.95)
		borderPadding := 4
		totalBorderSpace := borderPadding * 3
		availableWidth := totalAvailableWidth - totalBorderSpace

		baseUnit := availableWidth / 6
		remainder := availableWidth % 6

		sportsWidth := baseUnit
		matchesWidth := baseUnit * 4
		streamsWidth := baseUnit

		m.sports.SetWidth(sportsWidth + borderPadding)
		m.matches.SetWidth(matchesWidth + remainder + borderPadding)
		m.streams.SetWidth(streamsWidth + borderPadding)

		m.sports.SetHeight(usableHeight)
		m.matches.SetHeight(usableHeight)
		m.streams.SetHeight(usableHeight)
		return m, nil

	case tea.KeyMsg:
		switch {
		case msg.String() == "esc":
			m.currentView = viewMain
			return m, nil

		case key.Matches(msg, m.keys.Help):
			if m.currentView == viewHelp {
				m.currentView = viewMain
			} else {
				m.currentView = viewHelp
			}
			return m, nil
		}

		if m.currentView != viewMain {
			return m, nil
		}

		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, m.keys.Left):
			if m.focus > focusSports {
				m.focus--
			}
			return m, nil

		case key.Matches(msg, m.keys.Right):
			if m.focus < focusStreams {
				m.focus++
			}
			return m, nil

		case key.Matches(msg, m.keys.Up):
			switch m.focus {
			case focusSports:
				m.sports.CursorUp()
			case focusMatches:
				m.matches.CursorUp()
			case focusStreams:
				m.streams.CursorUp()
			}
			return m, nil

		case key.Matches(msg, m.keys.Down):
			switch m.focus {
			case focusSports:
				m.sports.CursorDown()
			case focusMatches:
				m.matches.CursorDown()
			case focusStreams:
				m.streams.CursorDown()
			}
			return m, nil

		case key.Matches(msg, m.keys.Enter):
			switch m.focus {
			case focusSports:
				if sport, ok := m.sports.Selected(); ok {
					m.lastError = nil
					m.status = fmt.Sprintf("Loading matches for %s…", sport.Name)
					m.streams.SetItems(nil)
					return m, m.fetchMatchesForSport(sport)
				}
			case focusMatches:
				if mt, ok := m.matches.Selected(); ok {
					m.lastError = nil
					m.status = fmt.Sprintf("Loading streams for %s…", mt.Title)
					return m, m.fetchStreamsForMatch(mt)
				}
			case focusStreams:
				if st, ok := m.streams.Selected(); ok {
					return m, tea.Batch(
						m.logToUI(fmt.Sprintf("Attempting extractor for %s", st.EmbedURL)),
						m.runExtractor(st),
					)
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.OpenBrowser):
			if m.focus == focusStreams {
				if st, ok := m.streams.Selected(); ok && st.EmbedURL != "" {
					_ = openBrowser(st.EmbedURL)
					m.lastError = nil
					m.status = fmt.Sprintf("🌐 Opened in browser: %s", st.EmbedURL)
				}
			}
			return m, nil
		}
		return m, nil

	case sportsLoadedMsg:
		m.sports.SetItems(msg)
		m.lastError = nil
		m.status = fmt.Sprintf("Loaded %d sports – pick one with Enter or stay on Popular Matches", len(msg))
		return m, nil

	case matchesLoadedMsg:
		m.matches.SetTitle(msg.Title)
		m.matches.SetItems(msg.Matches)
		m.lastError = nil
		m.status = fmt.Sprintf("Loaded %d matches – choose one to load streams", len(msg.Matches))
		return m, nil

	case streamsLoadedMsg:
		m.streams.SetItems(msg)
		m.lastError = nil
		m.status = fmt.Sprintf("Loaded %d streams – Enter to launch mpv, o to open in browser", len(msg))
		m.focus = focusStreams
		return m, nil

	case launchStreamMsg:
		m.lastError = nil
		m.status = fmt.Sprintf("🎥 Launched mpv: %s", msg.URL)
		return m, nil

	case errorMsg:
		m.lastError = msg
		m.status = "Encountered an error while contacting the API"
		return m, nil
	}
	return m, nil
}

// ────────────────────────────────
// FETCHERS
// ────────────────────────────────

func (m Model) fetchSports() tea.Cmd {
	return func() tea.Msg {
		sports, err := m.apiClient.GetSports(context.Background())
		if err != nil {
			return errorMsg(err)
		}
		return sportsLoadedMsg(sports)
	}
}

func (m Model) fetchPopularMatches() tea.Cmd {
	return func() tea.Msg {
		matches, err := m.apiClient.GetPopularMatches(context.Background())
		if err != nil {
			return errorMsg(err)
		}
		return matchesLoadedMsg{Matches: matches, Title: "Popular Matches"}
	}
}

func (m Model) fetchMatchesForSport(s Sport) tea.Cmd {
	return func() tea.Msg {
		matches, err := m.apiClient.GetMatchesBySport(context.Background(), s.ID)
		if err != nil {
			return errorMsg(err)
		}
		return matchesLoadedMsg{Matches: matches, Title: fmt.Sprintf("Matches (%s)", s.Name)}
	}
}

func (m Model) fetchStreamsForMatch(mt Match) tea.Cmd {
	return func() tea.Msg {
		streams, err := m.apiClient.GetStreamsForMatch(context.Background(), mt)
		if err != nil {
			return errorMsg(err)
		}
		return streamsLoadedMsg(streams)
	}
}

// ────────────────────────────────
// EXTRACTOR (chromedp integration)
// ────────────────────────────────

func (m Model) runExtractor(st Stream) tea.Cmd {
	return func() tea.Msg {
		if st.EmbedURL == "" {
			return debugLogMsg("Extractor aborted: empty embed URL")
		}

		logcb := func(line string) {
			m.debugLines = append(m.debugLines, line)
			if len(m.debugLines) > 200 {
				m.debugLines = m.debugLines[len(m.debugLines)-200:]
			}
		}

		logcb(fmt.Sprintf("[extractor] Starting puppeteer extractor for %s", st.EmbedURL))

		m3u8, hdrs, err := extractM3U8Lite(st.EmbedURL, func(line string) {
			m.debugLines = append(m.debugLines, line)
		})
		if err != nil {
			logcb(fmt.Sprintf("[extractor] ❌ %v", err))
			return debugLogMsg(fmt.Sprintf("Extractor failed: %v", err))
		}

		logcb(fmt.Sprintf("[extractor] ✅ Found M3U8: %s", m3u8))
		if len(hdrs) > 0 {
			logcb(fmt.Sprintf("[extractor] Captured %d headers", len(hdrs)))
		}

		if err := LaunchMPVWithHeaders(m3u8, hdrs, logcb, false); err != nil {
			logcb(fmt.Sprintf("[mpv] ❌ %v", err))
			return debugLogMsg(fmt.Sprintf("MPV error: %v", err))
		}

		logcb(fmt.Sprintf("[mpv] ▶ Streaming started for %s", st.EmbedURL))
		return debugLogMsg("Extractor completed successfully")
	}
}

// ────────────────────────────────
// LOG TO UI
// ────────────────────────────────

func (m Model) logToUI(line string) tea.Cmd {
	return func() tea.Msg {
		return debugLogMsg(line)
	}
}
