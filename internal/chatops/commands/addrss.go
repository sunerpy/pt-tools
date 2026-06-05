package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/app"
	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/models"
)

const addrssWizardTTL = 5 * time.Minute

const (
	addrssStepPickSite       = "addrss_pick_site"
	addrssStepRSSName        = "addrss_rss_name"
	addrssStepRSSURL         = "addrss_rss_url"
	addrssStepPickDownloader = "addrss_pick_downloader"
	addrssStepCategory       = "addrss_category"
	addrssStepTag            = "addrss_tag"
	addrssStepDownloadPath   = "addrss_download_path"
	addrssStepFilterMode     = "addrss_filter_mode"
	addrssStepFilterRules    = "addrss_filter_rules"
	addrssStepNotifyMode     = "addrss_notify_mode"
	addrssStepNotifyConfIDs  = "addrss_notify_conf_ids"
	addrssStepMaxNotify      = "addrss_max_notify"
	addrssStepConfirm        = "addrss_confirm"
)

type addrssWizardState struct {
	Step                    string   `json:"step"`
	Site                    string   `json:"site"`
	RSSName                 string   `json:"rss_name"`
	RSSURL                  string   `json:"rss_url"`
	DownloaderID            *uint    `json:"downloader_id,omitempty"`
	DownloaderName          string   `json:"downloader_name"`
	Category                string   `json:"category"`
	Tag                     string   `json:"tag"`
	DownloadPath            string   `json:"download_path"`
	FilterMode              string   `json:"filter_mode"`
	FilterRuleIDs           []uint   `json:"filter_rule_ids,omitempty"`
	FilterRuleNames         []string `json:"filter_rule_names,omitempty"`
	NotifyMode              string   `json:"notify_mode"`
	NotifyConfIDs           []uint   `json:"notify_conf_ids,omitempty"`
	NotifyConfNames         []string `json:"notify_conf_names,omitempty"`
	MaxNotificationsPerHour *int     `json:"max_notifications_per_hour,omitempty"`
}

const addrssShortcutExample = "快捷添加（单行，| 分隔，下载器可选）：\n/addrss 站点 | 订阅名 | RSS地址 | 下载器(可选)\n示例：/addrss hddolby | 高清电影 | https://www.hddolby.com/torrentrss.php?passkey=xxx | qbittorrent-default"

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "addrss",
		Description: "互动添加 RSS 订阅（文本向导，或单行 /addrss 站点|名|URL|下载器） (Add RSS subscription)",
		AdminOnly:   true,
		Handler:     addrssHandler,
	})
}

func addrssHandler(ctx context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
	svc := getServices()
	if svc == nil || svc.Sessions == nil {
		return errReply(src.ReplyLang, "会话存储未初始化", "session store not initialized"), nil
	}
	if svc.Site == nil {
		return errReply(src.ReplyLang, "站点服务不可用", "site service unavailable"), nil
	}
	if svc.RSSWizard == nil {
		return errReply(src.ReplyLang, "RSS 向导服务不可用", "RSS wizard service unavailable"), nil
	}

	// 单行快捷模式：/addrss 站点 | 订阅名 | RSS地址 [| 下载器]
	// parseCommand 用 strings.Fields 拆词，故先用空格拼回，再按 | 分隔。
	if joined := strings.TrimSpace(strings.Join(args, " ")); joined != "" {
		return handleAddrssShortcut(ctx, src, joined)
	}

	return setAddrssSession(ctx, src, addrssWizardState{Step: addrssStepPickSite})
}

