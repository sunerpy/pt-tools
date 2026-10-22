package v2

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	stdhtml "html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

// AttendanceChallenge 是签到前要过的一关：认验证码或者答题。站点定义里设了它，签到时要有 AI（AttendanceSolver）。
type AttendanceChallenge string

const (
	// ChallengeHDSkyCaptcha 是 HDSky 的签到验证码：POST /image_code_ajax.php 取 imagehash，
	// 认 /image.php?action=regimage&imagehash=… 上的 6 位字符，POST /showup.php 提交。
	ChallengeHDSkyCaptcha AttendanceChallenge = "hdsky_captcha"
	// ChallengeOpenCDCaptcha 是 OpenCD 签到插件的验证码：GET /plugin_sign-in.php 取表单里的 imagehash 与验证码图片，
	// POST /plugin_sign-in.php?cmd=signin 提交。
	ChallengeOpenCDCaptcha AttendanceChallenge = "opencd_captcha"
	// ChallengeBakaQuiz 是 CHDBits、52PT 的签到答题（bakatest.php）：单选或多选，选项的值是 1、2、4、8，
	// 按「提交」答题；答错站点照样记签到，只是奖励少。
	ChallengeBakaQuiz AttendanceChallenge = "baka_quiz"
)

// IsCaptcha 报告这一关是不是认验证码（要能看图的模型）；不是的话是答题（文字模型就行）。
func (c AttendanceChallenge) IsCaptcha() bool {
	return c == ChallengeHDSkyCaptcha || c == ChallengeOpenCDCaptcha
}

// CaptchaImage 是一张验证码图片。
type CaptchaImage struct {
	Data []byte
	// MIME 是图片类型，如 image/png
	MIME string
	// Length 是验证码的字符数，0 表示不确定
	Length int
	// UpperOnly 为真时验证码只有大写字母和数字（NexusPHP 的签到验证码），认出来的统一换成大写
	UpperOnly bool
}

// AttendanceQuestion 是一道签到选择题。
type AttendanceQuestion struct {
	Question string
	Options  []string
	// Multiple 为真时是多选题
	Multiple bool
}

// AttendanceSolver 帮签到过那一关：认验证码、答选择题。由 AI 助手实现，签到监控按设置放进 ctx（WithAttendanceSolver）。
type AttendanceSolver interface {
	// ReadCaptcha 读出验证码图片上的字符。
	ReadCaptcha(ctx context.Context, img CaptchaImage) (string, error)
	// ChooseAnswer 回答选择题，返回选中的选项下标（从 0 开始；多选题可以有几个）。
	ChooseAnswer(ctx context.Context, q AttendanceQuestion) ([]int, error)
}

type attendanceSolverKey struct{}

// WithAttendanceSolver 把签到用的 AI 放进 ctx。
func WithAttendanceSolver(ctx context.Context, s AttendanceSolver) context.Context {
	if s == nil {
		return ctx
	}
	return context.WithValue(ctx, attendanceSolverKey{}, s)
}

// AttendanceSolverFrom 取 ctx 里签到用的 AI，没有时为 nil。
func AttendanceSolverFrom(ctx context.Context) AttendanceSolver {
	s, _ := ctx.Value(attendanceSolverKey{}).(AttendanceSolver)
	return s
}

// AttendanceAIUsage 记一次签到里 AI 被调了几次、用的模型与 token：签到监控放进 ctx（WithAttendanceAIUsage），
// AI 那一边（cmd 里接的 AI 助手）每拿到一次回答往里加一次。并发安全。
type AttendanceAIUsage struct {
	mu               sync.Mutex
	calls            int
	model            string
	promptTokens     int64
	completionTokens int64
}

// Add 记一次模型的回答：用的模型与服务商报的 token（没报时为 0）。
func (u *AttendanceAIUsage) Add(model string, prompt, completion int64) {
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls++
	if model != "" {
		u.model = model
	}
	u.promptTokens += max(prompt, 0)
	u.completionTokens += max(completion, 0)
}

