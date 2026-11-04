package v2

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subtitleSite 是假的 NexusPHP 站点：details.php 回 pages[id]，downloadsubs.php 交给 download（为空时 404）。
func subtitleSite(t *testing.T, pages map[string]string, download http.HandlerFunc) (*NexusPHPDriver, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Equal(t, "c_secure_uid=1", r.Header.Get("Cookie"))
		switch r.URL.Path {
		case "/details.php":
			assert.Equal(t, "1", r.URL.Query().Get("hit"))
			page, ok := pages[r.URL.Query().Get("id")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(page))
		case "/downloadsubs.php":
			if download == nil {
				http.NotFound(w, r)
				return
			}
			download(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: srv.URL, Cookie: "c_secure_uid=1"}), &hits
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return string(b)
}

func TestNexusPHPListSubtitles(t *testing.T) {
	d, _ := subtitleSite(t, map[string]string{
		"123": readFixture(t, "nexusphp_details_subtitles.html"),
		"88":  readFixture(t, "nexusphp15_details_subtitles.html"),
		"124": readFixture(t, "nexusphp_details_nosubtitles.html"),
	}, nil)

	t.Run("php8 的写法：语言、国旗图、别的种子的字幕", func(t *testing.T) {
		subs, err := d.ListSubtitles(context.Background(), "123")
		require.NoError(t, err)
		assert.Equal(t, []Subtitle{
			{ID: "45", TorrentID: "123", Title: "Movie.2023.1080p.BluRay.chs.srt", Language: "简体中文", Flag: "china.gif"},
			{ID: "46", TorrentID: "123", Title: "Tom & Jerry 繁中.ass", Language: "繁體中文", Flag: "hongkong.gif"},
			{ID: "47", TorrentID: "120", Title: "subs-pack.zip", Language: "English", Flag: "uk.gif"},
		}, subs)
	})

	t.Run("1.5 的写法：语言表里没有时语言与国旗图为空", func(t *testing.T) {
		subs, err := d.ListSubtitles(context.Background(), " 88 ")
		require.NoError(t, err)
		assert.Equal(t, []Subtitle{
			{ID: "7", TorrentID: "88", Title: "某剧.S01E01.cht.srt", Language: "繁體中文", Flag: "hongkong.gif"},
			{ID: "8", TorrentID: "88", Title: "某剧.S01E02.简体.srt"},
		}, subs)
	})

	t.Run("暂无字幕", func(t *testing.T) {
		subs, err := d.ListSubtitles(context.Background(), "124")
		require.NoError(t, err)
		assert.Empty(t, subs)
	})
}

func TestNexusPHPListSubtitlesErrors(t *testing.T) {
	d, hits := subtitleSite(t, map[string]string{
		"9":  `<html><head><title>出错了</title></head><body><table><tr><td class="text">没有该ID的种子</td></tr></table></body></html>`,
		"10": `<html><head><title>登录</title></head><body><form method="post" action="takelogin.php"><input name="username"><input name="password" type="password"></form></body></html>`,
	}, nil)

	_, err := d.ListSubtitles(context.Background(), "9")
	require.ErrorIs(t, err, ErrTorrentNotFound)
	assert.Contains(t, err.Error(), "没有该ID的种子")

	_, err = d.ListSubtitles(context.Background(), "10")
	require.ErrorIs(t, err, ErrSessionExpired)

	// RSS 的 guid 是 info hash：不是数字的编号不发请求
	before := hits.Load()
	for _, id := range []string{"", "0123456789abcdef0123456789abcdef01234567", "12a", "-3"} {
		_, err = d.ListSubtitles(context.Background(), id)
		require.ErrorIs(t, err, ErrInvalidTorrentID, id)
	}
	assert.Equal(t, before, hits.Load())
}

func TestParseNexusSubtitlesIgnoresOtherLinks(t *testing.T) {
	// 同一份字幕的链接出现两次（例如站点另放了一个下载按钮）只算一次；参数不全、不是数字的链接不算
	page := `<html><body><table>
<tr><td><img src="pic/flag/japan.gif" alt="日本語" title="日本語"></td><td><a href="downloadsubs.php?torrentid=5&amp;subid=1">a.ja.srt</a></td></tr>
<tr><td><a href="/downloadsubs.php?torrentid=5&subid=1"><u>a.ja.srt（重复）</u></a></td></tr>
<tr><td><a href="downloadsubs.php?subid=2">缺种子编号</a></td></tr>
<tr><td><a href="downloadsubs.php?torrentid=5&subid=x">编号不对</a></td></tr>
<tr><td><a href="https://other.example/downloadsubs.php?torrentid=5&subid=3"><u>  b.srt  </u></a></td></tr>
</table></body></html>`
	d, _ := subtitleSite(t, map[string]string{"5": page}, nil)
	subs, err := d.ListSubtitles(context.Background(), "5")
	require.NoError(t, err)
	assert.Equal(t, []Subtitle{
		{ID: "1", TorrentID: "5", Title: "a.ja.srt", Language: "日本語", Flag: "japan.gif"},
		{ID: "3", TorrentID: "5", Title: "b.srt"},
	}, subs)
}

