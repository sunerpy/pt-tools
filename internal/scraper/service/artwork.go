// Package service: artwork.go implements the ArtworkDownloader.
//
// The downloader fetches artwork URLs over HTTP (using a caller-provided
// *http.Client with proxy already injected), caches bytes on disk via
// FSCache (keyed by SHA1(url)), and writes results to target paths using
// Jellyfin's artwork naming convention.
package service

import (
	"context"
	"crypto/sha1" //nolint:gosec // content-addressable cache key, not a security primitive
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

// nowFunc is overridable for testing mtime behavior.
var nowFunc = time.Now

// Config configures a new ArtworkDownloader.
type Config struct {
	HTTPClient    *http.Client
	CacheDir      string
	MaxCacheSize  int64
	MaxConcurrent int
}

// ArtworkDownloader downloads artwork URLs to target paths with an
// on-disk LRU cache and bounded concurrency.
type ArtworkDownloader struct {
	client        *http.Client
	cache         *FSCache
	maxConcurrent int
}

const (
	defaultMaxCacheSize  int64 = 1 << 30 // 1 GiB
	defaultMaxConcurrent       = 3
)

// NewArtworkDownloader constructs an ArtworkDownloader. HTTPClient and
// CacheDir are required. Zero values for MaxCacheSize and MaxConcurrent
// fall back to sensible defaults (1 GiB, 3).
func NewArtworkDownloader(cfg Config) (*ArtworkDownloader, error) {
	if cfg.HTTPClient == nil {
		return nil, errors.New("artwork downloader: HTTPClient is required")
	}
	if cfg.CacheDir == "" {
		return nil, errors.New("artwork downloader: CacheDir is required")
	}
	maxSize := cfg.MaxCacheSize
	if maxSize <= 0 {
		maxSize = defaultMaxCacheSize
	}
	conc := cfg.MaxConcurrent
	if conc <= 0 {
		conc = defaultMaxConcurrent
	}
	cache, err := NewFSCache(cfg.CacheDir, maxSize)
	if err != nil {
		return nil, err
	}
	return &ArtworkDownloader{
		client:        cfg.HTTPClient,
		cache:         cache,
		maxConcurrent: conc,
	}, nil
}

// Cache exposes the underlying FS cache (useful for inspection / tests).
func (d *ArtworkDownloader) Cache() *FSCache { return d.cache }

// MaxConcurrent returns the configured concurrency.
func (d *ArtworkDownloader) MaxConcurrent() int { return d.maxConcurrent }

// Download fetches a single URL into targetPath, using the cache when
// possible. The target directory is created if it does not exist.
func (d *ArtworkDownloader) Download(ctx context.Context, rawURL, targetPath string) error {
	if rawURL == "" {
		return errors.New("artwork downloader: url is empty")
	}
	if targetPath == "" {
		return errors.New("artwork downloader: targetPath is empty")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	key := cacheKey(rawURL)
	ext := extFromURL(rawURL)

	if cached, ok := d.cache.Get(key); ok {
		return copyFile(cached, targetPath)
	}

	content, err := d.fetch(ctx, rawURL)
	if err != nil {
		return err
	}
	if _, err := d.cache.Put(key, content, ext); err != nil {
		return err
	}
	return writeFile(targetPath, content)
}

// ArtworkTask describes one requested artwork download. Exported so that
// higher layers can build batches without going through MediaArtwork.
type ArtworkTask struct {
	URL      string
	Filename string
}

// DownloadArtworks downloads each artwork in the slice concurrently
// (bounded by d.maxConcurrent). Failures are collected and returned via
// errors.Join; ctx cancellation short-circuits remaining workers.
//
// targetDir is created if missing. Artwork filenames are derived from
// ArtworkType via the Jellyfin naming convention.
func (d *ArtworkDownloader) DownloadArtworks(
	ctx context.Context,
	artworks []core.MediaArtwork,
	targetDir string,
) error {
	if targetDir == "" {
		return errors.New("artwork downloader: targetDir is empty")
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("artwork downloader: mkdir %s: %w", targetDir, err)
	}

	tasks := make([]ArtworkTask, 0, len(artworks))
	for _, a := range artworks {
		if a.URL == "" {
			continue
		}
		name := artworkFilename(a)
		if name == "" {
			continue
		}
		tasks = append(tasks, ArtworkTask{
			URL:      a.URL,
			Filename: filepath.Join(targetDir, name),
		})
	}
	if len(tasks) == 0 {
		return nil
	}

	sem := make(chan struct{}, d.maxConcurrent)
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		errs   []error
		failed = func(e error) {
			mu.Lock()
			errs = append(errs, e)
			mu.Unlock()
		}
	)

	for _, t := range tasks {
		select {
		case <-ctx.Done():
			failed(ctx.Err())
			wg.Wait()
			return errors.Join(errs...)
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(task ArtworkTask) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				failed(err)
				return
			}
			if err := d.Download(ctx, task.URL, task.Filename); err != nil {
				failed(fmt.Errorf("artwork %s: %w", task.URL, err))
			}
		}(t)
	}

	wg.Wait()
	return errors.Join(errs...)
}

