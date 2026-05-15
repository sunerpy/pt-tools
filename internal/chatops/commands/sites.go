package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "sites",
		Description: "列出已配置站点或查看用户信息 (List configured sites or show user info)",
		Handler:     sitesHandler,
	})
}

func sitesHandler(ctx context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
	svc := getServices()
	if svc == nil || svc.Site == nil {
		return errReply(src.ReplyLang, "站点服务不可用", "site service unavailable"), nil
	}
	if len(args) >= 1 {
		name := strings.TrimSpace(args[0])
		info, err := svc.Site.GetSiteUserInfo(ctx, name)
		if err != nil {
			if errors.Is(err, app.ErrSiteNotFound) {
				return errReply(src.ReplyLang, "站点不存在: %s", "site not found: %s", name), nil
			}
			if errors.Is(err, app.ErrUserInfoUnavailable) {
				return errReply(src.ReplyLang, "站点 %s 暂无用户信息", "no user info for %s", name), nil
			}
			return errReply(src.ReplyLang, "查询失败: %v", "query failed: %v", err), nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Site: %s\nUser: %s\nUploaded: %s\nDownloaded: %s\nRatio: %s\nBonus: %s\nClass: %s",
			info.SiteName, info.Username, info.Uploaded, info.Downloaded, info.Ratio, info.Bonus, info.Class)
		return chatops.Reply{Text: wrapMono(b.String())}, nil
	}

	sites, err := svc.Site.ListSites(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "查询站点失败: %v", "list sites failed: %v", err), nil
	}
	if len(sites) == 0 {
		return okReply(tr(src.ReplyLang, "未配置站点", "no sites configured")), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, tr(src.ReplyLang, "站点（%d）\n", "Sites (%d)\n"), len(sites))
	for _, s := range sites {
		fmt.Fprintf(&b, "[%s] %s\n", s.Status, s.Name)
	}
	return okReply(strings.TrimRight(b.String(), "\n")), nil
}
