package v2

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// AttendanceConfig 描述站点的每日签到：GET 请求 Path，按返回页面的文字判断结果。
// NexusPHP 站点不配置时使用 DefaultNexusPHPAttendance；没有签到、签到要验证码或答题的站点
// 设置 Unsupported，值是界面上显示的原因。
type AttendanceConfig struct {
	// Path 是签到请求的路径，可以带查询参数（如 /attendance-ajax.php?act=sign）。
	Path string `json:"path,omitempty"`
	// SuccessPatterns 匹配「本次签到成功」的页面文字（正则）；为空时用 NexusPHP 的默认规则。
	SuccessPatterns []string `json:"successPatterns,omitempty"`
	// AlreadyPatterns 匹配「今天已经签到过」的页面文字（正则）；为空时用 NexusPHP 的默认规则。
	AlreadyPatterns []string `json:"alreadyPatterns,omitempty"`
	// Unsupported 非空表示该站不支持自动签到，值是原因。
	Unsupported string `json:"unsupported,omitempty"`
}

const defaultAttendancePath = "/attendance.php"

// NexusPHP 签到插件的页面文字。信息量大的规则排在前面，摘取的提示语更有用。
var (
	nexusPHPAttendanceSuccess = []string{
		`这是您的第\s*\d+\s*次签到`,
		`本次签到获得`,
		`签到已成功`,
		`签到成功`,
		`(?i)attendance successful`,
	}
	nexusPHPAttendanceAlready = []string{
		`今天已经签到过`,
		`今日已签到`,
		`今天已签到`,
		`(?i)already attended`,
	}
)

// DefaultNexusPHPAttendance 返回 NexusPHP 签到插件的默认配置：GET /attendance.php。
func DefaultNexusPHPAttendance() *AttendanceConfig {
	return &AttendanceConfig{
		Path:            defaultAttendancePath,
		SuccessPatterns: append([]string(nil), nexusPHPAttendanceSuccess...),
		AlreadyPatterns: append([]string(nil), nexusPHPAttendanceAlready...),
	}
}

// ResolveAttendanceConfig 返回站点生效的签到配置：以站点定义里的配置为准，缺的字段用 NexusPHP 的默认值补齐；
// 不是 NexusPHP 架构又没有配置的站点返回 nil，表示不支持。
func ResolveAttendanceConfig(def *SiteDefinition) *AttendanceConfig {
	if def == nil {
		return nil
	}
	if def.Attendance == nil {
		if def.Schema != SchemaNexusPHP {
			return nil
		}
		return DefaultNexusPHPAttendance()
	}
	cfg := *def.Attendance
	if cfg.Unsupported != "" {
		return &cfg
	}
	if cfg.Path == "" {
		cfg.Path = defaultAttendancePath
	}
	if len(cfg.SuccessPatterns) == 0 {
		cfg.SuccessPatterns = append([]string(nil), nexusPHPAttendanceSuccess...)
	}
	if len(cfg.AlreadyPatterns) == 0 {
		cfg.AlreadyPatterns = append([]string(nil), nexusPHPAttendanceAlready...)
	}
	return &cfg
}

// AttendanceSupport 只看站点定义判断能否自动签到，不支持时返回原因，供界面在不创建站点实例时展示。
func AttendanceSupport(def *SiteDefinition) (bool, string) {
	if def == nil {
		return false, "站点没有内置定义"
	}
	cfg := ResolveAttendanceConfig(def)
	switch {
	case cfg == nil:
		return false, "该站点的架构暂不支持签到"
	case cfg.Unsupported != "":
		return false, cfg.Unsupported
	}
	return true, ""
}

// AttendStatus 是一次签到的结果。
type AttendStatus string

const (
	// AttendSigned 表示本次签到成功。
	AttendSigned AttendStatus = "signed"
	// AttendAlready 表示今天已经签到过。
	AttendAlready AttendStatus = "already"
)

// AttendResult 是一次签到的结果与页面上的一句提示。
type AttendResult struct {
	Status  AttendStatus
	Message string
}

// Attender 由能签到的驱动实现。
type Attender interface {
	Attend(ctx context.Context) (AttendResult, error)
}

// AttendanceCapable 由 BaseSite 实现：SupportsAttendance 报告能否签到，Attend 先经过本站的限速器再签到。
type AttendanceCapable interface {
	SupportsAttendance() bool
	Attend(ctx context.Context) (AttendResult, error)
}

// attendanceSupportReporter 由驱动可选实现，报告站点定义是否把签到标记为不支持。
type attendanceSupportReporter interface {
	AttendanceUnsupportedReason() string
}

var (
	// ErrAttendanceUnsupported 表示站点不支持自动签到（架构不支持，或站点定义标记为不支持）。
	ErrAttendanceUnsupported = errors.New("attendance not supported")
	// ErrAttendanceUnrecognized 表示签到请求成功返回，但页面里既没有签到成功也没有今日已签的文字。
	ErrAttendanceUnrecognized = errors.New("attendance result not recognized")
)

// Match 在页面文字里找签到结果：先找签到成功，再找今日已签。
func (c *AttendanceConfig) Match(text string) (AttendResult, bool) {
	if c == nil {
		return AttendResult{}, false
	}
	text = collapseSpaces(text)
	if loc := firstPatternMatch(c.SuccessPatterns, text); loc != nil {
		return AttendResult{Status: AttendSigned, Message: attendanceSnippet(text, loc)}, true
	}
	if loc := firstPatternMatch(c.AlreadyPatterns, text); loc != nil {
		return AttendResult{Status: AttendAlready, Message: attendanceSnippet(text, loc)}, true
	}
	return AttendResult{}, false
}

