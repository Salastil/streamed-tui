# streamed-tui

Terminal UI for browsing streamed.pk sports streams, opening the selected embed URL in your browser or passing it straight to mpv.

<img width="1910" height="1038" alt="20251123_015918" src="https://github.com/user-attachments/assets/d0bf328b-9139-44ef-b764-25f1a53c7da7" />


## How it works

The client talks to the https://streamed.pk/ API to load sports, popular matches, and per-match streams in a three column layout: sports on the left, matches in the middle, and available streams on the right. Focus moves with the arrow keys or vim keys hjkl, and selecting a match triggers a stream lookup for that event. Press `o` to open the highlighted stream in your default browser, `p` or enter to pipe the embed URL to mpv. You can also bypass the TUI entirely with `-e <embed-url>` to extract and launch a single stream from the command line if you know the url from embed.top

**Debugging** – Start the app with `--debug` to append verbose extractor logs into the debug panel at the bottom of the UI, useful when diagnosing failed stream loads, best used with the -e flag so it will not render the TUI and the debug log is placed in stdout.  

**Admin Streams** - Streams flagged as Admin are not capable of being forwarded to mpv. These streams have heavier obsfucation and the typical m3u8 extraction method does not work as the javascript on these pages continously issue new m3u8 rather than the typical follow-along type on other streams. These streams can only be watched in the browser, hitting 'o' on the stream will open your browser as set by $XDG_OPEN. 

## Building from source

1. Install Go 1.24+ (matching the module version) and ensure your `$GOPATH/bin` is on `PATH`.
2. Refresh bundled Node.js dependencies (only needed if you change extractor packages):
   ```bash
   scripts/build_node_modules.sh
   ```
   This repacks the `puppeteer-extra` toolchain into `internal/assets/node_modules.tar.gz` so the Go binary can unpack it at runtime without requiring npm on the target machine.
3. Compile the TUI:
   ```bash
   go build -o streamed-tui .
   ```
4. Run it:
   ```bash
   ./streamed-tui           # launches the full TUI
   ./streamed-tui -e URL    # extracts and launches a single embed URL
   ./streamed-tui --debug   # shows extractor debug log in the footer
   ```

## Bundled Puppeteer dependencies

The extractor relies on `puppeteer-extra`, `puppeteer-extra-plugin-stealth`, and `puppeteer`. These Node.js packages are bundled into the final binary via `internal/assets/node_modules.tar.gz`. To refresh the archive (for example after updating dependency versions), run:

```
scripts/build_node_modules.sh
```

The script installs the dependencies into a temporary directory and regenerates the tarball so the Go binary can extract them at runtime without requiring `npm install` on the target system. When the binary starts it will automatically unpack the archive into the user's cache directory (or `$TMPDIR` fallback) and point Puppeteer at that cached `node_modules` tree, so the program can run as a single self-contained executable even when no dependencies exist alongside it.