// handleAddrssShortcut 解析 `站点 | 订阅名 | RSS地址 [| 下载器]` 一次性添加。
func handleAddrssShortcut(ctx context.Context, src chatops.Source, raw string) (chatops.Reply, error) {
	parts := strings.Split(strings.ReplaceAll(raw, "｜", "|"), "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	if len(parts) < 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return okReply("快捷格式有误，至少需要 站点、订阅名、RSS地址 三段。\n\n" + addrssShortcutExample), nil
	}
	siteInput, name, rawURL := parts[0], parts[1], parts[2]
	downloaderInput := ""
	if len(parts) >= 4 {
		downloaderInput = parts[3]
	}

	sites, err := enabledSiteNames(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "查询站点失败: %v", "list sites failed: %v", err), nil
	}
	site, ok := resolveSiteSelection(siteInput, sites)
	if !ok {
		return okReply("站点不存在或未启用：" + siteInput + "\n可用站点：" + strings.Join(sites, "、") + "\n\n" + addrssShortcutExample), nil
	}
	if !validAddrssRSSURL(rawURL) {
		return okReply("RSS URL 无效，请使用 http/https 开头且包含有效主机名的完整 URL。\n\n" + addrssShortcutExample), nil
	}
	dup, err := duplicateRSSURL(ctx, site, rawURL)
	if err != nil {
		return errReply(src.ReplyLang, "检查重复 URL 失败: %v", "check duplicate URL failed: %v", err), nil
	}
	if dup {
		return okReply("该站点已存在相同 RSS URL，请换一个 URL。"), nil
	}

	entry := models.RSSConfig{
		Name:          name,
		URL:           strings.TrimSpace(rawURL),
		FilterMode:    models.FilterMode(""),
		NotifyConfIDs: encodeUintJSONArray(nil),
	}
	if downloaderInput != "" && !isDefaultDownloaderInput(downloaderInput) {
		downloaders, derr := getServices().RSSWizard.ListDownloaders(ctx)
		if derr != nil {
			return errReply(src.ReplyLang, "查询下载器失败: %v", "list downloaders failed: %v", derr), nil
		}
		dl, dok := resolveDownloaderSelection(downloaderInput, downloaders)
		if !dok {
			names := make([]string, 0, len(downloaders))
			for _, d := range downloaders {
				names = append(names, d.Name)
			}
			return okReply("下载器不存在：" + downloaderInput + "\n可用下载器：" + strings.Join(names, "、") + "\n\n" + addrssShortcutExample), nil
		}
		id := dl.ID
		entry.DownloaderID = &id
	}

	created, err := getServices().RSSWizard.AppendRSSToSite(site, entry)
	if err != nil {
		return errReply(src.ReplyLang, "添加 RSS 订阅失败: %v", "add RSS subscription failed: %v", err), nil
	}
	return okReply(fmt.Sprintf("已添加 RSS 订阅：%s（ID: %d）\n提示：需要站点启用、RSS AutoStart/调度启用并有可用下载器后才会自动生效。", created.Name, created.ID)), nil
}

func addrssStepHandler(ctx context.Context, args []string, src chatops.Source, data string) (chatops.Reply, error) {
	var st addrssWizardState
	if err := json.Unmarshal([]byte(data), &st); err != nil {
		return errReply(src.ReplyLang, "向导状态已损坏，请重新发送 /addrss", "wizard state is invalid, send /addrss again"), nil
	}

	text := ""
	if len(args) > 0 {
		text = strings.TrimSpace(args[0])
	}

	switch st.Step {
	case addrssStepPickSite:
		return handleAddrssPickSite(ctx, src, st, text)
	case addrssStepRSSName:
		return handleAddrssRSSName(ctx, src, st, text)
	case addrssStepRSSURL:
		return handleAddrssRSSURL(ctx, src, st, text)
	case addrssStepPickDownloader:
		return handleAddrssPickDownloader(ctx, src, st, text)
	case addrssStepCategory:
		st.Category = optionalText(text)
		st.Step = addrssStepTag
		return setAddrssSession(ctx, src, st)
	case addrssStepTag:
		st.Tag = optionalText(text)
		st.Step = addrssStepDownloadPath
		return setAddrssSession(ctx, src, st)
	case addrssStepDownloadPath:
		st.DownloadPath = optionalText(text)
		st.Step = addrssStepFilterMode
		return setAddrssSession(ctx, src, st)
	case addrssStepFilterMode:
		return handleAddrssFilterMode(ctx, src, st, text)
	case addrssStepFilterRules:
		return handleAddrssFilterRules(ctx, src, st, text)
	case addrssStepNotifyMode:
		st.NotifyMode = optionalText(text)
		st.Step = addrssStepNotifyConfIDs
		return setAddrssSession(ctx, src, st)
	case addrssStepNotifyConfIDs:
		return handleAddrssNotifyConfIDs(ctx, src, st, text)
	case addrssStepMaxNotify:
		return handleAddrssMaxNotify(ctx, src, st, text)
	case addrssStepConfirm:
		return handleAddrssConfirm(ctx, src, st, text)
	default:
		return errReply(src.ReplyLang, "未知向导步骤，请重新发送 /addrss", "unknown wizard step, send /addrss again"), nil
	}
}

