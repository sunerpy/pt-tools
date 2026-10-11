// Package redact 遮住要交给外部的文本里可能带的凭证：App API 的错误信息、AI 助手存下来与发给各入口的会话内容。
package redact

import (
	"regexp"

	"github.com/sunerpy/pt-tools/internal/dlassistant"
)

var (
	// urlQueryRe 是文本里带查询串的地址（不分大小写，含 udp）：passkey、签名之类多在查询串里，整段去掉。
	urlQueryRe = regexp.MustCompile(`(?i)\b((?:https?|udp)://[^\s?#"'<>]+)\?[^\s"'<>]*`)
	// urlUserinfoRe 是地址里的用户名与密码。
	urlUserinfoRe = regexp.MustCompile(`(?i)\b((?:https?|udp)://)[^/\s@"'<>]+@`)
	// jsonSecretRe 是上游回应体里 JSON 形式的凭证字段。
	jsonSecretRe = regexp.MustCompile(`(?i)"(passkey|authkey|torrent_pass|credential|token|apikey|api_key|sign|secret|rsskey|key|password|passwd|cookie)"\s*:\s*"[^"]*"`)
	// plainSecretRe 补上 RedactTrackerMessage 不管的 password=、cookie=。
	plainSecretRe = regexp.MustCompile(`(?i)\b(password|passwd|pwd|cookie)=([^&\s"'<>(),;]+)`)
)

// Credentials 去掉地址的用户信息与整段查询串，遮 JSON 与 key=value 形式的凭证字段。不碰别的长串：
// 种子的 info hash 之类的编号原样留着（AI 助手存下来的工具结果要靠它们接着对话）。
func Credentials(s string) string {
	s = urlUserinfoRe.ReplaceAllString(s, "$1***@")
	s = urlQueryRe.ReplaceAllString(s, "$1?…")
	s = jsonSecretRe.ReplaceAllString(s, `"$1":"***"`)
	return plainSecretRe.ReplaceAllString(s, "$1=***")
}

// Secrets 在 Credentials 之外，再按下载器助手遮 tracker 回复的规则（RedactTrackerMessage）遮地址路径里的长串、
// passkey=… 一类参数与 24 位以上的长串。错误信息与用户、模型写的文字用它。
func Secrets(s string) string {
	return dlassistant.RedactTrackerMessage(Credentials(s))
}
