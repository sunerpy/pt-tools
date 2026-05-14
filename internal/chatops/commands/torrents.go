package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "torrents",
		Description: "List torrents in a downloader",
		Handler:     torrentsHandler,
		RateLimit:   &chatops.RateLimitSpec{Per: 10 * time.Second, Burst: 1},
	})
}

const torrentsPageSize = 20

func torrentsHandler(ctx context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
	svc := getServices()
	if svc == nil || svc.Torrent == nil {
		return errReply(src.ReplyLang, "种子服务不可用", "torrent service unavailable"), nil
	}
	dlName := parseDownloaderArg(args, 0)
	if dlName == "" {
		return errReply(src.ReplyLang, "用法: /torrents <下载器名>", "usage: /torrents <downloader>"), nil
	}
	items, total, err := svc.Torrent.ListByDownloader(ctx, dlName, 1, torrentsPageSize)
	if err != nil {
		return errReply(src.ReplyLang, "查询种子失败: %v", "list torrents failed: %v", err), nil
	}
	if len(items) == 0 {
		return okReply(tr(src.ReplyLang, "无种子", "no torrents")), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, tr(src.ReplyLang, "%s 种子（%d 总计，显示前 %d）\n", "%s torrents (%d total, showing first %d)\n"),
		dlName, total, len(items))
	for _, it := range items {
		fmt.Fprintf(&b, "[%s] %s %.1f%% %s\n", it.State, truncate(it.Name, 50), it.Progress*100, formatBytes(it.Size))
	}
	return chatops.Reply{Text: wrapMono(strings.TrimRight(b.String(), "\n"))}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
