package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sunerpy/pt-tools/internal/chatops"
	"github.com/sunerpy/pt-tools/internal/notify"
)

// SubscribeService 是 /sub、/subs、/follow、/runs、/unlock 用到的订阅能力。订阅服务比 ChatOps 晚建，用 SetSubscribeService 单独登记。
type SubscribeService interface {
	// Search 在 TMDB 上按关键字找电影与剧集（按热度排）。
	Search(ctx context.Context, keyword string) ([]SubscribeCandidate, error)
	// Subscribe 建订阅（剧集 season 为 0 时订最新一季），返回订阅的名字（剧集带季）。
	Subscribe(ctx context.Context, kind string, tmdbID, season int) (string, error)
	// Follow 建追剧（season 为 0 时追最新一季），按追剧的默认设置（按播出时间搜索、开着版本锁），返回名字与设置的说明。
	Follow(ctx context.Context, tmdbID, season int) (string, error)
	// List 列出订阅（新的在前）。
	List(ctx context.Context) ([]SubscribeLine, error)
	// Runs 是编号为 id 的订阅的名字与最近 limit 次运行（新的在前）。
	Runs(ctx context.Context, id uint, limit int) (string, []RunLine, error)
	// Unlock 让追剧的第 episode 集不受版本锁限制（并尽快搜一次），返回订阅的名字。
	Unlock(ctx context.Context, id uint, episode int) (string, error)
}

// SubscribeCandidate 是 /sub 列出的一个条目。
type SubscribeCandidate struct {
	Kind       string `json:"kind"`
	TMDBID     int    `json:"tmdb_id"`
	Title      string `json:"title"`
	Year       int    `json:"year"`
	Subscribed bool   `json:"subscribed"`
}

// SubscribeLine 是 /subs 的一行。
type SubscribeLine struct {
	// ID 是订阅的编号（/runs、/unlock 用它）
	ID       uint
	Title    string
	Status   string
	Progress string
	// Next 是下次主动搜索的时间（暂停、完成的是空）
	Next string
}

// RunLine 是 /runs 的一次运行。
type RunLine struct {
	At      time.Time
	Trigger string
	Status  string
	Summary string
	Steps   []string
}

var (
	subscribeMu  sync.RWMutex
	subscribeSvc SubscribeService
)

// SetSubscribeService 登记订阅服务（为 nil 时 /sub、/subs 回不可用）。
func SetSubscribeService(s SubscribeService) {
	subscribeMu.Lock()
	defer subscribeMu.Unlock()
	subscribeSvc = s
}

func getSubscribeService() SubscribeService {
	subscribeMu.RLock()
	defer subscribeMu.RUnlock()
	return subscribeSvc
}

const (
	subPickTTL    = 5 * time.Minute
	subPickStep   = "sub_pick"
	maxSubResults = 8
	maxSubLines   = 20
)

func init() {
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "sub",
		Description: "订阅电影或剧集：/sub 关键词，回复编号订阅（剧集可加季号） (Subscribe to a movie or TV season)",
		AdminOnly:   true,
		Handler:     subHandler,
	})
	chatops.RegisterCommand(chatops.CommandSpec{
		Name:        "subs",
		Description: "查看订阅与进度 (List subscriptions)",
		Handler:     subsHandler,
	})
}

func kindLabel(lang, kind string) string {
	if kind == "tv" {
		return tr(lang, "剧集", "TV")
	}
	return tr(lang, "电影", "movie")
}

func subHandler(ctx context.Context, args []string, src chatops.Source) (chatops.Reply, error) {
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
		return okReply(tr(src.ReplyLang, "用法：/sub 关键词（电影或剧集的名字）", "usage: /sub <title>")), nil
	}
	found, err := svc.Search(ctx, keyword)
	if err != nil {
		return errReply(src.ReplyLang, "搜索失败: %v", "search failed: %v", err), nil
	}
	if len(found) == 0 {
		return okReply(tr(src.ReplyLang, "TMDB 上没有找到「"+keyword+"」", "nothing found on TMDB for "+keyword)), nil
	}
	header := tr(src.ReplyLang,
		"找到这些（回复编号订阅；剧集可以加季号，比如「2 3」是第 2 个的第 3 季，不加时订最新一季；回复别的取消）：\n",
		"Reply with a number to subscribe (for TV add a season, e.g. \"2 3\"; latest season by default):\n")
	return offerCandidates(sessions, src, found, header, subPickStep, subPickHandler), nil
}

