package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type puppeteerResult struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Browser string            `json:"browser"`
}

func ensurePuppeteerAvailable() error {
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("node executable not found: %w", err)
	}

	// Verify both puppeteer-extra and the stealth plugin are available from the
	// project directory so the temporary runner can load them relative to cwd.
	requireScript := strings.Join([]string{
		"const { createRequire } = require('module');",
		"const req = createRequire(process.cwd() + '/');",
		"req.resolve('puppeteer-extra/package.json');",
		"req.resolve('puppeteer-extra-plugin-stealth/package.json');",
	}, "")

	check := exec.Command("node", "-e", requireScript)
	if err := check.Run(); err != nil {
		return fmt.Errorf("puppeteer-extra or stealth plugin missing. Run `npm install puppeteer-extra puppeteer-extra-plugin-stealth puppeteer` in the project directory: %w", err)
	}

	return nil
}

// extractM3U8Lite invokes a small Puppeteer runner that loads the embed page,
// watches for .m3u8 requests, and returns the first match plus its request
// headers.
func extractM3U8Lite(embedURL string, log func(string)) (string, map[string]string, error) {
	if log == nil {
		log = func(string) {}
	}

	if strings.TrimSpace(embedURL) == "" {
		return "", nil, errors.New("empty embed URL")
	}

	if err := ensurePuppeteerAvailable(); err != nil {
		return "", nil, err
	}

	runnerPath, err := writePuppeteerRunner()
	if err != nil {
		return "", nil, err
	}
	defer os.Remove(runnerPath)

	log(fmt.Sprintf("[puppeteer] launching chromium stealth runner for %s", embedURL))

	cmd := exec.Command("node", runnerPath, embedURL)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log(fmt.Sprintf("[puppeteer] runner error: %s", strings.TrimSpace(stderr.String())))
		return "", nil, fmt.Errorf("puppeteer runner failed: %w", err)
	}

	var res puppeteerResult
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		log(fmt.Sprintf("[puppeteer] decode error: %v", err))
		return "", nil, err
	}

	if res.URL == "" {
		if stderr.Len() > 0 {
			log(strings.TrimSpace(stderr.String()))
		}
		return "", nil, errors.New("m3u8 not found")
	}

	log(fmt.Sprintf("[puppeteer] ✅ found .m3u8 via %s: %s", res.Browser, res.URL))
	return res.URL, res.Headers, nil
}

