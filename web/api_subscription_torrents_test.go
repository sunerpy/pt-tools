package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/web/middleware"
)

// fakeSubOrganizer 是订阅里重新整理、刮削用的假整理入库：记下调用，回给定的结果。
type fakeSubOrganizer struct {
	organized []organize.Request
	rescraped []uint
	orgErr    error
}

func (f *fakeSubOrganizer) Retire(context.Context, uint, string) ([]string, error) { return nil, nil }

func (f *fakeSubOrganizer) Retry(context.Context, uint) (*organize.Result, error) {
	return &organize.Result{}, nil
}

func (f *fakeSubOrganizer) Retrying(uint) bool { return false }

func (f *fakeSubOrganizer) Organize(_ context.Context, req organize.Request) (*organize.Result, error) {
	f.organized = append(f.organized, req)
	if f.orgErr != nil {
		return nil, f.orgErr
	}
	return &organize.Result{Done: 1, Messages: []string{`推送失败: Post "http://10.0.0.5:8080/x": EOF`}}, nil
}

func (f *fakeSubOrganizer) Rescrape(_ context.Context, id uint) (*organize.RescrapeResult, error) {
	f.rescraped = append(f.rescraped, id)
	return &organize.RescrapeResult{Records: 1, Written: 2}, nil
}

// appWithTorrent 调带 {id} 与 {tid} 的处理函数。
func appWithTorrent(t testing.TB, h http.HandlerFunc, target, id, tid string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, nil)
	req.SetPathValue("id", id)
	req.SetPathValue("tid", tid)
	req = req.WithContext(middleware.WithPrincipal(req.Context(), &middleware.Principal{Kind: middleware.KindAPIToken, ID: "1", Scopes: []string{"app:read", "app:write"}}))
	w := httptest.NewRecorder()
	h(w, req)
	validateAppResponse(t, req, w)
	return w
}

// subscriptionWithTorrent 建一个剧集订阅和它下载过的一个种子（还有这个种子的整理记录）。
func subscriptionWithTorrent(t *testing.T, srv *Server, hist ...models.MediaTransferHistory) (sub, tor uint) {
	t.Helper()
	w := appAsWith(t, srv.appSubscriptionCreate, http.MethodPost, "/api/app/v1/subscriptions", `{"media_type":"tv","tmdb_id":1001,"season":1}`, "app:write")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var view AppSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &view))
	row := models.MediaSubscriptionTorrent{
		SubscriptionID: view.ID, SiteName: "hdsky", TorrentID: "91", Title: "Show.S01E01.1080p.WEB-DL", InfoHash: "abc123",
		DownloaderID: 3, Status: models.MediaSubTorrentDownloading, Episode: 1, EpisodeEnd: 1,
	}
	require.NoError(t, global.GlobalDB.DB.Create(&row).Error)
	for _, h := range hist {
		h.InfoHash = row.InfoHash
		require.NoError(t, global.GlobalDB.DB.Create(&h).Error)
	}
	return view.ID, row.ID
}

