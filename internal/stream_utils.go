package internal

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// openBrowser tries to open the embed URL in the system browser.
func openBrowser(link string) error {
	if link == "" {
		return errors.New("empty URL")
	}
	return exec.Command("xdg-open", link).Start()
}

// deriveHeaders guesses Origin, Referer, and User-Agent based on known embed domains.
func deriveHeaders(embed string) (origin, referer, ua string, err error) {
	if embed == "" {
		return "", "", "", errors.New("empty embed url")
	}

	u, err := url.Parse(embed)
	if err != nil {
		return "", "", "", fmt.Errorf("parse url: %w", err)
	}

	host := u.Host
	if strings.Contains(host, "embedsports") {
		origin = "https://embedsports.top"
		referer = "https://embedsports.top/"
	} else {
		origin = fmt.Sprintf("https://%s", host)
		referer = fmt.Sprintf("https://%s/", host)
	}

	ua = "Mozilla/5.0 (X11; Linux x86_64; rv:144.0) Gecko/20100101 Firefox/144.0"
	return origin, referer, ua, nil
}

// fetchHTML performs a GET request with proper headers and returns body text.
func fetchHTML(embed, ua, origin, referer string, timeout time.Duration) (string, error) {
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", embed, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Origin", origin)
	req.Header.Set("Referer", referer)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// extractM3U8 uses regex to find an .m3u8 playlist link from an embed page.
func extractM3U8(html string) string {
	re := regexp.MustCompile(`https?://[^\s'"]+\.m3u8[^\s'"]*`)
	m := re.FindString(html)
	return strings.TrimSpace(m)
}

// launchMPV executes mpv with all the necessary HTTP headers.
func launchMPV(m3u8, ua, origin, referer string) error {
	args := []string{
		"--no-terminal",
		"--really-quiet",
		fmt.Sprintf(`--http-header-fields=User-Agent: %s`, ua),
		fmt.Sprintf(`--http-header-fields=Origin: %s`, origin),
		fmt.Sprintf(`--http-header-fields=Referer: %s`, referer),
		m3u8,
	}

	cmd := exec.Command("mpv", args...)
	return cmd.Start()
}
