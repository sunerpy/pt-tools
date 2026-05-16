//go:build qa

package cmd

import (
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/web"
)

func wireQATestHooks(srv *web.Server, bs *chatopsBootstrap) {
	if srv == nil {
		return
	}
	if bs == nil || bs.Chain() == nil {
		global.GetSlogger().Warn("[QA] chatops chain unavailable; QA test hooks NOT wired")
		return
	}
	web.SetQAInboundProcessor(bs.Chain())
	srv.SetQAHook(web.RegisterQATestHooks)
	global.GetSlogger().Warn("[QA] test hook endpoints /test/telegram/inject and /test/qq/inject ENABLED (qa build only)")
}
