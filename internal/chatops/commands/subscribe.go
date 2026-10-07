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
)

// SubscribeService 是 /sub、/subs 用到的订阅能力。订阅服务比 ChatOps 晚建，用 SetSubscribeService 单独登记。
type SubscribeService interface {
	// Search 在 TMDB 上按关键字找电影与剧集（按热度排）。
	Search(ctx context.Context, keyword string) ([]SubscribeCandidate, error)
	// Subscribe 建订阅（剧集 season 为 0 时订最新一季），返回订阅的名字（剧集带季）。
	Subscribe(ctx context.Context, kind string, tmdbID, season int) (string, error)
	// List 列出订阅（新的在前）。
	List(ctx context.Context) ([]SubscribeLine, error)
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
	Title    string
	Status   string
	Progress string
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
	if len(found) > maxSubResults {
		found = found[:maxSubResults]
	}
	var b strings.Builder
	b.WriteString(tr(src.ReplyLang,
		"找到这些（回复编号订阅；剧集可以加季号，比如「2 3」是第 2 个的第 3 季，不加时订最新一季；回复别的取消）：\n",
		"Reply with a number to subscribe (for TV add a season, e.g. \"2 3\"; latest season by default):\n"))
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
		return errReply(src.ReplyLang, "保存候选失败: %v", "save candidates failed: %v", err), nil
	}
	state := string(data)
	sessions.Sessions.Set(src.ChannelType, src.ChannelConfID, src.ChannelUserID, chatops.SessionState{
		Step: subPickStep,
		Data: state,
		Handler: func(ctx context.Context, args []string, fsrc chatops.Source) (chatops.Reply, error) {
			return subPickHandler(ctx, args, fsrc, state)
		},
	}, subPickTTL)
	return okReply(strings.TrimRight(b.String(), "\n")), nil
}

// subPickHandler 处理回复的编号（可带季号）：订阅那个条目；不是编号时取消。
func subPickHandler(ctx context.Context, args []string, src chatops.Source, state string) (chatops.Reply, error) {
	var found []SubscribeCandidate
	if err := json.Unmarshal([]byte(state), &found); err != nil {
		return errReply(src.ReplyLang, "候选已失效，请重新发送 /sub", "candidates expired, send /sub again"), nil
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
		return okReply(tr(src.ReplyLang, "已取消订阅", "canceled")), nil
	}
	c := found[n-1]
	season := 0
	if len(fields) == 2 {
		s, err := strconv.Atoi(fields[1])
		if err != nil || s < 1 || c.Kind != "tv" {
			return okReply(tr(src.ReplyLang, "季号不对（只有剧集可以写季号），已取消", "invalid season, canceled")), nil
		}
		season = s
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

func subsHandler(ctx context.Context, _ []string, src chatops.Source) (chatops.Reply, error) {
	svc := getSubscribeService()
	if svc == nil {
		return errReply(src.ReplyLang, "订阅服务不可用", "subscription service unavailable"), nil
	}
	lines, err := svc.List(ctx)
	if err != nil {
		return errReply(src.ReplyLang, "读取订阅失败: %v", "list subscriptions failed: %v", err), nil
	}
	if len(lines) == 0 {
		return okReply(tr(src.ReplyLang, "还没有订阅，发送 /sub 关键词 订阅", "no subscriptions yet, send /sub <title>")), nil
	}
	var b strings.Builder
	for i, l := range lines {
		if i >= maxSubLines {
			fmt.Fprintf(&b, tr(src.ReplyLang, "……还有 %d 个，到网页上看", "... %d more on the web UI"), len(lines)-maxSubLines)
			break
		}
		fmt.Fprintf(&b, "%d. %s · %s", i+1, l.Title, l.Status)
		if l.Progress != "" {
			fmt.Fprintf(&b, " · %s", l.Progress)
		}
		b.WriteString("\n")
	}
	return okReply(strings.TrimRight(b.String(), "\n")), nil
}
