// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
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

func TestProcessSingleTorrent_PushFailUpdatesRetry(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "pfu", FreeEndTime: &future, IsPushed: &pushed, FreeLevel: "free"}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: addFailTransport{}}, "http://example")
	err := processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday"))
	require.Error(t, err)
	got, gerr := db.GetTorrentBySiteAndHash("springsunday", hash)
	require.NoError(t, gerr)
	assert.Equal(t, 1, got.RetryCount)
	assert.NotEmpty(t, got.LastError)
}

func TestProcessSingleTorrent_MaxRetryDeletesFree(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	require.NoError(t, core.NewConfigStore(db).SaveGlobalSettings(models.SettingsGlobal{
		DownloadDir: t.TempDir(), MaxRetry: 2,
	}))
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{
		SiteName: "springsunday", TorrentID: "mrf", FreeEndTime: &future, IsPushed: &pushed,
		FreeLevel: "free", RetryCount: 9,
	}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "over-max-retry file must be deleted")
}
