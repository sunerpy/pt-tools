package commands

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/internal/chatops"
)

// 追剧：/follow 建追剧（按播出时间搜索、开着版本锁），/runs 看一个订阅最近的运行，/unlock 让一集换版本。

const (
	followPickStep = "follow_pick"
	maxRunLines    = 5
	maxRunSteps    = 8
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "follow",
		Aliases:     []string{"追剧"},
		Description: "追剧：/follow 剧名，回复编号开始追（可加季号）；按播出时间搜索、开着版本锁 (Follow a TV show)",
		AdminOnly:   true,
		Handler:     followHandler,
	})
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "runs",
		Description: "订阅最近的运行：/runs 编号（编号见 /subs） (Recent runs of a subscription)",
		Handler:     runsHandler,
	})
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "unlock",
		Description: "追剧的一集换版本：/unlock 编号 集号（这一集不受版本锁限制） (Let one episode use another version)",
		AdminOnly:   true,
		Handler:     unlockHandler,
	})
}

func followHandler(ctx context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
	svc := getSubscribeService()
	if svc == nil {
		return errReply(src.ReplyLang, "订阅服务不可用", "subscription service unavailable"), nil
	}
	sessions := getServices()
	if sessions == nil || sessions.Sessions == nil {
		return errReply(src.ReplyLang, "会话存储未初始化", "session store not initialized"), nil
	}
	keyword := strings.TrimSpace(strings.Join(args, " "))
	if keyword == "" {
		return okReply(tr(src.ReplyLang, "用法：/follow 剧名", "usage: /follow <show>")), nil
	}
	all, err := svc.Search(ctx, keyword)
	if err != nil {
		return errReply(src.ReplyLang, "搜索失败: %v", "search failed: %v", err), nil
	}
	var found []SubscribeCandidate
	for _, c := range all {
		if c.Kind == "tv" {
			found = append(found, c)
		}
	}
	if len(found) == 0 {
		return okReply(tr(src.ReplyLang, "TMDB 上没有找到剧集「"+keyword+"」", "no TV show found on TMDB for "+keyword)), nil
	}
	header := tr(src.ReplyLang,
		"找到这些剧集（回复编号开始追；可以加季号，比如「2 3」是第 2 个的第 3 季，不加时追最新一季；回复别的取消）：\n",
		"Reply with a number to follow (add a season, e.g. \"2 3\"; latest season by default):\n")
	return offerCandidates(sessions, src, found, header, followPickStep, followPickHandler), nil
}

// followPickHandler 处理回复的编号（可带季号）：追那一季；不是编号时取消。
func followPickHandler(ctx context.Context, args []string, src chatops.Source, state string) (chatops.Reply, error) {
	c, season, done := parsePick(args, src, state, "follow")
	if done != nil {
		return *done, nil
	}
	svc := getSubscribeService()
	if svc == nil {
		return errReply(src.ReplyLang, "订阅服务不可用", "subscription service unavailable"), nil
	}
	text, err := svc.Follow(ctx, c.TMDBID, season)
	if err != nil {
		return errReply(src.ReplyLang, "追剧失败: %v", "follow failed: %v", err), nil
	}
	return okReply(tr(src.ReplyLang, "开始追剧："+text+"。/subs 看进度", "following: "+text)), nil
}

// parseSubID 读命令里的订阅编号（/subs 里 # 后面的数，可以带 #）。
func parseSubID(s string) (uint, bool) {
	n, err := strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(s), "#"), 10, 32)
	return uint(n), err == nil && n > 0
}

func runTrigger(lang, trigger string) string {
	switch trigger {
	case "create":
		return tr(lang, "建立", "created")
	case "schedule":
		return tr(lang, "定时搜索", "scheduled search")
	case "rss":
		return "RSS"
	case "manual":
		return tr(lang, "手动", "by hand")
	case "ai":
		return "AI"
	}
	return trigger
}

func runStatus(lang, status string) string {
	switch status {
	case "ok":
		return tr(lang, "成功", "ok")
	case "nothing":
		return tr(lang, "没有新的", "nothing new")
	case "waiting":
		return tr(lang, "在等", "waiting")
	case "failed":
		return tr(lang, "失败", "failed")
	case "partial":
		return tr(lang, "部分成功", "partly done")
	case "changed":
		return tr(lang, "改动", "changed")
	}
	return status
}

func runsHandler(ctx context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
	svc := getSubscribeService()
	if svc == nil {
		return errReply(src.ReplyLang, "订阅服务不可用", "subscription service unavailable"), nil
	}
	if len(args) != 1 {
		return okReply(tr(src.ReplyLang, "用法：/runs 编号（编号见 /subs）", "usage: /runs <id> (see /subs)")), nil
	}
	id, ok := parseSubID(args[0])
	if !ok {
		return errReply(src.ReplyLang, "编号不对：%s", "invalid id: %s", args[0]), nil
	}
	name, runs, err := svc.Runs(ctx, id, maxRunLines)
	if err != nil {
		return errReply(src.ReplyLang, "读取运行记录失败: %v", "read runs failed: %v", err), nil
	}
	if len(runs) == 0 {
		return okReply(tr(src.ReplyLang, name+" 还没有运行记录", "no runs yet for "+name)), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, tr(src.ReplyLang, "%s 最近的运行：\n", "Recent runs of %s:\n"), name)
	for i, r := range runs {
		fmt.Fprintf(&b, "%d. %s %s · %s", i+1, r.At.Local().Format("01-02 15:04"), runTrigger(src.ReplyLang, r.Trigger), runStatus(src.ReplyLang, r.Status))
		if r.Summary != "" {
			fmt.Fprintf(&b, "：%s", r.Summary)
		}
		b.WriteString("\n")
		// 最近的一次列出经过（最多几步；和说明一样的那步不重复）
		if i == 0 {
			steps := slices.DeleteFunc(slices.Clone(r.Steps), func(st string) bool { return st == r.Summary })
			for j, st := range steps {
				if j >= maxRunSteps {
					fmt.Fprintf(&b, tr(src.ReplyLang, "   ……还有 %d 步，到网页上看\n", "   ... %d more steps on the web UI\n"), len(steps)-maxRunSteps)
					break
				}
				fmt.Fprintf(&b, "   - %s\n", st)
			}
		}
	}
	return okReply(strings.TrimRight(b.String(), "\n")), nil
}

func unlockHandler(ctx context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
	svc := getSubscribeService()
	if svc == nil {
		return errReply(src.ReplyLang, "订阅服务不可用", "subscription service unavailable"), nil
	}
	if len(args) != 2 {
		return okReply(tr(src.ReplyLang, "用法：/unlock 编号 集号（编号见 /subs）", "usage: /unlock <id> <episode> (see /subs)")), nil
	}
	id, ok := parseSubID(args[0])
	if !ok {
		return errReply(src.ReplyLang, "编号不对：%s", "invalid id: %s", args[0]), nil
	}
	ep, err := strconv.Atoi(args[1])
	if err != nil || ep < 1 {
		return errReply(src.ReplyLang, "集号不对：%s", "invalid episode: %s", args[1]), nil
	}
	name, err := svc.Unlock(ctx, id, ep)
	if err != nil {
		return errReply(src.ReplyLang, "换版本失败: %v", "unlock failed: %v", err), nil
	}
	return okReply(tr(src.ReplyLang,
		fmt.Sprintf("%s 第 %d 集不再受版本锁限制，会尽快搜一次（别的集照旧只要锁住的版本）", name, ep),
		fmt.Sprintf("%s episode %d may now use another version; searching soon", name, ep))), nil
}
