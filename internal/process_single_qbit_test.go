// MIT License
// Copyright (c) 2025 pt-tools

// Additional branch coverage for processSingleTorrent: successful push
// (add succeeds) and the already-exists path via a fake qBittorrent server,
// plus the push-failure retry-count increment branch.

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

// addOKTransport answers auth + properties(404) + maindata + add(200) so
// processSingleTorrent reaches and completes the successful push branch.
type addOKTransport struct{}

func (addOKTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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
		return ok("Ok."), nil
	default:
		return ok("{}"), nil
	}
}

func TestProcessSingle_SuccessfulPush(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(1 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "ok1", FreeEndTime: &future, IsPushed: &pushed}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: addOKTransport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))

	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "pushed torrent file must be removed")
	got, err := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, err)
	require.NotNil(t, got.IsPushed)
	assert.True(t, *got.IsPushed)
}

func TestProcessSingle_MaxRetryWithFutureFree(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), MaxRetry: 2,
	}))
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)
	future := time.Now().Add(1 * time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "mr2", FreeEndTime: &future, IsPushed: &pushed, RetryCount: 5}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}
