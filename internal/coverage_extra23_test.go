// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

func TestProcessSingleTorrent_OrphanNoDBRow(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, _ := makeTorrentFile(t, dir)

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr), "orphan (no DB row) must be deleted")
}

func TestProcessSingleTorrent_PushedRowDeletesFile(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	pushed := true
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "pr", IsPushed: &pushed, FreeLevel: "free"}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestFetchAndDownloadFreeRSS_ContextCancelledDuringDispatch(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })

	items := ""
	for i := 0; i < 50; i++ {
		items += itemXML("cc"+itoa(int64(i)), "T", "http://x/t.torrent")
	}
	srv := feedServerUnified(t, rssBody(items))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	site := &legacyPTStub{enabled: true, discount: models.DISCOUNT_FREE, sizeMB: 1}
	_ = FetchAndDownloadFreeRSS(ctx, models.SiteGroup("springsunday"), site,
		models.RSSConfig{Name: "r", URL: srv.URL})
}
