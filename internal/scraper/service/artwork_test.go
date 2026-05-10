package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal/scraper/core"
)

func TestDownload_CacheMiss(t *testing.T) {
	ctx := context.Background()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte("fakeimagedata"))
	}))
	defer srv.Close()

	d, err := NewArtworkDownloader(Config{
		HTTPClient:    srv.Client(),
		CacheDir:      t.TempDir(),
		MaxCacheSize:  100 * 1024 * 1024,
		MaxConcurrent: 3,
	})
	require.NoError(t, err)

	target := filepath.Join(t.TempDir(), "poster.jpg")
	require.NoError(t, d.Download(ctx, srv.URL+"/poster.jpg", target))

	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "fakeimagedata", string(content))
	require.Equal(t, int32(1), atomic.LoadInt32(&hits))
}

func TestDownload_CacheHit(t *testing.T) {
	ctx := context.Background()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()

	cacheDir := t.TempDir()
	d, err := NewArtworkDownloader(Config{
		HTTPClient:    srv.Client(),
		CacheDir:      cacheDir,
		MaxCacheSize:  1024 * 1024,
		MaxConcurrent: 1,
	})
	require.NoError(t, err)

	target1 := filepath.Join(t.TempDir(), "a.jpg")
	target2 := filepath.Join(t.TempDir(), "b.jpg")

	require.NoError(t, d.Download(ctx, srv.URL+"/x.jpg", target1))
	require.NoError(t, d.Download(ctx, srv.URL+"/x.jpg", target2))

	require.Equal(t, int32(1), atomic.LoadInt32(&hits))

	c1, err := os.ReadFile(target1)
	require.NoError(t, err)
	c2, err := os.ReadFile(target2)
	require.NoError(t, err)
	require.Equal(t, c1, c2)
}

func TestDownload_HTTPError(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	d, err := NewArtworkDownloader(Config{
		HTTPClient:    srv.Client(),
		CacheDir:      t.TempDir(),
		MaxCacheSize:  1024,
		MaxConcurrent: 1,
	})
	require.NoError(t, err)

	target := filepath.Join(t.TempDir(), "x.jpg")
	err = d.Download(ctx, srv.URL+"/missing", target)
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
}

func TestDownloadArtworks_Concurrent(t *testing.T) {
	ctx := context.Background()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte("img-" + r.URL.Path))
	}))
	defer srv.Close()

	d, err := NewArtworkDownloader(Config{
		HTTPClient:    srv.Client(),
		CacheDir:      t.TempDir(),
		MaxCacheSize:  10 * 1024 * 1024,
		MaxConcurrent: 3,
	})
	require.NoError(t, err)

	artworks := make([]core.MediaArtwork, 0, 9)
	types := []core.ArtworkType{
		core.ArtworkTypePoster,
		core.ArtworkTypeBackground,
		core.ArtworkTypeBanner,
		core.ArtworkTypeClearlogo,
		core.ArtworkTypeClearart,
		core.ArtworkTypeDisc,
		core.ArtworkTypeKeyart,
		core.ArtworkTypeThumb,
		core.ArtworkTypeCharacterart,
	}
	for i, ty := range types {
		artworks = append(artworks, core.MediaArtwork{
			Type: ty,
			URL:  fmt.Sprintf("%s/img/%d.jpg", srv.URL, i),
		})
	}

	targetDir := t.TempDir()
	start := time.Now()
	err = d.DownloadArtworks(ctx, artworks, targetDir)
	elapsed := time.Since(start)
	require.NoError(t, err)

	require.Equal(t, int32(len(artworks)), atomic.LoadInt32(&hits))
	// 9 tasks @ 100ms each with 3 workers ≈ 300ms; allow generous envelope.
	require.GreaterOrEqual(t, elapsed, 250*time.Millisecond)
	require.Less(t, elapsed, 900*time.Millisecond)

	entries, err := os.ReadDir(targetDir)
	require.NoError(t, err)
	require.Len(t, entries, len(artworks))
}

