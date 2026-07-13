// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

func TestProcessSingleTorrent_HashComputeFails(t *testing.T) {
	_ = setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	bad := filepath.Join(dir, "notatorrent.torrent")
	require.NoError(t, os.WriteFile(bad, []byte("not-bencode-data"), 0o644))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	err := processSingleTorrent(context.Background(), client, bad, "cat", "tag", models.SiteGroup("springsunday"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "计算种子哈希")
}

func TestProcessSingleTorrent_GetExpiredNoFreeEndDeletes(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	// FreeEndTime nil + FreeLevel "" → GetExpired() true → mark expired + delete.
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "gx", IsPushed: &pushed, FreeLevel: ""}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &props404Transport{}}, "http://example")
	require.NoError(t, processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday")))
	_, statErr := os.Stat(path)
	assert.True(t, os.IsNotExist(statErr))
}

func TestProcessSingleTorrent_CheckExistsError(t *testing.T) {
	db := setupDB(t)
	t.Cleanup(func() { global.GlobalDB = nil })
	dir := t.TempDir()
	path, hash := makeTorrentFile(t, dir)

	future := time.Now().Add(time.Hour)
	pushed := false
	ti := &models.TorrentInfo{SiteName: "springsunday", TorrentID: "ce", FreeEndTime: &future, IsPushed: &pushed, FreeLevel: "free"}
	ti.TorrentHash = &hash
	require.NoError(t, db.UpsertTorrent(ti))

	client := qbit.NewQbitClientForTesting(&http.Client{Transport: &checkExistsErrTransport{}}, "http://example")
	err := processSingleTorrent(context.Background(), client, path, "cat", "tag", models.SiteGroup("springsunday"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "检查种子存在")
}

type checkExistsErrTransport struct{}

func (checkExistsErrTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Path == "/api/v2/auth/login" {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	if req.URL.Path == "/api/v2/torrents/properties" {
		return &http.Response{StatusCode: http.StatusInternalServerError, Body: http.NoBody, Header: make(http.Header)}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}
