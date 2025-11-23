package internal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

//go:embed assets/node_modules.tar.gz
var embeddedNodeModules []byte

// ensureEmbeddedNodeModules extracts the bundled Node.js dependencies into a
// deterministic cache directory derived from the archive hash and returns the
// path that contains the resulting node_modules directory.
func ensureEmbeddedNodeModules() (string, error) {
	if len(embeddedNodeModules) == 0 {
		return "", errors.New("no embedded node modules archive available")
	}

	sum := sha256.Sum256(embeddedNodeModules)
	hashPrefix := hex.EncodeToString(sum[:8])

	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		cacheRoot = os.TempDir()
	}
	baseDir := filepath.Join(cacheRoot, "streamed-tui", "node_modules", hashPrefix)

	marker := filepath.Join(baseDir, ".complete")
	if _, err := os.Stat(marker); err == nil {
		return baseDir, nil
	}

	if err := os.RemoveAll(baseDir); err != nil {
		return "", fmt.Errorf("failed to clear embedded node cache: %w", err)
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create embedded node cache: %w", err)
	}

	if err := untarGzip(bytes.NewReader(embeddedNodeModules), baseDir); err != nil {
		return "", fmt.Errorf("failed to extract embedded node modules: %w", err)
	}

	if err := os.WriteFile(marker, []byte(time.Now().Format(time.RFC3339)), 0o644); err != nil {
		return "", fmt.Errorf("failed to mark embedded node modules ready: %w", err)
	}

	return baseDir, nil
}

func untarGzip(r io.Reader, dest string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		default:
			// Ignore unsupported entries to keep extraction simple.
		}
	}
	return nil
}
