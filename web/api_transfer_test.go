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

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// transferFakeDL 只实现预览用到的方法：源里放着种子，目标里什么都没有。
type transferFakeDL struct {
	downloader.Downloader
	torrents []downloader.Torrent
}

func (f *transferFakeDL) GetTorrentsBy(filter downloader.TorrentFilter) ([]downloader.Torrent, error) {
	out := []downloader.Torrent{}
	for _, t := range f.torrents {
		for _, h := range filter.Hashes {
			if t.InfoHash == h {
				out = append(out, t)
			}
		}
	}
	return out, nil
}

func (f *transferFakeDL) GetAllTorrents() ([]downloader.Torrent, error) { return f.torrents, nil }

func (f *transferFakeDL) CheckTorrentExists(hash string) (bool, error) {
	for _, t := range f.torrents {
		if t.InfoHash == hash {
			return true, nil
		}
	}
	return false, nil
}

type transferFakeExporter struct{ *transferFakeDL }

func (transferFakeExporter) ExportTorrent(context.Context, string) ([]byte, error) {
	return nil, errors.New("not used")
}

// newTransferServer 注册转移做种路由，带两台下载器（qb 能导出、tr 是目标）和一个没启动的后台。
func newTransferServer(t *testing.T) (*Server, *http.ServeMux, models.DownloaderSetting, models.DownloaderSetting) {
	t.Helper()
	srv := setupServer(t)
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.TorrentTransferJob{}, &models.DownloaderPathMap{}, &models.TransferRule{}))
	src := models.DownloaderSetting{Name: "qb", Type: "qbittorrent", URL: "http://127.0.0.1:1", Enabled: true}
	dst := models.DownloaderSetting{Name: "tr", Type: "transmission", URL: "http://127.0.0.1:2", Enabled: true}
	require.NoError(t, db.Create(&src).Error)
	require.NoError(t, db.Create(&dst).Error)
	srcDL := transferFakeExporter{&transferFakeDL{torrents: []downloader.Torrent{
		{InfoHash: "aaaa", Name: "Movie.A", TotalSize: 1 << 30, SavePath: "/downloads", Progress: 1, IsCompleted: true},
	}}}
	dstDL := &transferFakeDL{}
	svc := transfer.New(transfer.Config{
		DB: db,
		Downloaders: transfer.DownloadersFunc(func(_ context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error) {
			switch id {
			case src.ID:
				return srcDL, src, nil
			case dst.ID:
				return dstDL, dst, nil
			}
			return nil, models.DownloaderSetting{}, fmt.Errorf("下载器 %d 不存在", id)
		}),
	})
	srv.mgr.SetTransferWorker(scheduler.NewTransferWorker(scheduler.TransferWorkerConfig{Service: svc, DB: db}))
	mux := http.NewServeMux()
	srv.registerTransferRoutes(mux)
	srv.sessions.put("sess-test", "admin")
	return srv, mux, src, dst
}

func TestTransferAPI_RequiresSession(t *testing.T) {
	_, mux, _, _ := newTransferServer(t)
	for _, path := range []string{
		"/api/transfer/preview", "/api/transfer/jobs", "/api/transfer/jobs/1/cancel",
		"/api/transfer/path-maps", "/api/transfer/rules", "/api/transfer/rules/1",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		assert.Contains(t, []int{http.StatusUnauthorized, http.StatusFound}, w.Code, path)
	}
}

