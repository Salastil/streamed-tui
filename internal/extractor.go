package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// extractM3U8Lite loads an embed page in headless Chrome via chromedp,
// runs any JavaScript, and extracts the final .m3u8 URL and its HTTP headers.
// It streams live log lines via the provided log callback.
func extractM3U8Lite(embedURL string, log func(string)) (string, map[string]string, error) {
	if log == nil {
		log = func(string) {}
	}

	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	// Capture the first .m3u8 request Chrome makes
	type capture struct {
		URL     string
		Headers map[string]string
	}
	found := make(chan capture, 1)

	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if e, ok := ev.(*network.EventRequestWillBeSent); ok {
			u := e.Request.URL
			if strings.Contains(u, ".m3u8") {
				h := make(map[string]string)
				for k, v := range e.Request.Headers {
					if s, ok := v.(string); ok {
						h[k] = s
					}
				}
				select {
				case found <- capture{URL: u, Headers: h}:
				default:
				}
			}
		}
	})

	log(fmt.Sprintf("[chromedp] launching Chrome for %s", embedURL))

	// Set reasonable headers for navigation
	headers := network.Headers{
		"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) Gecko/20100101 Firefox/144.0",
	}
	ctxTimeout, cancelNav := context.WithTimeout(ctx, 30*time.Second)
	defer cancelNav()

	if err := chromedp.Run(ctxTimeout,
		network.Enable(),
		network.SetExtraHTTPHeaders(headers),
		chromedp.Navigate(embedURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		log(fmt.Sprintf("[chromedp] navigation error: %v", err))
		return "", nil, err
	}

	log("[chromedp] page loaded, waiting for .m3u8 network requests...")

	select {
	case cap := <-found:
		log(fmt.Sprintf("[chromedp] ✅ found .m3u8 via network: %s", cap.URL))
		log(fmt.Sprintf("[chromedp] captured %d headers", len(cap.Headers)))
		return cap.URL, cap.Headers, nil
	case <-time.After(12 * time.Second):
		log("[chromedp] timeout waiting for .m3u8 request, attempting DOM fallback...")
	}

	// DOM fallback: look for <video> src or inline JS with a URL
	var candidate string
	if err := chromedp.Run(ctx,
		chromedp.EvaluateAsDevTools(`(function(){
			try {
				const v = document.querySelector('video');
				if(v){
					if(v.currentSrc) return v.currentSrc;
					if(v.src) return v.src;
					const s = v.querySelector('source');
					if(s && s.src) return s.src;
				}
				const html = document.documentElement.innerHTML;
				const match = html.match(/https?:\/\/[^'"\s]+\.m3u8[^'"\s]*/i);
				if(match) return match[0];
			}catch(e){}
			return '';
		})()`, &candidate),
	); err != nil {
		log(fmt.Sprintf("[chromedp] DOM evaluation error: %v", err))
	}

	candidate = strings.TrimSpace(candidate)
	if candidate != "" && strings.Contains(candidate, ".m3u8") {
		log(fmt.Sprintf("[chromedp] ✅ found .m3u8 via DOM: %s", candidate))
		return candidate, map[string]string{}, nil
	}

	log("[chromedp] ❌ failed to find .m3u8 via network or DOM")
	return "", nil, errors.New("m3u8 not found")
}

// LaunchMPVWithHeaders spawns mpv to play the given M3U8 URL,
// reusing all captured HTTP headers to mimic browser playback.
// Logs are streamed via the provided callback.
func LaunchMPVWithHeaders(m3u8 string, hdrs map[string]string, log func(string)) error {
	if log == nil {
		log = func(string) {}
	}
	if m3u8 == "" {
		return fmt.Errorf("empty m3u8 URL")
	}

	args := []string{"--no-terminal", "--really-quiet"}

	for k, v := range hdrs {
		if k == "" || v == "" {
			continue
		}
		switch strings.ToLower(k) {
		case "accept-encoding", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-dest",
			"sec-ch-ua", "sec-ch-ua-platform", "sec-ch-ua-mobile":
			continue // ignore internal Chromium headers
		}
		args = append(args, fmt.Sprintf("--http-header-fields=%s: %s", k, v))
	}

	args = append(args, m3u8)
	log(fmt.Sprintf("[mpv] launching with %d headers: %s", len(hdrs), m3u8))

	cmd := exec.Command("mpv", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log(fmt.Sprintf("[mpv] launch error: %v", err))
		return err
	}

	log(fmt.Sprintf("[mpv] started (pid %d)", cmd.Process.Pid))
	return nil
}