func TestNexusPHPDownloadSubtitle(t *testing.T) {
	cases := []struct {
		name        string
		disposition string
		body        string
		want        SubtitleFile
		wantErr     error
	}{
		{
			// Chrome 的 UA 带 Gecko：downloadsubs.php 回原样的文件名，后面多一段「 ; charset=utf-8」
			name: "原样的文件名", disposition: `attachment; filename="电影 2023.chs.srt" ; charset=utf-8`, body: "1\n00:00:01,000 --> 00:00:02,000\n你好\n",
			want: SubtitleFile{Name: "电影 2023.chs.srt", Data: []byte("1\n00:00:01,000 --> 00:00:02,000\n你好\n")},
		},
		{
			name: "rawurlencode 过的文件名", disposition: `attachment; filename=%E5%AD%97%E5%B9%95%20pack.zip`, body: "PK\x03\x04zip",
			want: SubtitleFile{Name: "字幕 pack.zip", Data: []byte("PK\x03\x04zip")},
		},
		{name: "没有文件名", body: "[Script Info]\n", want: SubtitleFile{Data: []byte("[Script Info]\n")}},
		{name: "文件名带目录", disposition: `attachment; filename="../../etc/a.srt"`, body: "x", want: SubtitleFile{Name: "a.srt", Data: []byte("x")}},
		{name: "找不到字幕", body: "Not found\n", wantErr: ErrSubtitleNotFound},
		{name: "找不到文件", body: "File not found\n", wantErr: ErrSubtitleNotFound},
		{name: "参数不全", body: "File name missing\n", wantErr: ErrSubtitleNotFound},
		{name: "空的", body: "", wantErr: ErrSubtitleNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := subtitleSite(t, nil, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "120", r.URL.Query().Get("torrentid"))
				assert.Equal(t, "47", r.URL.Query().Get("subid"))
				if tc.disposition != "" {
					w.Header().Set("Content-Disposition", tc.disposition)
					w.Header().Set("Content-Type", "application/octet-stream")
				}
				_, _ = w.Write([]byte(tc.body))
			})
			got, err := d.DownloadSubtitle(context.Background(), Subtitle{ID: "47", TorrentID: "120"})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				if body := strings.TrimSpace(tc.body); body != "" {
					assert.Contains(t, err.Error(), "站点回「"+body+"」", "写上站点回的话")
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}

	t.Run("编号不是数字", func(t *testing.T) {
		d, hits := subtitleSite(t, nil, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("x")) })
		_, err := d.DownloadSubtitle(context.Background(), Subtitle{ID: "a", TorrentID: "120"})
		require.ErrorIs(t, err, ErrInvalidTorrentID)
		_, err = d.DownloadSubtitle(context.Background(), Subtitle{ID: "47", TorrentID: ""})
		require.ErrorIs(t, err, ErrInvalidTorrentID)
		assert.Zero(t, hits.Load())
	})

	t.Run("登录失效", func(t *testing.T) {
		d, _ := subtitleSite(t, nil, func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`<html><head><title>Login</title></head><body><form action="takelogin.php"></form></body></html>`))
		})
		_, err := d.DownloadSubtitle(context.Background(), Subtitle{ID: "47", TorrentID: "120"})
		require.ErrorIs(t, err, ErrSessionExpired)
	})
}

func TestBaseSiteSubtitles(t *testing.T) {
	oldBase, oldMin := siteRetryBase, siteRetryMinAttempt
	siteRetryBase, siteRetryMinAttempt = time.Millisecond, 0
	t.Cleanup(func() { siteRetryBase, siteRetryMinAttempt = oldBase, oldMin })

	var details, downloads atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/details.php":
			// 第一次 502：一时的错误，按退避再取
			if details.Add(1) == 1 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte(readFixture(t, "nexusphp_details_subtitles.html")))
		case "/downloadsubs.php":
			if downloads.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Disposition", `attachment; filename="a.srt"`)
			_, _ = w.Write([]byte("sub"))
		}
	}))
	t.Cleanup(srv.Close)
	driver := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: srv.URL, Cookie: "c"})
	site := NewBaseSite(driver, BaseSiteConfig{ID: "demo", Name: "Demo", Kind: SiteNexusPHP, RateLimit: 1000, RateBurst: 10})

	var src SubtitleSource = site
	p := src.Subtitles()
	require.NotNil(t, p)
	subs, err := p.ListSubtitles(context.Background(), "123")
	require.NoError(t, err)
	assert.Len(t, subs, 3)
	assert.Equal(t, int32(2), details.Load())

	f, err := p.DownloadSubtitle(context.Background(), subs[2])
	require.NoError(t, err)
	assert.Equal(t, SubtitleFile{Name: "a.srt", Data: []byte("sub")}, f)
	assert.Equal(t, int32(2), downloads.Load())

	// 编号不对不是一时的错误：不发请求、不重试
	n0 := details.Load()
	_, err = p.ListSubtitles(context.Background(), "abc")
	require.ErrorIs(t, err, ErrInvalidTorrentID)
	assert.Equal(t, n0, details.Load())

	// 限速：令牌用完时等到 ctx 结束就不发请求
	slow := NewBaseSite(driver, BaseSiteConfig{ID: "slow", RateLimit: 0.001, RateBurst: 1})
	sp := slow.Subtitles()
	require.NotNil(t, sp)
	_, err = sp.ListSubtitles(context.Background(), "123")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	n := details.Load()
	_, err = sp.ListSubtitles(ctx, "123")
	require.Error(t, err)
	_, err = sp.DownloadSubtitle(ctx, subs[0])
	require.Error(t, err)
	assert.Equal(t, n, details.Load())
}

func TestBaseSiteSubtitlesUnsupported(t *testing.T) {
	// 驱动没有字幕时是 nil（M-Team 等走 API 的站点）
	site := NewBaseSite(NewMTorrentDriver(MTorrentDriverConfig{BaseURL: "https://api.example.test", APIKey: "k"}), BaseSiteConfig{ID: "mteam"})
	assert.Nil(t, site.Subtitles())
}