var attendancePatternCache sync.Map

func firstPatternMatch(patterns []string, text string) []int {
	for _, p := range patterns {
		re, ok := attendancePatternCache.Load(p)
		if !ok {
			compiled, err := regexp.Compile(p)
			if err != nil {
				continue
			}
			re, _ = attendancePatternCache.LoadOrStore(p, compiled)
		}
		if loc := re.(*regexp.Regexp).FindStringIndex(text); loc != nil {
			return loc
		}
	}
	return nil
}

func collapseSpaces(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

var (
	attendanceScriptPattern = regexp.MustCompile(`(?is)<(script|style)\b.*?</(script|style)>`)
	attendanceTagPattern    = regexp.MustCompile(`<[^>]*>`)
)

// attendancePageText 把签到页转成纯文本：去掉脚本和样式，标签换成空格，再还原实体。
// 标签换空格而不是直接删掉，相邻元素的文字（如「签到成功」标题和下一段）才不会粘成一句。
// JSON 返回里的 HTML 片段同样适用。
func attendancePageText(body []byte) string {
	text := attendanceScriptPattern.ReplaceAllString(string(body), " ")
	text = attendanceTagPattern.ReplaceAllString(text, " ")
	return collapseSpaces(html.UnescapeString(text))
}

// attendanceSnippet 取匹配所在的那句话：向前补到空白或句读为止（最多 20 字），向后取到句号为止，总长不超过 120 字。
func attendanceSnippet(text string, loc []int) string {
	const maxBack, maxLen = 20, 120
	runes := []rune(text)
	start := len([]rune(text[:loc[0]]))
	end := len([]rune(text[:loc[1]]))
	for back := 0; start > 0 && back < maxBack; back++ {
		r := runes[start-1]
		if unicode.IsSpace(r) || strings.ContainsRune("。！？!?，,；;：:", r) {
			break
		}
		start--
	}
	for end < len(runes) && end-start < maxLen {
		r := runes[end]
		end++
		if strings.ContainsRune("。！？!?\n", r) || (r == '.' && end-start > 1) {
			break
		}
	}
	return strings.TrimSpace(string(runes[start:end]))
}

func attendanceRequest(path string) (NexusPHPRequest, error) {
	u, err := url.Parse(path)
	if err != nil {
		return NexusPHPRequest{}, fmt.Errorf("invalid attendance path %q: %w", path, err)
	}
	req := NexusPHPRequest{Path: u.Path}
	if q := u.Query(); len(q) > 0 {
		req.Params = q
	}
	return req, nil
}

// Attend 按站点的签到配置发一次 GET 请求，按页面文字判断结果。请求经驱动的 Execute，
// 会话失效、权限错误等与其他页面请求的判断一致。
func (d *NexusPHPDriver) Attend(ctx context.Context) (AttendResult, error) {
	cfg := ResolveAttendanceConfig(d.attendanceDefinition())
	if cfg == nil {
		return AttendResult{}, ErrAttendanceUnsupported
	}
	if cfg.Unsupported != "" {
		return AttendResult{}, fmt.Errorf("%w: %s", ErrAttendanceUnsupported, cfg.Unsupported)
	}
	req, err := attendanceRequest(cfg.Path)
	if err != nil {
		return AttendResult{}, err
	}
	res, err := d.Execute(ctx, req)
	if err != nil {
		return AttendResult{}, fmt.Errorf("attendance request: %w", err)
	}
	if result, ok := cfg.Match(attendancePageText(res.RawBody)); ok {
		return result, nil
	}
	return AttendResult{}, fmt.Errorf("%w: 页面里没有签到成功或今日已签到的文字", ErrAttendanceUnrecognized)
}

// AttendanceUnsupportedReason 报告站点定义是否把签到标记为不支持，支持时返回空串。
func (d *NexusPHPDriver) AttendanceUnsupportedReason() string {
	cfg := ResolveAttendanceConfig(d.attendanceDefinition())
	if cfg == nil {
		return "该站点的架构暂不支持签到"
	}
	return cfg.Unsupported
}

func (d *NexusPHPDriver) attendanceDefinition() *SiteDefinition {
	if d.siteDefinition != nil {
		return d.siteDefinition
	}
	return &SiteDefinition{Schema: SchemaNexusPHP}
}

// SupportsAttendance 报告站点能否自动签到：驱动实现了签到，且站点定义没有标记为不支持。
func (b *BaseSite[Req, Res]) SupportsAttendance() bool {
	if _, ok := any(b.driver).(Attender); !ok {
		return false
	}
	if r, ok := any(b.driver).(attendanceSupportReporter); ok && r.AttendanceUnsupportedReason() != "" {
		return false
	}
	return true
}

// Attend 签到一次。与 GetUserInfo 一样先经过本站的限速器——不能像 GetDetailFetcher 那样把驱动直接交出去。
func (b *BaseSite[Req, Res]) Attend(ctx context.Context) (AttendResult, error) {
	a, ok := any(b.driver).(Attender)
	if !ok {
		return AttendResult{}, ErrAttendanceUnsupported
	}
	if r, ok := any(b.driver).(attendanceSupportReporter); ok {
		if reason := r.AttendanceUnsupportedReason(); reason != "" {
			return AttendResult{}, fmt.Errorf("%w: %s", ErrAttendanceUnsupported, reason)
		}
	}
	if err := b.limiter.Wait(ctx); err != nil {
		return AttendResult{}, fmt.Errorf("rate limit: %w", err)
	}
	return a.Attend(ctx)
}
