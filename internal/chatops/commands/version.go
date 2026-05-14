package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/version"
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "version",
		Description: "Show version info",
		Aliases:     []string{"v", "ver"},
		Handler:     versionHandler,
	})
}

func versionHandler(_ context.Context, _ []string, src chatops.Source) (chatops.Reply, error) {
	info := version.GetVersionInfo()
	var b strings.Builder
	fmt.Fprintf(&b, "Version: %s\nBuild: %s\nCommit: %s", info.Version, info.BuildTime, info.CommitID)
	if cached := version.GetChecker().GetCachedResult(); cached != nil {
		if cached.HasUpdate {
			fmt.Fprintf(&b, "\n%s", tr(src.ReplyLang, "有新版本可用", "Update available"))
			if len(cached.NewReleases) > 0 {
				fmt.Fprintf(&b, ": %s", cached.NewReleases[0].Version)
			}
		} else {
			fmt.Fprintf(&b, "\n%s", tr(src.ReplyLang, "已是最新版本", "Up to date"))
		}
	}
	return chatops.Reply{Text: wrapMono(b.String())}, nil
}
