package internal

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"
)

// extractM3U8Lite performs static extraction like yt-dlp,
// logging each step to the provided debug callback (for the F12 panel).
func extractM3U8Lite(embedURL string, log func(string)) (string, error) {
	const origin = "https://embedsports.top"
	const referer = "https://embedsports.top/"
	const ua = "Mozilla/5.0 (X11; Linux x86_64; rv:144.0) Gecko/20100101 Firefox/144.0"

	if log != nil {
		log(fmt.Sprintf("[extractor] Fetching embed page: %s", embedURL))
	}

	body, err := fetchHTML(embedURL, ua, origin, referer, 10*time.Second)
	if err != nil {
		if log != nil {
			log(fmt.Sprintf("[extractor] ❌ fetch failed: %v", err))
		}
		return "", fmt.Errorf("fetch embed: %w", err)
	}

	k, i, s := parseEmbedVars(body)
	if log != nil {
		log(fmt.Sprintf("[extractor] Parsed vars → k=%q i=%q s=%q", k, i, s))
	}

	if k == "" || i == "" || s == "" {
		if log != nil {
			log("[extractor] ❌ missing vars (k,i,s) in embed HTML")
		}
		return "", errors.New("missing vars (k,i,s) in embed HTML")
	}

	if log != nil {
		log("[extractor] Trying to extract secure token from bundle.js")
	}
	token, ok := extractTokenFromJS(origin+"/js/bundle.js", ua, origin, referer, log)
	if ok {
		if log != nil {
			log(fmt.Sprintf("[extractor] Found token: %s", token))
		}
		for lb := 1; lb <= 9; lb++ {
			url := fmt.Sprintf("https://lb%d.strmd.top/secure/%s/%s/stream/%s/%s/playlist.m3u8",
				lb, token, k, i, s)
			if log != nil {
				log(fmt.Sprintf("[extractor] Probing %s", url))
			}
			if probeURL(url) {
				if log != nil {
					log(fmt.Sprintf("[extractor] ✅ Found working m3u8: %s", url))
				}
				return url, nil
			}
		}
		if log != nil {
			log("[extractor] ⚠️ No working load balancer found with token")
		}
	}

	if log != nil {
		log("[extractor] Fallback: probing default tokens")
	}
	for lb := 1; lb <= 9; lb++ {
		testURL := fmt.Sprintf("https://lb%d.strmd.top/secure/test/%s/stream/%s/%s/playlist.m3u8",
			lb, k, i, s)
		if log != nil {
			log(fmt.Sprintf("[extractor] Probing fallback %s", testURL))
		}
		if probeURL(testURL) {
			if log != nil {
				log(fmt.Sprintf("[extractor] ✅ Fallback succeeded: %s", testURL))
			}
			return testURL, nil
		}
	}

	if log != nil {
		log("[extractor] ❌ Unable to derive .m3u8 from embed")
	}
	return "", errors.New("unable to derive .m3u8")
}

// extractTokenFromJS is modified to accept a logger callback
func extractTokenFromJS(url, ua, origin, referer string, log func(string)) (string, bool) {
	if log != nil {
		log(fmt.Sprintf("[extractor] Fetching JS: %s", url))
	}
	body, err := fetchHTML(url, ua, origin, referer, 8*time.Second)
	if err != nil {
		if log != nil {
			log(fmt.Sprintf("[extractor] ⚠️ fetch JS failed: %v", err))
		}
		return "", false
	}

	if m := regexp.MustCompile(`https://lb\d+\.strmd\.top/secure/([A-Za-z0-9]+)/`).FindStringSubmatch(body); len(m) > 1 {
		if log != nil {
			log("[extractor] Found literal /secure/ token in JS")
		}
		return m[1], true
	}

	if m := regexp.MustCompile(`atob\("([A-Za-z0-9+/=]+)"\)`).FindStringSubmatch(body); len(m) > 1 {
		if log != nil {
			log("[extractor] Found base64 token pattern, decoding…")
		}
		decoded, err := base64.StdEncoding.DecodeString(m[1])
		if err == nil {
			tok := reverseString(string(decoded))
			if len(tok) > 16 {
				if log != nil {
					log(fmt.Sprintf("[extractor] Extracted token (decoded+reversed): %s", tok))
				}
				return tok, true
			}
		}
	}

	if m := regexp.MustCompile(`"([A-Za-z0-9]{24,})"\.split\(""\)\.reverse`).FindStringSubmatch(body); len(m) > 1 {
		if log != nil {
			log("[extractor] Found reversed string token")
		}
		return reverseString(m[1]), true
	}

	if log != nil {
		log("[extractor] No token found in JS")
	}
	return "", false
}

// parseEmbedVars extracts var k, i, s from HTML
func parseEmbedVars(html string) (k, i, s string) {
	reK := regexp.MustCompile(`var k\s*=\s*"([^"]+)"`)
	reI := regexp.MustCompile(`var i\s*=\s*"([^"]+)"`)
	reS := regexp.MustCompile(`var s\s*=\s*"([^"]+)"`)

	if m := reK.FindStringSubmatch(html); len(m) > 1 {
		k = m[1]
	}
	if m := reI.FindStringSubmatch(html); len(m) > 1 {
		i = m[1]
	}
	if m := reS.FindStringSubmatch(html); len(m) > 1 {
		s = m[1]
	}
	return
}

// probeURL sends a quick HEAD to see if the URL exists
func probeURL(u string) bool {
	client := &http.Client{Timeout: 4 * time.Second}
	req, _ := http.NewRequest("HEAD", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// reverseString helper
func reverseString(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}