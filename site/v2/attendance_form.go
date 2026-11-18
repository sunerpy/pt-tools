package v2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
)

// 新版 NexusPHP（xiaomlove/nexusphp 2025-10-14 起，设置 captcha.attendance.enabled 默认打开）打开 attendance.php 不再直接签到，
// 只显示一个「立即签到」按钮：表单 POST 回 attendance.php。站点打开了验证码（安全设置 iv=yes）时，表单里还有
// show_image_code() 输出的验证码：默认是 NexusPHP 的图片验证码（img image.php?action=regimage、imagestring 输入框、
// imagehash 隐藏字段，6 位大写字母和数字，用过一次就作废），站点也可以换成 Cloudflare Turnstile 或 reCAPTCHA v2。
// 提交后成功页与旧版相同（「签到成功」「这是您的第 N 次签到」），已经签过时回「您今天已经签到过了」，
// 验证码不对时回「图片代码无效」。

// ErrAttendanceCaptchaRequired 表示签到表单要过 pt-tools 过不了的验证（Turnstile、reCAPTCHA 等人机验证组件），当天重试也签不上。
// 图片验证码不算：交给 AI 识别，没有 AI 时是 ErrAttendanceNeedsAI。
var ErrAttendanceCaptchaRequired = errors.New("attendance needs a captcha")

const (
	// attendanceWidgetReason 是签到表单要人机验证时写进签到记录的原因。
	attendanceWidgetReason = "签到页要通过人机验证（Cloudflare Turnstile、reCAPTCHA 或 hCaptcha），AI 也过不了，请在网页上签到"
	// attendanceUnknownCaptchaReason 是签到表单有验证码输入框、却找不到 NexusPHP 验证码图片时的原因。
	attendanceUnknownCaptchaReason = "签到页的验证码 pt-tools 认不出来（表单里没有 NexusPHP 的验证码图片），请在网页上签到"
	// attendanceImageCaptchaReason 是签到表单要填图片验证码、而这次没有 AI 时的原因（怎么打开 AI 由签到监控补上）。
	attendanceImageCaptchaReason = "签到页要输入验证码"
	// nexusPHPCaptchaLength 是 NexusPHP 图片验证码的字数（random_str 默认 6 位）。
	nexusPHPCaptchaLength = 6
)

// attendanceCaptcha 是签到表单里的验证码。
type attendanceCaptcha int

const (
	captchaNone attendanceCaptcha = iota
	// captchaImage 是 NexusPHP 的图片验证码：认图后填 imagestring，连同 imagehash 一起提交。
	captchaImage
	// captchaWidget 是 Turnstile、reCAPTCHA、hCaptcha 这类人机验证组件。
	captchaWidget
	// captchaUnknown 是有验证码输入框，但不是能认的 NexusPHP 图片验证码（没有图片或 imagehash）。
	captchaUnknown
)

// attendanceForm 是签到页上提交回本页的「立即签到」表单。
type attendanceForm struct {
	values  url.Values
	captcha attendanceCaptcha
	// imageHash 与 imageSrc 是图片验证码的 imagehash 与图片地址（captchaImage 时才有）
	imageHash string
	imageSrc  string
}

// findAttendanceForm 在签到页里找提交回本页（pagePath）的 POST 表单：只有隐藏字段和提交按钮，或者另外只有验证码。
// 表单里还要填别的（文本框、单选、下拉等）时不算，交给调用方按「认不出」处理。
func findAttendanceForm(doc *goquery.Document, pagePath string) (attendanceForm, bool) {
	if doc == nil {
		return attendanceForm{}, false
	}
	var out attendanceForm
	found := false
	doc.Find("form").EachWithBreak(func(_ int, f *goquery.Selection) bool {
		if !strings.EqualFold(strings.TrimSpace(f.AttrOr("method", "")), http.MethodPost) || !postsToPage(f.AttrOr("action", ""), pagePath) {
			return true
		}
		if f.Find("input[type='submit'], input[type='image'], button").Length() == 0 {
			return true
		}
		form := attendanceForm{values: url.Values{}}
		fillable := true
		f.Find("input, select, textarea").Each(func(_ int, in *goquery.Selection) {
			name := strings.TrimSpace(in.AttrOr("name", ""))
			switch typ := strings.ToLower(strings.TrimSpace(in.AttrOr("type", "text"))); {
			case goquery.NodeName(in) != "input":
				// 人机验证组件自己塞进表单的回执（g-recaptcha-response 等）不算要填的东西，交给 classifyCaptcha
				if name != "" && !isWidgetResponseField(name) {
					fillable = false
				}
			case typ == "hidden":
				if name != "" && name != "imagehash" && !isWidgetResponseField(name) {
					form.values.Add(name, in.AttrOr("value", ""))
				}
			case typ == "submit" || typ == "image" || typ == "button" || typ == "reset":
				// 浏览器只带上点的那个按钮；页面上通常只有一个，有名字时照样带上
				if typ == "submit" && name != "" && !form.values.Has(name) {
					form.values.Set(name, in.AttrOr("value", ""))
				}
			case name == "imagestring":
				// 验证码输入框，见 classifyCaptcha
			case name != "":
				fillable = false
			}
		})
		if !fillable {
			return true
		}
		form.captcha, form.imageHash, form.imageSrc = classifyCaptcha(f)
		out, found = form, true
		return false
	})
	return out, found
}

