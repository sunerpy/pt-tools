package v2

import (
	"context"
	"fmt"
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
	t.Run("image captcha without AI needs AI", func(t *testing.T) {
		srv, seen := fakeFormAttendanceSite(t, attendanceFormCaptchaPage, attendanceSuccessPage)
		_, err := newAttendanceDriver(srv.URL, def).Attend(context.Background())
		require.ErrorIs(t, err, ErrAttendanceNeedsAI)
		assert.ErrorContains(t, err, "签到页要输入验证码")
		assert.Len(t, seen(), 1, "no captcha fetched and no POST without an AI to read it")
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

// fakeCaptchaAttendance 模拟打开了图片验证码的新版 NexusPHP 签到（照 ImageCaptchaDriver）：每次打开签到页发一个新的
// imagehash（hash1、hash2…），答案是 CODE0<序号>；提交时核对，核对完这个 imagehash 就作废，答错回 stderr 的「图片代码无效」。
type fakeCaptchaAttendance struct {
	t             *testing.T
	mu            sync.Mutex
	issued        int
	valid         map[string]bool
	pages         int
	images        []string
	imageReferers []string
	posts         []url.Values
	// reply 不为空时提交后一律回它（不核对验证码）
	reply string
}

const attendanceInvalidCaptchaPage = `<html><head><title>站点 :: 出错了</title></head><body><h2>Error</h2><table><tr><td class="text">` +
	`图片代码无效！<br /><b>不要返回，图片代码已被清除！</b><br /><br />点击<a href="attendance.php"><b>这里</b></a>获取新的图片代码。</td></tr></table></body></html>`

func (f *fakeCaptchaAttendance) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.URL.Path == "/attendance.php" && r.Method == http.MethodGet:
		f.pages++
		f.issued++
		hash := fmt.Sprintf("hash%d", f.issued)
		f.valid[hash] = true
		_, _ = w.Write([]byte(strings.ReplaceAll(attendanceFormCaptchaPage, "0a1b2c3d", hash)))
	case r.URL.Path == "/image.php":
		assert.Equal(f.t, "regimage", r.URL.Query().Get("action"))
		f.images = append(f.images, r.URL.Query().Get("imagehash"))
		f.imageReferers = append(f.imageReferers, r.Header.Get("Referer"))
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(testCaptchaPNG(f.t))
	case r.URL.Path == "/attendance.php" && r.Method == http.MethodPost:
		require.NoError(f.t, r.ParseForm())
		f.posts = append(f.posts, r.PostForm)
		if f.reply != "" {
			_, _ = w.Write([]byte(f.reply))
			return
		}
		hash, code := r.PostForm.Get("imagehash"), r.PostForm.Get("imagestring")
		ok := f.valid[hash] && code == "CODE0"+strings.TrimPrefix(hash, "hash")
		delete(f.valid, hash)
		if ok {
			_, _ = w.Write([]byte(attendanceSuccessPage))
			return
		}
		_, _ = w.Write([]byte(attendanceInvalidCaptchaPage))
	default:
		http.NotFound(w, r)
	}
}

func newFakeCaptchaAttendance(t *testing.T) (*fakeCaptchaAttendance, *httptest.Server) {
	f := &fakeCaptchaAttendance{t: t, valid: map[string]bool{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

// 签到页的表单带 NexusPHP 图片验证码：取图交给 AI 认，填进 imagestring 连同 imagehash 提交；认错了重新打开签到页换一张。
func TestNexusPHPDriver_AttendFillsTheImageCaptchaWithAI(t *testing.T) {
	def := &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP}
	attend := func(srvURL string, solver *fakeSolver) (AttendResult, error) {
		return newAttendanceDriver(srvURL, def).Attend(WithAttendanceSolver(context.Background(), solver))
	}

	t.Run("read right the first time", func(t *testing.T) {
		site, srv := newFakeCaptchaAttendance(t)
		solver := &fakeSolver{captchas: []string{"code01"}}
		res, err := attend(srv.URL, solver)
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.True(t, strings.HasPrefix(res.Message, "这是您的第 128 次签到"), "got %q", res.Message)

		require.Len(t, solver.images, 1)
		assert.Equal(t, nexusPHPCaptchaLength, solver.images[0].Length)
		assert.True(t, solver.images[0].UpperOnly, "NexusPHP 的验证码只有大写字母和数字")
		assert.Equal(t, "image/png", solver.images[0].MIME)
		assert.Equal(t, []string{"hash1"}, site.images, "the image named in the form")
		assert.Equal(t, srv.URL+"/attendance.php", site.imageReferers[0], "fetched from the check-in page, like the browser")
		require.Len(t, site.posts, 1)
		assert.Equal(t, url.Values{"imagehash": {"hash1"}, "imagestring": {"CODE01"}}, site.posts[0], "lower case read as upper case")
		assert.Equal(t, 1, site.pages)
	})
	t.Run("a wrong answer gets a new captcha", func(t *testing.T) {
		site, srv := newFakeCaptchaAttendance(t)
		solver := &fakeSolver{captchas: []string{"WRONG1", "CODE02"}}
		res, err := attend(srv.URL, solver)
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Equal(t, 2, site.pages, "the used imagehash is gone, so the page is opened again")
		assert.Equal(t, []string{"hash1", "hash2"}, site.images)
		require.Len(t, site.posts, 2)
		assert.Equal(t, "hash2", site.posts[1].Get("imagehash"))
		assert.Equal(t, "CODE02", site.posts[1].Get("imagestring"))
	})
	t.Run("an answer of the wrong length is not submitted", func(t *testing.T) {
		site, srv := newFakeCaptchaAttendance(t)
		solver := &fakeSolver{captchas: []string{"AB-C", "CODE02"}}
		res, err := attend(srv.URL, solver)
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		require.Len(t, site.posts, 1, "a 3-character answer is not posted")
		assert.Equal(t, "hash2", site.posts[0].Get("imagehash"))
	})
	t.Run("gives up after three wrong answers", func(t *testing.T) {
		site, srv := newFakeCaptchaAttendance(t)
		solver := &fakeSolver{captchas: []string{"WRONG1", "WRONG2", "WRONG3", "CODE04"}}
		_, err := attend(srv.URL, solver)
		require.ErrorIs(t, err, ErrAttendanceChallengeFailed)
		assert.ErrorContains(t, err, "试了 3 次，验证码没认对")
		assert.Len(t, site.posts, captchaTries)
		assert.Len(t, solver.captchas, 1, "no fourth read")
	})
	t.Run("an unknown reply after submitting is not retried", func(t *testing.T) {
		site, srv := newFakeCaptchaAttendance(t)
		site.reply = `<html><head><title>站点 :: 维护中</title></head><body>站点维护中</body></html>`
		solver := &fakeSolver{captchas: []string{"CODE01", "CODE02"}}
		_, err := attend(srv.URL, solver)
		require.ErrorIs(t, err, ErrAttendanceUnrecognized)
		assert.ErrorContains(t, err, "提交签到表单后页面里没有签到成功或今日已签到的文字（页面标题「站点 :: 维护中」）")
		assert.Len(t, site.posts, 1)
	})
	t.Run("already signed after submitting", func(t *testing.T) {
		site, srv := newFakeCaptchaAttendance(t)
		site.reply = attendanceAlreadyPage
		res, err := attend(srv.URL, &fakeSolver{captchas: []string{"CODE01"}})
		require.NoError(t, err)
		assert.Equal(t, AttendAlready, res.Status)
	})
	t.Run("the AI failing is reported", func(t *testing.T) {
		site, srv := newFakeCaptchaAttendance(t)
		_, err := attend(srv.URL, &fakeSolver{})
		require.Error(t, err)
		assert.ErrorContains(t, err, "AI 认验证码")
		assert.Empty(t, site.posts)
	})
}

// 人机验证组件（Turnstile、reCAPTCHA、hCaptcha）AI 过不了；有验证码输入框却没有 NexusPHP 的验证码图片、
// 图片在别的主机上时也认不了：都不交给 AI、不提交，当天记失败。
func TestNexusPHPDriver_AttendStopsAtCaptchasAICannotPass(t *testing.T) {
	def := &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP}
	form := func(inner string) string {
		return `<html><head><title>站点 :: 签到</title></head><body><form method="post" action="attendance.php"><table>` + inner +
			`<tr><td><input type="submit" value="立即签到" class="btn" /></td></tr></table></form></body></html>`
	}
	cases := map[string]struct{ page, reason string }{
		"turnstile": {form(`<tr><td class="rowhead">安全验证</td><td><div class="cf-turnstile" data-sitekey="k" data-theme="light"></div></td></tr>`), "人机验证"},
		"recaptcha": {form(`<tr><td class="rowhead">安全验证</td><td><div class="g-recaptcha" data-sitekey="k"></div></td></tr>`), "人机验证"},
		"hcaptcha":  {form(`<tr><td><div class="h-captcha" data-sitekey="k"></div></td></tr>`), "人机验证"},
		"no image":  {form(`<tr><td class="rowhead">验证码</td><td><input type="text" name="imagestring" value="" /></td></tr>`), "认不出来"},
		"no imagehash": {form(`<tr><td><img src="image.php?action=regimage&amp;imagehash=h1" alt="CAPTCHA" /></td></tr>` +
			`<tr><td><input type="text" name="imagestring" value="" /></td></tr>`), "认不出来"},
		"image on another host": {form(`<tr><td><img src="https://elsewhere.example/image.php?action=regimage&amp;imagehash=h1" alt="CAPTCHA" /></td></tr>` +
			`<tr><td><input type="text" name="imagestring" value="" /><input type="hidden" name="imagehash" value="h1" /></td></tr>`), "认不出来"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			srv, seen := fakeFormAttendanceSite(t, tc.page, attendanceSuccessPage)
			solver := &fakeSolver{captchas: []string{"CODE01"}}
			_, err := newAttendanceDriver(srv.URL, def).Attend(WithAttendanceSolver(context.Background(), solver))
			require.ErrorIs(t, err, ErrAttendanceCaptchaRequired)
			assert.ErrorContains(t, err, tc.reason)
			assert.ErrorContains(t, err, "请在网页上签到")
			assert.Empty(t, solver.images, "nothing handed to the AI")
			assert.Len(t, seen(), 1, "nothing submitted")
		})
	}
}

func TestCaptchaImagePath(t *testing.T) {
	d := newAttendanceDriver("https://pt.example.org", &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP})
	for _, tc := range []struct {
		page, src, want string
		ok              bool
	}{
		{"/attendance.php", "image.php?action=regimage&imagehash=h1&secret=", "/image.php?action=regimage&imagehash=h1&secret=", true},
		{"/attendance.php", "/image.php?action=regimage&imagehash=h1", "/image.php?action=regimage&imagehash=h1", true},
		{"/attendance.php", "https://PT.example.org/image.php?action=regimage&imagehash=h1", "/image.php?action=regimage&imagehash=h1", true},
		{"/plugin/attendance.php", "image.php?action=regimage&imagehash=h1", "/plugin/image.php?action=regimage&imagehash=h1", true},
		{"/attendance.php", "https://elsewhere.example/image.php?action=regimage&imagehash=h1", "", false},
		{"/attendance.php", "//elsewhere.example/image.php?action=regimage", "", false},
		{"/attendance.php", "%zz", "", false},
	} {
		got, ok := d.captchaImagePath(tc.page, tc.src)
		assert.Equal(t, tc.ok, ok, "src %q", tc.src)
		assert.Equal(t, tc.want, got, "src %q", tc.src)
	}
}

func TestAttendanceMayUseAI(t *testing.T) {
	assert.False(t, AttendanceMayUseAI(nil))
	assert.True(t, AttendanceMayUseAI(&SiteDefinition{Schema: SchemaNexusPHP}), "the default GET page may show a captcha form")
	assert.True(t, AttendanceMayUseAI(&SiteDefinition{Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Challenge: ChallengeHDSkyCaptcha}}))
	assert.False(t, AttendanceMayUseAI(&SiteDefinition{Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Unsupported: "没有签到"}}))
	assert.False(t, AttendanceMayUseAI(&SiteDefinition{Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Path: "/sign_in.php", Method: http.MethodPost}}),
		"a POST sign-in never follows a form")
	assert.False(t, AttendanceMayUseAI(&SiteDefinition{Schema: SchemaHDDolby}))
}
