package qbitcompat

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/apitoken"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// torrentFile 造一个种子文件：announce、comment 与单文件的 info（bencode 的键按字典序）。
func torrentFile(name, announce, comment string) []byte {
	info := fmt.Sprintf("d6:lengthi1024e4:name%d:%s12:piece lengthi16384e6:pieces20:%se", len(name), name, strings.Repeat("x", 20))
	return []byte(fmt.Sprintf("d8:announce%d:%s7:comment%d:%s4:info%se", len(announce), announce, len(comment), comment, info))
}

func hashOf(t *testing.T, data []byte) string {
	t.Helper()
	p, err := v2.ParseTorrent(data)
	require.NoError(t, err)
	return strings.ToLower(p.InfoHash)
}

// fakeSite 是已启用的站点：Download 记下要的编号，回一个种子文件。
type fakeSite struct {
	v2.Site
	got []string
}

func (f *fakeSite) Download(_ context.Context, id string) ([]byte, error) {
	f.got = append(f.got, id)
	return torrentFile("site-"+id, "https://tracker.qa.example/announce.php?passkey=pk", ""), nil
}

// withSites 给兼容入口接上站点定义（qasite 已启用，offsite 没启用）与站点实例。
func (e *env) withSites() *fakeSite {
	site := &fakeSite{}
	e.srv.deps.Resolver = v2.NewTrackerResolverFrom(
		&v2.SiteDefinition{ID: "qasite", Schema: v2.SchemaNexusPHP, URLs: []string{"https://qa.example/"}},
		&v2.SiteDefinition{ID: "offsite", Schema: v2.SchemaNexusPHP, URLs: []string{"https://off.example/"}},
	)
	e.srv.deps.Site = func(id string) v2.Site {
		if id == "qasite" {
			return site
		}
		return nil
	}
	return site
}

// postAdd 发一个 multipart 的 torrents/add。
func (e *env) postAdd(ck *http.Cookie, files [][]byte, fields map[string]string) *httptest.ResponseRecorder {
	e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for i, f := range files {
		fw, err := mw.CreateFormFile("torrents", fmt.Sprintf("t%d.torrent", i))
		require.NoError(e.t, err)
		_, _ = fw.Write(f)
	}
	for k, v := range fields {
		require.NoError(e.t, mw.WriteField(k, v))
	}
	require.NoError(e.t, mw.Close())
	req := httptest.NewRequest(http.MethodPost, "/api/v2/torrents/add", &buf)
	req.RemoteAddr = "192.0.2.10:5000"
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(ck)
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, req)
	return w
}

func (e *env) gotPushes() []internal.PushTorrentRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]internal.PushTorrentRequest(nil), e.pushes...)
}

