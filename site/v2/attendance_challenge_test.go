package v2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSolver 按顺序给出验证码答案和题目答案，记下收到的图片与题目。
type fakeSolver struct {
	mu        sync.Mutex
	captchas  []string
	answers   [][]int
	answerErr error
	images    []CaptchaImage
	questions []AttendanceQuestion
}

func (f *fakeSolver) ReadCaptcha(_ context.Context, img CaptchaImage) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.images = append(f.images, img)
	if len(f.captchas) == 0 {
		return "", errors.New("no more captcha answers")
	}
	out := f.captchas[0]
	f.captchas = f.captchas[1:]
	return out, nil
}

func (f *fakeSolver) ChooseAnswer(_ context.Context, q AttendanceQuestion) ([]int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.questions = append(f.questions, q)
	if f.answerErr != nil {
		return nil, f.answerErr
	}
	if len(f.answers) == 0 {
		return nil, errors.New("no more answers")
	}
	out := f.answers[0]
	f.answers = f.answers[1:]
	return out, nil
}

func testCaptchaPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewGray(image.Rect(0, 0, 4, 2))))
	return buf.Bytes()
}

func challengeDriver(baseURL string, c AttendanceChallenge) *NexusPHPDriver {
	return newAttendanceDriver(baseURL, &SiteDefinition{ID: "qa", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Challenge: c}})
}

// fakeHDSky 模拟 HDSky 的签到：image_code_ajax.php 每次给一个新的 imagehash，验证码答案是 CODE0<序号>（6 位），
// showup.php 核对答案；already 为真时首页显示「已签到」。
type fakeHDSky struct {
	t        *testing.T
	mu       sync.Mutex
	issued   int
	already  bool
	dateGone bool
	posts    []string
	referers []string
}

func (f *fakeHDSky) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.URL.Path == "/index.php":
		if f.already {
			_, _ = w.Write([]byte(`<html><body><a href="userdetails.php?id=1">me</a> <span>[已签到]</span></body></html>`))
			return
		}
		_, _ = w.Write([]byte(`<html><body><a href="userdetails.php?id=1">me</a> <a href="#" id="showup">[签到]</a></body></html>`))
	case r.URL.Path == "/image_code_ajax.php" && r.Method == http.MethodPost:
		require.NoError(f.t, r.ParseForm())
		assert.Equal(f.t, "new", r.PostForm.Get("action"))
		f.referers = append(f.referers, r.Header.Get("Referer"))
		f.issued++
		_, _ = fmt.Fprintf(w, `{"success":true,"code":"hash%d"}`, f.issued)
	case r.URL.Path == "/image.php":
		assert.Equal(f.t, "regimage", r.URL.Query().Get("action"))
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(testCaptchaPNG(f.t))
	case r.URL.Path == "/showup.php" && r.Method == http.MethodPost:
		require.NoError(f.t, r.ParseForm())
		f.posts = append(f.posts, r.PostForm.Encode())
		assert.Equal(f.t, "showup", r.PostForm.Get("action"))
		hash, code := r.PostForm.Get("imagehash"), r.PostForm.Get("imagestring")
		switch {
		case f.dateGone:
			_, _ = w.Write([]byte(`{"success":false,"message":"date_unmatch"}`))
		case code == "CODE0"+strings.TrimPrefix(hash, "hash"):
			_, _ = w.Write([]byte(`{"success":true,"message":12}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"message":"invalid_imagehash"}`))
		}
	default:
		http.NotFound(w, r)
	}
}

