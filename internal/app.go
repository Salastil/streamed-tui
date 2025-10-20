package internal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

//
// ────────────────────────────────
// KEY MAP + HELP
// ────────────────────────────────
//

type keyMap struct {
	Up, Down, Left, Right key.Binding
	Enter, Quit, Refresh  key.Binding
	OpenBrowser, OpenMPV  key.Binding
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
	}
}

// implement help.KeyMap interface
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Left, k.Right, k.Enter, k.OpenBrowser, k.OpenMPV, k.Quit}
}
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Enter, k.OpenBrowser, k.OpenMPV, k.Refresh, k.Quit},
	}
}

//
// ────────────────────────────────
// MESSAGE TYPES
// ────────────────────────────────
//

type (
	sportsLoadedMsg []Sport
	matchesLoadedMsg struct {
		Matches []Match
		Title   string
	}
	streamsLoadedMsg []Stream
	errorMsg         error
	launchStreamMsg  struct{ URL string }
)

//
// ────────────────────────────────
// MODEL
// ────────────────────────────────
//

type focusCol int

const (
	focusSports focusCol = iota
	focusMatches
	focusStreams
)

type Model struct {
	apiClient *Client

	styles    Styles
	keys      keyMap
	help      help.Model
	focus     focusCol
	lastError error

	sports  *ListColumn[Sport]
	matches *ListColumn[Match]
	streams *ListColumn[Stream]

	status string
}

//
// ────────────────────────────────
// APP ENTRYPOINT
// ────────────────────────────────
//

func Run() error {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func New() Model {
	base := BaseURLFromEnv()
	client := NewClient(base, 15*time.Second)

	styles := NewStyles()
	m := Model{
		apiClient: client,
		styles:    styles,
		keys:      defaultKeys(),
		help:      help.New(),
		focus:     focusSports,
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

	m.status = fmt.Sprintf("Using API %s | Loading…", base)
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchSports(), m.fetchPopularMatches())
}

//
// ────────────────────────────────
// VIEW
// ────────────────────────────────
//

func (m Model) dynamicHelp() string {
	switch m.focus {
	case focusSports, focusMatches:
		return m.help.View(keyMap{
			Up: m.keys.Up, Down: m.keys.Down, Left: m.keys.Left, Right: m.keys.Right,
			Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
			OpenBrowser: m.keys.OpenBrowser, OpenMPV: m.keys.OpenMPV,
			Quit: m.keys.Quit, Refresh: m.keys.Refresh,
		})
	case focusStreams:
		return m.help.View(keyMap{
			Up: m.keys.Up, Down: m.keys.Down, Left: m.keys.Left, Right: m.keys.Right,
			Enter: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "mpv / browser")),
			OpenBrowser: m.keys.OpenBrowser, OpenMPV: m.keys.OpenMPV,
			Quit: m.keys.Quit, Refresh: m.keys.Refresh,
		})
	default:
		return m.help.View(m.keys)
	}
}

func (m Model) View() string {
	cols := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.sports.View(m.styles, m.focus == focusSports),
		m.matches.View(m.styles, m.focus == focusMatches),
		m.streams.View(m.styles, m.focus == focusStreams),
	)
	
	status := m.styles.Status.Render(m.status)
	if m.lastError != nil {
		status = m.styles.Error.Render(fmt.Sprintf("⚠️  %v", m.lastError))
	}
	return lipgloss.JoinVertical(lipgloss.Left, cols, status, m.dynamicHelp())
}

