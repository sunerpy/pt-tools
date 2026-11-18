package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// keepFakeDownloader 记下加、去标签的请求；qBittorrent 型的还实现 TorrentTagRemover。
type keepFakeDownloader struct {
	*fakeDownloader
	setTags    [][2]string
	removeTags [][2]string
	setErr     error
	removeErr  error
}

func (f *keepFakeDownloader) SetTorrentTags(id, tags string) error {
	f.setTags = append(f.setTags, [2]string{id, tags})
	return f.setErr
}

func (f *keepFakeDownloader) RemoveTorrentTags(ids []string, tags string) error {
	for _, id := range ids {
		f.removeTags = append(f.removeTags, [2]string{id, tags})
	}
	return f.removeErr
}

// setupKeepServer 是带一台假下载器（qBittorrent 或 Transmission）的网页服务，返回下载器在设置里的编号。
func setupKeepServer(t *testing.T, typ downloader.DownloaderType, torrents []downloader.Torrent) (*Server, *keepFakeDownloader, uint) {
	t.Helper()
	server, db := setupTestServer(t)
	require.NoError(t, db.AutoMigrate(&models.BrushTask{}, &models.BrushTorrent{}))
	fake := &keepFakeDownloader{fakeDownloader: &fakeDownloader{torrents: torrents}}
	mgr := scheduler.NewManager()
	t.Cleanup(func() { mgr.StopAll() })
	dm := mgr.GetDownloaderManager()
	dm.RegisterFactory(typ, func(_ downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		fake.name, fake.dlType = name, typ
		return fake, nil
	})
	require.NoError(t, dm.RegisterConfig("dl1", downloader.NewGenericConfig(typ, "http://localhost:8080", "u", "p", true), true))
	server.mgr = mgr
	ds := models.DownloaderSetting{Name: "dl1", Type: string(typ), Enabled: true}
	require.NoError(t, db.Create(&ds).Error)
	return server, fake, ds.ID
}