func newFakeHDSky(t *testing.T) (*fakeHDSky, *httptest.Server) {
	f := &fakeHDSky{t: t}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func TestAttendHDSkyCaptcha(t *testing.T) {
	t.Run("认对验证码就签到成功", func(t *testing.T) {
		site, srv := newFakeHDSky(t)
		solver := &fakeSolver{captchas: []string{"CODE01"}}
		res, err := challengeDriver(srv.URL, ChallengeHDSkyCaptcha).Attend(WithAttendanceSolver(context.Background(), solver))
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		require.Len(t, solver.images, 1)
		assert.Equal(t, "image/png", solver.images[0].MIME)
		assert.Equal(t, 6, solver.images[0].Length)
		assert.True(t, solver.images[0].UpperOnly, "NexusPHP 的验证码只有大写字母和数字")
		assert.NotEmpty(t, solver.images[0].Data)
		require.Len(t, site.posts, 1)
		assert.Contains(t, site.posts[0], "imagehash=hash1")
		assert.Contains(t, site.referers[0], "/index.php", "取验证码的请求带上首页做 Referer，和浏览器里一样")
	})

	t.Run("认错了换一张再认", func(t *testing.T) {
		site, srv := newFakeHDSky(t)
		solver := &fakeSolver{captchas: []string{"WRONG1", "CODE02"}}
		res, err := challengeDriver(srv.URL, ChallengeHDSkyCaptcha).Attend(WithAttendanceSolver(context.Background(), solver))
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Equal(t, 2, site.issued, "第二次用的是新的验证码")
		assert.Len(t, site.posts, 2)
	})

	t.Run("认成小写的换成大写再提交", func(t *testing.T) {
		site, srv := newFakeHDSky(t)
		res, err := challengeDriver(srv.URL, ChallengeHDSkyCaptcha).Attend(
			WithAttendanceSolver(context.Background(), &fakeSolver{captchas: []string{"code01"}}),
		)
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Contains(t, site.posts[0], "imagestring=CODE01")
	})

	t.Run("认出的字数不对不提交", func(t *testing.T) {
		site, srv := newFakeHDSky(t)
		solver := &fakeSolver{captchas: []string{"AB C", "CODE02"}}
		res, err := challengeDriver(srv.URL, ChallengeHDSkyCaptcha).Attend(WithAttendanceSolver(context.Background(), solver))
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Len(t, site.posts, 1, "第一次认出的不是 6 位，没有提交")
	})

	t.Run("三次都认错就报验证码没认对", func(t *testing.T) {
		site, srv := newFakeHDSky(t)
		solver := &fakeSolver{captchas: []string{"WRONG1", "WRONG2", "WRONG3"}}
		_, err := challengeDriver(srv.URL, ChallengeHDSkyCaptcha).Attend(WithAttendanceSolver(context.Background(), solver))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAttendanceChallengeFailed)
		assert.Contains(t, err.Error(), "验证码")
		assert.Len(t, site.posts, 3)
	})

	t.Run("首页显示已签到就不再取验证码", func(t *testing.T) {
		site, srv := newFakeHDSky(t)
		site.already = true
		solver := &fakeSolver{}
		res, err := challengeDriver(srv.URL, ChallengeHDSkyCaptcha).Attend(WithAttendanceSolver(context.Background(), solver))
		require.NoError(t, err)
		assert.Equal(t, AttendAlready, res.Status)
		assert.Zero(t, site.issued)
		assert.Empty(t, solver.images)
	})

	t.Run("站点回 date_unmatch 算今天已签到", func(t *testing.T) {
		site, srv := newFakeHDSky(t)
		site.dateGone = true
		res, err := challengeDriver(srv.URL, ChallengeHDSkyCaptcha).Attend(
			WithAttendanceSolver(context.Background(), &fakeSolver{captchas: []string{"CODE01"}}),
		)
		require.NoError(t, err)
		assert.Equal(t, AttendAlready, res.Status)
	})

	t.Run("没有 AI 时说明要 AI，不发请求", func(t *testing.T) {
		site, srv := newFakeHDSky(t)
		_, err := challengeDriver(srv.URL, ChallengeHDSkyCaptcha).Attend(context.Background())
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAttendanceNeedsAI)
		assert.Contains(t, err.Error(), "验证码")
		assert.Zero(t, site.issued)
	})
}

// fakeOpenCD 模拟 OpenCD 的签到插件，照 2026-11-17 在真站点上看到的样子：
//   - 请求没带 c_csrf 时站点用 Set-Cookie 发一个（HttpOnly）；签到表单里的 csrf_token 就是这个值；
//   - plugin_sign-in.php 不管今天签没签都显示表单（csrf_token、imagehash、验证码图片），cmd=signin 先核对
//     csrf_token 与 Cookie 里的 c_csrf，不一样时回 HTTP 403 和纯文本「CSRF token verification failed」，再核对验证码（CODE0<序号>）；
//   - 首页顶栏今天签过以后是「查看簽到記錄」的链接，没签时是调用 signin(this) 的签到按钮。
type fakeOpenCD struct {
	t      *testing.T
	mu     sync.Mutex
	issued int
	// signed 是今天签过了
	signed bool
	// homeStale 为真时首页一直显示签到按钮（例如别处刚签完）
	homeStale bool
	// homeBoth 为真时首页顶栏既有签到按钮又有「查看簽到記錄」
	homeBoth bool
	// noForm 为真时签到页没有表单、只写着今天签过了
	noForm bool
	// badToken 为真时表单里的 csrf_token 和 Cookie 对不上（站点改了校验方式）
	badToken bool
	csrfSet  int
	posts    []url.Values
}

