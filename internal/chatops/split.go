package chatops

import "strings"

// 一条 ChatOps 回复的字数上限（Telegram 一条消息最多 4096 个字符）；更长的回复分几条发，最多 ReplyParts 条。
const (
	ReplyRunes = 3500
	ReplyParts = 4
)

// SplitReply 把太长的回复分成几条（每条不超过 ReplyRunes 个字，尽量在换行处分），最多 ReplyParts 条，
// 再多的省略并注明（注明的话也算在字数里）。lang 以 en 开头时注明用英文。
func SplitReply(s, lang string) []string {
	var parts []string
	rest := []rune(s)
	for len(rest) > ReplyRunes && len(parts) < ReplyParts-1 {
		cut := ReplyRunes
		for i := ReplyRunes; i > ReplyRunes/2; i-- {
			if rest[i-1] == '\n' {
				cut = i
				break
			}
		}
		parts = append(parts, strings.TrimRight(string(rest[:cut]), "\n"))
		rest = rest[cut:]
	}
	if len(rest) > ReplyRunes {
		note := []rune("\n…（太长了，后面的省略了）")
		if strings.HasPrefix(strings.ToLower(lang), "en") {
			note = []rune("\n… (too long; the rest is left out)")
		}
		rest = append(rest[:ReplyRunes-len(note):ReplyRunes-len(note)], note...)
	}
	return append(parts, strings.TrimRight(string(rest), "\n"))
}
