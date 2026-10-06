package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/dlassistant"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// trackerEditingFake 在 fakeDownloader 之上加修改 tracker 的能力，并按种子返回各自的 tracker。
type trackerEditingFake struct {
	*fakeDownloader
	byID  map[string][]downloader.TorrentTracker
	edits [][3]string
}

func (f *trackerEditingFake) GetTorrentTrackers(id string) ([]downloader.TorrentTracker, error) {
	return f.byID[id], nil
}

func (f *trackerEditingFake) EditTracker(_ context.Context, hash, oldURL, newURL string) error {
	f.edits = append(f.edits, [3]string{hash, oldURL, newURL})
	return nil
}

func newAssistantServer(t *testing.T, dl downloader.Downloader) (*Server, *http.ServeMux, uint) {
	t.Helper()
	server, db := setupTestServer(t)
	mgr := scheduler.NewManager()
	t.Cleanup(mgr.StopAll)
	dm := mgr.GetDownloaderManager()
	dm.RegisterFactory(downloader.DownloaderQBittorrent, func(downloader.DownloaderConfig, string) (downloader.Downloader, error) { return dl, nil })
	require.NoError(t, dm.RegisterConfig("qb1", downloader.NewGenericConfig(downloader.DownloaderQBittorrent, "http://localhost:8080", "u", "p", true), true))
	server.mgr = mgr
	rec := models.DownloaderSetting{Name: "qb1", Type: "qbittorrent", Enabled: true}
	require.NoError(t, db.Create(&rec).Error)
	mux := http.NewServeMux()
	server.registerDownloaderAssistantRoutes(mux)
	server.sessions.put("sess-test", "admin")
	return server, mux, rec.ID
}

const hdskyTracker = "https://tracker.hdsky.me/announce.php?passkey=SECRET"

func TestAssistantAPI_RequiresSession(t *testing.T) {
	_, mux, _ := newAssistantServer(t, &fakeDownloader{})
	for _, p := range []string{"/api/downloader-assistant/site-tags", "/api/downloader-assistant/trackers", "/api/downloader-assistant/dead", "/api/downloader-assistant/dead-scan"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, p, nil))
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusFound}, w.Code, p)
	}
}