const fakeOpenCDToken = "f00dfacecafe"

func (f *fakeOpenCD) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	csrf := ""
	if c, err := r.Cookie("c_csrf"); err == nil {
		csrf = c.Value
	} else {
		csrf = fakeOpenCDToken
		f.csrfSet++
		http.SetCookie(w, &http.Cookie{Name: "c_csrf", Value: csrf, Path: "/", HttpOnly: true})
	}
	switch {
	case r.URL.Path == "/index.php":
		signIn := `[<a href="javascript:void(0);" onclick="signin(this);"><b>簽到</b></a>]`
		signLog := `[<a href='/plugin_sign-in.php?cmd=show-log' style='color:blue;' target='_blank'>查看簽到記錄</a>]`
		bar := signIn
		switch {
		case f.homeBoth:
			bar = signIn + " " + signLog
		case f.signed && !f.homeStale:
			bar = signLog
		}
		_, _ = fmt.Fprintf(w, `<html><body><div class="infos-bar">%s [<a href="blackjack.php">21點</a>]</div></body></html>`, bar)
	case r.URL.Path == "/plugin_sign-in.php" && r.Method == http.MethodGet:
		if f.noForm {
			_, _ = w.Write([]byte(`<html><body><p>您今天已经签到过了，明天再来吧</p></body></html>`))
			return
		}
		f.issued++
		token := csrf
		if f.badToken {
			token = "not-" + csrf
		}
		_, _ = fmt.Fprintf(w, `<html><body><form enctype="multipart/form-data" action="plugin_sign-in.php?cmd=signin" method="post" id="frmSignin"><input type="hidden" name="csrf_token" value="%s">
<table><tbody><tr><td><input type="hidden" name="imagehash" value="h%d">
<img src="image.php?action=regimage&amp;imagehash=h%d" border="0">
<div><input type="text" autocomplete="off" id="imagestring" name="imagestring" value="">
<button id="ok" onclick="javascript:do_signin(this);return false;">签到</button>
<button onclick="javascript:jQuery(parent.window.document.body).find('#i_signin').remove();return false;">算了</button></div>
</td></tr></tbody></table></form></body></html>`, token, f.issued, f.issued)
	case r.URL.Path == "/image.php":
		assert.Equal(f.t, fmt.Sprintf("h%d", f.issued), r.URL.Query().Get("imagehash"))
		_, _ = w.Write(testCaptchaPNG(f.t))
	case r.URL.Path == "/plugin_sign-in.php" && r.Method == http.MethodPost:
		assert.Equal(f.t, "signin", r.URL.Query().Get("cmd"))
		require.NoError(f.t, r.ParseForm())
		f.posts = append(f.posts, r.PostForm)
		if r.PostForm.Get("csrf_token") == "" || r.PostForm.Get("csrf_token") != csrf {
			w.Header().Set("Content-Type", "text/html; charset=UTF-8")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("CSRF token verification failed"))
			return
		}
		if r.PostForm.Get("imagestring") != "CODE0"+strings.TrimPrefix(r.PostForm.Get("imagehash"), "h") {
			_, _ = w.Write([]byte(`{"state":"false","msg":"验证码错误"}`))
			return
		}
		if f.signed {
			// 真站点今天签过以后验证码认对了回什么没见过（认错时照样是「验证码错误」），这里按这类插件常见的说法
			_, _ = w.Write([]byte(`{"state":"false","msg":"您今天已经签到过了"}`))
			return
		}
		f.signed = true
		_, _ = w.Write([]byte(`{"state":"success","signindays":"3","integral":"10"}`))
	default:
		http.NotFound(w, r)
	}
}

