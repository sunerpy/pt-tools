// MIT License
// Copyright (c) 2025 pt-tools

// Coverage for the remaining internal/common.go helpers: sweepStagingDir /
// shouldSweep, recordDiskProtectError, attemptDownloadWithContext /
// downloadTorrent (against a fake torrent-serving HTTP server), and
// invalidTorrentPreview.

package internal

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeebo/bencode"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

func writeStagingTorrent(t *testing.T, dir, name string) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": name}}))
	p := filepath.Join(dir, "springsunday-"+name+".torrent")
	require.NoError(t, os.WriteFile(p, buf.Bytes(), 0o644))
	h := hashOfBytes(t, buf.Bytes())
	return p, h
}

func hashOfBytes(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "tmp.torrent")
	require.NoError(t, os.WriteFile(p, data, 0o644))
	return computeHash(t, p)
}

func computeHash(t *testing.T, path string) string {
	t.Helper()
	h, err := qbit.ComputeTorrentHashWithPath(path)
	require.NoError(t, err)
	return h
}

func TestRecordDiskProtectError(t *testing.T) {
	global.GlobalDB = nil
	recordDiskProtectError(models.SiteGroup("s"), "h", "msg") // no panic

	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	hash := "abc123"
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "id1"}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	recordDiskProtectError(models.SiteGroup("springsunday"), hash, "磁盘满")
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "磁盘满", got.LastError)
}

func TestSweepStagingDir_Disabled(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	p, _ := writeStagingTorrent(t, dir, "keep")
	// retainHours <= 0 -> no-op.
	sweepStagingDir(dir, models.SiteGroup("springsunday"), 0)
	_, err := os.Stat(p)
	require.NoError(t, err, "file must remain when retain disabled")
}

func TestSweepStagingDir_RemovesPushed(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	p, hash := writeStagingTorrent(t, dir, "pushed")

	pushed := true
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "pid"}
	ti.TorrentHash = &hash
	ti.IsPushed = &pushed
	require.NoError(t, db.UpsertTorrent(ti))

	sweepStagingDir(dir, models.SiteGroup("springsunday"), 24)
	_, err := os.Stat(p)
	assert.True(t, os.IsNotExist(err), "pushed torrent file must be swept")
}

func TestShouldSweep(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()

	// Invalid file -> hash fails -> false.
	bad := filepath.Join(dir, "springsunday-bad.torrent")
	require.NoError(t, os.WriteFile(bad, []byte("not a torrent"), 0o644))
	assert.False(t, shouldSweep(bad, models.SiteGroup("springsunday"), 24))

	// No DB record -> true (orphan).
	p, _ := writeStagingTorrent(t, dir, "orphan")
	assert.True(t, shouldSweep(p, models.SiteGroup("springsunday"), 24))

	// Fresh unpushed record within retain window -> false.
	p2, hash2 := writeStagingTorrent(t, dir, "fresh")
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "fid"}
	ti.TorrentHash = &hash2
	ti.IsPushed = &pushed
	require.NoError(t, db.UpsertTorrent(ti))
	assert.False(t, shouldSweep(p2, models.SiteGroup("springsunday"), 24))

	// MaxRetry exceeded -> true.
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: dir, MaxRetry: 1,
	}))
	p3, hash3 := writeStagingTorrent(t, dir, "retried")
	ti3 := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "rid", RetryCount: 5}
	ti3.TorrentHash = &hash3
	ti3.IsPushed = &pushed
	require.NoError(t, db.UpsertTorrent(ti3))
	assert.True(t, shouldSweep(p3, models.SiteGroup("springsunday"), 24))
}

func TestAttemptDownloadWithContext_Success(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "dl"}}))
	payload := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	hash, err := attemptDownloadWithContext(context.Background(), srv.URL, "My Title", dir)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	_, statErr := os.Stat(filepath.Join(dir, "My Title.torrent"))
	require.NoError(t, statErr)
}

func TestAttemptDownloadWithContext_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	_, err := attemptDownloadWithContext(context.Background(), srv.URL, "t", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 状态码错误")
}

func TestAttemptDownloadWithContext_InvalidTorrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>not a torrent</html>"))
	}))
	t.Cleanup(srv.Close)
	_, err := attemptDownloadWithContext(context.Background(), srv.URL, "t", t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不是有效种子文件")
}

func TestDownloadTorrent_RetryThenFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	_, err := downloadTorrent(srv.URL, "t", t.TempDir(), 2, time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "下载失败")
}

func TestDownloadTorrent_Success(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, bencode.NewEncoder(&buf).Encode(map[string]any{"info": map[string]any{"name": "ok"}}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	hash, err := downloadTorrent(srv.URL, "ok", t.TempDir(), 2, time.Millisecond)
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
}

func TestInvalidTorrentPreview(t *testing.T) {
	// Control chars replaced, whitespace collapsed.
	got := invalidTorrentPreview([]byte("hello\x00\x01  world\n\tfoo"))
	assert.Equal(t, "hello world foo", got)

	// Long input truncated to 160 runes.
	long := make([]byte, 0, 300)
	for i := 0; i < 300; i++ {
		long = append(long, 'a')
	}
	out := invalidTorrentPreview(long)
	assert.Len(t, []rune(out), 160)
}
