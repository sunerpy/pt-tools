package telegram

import "strings"

// markdownV2Specials lists characters Telegram Bot API requires escaping in
// MarkdownV2 messages. Reference:
// https://core.telegram.org/bots/api#markdownv2-style
var markdownV2Specials = []string{
	"_", "*", "[", "]", "(", ")", "~", "`", ">", "#", "+",
	"-", "=", "|", "{", "}", ".", "!", "\\",
}

// EscapeMarkdownV2 escapes Telegram MarkdownV2 reserved characters. Not used
// by the default Send path on purpose: torrent titles routinely contain `_`,
// `[`, `]`, `*` etc. and accidental escaping disfigures filenames.
func EscapeMarkdownV2(s string) string {
	for _, ch := range markdownV2Specials {
		s = strings.ReplaceAll(s, ch, "\\"+ch)
	}
	return s
}
