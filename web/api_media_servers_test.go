package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
)

// fakeJellyfinAPI 是假的 Jellyfin：/System/Info、/Items、/Users 与 u1 的继续观看。
type fakeJellyfinAPI struct {
	*httptest.Server
	mu    sync.Mutex
	items []string
}

func newFakeJellyfinAPI(t *testing.T) *fakeJellyfinAPI {
	f := &fakeJellyfinAPI{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Authorization"), `Token="jf"`) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/System/Info":
			_, _ = w.Write([]byte(`{"ServerName":"jf","Version":"12.2.0","Id":"jf-uid"}`))
		case "/Items":
			_, _ = fmt.Fprintf(w, `{"Items":[%s],"TotalRecordCount":%d}`, strings.Join(f.items, ","), len(f.items))
		case "/Users":
			_, _ = w.Write([]byte(`[{"Id":"u1","Name":"admin"}]`))
		case "/Users/u1/Items/Resume":
			_, _ = w.Write([]byte(`{"Items":[{"Id":"m1","Name":"肖申克的救赎","Type":"Movie","ProviderIds":{"Tmdb":"278"},"UserData":{"PlayedPercentage":40}}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeJellyfinAPI) set(items ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = items
}

// saveServer 存一台 Jellyfin，回它的编号。
func (e *organizeTestEnv) saveServer(t *testing.T, f *fakeJellyfinAPI, extra string) string {
	t.Helper()
	w := serveAuthed(e.mux, http.MethodPost, "/api/media/servers", `{"name":"Jellyfin","kind":"jellyfin","url":"`+f.URL+
		`","token":"jf","enabled":true,"local_prefix":"/media","server_prefix":"/data"`+extra+`}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var v struct {
		ID uint `json:"id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &v))
	return strconv.FormatUint(uint64(v.ID), 10)
}

func TestMediaServerAPI_RequiresSessionAndService(t *testing.T) {
	e := newOrganizeServer(t)
	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/media/servers/1/sync"},
		{http.MethodPost, "/api/media/servers/users"},
		{http.MethodPost, "/api/media/servers/1/webhook"},
		{http.MethodDelete, "/api/media/servers/1/webhook"},
		{http.MethodGet, "/api/media/resume"},
		{http.MethodGet, "/api/media/issues"},
		{http.MethodPost, "/api/media/issues/1/dismiss"},
	}
	for _, c := range routes {
		w := httptest.NewRecorder()
		e.mux.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, c.method+" "+c.path)
	}

	srv := setupServer(t)
	srv.mgr.StopAll()
	bare := http.NewServeMux()
	srv.registerOrganizeRoutes(bare)
	srv.sessions.put("sess-test", "admin")
	routes = append(routes, struct{ method, path string }{http.MethodPost, "/api/media/servers/1/webhook/x"})
	for _, c := range routes {
		assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(bare, c.method, c.path, "{}").Code, c.method+" "+c.path)
	}
}

func TestMediaServerAPI_SyncUsersResumeIssues(t *testing.T) {
	e := newOrganizeServer(t)
	f := newFakeJellyfinAPI(t)
	f.set(`{"Id":"m1","Name":"肖申克的救赎","Type":"Movie","Path":"/data/movies/肖申克的救赎 (1994)/肖申克的救赎 (1994).mkv","ProviderIds":{"Tmdb":"278"}}`)
	id := e.saveServer(t, f, `,"index_hours":6,"count_library":true,"resume_user_id":"u1","resume_user_name":"admin","play_url":"https://jf.example.com"`)

	w := serveAuthed(e.mux, http.MethodPost, "/api/media/servers/users", `{"id":`+id+`,"kind":"jellyfin","url":"`+f.URL+`"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `[{"id":"u1","name":"admin"}]`, w.Body.String())
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/servers/users", `{"kind":"plex","url":"http://plex:32400","token":"t"}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "Plex 不支持")
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/servers/users", `{"kind":"jellyfin","url":"`+f.URL+`","token":"t","x":1}`)
	assert.Equal(t, http.StatusBadRequest, w.Code, "拼错的字段")

	w = serveAuthed(e.mux, http.MethodPost, "/api/media/servers/"+id+"/sync", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"movies":1`)
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/servers", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"index_movies":1`)
	assert.Contains(t, w.Body.String(), `"index_hours":6`)
	assert.NotContains(t, w.Body.String(), "jf-uid", "服务器 ID 不回给页面")
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodPost, "/api/media/servers/99/sync", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/servers/x/sync", "").Code)

	w = serveAuthed(e.mux, http.MethodGet, "/api/media/resume", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resume struct {
		Servers []struct {
			User  string `json:"user"`
			Items []struct {
				ItemID  string `json:"item_id"`
				OpenURL string `json:"open_url"`
			} `json:"items"`
		} `json:"servers"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resume))
	require.Len(t, resume.Servers, 1)
	require.Len(t, resume.Servers[0].Items, 1)
	assert.Equal(t, "admin", resume.Servers[0].User)
	assert.Equal(t, "m1", resume.Servers[0].Items[0].ItemID)
	assert.Equal(t, "https://jf.example.com/web/#/details?id=m1&serverId=jf-uid", resume.Servers[0].Items[0].OpenURL)

	// 整理过的电影在服务器里没了：开一个问题，能列出与忽略
	require.NoError(t, global.GlobalDB.DB.Create(&models.MediaTransferHistory{
		Status: models.MediaTransferDone, MediaType: models.MediaKindMovie, TMDBID: 278, SourcePath: "/downloads/a.mkv",
		TargetPath: "/media/movies/肖申克的救赎 (1994)/肖申克的救赎 (1994).mkv",
	}).Error)
	f.set()
	require.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodPost, "/api/media/servers/"+id+"/sync", "").Code)
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/issues?status=open&media_type=movie&tmdb_id=278", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var list struct {
		Items []struct {
			ID         uint   `json:"id"`
			ServerName string `json:"server_name"`
			Kind       string `json:"kind"`
		} `json:"items"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list.Items, 1)
	assert.Equal(t, [2]string{"Jellyfin", models.MediaIssueServerRemoved}, [2]string{list.Items[0].ServerName, list.Items[0].Kind})
	issue := strconv.FormatUint(uint64(list.Items[0].ID), 10)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/issues/"+issue+"/dismiss", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"status":"dismissed"`)
	w = serveAuthed(e.mux, http.MethodPost, "/api/media/issues/"+issue+"/dismiss", "")
	require.Equal(t, http.StatusOK, w.Code, "再点一次不报错")
	assert.Contains(t, w.Body.String(), `"status":"dismissed"`)
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodPost, "/api/media/issues/999/dismiss", "").Code)
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/issues?status=open", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"items":[]}`, w.Body.String())
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodGet, "/api/media/issues?status=gone", "").Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/issues/x/dismiss", "").Code)

	// 关掉索引以后不能手动同步
	w = serveAuthed(e.mux, http.MethodPut, "/api/media/servers/"+id, `{"name":"Jellyfin","kind":"jellyfin","url":"`+f.URL+`","enabled":true,"index_hours":0}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, http.StatusBadRequest, serveAuthed(e.mux, http.MethodPost, "/api/media/servers/"+id+"/sync", "").Code)
}

func TestMediaServerAPI_Webhook(t *testing.T) {
	e := newOrganizeServer(t)
	f := newFakeJellyfinAPI(t)
	id := e.saveServer(t, f, "")

	w := serveAuthed(e.mux, http.MethodPost, "/api/media/servers/"+id+"/webhook", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var tok struct{ Token, Path string }
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &tok))
	require.Len(t, tok.Token, 43)
	assert.Equal(t, "/api/media/servers/"+id+"/webhook/"+tok.Token, tok.Path)
	w = serveAuthed(e.mux, http.MethodGet, "/api/media/servers", "")
	assert.Contains(t, w.Body.String(), `"webhook":true`)
	assert.NotContains(t, w.Body.String(), tok.Token, "令牌只回一次")
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodPost, "/api/media/servers/99/webhook", "").Code)

	post := func(path, contentType string, body []byte) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		e.mux.ServeHTTP(rec, req)
		return rec
	}
	added := []byte(`{"NotificationType":"ItemAdded","ItemId":"m1","ItemType":"Movie","Name":"肖申克的救赎"}`)
	assert.Equal(t, http.StatusNoContent, post(tok.Path, "text/plain; charset=utf-8", added).Code, "不认会话，凭令牌")
	assert.Equal(t, http.StatusNotFound, post(tok.Path+"x", "text/plain", added).Code, "令牌不对")
	assert.Equal(t, http.StatusNotFound, post("/api/media/servers/99/webhook/"+tok.Token, "text/plain", added).Code)
	assert.Equal(t, http.StatusNotFound, post("/api/media/servers/x/webhook/"+tok.Token, "text/plain", added).Code)
	assert.Equal(t, http.StatusNotFound, post("/api/media/servers/0/webhook/"+tok.Token, "text/plain", added).Code)
	assert.Equal(t, http.StatusBadRequest, post(tok.Path, "application/json", []byte("nope")).Code)
	big := bytes.Repeat([]byte("a"), 2<<20)
	assert.Equal(t, http.StatusRequestEntityTooLarge, post(tok.Path, "application/json", append([]byte(`{"Event":"`), big...)).Code)

	// Plex 的 multipart：payload 在前，缩略图在后
	var mp bytes.Buffer
	mw := multipart.NewWriter(&mp)
	require.NoError(t, mw.WriteField("payload", `{"event":"library.new","Metadata":{"ratingKey":"9","type":"movie","title":"肖申克的救赎"}}`))
	part, err := mw.CreateFormFile("thumb", "thumb.jpg")
	require.NoError(t, err)
	_, _ = part.Write([]byte("jpeg"))
	require.NoError(t, mw.Close())
	assert.Equal(t, http.StatusNoContent, post(tok.Path, mw.FormDataContentType(), mp.Bytes()).Code)
	var row models.MediaServer
	require.NoError(t, global.GlobalDB.DB.First(&row, id).Error)
	assert.Equal(t, "library.new", row.WebhookEvent)

	// 每分钟最多 60 次：上面令牌对的已经收了 4 次（看不懂的、太大的也算）
	codes := map[int]int{}
	for range 60 {
		codes[post(tok.Path, "text/plain", added).Code]++
	}
	assert.Equal(t, map[int]int{http.StatusNoContent: 56, http.StatusTooManyRequests: 4}, codes)

	assert.Equal(t, http.StatusOK, serveAuthed(e.mux, http.MethodDelete, "/api/media/servers/"+id+"/webhook", "").Code)
	assert.Equal(t, http.StatusNotFound, post(tok.Path, "text/plain", added).Code, "关掉以后不认")
	assert.Equal(t, http.StatusNotFound, serveAuthed(e.mux, http.MethodDelete, "/api/media/servers/99/webhook", "").Code)
}

func TestLogPathMasksWebhookToken(t *testing.T) {
	assert.Equal(t, "/api/media/servers/3/webhook/***", logPath("/api/media/servers/3/webhook/abcDEF_-123"))
	assert.Equal(t, "/api/media/servers/3/webhook", logPath("/api/media/servers/3/webhook"))
	assert.Equal(t, "/api/media/servers", logPath("/api/media/servers"))
	assert.Equal(t, "/x/webhook/y/z", logPath("/x/webhook/y/z"), "别的地址不动")
}
