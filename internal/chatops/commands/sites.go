package commands

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
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
		// 站点报的下载量是计费口径（按免费、折扣折算以后计入分享率的数），不是实际传输量
		fmt.Fprintf(&b, "Site: %s\nUser: %s\nUploaded: %s\nDownloaded (billed): %s\nRatio: %s\nBonus: %s\nClass: %s",
			info.SiteName, info.Username, info.Uploaded, info.Downloaded, info.Ratio, info.Bonus, info.Class)
		return eventReply(wrapMono(b.String()), notify.EventReplySiteInfo, map[string]any{
			"site": info.SiteName, "user": info.Username, "uploaded": info.Uploaded, "downloaded": info.Downloaded,
			"uploaded_bytes": info.UploadedBytes, "downloaded_bytes": info.DownloadedBytes,
			"ratio": info.RatioValue, "bonus": info.BonusValue, "class": info.Class,
		}), nil
	}

	sites, err := svc.Site.ListSites(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "查询站点失败: %v", "list sites failed: %v", err), nil
	}
	items := make([]map[string]any, 0, len(sites))
	for _, s := range sites {
		items = append(items, map[string]any{"name": s.Name, "status": s.Status, "enabled": s.Enabled})
	}
	fields := map[string]any{"count": len(sites), "items": items}
	if len(sites) == 0 {
		return eventReply(tr(src.ReplyLang, "未配置站点", "no sites configured"), notify.EventReplySites, fields), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, tr(src.ReplyLang, "站点（%d）\n", "Sites (%d)\n"), len(sites))
	for _, s := range sites {
		fmt.Fprintf(&b, "[%s] %s\n", s.Status, s.Name)
	}
	return eventReply(strings.TrimRight(b.String(), "\n"), notify.EventReplySites, fields), nil
}
