// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

type addFailTransport struct{}

func (addFailTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ok := func(body string) *http.Response {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}
	}
	switch req.URL.Path {
	case "/api/v2/auth/login":
		return ok("Ok."), nil
	case "/api/v2/torrents/properties":
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Header: make(http.Header)}, nil
	case "/api/v2/sync/maindata":
		return ok(`{"server_state":{"free_space_on_disk":107374182400}}`), nil
	case "/api/v2/torrents/add":
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: io.NopCloser(bytes.NewBufferString("boom")), Header: make(http.Header)}, nil
	default:
		return ok("{}"), nil
	}
}

type existsTransport struct{}

func (existsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ok := func(body string) *http.Response {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewBufferString(body)), Header: make(http.Header)}
	}
	switch req.URL.Path {
	case "/api/v2/auth/login":
		return ok("Ok."), nil
	case "/api/v2/torrents/properties":
		return ok(`{"save_path":"/downloads"}`), nil
	default:
		return ok("{}"), nil
	}
}

func TestProcessSingleTorrent_ExpiredMarksAndDeletes(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	past := time.Now().Add(-time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "exp", FreeEndTime: &past, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	assert.True(t, got.IsExpired)
}

func TestProcessSingleTorrent_ExistsSyncsPushed(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "ex", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: existsTransport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
}

func TestProcessSingleTorrent_PushFailIncrementsRetry(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "pf", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: addFailTransport{}}, "http://example")
	err := processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday"))
	require.Error(t, err)
	got, gerr := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, gerr)
	assert.Equal(t, 1, got.RetryCount)
}

func TestProcessSingleTorrent_RetainHoursDeletesUnpushed(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), RetainHours: 1,
	}))
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	oldCheck := time.Now().Add(-5 * time.Hour)
	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "rh", FreeEndTime: &future,
		IsPushed: &pushed, LastCheckTime: &oldCheck,
	}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}
