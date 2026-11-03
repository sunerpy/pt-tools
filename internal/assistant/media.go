package assistant

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"

	"github.com/sunerpy/pt-tools/internal/redact"
)

// 整理入库认不出条目时让 AI 认：把种子名、站点标题与 TMDB 上搜到的候选发给助手的模型，让它挑一个或说都不是。
// 只认、不整理：整理由用户看了预览以后自己点。

// mediaChooseTimeout 是一次让 AI 认条目等模型的上限（测试里改短）。
var mediaChooseTimeout = 30 * time.Second

// mediaOverviewRunes 是候选的简介发给模型的字数。
const mediaOverviewRunes = 80

// MediaCandidate 是让 AI 挑的一个候选条目。
type MediaCandidate struct {
	// Kind 是 movie 或 tv
	Kind          string
	Title         string
	OriginalTitle string
	Year          int
	Overview      string
}

// MediaChoice 是 AI 认的结果：Index 是选中的候选下标（从 0 开始），-1 是都不是；Reason 是模型给的理由。
type MediaChoice struct {
	Index  int
	Reason string
	Usage  Usage
}

// ChooseMedia 让助手的模型从候选里挑出种子对应的条目（一次不带工具的对话，最多等 30 秒）。回答写「都不是」、
// 第一个编号超出范围或没有编号时 Index 为 -1。助手没启用或没配好时返回 ErrDisabled（见 Ready）。
func (s *Service) ChooseMedia(ctx context.Context, torrent, siteTitle string, cands []MediaCandidate) (MediaChoice, error) {
	if len(cands) == 0 {
		return MediaChoice{}, fmt.Errorf("%w: 没有候选", ErrInvalid)
	}
	eff, err := s.effectiveSettings(ctx)
	if err != nil {
		return MediaChoice{}, err
	}
	var b strings.Builder
	b.WriteString("下面是 PT 站点上一个种子的名字和 TMDB 上搜到的候选条目，请判断这个种子是哪一个条目。" +
		"第一行只写编号；都不是时写「都不是」。第二行用一句话说理由。\n\n")
	fmt.Fprintf(&b, "种子名：%s\n", redact.Text(torrent))
	if t := strings.TrimSpace(siteTitle); t != "" && t != torrent {
		fmt.Fprintf(&b, "站点上的标题：%s\n", redact.Text(t))
	}
	b.WriteString("候选：\n")
	for i, c := range cands {
		kind := "电影"
		if c.Kind == "tv" {
			kind = "剧集"
		}
		fmt.Fprintf(&b, "%d. %s：%s", i+1, kind, c.Title)
		var extra []string
		if c.OriginalTitle != "" && c.OriginalTitle != c.Title {
			extra = append(extra, c.OriginalTitle)
		}
		if c.Year > 0 {
			extra = append(extra, strconv.Itoa(c.Year))
		}
		if len(extra) > 0 {
			fmt.Fprintf(&b, "（%s）", strings.Join(extra, "，"))
		}
		if o := strings.TrimSpace(c.Overview); o != "" {
			fmt.Fprintf(&b, "：%s", truncateRunes(o, mediaOverviewRunes))
		}
		b.WriteString("\n")
	}
	var used Usage
	ctx = WithUsageSink(ctx, func(u Usage) { used = u })
	reply, err := s.askOnce(ctx, eff.llm, eff.llm.Model, openai.UserMessage(b.String()), mediaChooseTimeout)
	if err != nil {
		return MediaChoice{}, modelError(err, eff.llm.APIKey)
	}
	index, reason := parseMediaChoice(reply, len(cands))
	return MediaChoice{Index: index, Reason: redact.Text(reason), Usage: used}, nil
}

// parseMediaChoice 从回答里取选中的下标（第一行写了「都不是」、第一个编号超出范围或没有编号时为 -1）与理由。
func parseMediaChoice(reply string, n int) (int, string) {
	reply = strings.TrimSpace(stripThinking(reply))
	first, rest, _ := strings.Cut(reply, "\n")
	index := -1
	if !strings.Contains(first, "都不是") {
		if m := numberPattern.FindStringIndex(reply); m != nil {
			if v, err := strconv.Atoi(reply[m[0]:m[1]]); err == nil && v >= 1 && v <= n {
				index = v - 1
			}
		}
	}
	reason := strings.TrimSpace(rest)
	if reason == "" {
		// 写在一行里的：去掉开头的编号或「都不是」
		reason = strings.TrimLeft(strings.TrimPrefix(strings.TrimLeft(first, "0123456789"), "都不是"), " .、，,:：。")
	}
	return index, truncateRunes(strings.TrimSpace(reason), 200)
}
