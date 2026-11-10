package v2

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 新版 NexusPHP 签到页的形状：照 xiaomlove/nexusphp public/attendance.php（2025-10-14 起）还没签时的输出——
// 页头 [签到得魔力]，一个提交回 attendance.php 的表单，只有「立即签到」按钮；打开了图片验证码时多出 show_image_code() 的两行。
const (
	attendanceFormPage = `<html><head><title>站点 :: 签到 - Powered by NexusPHP</title></head><body>
<table id="info_block"><tr><td>魔力值: 1,024.5 <a href="attendance.php">[签到得魔力]</a></td></tr></table>
<h2>签到</h2><table width="100%" border="1" cellspacing="0" cellpadding="10"><tbody><tr><td class="text">
<div style="margin-top: 20px; text-align: center;"><form method="post" action="attendance.php" style="display: inline-block;">
<table border="0" cellpadding="5"><tr><td class="toolbox" colspan="2" align="center"><input type="submit" value="立即签到" class="btn" /></td></tr></table>
</form></div></td></tr></tbody></table></body></html>`
	attendanceFormCaptchaPage = `<html><head><title>站点 :: 签到 - Powered by NexusPHP</title></head><body>
<table id="info_block"><tr><td><a href="attendance.php">[签到得魔力]</a></td></tr></table>
<h2>签到</h2><form method="post" action="attendance.php"><table border="0" cellpadding="5">
<tr><td class="rowhead">验证图片</td><td align="left"><img src="image.php?action=regimage&amp;imagehash=0a1b2c3d&amp;secret=" border="0" alt="CAPTCHA" /></td></tr>
<tr><td class="rowhead">验证码</td><td align="left"><input type="text" autocomplete="off" name="imagestring" value="" /><input type="hidden" name="imagehash" value="0a1b2c3d" /></td></tr>
<tr><td class="toolbox" colspan="2" align="center"><input type="submit" value="立即签到" class="btn" /></td></tr></table></form></body></html>`
)

type attendanceRequestSeen struct {
	method, path, contentType, body, referer, cookie string
}

// fakeFormAttendanceSite 起一个新版 NexusPHP 的假站点：GET /attendance.php 回 getPage，POST 回 postPage，按顺序记下每次请求。
func fakeFormAttendanceSite(t *testing.T, getPage, postPage string) (*httptest.Server, func() []attendanceRequestSeen) {
	t.Helper()
	var mu sync.Mutex
	var seen []attendanceRequestSeen
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/attendance.php" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, attendanceRequestSeen{r.Method, r.URL.Path, r.Header.Get("Content-Type"), string(body), r.Header.Get("Referer"), r.Header.Get("Cookie")})
		mu.Unlock()
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(postPage))
			return
		}
		_, _ = w.Write([]byte(getPage))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []attendanceRequestSeen {
		mu.Lock()
		defer mu.Unlock()
		return append([]attendanceRequestSeen(nil), seen...)
	}
}

// 新版 NexusPHP 打开签到页只显示「立即签到」按钮：照按钮的做法 POST 回 attendance.php，按提交后的页面认结果。
func TestNexusPHPDriver_AttendSubmitsTheCheckInForm(t *testing.T) {
	def := &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP}

	t.Run("signed after submitting", func(t *testing.T) {
		srv, seen := fakeFormAttendanceSite(t, attendanceFormPage, attendanceSuccessPage)
		res, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.True(t, strings.HasPrefix(res.Message, "这是您的第 128 次签到"), "got %q", res.Message)

		reqs := seen()
		require.Len(t, reqs, 2)
		assert.Equal(t, http.MethodGet, reqs[0].method)
		post := reqs[1]
		assert.Equal(t, http.MethodPost, post.method)
		assert.Equal(t, "application/x-www-form-urlencoded", post.contentType)
		assert.Empty(t, post.body, "the upstream form has no fields besides its unnamed submit button")
		assert.Equal(t, srv.URL+"/attendance.php", post.referer, "posted from the check-in page, like the button")
		assert.Equal(t, "uid=1; pass=2", post.cookie)
	})
	t.Run("already signed today", func(t *testing.T) {
		srv, seen := fakeFormAttendanceSite(t, attendanceFormPage, attendanceAlreadyPage)
		res, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
		require.NoError(t, err)
		assert.Equal(t, AttendAlready, res.Status)
		assert.Contains(t, res.Message, "您今天已经签到过了")
		assert.Len(t, seen(), 2)
	})
	t.Run("captcha is not submitted", func(t *testing.T) {
		srv, seen := fakeFormAttendanceSite(t, attendanceFormCaptchaPage, attendanceSuccessPage)
		_, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
		require.ErrorIs(t, err, ErrAttendanceCaptchaRequired)
		assert.ErrorContains(t, err, "签到页要输入验证码")
		assert.Len(t, seen(), 1, "no POST without the captcha answer")
	})
	t.Run("unrecognized after submitting names the page", func(t *testing.T) {
		srv, seen := fakeFormAttendanceSite(t, attendanceFormPage, `<html><head><title>站点 :: 出错了</title></head><body><h2>出错了</h2></body></html>`)
		_, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
		require.ErrorIs(t, err, ErrAttendanceUnrecognized)
		assert.ErrorContains(t, err, "提交签到表单后页面里没有签到成功或今日已签到的文字（页面标题「站点 :: 出错了」）")
		assert.Len(t, seen(), 2, "submitted once, not again")
	})
	t.Run("hidden fields and a named button are posted", func(t *testing.T) {
		page := `<html><body><form method="POST" action="/attendance.php"><input type="hidden" name="token" value="t-1"/>` +
			`<input type="submit" name="act" value="签到"/></form></body></html>`
		srv, seen := fakeFormAttendanceSite(t, page, attendanceSuccessPage)
		res, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		reqs := seen()
		require.Len(t, reqs, 2)
		form, err := url.ParseQuery(reqs[1].body)
		require.NoError(t, err)
		assert.Equal(t, url.Values{"token": {"t-1"}, "act": {"签到"}}, form)
	})
	// 旧版打开就签到的站点照旧只请求一次
	t.Run("old plugin signs on GET", func(t *testing.T) {
		srv, seen := fakeFormAttendanceSite(t, attendanceSuccessPage, attendanceAlreadyPage)
		res, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Len(t, seen(), 1)
	})
}

