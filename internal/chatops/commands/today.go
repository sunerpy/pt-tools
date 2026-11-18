package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
)

// /today：今天订阅里播出的集与上映的电影，入库了没有（只读）。

// maxTodayLines 是 /today 最多列几项。
const maxTodayLines = 30

// TodayLine 是 /today 的一项：今天播出的一集，或上映的电影。
type TodayLine struct {
	// Title 是订阅名（带年份与季）加集号与集名；电影写上映
	Title     string
	MediaType string
	Season    int
	Episode   int
	Name      string
	// AirTime 是播出时刻（按播出时间追剧的才有，例如 21:30）
	AirTime string
	// State 是 library、downloading、waiting_version、missing、upcoming、skipped
	State string
}

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "today",
		Aliases:     []string{"今日"},
		Description: "今天订阅里播出的集与上映的电影，入库了没有 (What airs today)",
		Handler:     todayHandler,
	})
}

// todayStateLabel 是状态的说明（同网页上的台账）。
func todayStateLabel(lang, state string) string {
	switch state {
	case "library":
		return tr(lang, "已入库", "in library")
	case "downloading":
		return tr(lang, "下载中", "downloading")
	case "waiting_version":
		return tr(lang, "等版本", "waiting for the locked version")
	case "missing":
		return tr(lang, "缺", "missing")
	case "upcoming":
		return tr(lang, "还没播", "not aired yet")
	case "skipped":
		return tr(lang, "不追", "skipped")
	}
	return state
}

func todayHandler(ctx context.Context, _ []string, src chatops.Source) (chatops.Reply, error) {
	svc := getSubscribeService()
	if svc == nil {
		return errReply(src.ReplyLang, "订阅服务不可用", "subscription service unavailable"), nil
	}
	day, lines, err := svc.Today(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "读取今日更新失败: %v", "read today's updates failed: %v", err), nil
	}
	items := make([]map[string]any, 0, len(lines))
	inLib, episodes := 0, 0
	for _, l := range lines {
		if l.State == "library" {
			inLib++
		}
		if l.MediaType == "tv" {
			episodes++
		}
		items = append(items, map[string]any{
			"title": l.Title, "media_type": l.MediaType, "season": l.Season, "episode": l.Episode, "name": l.Name,
			"air_time": l.AirTime, "state": l.State, "state_label": todayStateLabel(src.ReplyLang, l.State),
		})
	}
	fields := map[string]any{"day": day, "count": len(lines), "in_library": inLib, "items": items}
	if len(lines) == 0 {
		return eventReply(tr(src.ReplyLang, day+" 订阅里没有播出的集", "nothing in your subscriptions airs today ("+day+")"), notify.EventReplyToday, fields), nil
	}
	var b strings.Builder
	switch movies := len(lines) - episodes; {
	case movies == 0:
		fmt.Fprintf(&b, tr(src.ReplyLang, "%s 播出 %d 集：", "%s: %d episode(s):"), day, episodes)
	case episodes == 0:
		fmt.Fprintf(&b, tr(src.ReplyLang, "%s 上映 %d 部电影：", "%s: %d movie(s):"), day, movies)
	default:
		fmt.Fprintf(&b, tr(src.ReplyLang, "%s 播出 %d 集、上映 %d 部电影：", "%s: %d episode(s), %d movie(s):"), day, episodes, movies)
	}
	for i, l := range lines {
		if i >= maxTodayLines {
			fmt.Fprintf(&b, tr(src.ReplyLang, "\n……还有 %d 项，到网页的订阅日历看", "\n... %d more in the calendar on the web UI"), len(lines)-maxTodayLines)
			break
		}
		b.WriteString("\n· " + l.Title)
		if l.AirTime != "" {
			b.WriteString(" " + l.AirTime)
		}
		b.WriteString(" · " + todayStateLabel(src.ReplyLang, l.State))
	}
	return eventReply(b.String(), notify.EventReplyToday, fields), nil
}
