package tmdb

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

func TestImages(t *testing.T) {
	var hits atomic.Int32
	var query atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		query.Store(r.URL.Query())
		if r.URL.Path != "/3/movie/278/images" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var posters []string
		for i := 1; i <= 12; i++ {
			posters = append(posters, fmt.Sprintf(`{"file_path":"/zh%d.jpg","iso_639_1":"zh","vote_average":%d,"vote_count":3}`, i, i))
		}
		posters = append(posters,
			`{"file_path":"/en.jpg","iso_639_1":"en","vote_average":5,"vote_count":1}`,
			`{"file_path":"/none-few.jpg","iso_639_1":null,"vote_average":9,"vote_count":1}`,
			`{"file_path":"/none-many.jpg","iso_639_1":null,"vote_average":9,"vote_count":40}`,
			`{"file_path":"bad path.jpg","iso_639_1":"zh","vote_average":99}`)
		_, _ = fmt.Fprintf(w, `{"id":278,"posters":[%s],"backdrops":[{"file_path":"/bd.jpg","iso_639_1":null,"vote_average":5}],
			"logos":[{"file_path":"/logo.svg","iso_639_1":"zh","vote_average":9},{"file_path":"/logo.png","iso_639_1":"zh","vote_average":4}]}`,
			strings.Join(posters, ","))
	}))
	t.Cleanup(srv.Close)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.MediaCache{}))
	c, err := New(Options{APIKey: "0123456789abcdef0123456789abcdef", BaseURL: srv.URL + "/3", RatePerSecond: 1000, Cache: NewDBCache(db)})
	require.NoError(t, err)

	imgs, err := c.Images(context.Background(), KindMovie, 278)
	require.NoError(t, err)
	q := query.Load().(url.Values)
	assert.Equal(t, []string{"zh,en,ja,null"}, q["include_image_language"])

	var zh []string
	for _, p := range imgs.Posters {
		if p.Lang == "zh" {
			zh = append(zh, p.Path)
		}
	}
	assert.Len(t, zh, maxImagesPerLang, "每种语言最多留 10 张")
	assert.Equal(t, "/zh12.jpg", zh[0], "按评分从高到低")
	assert.NotContains(t, zh, "/zh1.jpg")
	assert.NotContains(t, zh, "bad path.jpg", "路径格式不对的不要")
	assert.Equal(t, "/zh12.jpg", imgs.Posters[0].Path)
	var none []string
	for _, p := range imgs.Posters {
		if p.Lang == "" {
			none = append(none, p.Path)
		}
	}
	assert.Equal(t, []string{"/none-many.jpg", "/none-few.jpg"}, none, "评分一样时看评分人数；null 是没有文字的")
	assert.Equal(t, []Image{{Path: "/bd.jpg", Vote: 5}}, imgs.Backdrops)
	assert.Equal(t, []Image{{Path: "/logo.png", Lang: "zh", Vote: 4}}, imgs.Logos, "SVG 不要")

	again, err := c.Images(context.Background(), KindMovie, 278)
	require.NoError(t, err)
	assert.Equal(t, imgs, again)
	assert.EqualValues(t, 1, hits.Load(), "第二次读缓存")

	_, err = c.Images(context.Background(), "music", 1)
	require.Error(t, err)
	_, err = c.Images(context.Background(), KindTV, 0)
	require.ErrorIs(t, err, ErrNotFound)
	_, err = c.Images(context.Background(), KindTV, 1399)
	require.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, "zh-CN", c.Language())
}
