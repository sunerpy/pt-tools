package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "torrents",
		Description: "列出下载器中的种子 (List torrents in a downloader)",
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
	list := make([]map[string]any, 0, len(items))
	for _, it := range items {
		list = append(list, map[string]any{
			"name": it.Name, "state": it.State, "progress": it.Progress * 100, "size": formatBytes(it.Size), "size_bytes": it.Size,
		})
	}
	fields := map[string]any{"downloader": dlName, "total": total, "items": list, "items_more": max(total-len(items), 0)}
	if len(items) == 0 {
		return eventReply(tr(src.ReplyLang, "无种子", "no torrents"), notify.EventReplyTorrents, fields), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, tr(src.ReplyLang, "%s 种子（%d 总计，显示前 %d）\n", "%s torrents (%d total, showing first %d)\n"),
		dlName, total, len(items))
	for _, it := range items {
		fmt.Fprintf(&b, "[%s] %s %.1f%% %s\n", it.State, truncate(it.Name, 50), it.Progress*100, formatBytes(it.Size))
	}
	return eventReply(wrapMono(strings.TrimRight(b.String(), "\n")), notify.EventReplyTorrents, fields), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
