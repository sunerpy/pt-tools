package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 删除刷流任务时选留下的种子怎么办（DELETE /api/brush/tasks/{id}?torrents=）：handover（默认）交给自动删种，
// keep 给任务名下进行中的种子逐个加上 pt-tools-keep 并交还刷流记录，全部成功（或种子已经不在下载器里）才删任务。

// brushDeleteDL 是假下载器：记下加、去标签的请求，以及加标签那一刻删除锁是不是被拿着（见 internal.RemovalMutex）。
type brushDeleteDL struct {
	*keepFakeDownloader
	lockedWhileTagging []bool
}

func (d *brushDeleteDL) SetTorrentTags(id, tags string) error {
	mu := ptinternal.RemovalMutex()
	free := mu.TryLock()
	if free {
		mu.Unlock()
	}
	d.lockedWhileTagging = append(d.lockedWhileTagging, !free)
	return d.keepFakeDownloader.SetTorrentTags(id, tags)
}

// brushDeleteRig 是带刷流路由（经 s.auth）的网页服务、下载器 qb 上的一个刷流任务，以及按名字登记的假下载器。
type brushDeleteRig struct {
	srv  *Server
	repo *models.BrushRepository
	task models.BrushTask
	dls  map[string]*brushDeleteDL
	ids  map[string]uint
	serv func(method, path string) (int, string)
}

func newBrushDeleteRig(t *testing.T) *brushDeleteRig {
	t.Helper()
	srv, mux, qb := newBrushServer(t)
	rig := &brushDeleteRig{
		srv: srv, repo: models.NewBrushRepository(global.GlobalDB.DB),
		dls: map[string]*brushDeleteDL{}, ids: map[string]uint{"qb": qb.ID},
	}
	rig.serv = func(method, path string) (int, string) {
		w := serveAuthed(mux, method, path, "")
		return w.Code, w.Body.String()
	}
	dm := srv.mgr.GetDownloaderManager()
	factory := func(_ downloader.DownloaderConfig, name string) (downloader.Downloader, error) {
		if d, ok := rig.dls[name]; ok {
			return d, nil
		}
		return nil, fmt.Errorf("没有假下载器 %s", name)
	}
	dm.RegisterFactory(downloader.DownloaderQBittorrent, factory)
	dm.RegisterFactory(downloader.DownloaderTransmission, factory)
	rig.fake(t, "qb", downloader.DownloaderQBittorrent)
	rig.task = models.BrushTask{Name: "hdsky 刷流", SiteName: "hdsky", DownloaderID: qb.ID, IntervalMin: 10, MaxDownloading: 3}
	require.NoError(t, rig.repo.SaveTask(&rig.task))
	return rig
}

// fake 登记一台连得上的假下载器（qb 的设置 newBrushServer 已经建了）。
func (r *brushDeleteRig) fake(t *testing.T, name string, typ downloader.DownloaderType) *brushDeleteDL {
	t.Helper()
	if _, ok := r.ids[name]; !ok {
		r.setting(t, name, typ, true)
	}
	d := &brushDeleteDL{keepFakeDownloader: &keepFakeDownloader{fakeDownloader: &fakeDownloader{name: name, dlType: typ}}}
	r.dls[name] = d
	require.NoError(t, r.srv.mgr.GetDownloaderManager().RegisterConfig(name, downloader.NewGenericConfig(typ, "http://127.0.0.1:1", "u", "p", true), false))
	return d
}

// setting 只在设置里建一台下载器（不登记到下载器管理器：取它的实例会失败，相当于连不上）。
func (r *brushDeleteRig) setting(t *testing.T, name string, typ downloader.DownloaderType, enabled bool) uint {
	t.Helper()
	ds := models.DownloaderSetting{Name: name, Type: string(typ), URL: "http://127.0.0.1:1", Enabled: true}
	require.NoError(t, global.GlobalDB.DB.Create(&ds).Error)
	if !enabled {
		require.NoError(t, global.GlobalDB.DB.Model(&ds).Update("enabled", false).Error)
	}
	r.ids[name] = ds.ID
	return ds.ID
}

// record 记一个刷流任务加的种子；dl 为空时是下载器编号为 0 的旧记录（按任务的下载器）。
func (r *brushDeleteRig) record(t *testing.T, dl, hash, state string) models.BrushTorrent {
	t.Helper()
	bt := models.BrushTorrent{
		TaskID: r.task.ID, InfoHash: hash, SiteName: "hdsky", TorrentID: hash, Title: "T-" + hash,
		DownloaderID: r.ids[dl], State: state, AddedAt: time.Now(),
	}
	require.NoError(t, global.GlobalDB.DB.Create(&bt).Error)
	return bt
}