func handleAddrssPickSite(ctx context.Context, src chatops.Source, st addrssWizardState, text string) (chatops.Reply, error) {
	sites, err := enabledSiteNames(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "查询站点失败: %v", "list sites failed: %v", err), nil
	}
	if site, ok := resolveSiteSelection(text, sites); ok {
		st.Site = site
		st.Step = addrssStepRSSName
		return setAddrssSession(ctx, src, st)
	}
	return retryAddrssSession(ctx, src, st, "站点不存在或未启用，请回复列表中的站点名或序号。")
}

func handleAddrssRSSName(ctx context.Context, src chatops.Source, st addrssWizardState, text string) (chatops.Reply, error) {
	if text == "" {
		return retryAddrssSession(ctx, src, st, "RSS 订阅名不能为空。")
	}
	st.RSSName = text
	st.Step = addrssStepRSSURL
	return setAddrssSession(ctx, src, st)
}

func handleAddrssRSSURL(ctx context.Context, src chatops.Source, st addrssWizardState, text string) (chatops.Reply, error) {
	if !validAddrssRSSURL(text) {
		return retryAddrssSession(ctx, src, st, "RSS URL 无效，请粘贴 http/https 开头且包含有效主机名的完整 RSS URL。")
	}
	dup, err := duplicateRSSURL(ctx, st.Site, text)
	if err != nil {
		return errReply(src.ReplyLang, "检查重复 URL 失败: %v", "check duplicate URL failed: %v", err), nil
	}
	if dup {
		return retryAddrssSession(ctx, src, st, "该站点已存在相同 RSS URL，请换一个 URL。")
	}
	st.RSSURL = strings.TrimSpace(text)
	st.Step = addrssStepPickDownloader
	return setAddrssSession(ctx, src, st)
}

func handleAddrssPickDownloader(ctx context.Context, src chatops.Source, st addrssWizardState, text string) (chatops.Reply, error) {
	if isDefaultDownloaderInput(text) {
		st.DownloaderID = nil
		st.DownloaderName = "默认"
		st.Step = addrssStepCategory
		return setAddrssSession(ctx, src, st)
	}

	downloaders, err := getServices().RSSWizard.ListDownloaders(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "查询下载器失败: %v", "list downloaders failed: %v", err), nil
	}
	if dl, ok := resolveDownloaderSelection(text, downloaders); ok {
		id := dl.ID
		st.DownloaderID = &id
		st.DownloaderName = dl.Name
		st.Step = addrssStepCategory
		return setAddrssSession(ctx, src, st)
	}
	return retryAddrssSession(ctx, src, st, "下载器不存在，请回复列表中的下载器名称或序号，或回复 默认 使用默认下载器。")
}

func handleAddrssFilterMode(ctx context.Context, src chatops.Source, st addrssWizardState, text string) (chatops.Reply, error) {
	if isSkipInput(text) {
		st.FilterMode = ""
		st.Step = addrssStepFilterRules
		return setAddrssSession(ctx, src, st)
	}
	mode := models.FilterMode(strings.TrimSpace(text))
	if mode != models.FilterModeAutoFree && mode != models.FilterModeFilterOnly && mode != models.FilterModeFreeOnly {
		return retryAddrssSession(ctx, src, st, "过滤模式无效，请回复 auto_free、filter_only、free_only，或回复 skip 使用默认。")
	}
	st.FilterMode = string(mode)
	st.Step = addrssStepFilterRules
	return setAddrssSession(ctx, src, st)
}

func handleAddrssFilterRules(ctx context.Context, src chatops.Source, st addrssWizardState, text string) (chatops.Reply, error) {
	if isSkipInput(text) {
		st.FilterRuleIDs = nil
		st.FilterRuleNames = nil
		st.Step = addrssStepNotifyMode
		return setAddrssSession(ctx, src, st)
	}
	options, err := getServices().RSSWizard.ListFilterRules(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "查询过滤规则失败: %v", "list filter rules failed: %v", err), nil
	}
	ids, names, ok := parseIDNameSelection(text, options)
	if !ok {
		return retryAddrssSession(ctx, src, st, "过滤规则不存在，请回复逗号分隔的规则名称或 ID，或回复 skip 跳过。")
	}
	st.FilterRuleIDs = ids
	st.FilterRuleNames = names
	st.Step = addrssStepNotifyMode
	return setAddrssSession(ctx, src, st)
}