func TestDownloadArtworks_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()

	d, err := NewArtworkDownloader(Config{
		HTTPClient:    srv.Client(),
		CacheDir:      t.TempDir(),
		MaxCacheSize:  1024 * 1024,
		MaxConcurrent: 2,
	})
	require.NoError(t, err)

	artworks := []core.MediaArtwork{
		{Type: core.ArtworkTypePoster, URL: srv.URL + "/1.jpg"},
		{Type: core.ArtworkTypeBackground, URL: srv.URL + "/2.jpg"},
		{Type: core.ArtworkTypeBanner, URL: srv.URL + "/3.jpg"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = d.DownloadArtworks(ctx, artworks, t.TempDir())
	require.Error(t, err)
}

func TestDownloadArtworks_SkipsEmpty(t *testing.T) {
	ctx := context.Background()
	d, err := NewArtworkDownloader(Config{
		HTTPClient:    &http.Client{},
		CacheDir:      t.TempDir(),
		MaxCacheSize:  1024,
		MaxConcurrent: 1,
	})
	require.NoError(t, err)

	err = d.DownloadArtworks(ctx, []core.MediaArtwork{{Type: core.ArtworkTypePoster, URL: ""}}, t.TempDir())
	require.NoError(t, err)

	err = d.DownloadArtworks(ctx, nil, t.TempDir())
	require.NoError(t, err)
}

func TestArtworkFilename_Season(t *testing.T) {
	cases := []struct {
		ty     core.ArtworkType
		season int
		want   string
	}{
		{core.ArtworkTypeSeasonPoster, 1, "season01-poster.jpg"},
		{core.ArtworkTypeSeasonFanart, 2, "season02-fanart.jpg"},
		{core.ArtworkTypeSeasonBanner, 10, "season10-banner.jpg"},
		{core.ArtworkTypeSeasonThumb, 0, "season00-landscape.jpg"},
		{core.ArtworkTypePoster, 3, "poster.jpg"},
		{core.ArtworkTypeBackground, 0, "fanart.jpg"},
	}
	for _, c := range cases {
		got := artworkFilename(core.MediaArtwork{Type: c.ty, Season: c.season})
		require.Equal(t, c.want, got, "type=%s season=%d", c.ty.String(), c.season)
	}
}

func TestExtFromURL(t *testing.T) {
	cases := map[string]string{
		"https://x.com/a.jpg":              ".jpg",
		"https://x.com/a.PNG":              ".png",
		"https://x.com/a":                  ".jpg",
		"https://x.com/a.webp?token=abc":   ".webp",
		"":                                 ".jpg",
		"https://x.com/path/nested/b.jpeg": ".jpeg",
	}
	for in, want := range cases {
		require.Equal(t, want, extFromURL(in), "url=%s", in)
	}
}

func TestCache_LRU(t *testing.T) {
	cache, err := NewFSCache(t.TempDir(), 100)
	require.NoError(t, err)

	// Use distinct mtimes so LRU sort order is deterministic.
	past := time.Now().Add(-10 * time.Minute)
	_, err = cache.Put("key1", []byte(strings.Repeat("a", 40)), ".jpg")
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(filepath.Join(cache.Dir(), "key1.jpg"), past, past))

	mid := time.Now().Add(-5 * time.Minute)
	_, err = cache.Put("key2", []byte(strings.Repeat("b", 40)), ".jpg")
	require.NoError(t, err)
	require.NoError(t, os.Chtimes(filepath.Join(cache.Dir(), "key2.jpg"), mid, mid))

	_, err = cache.Put("key3", []byte(strings.Repeat("c", 40)), ".jpg")
	require.NoError(t, err)

	_, found := cache.Get("key1")
	require.False(t, found, "key1 should have been evicted")
	_, found = cache.Get("key3")
	require.True(t, found, "key3 should remain")

	size, err := cache.Size()
	require.NoError(t, err)
	require.LessOrEqual(t, size, int64(100))
}

func TestCache_GetMissing(t *testing.T) {
	cache, err := NewFSCache(t.TempDir(), 1024)
	require.NoError(t, err)

	_, ok := cache.Get("nope")
	require.False(t, ok)
}

func TestCache_ReplaceExistingKey(t *testing.T) {
	cache, err := NewFSCache(t.TempDir(), 1024)
	require.NoError(t, err)

	p1, err := cache.Put("k", []byte("hello"), ".jpg")
	require.NoError(t, err)
	_ = p1

	p2, err := cache.Put("k", []byte("world!"), ".png")
	require.NoError(t, err)

	content, err := os.ReadFile(p2)
	require.NoError(t, err)
	require.Equal(t, "world!", string(content))

	entries, err := os.ReadDir(cache.Dir())
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestCache_InvalidConfig(t *testing.T) {
	_, err := NewFSCache("", 1024)
	require.Error(t, err)

	_, err = NewFSCache(t.TempDir(), 0)
	require.Error(t, err)
}

func TestNewArtworkDownloader_Validation(t *testing.T) {
	_, err := NewArtworkDownloader(Config{CacheDir: t.TempDir()})
	require.Error(t, err)

	_, err = NewArtworkDownloader(Config{HTTPClient: &http.Client{}})
	require.Error(t, err)
}