func (r *brushDeleteRig) path(query string) string {
	return fmt.Sprintf("/api/brush/tasks/%d%s", r.task.ID, query)
}

func (r *brushDeleteRig) taskExists(t *testing.T) bool {
	t.Helper()
	code, body := r.serv(http.MethodGet, r.path(""))
	require.Contains(t, []int{http.StatusOK, http.StatusNotFound}, code, body)
	return code == http.StatusOK
}

func (r *brushDeleteRig) row(t *testing.T, id uint) models.BrushTorrent {
	t.Helper()
	var bt models.BrushTorrent
	require.NoError(t, global.GlobalDB.DB.First(&bt, id).Error)
	return bt
}

// brushDeleteBody 是删除接口回的 JSON（选了保留时）。
type brushDeleteBody struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Kept    int    `json:"kept"`
	Missing int    `json:"missing"`
	Failed  []struct {
		Title          string `json:"title"`
		InfoHash       string `json:"info_hash"`
		DownloaderID   uint   `json:"downloader_id"`
		DownloaderName string `json:"downloader_name"`
		Message        string `json:"message"`
	} `json:"failed"`
}

func decodeBrushDelete(t *testing.T, body string) brushDeleteBody {
	t.Helper()
	var out brushDeleteBody
	require.NoError(t, json.Unmarshal([]byte(body), &out), body)
	return out
}

// 选「保留」：qBittorrent 用 addTags 加 pt-tools-keep，Transmission 读出原来的 labels 再整份写回；
// 本来就带保留标签的不再加；已经不在下载器里的不算失败；下载器编号为 0 的旧记录按任务的下载器；
// 已经结束的刷流记录不碰。全部成功后任务和它的刷流记录一起删掉。加标签都在删除锁里。
func TestBrushDelete_KeepTagsTorrentsThenDeletes(t *testing.T) {
	r := newBrushDeleteRig(t)
	qb := r.dls["qb"]
	tr := r.fake(t, "tr", downloader.DownloaderTransmission)
	tag := models.BrushTaskTag(r.task.ID)
	qb.torrents = []downloader.Torrent{
		{ID: "h1", InfoHash: "h1", Tags: "hdsky,pt-tools-brush," + tag},
		{ID: "h0", InfoHash: "h0", Tags: "hdsky,pt-tools-brush," + tag},
		{ID: "h2", InfoHash: "h2", Tags: "hdsky,pt-tools-brush," + tag + ",pt-tools-keep"},
		{ID: "ended", InfoHash: "ended", Tags: "hdsky,pt-tools-brush," + tag},
	}
	tr.torrents = []downloader.Torrent{{ID: "5", InfoHash: "ABC", Tags: "movie," + tag, Category: "movie", Label: "movie"}}
	r.record(t, "qb", "h1", models.BrushTorrentActive)
	r.record(t, "", "h0", models.BrushTorrentActive)
	r.record(t, "qb", "h2", models.BrushTorrentActive)
	r.record(t, "qb", "missing", models.BrushTorrentActive)
	r.record(t, "qb", "ended", models.BrushTorrentRemoved)
	r.record(t, "tr", "abc", models.BrushTorrentActive)

	code, body := r.serv(http.MethodDelete, r.path("?torrents=keep"))
	require.Equal(t, http.StatusOK, code, body)
	got := decodeBrushDelete(t, body)
	assert.True(t, got.Success)
	assert.Equal(t, 4, got.Kept, "h1、h0、h2 与 Transmission 上的 abc")
	assert.Equal(t, 1, got.Missing)
	assert.Empty(t, got.Failed)

	assert.Equal(t, [][2]string{{"h1", models.KeepTag}, {"h0", models.KeepTag}}, qb.setTags, "带着保留标签的 h2 不再加；已经结束的不碰")
	assert.Equal(t, [][2]string{{"5", "movie," + tag + ",pt-tools-keep"}}, tr.setTags)
	assert.Equal(t, []bool{true, true}, qb.lockedWhileTagging, "加标签时拿着删除锁")
	assert.Equal(t, []bool{true}, tr.lockedWhileTagging)
	assert.Empty(t, qb.removeTags)

	assert.False(t, r.taskExists(t))
	var n int64
	require.NoError(t, global.GlobalDB.DB.Model(&models.BrushTorrent{}).Where("task_id = ?", r.task.ID).Count(&n).Error)
	assert.Zero(t, n, "任务的刷流记录一起删掉")
}

