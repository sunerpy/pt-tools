package cmd

import (
	"context"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/assistant"
	chatopscmds "github.com/sunerpy/pt-tools/internal/chatops/commands"
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