func TestAttendOpenCDCaptcha(t *testing.T) {
	newSite := func(t *testing.T) (*fakeOpenCD, *httptest.Server) {
		f := &fakeOpenCD{t: t}
		srv := httptest.NewServer(http.HandlerFunc(f.serve))
		t.Cleanup(srv.Close)
		return f, srv
	}
	// withCookie 是用这组 Cookie 建的 OpenCD 驱动（CookieCloud、扩展同步过来的 Cookie 带着 c_csrf）
	withCookie := func(baseURL, cookie string) *NexusPHPDriver {
		d := NewNexusPHPDriver(NexusPHPDriverConfig{BaseURL: baseURL, Cookie: cookie})
		d.SetSiteDefinition(&SiteDefinition{ID: "qa", Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Challenge: ChallengeOpenCDCaptcha}})
		return d
	}

	t.Run("连同表单里的 csrf_token 提交，签到成功，写明奖励与连续天数", func(t *testing.T) {
		site, srv := newSite(t)
		res, err := withCookie(srv.URL, "c_secure_uid=1; c_secure_pass=2; c_csrf="+fakeOpenCDToken).Attend(
			WithAttendanceSolver(context.Background(), &fakeSolver{captchas: []string{"CODE01"}}),
		)
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Contains(t, res.Message, "10")
		assert.Contains(t, res.Message, "3")
		require.Len(t, site.posts, 1)
		assert.Equal(t, fakeOpenCDToken, site.posts[0].Get("csrf_token"))
		assert.Equal(t, "h1", site.posts[0].Get("imagehash"))
		assert.Equal(t, "CODE01", site.posts[0].Get("imagestring"))
		assert.Zero(t, site.csrfSet, "Cookie 里有 c_csrf，站点不用再发")
	})

	t.Run("保存的 Cookie 里没有 c_csrf：用站点这次发的那个", func(t *testing.T) {
		site, srv := newSite(t)
		res, err := challengeDriver(srv.URL, ChallengeOpenCDCaptcha).Attend(
			WithAttendanceSolver(context.Background(), &fakeSolver{captchas: []string{"CODE01"}}),
		)
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Equal(t, 1, site.csrfSet, "只在第一个请求发一次，之后的请求带着它")
	})

	t.Run("认错了重新打开签到页拿新验证码", func(t *testing.T) {
		site, srv := newSite(t)
		res, err := challengeDriver(srv.URL, ChallengeOpenCDCaptcha).Attend(
			WithAttendanceSolver(context.Background(), &fakeSolver{captchas: []string{"WRONG1", "CODE02"}}),
		)
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Equal(t, 2, site.issued)
	})

	t.Run("三次都错报站点给的原因", func(t *testing.T) {
		_, srv := newSite(t)
		_, err := challengeDriver(srv.URL, ChallengeOpenCDCaptcha).Attend(
			WithAttendanceSolver(context.Background(), &fakeSolver{captchas: []string{"WRONG1", "WRONG2", "WRONG3"}}),
		)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAttendanceChallengeFailed)
		assert.Contains(t, err.Error(), "验证码错误")
	})

	t.Run("首页顶栏已经是「查看簽到記錄」：今天签过，不打开签到页、不调 AI", func(t *testing.T) {
		site, srv := newSite(t)
		site.signed = true
		solver := &fakeSolver{}
		res, err := challengeDriver(srv.URL, ChallengeOpenCDCaptcha).Attend(WithAttendanceSolver(context.Background(), solver))
		require.NoError(t, err)
		assert.Equal(t, AttendAlready, res.Status)
		assert.Empty(t, solver.images)
		assert.Zero(t, site.issued)
	})

	t.Run("首页顶栏还有签到按钮时照样签到", func(t *testing.T) {
		site, srv := newSite(t)
		site.homeBoth = true
		res, err := challengeDriver(srv.URL, ChallengeOpenCDCaptcha).Attend(
			WithAttendanceSolver(context.Background(), &fakeSolver{captchas: []string{"CODE01"}}),
		)
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
	})

	t.Run("提交后站点说今天已经签过：记为已签到，不再换验证码", func(t *testing.T) {
		site, srv := newSite(t)
		site.signed, site.homeStale = true, true
		solver := &fakeSolver{captchas: []string{"CODE01", "CODE02"}}
		res, err := challengeDriver(srv.URL, ChallengeOpenCDCaptcha).Attend(WithAttendanceSolver(context.Background(), solver))
		require.NoError(t, err)
		assert.Equal(t, AttendAlready, res.Status)
		assert.Len(t, solver.images, 1)
	})

	t.Run("CSRF 校验没过：写明是 CSRF，不说 Cookie 失效，也不再换验证码", func(t *testing.T) {
		site, srv := newSite(t)
		site.badToken = true
		solver := &fakeSolver{captchas: []string{"CODE01", "CODE02", "CODE03"}}
		_, err := challengeDriver(srv.URL, ChallengeOpenCDCaptcha).Attend(WithAttendanceSolver(context.Background(), solver))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidCredentials)
		assert.Contains(t, err.Error(), "CSRF")
		assert.Contains(t, err.Error(), "c_csrf")
		assert.True(t, strings.HasPrefix(err.Error(), ErrInvalidCredentials.Error()+": "),
			"原因紧跟在 ErrInvalidCredentials 后面，签到记录才显示它而不是「Cookie 已失效」：%v", err)
		assert.Len(t, solver.images, 1)
		assert.Len(t, site.posts, 1)
	})

	t.Run("签到页没有表单、写着已签到", func(t *testing.T) {
		site, srv := newSite(t)
		site.noForm = true
		solver := &fakeSolver{}
		res, err := challengeDriver(srv.URL, ChallengeOpenCDCaptcha).Attend(WithAttendanceSolver(context.Background(), solver))
		require.NoError(t, err)
		assert.Equal(t, AttendAlready, res.Status)
		assert.Empty(t, solver.images)
	})
}