// isWidgetResponseField 报告字段是不是人机验证组件放回执的字段：这些由组件在浏览器里填，pt-tools 填不了。
func isWidgetResponseField(name string) bool {
	switch name {
	case "cf-turnstile-response", "g-recaptcha-response", "h-captcha-response":
		return true
	}
	return false
}

// classifyCaptcha 认出表单里的验证码：人机验证组件优先（有它就过不了）；
// imagestring 输入框、imagehash 与 image.php?action=regimage 的图片都在时是 NexusPHP 图片验证码；只缺一样也算有验证码，但认不了。
func classifyCaptcha(f *goquery.Selection) (attendanceCaptcha, string, string) {
	if f.Find(".cf-turnstile, .g-recaptcha, .h-captcha").Length() > 0 {
		return captchaWidget, "", ""
	}
	input := f.Find("input[name='imagestring']")
	hash := strings.TrimSpace(f.Find("input[name='imagehash']").AttrOr("value", ""))
	src := strings.TrimSpace(f.Find("img[src*='action=regimage']").AttrOr("src", ""))
	switch {
	case input.Length() > 0 && hash != "" && src != "":
		return captchaImage, hash, src
	case input.Length() > 0 || hash != "" || src != "":
		return captchaUnknown, "", ""
	}
	return captchaNone, "", ""
}

// postsToPage 报告表单的 action 是不是提交回签到页本身：空的 action、相对或以 / 开头的同一路径都算，查询参数不同或指向别的主机不算。
func postsToPage(action, pagePath string) bool {
	target, err := url.Parse(strings.TrimSpace(action))
	if err != nil || target.Host != "" || target.Scheme != "" || target.RawQuery != "" {
		return false
	}
	base := &url.URL{Path: pagePath}
	return base.ResolveReference(target).Path == pagePath
}

// submitAttendanceForm 提交签到页上的「立即签到」表单：没有验证码时提交一次；NexusPHP 的图片验证码交给 AI 认，
// 认错了重新打开签到页换一张再认，最多 captchaTries 次；人机验证组件过不了，直接报 ErrAttendanceCaptchaRequired。
func (d *NexusPHPDriver) submitAttendanceForm(ctx context.Context, cfg *AttendanceConfig, pagePath string, form attendanceForm) (AttendResult, error) {
	switch form.captcha {
	case captchaNone:
		return d.postAttendanceForm(ctx, cfg, pagePath, form.values)
	case captchaImage:
		return d.attendFormWithImageCaptcha(ctx, cfg, pagePath, form)
	case captchaWidget:
		return AttendResult{}, fmt.Errorf("%w: %s", ErrAttendanceCaptchaRequired, attendanceWidgetReason)
	default:
		return AttendResult{}, fmt.Errorf("%w: %s", ErrAttendanceCaptchaRequired, attendanceUnknownCaptchaReason)
	}
}

// postAttendanceForm 照按钮的做法 POST 回签到页（Referer 是签到页），按提交后的页面认结果。
func (d *NexusPHPDriver) postAttendanceForm(ctx context.Context, cfg *AttendanceConfig, pagePath string, values url.Values) (AttendResult, error) {
	res, err := d.Execute(ctx, NexusPHPRequest{Path: pagePath, Method: http.MethodPost, Params: values, Referer: pagePath})
	if err != nil {
		return AttendResult{}, attendanceRequestError(err, pagePath)
	}
	if result, ok := cfg.Match(attendancePageText(res.RawBody)); ok {
		return result, nil
	}
	return AttendResult{}, unrecognizedAttendance(&res, "提交签到表单后")
}

// invalidCaptchaPattern 是验证码不对时站点回的话：NexusPHP 的「图片代码无效」（简体、繁体、英文），以及常见的改写。
var invalidCaptchaPattern = regexp.MustCompile(`图片代码无效|圖片代碼無效|(?i:invalid image code)|验证码(?:错误|不正确|无效)|驗證碼(?:錯誤|不正確|無效)`)