// App API：订阅详情的种子带上每个文件的整理、刮削情况；失败的能重新整理（按订阅的条目），整理好的能重新刮削；
// 不是这个订阅的种子回 404，没有整理好的文件不能刮削
func TestAppSubscriptionTorrentOrganizeAndScrape(t *testing.T) {
	org := &fakeSubOrganizer{}
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.Organizer = org })
	sid, tid := subscriptionWithTorrent(t, srv,
		models.MediaTransferHistory{SourcePath: "/dl/Show/E01.mkv", MediaType: models.MediaKindTV, TMDBID: 1001, Season: 1, Episode: 1, Status: models.MediaTransferDone, ScrapeStatus: models.MediaScrapePartial, ScrapeNote: "豆瓣没取到"},
		models.MediaTransferHistory{SourcePath: `D:\dl\Show\E02.mkv`, MediaType: models.MediaKindTV, TMDBID: 1001, Season: 1, Episode: 2, Status: models.MediaTransferFailed, Message: `Get "http://10.0.0.5:8096/x": EOF`},
	)
	id, torrent := strconv.Itoa(int(sid)), strconv.Itoa(int(tid))

	w := appWithID(t, srv.appSubscriptionDetail, http.MethodGet, "/api/app/v1/subscriptions/"+id, id, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNoSecrets(t, w.Body.Bytes())
	assert.NotContains(t, w.Body.String(), "10.0.0.5", "整理的报错去掉媒体服务器地址")
	var d AppSubscriptionDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &d))
	require.Len(t, d.Torrents, 1)
	got := d.Torrents[0]
	assert.Equal(t, tid, got.ID)
	assert.True(t, got.CanOrganize)
	assert.True(t, got.CanRescrape)
	require.Len(t, got.Files, 2)
	assert.Equal(t, "E01.mkv", got.Files[0].Name)
	assert.Equal(t, models.MediaScrapePartial, got.Files[0].ScrapeStatus)
	assert.Equal(t, "E02.mkv", got.Files[1].Name, "Windows 路径也只留文件名")
	assert.Equal(t, models.MediaTransferFailed, got.Files[1].Status)

	w = appWithTorrent(t, srv.appSubscriptionTorrentOrganize, "/api/app/v1/subscriptions/"+id+"/torrents/"+torrent+"/organize", id, torrent)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "10.0.0.5")
	var res AppOrganizeResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	assert.Equal(t, 1, res.Done)
	require.Len(t, org.organized, 1)
	assert.Equal(t, organize.Request{DownloaderID: 3, Hash: "abc123", MediaType: models.MediaKindTV, TMDBID: 1001}, org.organized[0])

	w = appWithTorrent(t, srv.appSubscriptionTorrentScrape, "/api/app/v1/subscriptions/"+id+"/torrents/"+torrent+"/scrape", id, torrent)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var sr AppRescrapeResult
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &sr))
	assert.Equal(t, AppRescrapeResult{Records: 1, Written: 2, Messages: []string{}}, sr)
	require.Len(t, org.rescraped, 1)
	assert.Equal(t, got.Files[0].ID, org.rescraped[0], "按整理好的那条记录刮削")

	// 运行记录里各记一条
	w = appWithID(t, srv.appSubscriptionRuns, http.MethodGet, "/api/app/v1/subscriptions/"+id+"/runs", id, "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), "重新整理「Show.S01E01.1080p.WEB-DL」")
	assert.Contains(t, w.Body.String(), "重新刮削「Show.S01E01.1080p.WEB-DL」")

	for _, c := range []struct {
		h        http.HandlerFunc
		id, tid  string
		wantCode int
	}{
		{srv.appSubscriptionTorrentOrganize, id, "0", http.StatusBadRequest},
		{srv.appSubscriptionTorrentOrganize, id, "x", http.StatusBadRequest},
		{srv.appSubscriptionTorrentOrganize, id, "99", http.StatusNotFound},
		{srv.appSubscriptionTorrentOrganize, "99", torrent, http.StatusNotFound},
		{srv.appSubscriptionTorrentScrape, id, "99", http.StatusNotFound},
	} {
		assert.Equal(t, c.wantCode, appWithTorrent(t, c.h, "/api/app/v1/subscriptions/"+c.id+"/torrents/"+c.tid+"/organize", c.id, c.tid).Code, c.id+"/"+c.tid)
	}

	// 整理入库回的错误按整理的状态码，原因写进回复（去掉地址）
	org.orgErr = fmt.Errorf("%w: 媒体库停用了", organize.ErrInvalid)
	w = appWithTorrent(t, srv.appSubscriptionTorrentOrganize, "/api/app/v1/subscriptions/"+id+"/torrents/"+torrent+"/organize", id, torrent)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "媒体库停用了")
	org.orgErr = errors.New(`Get "http://10.0.0.5:8080/api/v2/torrents/files": EOF`)
	w = appWithTorrent(t, srv.appSubscriptionTorrentOrganize, "/api/app/v1/subscriptions/"+id+"/torrents/"+torrent+"/organize", id, torrent)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.NotContains(t, w.Body.String(), "10.0.0.5")
	org.orgErr = nil

	// 洗版换掉的旧版本：详情里不能整理、刮削，直接调接口（MCP 带 confirm=true 也是调这两个接口）也回 400，不去整理
	require.NoError(t, global.GlobalDB.DB.Model(&models.MediaSubscriptionTorrent{}).Where("id = ?", tid).Update("status", models.MediaSubTorrentReplaced).Error)
	organized, rescraped := len(org.organized), len(org.rescraped)
	for _, h := range []http.HandlerFunc{srv.appSubscriptionTorrentOrganize, srv.appSubscriptionTorrentScrape} {
		w = appWithTorrent(t, h, "/api/app/v1/subscriptions/"+id+"/torrents/"+torrent+"/organize", id, torrent)
		assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "洗版")
	}
	assert.Len(t, org.organized, organized)
	assert.Len(t, org.rescraped, rescraped)
}