func runKeep(t *testing.T, server *Server, action string, dlID uint, ids ...string) BatchTorrentActionResponse {
	t.Helper()
	req := BatchTorrentActionRequest{Action: action}
	for _, id := range ids {
		req.Targets = append(req.Targets, TorrentActionTarget{DownloaderID: dlID, TaskID: id})
	}
	body, _ := json.Marshal(req)
	w := httptest.NewRecorder()
	server.apiDownloaderTorrentActions(w, httptest.NewRequest(http.MethodPost, "/api/downloader-torrents/batch-action", bytes.NewReader(body)))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp BatchTorrentActionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

// addKeepBrush 在下载器 dlID 上记一个刷流任务正在管的种子。
func addKeepBrush(t *testing.T, dlID uint, hash string) models.BrushTorrent {
	t.Helper()
	db := global.GlobalDB.DB
	task := models.BrushTask{Name: "brush-" + hash, SiteName: "s", DownloaderID: dlID}
	require.NoError(t, db.Create(&task).Error)
	bt := models.BrushTorrent{TaskID: task.ID, InfoHash: hash, SiteName: "s", TorrentID: hash, DownloaderID: dlID, State: models.BrushTorrentActive, AddedAt: time.Now()}
	require.NoError(t, db.Create(&bt).Error)
	return bt
}

// qBittorrent：保留用 addTags 加 pt-tools-keep，同时交还刷流记录；取消保留用 removeTags 只去这一个标签。
func TestKeepTorrents_QBittorrent(t *testing.T) {
	server, fake, dlID := setupKeepServer(t, downloader.DownloaderQBittorrent, []downloader.Torrent{
		{ID: "hash1", InfoHash: "hash1", Tags: "s,pt-tools-brush,pt-brush-1"},
		{ID: "hash2", InfoHash: "hash2", Tags: "a, pt-tools-keep"},
	})
	bt := addKeepBrush(t, dlID, "hash1")

	resp := runKeep(t, server, "keep", dlID, "hash1")
	require.Equal(t, 1, resp.SuccessCount, resp.Results)
	assert.Equal(t, [][2]string{{"hash1", models.KeepTag}}, fake.setTags)
	var got models.BrushTorrent
	require.NoError(t, global.GlobalDB.DB.First(&got, bt.ID).Error)
	assert.Equal(t, models.BrushTorrentGone, got.State)
	assert.Equal(t, keepHandOverReason, got.RemoveReason)

	resp = runKeep(t, server, "unkeep", dlID, "hash2", "hash1")
	assert.Equal(t, 2, resp.SuccessCount, resp.Results)
	assert.Equal(t, [][2]string{{"hash2", models.KeepTag}}, fake.removeTags, "hash1 在假下载器里没带标签：不用去")
}

// Transmission：labels 只能整份改写，读出原来的再加、去 pt-tools-keep，分类（第一个 label）不动。
func TestKeepTorrents_Transmission(t *testing.T) {
	server, fake, dlID := setupKeepServer(t, downloader.DownloaderTransmission, []downloader.Torrent{
		{ID: "5", InfoHash: "abc", Tags: "movie,x", Category: "movie", Label: "movie"},
		{ID: "6", InfoHash: "def", Tags: "movie,pt-tools-keep,x", Category: "movie", Label: "movie"},
	})
	resp := runKeep(t, server, "keep", dlID, "5")
	require.Equal(t, 1, resp.SuccessCount, resp.Results)
	resp = runKeep(t, server, "unkeep", dlID, "6")
	require.Equal(t, 1, resp.SuccessCount, resp.Results)
	assert.Equal(t, [][2]string{{"5", "movie,x,pt-tools-keep"}, {"6", "movie,x"}}, fake.setTags)
	assert.Empty(t, fake.removeTags)
}

// 种子已经不在下载器里（例如刚被自动删种删掉）：报失败，不改标签。
func TestKeepTorrents_MissingTorrent(t *testing.T) {
	server, fake, dlID := setupKeepServer(t, downloader.DownloaderQBittorrent, nil)
	resp := runKeep(t, server, "keep", dlID, "gone")
	require.Equal(t, 1, resp.FailedCount)
	assert.Equal(t, "种子不在下载器里了", resp.Results[0].Message)
	assert.Empty(t, fake.setTags)
}

// 交还刷流记录失败：撤回刚加的标签、报失败；撤回也失败时报部分失败；本来就带保留标签的不撤回。
func TestKeepTorrents_HandOverFailure(t *testing.T) {
	for _, tc := range []struct {
		name, tags, want string
		revertErr        error
		wantRemove       [][2]string
	}{
		{"撤回成功", "", "没能从刷流任务交还", nil, [][2]string{{"hash1", models.KeepTag}}},
		{"撤回也失败", "", "已加保留标签，刷流记录没改", errors.New("boom"), [][2]string{{"hash1", models.KeepTag}}},
		{"本来就带保留标签", models.KeepTag, "刷流记录没改", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, fake, dlID := setupKeepServer(t, downloader.DownloaderQBittorrent, []downloader.Torrent{
				{ID: "hash1", InfoHash: "hash1", Tags: tc.tags},
			})
			fake.removeErr = tc.revertErr
			require.NoError(t, global.GlobalDB.DB.Migrator().DropTable(&models.BrushTorrent{}))
			resp := runKeep(t, server, "keep", dlID, "hash1")
			require.Equal(t, 1, resp.FailedCount)
			assert.Contains(t, resp.Results[0].Message, tc.want)
			assert.Equal(t, tc.wantRemove, fake.removeTags)
		})
	}
}

// 能力接口告诉页面两种下载器都能保留；App API 的动作白名单不加保留。
func TestKeepCapabilityAndAppWhitelist(t *testing.T) {
	assert.True(t, downloaderCapabilityFromRecord(downloaderRecord{Type: "qbittorrent"}).CanKeep)
	assert.True(t, downloaderCapabilityFromRecord(downloaderRecord{Type: "transmission"}).CanKeep)
	assert.NotContains(t, appTorrentActions, "keep")
	assert.NotContains(t, appTorrentActions, "unkeep")
}