// fakeBaka 模拟 bakatest.php 的每日签到答题（CHDBits、52PT）：选项值是 1/2/4/8，答案按位相加核对。
type fakeBaka struct {
	t        *testing.T
	mu       sync.Mutex
	multiple bool
	answer   int
	page     string // 不为空时 GET 直接回它（已签到、考试页）
	posts    []map[string][]string
}

func (f *fakeBaka) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.URL.Path != "/bakatest.php" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodPost {
		require.NoError(f.t, r.ParseForm())
		f.posts = append(f.posts, r.PostForm)
		sum := 0
		for _, v := range r.PostForm["choice[]"] {
			var n int
			_, _ = fmt.Sscanf(v, "%d", &n)
			sum += n
		}
		notice := "答题错误,获得5点魔力值"
		if sum == f.answer {
			notice = "连续3天签到,获得14点魔力值"
		}
		_, _ = fmt.Fprintf(w, `<html><body><table><tr><td><b><font color="white">%s</font></b></td></tr></table><h1>2026-10-19 签到记录</h1></body></html>`, notice)
		return
	}
	if f.page != "" {
		_, _ = w.Write([]byte(f.page))
		return
	}
	kind, label := "radio", "[单选]"
	if f.multiple {
		kind, label = "checkbox", "[多选]"
	}
	_, _ = fmt.Fprintf(w, `<html><body><div id="outer"><h1>每日签到</h1>
<form action="/bakatest.php" method="post"><table>
<tr><td class="text" align="left" width="100%%">1、[老手级]%s 请问：以下哪些是 BT 客户端？</td></tr>
<tr><td class="text" align="left" width="100%%">
<input type="hidden" name="questionid" value="450" />
<input type='%s' name='choice[]' value='8' >Photoshop<br /><input type='%s' name='choice[]' value='1' >qBittorrent<br /><input type='%s' name='choice[]' value='4' ><b>Transmission</b> 4.x<br /><input type='%s' name='choice[]' value='2' >Excel<br />
</td></tr>
<tr><td class='text'><textarea name='usercomment'>此刻心情:无</textarea></td></tr>
<tr><td class="text" align="center"><input type="submit" name="submit" value="提交" /><input type="submit" name="wantskip" value="不会" /></td></tr>
</table></form></div></body></html>`, label, kind, kind, kind, kind)
}

