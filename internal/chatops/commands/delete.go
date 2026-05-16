package commands

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
)

const deleteConfirmTTL = 5 * time.Minute

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "delete",
		Description: "删除种子（需 YES 确认） (Delete a torrent, requires YES confirm)",
		AdminOnly:   true,
		Handler:     deleteHandler,
	})
}

func deleteHandler(_ context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
	if len(args) < 1 {
		return errReply(src.ReplyLang,
			"用法: /delete <种子ID> [--with-data] [下载器]",
			"usage: /delete <torrent_id> [--with-data] [downloader]"), nil
	}
	torrentID := args[0]
	withData := false
	dlName := ""
	for _, a := range args[1:] {
		if a == "--with-data" {
			withData = true
			continue
		}
		if dlName == "" {
			dlName = a
		}
	}

	svc := getServices()
	if svc == nil || svc.Sessions == nil {
		return errReply(src.ReplyLang, "会话存储未初始化", "session store not initialized"), nil
	}

	confirmHandler := func(ctx context.Context, fargs []string, fsrc chatops.Source) (chatops.Reply, error) {
		text := ""
		if len(fargs) > 0 {
			text = strings.TrimSpace(fargs[0])
		}
		if !strings.EqualFold(text, "YES") {
			return okReply(tr(fsrc.ReplyLang, "已取消删除", "delete canceled")), nil
		}
		s := getServices()
		if s == nil || s.Torrent == nil {
			return errReply(fsrc.ReplyLang, "种子服务不可用", "torrent service unavailable"), nil
		}
		if err := s.Torrent.Delete(ctx, dlName, torrentID, withData); err != nil {
			if errors.Is(err, app.ErrTorrentNotFound) {
				return errReply(fsrc.ReplyLang, "种子不存在: %s", "torrent not found: %s", torrentID), nil
			}
			if errors.Is(err, app.ErrDownloaderNotFound) {
				return errReply(fsrc.ReplyLang, "下载器不存在: %s", "downloader not found: %s", dlName), nil
			}
			return errReply(fsrc.ReplyLang, "删除失败: %v", "delete failed: %v", err), nil
		}
		return okReply(tr(fsrc.ReplyLang, "已删除 "+torrentID, "deleted "+torrentID)), nil
	}

	svc.Sessions.Set(src.ChannelType, src.ChannelConfID, src.ChannelUserID, chatops.SessionState{
		Step:    "confirm_delete",
		Data:    torrentID,
		Handler: confirmHandler,
	}, deleteConfirmTTL)

	zh := "确认删除 " + torrentID
	if withData {
		zh += "（含数据）"
	}
	zh += "? 回复 YES 在 5 分钟内确认"
	en := "Confirm delete " + torrentID
	if withData {
		en += " (with data)"
	}
	en += "? Reply YES within 5 minutes"
	return chatops.Reply{Text: tr(src.ReplyLang, zh, en)}, nil
}
