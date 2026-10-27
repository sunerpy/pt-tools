package v2

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func detectDoc(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	require.NoError(t, err)
	return doc
}

// 登录以后的 HDSky 免费列表：脚本里有带「2fa」三个字母的随机串（哈希、令牌），不是二次验证页（2026-10-27 生产误判）。
const listWith2faInScripts = `<html><head><title>HDSky :: 种子 高清视界</title>
<script>var token = "e3b0c442a2fa98fc1c149afbf4c8996f";</script>
<script src="/js/a2fa.js"></script></head><body>
<div id="info_block"><a href="userdetails.php?id=1">me</a> <a href="logout.php">退出</a></div>
<script>var maxpage=19; var h="1d2fa3";</script>
<table class="torrents"><tbody><tr><td>头</td></tr>
<tr><td><img alt="Movies"></td><td><a href="details.php?id=5"><b>Some.Movie.2Fast.2026</b></a></td></tr>
</tbody></table></body></html>`

func TestIs2FAPage_OnlyRealMarkers(t *testing.T) {
	assert.False(t, is2FAPage(detectDoc(t, listWith2faInScripts)), "脚本里的 2fa 三个字母不算")
	assert.False(t, is2FAPage(detectDoc(t, `<html><head><title>2Fast 2Furious 1080p</title></head></html>`)), "标题里的 2Fast 不算")

	for name, html := range map[string]string{
		"跳到 take2fa":   `<html><head><title>x</title><script>location.href='take2fa.php?returnto=/'</script></head></html>`,
		"2FA 表单":       `<html><body><form action="take2fa.php" method="post"><input name="code"></form></body></html>`,
		"表单大写":         `<html><body><form action="/verify2FA" method="post"></form></body></html>`,
		"标题二次验证":       `<html><head><title>二次验证 - HDSky</title></head></html>`,
		"标题两步验证":       `<html><head><title>两步验证</title></head></html>`,
		"标题 2FA":       `<html><head><title>2FA Verification</title></head></html>`,
		"标题 Site::2fa": `<html><head><title>Site :: 2fa</title></head></html>`,
	} {
		assert.True(t, is2FAPage(detectDoc(t, html)), name)
	}
}

func detectServer(t *testing.T, body string) *NexusPHPDriver {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: srv.URL, Cookie: "c=1"})
}

// 登录以后的列表页照常解析，即使脚本里有 2fa 三个字母。
func TestNexusPHPExecute_ListWith2faInScriptsIsNotAuthFailure(t *testing.T) {
	d := detectServer(t, listWith2faInScripts)
	res, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/torrents.php"})
	require.NoError(t, err)
	items, err := d.ParseSearch(res)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "5", items[0].ID)
}

// 真的是登录页、二次验证页：错误照旧（errors.Is 认得），并写上返回页面的标题。
func TestNexusPHPExecute_AuthErrorsCarryPageTitle(t *testing.T) {
	d := detectServer(t, `<html><head><title>HDSky :: 登录</title></head><body>
<form action="takelogin.php" method="post"><input name="username"><input name="password"></form></body></html>`)
	_, err := d.Execute(context.Background(), NexusPHPRequest{Path: "/torrents.php"})
	require.ErrorIs(t, err, ErrSessionExpired)
	assert.Contains(t, err.Error(), "页面标题：HDSky :: 登录")

	d = detectServer(t, `<html><head><title>二次验证</title></head><body><form action="take2fa.php"></form></body></html>`)
	_, err = d.Execute(context.Background(), NexusPHPRequest{Path: "/torrents.php"})
	require.ErrorIs(t, err, Err2FARequired)
	assert.Contains(t, err.Error(), "页面标题：二次验证")

	// 没有标题时原样
	d = detectServer(t, `<html><body><form action="take2fa.php"></form></body></html>`)
	_, err = d.Execute(context.Background(), NexusPHPRequest{Path: "/torrents.php"})
	assert.Equal(t, Err2FARequired.Error(), err.Error())
}

// 一个种子都没有：登录以后的页面（有用户信息栏）就是列表空了；不像登录以后的页面就报站点返回的不是种子列表，带上标题与开头的文字。
func TestNexusPHPParseSearch_UnexpectedPage(t *testing.T) {
	d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: "https://example.test", Cookie: "c=1"})

	empty := detectDoc(t, `<html><head><title>HDSky :: 种子</title></head><body>
<div id="info_block"><a href="userdetails.php?id=1">me</a></div><h2>没有种子 :(</h2></body></html>`)
	items, err := d.ParseSearch(NexusPHPResponse{Document: empty})
	require.NoError(t, err)
	assert.Empty(t, items)

	blocked := detectDoc(t, `<html><head><title>站点维护中</title><style>body{color:red}</style></head>
<body><script>var x=1;</script><h1>维护中</h1><p>预计   一小时后恢复，请稍后再来。</p></body></html>`)
	_, err = d.ParseSearch(NexusPHPResponse{Document: blocked})
	require.ErrorIs(t, err, ErrUnexpectedPage)
	assert.Contains(t, err.Error(), "页面标题：站点维护中")
	assert.Contains(t, err.Error(), "开头：维护中 预计 一小时后恢复，请稍后再来。")
	assert.NotContains(t, err.Error(), "var x")
	assert.NotContains(t, err.Error(), "color:red")

	// 有列表的行（比如只剩没标题的行）时不报：列表结构还在
	rowsOnly := detectDoc(t, `<html><body><table class="torrents"><tbody><tr><td>头</td></tr><tr><td>空</td></tr></tbody></table></body></html>`)
	items, err = d.ParseSearch(NexusPHPResponse{Document: rowsOnly})
	require.NoError(t, err)
	assert.Empty(t, items)

	// 标题与正文都很长时截短
	long := detectDoc(t, "<html><head><title>"+strings.Repeat("长", 100)+"</title></head><body>"+strings.Repeat("字", 200)+"</body></html>")
	_, err = d.ParseSearch(NexusPHPResponse{Document: long})
	require.True(t, errors.Is(err, ErrUnexpectedPage))
	assert.Contains(t, err.Error(), strings.Repeat("长", pageTitleLimit)+"…")
	assert.NotContains(t, err.Error(), strings.Repeat("字", unexpectedPageTextLimit+1))
}
