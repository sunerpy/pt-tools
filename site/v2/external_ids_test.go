package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeIMDbID(t *testing.T) {
	cases := map[string]string{
		"https://www.imdb.com/title/tt0111161/":           "tt0111161",
		"http://imdb.com/title/tt0111161?ref_=fn_al_tt_1": "tt0111161",
		"https://m.imdb.com/title/tt12345678/":            "tt12345678",
		"tt0111161":                                       "tt0111161",
		" TT0111161 ":                                     "tt0111161",
		"":                                                "",
		"none":                                            "",
		"tt12":                                            "",
		"abctt0111161":                                    "",
	}
	for in, want := range cases {
		assert.Equal(t, want, NormalizeIMDbID(in), in)
	}
}

func TestNormalizeDoubanID(t *testing.T) {
	cases := map[string]string{
		"https://movie.douban.com/subject/1292052/":   "1292052",
		"http://movie.douban.com/subject/1292052":     "1292052",
		"https://www.douban.com/subject/1292052/":     "1292052",
		"https://m.douban.com/movie/subject/1292052/": "1292052",
		"douban.com/subject/1292052":                  "1292052",
		"1292052":                                     "1292052",
		" 1292052 ":                                   "1292052",
		"https://book.douban.com/subject/1084336/":    "",
		"https://music.douban.com/subject/1394539/":   "",
		"https://movie.douban.com/celebrity/1054521/": "",
		"":   "",
		"12": "",
	}
	for in, want := range cases {
		assert.Equal(t, want, NormalizeDoubanID(in), in)
	}
}

// NexusPHP 详情页：IMDb 与豆瓣链接可能在链接上，也可能写在简介正文里（◎IMDb链接、◎豆瓣链接）。
func TestNexusPHPParser_ParseExternalIDs(t *testing.T) {
	parser := NewNexusPHPParser()
	doc := parseHTML(t, `<html>
		<input name="torrent_name" value="The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi">
		<input name="detail_torrent_id" value="10086">
		<div id="kdescr">◎译　　名　肖申克的救赎<br>
		◎IMDb链接　<a href="https://www.imdb.com/title/tt0111161/">https://www.imdb.com/title/tt0111161/</a><br>
		◎豆瓣链接　https://movie.douban.com/subject/1292052/<br>
		◎推荐　https://www.imdb.com/title/tt0068646/</div>
	</html>`)
	info := parser.ParseAll(doc)
	assert.Equal(t, "tt0111161", info.IMDbID, "取第一个 IMDb 链接")
	assert.Equal(t, "1292052", info.DoubanID)

	none := parser.ParseAll(parseHTML(t, `<html><input name="torrent_name" value="x"></html>`))
	assert.Empty(t, none.IMDbID)
	assert.Empty(t, none.DoubanID)

	// 站点定义可以换正则
	custom := NewNexusPHPParserFromDefinition(&SiteDefinition{DetailParser: &DetailParserConfig{
		IMDbRegex:   `imdb:(tt\d+)`,
		DoubanRegex: `db:(\d+)`,
	}})
	got := custom.ParseAll(parseHTML(t, `<html><p>imdb:tt7654321 db:30122633</p></html>`))
	assert.Equal(t, "tt7654321", got.IMDbID)
	assert.Equal(t, "30122633", got.DoubanID)
}

func TestNexusPHPDriver_GetTorrentDetail_ExternalIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body>
			<input name="torrent_name" value="Detail.Movie.2024.1080p">
			<input name="detail_torrent_id" value="42">
			<a href="https://www.imdb.com/title/tt1375666/">IMDb</a>
			<a href="https://movie.douban.com/subject/3541415/">豆瓣</a>
		</body></html>`))
	}))
	defer srv.Close()
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: srv.URL, Cookie: "c=1"})
	item, err := d.GetTorrentDetail(context.Background(), "42", srv.URL+"/details.php?id=42", "")
	require.NoError(t, err)
	assert.Equal(t, "tt1375666", item.IMDbID)
	assert.Equal(t, "3541415", item.DoubanID)
}

func TestMTorrentDriver_ExternalIDs(t *testing.T) {
	d := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.m-team.cc", APIKey: "k"})
	items, err := d.ParseSearch(MTorrentResponse{Code: "0", Data: []byte(`{"data":[
		{"id":"1","name":"A","size":"1","status":{"discount":"NORMAL"},"category":"401",
		 "imdb":"https://www.imdb.com/title/tt0111161/","douban":"https://movie.douban.com/subject/1292052/"},
		{"id":"2","name":"B","size":"1","status":{"discount":"NORMAL"},"category":"401","imdb":"","douban":null}
	],"total":2}`)})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "tt0111161", items[0].IMDbID)
	assert.Equal(t, "1292052", items[0].DoubanID)
	assert.Empty(t, items[1].IMDbID)
	assert.Empty(t, items[1].DoubanID)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{
			"id":"555","name":"Detail.Movie","size":"1","status":{"discount":"FREE"},
			"imdb":"tt1375666","douban":"https://movie.douban.com/subject/3541415/"}}`))
	}))
	defer srv.Close()
	dd := NewMTorrentDriver(MTorrentDriverConfig{BaseURL: srv.URL, APIKey: "k"})
	item, err := dd.GetTorrentDetail(context.Background(), "555", "", "")
	require.NoError(t, err)
	assert.Equal(t, "tt1375666", item.IMDbID)
	assert.Equal(t, "3541415", item.DoubanID)
}