func handleAddrssNotifyConfIDs(ctx context.Context, src chatops.Source, st addrssWizardState, text string) (chatops.Reply, error) {
	if isSkipInput(text) {
		st.NotifyConfIDs = nil
		st.NotifyConfNames = nil
		st.Step = addrssStepMaxNotify
		return setAddrssSession(ctx, src, st)
	}
	options, err := getServices().RSSWizard.ListNotificationChannels(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "查询通知通道失败: %v", "list notification channels failed: %v", err), nil
	}
	ids, names, ok := parseIDNameSelection(text, options)
	if !ok {
		return retryAddrssSession(ctx, src, st, "通知通道不存在，请回复逗号分隔的通道名称或 ID，或回复 skip 跳过。")
	}
	st.NotifyConfIDs = ids
	st.NotifyConfNames = names
	st.Step = addrssStepMaxNotify
	return setAddrssSession(ctx, src, st)
}

func handleAddrssMaxNotify(ctx context.Context, src chatops.Source, st addrssWizardState, text string) (chatops.Reply, error) {
	if isSkipInput(text) {
		st.MaxNotificationsPerHour = nil
		st.Step = addrssStepConfirm
		return setAddrssSession(ctx, src, st)
	}
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n < 0 {
		return retryAddrssSession(ctx, src, st, "每小时通知上限必须是大于等于 0 的整数，或回复 skip 使用默认。")
	}
	st.MaxNotificationsPerHour = &n
	st.Step = addrssStepConfirm
	return setAddrssSession(ctx, src, st)
}

func handleAddrssConfirm(ctx context.Context, src chatops.Source, st addrssWizardState, text string) (chatops.Reply, error) {
	if !strings.EqualFold(text, "YES") {
		return okReply("已取消添加 RSS 订阅"), nil
	}
	svc := getServices()
	if svc == nil || svc.RSSWizard == nil {
		return errReply(src.ReplyLang, "RSS 向导服务不可用", "RSS wizard service unavailable"), nil
	}
	entry := models.RSSConfig{
		Name:          st.RSSName,
		URL:           st.RSSURL,
		Category:      st.Category,
		Tag:           st.Tag,
		DownloadPath:  st.DownloadPath,
		DownloaderID:  st.DownloaderID,
		FilterRuleIDs: st.FilterRuleIDs,
		FilterMode:    models.FilterMode(st.FilterMode),
		NotifyMode:    st.NotifyMode,
		NotifyConfIDs: encodeUintJSONArray(st.NotifyConfIDs),
	}
	if st.MaxNotificationsPerHour != nil {
		entry.MaxNotificationsPerHour = *st.MaxNotificationsPerHour
	}
	created, err := svc.RSSWizard.AppendRSSToSite(st.Site, entry)
	if err != nil {
		return errReply(src.ReplyLang, "添加 RSS 订阅失败: %v", "add RSS subscription failed: %v", err), nil
	}
	return okReply(fmt.Sprintf("已添加 RSS 订阅：%s（ID: %d）\n提示：需要站点启用、RSS AutoStart/调度启用并有可用下载器后才会自动生效。", created.Name, created.ID)), nil
}

func setAddrssSession(ctx context.Context, src chatops.Source, st addrssWizardState) (chatops.Reply, error) {
	data, err := json.Marshal(st)
	if err != nil {
		return errReply(src.ReplyLang, "保存向导状态失败: %v", "save wizard state failed: %v", err), nil
	}
	prompt, keep, err := renderAddrssPrompt(ctx, st)
	if err != nil {
		return errReply(src.ReplyLang, "生成向导提示失败: %v", "render wizard prompt failed: %v", err), nil
	}
	if !keep {
		return okReply(prompt), nil
	}
	stateData := string(data)
	getServices().Sessions.Set(src.ChannelType, src.ChannelConfID, src.ChannelUserID, chatops.SessionState{
		Step: st.Step,
		Data: stateData,
		Handler: func(ctx context.Context, args []string, fsrc chatops.Source) (chatops.Reply, error) {
			return addrssStepHandler(ctx, args, fsrc, stateData)
		},
	}, addrssWizardTTL)
	return okReply(prompt), nil
}

func retryAddrssSession(ctx context.Context, src chatops.Source, st addrssWizardState, msg string) (chatops.Reply, error) {
	reply, err := setAddrssSession(ctx, src, st)
	if err != nil {
		return reply, err
	}
	reply.Text = msg + "\n\n" + reply.Text
	return reply, nil
}