//
// ────────────────────────────────
// UPDATE
// ────────────────────────────────
//

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve space for help and status at bottom
		usableHeight := int(float64(msg.Height) * 0.9)
		
		// Use 85% of terminal width to leave breathing room on the right
		totalAvailableWidth := int(float64(msg.Width) * 0.97)
		
		// Each column needs 4 chars for border (2) + padding (2)
		borderPadding := 4
		
		// Calculate width per column
		// We have 3 columns, each needs borderPadding space
		totalBorderSpace := borderPadding * 3
		availableWidth := totalAvailableWidth - totalBorderSpace
		
		// Divide available width equally among 3 columns
		colWidth := availableWidth / 3
		
		// Give any remainder to the last column
		remainder := availableWidth % 3
		
		m.sports.SetWidth(colWidth + borderPadding)
		m.matches.SetWidth(colWidth + borderPadding)
		m.streams.SetWidth(colWidth + remainder + borderPadding)

		m.sports.SetHeight(usableHeight)
		m.matches.SetHeight(usableHeight)
		m.streams.SetHeight(usableHeight)
		return m, nil

	case tea.KeyMsg:
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
					m.status = fmt.Sprintf("Loading matches for %s…", sport.Name)
					m.streams.SetItems(nil)
					return m, m.fetchMatchesForSport(sport)
				}
			case focusMatches:
				if mt, ok := m.matches.Selected(); ok {
					m.status = fmt.Sprintf("Loading streams for %s…", mt.Title)
					return m, m.fetchStreamsForMatch(mt)
				}
			case focusStreams:
				if st, ok := m.streams.Selected(); ok {
					return m, m.launchMPV(st)
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.OpenBrowser):
			if m.focus == focusStreams {
				if st, ok := m.streams.Selected(); ok && st.EmbedURL != "" {
					_ = openBrowser(st.EmbedURL)
					m.status = fmt.Sprintf("🌐 Opened in browser: %s", st.EmbedURL)
				}
			}
			return m, nil

		case key.Matches(msg, m.keys.OpenMPV):
			if m.focus == focusStreams {
				if st, ok := m.streams.Selected(); ok {
					go func(st Stream) {
						if err := m.forceMPVLaunch(st); err != nil {
							m.lastError = err
						}
					}(st)
					m.status = fmt.Sprintf("🎞️ Attempting mpv: %s", st.EmbedURL)
				}
			}
			return m, nil
		}
		return m, nil

	case sportsLoadedMsg:
		m.sports.SetItems(msg)
		m.status = fmt.Sprintf("Loaded %d sports", len(msg))
		return m, nil

	case matchesLoadedMsg:
		m.matches.SetTitle(msg.Title)
		m.matches.SetItems(msg.Matches)
		m.status = fmt.Sprintf("Loaded %d matches", len(msg.Matches))
		return m, nil

	case streamsLoadedMsg:
		m.streams.SetItems(msg)
		m.status = fmt.Sprintf("Loaded %d streams", len(msg))
		m.focus = focusStreams
		return m, nil

	case launchStreamMsg:
		m.status = fmt.Sprintf("🎥 Launched mpv: %s", msg.URL)
		return m, nil

	case errorMsg:
		m.lastError = msg
		return m, nil
	}
	return m, nil
}

//
// ────────────────────────────────
// COMMANDS
// ────────────────────────────────
//

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

func (m Model) launchMPV(st Stream) tea.Cmd {
	return func() tea.Msg {
		url := st.EmbedURL
		if url == "" {
			return errorMsg(fmt.Errorf("empty embedUrl for stream %s", st.ID))
		}
		cmd := exec.Command("mpv", "--no-terminal", "--really-quiet", url)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Start()
		return launchStreamMsg{URL: url}
	}
}

// forceMPVLaunch attempts to extract .m3u8 and open it in mpv directly.
func (m Model) forceMPVLaunch(st Stream) error {
	embed := strings.TrimSpace(st.EmbedURL)
	if embed == "" {
		return fmt.Errorf("no embed URL for stream %s", st.ID)
	}

	origin, referer, ua, err := deriveHeaders(embed)
	if err != nil {
		return fmt.Errorf("bad embed URL: %w", err)
	}

	body, err := fetchHTML(embed, ua, origin, referer, 12*time.Second)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}

	m3u8 := extractM3U8(body)
	if m3u8 == "" {
		return fmt.Errorf("no .m3u8 found in embed page")
	}

	if err := launchMPV(m3u8, ua, origin, referer); err != nil {
		return fmt.Errorf("mpv launch failed: %w", err)
	}

	return nil
}