// attendFormWithImageCaptcha 填 NexusPHP 的图片验证码签到：取图、让 AI 认、连同 imagehash 提交。
// 站点核对后就删掉这个 imagehash，认错了要重新打开签到页拿一张新的。
func (d *NexusPHPDriver) attendFormWithImageCaptcha(ctx context.Context, cfg *AttendanceConfig, pagePath string, form attendanceForm) (AttendResult, error) {
	solver := AttendanceSolverFrom(ctx)
	if solver == nil {
		return AttendResult{}, fmt.Errorf("%w: %s", ErrAttendanceNeedsAI, attendanceImageCaptchaReason)
	}
	reason := "验证码一直没认对"
	for try := range captchaTries {
		if try > 0 {
			page, err := d.Execute(ctx, NexusPHPRequest{Path: pagePath})
			if err != nil {
				return AttendResult{}, attendanceRequestError(err, pagePath)
			}
			if result, ok := cfg.Match(attendancePageText(page.RawBody)); ok {
				return result, nil
			}
			next, ok := findAttendanceForm(page.Document, pagePath)
			if !ok || next.captcha != captchaImage {
				return AttendResult{}, unrecognizedAttendance(&page, "重新打开签到页后")
			}
			form = next
		}
		imagePath, ok := d.captchaImagePath(pagePath, form.imageSrc)
		if !ok {
			return AttendResult{}, fmt.Errorf("%w: %s", ErrAttendanceCaptchaRequired, attendanceUnknownCaptchaReason)
		}
		img, err := d.fetchCaptcha(ctx, imagePath, pagePath, nexusPHPCaptchaLength)
		if err != nil {
			return AttendResult{}, err
		}
		code, err := readCaptcha(ctx, solver, img)
		if err != nil {
			return AttendResult{}, fmt.Errorf("AI 认验证码: %w", err)
		}
		if code == "" {
			reason = fmt.Sprintf("AI 认出的验证码不是 %d 位", nexusPHPCaptchaLength)
			continue
		}
		values := url.Values{}
		for k, v := range form.values {
			values[k] = append([]string(nil), v...)
		}
		values.Set("imagehash", form.imageHash)
		values.Set("imagestring", code)
		res, err := d.Execute(ctx, NexusPHPRequest{Path: pagePath, Method: http.MethodPost, Params: values, Referer: pagePath})
		if err != nil {
			return AttendResult{}, attendanceRequestError(err, pagePath)
		}
		text := attendancePageText(res.RawBody)
		if result, ok := cfg.Match(text); ok {
			return result, nil
		}
		if !invalidCaptchaPattern.MatchString(text) {
			return AttendResult{}, unrecognizedAttendance(&res, "提交签到表单后")
		}
		reason = "验证码没认对"
	}
	return AttendResult{}, fmt.Errorf("%w: 试了 %d 次，%s", ErrAttendanceChallengeFailed, captchaTries, reason)
}

// captchaImagePath 把验证码图片的地址换成本站的请求路径（带查询参数）：相对地址按签到页解析；
// 写了主机的只认本站，指向别的主机时不取（Cookie 不能带给别的站）。
func (d *NexusPHPDriver) captchaImagePath(pagePath, src string) (string, bool) {
	ref, err := url.Parse(src)
	if err != nil {
		return "", false
	}
	if ref.Host != "" || ref.Scheme != "" {
		base, err := url.Parse(d.BaseURL)
		if err != nil || !strings.EqualFold(ref.Host, base.Host) {
			return "", false
		}
	}
	resolved := (&url.URL{Path: pagePath}).ResolveReference(ref)
	path := resolved.EscapedPath()
	if !strings.HasPrefix(path, "/") {
		return "", false
	}
	if resolved.RawQuery != "" {
		path += "?" + resolved.RawQuery
	}
	return path, true
}

// AttendanceMayUseAI 报告签到时可能要用 AI：站点定义写明了要过的一关，
// 或者走 NexusPHP 默认的 GET 签到——新版签到页的表单可能带图片验证码，打开签到页才知道。
// 签到监控按它决定要不要备好 AI。
func AttendanceMayUseAI(def *SiteDefinition) bool {
	if AttendanceChallengeOf(def) != "" {
		return true
	}
	if def == nil || def.Schema != SchemaNexusPHP {
		return false
	}
	cfg := ResolveAttendanceConfig(def)
	return cfg != nil && cfg.Unsupported == "" && strings.EqualFold(strings.TrimSpace(cfg.Method), http.MethodGet)
}

// unrecognizedAttendance 是页面里认不出签到结果时的错误：带上页面标题（登录页、Cloudflare 质询页、签到页一看就分得出），
// 下次再遇到能知道站点回的是什么页面。after 不为空时写在前面，说明是哪一步之后（如「提交签到表单后」）。
func unrecognizedAttendance(res *NexusPHPResponse, after string) error {
	msg := after + "页面里没有签到成功或今日已签到的文字"
	if title := attendancePageTitle(res); title != "" {
		msg += fmt.Sprintf("（页面标题「%s」）", title)
	}
	return fmt.Errorf("%w: %s", ErrAttendanceUnrecognized, msg)
}

// attendancePageTitle 取页面的 <title>，空白合并，最多 60 字。
func attendancePageTitle(res *NexusPHPResponse) string {
	if res == nil || res.Document == nil {
		return ""
	}
	title := collapseSpaces(res.Document.Find("title").First().Text())
	const maxRunes = 60
	if utf8.RuneCountInString(title) > maxRunes {
		title = string([]rune(title)[:maxRunes]) + "…"
	}
	return title
}
