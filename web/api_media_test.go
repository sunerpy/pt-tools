package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
)

const mediaTestKey = "0123456789abcdef0123456789abcdef"

// newMediaServer 起一个接好媒体识别服务的接口，TMDB 指向本地假服务（认 mediaTestKey，能搜到肖申克的救赎）。
func newMediaServer(t *testing.T) *http.ServeMux {
	t.Helper()
	srv := setupServer(t)
	srv.mgr.StopAll()
	db := global.GlobalDB.DB
	require.NoError(t, db.AutoMigrate(&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{}))
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("api_key") != mediaTestKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/3/configuration":
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/3/search/movie" && strings.Contains(r.URL.Query().Get("query"), "Shawshank"):
			_, _ = w.Write([]byte(`{"results":[{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23"}]}`))
		case strings.HasPrefix(r.URL.Path, "/3/search/"):
			_, _ = w.Write([]byte(`{"results":[]}`))
		case r.URL.Path == "/3/movie/278":
			_, _ = w.Write([]byte(`{"id":278,"title":"肖申克的救赎","original_title":"The Shawshank Redemption","release_date":"1994-09-23","overview":"x"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(fake.Close)
	srv.SetMediaService(recognize.New(recognize.Config{DB: db, Cipher: apiTestCipher{}, BaseURL: fake.URL + "/3", RatePerSecond: 1000}))
	mux := http.NewServeMux()
	srv.registerMediaRoutes(mux)
	srv.sessions.put("sess-test", "admin")
	return mux
}

func TestMediaAPI_RequiresSessionAndService(t *testing.T) {
	mux := newMediaServer(t)
	for _, c := range []struct{ method, path string }{
		{http.MethodGet, "/api/media/settings"},
		{http.MethodPut, "/api/media/settings"},
		{http.MethodPost, "/api/media/settings/test"},
		{http.MethodPost, "/api/media/recognize"},
		{http.MethodGet, "/api/media/overrides"},
		{http.MethodPost, "/api/media/overrides"},
		{http.MethodDelete, "/api/media/overrides/1"},
		{http.MethodGet, "/api/media/words"},
		{http.MethodPost, "/api/media/words"},
		{http.MethodPut, "/api/media/words/1"},
		{http.MethodDelete, "/api/media/words/1"},
	} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(c.method, c.path, nil))
		assert.Equal(t, http.StatusUnauthorized, w.Code, c.path)
	}

	srv := setupServer(t)
	srv.mgr.StopAll()
	bare := http.NewServeMux()
	srv.registerMediaRoutes(bare)
	srv.sessions.put("sess-test", "admin")
	assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(bare, http.MethodGet, "/api/media/settings", "").Code)
	assert.Equal(t, http.StatusServiceUnavailable, serveAuthed(bare, http.MethodPost, "/api/media/recognize", `{"title":"x"}`).Code)
}

func TestMediaAPI_SettingsAndRecognize(t *testing.T) {
	mux := newMediaServer(t)
	w := serveAuthed(mux, http.MethodGet, "/api/media/settings", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"has_tmdb_key":false,"language":"zh-CN","proxy_url":""}`, w.Body.String())

	// 没有 Key：识别只做解析；测试连接回 400
	w = serveAuthed(mux, http.MethodPost, "/api/media/recognize", `{"title":"The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"source":"none"`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/settings/test", "").Code)

	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/media/settings", `{"tmdb_key":"x"}`).Code, "拼错的字段")
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPut, "/api/media/settings", `{"language":"fr-FR"}`).Code)
	w = serveAuthed(mux, http.MethodPut, "/api/media/settings", `{"tmdb_api_key":"BADKEY0123456789abcdef","proxy_url":"http://u:secret@127.0.0.1:7890"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "BADKEY", "API Key 只写不读")
	assert.NotContains(t, w.Body.String(), "secret", "代理密码不回显")
	// 代理指向本地一个不可达的端口：先清掉代理再测
	w = serveAuthed(mux, http.MethodPut, "/api/media/settings", `{"proxy_url":""}`)
	require.Equal(t, http.StatusOK, w.Code)
	w = serveAuthed(mux, http.MethodPost, "/api/media/settings/test", "")
	assert.Equal(t, http.StatusBadRequest, w.Code, "Key 无效")
	assert.Contains(t, w.Body.String(), "TMDB API Key 无效")

	require.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodPut, "/api/media/settings", `{"tmdb_api_key":"`+mediaTestKey+`"}`).Code)
	w = serveAuthed(mux, http.MethodPost, "/api/media/settings/test", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/recognize", `{}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/recognize", `{"name":"x"}`).Code)
	w = serveAuthed(mux, http.MethodPost, "/api/media/recognize", `{"title":"The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var res recognize.Result
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.NotNil(t, res.Match)
	assert.Equal(t, 278, res.Match.ID)
	assert.Equal(t, recognize.SourceSearch, res.Source)
	assert.Equal(t, "WiKi", res.Meta.Group)
}

func TestMediaAPI_OverridesAndWords(t *testing.T) {
	mux := newMediaServer(t)
	require.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodPut, "/api/media/settings", `{"tmdb_api_key":"`+mediaTestKey+`"}`).Code)

	w := serveAuthed(mux, http.MethodGet, "/api/media/overrides", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `[]`, w.Body.String())
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/overrides", `{"title":"Some.Movie.2020","tmdb_id":278,"media_type":"music"}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/overrides", `{"title":"Some.Movie.2020","tmdb_id":999,"media_type":"movie"}`).Code, "TMDB 上没有")
	w = serveAuthed(mux, http.MethodPost, "/api/media/overrides", `{"title":"Some.Movie.2020.1080p","tmdb_id":278,"media_type":"movie"}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var ov models.MediaOverride
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ov))
	assert.Equal(t, "肖申克的救赎", ov.Title)
	w = serveAuthed(mux, http.MethodPost, "/api/media/recognize", `{"title":"Some.Movie.2020.2160p"}`)
	assert.Contains(t, w.Body.String(), `"source":"override"`)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodDelete, "/api/media/overrides/x", "").Code)
	assert.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodDelete, "/api/media/overrides/"+strconv.Itoa(int(ov.ID)), "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodDelete, "/api/media/overrides/"+strconv.Itoa(int(ov.ID)), "").Code)

	w = serveAuthed(mux, http.MethodGet, "/api/media/words", "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `[]`, w.Body.String())
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/words", `{"kind":"block","pattern":"(","is_regex":true}`).Code)
	assert.Equal(t, http.StatusBadRequest, serveAuthed(mux, http.MethodPost, "/api/media/words", `{"kind":"block","pattern":"x","extra":1}`).Code)
	w = serveAuthed(mux, http.MethodPost, "/api/media/words", `{"kind":"replace","pattern":"TSR","replacement":"The.Shawshank.Redemption","enabled":true}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var word models.MediaWordRule
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &word))
	w = serveAuthed(mux, http.MethodPost, "/api/media/recognize", `{"title":"TSR.1994.1080p"}`)
	assert.Contains(t, w.Body.String(), `"rule_hits":[`+strconv.Itoa(int(word.ID))+`]`)
	id := strconv.Itoa(int(word.ID))
	w = serveAuthed(mux, http.MethodPut, "/api/media/words/"+id, `{"kind":"replace","pattern":"TSR","replacement":"The.Shawshank.Redemption","enabled":false}`)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Contains(t, w.Body.String(), `"enabled":false`)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodPut, "/api/media/words/999", `{"kind":"block","pattern":"x"}`).Code)
	assert.Equal(t, http.StatusOK, serveAuthed(mux, http.MethodDelete, "/api/media/words/"+id, "").Code)
	assert.Equal(t, http.StatusNotFound, serveAuthed(mux, http.MethodDelete, "/api/media/words/"+id, "").Code)
}
