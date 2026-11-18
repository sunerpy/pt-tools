package v2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// noticePage 是上游 NexusPHP 的「下载提示」页（downloadnotice.php）：表单里有隐藏的 id、type 与「下次不显示此提示」的勾选框，
// 分享率警告时勾选框不能取消、另有「我会提高分享率的」。表单与 2026-11-02 织梦的提示页一致。
func noticePage(torrentID, typ string) string {
	heading := map[string]string{"firsttime": "第一次下载提示", "client": "被禁止客户端提示", "ratio": "低分享率提示"}[typ]
	extra := ""
	if typ == "ratio" {
		extra = `<br /><input type="checkbox" name="letmedown" id="letmedown" value="ratio" /><label for="letmedown">我会提高分享率的。</label>`
	}
	return `<html><head><title>织梦 :: 下载提示 - Powered by NexusPHP</title></head><body><h2>` + heading + `</h2>
<form action="?" method="post"><p>了解更多信息，见<a class="faqlink" href="rules.php">规则</a><br />
<input type="hidden" name="id" value="` + torrentID + `" /><input type="hidden" name="type" value="` + typ + `" />
<input type="checkbox" name="hidenotice" id="hidenotice" value="1" checked="checked" /><label for="hidenotice">下次不显示此提示</label>` + extra + `</p>
<div><input type="submit" name="submit" id="continuedownload" value="下载种子文件" /></div></form></body></html>`
}

// noticeSite 是一个 Cookie 下载会先跳到「下载提示」页的站点（上游 download.php：没带 letdown=1 时按账号的设置跳过去）。
type noticeSite struct {
	notice   string // firsttime / client / ratio；空的时直接给种子
	torrent  []byte
	plain    atomic.Int32 // 不带 letdown 的下载次数
	letdown  atomic.Int32 // 带 letdown=1 的下载次数
	posts    atomic.Int32 // 往提示页提交表单（会改掉账号的「下次不显示」设置）的次数
	cookie   atomic.Value // 带 letdown 那次请求的 Cookie
	notices  atomic.Int32 // 打开提示页的次数
	firstURL atomic.Value // 第一次下载请求的查询参数
}

func (s *noticeSite) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/download.php":
			if r.URL.Query().Get("letdown") == "1" {
				s.letdown.Add(1)
				s.cookie.Store(r.Header.Get("Cookie"))
				_, _ = w.Write(s.torrent)
				return
			}
			if s.plain.Add(1) == 1 {
				s.firstURL.Store(r.URL.RawQuery)
			}
			if s.notice == "" {
				_, _ = w.Write(s.torrent)
				return
			}
			http.Redirect(w, r, "/downloadnotice.php?torrentid="+r.URL.Query().Get("id")+"&type="+s.notice, http.StatusFound)
		case "/downloadnotice.php":
			if r.Method == http.MethodPost {
				s.posts.Add(1)
				t.Error("不该替用户在提示页上提交表单（会改掉账号的「下次不显示此提示」设置）")
				return
			}
			s.notices.Add(1)
			_, _ = w.Write([]byte(noticePage(r.URL.Query().Get("torrentid"), r.URL.Query().Get("type"))))
		default:
			http.NotFound(w, r)
		}
	})
}

