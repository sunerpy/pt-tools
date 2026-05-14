package commands

import (
	"context"
	"errors"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "pause",
		Description: "Pause a torrent",
		AdminOnly:   true,
		Handler:     pauseHandler,
	})
}

func pauseHandler(ctx context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
	if len(args) < 1 {
		return errReply(src.ReplyLang, "用法: /pause <种子ID> [下载器]", "usage: /pause <torrent_id> [downloader]"), nil
	}
	svc := getServices()
	if svc == nil || svc.Torrent == nil {
		return errReply(src.ReplyLang, "种子服务不可用", "torrent service unavailable"), nil
	}
	torrentID := args[0]
	dlName := parseDownloaderArg(args, 1)
	if err := svc.Torrent.Pause(ctx, dlName, torrentID); err != nil {
		if errors.Is(err, app.ErrTorrentNotFound) {
			return errReply(src.ReplyLang, "种子不存在: %s", "torrent not found: %s", torrentID), nil
		}
		if errors.Is(err, app.ErrDownloaderNotFound) {
			return errReply(src.ReplyLang, "下载器不存在: %s", "downloader not found: %s", dlName), nil
		}
		return errReply(src.ReplyLang, "暂停失败: %v", "pause failed: %v", err), nil
	}
	return okReply(tr(src.ReplyLang, "已暂停 "+torrentID, "paused "+torrentID)), nil
}