func TestAttendBakaQuiz(t *testing.T) {
	newSite := func(t *testing.T, f *fakeBaka) *httptest.Server {
		f.t = t
		srv := httptest.NewServer(http.HandlerFunc(f.serve))
		t.Cleanup(srv.Close)
		return srv
	}

	t.Run("单选：AI 选对就签到成功", func(t *testing.T) {
		site := &fakeBaka{answer: 1}
		srv := newSite(t, site)
		solver := &fakeSolver{answers: [][]int{{1}}}
		res, err := challengeDriver(srv.URL, ChallengeBakaQuiz).Attend(WithAttendanceSolver(context.Background(), solver))
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Contains(t, res.Message, "14点魔力值")
		require.Len(t, solver.questions, 1)
		q := solver.questions[0]
		assert.Equal(t, "以下哪些是 BT 客户端？", q.Question)
		assert.Equal(t, []string{"Photoshop", "qBittorrent", "Transmission 4.x", "Excel"}, q.Options)
		assert.False(t, q.Multiple)
		require.Len(t, site.posts, 1)
		assert.Equal(t, []string{"1"}, site.posts[0]["choice[]"])
		assert.Equal(t, []string{"450"}, site.posts[0]["questionid"])
		assert.Equal(t, []string{"提交"}, site.posts[0]["submit"], "按「提交」答题，不是「不会」")
		assert.Empty(t, site.posts[0]["wantskip"])
	})

	t.Run("多选：选中的几项一起提交", func(t *testing.T) {
		site := &fakeBaka{answer: 5, multiple: true}
		srv := newSite(t, site)
		solver := &fakeSolver{answers: [][]int{{1, 2}}}
		res, err := challengeDriver(srv.URL, ChallengeBakaQuiz).Attend(WithAttendanceSolver(context.Background(), solver))
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.True(t, solver.questions[0].Multiple)
		assert.ElementsMatch(t, []string{"1", "4"}, site.posts[0]["choice[]"])
	})

	t.Run("答错了站点照样记签到，说明里写明答错", func(t *testing.T) {
		srv := newSite(t, &fakeBaka{answer: 1})
		res, err := challengeDriver(srv.URL, ChallengeBakaQuiz).Attend(
			WithAttendanceSolver(context.Background(), &fakeSolver{answers: [][]int{{3}}}),
		)
		require.NoError(t, err)
		assert.Equal(t, AttendSigned, res.Status)
		assert.Contains(t, res.Message, "答错")
		assert.Contains(t, res.Message, "5点魔力值")
	})

	t.Run("今天已经签过就不答题", func(t *testing.T) {
		site := &fakeBaka{page: `<html><body><table><tr><td><b>今天已经签过到了(已连续3天签到)</b></td></tr></table></body></html>`}
		srv := newSite(t, site)
		solver := &fakeSolver{}
		res, err := challengeDriver(srv.URL, ChallengeBakaQuiz).Attend(WithAttendanceSolver(context.Background(), solver))
		require.NoError(t, err)
		assert.Equal(t, AttendAlready, res.Status)
		assert.Empty(t, solver.questions)
		assert.Empty(t, site.posts)
	})

	t.Run("页面是新用户考试不是签到时不答", func(t *testing.T) {
		site := &fakeBaka{page: `<html><body><h1>PT使用资格认证考试</h1><p>这货不是签到!</p></body></html>`}
		srv := newSite(t, site)
		_, err := challengeDriver(srv.URL, ChallengeBakaQuiz).Attend(
			WithAttendanceSolver(context.Background(), &fakeSolver{answers: [][]int{{0}}}),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "考试")
		assert.Empty(t, site.posts)
	})

	t.Run("AI 没给出答案时不提交", func(t *testing.T) {
		site := &fakeBaka{answer: 1}
		srv := newSite(t, site)
		_, err := challengeDriver(srv.URL, ChallengeBakaQuiz).Attend(
			WithAttendanceSolver(context.Background(), &fakeSolver{answerErr: errors.New("模型超时")}),
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "模型超时")
		assert.Empty(t, site.posts)
	})

	t.Run("AI 给的选项编号越界时不提交", func(t *testing.T) {
		site := &fakeBaka{answer: 1}
		srv := newSite(t, site)
		_, err := challengeDriver(srv.URL, ChallengeBakaQuiz).Attend(
			WithAttendanceSolver(context.Background(), &fakeSolver{answers: [][]int{{7}}}),
		)
		require.Error(t, err)
		assert.Empty(t, site.posts)
	})
}

func TestAttendanceChallengeOf(t *testing.T) {
	assert.Equal(t, AttendanceChallenge(""), AttendanceChallengeOf(nil))
	assert.Equal(t, AttendanceChallenge(""), AttendanceChallengeOf(&SiteDefinition{Schema: SchemaNexusPHP}))
	def := &SiteDefinition{Schema: SchemaNexusPHP, Attendance: &AttendanceConfig{Challenge: ChallengeBakaQuiz}}
	assert.Equal(t, ChallengeBakaQuiz, AttendanceChallengeOf(def))
	ok, reason := AttendanceSupport(def)
	assert.True(t, ok, "要过一关的站点也算支持：有 AI 时能签")
	assert.Empty(t, reason)
}
