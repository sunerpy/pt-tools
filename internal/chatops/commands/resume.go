package commands

import (
	"context"
	"errors"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "resume",
		Description: "Resume a torrent",
		AdminOnly:   true,
		Handler:     resumeHandler,
	})
}

func resumeHandler(ctx context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
	if len(args) < 1 {
		return errReply(src.ReplyLang, "用法: /resume <种子ID> [下载器]", "usage: /resume <torrent_id> [downloader]"), nil
	}
	svc := getServices()
	if svc == nil || svc.Torrent == nil {
		return errReply(src.ReplyLang, "种子服务不可用", "torrent service unavailable"), nil
	}
	torrentID := args[0]
	dlName := parseDownloaderArg(args, 1)
	if err := svc.Torrent.Resume(ctx, dlName, torrentID); err != nil {
		if errors.Is(err, app.ErrTorrentNotFound) {
			return errReply(src.ReplyLang, "种子不存在: %s", "torrent not found: %s", torrentID), nil
		}
		if errors.Is(err, app.ErrDownloaderNotFound) {
			return errReply(src.ReplyLang, "下载器不存在: %s", "downloader not found: %s", dlName), nil
		}
		return errReply(src.ReplyLang, "恢复失败: %v", "resume failed: %v", err), nil
	}
	return okReply(tr(src.ReplyLang, "已恢复 "+torrentID, "resumed "+torrentID)), nil
}