// 有种子保留失败（标签加不上、读不出种子列表、下载器连不上或没启用）：任务不删，回 502，列出是哪几个；
// 加上了标签的保持保留，刷流记录已经交还（gone）。之后改选「交给自动删种」照常删除，已经保留的标签不撤回。
func TestBrushDelete_KeepFailuresLeaveTask(t *testing.T) {
	r := newBrushDeleteRig(t)
	qb := r.dls["qb"]
	tr := r.fake(t, "tr", downloader.DownloaderTransmission)
	bad := r.fake(t, "qb-list", downloader.DownloaderQBittorrent)
	r.setting(t, "qb-down", downloader.DownloaderQBittorrent, true)
	r.setting(t, "qb-off", downloader.DownloaderQBittorrent, false)
	qb.torrents = []downloader.Torrent{{ID: "h1", InfoHash: "h1"}}
	tr.torrents = []downloader.Torrent{{ID: "7", InfoHash: "t1", Tags: "movie"}}
	tr.setErr = errors.New("boom")
	bad.listErr = errors.New("timeout")
	kept := r.record(t, "qb", "h1", models.BrushTorrentActive)
	r.record(t, "tr", "t1", models.BrushTorrentActive)
	r.record(t, "qb-list", "l1", models.BrushTorrentActive)
	r.record(t, "qb-down", "d1", models.BrushTorrentActive)
	r.record(t, "qb-off", "o1", models.BrushTorrentActive)
	r.ids["ghost"] = 9999 // 下载器已经从设置里删掉了
	r.record(t, "ghost", "g1", models.BrushTorrentActive)

	code, body := r.serv(http.MethodDelete, r.path("?torrents=keep"))
	require.Equal(t, http.StatusBadGateway, code, body)
	got := decodeBrushDelete(t, body)
	assert.False(t, got.Success)
	assert.Equal(t, "有 5 个种子保留失败，任务没有删除", got.Error)
	assert.Equal(t, 1, got.Kept)
	require.Len(t, got.Failed, 5)
	byHash := map[string]string{}
	for _, f := range got.Failed {
		byHash[f.InfoHash] = f.DownloaderName + "｜" + f.Title + "｜" + f.Message
		assert.NotZero(t, f.DownloaderID)
	}
	assert.Contains(t, byHash["t1"], "tr｜T-t1｜加保留标签失败: boom")
	assert.Contains(t, byHash["l1"], "qb-list｜T-l1｜读取种子列表失败: timeout")
	assert.Contains(t, byHash["d1"], "qb-down｜T-d1｜下载器不可用")
	assert.Contains(t, byHash["o1"], "qb-off｜T-o1｜下载器 qb-off 未启用")
	assert.Equal(t, "｜T-g1｜下载器 9999 已经不存在", byHash["g1"])

	assert.True(t, r.taskExists(t), "有种子保留失败时任务不删")
	assert.Equal(t, [][2]string{{"h1", models.KeepTag}}, qb.setTags)
	row := r.row(t, kept.ID)
	assert.Equal(t, models.BrushTorrentGone, row.State, "保留下来的已经交还给用户")
	assert.Equal(t, "删除刷流任务时选了保留，交还给用户", row.RemoveReason)
	active, err := r.repo.ActiveTorrents(r.task.ID)
	require.NoError(t, err)
	assert.Len(t, active, 5, "保留失败的仍归任务管")

	code, body = r.serv(http.MethodDelete, r.path("?torrents=handover"))
	require.Equal(t, http.StatusOK, code, body)
	assert.False(t, r.taskExists(t))
	assert.Empty(t, qb.removeTags, "已经保留的标签不撤回")
}

// torrents 只认 handover 与 keep：不带也是 handover（只删任务，不碰下载器），别的值回 400、任务不动。
func TestBrushDelete_TorrentsParam(t *testing.T) {
	for _, q := range []string{"?torrents=remove", "?torrents=KEEP", "?torrents="} {
		t.Run(q, func(t *testing.T) {
			r := newBrushDeleteRig(t)
			r.dls["qb"].torrents = []downloader.Torrent{{ID: "h1", InfoHash: "h1"}}
			r.record(t, "qb", "h1", models.BrushTorrentActive)
			code, body := r.serv(http.MethodDelete, r.path(q))
			if q == "?torrents=" {
				require.Equal(t, http.StatusOK, code, body)
				assert.False(t, r.taskExists(t))
				return
			}
			assert.Equal(t, http.StatusBadRequest, code, body)
			assert.Contains(t, body, "handover")
			assert.True(t, r.taskExists(t))
		})
	}
	for _, q := range []string{"", "?torrents=handover"} {
		t.Run("交给自动删种"+q, func(t *testing.T) {
			r := newBrushDeleteRig(t)
			r.dls["qb"].torrents = []downloader.Torrent{{ID: "h1", InfoHash: "h1"}}
			r.record(t, "qb", "h1", models.BrushTorrentActive)
			code, body := r.serv(http.MethodDelete, r.path(q))
			require.Equal(t, http.StatusOK, code, body)
			assert.JSONEq(t, `{"success":true}`, body)
			assert.False(t, r.taskExists(t))
			assert.Empty(t, r.dls["qb"].setTags, "交给自动删种：不加标签")
		})
	}
}