// writePuppeteerRunner materializes a temporary Node.js script that performs
// the actual page load and .m3u8 discovery with puppeteer-extra stealth
// protections.
func writePuppeteerRunner() (string, error) {
	script := `const { createRequire } = require('module');
const requireFromCwd = createRequire(process.cwd() + '/');

let puppeteer;
let StealthPlugin;
try {
  puppeteer = requireFromCwd('puppeteer-extra');
  StealthPlugin = requireFromCwd('puppeteer-extra-plugin-stealth');
  puppeteer.use(StealthPlugin());
} catch (err) {
  console.error('[puppeteer] required packages missing. install with "npm install puppeteer-extra puppeteer-extra-plugin-stealth puppeteer" in the project directory.');
  process.exit(1);
}

const embedURL = process.argv[2];
const timeoutMs = 45000;

if (!embedURL) {
  console.error('missing embed URL');
  process.exit(1);
}

const viewport = { width: 1280, height: 720 };
const launchArgs = ['--disable-blink-features=AutomationControlled', '--no-sandbox', '--disable-web-security', '--window-size=1920,1080'];
const userAgent = 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36';

async function launchBrowser() {
  const chromiumOptions = {
    headless: 'new',
    args: launchArgs,
    defaultViewport: viewport,
  };
  const browser = await puppeteer.launch(chromiumOptions);
  return { browser, flavor: 'chromium' };
}

function installTouchAndWindowSpoofing(page) {
  return page.evaluateOnNewDocument(() => {
    const { width, height } = window.screen || { width: 1920, height: 1080 };
    Object.defineProperty(navigator, 'maxTouchPoints', { get: () => 1 });
    Object.defineProperty(navigator, 'platform', { get: () => 'Linux x86_64' });
    Object.defineProperty(navigator, 'hardwareConcurrency', { get: () => 8 });
    Object.defineProperty(window, 'outerWidth', { get: () => width });
    Object.defineProperty(window, 'outerHeight', { get: () => height });
  });
}

(async () => {
  const { browser, flavor } = await launchBrowser();
  const page = await browser.newPage();
  await installTouchAndWindowSpoofing(page);

  await page.setUserAgent(userAgent);
  await page.setViewport(viewport);
  await page.setExtraHTTPHeaders({
    'accept-language': 'en-US,en;q=0.9',
    'sec-fetch-site': 'same-origin',
    'sec-fetch-mode': 'navigate',
    'sec-fetch-user': '?1',
    'sec-fetch-dest': 'document',
    'sec-ch-ua': '"Chromium";v="124", "Not=A?Brand";v="99", "Google Chrome";v="124"',
    'sec-ch-ua-platform': 'Linux',
    'sec-ch-ua-mobile': '?0',
  });

  let captured = null;
  const capturePromise = new Promise(resolve => {
    page.on('request', req => {
      const url = req.url();
      if (!captured && url.includes('.m3u8')) {
        const headers = req.headers();
        captured = { url, headers };
        resolve();
      }
    });
  });

  try {
    await page.goto(embedURL, { waitUntil: 'networkidle2', timeout: timeoutMs });
    await page.waitForNavigation({ waitUntil: 'networkidle2', timeout: timeoutMs }).catch(() => {});
  } catch (err) {
    console.error('[puppeteer] navigation warning: ' + err.message);
  }

  await Promise.race([
    capturePromise,
    new Promise(resolve => setTimeout(resolve, 20000)),
  ]);

  if (!captured) {
    const candidate = await page.evaluate(() => {
      try {
        const video = document.querySelector('video');
        if (video) {
          if (video.currentSrc) return video.currentSrc;
          if (video.src) return video.src;
          const source = video.querySelector('source');
          if (source && source.src) return source.src;
        }
        const html = document.documentElement.innerHTML;
        const match = html.match(/https?:\/\/[^'"\s]+\.m3u8[^'"\s]*/i);
        if (match) return match[0];
      } catch (e) {}
      return '';
    });
    if (candidate && candidate.includes('.m3u8')) {
      captured = { url: candidate, headers: {} };
    }
  }

  if (captured) {
    // Enrich headers with cookies and referer if missing.
    const cookies = await page.cookies();
    if (cookies && cookies.length > 0) {
      const cookieHeader = cookies.map(c => c.name + '=' + c.value).join('; ');
      if (!captured.headers) captured.headers = {};
      captured.headers['cookie'] = captured.headers['cookie'] || cookieHeader;
    }
    captured.headers = captured.headers || {};
    captured.headers['user-agent'] = userAgent;
    captured.headers['referer'] = captured.headers['referer'] || embedURL;
    try {
      const origin = new URL(embedURL).origin;
      captured.headers['origin'] = captured.headers['origin'] || origin;
    } catch (e) {}
  }

  await browser.close();

  const output = captured || { url: '', headers: {} };
  output.browser = flavor;
  console.log(JSON.stringify(output));
})().catch(err => {
  console.error(err.stack || err.message);
  process.exit(1);
});
`

	dir := os.TempDir()
	path := filepath.Join(dir, fmt.Sprintf("puppeteer-runner-%d.js", time.Now().UnixNano()))
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// lookupHeaderValue returns the first header value matching name, using a
// case-insensitive comparison for keys sourced from the Puppeteer request map.
func lookupHeaderValue(hdrs map[string]string, name string) string {
	for k, v := range hdrs {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

// LaunchMPVWithHeaders spawns mpv to play the given M3U8 URL using the minimal
// header set required for successful playback (User-Agent, Origin, Referer).
// When attachOutput is true, mpv stays attached to the current terminal and the
// call blocks until the player exits; otherwise mpv is started quietly so the
// TUI can continue running. Logs are streamed via the provided callback.
func LaunchMPVWithHeaders(m3u8 string, hdrs map[string]string, log func(string), attachOutput bool) error {
	if log == nil {
		log = func(string) {}
	}
	if m3u8 == "" {
		return fmt.Errorf("empty m3u8 URL")
	}

	args := []string{}
	if !attachOutput {
		args = append(args, "--no-terminal", "--really-quiet")
	}

	// Only forward the minimal headers mpv requires to mirror the working
	// curl→mpv handoff: User-Agent, Origin, and Referer. Extra headers
	// captured in the browser session can cause mpv to reject the request
	// or send malformed values when duplicated, so we constrain the set
	// explicitly and tolerate case-insensitive keys from Puppeteer.
	headerKeys := []struct {
		lookup  string
		display string
	}{
		{lookup: "user-agent", display: "User-Agent"},
		{lookup: "origin", display: "Origin"},
		{lookup: "referer", display: "Referer"},
	}
	headerCount := 0
	for _, hk := range headerKeys {
		if v := lookupHeaderValue(hdrs, hk.lookup); v != "" {
			args = append(args, fmt.Sprintf("--http-header-fields=%s: %s", hk.display, v))
			headerCount++
		}
	}

	args = append(args, m3u8)
	log(fmt.Sprintf("[mpv] launching with %d headers: %s", headerCount, m3u8))

	cmd := exec.Command("mpv", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log(fmt.Sprintf("[mpv] launch error: %v", err))
		return err
	}

	if attachOutput {
		log("[mpv] started (attached)")
		if err := cmd.Wait(); err != nil {
			log(fmt.Sprintf("[mpv] exited with error: %v", err))
			return err
		}
		log("[mpv] exited")
		return nil
	}

	log(fmt.Sprintf("[mpv] started (pid %d)", cmd.Process.Pid))
	return nil
}

// RunExtractorCLI provides a non-TUI entry point to run the extractor directly
// from the command line ("-e <embedURL>"). When debug is true, verbose output
// from the Puppeteer runner and mpv launch is printed to stdout.
func RunExtractorCLI(embedURL string, debug bool) error {
	if strings.TrimSpace(embedURL) == "" {
		return errors.New("missing embed URL")
	}

	logger := func(string) {}
	if debug {
		logger = func(line string) { fmt.Println(line) }
	}

	fmt.Printf("[extractor] starting for %s\n", embedURL)
	m3u8, hdrs, err := extractM3U8Lite(embedURL, logger)
	if err != nil {
		fmt.Printf("[extractor] ❌ %v\n", err)
		return err
	}

	fmt.Printf("[extractor] ✅ found M3U8: %s\n", m3u8)
	if len(hdrs) > 0 && debug {
		fmt.Printf("[extractor] captured %d headers\n", len(hdrs))
	}

	if err := LaunchMPVWithHeaders(m3u8, hdrs, logger, true); err != nil {
		fmt.Printf("[mpv] ❌ %v\n", err)
		return err
	}

	fmt.Println("[mpv] ▶ streaming started")
	return nil
}
