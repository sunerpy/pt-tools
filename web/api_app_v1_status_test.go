package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/version"
)

// GET /downloaders：启用的下载器与传输状态（没有地址、账号与密码）；连不上的写明原因
func TestAppDownloaders(t *testing.T) {
	fake := &fakeDownloader{freSpace: 5 << 30}
	srv, id := setupServerWithFakeDownloader(t, fake)
	require.NoError(t, global.GlobalDB.DB.Create(&models.DownloaderSetting{Name: "gone", Type: "qbittorrent", URL: "http://admin:secret@127.0.0.1:9", Enabled: true}).Error)
	w := appAs(srv.appDownloaders, http.MethodGet, "/api/app/v1/downloaders")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "secret")
	var out AppDownloaderList
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	require.Len(t, out.Items, 2)
	assert.Equal(t, id, out.Items[0].ID)
	assert.True(t, out.Items[0].Reachable)
	assert.EqualValues(t, 5<<30, out.Items[0].FreeSpace)
	assert.Equal(t, "gone", out.Items[1].Name)
	assert.False(t, out.Items[1].Reachable)
	assert.NotEmpty(t, out.Items[1].Error)

	prev := global.GlobalDB
	global.GlobalDB = nil
	t.Cleanup(func() { global.GlobalDB = prev })
	assert.Equal(t, http.StatusServiceUnavailable, appAs(srv.appDownloaders, http.MethodGet, "/api/app/v1/downloaders").Code)
}

// GET /updates：include_prerelease=1 时也报预览版；查不了（又没有上次的结果）时回 502
func TestAppUpdates(t *testing.T) {
	srv := setupServer(t)
	var got version.CheckOptions
	srv.checkUpdates = func(_ context.Context, opts version.CheckOptions) (*version.VersionCheckResult, error) {
		got = opts
		return &version.VersionCheckResult{CurrentVersion: "v1", HasUpdate: true}, nil
	}
	w := appAs(srv.appUpdates, http.MethodGet, "/api/app/v1/updates?include_prerelease=1")
	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, got.IncludePrerelease)
	assert.Contains(t, w.Body.String(), `"has_update":true`)

	srv.checkUpdates = func(context.Context, version.CheckOptions) (*version.VersionCheckResult, error) {
		return nil, errors.New("连不上 GitHub")
	}
	assert.Equal(t, http.StatusBadGateway, appAs(srv.appUpdates, http.MethodGet, "/api/app/v1/updates").Code)
}