// 上传种子文件：按 tracker 认站点、从 comment 的详情页地址取编号、追加站点标签；选项照 qB 的字段传下去；没给目录时用分类记下的目录
func TestAddTorrentFile(t *testing.T) {
	e := newEnv(t)
	e.withSites()
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	require.NoError(t, e.db.Create(&models.QbitCompatSetting{ID: 1, Categories: `{"radarr":"/movies/radarr"}`}).Error)

	known := torrentFile("Dune.2024.2160p", "https://qa.example/announce.php?passkey=pk", "https://qa.example/details.php?id=42")
	unknown := torrentFile("other", "https://tracker.unknown.example/announce", "")
	w := e.postAdd(ck, [][]byte{known, unknown}, map[string]string{
		"category": "radarr", "tags": "mp, 4k", "paused": "true", "rename": "沙丘2", "upLimit": "1048576", "dlLimit": "100",
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "Ok.", w.Body.String())
	p := e.gotPushes()
	require.Len(t, p, 2)
	assert.Equal(t, "qasite", p[0].SiteID)
	assert.Equal(t, "42", p[0].TorrentID, "编号取自 comment 里的详情页地址")
	assert.Equal(t, "mp,4k,qasite", p[0].Tags, "追加站点标签")
	assert.Equal(t, "radarr", p[0].Category)
	assert.Equal(t, "/movies/radarr", p[0].SavePath, "没给目录：用分类记下的")
	assert.True(t, p[0].AddPaused)
	assert.Equal(t, "沙丘2", p[0].Rename)
	assert.Equal(t, "沙丘2", p[0].Title)
	assert.Equal(t, 1024, p[0].UpLimitKBs)
	assert.Equal(t, 1, p[0].DlLimitKBs, "不足 1 KB/s 的按 1 KB/s")
	assert.Equal(t, Source, p[0].Source)
	assert.Equal(t, e.dlSet.ID, p[0].DownloaderID)

	assert.Empty(t, p[1].SiteID, "认不出站点")
	assert.Equal(t, "hash:"+hashOf(t, unknown), p[1].TorrentID)
	assert.Equal(t, "mp,4k", p[1].Tags)

	a := e.audit.all()
	require.Len(t, a, 1)
	assert.Equal(t, "POST /api/v2/torrents/add", a[0].Command)
	assert.Equal(t, "success", a[0].Result)
	assert.EqualValues(t, 2, a[0].Args["added"])
}

// 链接：只认已启用站点的种子下载地址，由 pt-tools 经站点下载（编号来自地址）；磁力链接、认不出的地址、没启用的站点都拒绝
func TestAddURLs(t *testing.T) {
	e := newEnv(t)
	site := e.withSites()
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	form := url.Values{"urls": {strings.Join([]string{
		"https://qa.example/download.php?id=77&passkey=secret",
		"magnet:?xt=urn:btih:" + hashDebian,
		"https://evil.example/x.torrent",
		"https://off.example/download.php?id=1",
	}, "\n")}, "category": {"tv"}}
	w := e.do(http.MethodPost, "/api/v2/torrents/add", form, ck)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Ok.", w.Body.String(), "有一个加进去就是 Ok.")
	assert.Equal(t, []string{"77"}, site.got, "只经站点下载认得出的那条，不请求客户端给的地址")
	p := e.gotPushes()
	require.Len(t, p, 1)
	assert.Equal(t, "qasite", p[0].SiteID)
	assert.Equal(t, "77", p[0].TorrentID)
	assert.Equal(t, "qasite", p[0].Tags)
	a := e.audit.all()
	require.Len(t, a, 1)
	assert.Equal(t, "error:partial", a[0].Result)
	assert.EqualValues(t, 3, a[0].Args["failed"])
	assert.NotContains(t, fmt.Sprint(a[0].Args), "secret")

	w = e.do(http.MethodPost, "/api/v2/torrents/add", url.Values{"urls": {"magnet:?xt=urn:btih:" + hashDebian}}, ck)
	assert.Equal(t, "Fails.", w.Body.String())
	assert.Equal(t, "denied:magnet", e.audit.all()[1].Result)
	w = e.do(http.MethodPost, "/api/v2/torrents/add", url.Values{"urls": {"https://evil.example/x.torrent"}}, ck)
	assert.Equal(t, "Fails.", w.Body.String())
	assert.Equal(t, "denied:url", e.audit.all()[2].Result)
	assert.Equal(t, []string{"77"}, site.got)
}

// 只有不对的种子文件：415；推送被拦下（磁盘空间、站点做种容量）：Fails. 并记审计；已经在下载器里：Ok.；什么都没给：Fails.
func TestAddFailures(t *testing.T) {
	e := newEnv(t)
	e.withSites()
	ck := e.login(e.token(apitoken.ScopeQbitCompat))
	w := e.postAdd(ck, [][]byte{[]byte("not a torrent")}, nil)
	assert.Equal(t, http.StatusUnsupportedMediaType, w.Code)
	assert.Equal(t, "Torrent file is not valid", w.Body.String())

	e.srv.deps.Push = func(context.Context, internal.PushTorrentRequest) (*internal.PushTorrentResult, error) {
		return &internal.PushTorrentResult{Success: false, Message: "磁盘空间不足"}, nil
	}
	w = e.postAdd(ck, [][]byte{torrentFile("a", "https://qa.example/announce", "")}, nil)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "Fails.", w.Body.String())
	last := e.audit.all()[len(e.audit.all())-1]
	assert.Equal(t, "error:add_failed", last.Result)
	assert.Contains(t, fmt.Sprint(last.Args["reasons"]), "磁盘空间不足")

	e.srv.deps.Push = func(context.Context, internal.PushTorrentRequest) (*internal.PushTorrentResult, error) {
		return &internal.PushTorrentResult{Success: true, Skipped: true}, nil
	}
	assert.Equal(t, "Ok.", e.postAdd(ck, [][]byte{torrentFile("a", "https://qa.example/announce", "")}, nil).Body.String())
	assert.Equal(t, "Fails.", e.postAdd(ck, nil, map[string]string{"category": "x"}).Body.String())

	big := e.postAdd(ck, [][]byte{bytes.Repeat([]byte("x"), maxAddBody+1)}, nil)
	assert.Equal(t, http.StatusBadRequest, big.Code, "超过 32 MiB")
}
