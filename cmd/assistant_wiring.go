package cmd

import (
	"context"
	"errors"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/assistant"
	chatopscmds "github.com/sunerpy/pt-tools/internal/chatops/commands"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/version"
	"github.com/sunerpy/pt-tools/web"
)

// newAssistantService 构造内置 AI 助手（M19）：API Key 与代理地址用 ConfigStore 的 Cookie 密钥加解密，
// 工具是 web 的 AssistantToolServer（和 /mcp 同一套，经 App API 执行）。Start 失败只记日志，助手照样可用。
func newAssistantService(ctx context.Context, store *core.ConfigStore, srv *web.Server) *assistant.Service {
	if global.GlobalDB == nil || store == nil || srv == nil {
		return nil
	}
	svc := assistant.New(assistant.Config{
		DB: global.GlobalDB.DB, Cipher: storeCipher{store: store}, Tools: srv.AssistantToolServer,
		ResolveURL: v2.NewTrackerResolver().ResolveDownloadURL, Version: version.GetVersionInfo().Version,
	})
	if err := svc.Start(ctx); err != nil {
		global.GetSlogger().Warnf("[AI 助手] 启动失败: %v", err)
	}
	return svc
}

// wireAssistantChatOps 把通知渠道里绑定用户的非命令消息交给助手，并接上 /newchat。回复经运行中的通道异步发出。
// 助手或 ChatOps 没起来时什么也不做（非命令消息照旧回「只识别命令」）。
func wireAssistantChatOps(svc *assistant.Service, bs *chatopsBootstrap) {
	if svc == nil || bs == nil || bs.chain == nil || bs.manager == nil {
		return
	}
	h := assistant.NewChatOpsHandler(svc, bs.manager)
	h.SetTemplates(bs.manager.templates)
	bs.chain.SetAssistant(h)
	chatopscmds.SetAssistant(h)
}

// wireSubscribeReview 把 AI 助手接到订阅的下载前复核（媒体中心 M9b）：订阅设置里打开了才问，助手没启用时复核没做成、照常推送。
func wireSubscribeReview(sub *subscribe.Service, svc *assistant.Service) {
	if sub == nil || svc == nil {
		return
	}
	sub.SetReviewer(subscribeReviewer(svc))
}

func subscribeReviewer(svc *assistant.Service) subscribe.Reviewer {
	return func(ctx context.Context, in subscribe.ReviewInput) (subscribe.ReviewResult, error) {
		v, err := svc.ReviewCandidate(ctx, assistant.CandidateReview{
			Kind: in.Kind, Title: in.Title, OriginalTitle: in.OriginalTitle, Year: in.Year, Season: in.Season, Aliases: in.Aliases,
			TorrentTitle: in.TorrentTitle, TorrentSubtitle: in.TorrentSubtitle, Parsed: in.Parsed,
		})
		switch {
		case errors.Is(err, assistant.ErrDisabled):
			return subscribe.ReviewResult{}, errors.New("AI 助手没有启用或没有配置好（在「AI 助手设置」里配好并启用）")
		case errors.Is(err, assistant.ErrReviewUnclear):
			return subscribe.ReviewResult{}, subscribe.ErrReviewUnclear
		case err != nil:
			return subscribe.ReviewResult{}, err
		}
		return subscribe.ReviewResult{Match: v.Match, Reason: v.Reason, Tokens: v.Usage.PromptTokens + v.Usage.CompletionTokens}, nil
	}
}
