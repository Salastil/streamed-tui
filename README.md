# streamed-tui
TUI Application for launching streamed.pk feeds

## Bundled Puppeteer dependencies

The extractor relies on `puppeteer-extra`, `puppeteer-extra-plugin-stealth`, and `puppeteer`. These Node.js packages are
bundled into the final binary via `internal/assets/node_modules.tar.gz`. To refresh the archive (for example after updating
dependency versions), run:

```
scripts/build_node_modules.sh
```

The script installs the dependencies into a temporary directory and regenerates the tarball so the Go binary can extract
them at runtime without requiring `npm install` on the target system. When the binary starts it will automatically unpack the
archive into the user's cache directory (or `$TMPDIR` fallback) and point Puppeteer at that cached `node_modules` tree, so the
program can run as a single self-contained executable even when no dependencies exist alongside it.
