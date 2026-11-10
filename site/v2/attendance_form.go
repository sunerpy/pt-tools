package v2

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/PuerkitoBio/goquery"
)

// 新版 NexusPHP（xiaomlove/nexusphp 2025-10-14 起，设置 captcha.attendance.enabled 默认打开）打开 attendance.php 不再直接签到，
// 只显示一个「立即签到」按钮：表单 POST 回 attendance.php，站点打开了图片验证码时表单里还有 imagehash 与 imagestring。
// 提交后成功页与旧版相同（「签到成功」「这是您的第 N 次签到」），已经签过时回「您今天已经签到过了」。

// ErrAttendanceCaptchaRequired 表示签到表单要填验证码：站点打开了签到验证码，pt-tools 还不会填，当天重试也签不上。
var ErrAttendanceCaptchaRequired = errors.New("attendance needs a captcha")

// attendanceCaptchaReason 是签到表单要验证码时写进签到记录的原因。
const attendanceCaptchaReason = "签到页要输入验证码（站点打开了签到验证码），pt-tools 暂不能自动签到，请在网页上签到"

// attendanceForm 是签到页上提交回本页的「立即签到」表单。
type attendanceForm struct {
	values  url.Values
	captcha bool
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
		form := attendanceForm{values: url.Values{}, captcha: hasCaptcha(f)}
		fillable := true
		f.Find("input, select, textarea").Each(func(_ int, in *goquery.Selection) {
			name := strings.TrimSpace(in.AttrOr("name", ""))
			switch typ := strings.ToLower(strings.TrimSpace(in.AttrOr("type", "text"))); {
			case goquery.NodeName(in) != "input":
				if name != "" {
					fillable = false
				}
			case typ == "hidden":
				if name != "" && name != "imagehash" {
					form.values.Add(name, in.AttrOr("value", ""))
				}
			case typ == "submit" || typ == "image" || typ == "button" || typ == "reset":
				// 浏览器只带上点的那个按钮；页面上通常只有一个，有名字时照样带上
				if typ == "submit" && name != "" && !form.values.Has(name) {
					form.values.Set(name, in.AttrOr("value", ""))
				}
			case name == "imagestring":
				// 验证码输入框，见 hasCaptcha
			case name != "":
				fillable = false
			}
		})
		if !fillable {
			return true
		}
		out, found = form, true
		return false
	})
	return out, found
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

// hasCaptcha 报告表单里有没有验证码：NexusPHP 的图片验证码（imagehash、imagestring、image.php?action=regimage），
// 或者 Cloudflare Turnstile、reCAPTCHA、hCaptcha 的组件。
func hasCaptcha(f *goquery.Selection) bool {
	return f.Find(strings.Join([]string{
		"input[name='imagehash']", "input[name='imagestring']", "img[src*='action=regimage']",
		".cf-turnstile", ".g-recaptcha", ".h-captcha",
	}, ", ")).Length() > 0
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