func TestTransferAPI_NotStarted(t *testing.T) {
	srv := setupServer(t)
	srv.mgr.StopAll()
	mux := http.NewServeMux()
	srv.registerTransferRoutes(mux)
	srv.sessions.put("sess-test", "admin")
	w := serveAuthed(mux, http.MethodGet, "/api/transfer/jobs", "")
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestTransferAPI_PreviewAndJobs(t *testing.T) {
	_, mux, src, dst := newTransferServer(t)
	body := fmt.Sprintf(`{"target_id":%d,"items":[{"source_id":%d,"hash":"AAAA"},{"source_id":%d,"hash":"bbbb"}]}`, dst.ID, src.ID, src.ID)

	w := serveAuthed(mux, http.MethodPost, "/api/transfer/preview", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var preview struct {
		Items []transfer.PreviewItem `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &preview))
	require.Len(t, preview.Items, 2)
	assert.True(t, preview.Items[0].OK)
	assert.Equal(t, "qb", preview.Items[0].SourceName)
	assert.Equal(t, "源下载器里没有这个种子", preview.Items[1].Reason)

	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/transfer/preview", fmt.Sprintf(`{"target_id":%d}`, dst.ID)).Code)
	assert.Equal(t, http.StatusBadGateway, serveAuthed(mux, http.MethodPost, "/api/transfer/preview", `{"target_id":99,"items":[{"source_id":1,"hash":"a"}]}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/transfer/preview", `{`).Code)
	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(mux, http.MethodGet, "/api/transfer/preview", "").Code)

	w = serveAuthed(mux, http.MethodPost, "/api/transfer/jobs", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var created struct {
		Created []TransferJobView      `json:"created"`
		Skipped []transfer.PreviewItem `json:"skipped"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.Len(t, created.Created, 1)
	assert.Len(t, created.Skipped, 1)
	job := created.Created[0]
	assert.Equal(t, "qb", job.SourceName)
	assert.Equal(t, "tr", job.TargetName)
	assert.Equal(t, models.TransferPending, job.State)

	w = serveAuthed(mux, http.MethodGet, "/api/transfer/jobs?status=active", "")
	require.Equal(t, http.StatusOK, w.Code)
	var list struct {
		Items []TransferJobView `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	assert.False(t, list.Items[0].Final)

	cancelPath := fmt.Sprintf("/api/transfer/jobs/%d/cancel", job.ID)
	assert.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodPost, cancelPath, "").Code)
	assert.Equal(t, http.StatusConflict, serveAuthed(mux, http.MethodPost, cancelPath, "").Code, "已经结束")
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodPost, "/api/transfer/jobs/999/cancel", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/transfer/jobs/x/cancel", "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodPost, fmt.Sprintf("/api/transfer/jobs/%d/retry", job.ID), "").Code)
	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(mux, http.MethodGet, cancelPath, "").Code)

	w = serveAuthed(mux, http.MethodDelete, "/api/transfer/jobs", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"deleted":1}`, w.Body.String())
	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(mux, http.MethodPut, "/api/transfer/jobs", "").Code)
}

func TestTransferAPI_PathMaps(t *testing.T) {
	_, mux, src, dst := newTransferServer(t)
	body := fmt.Sprintf(`{"source_id":%d,"target_id":%d,"items":[{"source_prefix":" /downloads ","target_prefix":"/data"}]}`, src.ID, dst.ID)
	w := serveAuthed(mux, http.MethodPut, "/api/transfer/path-maps", body)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got struct {
		Items []models.DownloaderPathMap `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	require.Len(t, got.Items, 1)
	assert.Equal(t, "/downloads", got.Items[0].SourcePrefix)

	w = serveAuthed(mux, http.MethodGet, fmt.Sprintf("/api/transfer/path-maps?source_id=%d&target_id=%d", src.ID, dst.ID), "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Len(t, got.Items, 1)

	bad := fmt.Sprintf(`{"source_id":%d,"target_id":%d,"items":[{"source_prefix":"/a","target_prefix":""}]}`, src.ID, dst.ID)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/transfer/path-maps", bad).Code)
	same := fmt.Sprintf(`{"source_id":%d,"target_id":%d,"items":[]}`, src.ID, src.ID)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/transfer/path-maps", same).Code)
	dup := fmt.Sprintf(`{"source_id":%d,"target_id":%d,"items":[{"source_prefix":"/a","target_prefix":"/b"},{"source_prefix":"/a/","target_prefix":"/c"}]}`, src.ID, dst.ID)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/transfer/path-maps", dup).Code)

	clear := fmt.Sprintf(`{"source_id":%d,"target_id":%d,"items":[]}`, src.ID, dst.ID)
	w = serveAuthed(mux, http.MethodPut, "/api/transfer/path-maps", clear)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Empty(t, got.Items, "整体替换：空列表清掉这一对的映射")
	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(mux, http.MethodPost, "/api/transfer/path-maps", clear).Code)
}

func TestTransferAPI_Rules(t *testing.T) {
	_, mux, src, dst := newTransferServer(t)
	body := func(name string, srcID, dstID uint) string {
		return fmt.Sprintf(`{"name":%q,"enabled":false,"source_downloader_id":%d,"target_downloader_id":%d,"tag":"keep","min_seeding_hours":24}`, name, srcID, dstID)
	}
	w := serveAuthed(mux, http.MethodPost, "/api/transfer/rules", body("archive", src.ID, dst.ID))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var rule TransferRuleView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rule))
	assert.False(t, rule.Enabled, "关掉的开关原样保存")
	assert.Equal(t, models.TransferRuleDefaultMaxPerRun, rule.MaxPerRun)
	assert.Equal(t, "qb", rule.SourceName)

	assert.Equal(t, http.StatusConflict, serveAuthed(mux, http.MethodPost, "/api/transfer/rules", body("archive", src.ID, dst.ID)).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/transfer/rules", body("x", src.ID, src.ID)).Code)

	path := fmt.Sprintf("/api/transfer/rules/%d", rule.ID)
	w = serveAuthed(mux, http.MethodPut, path, `{"name":"archive","enabled":true,"source_downloader_id":`+fmt.Sprint(src.ID)+`,"target_downloader_id":`+fmt.Sprint(dst.ID)+`,"interval_min":30}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rule))
	assert.True(t, rule.Enabled)
	assert.Equal(t, 30, rule.IntervalMin)

	w = serveAuthed(mux, http.MethodPost, path+"/run", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "建了 1 个任务")
	w = serveAuthed(mux, http.MethodGet, path, "")
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rule))
	require.NotNil(t, rule.LastRunAt)
	assert.Contains(t, rule.LastResult, "建了 1 个任务")

	w = serveAuthed(mux, http.MethodGet, "/api/transfer/rules", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"name":"archive"`)

	assert.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodDelete, path, "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodDelete, path, "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodGet, path, "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodPost, path+"/run", "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodGet, path+"/other", "").Code)
	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(mux, http.MethodPatch, path, "").Code)
	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(mux, http.MethodGet, path+"/run", "").Code)
	assert.Equal(t, http.StatusMethodNotAllowed, serveAuthed(mux, http.MethodDelete, "/api/transfer/rules", "").Code)
}
