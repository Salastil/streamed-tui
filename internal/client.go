package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// ────────────────────────────────
// API DATA TYPES
// ────────────────────────────────

type Client struct {
	base string
	http *http.Client
}

func NewClient(base string, timeout time.Duration) *Client {
	return &Client{
		base: base,
		http: &http.Client{Timeout: timeout},
	}
}

func BaseURLFromEnv() string {
	val := strings.TrimSpace(os.Getenv("STREAMED_BASE"))
	if val == "" {
		val = "https://streamed.pk"
	}
	return strings.TrimRight(val, "/")
}

type Sport struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Team struct {
	Name  string `json:"name"`
	Badge string `json:"badge"`
}

type Teams struct {
	Home *Team `json:"home"`
	Away *Team `json:"away"`
}

type Match struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Date     int64  `json:"date"`
	Poster   string `json:"poster"`
	Popular  bool   `json:"popular"`
	Teams    *Teams `json:"teams"`
	Sources  []struct {
		Source string `json:"source"`
		ID     string `json:"id"`
	} `json:"sources"`

	Viewers int `json:"viewers"`
}

type Stream struct {
	ID       string `json:"id"`
	StreamNo int    `json:"streamNo"`
	Language string `json:"language"`
	HD       bool   `json:"hd"`
	EmbedURL string `json:"embedUrl"`
	Source   string `json:"source"`
	Viewers  int    `json:"viewers"`
}

// ────────────────────────────────
// API CLIENT
// ────────────────────────────────

func (c *Client) GetSports(ctx context.Context) ([]Sport, error) {
	url := c.base + "/api/sports"
	var out []Sport
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) GetPopularMatches(ctx context.Context) ([]Match, error) {
	url := c.base + "/api/matches/all/popular"
	matches, err := c.getMatches(ctx, url)
	if err != nil {
		return nil, err
	}

	viewCounts, err := c.GetPopularViewCounts(ctx)
	if err != nil {
		return nil, err
	}

	for i := range matches {
		// Prefer a direct match on the match ID.
		if viewers, ok := viewCounts.ByMatchID[matches[i].ID]; ok {
			matches[i].Viewers = viewers
			continue
		}

		// Fallback: some IDs can differ between endpoints, so also try source IDs.
		for _, src := range matches[i].Sources {
			if viewers, ok := viewCounts.BySourceID[src.ID]; ok {
				matches[i].Viewers = viewers
				break
			}
		}
	}

	return matches, nil
}

func (c *Client) GetMatchesBySport(ctx context.Context, sportID string) ([]Match, error) {
	url := fmt.Sprintf("%s/api/matches/%s", c.base, sportID)
	return c.getMatches(ctx, url)
}

type PopularViewCounts struct {
	ByMatchID  map[string]int
	BySourceID map[string]int
}

func (c *Client) GetPopularViewCounts(ctx context.Context) (PopularViewCounts, error) {
	url := "https://streami.su/api/matches/live/popular-viewcount"

	var payload []struct {
		ID      string `json:"id"`
		Viewers int    `json:"viewers"`
		Sources []struct {
			ID string `json:"id"`
		} `json:"sources"`
	}

	if err := c.get(ctx, url, &payload); err != nil {
		return PopularViewCounts{}, err
	}

	matchMap := make(map[string]int, len(payload))
	sourceMap := make(map[string]int, len(payload))
	for _, item := range payload {
		matchMap[item.ID] = item.Viewers
		for _, src := range item.Sources {
			if src.ID == "" {
				continue
			}
			sourceMap[src.ID] = item.Viewers
		}
	}

	return PopularViewCounts{ByMatchID: matchMap, BySourceID: sourceMap}, nil
}

func (c *Client) GetStreamsForMatch(ctx context.Context, mt Match) ([]Stream, error) {
	var all []Stream
	for _, src := range mt.Sources {
		url := fmt.Sprintf("%s/api/stream/%s/%s", c.base, src.Source, src.ID)
		var list []Stream
		if err := c.get(ctx, url, &list); err != nil {
			return nil, err
		}
		all = append(all, list...)
	}
	return all, nil
}

func (c *Client) getMatches(ctx context.Context, url string) ([]Match, error) {
	var out []Match
	if err := c.get(ctx, url, &out); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

func (c *Client) get(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "StreamedTUI/1.0 (+https://github.com/Salastil/streamed-tui)")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
