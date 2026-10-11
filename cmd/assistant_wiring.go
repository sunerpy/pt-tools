package cmd

import (
	"context"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/assistant"
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