func renderAddrssPrompt(ctx context.Context, st addrssWizardState) (string, bool, error) {
	switch st.Step {
	case addrssStepPickSite:
		sites, err := enabledSiteNames(ctx)
		if err != nil {
			return "", false, err
		}
		if len(sites) == 0 {
			return "未找到已启用站点，请先在 Web 界面配置并启用站点。", false, nil
		}
		return "请选择要添加 RSS 的站点（回复站点名或序号）：\n" + numberedLines(sites) + "\n\n" + addrssShortcutExample, true, nil
	case addrssStepRSSName:
		return "请输入 RSS 订阅名：", true, nil
	case addrssStepRSSURL:
		return "请粘贴完整 RSS URL（仅校验 http/https 与主机名，不会抓取）：", true, nil
	case addrssStepPickDownloader:
		text, keep, err := downloaderPrompt(ctx)
		return text, keep, err
	case addrssStepCategory:
		return "可选：请输入分类 category；回复 skip 或空消息使用默认。", true, nil
	case addrssStepTag:
		return "可选：请输入标签 tag；回复 skip 或空消息使用默认。", true, nil
	case addrssStepDownloadPath:
		return "可选：请输入下载路径 download_path；回复 skip 或空消息使用默认。", true, nil
	case addrssStepFilterMode:
		return "可选：请输入过滤模式 filter_mode：auto_free / filter_only / free_only；回复 skip 或空消息使用默认。", true, nil
	case addrssStepFilterRules:
		text, err := idNamePrompt(ctx, "可选：选择过滤规则（回复逗号分隔的名称或 ID；回复 skip 或空消息跳过）：", true)
		return text, true, err
	case addrssStepNotifyMode:
		return "可选：请输入通知模式 notify_mode；回复 skip 或空消息使用默认。", true, nil
	case addrssStepNotifyConfIDs:
		text, err := idNamePrompt(ctx, "可选：选择通知通道（回复逗号分隔的名称或 ID；回复 skip 或空消息跳过）：", false)
		return text, true, err
	case addrssStepMaxNotify:
		return "可选：请输入每小时通知上限 max_notifications_per_hour；回复 skip 或空消息使用默认。", true, nil
	case addrssStepConfirm:
		return formatAddrssConfirm(st), true, nil
	default:
		return "未知向导步骤，请重新发送 /addrss。", false, nil
	}
}