// Snapshot 返回到现在为止的调用次数、最后用的模型与 token 合计。
func (u *AttendanceAIUsage) Snapshot() (calls int, model string, prompt, completion int64) {
	if u == nil {
		return 0, "", 0, 0
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.calls, u.model, u.promptTokens, u.completionTokens
}

type attendanceAIUsageKey struct{}

// WithAttendanceAIUsage 把记 AI 用量的地方放进 ctx。
func WithAttendanceAIUsage(ctx context.Context, u *AttendanceAIUsage) context.Context {
	if u == nil {
		return ctx
	}
	return context.WithValue(ctx, attendanceAIUsageKey{}, u)
}

// AttendanceAIUsageFrom 取 ctx 里记 AI 用量的地方，没有时为 nil（Add 对 nil 什么也不做）。
func AttendanceAIUsageFrom(ctx context.Context) *AttendanceAIUsage {
	u, _ := ctx.Value(attendanceAIUsageKey{}).(*AttendanceAIUsage)
	return u
}

var (
	// ErrAttendanceNeedsAI 表示站点签到要过验证码或答题，而这次没有可用的 AI。
	ErrAttendanceNeedsAI = errors.New("attendance needs AI")
	// ErrAttendanceChallengeFailed 表示几次都没过那一关（验证码一直没认对、页面里找不到题目等）。
	ErrAttendanceChallengeFailed = errors.New("attendance challenge failed")
)

// AttendanceChallengeOf 返回站点签到要过的一关；不用过的返回空串。
func AttendanceChallengeOf(def *SiteDefinition) AttendanceChallenge {
	if def == nil || def.Attendance == nil {
		return ""
	}
	return def.Attendance.Challenge
}

// NeedsAIReason 是没有 AI 时写给用户的原因（不含怎么打开，那一半由签到监控按设置补上）。
func (c AttendanceChallenge) NeedsAIReason() string {
	if c.IsCaptcha() {
		return "这个站点签到要识别验证码"
	}
	return "这个站点签到要答题"
}

// captchaTries 是一次签到里最多认几张验证码：认错了换一张再认。
const captchaTries = 3

func (d *NexusPHPDriver) attendWithChallenge(ctx context.Context, c AttendanceChallenge) (AttendResult, error) {
	solver := AttendanceSolverFrom(ctx)
	if solver == nil {
		return AttendResult{}, fmt.Errorf("%w: %s", ErrAttendanceNeedsAI, c.NeedsAIReason())
	}
	switch c {
	case ChallengeHDSkyCaptcha:
		return d.attendHDSky(ctx, solver)
	case ChallengeOpenCDCaptcha:
		return d.attendOpenCD(ctx, solver)
	case ChallengeBakaQuiz:
		return d.attendBakaQuiz(ctx, solver)
	default:
		return AttendResult{}, fmt.Errorf("%w: 不认识的签到方式 %q", ErrAttendanceUnsupported, c)
	}
}

// fetchCaptcha 取一张验证码图片；回来的不是图片（例如登录页）时报错。
func (d *NexusPHPDriver) fetchCaptcha(ctx context.Context, path, referer string, length int) (CaptchaImage, error) {
	// HDSky、OpenCD 用的是 NexusPHP 的验证码（image.php?action=regimage）：大写字母和数字
	res, err := d.Execute(ctx, NexusPHPRequest{Path: path, Referer: referer})
	if err != nil {
		return CaptchaImage{}, fmt.Errorf("取验证码图片: %w", err)
	}
	mime := http.DetectContentType(res.RawBody)
	if !strings.HasPrefix(mime, "image/") {
		return CaptchaImage{}, fmt.Errorf("%w: 验证码地址回的不是图片（%s）", ErrAttendanceChallengeFailed, mime)
	}
	return CaptchaImage{Data: res.RawBody, MIME: mime, Length: length, UpperOnly: true}, nil
}

// readCaptcha 让 AI 认图，只留字母和数字；字数不对时返回空串（不提交，换一张）。
func readCaptcha(ctx context.Context, s AttendanceSolver, img CaptchaImage) (string, error) {
	text, err := s.ReadCaptcha(ctx, img)
	if err != nil {
		return "", err
	}
	text = keepAlnum(text)
	if img.UpperOnly {
		text = strings.ToUpper(text)
	}
	if img.Length > 0 && len(text) != img.Length {
		return "", nil
	}
	return text, nil
}

func keepAlnum(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var hdskySignedPattern = regexp.MustCompile(`已签到|已簽到`)

// attendHDSky：首页已经显示「已签到」就不取验证码；否则取 imagehash、认图、提交，认错了换一张，最多 captchaTries 次。
func (d *NexusPHPDriver) attendHDSky(ctx context.Context, s AttendanceSolver) (AttendResult, error) {
	home, err := d.Execute(ctx, NexusPHPRequest{Path: "/index.php"})
	if err != nil {
		return AttendResult{}, fmt.Errorf("打开首页: %w", err)
	}
	if hdskySignedPattern.MatchString(attendancePageText(home.RawBody)) {
		return AttendResult{Status: AttendAlready, Message: "今天已经签到过"}, nil
	}
	reason := "验证码一直没认对"
	for range captchaTries {
		res, err := d.Execute(ctx, NexusPHPRequest{
			Path: "/image_code_ajax.php", Method: http.MethodPost,
			Params: url.Values{"action": {"new"}}, Referer: "/index.php",
		})
		if err != nil {
			return AttendResult{}, fmt.Errorf("取验证码: %w", err)
		}
		var hash struct {
			Success bool   `json:"success"`
			Code    string `json:"code"`
		}
		if json.Unmarshal(res.RawBody, &hash) != nil || !hash.Success || hash.Code == "" {
			return AttendResult{}, fmt.Errorf("%w: 站点没有给出验证码（%s）", ErrAttendanceChallengeFailed, snippet(res.RawBody))
		}
		img, err := d.fetchCaptcha(ctx, "/image.php?action=regimage&imagehash="+url.QueryEscape(hash.Code), "/index.php", 6)
		if err != nil {
			return AttendResult{}, err
		}
		code, err := readCaptcha(ctx, s, img)
		if err != nil {
			return AttendResult{}, fmt.Errorf("AI 认验证码: %w", err)
		}
		if code == "" {
			reason = "AI 认出的验证码不是 6 位"
			continue
		}
		res, err = d.Execute(ctx, NexusPHPRequest{
			Path: "/showup.php", Method: http.MethodPost, Referer: "/index.php",
			Params: url.Values{"action": {"showup"}, "imagehash": {hash.Code}, "imagestring": {code}},
		})
		if err != nil {
			return AttendResult{}, fmt.Errorf("提交签到: %w", err)
		}
		var out struct {
			Success bool            `json:"success"`
			Message json.RawMessage `json:"message"`
		}
		if json.Unmarshal(res.RawBody, &out) != nil {
			return AttendResult{}, fmt.Errorf("%w: 签到返回的不是 JSON（%s）", ErrAttendanceUnrecognized, snippet(res.RawBody))
		}
		msg := rawText(out.Message)
		switch {
		case out.Success:
			if n, err := strconv.Atoi(msg); err == nil && n > 0 {
				return AttendResult{Status: AttendSigned, Message: fmt.Sprintf("签到成功，获得 %d 魔力值", n)}, nil
			}
			return AttendResult{Status: AttendSigned, Message: "签到成功"}, nil
		case msg == "date_unmatch":
			return AttendResult{Status: AttendAlready, Message: "今天已经签到过"}, nil
		case msg == "invalid_imagehash":
			reason = "验证码没认对"
		default:
			return AttendResult{}, fmt.Errorf("%w: 站点回的是 %q", ErrAttendanceUnrecognized, msg)
		}
	}
	return AttendResult{}, fmt.Errorf("%w: 试了 %d 次，%s", ErrAttendanceChallengeFailed, captchaTries, reason)
}

var openCDSignedPattern = regexp.MustCompile(`(?:今天|今日)?(?:已经|已經)?(?:签到|簽到)过了|已(?:经|經)?(?:签到|簽到)`)

// attendOpenCD：每次打开签到页拿新的 imagehash 与验证码，认图后提交；页面上没有签到表单、写着已签到时算今天已签。
func (d *NexusPHPDriver) attendOpenCD(ctx context.Context, s AttendanceSolver) (AttendResult, error) {
	reason := "验证码一直没认对"
	for range captchaTries {
		page, err := d.Execute(ctx, NexusPHPRequest{Path: "/plugin_sign-in.php"})
		if err != nil {
			return AttendResult{}, fmt.Errorf("打开签到页: %w", err)
		}
		form := page.Document.Find("form#frmSignin")
		hash, _ := form.Find("input[name='imagehash']").Attr("value")
		src, _ := form.Find("img").Attr("src")
		if form.Length() == 0 || hash == "" || src == "" {
			if openCDSignedPattern.MatchString(attendancePageText(page.RawBody)) {
				return AttendResult{Status: AttendAlready, Message: "今天已经签到过"}, nil
			}
			return AttendResult{}, fmt.Errorf("%w: 签到页上没有签到表单", ErrAttendanceChallengeFailed)
		}
		img, err := d.fetchCaptcha(ctx, "/"+strings.TrimPrefix(src, "/"), "/plugin_sign-in.php", 6)
		if err != nil {
			return AttendResult{}, err
		}
		code, err := readCaptcha(ctx, s, img)
		if err != nil {
			return AttendResult{}, fmt.Errorf("AI 认验证码: %w", err)
		}
		if code == "" {
			reason = "AI 认出的验证码不是 6 位"
			continue
		}
		res, err := d.Execute(ctx, NexusPHPRequest{
			Path: "/plugin_sign-in.php?cmd=signin", Method: http.MethodPost, Referer: "/plugin_sign-in.php",
			Params: url.Values{"imagehash": {hash}, "imagestring": {code}},
		})
		if err != nil {
			return AttendResult{}, fmt.Errorf("提交签到: %w", err)
		}
		var out struct {
			State      string          `json:"state"`
			SigninDays json.RawMessage `json:"signindays"`
			Integral   json.RawMessage `json:"integral"`
			Msg        string          `json:"msg"`
		}
		if json.Unmarshal(res.RawBody, &out) != nil {
			return AttendResult{}, fmt.Errorf("%w: 签到返回的不是 JSON（%s）", ErrAttendanceUnrecognized, snippet(res.RawBody))
		}
		if out.State == "success" {
			msg := "签到成功"
			if v := rawText(out.Integral); v != "" {
				msg += "，获得 " + v + " 魔力值"
			}
			if v := rawText(out.SigninDays); v != "" && v != "0" {
				msg += "，连续 " + v + " 天"
			}
			return AttendResult{Status: AttendSigned, Message: msg}, nil
		}
		reason = "验证码没认对"
		if out.Msg != "" {
			reason = "站点说「" + out.Msg + "」"
		}
	}
	return AttendResult{}, fmt.Errorf("%w: 试了 %d 次，%s", ErrAttendanceChallengeFailed, captchaTries, reason)
}

var (
	bakaAlreadyPattern = regexp.MustCompile(`今天已经签过到了|今天已經簽過到了`)
	// bakaSignedPattern 是答对后的提示：「签到成功,获得14点魔力值」「连续3天签到,获得14点魔力值」
	bakaSignedPattern = regexp.MustCompile(`(?:签到成功|连续\d+天签到)\s*[,，]\s*获得[\d+\-]+点魔力值(?:\(连续\d+天签到\))?`)
	// bakaWrongPattern 是答错后的提示：站点照样记了签到，只是奖励少
	bakaWrongPattern = regexp.MustCompile(`答题错误\s*[,，]\s*获得[\d+\-]+点魔力值(?:\(连续\d+天签到\))?`)
	bakaExamPattern  = regexp.MustCompile(`资格认证考试|这货不是签到|答题完毕`)
)

// attendBakaQuiz：今天签过就不答；页面是新用户考试（不是每日签到）时不答；否则把题目和选项交给 AI，按「提交」答题。
func (d *NexusPHPDriver) attendBakaQuiz(ctx context.Context, s AttendanceSolver) (AttendResult, error) {
	page, err := d.Execute(ctx, NexusPHPRequest{Path: "/bakatest.php"})
	if err != nil {
		return AttendResult{}, fmt.Errorf("打开签到页: %w", err)
	}
	text := attendancePageText(page.RawBody)
	if bakaAlreadyPattern.MatchString(text) {
		return AttendResult{Status: AttendAlready, Message: "今天已经签过到了"}, nil
	}
	if bakaExamPattern.MatchString(text) {
		return AttendResult{}, fmt.Errorf("%w: 这一页是站点的考试题，不是每日签到：请先在浏览器里做完考试", ErrAttendanceChallengeFailed)
	}
	q, values, questionID, err := parseBakaQuestion(page.Document)
	if err != nil {
		return AttendResult{}, err
	}
	picked, err := s.ChooseAnswer(ctx, q)
	if err != nil {
		return AttendResult{}, fmt.Errorf("AI 答题: %w", err)
	}
	choices, err := pickedValues(picked, values, q.Multiple)
	if err != nil {
		return AttendResult{}, err
	}
	res, err := d.Execute(ctx, NexusPHPRequest{
		Path: "/bakatest.php", Method: http.MethodPost, Referer: "/bakatest.php",
		Params: url.Values{
			"questionid": {questionID}, "choice[]": choices,
			"usercomment": {"此刻心情:无"}, "submit": {"提交"},
		},
	})
	if err != nil {
		return AttendResult{}, fmt.Errorf("提交答案: %w", err)
	}
	text = attendancePageText(res.RawBody)
	switch {
	case bakaSignedPattern.MatchString(text):
		return AttendResult{Status: AttendSigned, Message: bakaSignedPattern.FindString(text)}, nil
	case bakaWrongPattern.MatchString(text):
		return AttendResult{Status: AttendSigned, Message: "AI 答错了，站点照样记了签到：" + bakaWrongPattern.FindString(text)}, nil
	case bakaAlreadyPattern.MatchString(text):
		return AttendResult{Status: AttendAlready, Message: "今天已经签过到了"}, nil
	}
	return AttendResult{}, fmt.Errorf("%w: 提交答案后页面里没有签到结果", ErrAttendanceUnrecognized)
}

// parseBakaQuestion 读出题目、选项（文字与值）和题号。题目在「请问：」后面；选项是 name="choice[]" 的单选或复选框，
// 文字跟在框后面、到 <br> 为止。
func parseBakaQuestion(doc *goquery.Document) (AttendanceQuestion, []string, string, error) {
	var q AttendanceQuestion
	questionID, _ := doc.Find("input[name='questionid']").Attr("value")
	doc.Find("td").EachWithBreak(func(_ int, td *goquery.Selection) bool {
		t := collapseSpaces(td.Text())
		if _, after, ok := strings.Cut(t, "请问："); ok && td.Find("td").Length() == 0 {
			q.Question = strings.TrimSpace(after)
			return false
		}
		return true
	})
	var values []string
	doc.Find("input[name='choice[]']").Each(func(_ int, in *goquery.Selection) {
		v, _ := in.Attr("value")
		if t, _ := in.Attr("type"); strings.EqualFold(t, "checkbox") {
			q.Multiple = true
		}
		values = append(values, v)
		q.Options = append(q.Options, optionLabel(in.Nodes[0]))
	})
	if questionID == "" || q.Question == "" || len(values) < 2 {
		return q, nil, "", fmt.Errorf("%w: 签到页上没有找到题目和选项", ErrAttendanceChallengeFailed)
	}
	return q, values, questionID, nil
}

// optionLabel 取选项框后面的文字：往后一直读到 <br> 或下一个输入框。
func optionLabel(n *html.Node) string {
	var b strings.Builder
	for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
		if sib.Type == html.ElementNode && (sib.Data == "br" || sib.Data == "input") {
			break
		}
		switch sib.Type {
		case html.TextNode:
			b.WriteString(sib.Data)
		case html.ElementNode:
			b.WriteString(goquery.NewDocumentFromNode(sib).Text())
		}
	}
	return collapseSpaces(stdhtml.UnescapeString(b.String()))
}

// pickedValues 把 AI 选的下标换成表单里的值：越界、重复的不要；单选只取第一个；一个都没有就报错。
func pickedValues(picked []int, values []string, multiple bool) ([]string, error) {
	seen := map[int]bool{}
	var out []string
	for _, i := range picked {
		if i < 0 || i >= len(values) || seen[i] {
			continue
		}
		seen[i] = true
		out = append(out, values[i])
		if !multiple {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: AI 选的选项不在题目里", ErrAttendanceChallengeFailed)
	}
	return out, nil
}

// rawText 把 JSON 里的字符串或数字统一成文字。
func rawText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return strings.Trim(strings.TrimSpace(string(raw)), `"`)
}

// snippet 取返回内容的开头，放进错误信息（不超过 80 字）。
func snippet(body []byte) string {
	t := []rune(attendancePageText(body))
	if len(t) > 80 {
		t = append(t[:80], '…')
	}
	return string(t)
}