// MCP 带 confirm=true 直接执行（不经过预览）：洗版换掉的旧版本经真实的 App API 与订阅服务被拒，不去整理、刮削，审计记下没成
func TestMCPConfirmedReorganizeOfReplacedTorrentRefused(t *testing.T) {
	org := &fakeSubOrganizer{}
	srv, _ := newSubscribeEnv(t, func(c *subscribe.Config) { c.Organizer = org })
	sid, tid := subscriptionWithTorrent(t, srv,
		models.MediaTransferHistory{SourcePath: "/dl/Show/E01.mkv", MediaType: models.MediaKindTV, TMDBID: 1001, Season: 1, Episode: 1, Status: models.MediaTransferDone},
		models.MediaTransferHistory{SourcePath: "/dl/Show/E02.mkv", MediaType: models.MediaKindTV, TMDBID: 1001, Season: 1, Episode: 2, Status: models.MediaTransferFailed},
	)
	require.NoError(t, global.GlobalDB.DB.Model(&models.MediaSubscriptionTorrent{}).Where("id = ?", tid).Update("status", models.MediaSubTorrentReplaced).Error)
	require.NoError(t, global.GlobalDB.DB.AutoMigrate(&models.APIToken{}))
	tokens := apitoken.New(global.GlobalDB.DB)
	audit := &fakeAppAudit{}
	srv.SetAPITokens(tokens, audit)
	enableMCPForTest(t)
	ts := httptest.NewServer(srv.buildHandler())
	t.Cleanup(ts.Close)
	_, plain, err := tokens.Create(context.Background(), apitoken.CreateInput{Name: "mcp", Scopes: []string{apitoken.ScopeMCPRead, apitoken.ScopeMCPWrite}}, "admin")
	require.NoError(t, err)
	cs, err := mcpSession(t, ts.URL, plain)
	require.NoError(t, err)

	for _, tool := range []string{"reorganize_subscription_torrent", "rescrape_subscription_torrent"} {
		res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: tool, Arguments: map[string]any{"id": sid, "subscription_torrent_id": tid, "confirm": true}})
		require.NoError(t, err)
		var text strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(*sdk.TextContent); ok {
				text.WriteString(tc.Text)
			}
		}
		assert.True(t, res.IsError, tool)
		assert.Contains(t, text.String(), "洗版", tool)
	}
	assert.Empty(t, org.organized, "没有去整理")
	assert.Empty(t, org.rescraped, "没有去刮削")
	a := audit.all()
	require.Len(t, a, 2)
	for _, x := range a {
		assert.Equal(t, "mcp", x.ChannelType)
		assert.NotEqual(t, "success", x.Result, x.Command)
	}
}

// 网页：同样的两个接口（要登录）；没有整理好的文件时重新刮削回 400；没有整理入库时回 400 说明
func TestSubscribeAPI_TorrentOrganizeAndScrape(t *testing.T) {
	org := &fakeSubOrganizer{}
	srv, mux := newSubscribeEnv(t, func(c *subscribe.Config) { c.Organizer = org })
	sid, tid := subscriptionWithTorrent(t, srv,
		models.MediaTransferHistory{SourcePath: "/dl/Show/E01.mkv", MediaType: models.MediaKindTV, TMDBID: 1001, Season: 1, Episode: 1, Status: models.MediaTransferFailed, Message: "目标已有同名文件"})
	base := "/api/media/subscriptions/" + strconv.Itoa(int(sid)) + "/torrents/" + strconv.Itoa(int(tid))

	w := serveAuthed(mux, http.MethodGet, "/api/media/subscriptions/"+strconv.Itoa(int(sid)), "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"can_organize":true`)
	assert.Contains(t, w.Body.String(), `"can_rescrape":false`)
	assert.Contains(t, w.Body.String(), `"name":"E01.mkv"`)

	w = serveAuthed(mux, http.MethodPost, base+"/organize", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"done":1`)
	require.Len(t, org.organized, 1)

	w = serveAuthed(mux, http.MethodPost, base+"/scrape", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "先重新整理")
	assert.Empty(t, org.rescraped)

	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+strconv.Itoa(int(sid))+"/torrents/x/organize", "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodPost, "/api/media/subscriptions/"+strconv.Itoa(int(sid))+"/torrents/99/organize", "").Code)

	// 没有整理入库：说明原因
	bare, bareMux := newSubscribeEnv(t)
	sid, tid = subscriptionWithTorrent(t, bare)
	w = serveAuthed(bareMux, http.MethodPost, "/api/media/subscriptions/"+strconv.Itoa(int(sid))+"/torrents/"+strconv.Itoa(int(tid))+"/organize", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), "整理入库没有启动"), w.Body.String())
}
