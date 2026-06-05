package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/models"
)

const delrssWizardTTL = 5 * time.Minute

const (
	delrssStepPickSite = "delrss_pick_site"
	delrssStepPickRSS  = "delrss_pick_rss"
	delrssStepConfirm  = "delrss_confirm"
)

type delrssWizardState struct {
	Step    string `json:"step"`
	Site    string `json:"site"`
	RSSID   uint   `json:"rss_id"`
	RSSName string `json:"rss_name"`
	RSSURL  string `json:"rss_url"`
}

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "delrss",
		Description: "互动删除 RSS 订阅（先列出后选择） (Delete RSS subscription)",
		AdminOnly:   true,
		Handler:     delrssHandler,
	})
}

func delrssHandler(ctx context.Context, _ []string, src chatops.Source) (chatops.Reply, error) {
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
	return setDelrssSession(ctx, src, delrssWizardState{Step: delrssStepPickSite})
}

func delrssStepHandler(ctx context.Context, args []string, src chatops.Source, data string) (chatops.Reply, error) {
	var st delrssWizardState
	if err := json.Unmarshal([]byte(data), &st); err != nil {
		return errReply(src.ReplyLang, "向导状态已损坏，请重新发送 /delrss", "wizard state is invalid, send /delrss again"), nil
	}
	text := ""
	if len(args) > 0 {
		text = strings.TrimSpace(args[0])
	}
	switch st.Step {
	case delrssStepPickSite:
		return handleDelrssPickSite(ctx, src, st, text)
	case delrssStepPickRSS:
		return handleDelrssPickRSS(ctx, src, st, text)
	case delrssStepConfirm:
		return handleDelrssConfirm(ctx, src, st, text)
	default:
		return errReply(src.ReplyLang, "未知向导步骤，请重新发送 /delrss", "unknown wizard step, send /delrss again"), nil
	}
}

func handleDelrssPickSite(ctx context.Context, src chatops.Source, st delrssWizardState, text string) (chatops.Reply, error) {
	sites, err := enabledSiteNames(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "查询站点失败: %v", "list sites failed: %v", err), nil
	}
	site, ok := resolveSiteSelection(text, sites)
	if !ok {
		return retryDelrssSession(ctx, src, st, "站点不存在或未启用，请回复列表中的站点名或序号。")
	}
	st.Site = site
	st.Step = delrssStepPickRSS
	return setDelrssSession(ctx, src, st)
}

func handleDelrssPickRSS(ctx context.Context, src chatops.Source, st delrssWizardState, text string) (chatops.Reply, error) {
	list, err := getServices().RSSWizard.ListRSSForSite(st.Site)
	if err != nil {
		return errReply(src.ReplyLang, "查询 RSS 订阅失败: %v", "list RSS failed: %v", err), nil
	}
	chosen, ok := resolveRSSSelection(text, list)
	if !ok {
		return retryDelrssSession(ctx, src, st, "RSS 订阅不存在，请回复列表中的名称、ID 或序号。")
	}
	st.RSSID = chosen.ID
	st.RSSName = chosen.Name
	st.RSSURL = chosen.URL
	st.Step = delrssStepConfirm
	return setDelrssSession(ctx, src, st)
}

func handleDelrssConfirm(ctx context.Context, src chatops.Source, st delrssWizardState, text string) (chatops.Reply, error) {
	if !strings.EqualFold(text, "YES") {
		return okReply("已取消删除 RSS 订阅"), nil
	}
	svc := getServices()
	if svc == nil || svc.RSSWizard == nil {
		return errReply(src.ReplyLang, "RSS 向导服务不可用", "RSS wizard service unavailable"), nil
	}
	deleted, err := svc.RSSWizard.DeleteRSSFromSite(st.Site, st.RSSID)
	if err != nil {
		return errReply(src.ReplyLang, "删除 RSS 订阅失败: %v", "delete RSS subscription failed: %v", err), nil
	}
	name := deleted.Name
	if name == "" {
		name = st.RSSName
	}
	return okReply(fmt.Sprintf("已删除 RSS 订阅：%s（ID: %d）", name, st.RSSID)), nil
}

func setDelrssSession(ctx context.Context, src chatops.Source, st delrssWizardState) (chatops.Reply, error) {
	data, err := json.Marshal(st)
	if err != nil {
		return errReply(src.ReplyLang, "保存向导状态失败: %v", "save wizard state failed: %v", err), nil
	}
	prompt, keep, err := renderDelrssPrompt(ctx, st)
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
			return delrssStepHandler(ctx, args, fsrc, stateData)
		},
	}, delrssWizardTTL)
	return okReply(prompt), nil
}

func retryDelrssSession(ctx context.Context, src chatops.Source, st delrssWizardState, msg string) (chatops.Reply, error) {
	reply, err := setDelrssSession(ctx, src, st)
	if err != nil {
		return reply, err
	}
	reply.Text = msg + "\n\n" + reply.Text
	return reply, nil
}

func renderDelrssPrompt(ctx context.Context, st delrssWizardState) (string, bool, error) {
	switch st.Step {
	case delrssStepPickSite:
		sites, err := enabledSiteNames(ctx)
		if err != nil {
			return "", false, err
		}
		if len(sites) == 0 {
			return "未找到已启用站点，请先在 Web 界面配置并启用站点。", false, nil
		}
		return "请选择要删除 RSS 的站点（回复站点名或序号）：\n" + numberedLines(sites), true, nil
	case delrssStepPickRSS:
		list, err := getServices().RSSWizard.ListRSSForSite(st.Site)
		if err != nil {
			return "", false, err
		}
		if len(list) == 0 {
			return fmt.Sprintf("站点 %s 当前没有 RSS 订阅，无需删除。", st.Site), false, nil
		}
		return "请选择要删除的 RSS 订阅（回复名称、ID 或序号）：\n" + delrssNumberedRSS(list), true, nil
	case delrssStepConfirm:
		return fmt.Sprintf("请确认删除以下 RSS 订阅，回复 YES 确认；回复其他内容取消。\n站点: %s\n名称: %s\nID: %d\nURL: %s", st.Site, st.RSSName, st.RSSID, st.RSSURL), true, nil
	default:
		return "未知向导步骤，请重新发送 /delrss。", false, nil
	}
}

func delrssNumberedRSS(list []models.RSSConfig) string {
	var b strings.Builder
	for i, r := range list {
		fmt.Fprintf(&b, "%d. %s（ID: %d）", i+1, r.Name, r.ID)
		if i != len(list)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func resolveRSSSelection(input string, list []models.RSSConfig) (models.RSSConfig, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return models.RSSConfig{}, false
	}
	for _, r := range list {
		if matchNameOrID(input, r.Name, r.ID) {
			return r, true
		}
	}
	if n, err := strconv.Atoi(input); err == nil && n >= 1 && n <= len(list) {
		return list[n-1], true
	}
	return models.RSSConfig{}, false
}