// 只跟随提交回签到页本身、不用另外填东西的表单；别的表单不提交，按认不出处理并写明页面标题。
func TestNexusPHPDriver_AttendLeavesOtherFormsAlone(t *testing.T) {
	def := &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP}
	pages := map[string]string{
		"posts elsewhere":   `<html><head><title>站点 :: 签到</title></head><body><form method="post" action="ajax.php"><input type="submit" value="立即签到"/></form></body></html>`,
		"GET form":          `<html><head><title>站点 :: 签到</title></head><body><form method="get" action="attendance.php"><input type="submit" value="立即签到"/></form></body></html>`,
		"needs an answer":   `<html><head><title>站点 :: 签到</title></head><body><form method="post" action="attendance.php"><input type="radio" name="choice" value="1"/><input type="submit" value="提交"/></form></body></html>`,
		"other host":        `<html><head><title>站点 :: 签到</title></head><body><form method="post" action="https://elsewhere.example/attendance.php"><input type="submit" value="立即签到"/></form></body></html>`,
		"different query":   `<html><head><title>站点 :: 签到</title></head><body><form method="post" action="attendance.php?do=other"><input type="submit" value="立即签到"/></form></body></html>`,
		"no submit button":  `<html><head><title>站点 :: 签到</title></head><body><form method="post" action="attendance.php"><input type="hidden" name="x" value="1"/></form></body></html>`,
		"has a select list": `<html><head><title>站点 :: 签到</title></head><body><form method="post" action="attendance.php"><select name="mood"><option>1</option></select><input type="submit" value="立即签到"/></form></body></html>`,
	}
	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			srv, seen := fakeFormAttendanceSite(t, page, attendanceSuccessPage)
			_, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
			require.ErrorIs(t, err, ErrAttendanceUnrecognized)
			assert.ErrorContains(t, err, "页面里没有签到成功或今日已签到的文字（页面标题「站点 :: 签到」）")
			assert.Len(t, seen(), 1, "nothing was submitted")
		})
	}

	// 配置成 POST 签到的站点本来就提交表单：页面里有表单也不再提交第二次
	post := &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Method: http.MethodPost}}
	srv, seen := fakeFormAttendanceSite(t, attendanceFormPage, attendanceFormPage)
	_, err := newAttendanceDriver(srv.URL, post).Attend(context.Background())
	require.ErrorIs(t, err, ErrAttendanceUnrecognized)
	assert.Len(t, seen(), 1)
}

// 认不出时写明页面标题；没有标题时照旧那句，标题太长截到 60 字。
func TestUnrecognizedAttendance(t *testing.T) {
	doc := func(html string) *NexusPHPResponse {
		res := &NexusPHPResponse{RawBody: []byte(html)}
		d, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		require.NoError(t, err)
		res.Document = d
		return res
	}
	assert.EqualError(t, unrecognizedAttendance(doc(`<html><body>x</body></html>`), ""),
		"attendance result not recognized: 页面里没有签到成功或今日已签到的文字")
	assert.EqualError(t, unrecognizedAttendance(doc("<html><head><title>\n Just a moment... \n</title></head></html>"), ""),
		"attendance result not recognized: 页面里没有签到成功或今日已签到的文字（页面标题「Just a moment...」）")
	long := strings.Repeat("标", 70)
	assert.ErrorContains(t, unrecognizedAttendance(doc("<title>"+long+"</title>"), ""), "「"+strings.Repeat("标", 60)+"…」")
	assert.EqualError(t, unrecognizedAttendance(nil, "提交签到表单后"),
		"attendance result not recognized: 提交签到表单后页面里没有签到成功或今日已签到的文字")
}

func TestPostsToPage(t *testing.T) {
	for action, want := range map[string]bool{
		"":                                     true,
		"attendance.php":                       true,
		"/attendance.php":                      true,
		"./attendance.php":                     true,
		" attendance.php ":                     true,
		"attendance.php?x=1":                   false,
		"ajax.php":                             false,
		"/plugin/attendance.php":               false,
		"https://other.example/attendance.php": false,
		"//other.example/attendance.php":       false,
		"%zz":                                  false,
	} {
		assert.Equal(t, want, postsToPage(action, "/attendance.php"), "action %q", action)
	}
}