func enabledSiteNames(ctx context.Context) ([]string, error) {
	svc := getServices()
	if svc == nil || svc.Site == nil {
		return nil, app.ErrSiteNotFound
	}
	sites, err := svc.Site.ListSites(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(sites))
	for _, site := range sites {
		if site.Enabled {
			out = append(out, site.Name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// resolveSiteSelection 接受站点名（不分大小写）或列表 1-based 序号，返回规范站点名。
func resolveSiteSelection(input string, sites []string) (string, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", false
	}
	for _, site := range sites {
		if strings.EqualFold(input, site) {
			return site, true
		}
	}
	if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(sites) {
		return sites[n-1], true
	}
	return "", false
}

// resolveDownloaderSelection 接受下载器名、DB ID 或列表 1-based 序号。
func resolveDownloaderSelection(input string, downloaders []DownloaderOption) (DownloaderOption, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return DownloaderOption{}, false
	}
	for _, dl := range downloaders {
		if matchNameOrID(input, dl.Name, dl.ID) {
			return dl, true
		}
	}
	if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(downloaders) {
		return downloaders[n-1], true
	}
	return DownloaderOption{}, false
}

func downloaderPrompt(ctx context.Context) (string, bool, error) {
	downloaders, err := getServices().RSSWizard.ListDownloaders(ctx)
	if err != nil {
		return "", false, err
	}
	if len(downloaders) == 0 {
		return "未配置下载器，请先在 Web 界面配置下载器。向导已取消。", false, nil
	}
	var b strings.Builder
	b.WriteString("请选择下载器（回复名称或序号；回复 默认 或空消息使用默认下载器）：")
	for i, dl := range downloaders {
		mark := ""
		if dl.IsDefault {
			mark = "（默认）"
		}
		fmt.Fprintf(&b, "\n%d. %s%s", i+1, dl.Name, mark)
	}
	return b.String(), true, nil
}

func idNamePrompt(ctx context.Context, header string, filterRules bool) (string, error) {
	var (
		options []IDNameOption
		err     error
	)
	if filterRules {
		options, err = getServices().RSSWizard.ListFilterRules(ctx)
	} else {
		options, err = getServices().RSSWizard.ListNotificationChannels(ctx)
	}
	if err != nil {
		return "", err
	}
	if len(options) == 0 {
		return header + "\n当前没有启用的可选项。", nil
	}
	var b strings.Builder
	b.WriteString(header)
	for _, option := range options {
		fmt.Fprintf(&b, "\n- %s（ID: %d）", option.Name, option.ID)
	}
	return b.String(), nil
}

func formatAddrssConfirm(st addrssWizardState) string {
	var b strings.Builder
	b.WriteString("请确认添加 RSS 订阅，回复 YES 确认；回复其他内容取消。\n")
	fmt.Fprintf(&b, "站点: %s\n", st.Site)
	fmt.Fprintf(&b, "名称: %s\n", st.RSSName)
	fmt.Fprintf(&b, "URL: %s\n", st.RSSURL)
	fmt.Fprintf(&b, "下载器: %s\n", defaultString(st.DownloaderName))
	fmt.Fprintf(&b, "分类: %s\n", defaultString(st.Category))
	fmt.Fprintf(&b, "标签: %s\n", defaultString(st.Tag))
	fmt.Fprintf(&b, "下载路径: %s\n", defaultString(st.DownloadPath))
	fmt.Fprintf(&b, "过滤模式: %s\n", defaultString(st.FilterMode))
	fmt.Fprintf(&b, "过滤规则: %s\n", defaultString(strings.Join(st.FilterRuleNames, ", ")))
	fmt.Fprintf(&b, "通知模式: %s\n", defaultString(st.NotifyMode))
	fmt.Fprintf(&b, "通知通道: %s\n", defaultString(strings.Join(st.NotifyConfNames, ", ")))
	maxNotify := "默认"
	if st.MaxNotificationsPerHour != nil {
		maxNotify = strconv.Itoa(*st.MaxNotificationsPerHour)
	}
	fmt.Fprintf(&b, "每小时通知上限: %s", maxNotify)
	return b.String()
}

func duplicateRSSURL(ctx context.Context, siteName, raw string) (bool, error) {
	if global.GlobalDB == nil || global.GlobalDB.DB == nil {
		return false, nil
	}
	var site models.SiteSetting
	if err := global.GlobalDB.DB.WithContext(ctx).Where("name = ?", siteName).First(&site).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}
	var count int64
	normalized := strings.TrimSpace(strings.ToLower(raw))
	err := global.GlobalDB.DB.WithContext(ctx).Model(&models.RSSSubscription{}).
		Where("site_id = ? AND lower(trim(url)) = ?", site.ID, normalized).
		Count(&count).Error
	return count > 0, err
}

func validAddrssRSSURL(raw string) bool {
	if raw == "" {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	host := u.Hostname()
	if host == "" || host == "rss.m-team.xxx" {
		return false
	}
	return true
}

func optionalText(text string) string {
	if isSkipInput(text) {
		return ""
	}
	return strings.TrimSpace(text)
}

func isSkipInput(text string) bool {
	text = strings.TrimSpace(text)
	return text == "" || strings.EqualFold(text, "skip")
}

func isDefaultDownloaderInput(text string) bool {
	text = strings.TrimSpace(text)
	return text == "" || text == "默认" || strings.EqualFold(text, "default")
}

func matchNameOrID(input, name string, id uint) bool {
	input = strings.TrimSpace(input)
	if strings.EqualFold(input, name) {
		return true
	}
	parsed, err := strconv.ParseUint(input, 10, 64)
	return err == nil && uint(parsed) == id
}

func parseIDNameSelection(input string, options []IDNameOption) ([]uint, []string, bool) {
	parts := strings.Split(strings.ReplaceAll(input, "，", ","), ",")
	ids := make([]uint, 0, len(parts))
	names := make([]string, 0, len(parts))
	seen := make(map[uint]struct{}, len(parts))
	for _, part := range parts {
		token := strings.TrimSpace(part)
		if token == "" {
			continue
		}
		matched := false
		for _, option := range options {
			if matchNameOrID(token, option.Name, option.ID) {
				if _, ok := seen[option.ID]; !ok {
					ids = append(ids, option.ID)
					names = append(names, option.Name)
					seen[option.ID] = struct{}{}
				}
				matched = true
				break
			}
		}
		if !matched {
			return nil, nil, false
		}
	}
	return ids, names, len(ids) > 0
}

func encodeUintJSONArray(values []uint) string {
	if len(values) == 0 {
		return "[]"
	}
	b, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func numberedLines(values []string) string {
	var b strings.Builder
	for i, value := range values {
		fmt.Fprintf(&b, "%d. %s", i+1, value)
		if i != len(values)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func defaultString(value string) string {
	if strings.TrimSpace(value) == "" {
		return "默认"
	}
	return value
}
