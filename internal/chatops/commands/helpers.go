package commands

import (
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

func tr(lang, zh, en string) string {
	if strings.EqualFold(lang, "en") {
		return en
	}
	return zh
}

func errReply(lang, zh, en string, args ...any) chatops.Reply {
	return chatops.Reply{Text: fmt.Sprintf(tr(lang, zh, en), args...)}
}

func okReply(text string) chatops.Reply {
	return chatops.Reply{Text: text}
}

func formatBytes(n int64) string {
	const unit = int64(1024)
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := unit, 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	suffix := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}[exp]
	return fmt.Sprintf("%.2f %s", float64(n)/float64(div), suffix)
}

func wrapMono(s string) string {
	return "```\n" + s + "\n```"
}

func parseDownloaderArg(args []string, idx int) string {
	if len(args) > idx {
		return args[idx]
	}
	return ""
}