// --- HTTP fetch ---

func (d *ArtworkDownloader) fetch(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("artwork downloader: build request: %w", err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("artwork downloader: fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("artwork downloader: fetch %s: status %d", rawURL, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("artwork downloader: read body %s: %w", rawURL, err)
	}
	return body, nil
}

// --- Jellyfin filename mapping ---

// artworkFilenames maps non-season ArtworkType values to Jellyfin-standard
// file names. See https://jellyfin.org/docs/general/server/media/images/
var artworkFilenames = map[core.ArtworkType]string{
	core.ArtworkTypePoster:       "poster.jpg",
	core.ArtworkTypeBackground:   "fanart.jpg",
	core.ArtworkTypeBanner:       "banner.jpg",
	core.ArtworkTypeClearlogo:    "clearlogo.png",
	core.ArtworkTypeClearart:     "clearart.png",
	core.ArtworkTypeDisc:         "disc.png",
	core.ArtworkTypeKeyart:       "keyart.jpg",
	core.ArtworkTypeThumb:        "landscape.jpg",
	core.ArtworkTypeCharacterart: "characterart.png",
}

// artworkFilename picks the Jellyfin filename for the given artwork,
// including the season-specific prefix when applicable.
func artworkFilename(a core.MediaArtwork) string {
	if name, ok := artworkFilenames[a.Type]; ok {
		return name
	}
	return artworkFilenameForSeason(a.Type, a.Season)
}

// artworkFilenameForSeason produces season-scoped artwork filenames
// like "season01-poster.jpg". Returns "" for non-season types.
func artworkFilenameForSeason(t core.ArtworkType, seasonNum int) string {
	if seasonNum < 0 {
		seasonNum = 0
	}
	base := fmt.Sprintf("season%02d", seasonNum)
	switch t {
	case core.ArtworkTypeSeasonPoster:
		return base + "-poster.jpg"
	case core.ArtworkTypeSeasonFanart:
		return base + "-fanart.jpg"
	case core.ArtworkTypeSeasonBanner:
		return base + "-banner.jpg"
	case core.ArtworkTypeSeasonThumb:
		return base + "-landscape.jpg"
	}
	return ""
}

// --- small helpers ---

// cacheKey returns the SHA1 hex digest of the URL, used as the cache key.
func cacheKey(rawURL string) string {
	h := sha1.Sum([]byte(rawURL)) //nolint:gosec // not a security primitive
	return hex.EncodeToString(h[:])
}

// extFromURL returns the lowercase file extension from the URL path,
// or ".jpg" when absent. Query strings and fragments are ignored.
func extFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Path == "" {
		return ".jpg"
	}
	ext := strings.ToLower(path.Ext(u.Path))
	if ext == "" {
		return ".jpg"
	}
	return ext
}

func writeFile(target string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("artwork downloader: mkdir %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, content, 0o644); err != nil {
		return fmt.Errorf("artwork downloader: write %s: %w", target, err)
	}
	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("artwork downloader: read cache %s: %w", src, err)
	}
	return writeFile(dst, data)
}
