package recognize

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// 种子名用英文名或罗马音时，中文搜索结果里的中文名与原名对不上，再比 TMDB 的英文名与别名。
func TestRecognizeByEnglishAndAlternativeTitles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q, lang := r.URL.Query().Get("query"), r.URL.Query().Get("language")
		switch {
		case r.URL.Path == "/3/search/movie" && strings.Contains(q, "Your Name"):
			_, _ = w.Write([]byte(`{"results":[{"id":372058,"title":"你的名字。","original_title":"君の名は。","release_date":"2016-08-26","popularity":90}]}`))
		case r.URL.Path == "/3/movie/372058" && lang == "en-US":
			_, _ = w.Write([]byte(`{"id":372058,"title":"Your Name.","alternative_titles":{"titles":[{"title":"Kimi no Na wa."}]}}`))
		case r.URL.Path == "/3/movie/372058":
			_, _ = w.Write([]byte(`{"id":372058,"title":"你的名字。","original_title":"君の名は。","release_date":"2016-08-26","overview":"……"}`))
		case r.URL.Path == "/3/search/tv" && strings.Contains(q, "Sousou no Frieren"):
			_, _ = w.Write([]byte(`{"results":[{"id":209867,"name":"葬送的芙莉莲","original_name":"葬送のフリーレン","first_air_date":"2023-09-29","popularity":80}]}`))
		case r.URL.Path == "/3/tv/209867" && lang == "en-US":
			_, _ = w.Write([]byte(`{"id":209867,"name":"Frieren: Beyond Journey's End","alternative_titles":{"results":[{"title":"Sousou no Frieren"}]}}`))
		case r.URL.Path == "/3/tv/209867":
			_, _ = w.Write([]byte(`{"id":209867,"name":"葬送的芙莉莲","original_name":"葬送のフリーレン","first_air_date":"2023-09-29","genres":[{"id":16,"name":"动画"}]}`))
		case r.URL.Path == "/3/search/movie" && strings.Contains(q, "Kimi"):
			// 名字对得上别名、年份差太多：不认
			_, _ = w.Write([]byte(`{"results":[{"id":372058,"title":"你的名字。","original_title":"君の名は。","release_date":"2016-08-26","popularity":90}]}`))
		case strings.HasPrefix(r.URL.Path, "/3/search/"):
			_, _ = w.Write([]byte(`{"results":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.MediaSetting{}, &models.MediaWordRule{}, &models.MediaOverride{}, &models.MediaCache{}))
	s := New(Config{DB: db, Cipher: fakeCipher{}, BaseURL: srv.URL + "/3", RatePerSecond: 1000})
	withKey(t, s)
	ctx := context.Background()

	res, err := s.Recognize(ctx, Input{Title: "Your.Name.2016.1080p.BluRay.x264-WiKi"})
	require.NoError(t, err)
	require.NotNil(t, res.Match, res.Message)
	assert.Equal(t, 372058, res.Match.ID)
	assert.Equal(t, "你的名字。", res.Match.Title, "认定后用中文详情")
	assert.Equal(t, SourceSearch, res.Source)

	res, err = s.Recognize(ctx, Input{Title: "[Group] Sousou no Frieren - 01 [1080p]"})
	require.NoError(t, err)
	require.NotNil(t, res.Match, res.Message)
	assert.Equal(t, 209867, res.Match.ID)

	res, err = s.Recognize(ctx, Input{Title: "Kimi.no.Na.wa.1990.1080p.mkv"})
	require.NoError(t, err)
	assert.Nil(t, res.Match, "别名相同但年份差得多")
	assert.NotEmpty(t, res.Candidates)
}
