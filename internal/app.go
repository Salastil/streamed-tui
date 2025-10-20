package internal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Salastil/streamed-tui/internal/api"
)

// ────────────────────────────────
// KEY MAP
// ────────────────────────────────

type keyMap struct {
	Up, Down, Left, Right key.Binding
	Enter, Quit, Refresh  key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Left:    key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "focus left")),
		Right:   key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "focus right")),
		Enter:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
		Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
}

// ────────────────────────────────
// MODEL
// ────────────────────────────────

type focusCol int

const (
	focusSports focusCol = iota
	focusMatches
	focusStreams
)

type Model struct {
	apiClient *api.Client

	styles Styles
	keys   keyMap
	help   help.Model
	focus  focusCol

	sports  *ListColumn[api.Sport]
	matches *ListColumn[api.Match]
	streams *ListColumn[api.Stream]

	status string
}

// ────────────────────────────────
// APP ENTRYPOINT
// ────────────────────────────────

func Run() error {
	p := tea.NewProgram(New(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func New() Model {
	base := api.BaseURLFromEnv()
	client := api.NewClient(base, 15*time.Second)

	styles := NewStyles()
	m := Model{
		apiClient: client,
		styles:    styles,
		keys:      defaultKeys(),
		help:      help.New(),
		focus:     focusSports,
	}

	m.sports = NewListColumn[api.Sport]("Sports", func(s api.Sport) string { return s.Name })
	m.matches = NewListColumn[api.Match]("Popular Matches", func(mt api.Match) string {
		when := time.UnixMilli(mt.Date).Local().Format("Jan 2 15:04")
		title := mt.Title
		if mt.Teams != nil && mt.Teams.Home != nil && mt.Teams.Away != nil {
			title = fmt.Sprintf("%s vs %s", mt.Teams.Home.Name, mt.Teams.Away.Name)
		}
		return fmt.Sprintf("%s  %s  (%s)", when, title, mt.Category)
	})
	m.streams = NewListColumn[api.Stream]("Streams", func(st api.Stream) string {
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

// ────────────────────────────────
// VIEW
// ────────────────────────────────

func (m Model) View() string {
	cols := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.sports.View(m.styles, m.focus == focusSports),
		m.matches.View(m.styles, m.focus == focusMatches),
		m.streams.View(m.styles, m.focus == focusStreams),
	)
	status := m.styles.Status.Render(m.status)
	return lipgloss.JoinVertical(lipgloss.Left, cols, status, m.help.View(m.keys))
}

// ────────────────────────────────
// UPDATE
// ────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.sports.SetWidth(msg.Width / 3)
		m.matches.SetWidth(msg.Width / 3)
		m.streams.SetWidth(msg.Width / 3)
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
		m.status = fmt.Sprintf("Error: %v", msg)
		return m, nil
	}
	return m, nil
}

// ────────────────────────────────
// COMMANDS
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

func (m Model) fetchMatchesForSport(s api.Sport) tea.Cmd {
	return func() tea.Msg {
		matches, err := m.apiClient.GetMatchesBySport(context.Background(), s.ID)
		if err != nil {
			return errorMsg(err)
		}
		return matchesLoadedMsg{Matches: matches, Title: fmt.Sprintf("Matches (%s)", s.Name)}
	}
}

func (m Model) fetchStreamsForMatch(mt api.Match) tea.Cmd {
	return func() tea.Msg {
		streams, err := m.apiClient.GetStreamsForMatch(context.Background(), mt)
		if err != nil {
			return errorMsg(err)
		}
		return streamsLoadedMsg(streams)
	}
}

func (m Model) launchMPV(st api.Stream) tea.Cmd {
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