func TestAssistantAPI_SiteTags(t *testing.T) {
	fake := &fakeDownloader{torrents: []downloader.Torrent{{ID: "h1", InfoHash: "h1", Name: "Movie", Tags: "4k", Tracker: hdskyTracker}}}
	_, mux, id := newAssistantServer(t, fake)

	w := serveAuthed(mux, http.MethodGet, fmt.Sprintf("/api/downloader-assistant/site-tags?downloader_id=%d", id), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "SECRET", "不返回 passkey")
	var got struct {
		Items []dlassistant.SiteTagSuggestion `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Items, 1)
	assert.Equal(t, "hdsky", got.Items[0].Site)
	assert.Equal(t, "tracker.hdsky.me", got.Items[0].TrackerHost)

	w = serveAuthed(mux, http.MethodPost, "/api/downloader-assistant/site-tags", fmt.Sprintf(`{"downloader_id":%d,"items":[{"hash":"h1","site":"hdsky"}]}`, id))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res dlassistant.ApplyResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 1, res.Done)

	w = serveAuthed(mux, http.MethodPost, "/api/downloader-assistant/site-tags", fmt.Sprintf(`{"downloader_id":%d,"items":[]}`, id))
	assert.Equal(t, http.StatusBadRequest, w.Code, "没有选中种子")
	w = serveAuthed(mux, http.MethodPost, "/api/downloader-assistant/site-tags", "{")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = serveAuthed(mux, http.MethodDelete, "/api/downloader-assistant/site-tags", "")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestAssistantAPI_DownloaderLookup(t *testing.T) {
	fake := &fakeDownloader{listErr: errors.New("conn refused")}
	_, mux, id := newAssistantServer(t, fake)
	cases := map[string]int{
		"/api/downloader-assistant/site-tags":                              http.StatusBadRequest,
		"/api/downloader-assistant/site-tags?downloader_id=999":            http.StatusNotFound,
		fmt.Sprintf("/api/downloader-assistant/dead?downloader_id=%d", id): http.StatusBadGateway,
	}
	for p, want := range cases {
		w := serveAuthed(mux, http.MethodGet, p, "")
		assert.Equal(t, want, w.Code, p)
	}
}

func TestAssistantAPI_TrackersAndDead(t *testing.T) {
	fake := &trackerEditingFake{
		fakeDownloader: &fakeDownloader{torrents: []downloader.Torrent{
			{ID: "h1", InfoHash: "h1", Name: "A", Tracker: hdskyTracker},
			{ID: "h2", InfoHash: "h2", Name: "B"},
		}},
		byID: map[string][]downloader.TorrentTracker{
			"h1": {{URL: hdskyTracker, Status: 2}},
			"h2": {{URL: hdskyTracker, Status: 4, Message: "Unregistered torrent"}},
		},
	}
	_, mux, id := newAssistantServer(t, fake)

	w := serveAuthed(mux, http.MethodGet, fmt.Sprintf("/api/downloader-assistant/trackers?downloader_id=%d&from=tracker.hdsky.me&to=tracker2.hdsky.me", id), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "SECRET")
	var prev struct {
		Items     []dlassistant.TrackerMatch `json:"items"`
		Supported bool                       `json:"supported"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &prev))
	assert.True(t, prev.Supported)
	assert.Len(t, prev.Items, 2)

	w = serveAuthed(mux, http.MethodGet, fmt.Sprintf("/api/downloader-assistant/trackers?downloader_id=%d&from=x&to=y", id), "")
	assert.Equal(t, http.StatusBadRequest, w.Code, "原内容太短")

	w = serveAuthed(mux, http.MethodPost, "/api/downloader-assistant/trackers", fmt.Sprintf(`{"downloader_id":%d,"from":"tracker.hdsky.me","to":"tracker2.hdsky.me","hashes":["h1"]}`, id))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Len(t, fake.edits, 1)
	assert.Equal(t, "https://tracker2.hdsky.me/announce.php?passkey=SECRET", fake.edits[0][2])

	w = serveAuthed(mux, http.MethodGet, fmt.Sprintf("/api/downloader-assistant/dead?downloader_id=%d", id), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var dead struct {
		Items []dlassistant.DeadTorrent `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &dead))
	require.Len(t, dead.Items, 1)
	assert.Equal(t, "h2", dead.Items[0].Hash)
	assert.Equal(t, "hdsky", dead.Items[0].Site)

	w = serveAuthed(mux, http.MethodPost, "/api/downloader-assistant/dead", fmt.Sprintf(`{"downloader_id":%d,"hashes":["h2"],"remove_data":true}`, id))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res dlassistant.ApplyResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 1, res.Done)
}

func TestAssistantAPI_TrackersUnsupported(t *testing.T) {
	fake := &fakeDownloader{torrents: []downloader.Torrent{{ID: "h1", InfoHash: "h1", Name: "A", Tracker: hdskyTracker}}, trackers: []downloader.TorrentTracker{{URL: hdskyTracker}}}
	_, mux, id := newAssistantServer(t, fake)
	w := serveAuthed(mux, http.MethodGet, fmt.Sprintf("/api/downloader-assistant/trackers?downloader_id=%d&from=tracker.hdsky.me&to=t2.hdsky.me", id), "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"supported":false`)
	w = serveAuthed(mux, http.MethodPost, "/api/downloader-assistant/trackers", fmt.Sprintf(`{"downloader_id":%d,"from":"tracker.hdsky.me","to":"t2.hdsky.me","hashes":["h1"]}`, id))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "不支持修改 tracker")
}

func TestAssistantAPI_DeadScanSettings(t *testing.T) {
	server, mux, _ := newAssistantServer(t, &fakeDownloader{})
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.NotificationConf{}))
	require.NoError(t, server.store.SaveGlobalSettings(models.SettingsGlobal{DownloadDir: t.TempDir(), DefaultIntervalMinutes: 10}))

	w := serveAuthed(mux, http.MethodGet, "/api/downloader-assistant/dead-scan", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var cfg core.DeadTorrentScanSettings
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cfg))
	assert.Equal(t, core.DefaultDeadTorrentScanIntervalH, cfg.IntervalHours)

	w = serveAuthed(mux, http.MethodPut, "/api/downloader-assistant/dead-scan", `{"enabled":true,"interval_hours":24,"channel_ids":[]}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "开启要选通道")
	conf := models.NotificationConf{ChannelType: "webhook", Name: "w", Enabled: true}
	require.NoError(t, global.GlobalDB.DB.Create(&conf).Error)
	w = serveAuthed(mux, http.MethodPut, "/api/downloader-assistant/dead-scan", fmt.Sprintf(`{"enabled":true,"interval_hours":12,"channel_ids":[%d]}`, conf.ID))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cfg))
	assert.True(t, cfg.Enabled)
	assert.Equal(t, 12, cfg.IntervalHours)

	w = serveAuthed(mux, http.MethodPut, "/api/downloader-assistant/dead-scan", "{")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	w = serveAuthed(mux, http.MethodPost, "/api/downloader-assistant/dead-scan", "{}")
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)

	server.store = nil
	w = serveAuthed(mux, http.MethodGet, "/api/downloader-assistant/dead-scan", "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}