// 任务正在运行一轮：选「保留」也回 409、不加标签；任务不存在回 404。
func TestBrushDelete_KeepBusyAndNotFound(t *testing.T) {
	r := newBrushDeleteRig(t)
	r.dls["qb"].torrents = []downloader.Torrent{{ID: "h1", InfoHash: "h1"}}
	r.record(t, "qb", "h1", models.BrushTorrentActive)
	mon := scheduler.NewBrushMonitor(scheduler.BrushMonitorConfig{DB: global.GlobalDB.DB})
	r.srv.mgr.SetBrushMonitor(mon)
	require.NoError(t, mon.WithTaskLock(r.task.ID, func() error {
		code, body := r.serv(http.MethodDelete, r.path("?torrents=keep"))
		assert.Equal(t, http.StatusConflict, code, body)
		return nil
	}))
	assert.Empty(t, r.dls["qb"].setTags)
	assert.True(t, r.taskExists(t))

	code, body := r.serv(http.MethodDelete, "/api/brush/tasks/999?torrents=keep")
	assert.Equal(t, http.StatusNotFound, code, body)
}

// 没有下载器管理器（服务还没起好）：种子都算保留失败，任务不删；读不出下载器设置时回 500，任务也不删。
func TestBrushDelete_KeepWithoutDownloaders(t *testing.T) {
	r := newBrushDeleteRig(t)
	r.record(t, "qb", "h1", models.BrushTorrentActive)
	bare := NewServer(core.NewConfigStore(global.GlobalDB), nil) // 没有调度器，也就没有下载器管理器
	t.Cleanup(bare.background.Wait)
	w := httptest.NewRecorder()
	bare.brushTaskCRUD(w, httptest.NewRequest(http.MethodDelete, r.path("?torrents=keep"), nil), r.repo, r.task.ID)
	require.Equal(t, http.StatusBadGateway, w.Code, w.Body.String())
	got := decodeBrushDelete(t, w.Body.String())
	require.Len(t, got.Failed, 1)
	assert.Equal(t, "下载器管理器未初始化", got.Failed[0].Message)
	assert.Equal(t, "qb", got.Failed[0].DownloaderName)
	assert.True(t, r.taskExists(t))

	require.NoError(t, global.GlobalDB.DB.Migrator().DropTable(&models.DownloaderSetting{}))
	code, body := r.serv(http.MethodDelete, r.path("?torrents=keep"))
	assert.Equal(t, http.StatusInternalServerError, code, body)
	assert.Contains(t, body, "读取下载器失败")
	_, err := r.repo.GetTask(r.task.ID)
	assert.NoError(t, err, "任务还在（任务详情要读下载器表，这里直接查库）")
}

// 任务名下没有进行中的种子：选「保留」直接删任务，不连下载器。
func TestBrushDelete_KeepWithoutTorrents(t *testing.T) {
	r := newBrushDeleteRig(t)
	r.record(t, "qb", "old", models.BrushTorrentRemoved)
	code, body := r.serv(http.MethodDelete, r.path("?torrents=keep"))
	require.Equal(t, http.StatusOK, code, body)
	assert.JSONEq(t, `{"success":true}`, body)
	assert.False(t, r.taskExists(t))
	assert.Empty(t, r.dls["qb"].setTags)
}

// App API 与 MCP 删除刷流任务都经配置网关（appBrushChange → ConfigApply），按默认值交给自动删种：不加标签。
// 网关的删除预览里写明这一点。
func TestBrushDelete_ConfigGatewayHandsOver(t *testing.T) {
	r := newBrushDeleteRig(t)
	r.srv.buildHandler()
	r.dls["qb"].torrents = []downloader.Torrent{{ID: "h1", InfoHash: "h1"}}
	r.record(t, "qb", "h1", models.BrushTorrentActive)
	id := strconv.FormatUint(uint64(r.task.ID), 10)
	preview, err := r.srv.ConfigPreview(context.Background(), "brush_tasks", ConfigChange{Op: configOpDelete, ID: id})
	require.NoError(t, err)
	assert.Contains(t, preview.Impact, "删除任务时，留下的种子交给自动删种")
	_ = configRoundTrip(t, r.srv, "brush_tasks", ConfigChange{Op: configOpDelete, ID: id})
	assert.False(t, r.taskExists(t))
	assert.Empty(t, r.dls["qb"].setTags)
}