// offerCandidates 列出候选（最多 maxSubResults 个），把它们存进会话，等回复的编号交给 pick。
func offerCandidates(sessions *Services, src chatops.Source, found []SubscribeCandidate, header, step string,
	pick func(ctx context.Context, args []string, src chatops.Source, state string) (chatops.Reply, error),
) chatops.Reply {
	if len(found) > maxSubResults {
		found = found[:maxSubResults]
	}
	var b strings.Builder
	b.WriteString(header)
	for i, c := range found {
		fmt.Fprintf(&b, "%d. %s", i+1, c.Title)
		if c.Year > 0 {
			fmt.Fprintf(&b, " (%d)", c.Year)
		}
		fmt.Fprintf(&b, " %s", kindLabel(src.ReplyLang, c.Kind))
		if c.Subscribed {
			b.WriteString(tr(src.ReplyLang, "（已订阅）", " (subscribed)"))
		}
		b.WriteString("\n")
	}
	data, err := json.Marshal(found)
	if err != nil {
		return errReply(src.ReplyLang, "保存候选失败: %v", "save candidates failed: %v", err)
	}
	state := string(data)
	sessions.Sessions.Set(src.ChannelType, src.ChannelConfID, src.ChannelUserID, chatops.SessionState{
		Step: step,
		Data: state,
		Handler: func(ctx context.Context, args []string, fsrc chatops.Source) (chatops.Reply, error) {
			return pick(ctx, args, fsrc, state)
		},
	}, subPickTTL)
	return okReply(strings.TrimRight(b.String(), "\n"))
}

// subPickHandler 处理回复的编号（可带季号）：订阅那个条目；不是编号时取消。
func subPickHandler(ctx context.Context, args []string, src chatops.Source, state string) (chatops.Reply, error) {
	c, season, done := parsePick(args, src, state, "sub")
	if done != nil {
		return *done, nil
	}
	svc := getSubscribeService()
	if svc == nil {
		return errReply(src.ReplyLang, "订阅服务不可用", "subscription service unavailable"), nil
	}
	name, err := svc.Subscribe(ctx, c.Kind, c.TMDBID, season)
	if err != nil {
		return errReply(src.ReplyLang, "订阅失败: %v", "subscribe failed: %v", err), nil
	}
	return okReply(tr(src.ReplyLang, "已订阅："+name+"，会定时搜索，RSS 里出现时也会下载", "subscribed: "+name)), nil
}

// parsePick 读回复的编号与季号：选中的候选与季号（0 是最新一季）；取消、季号不对、候选失效时 done 是要回的话。
func parsePick(args []string, src chatops.Source, state, command string) (c SubscribeCandidate, season int, done *chatops.Reply) {
	var found []SubscribeCandidate
	if err := json.Unmarshal([]byte(state), &found); err != nil {
		r := errReply(src.ReplyLang, "候选已失效，请重新发送 /"+command, "candidates expired, send /"+command+" again")
		return c, 0, &r
	}
	text := ""
	if len(args) > 0 {
		text = strings.TrimSpace(args[0])
	}
	fields := strings.Fields(text)
	n := 0
	if len(fields) > 0 {
		n, _ = strconv.Atoi(fields[0])
	}
	if n < 1 || n > len(found) || len(fields) > 2 {
		r := okReply(tr(src.ReplyLang, "已取消订阅", "canceled"))
		return c, 0, &r
	}
	c = found[n-1]
	if len(fields) == 2 {
		s, err := strconv.Atoi(fields[1])
		if err != nil || s < 1 || c.Kind != "tv" {
			r := okReply(tr(src.ReplyLang, "季号不对（只有剧集可以写季号），已取消", "invalid season, canceled"))
			return c, 0, &r
		}
		season = s
	}
	return c, season, nil
}

func subsHandler(ctx context.Context, _ []string, src chatops.Source) (chatops.Reply, error) {
	svc := getSubscribeService()
	if svc == nil {
		return errReply(src.ReplyLang, "订阅服务不可用", "subscription service unavailable"), nil
	}
	lines, err := svc.List(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "读取订阅失败: %v", "list subscriptions failed: %v", err), nil
	}
	shown := lines[:min(len(lines), maxSubLines)]
	items := make([]map[string]any, 0, len(shown))
	for _, l := range shown {
		items = append(items, map[string]any{"id": l.ID, "title": l.Title, "status": l.Status, "progress": l.Progress, "next_search": l.Next})
	}
	fields := map[string]any{"count": len(lines), "items": items, "items_more": len(lines) - len(shown)}
	if len(lines) == 0 {
		return eventReply(tr(src.ReplyLang, "还没有订阅，发送 /sub 关键词 订阅、/follow 关键词 追剧", "no subscriptions yet, send /sub <title> or /follow <show>"), notify.EventReplySubs, fields), nil
	}
	var b strings.Builder
	for i, l := range lines {
		if i >= maxSubLines {
			fmt.Fprintf(&b, tr(src.ReplyLang, "……还有 %d 个，到网页上看", "... %d more on the web UI"), len(lines)-maxSubLines)
			break
		}
		fmt.Fprintf(&b, "#%d %s · %s", l.ID, l.Title, l.Status)
		if l.Progress != "" {
			fmt.Fprintf(&b, " · %s", l.Progress)
		}
		if l.Next != "" {
			fmt.Fprintf(&b, tr(src.ReplyLang, " · 下次 %s", " · next %s"), l.Next)
		}
		b.WriteString("\n")
	}
	return eventReply(strings.TrimRight(b.String(), "\n"), notify.EventReplySubs, fields), nil
}