func detailDoc(t *testing.T, href string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<html><body><a href="` + href + `">dl</a></body></html>`))
	require.NoError(t, err)
	return doc
}

// 账号开着「第一次下载提示」（织梦，2026-11-02 辅种全部「info hash 不符」）：Cookie 下载先跳到提示页、拿到的是网页。
// 照提示页上「下载种子文件」按钮的做法带上 letdown=1 再下一次，拿到种子；不往提示页提交表单，不改账号的设置。
func TestNexusPHPDriver_Download_SkipsFirstTimeNotice(t *testing.T) {
	s := &noticeSite{notice: "firsttime", torrent: []byte("d8:announce4:infod4:name1:aee")}
	server := httptest.NewServer(s.handler(t))
	defer server.Close()

	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c_secure_uid=1"})
	data, err := d.ParseDownloadContext(context.Background(), NexusPHPResponse{Document: detailDoc(t, "download.php?id=21122")})
	require.NoError(t, err)
	assert.Equal(t, s.torrent, data)
	assert.Equal(t, int32(1), s.plain.Load())
	assert.Equal(t, int32(1), s.notices.Load(), "第一次下到的是提示页")
	assert.Equal(t, int32(1), s.letdown.Load())
	assert.Equal(t, "c_secure_uid=1", s.cookie.Load(), "带 letdown 那次照样带 Cookie")
	assert.Zero(t, s.posts.Load())

	// 账号已经关了提示：一次就拿到，不多发请求
	s2 := &noticeSite{torrent: s.torrent}
	server2 := httptest.NewServer(s2.handler(t))
	defer server2.Close()
	d = NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server2.URL, Cookie: "c_secure_uid=1"})
	data, err = d.ParseDownloadContext(context.Background(), NexusPHPResponse{Document: detailDoc(t, "download.php?id=7&passkey=abc")})
	require.NoError(t, err)
	assert.Equal(t, s.torrent, data)
	assert.Equal(t, int32(1), s2.plain.Load())
	assert.Zero(t, s2.letdown.Load())
	assert.Equal(t, "id=7&passkey=abc", s2.firstURL.Load(), "下载地址原样请求")
}

// 被禁止客户端、低分享率的提示是账号的状态，要用户自己处理（分享率警告时站点要每次下载都在网页上确认）：
// 不替用户跳过，报清楚是哪种提示；仍算「不是种子文件」。
func TestNexusPHPDriver_Download_AccountNoticesNotSkipped(t *testing.T) {
	for typ, want := range map[string]string{"client": "被禁止客户端提示", "ratio": "低分享率提示"} {
		t.Run(typ, func(t *testing.T) {
			s := &noticeSite{notice: typ, torrent: []byte("d4:infod4:name1:aee")}
			server := httptest.NewServer(s.handler(t))
			defer server.Close()

			d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
			_, err := d.ParseDownloadContext(context.Background(), NexusPHPResponse{Document: detailDoc(t, "download.php?id=5")})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrNotTorrentFile)
			assert.Contains(t, err.Error(), want)
			assert.Zero(t, s.letdown.Load(), "不带 letdown=1 跳过")
			assert.Zero(t, s.posts.Load())
			assert.Equal(t, int32(1), s.plain.Load(), "不是一时的错误，不重试")
		})
	}
}

// 站点回的不是种子文件：报出是哪一页（登录页、两步验证页带上对应的错误），不再当成种子交给调用方（辅种以前因此报「info hash 不符」）。
// 错误里的地址脱敏。
func TestNexusPHPDriver_Download_NotTorrentResponses(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		is     error
		want   string
		reject string
	}{
		{
			"login", `<html><head><title>UHDDolby :: 登录 - Powered by NexusPHP</title></head><body><form method="post" action="takelogin.php"><input name="username"><input name="password"></form></body></html>`,
			ErrSessionExpired, "登录页", "",
		},
		{
			"2fa", `<html><head><title>站点 :: 两步验证</title></head><body><script>window.location.href = "take2fa.php?returnto=%2Fdownload.php";</script></body></html>`,
			Err2FARequired, "两步验证", "",
		},
		{"page", `<html><head><title>织梦 :: 出错了 - Powered by NexusPHP</title></head><body>系统繁忙</body></html>`, nil, "页面标题：织梦 :: 出错了", ""},
		{"untitled", `<!DOCTYPE html><html><body><p>维护中</p></body></html>`, nil, "字节的网页", ""},
		{"json", `{"code":403,"msg":"下载次数用完了"}`, nil, "下载次数用完了", ""},
		{"text", "invalid passkey, see https://pt.example/download.php?id=1&passkey=SECRETPASSKEY", nil, "invalid passkey", "SECRETPASSKEY"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: server.URL, Cookie: "c=1"})
			_, err := d.ParseDownloadContext(context.Background(), NexusPHPResponse{Document: detailDoc(t, "download.php?id=1")})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrNotTorrentFile)
			if tc.is != nil {
				assert.ErrorIs(t, err, tc.is)
			}
			assert.Contains(t, err.Error(), "站点返回的不是种子文件")
			assert.Contains(t, err.Error(), tc.want)
			if tc.reject != "" {
				assert.NotContains(t, err.Error(), tc.reject)
			}
		})
	}
}

func TestCheckTorrentFile(t *testing.T) {
	require.NoError(t, CheckTorrentFile([]byte("d8:announce4:infod4:name1:aee")))
	err := CheckTorrentFile([]byte(noticePage("9", "firsttime")))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotTorrentFile)
	assert.Contains(t, err.Error(), "第一次下载提示")
	var notice *downloadNoticeError
	require.True(t, errors.As(err, &notice))
	assert.Equal(t, noticeFirstTime, notice.noticeType)

	err = CheckTorrentFile(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNotTorrentFile)

	// 只有勾选框、没有提示类型的表单不算提示页
	err = CheckTorrentFile([]byte(`<html><head><title>设置</title></head><body><form><input type="checkbox" name="hidenotice"></form></body></html>`))
	require.Error(t, err)
	assert.False(t, errors.As(err, &notice))
	assert.Contains(t, err.Error(), "页面标题：设置")
}

func TestWithLetDown(t *testing.T) {
	for in, want := range map[string]string{
		"https://zmpt.cc/download.php?id=21122":            "https://zmpt.cc/download.php?id=21122&letdown=1",
		"https://pt.example/download.php?id=5&passkey=a+b": "https://pt.example/download.php?id=5&passkey=a+b&letdown=1",
		"https://pt.example/Download.php":                  "https://pt.example/Download.php?letdown=1",
	} {
		got, ok := withLetDown(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got)
	}
	for _, in := range []string{
		"https://pt.example/download.php?id=5&letdown=1", // 已经带了，再下一次也一样
		"https://pt.example/dl/5",
		"://bad",
	} {
		_, ok := withLetDown(in)
		assert.False(t, ok, in)
	}
}

// 带上 letdown=1 以后还是提示页（站点改了做法）：不再试，报第一次下载提示，告诉用户在网页上关掉它。
func TestFetchSkippingFirstNotice_GivesUpAfterOneRetry(t *testing.T) {
	var calls []string
	_, err := fetchSkippingFirstNotice("https://pt.example/download.php?id=1", func(u string) ([]byte, error) {
		calls = append(calls, u)
		return nil, &downloadNoticeError{noticeType: noticeFirstTime}
	})
	require.Error(t, err)
	assert.Equal(t, []string{"https://pt.example/download.php?id=1", "https://pt.example/download.php?id=1&letdown=1"}, calls)
	assert.Contains(t, err.Error(), "下次不显示此提示")

	// 别的错误原样返回，不重下
	calls = nil
	_, err = fetchSkippingFirstNotice("https://pt.example/download.php?id=1", func(u string) ([]byte, error) {
		calls = append(calls, u)
		return nil, fmt.Errorf("boom: %w", ErrNetworkError)
	})
	require.ErrorIs(t, err, ErrNetworkError)
	assert.Len(t, calls, 1)
}
