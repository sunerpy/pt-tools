package assistant

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"

	"github.com/sunerpy/pt-tools/internal/redact"
)

// 订阅下载前让 AI 复核（媒体中心 M9b）：推送一个候选种子以前，把订阅要的条目与种子的标题、副标题、解析出的信息发给助手的模型，
// 问它这个种子是不是这个条目（剧集还要是这一季）。只回答是或不是，不推送、不改设置。

// reviewTimeout 是一次复核等模型的上限（测试里改短）。
var reviewTimeout = 30 * time.Second

// CandidateReview 是要复核的一个候选：订阅的条目与站点上的种子。
type CandidateReview struct {
	// Kind 是 movie 或 tv
	Kind          string
	Title         string
	OriginalTitle string
	Year          int
	// Season 是剧集的季（电影为 0）
	Season  int
	Aliases []string
	// TorrentTitle、TorrentSubtitle 是站点上的标题与副标题；Parsed 是从标题解析出的信息（一行）
	TorrentTitle    string
	TorrentSubtitle string
	Parsed          string
}

// ReviewVerdict 是复核的结论：Match 为真是「是」；Reason 是模型给的理由。
type ReviewVerdict struct {
	Match  bool
	Reason string
	Usage  Usage
}

// ErrReviewUnclear 是模型的回答看不出是还是不是。
var ErrReviewUnclear = errors.New("看不出模型回答的是还是不是")

// maxReviewAliases 是发给模型的别名个数上限。
const maxReviewAliases = 8

// ReviewCandidate 让助手的模型复核一个候选种子是不是订阅要的条目（一次不带工具的对话，最多等 30 秒）。回答的第一行看不出
// 是还是不是时返回 ErrReviewUnclear。助手没启用或没配好时返回 ErrDisabled（见 Ready）。
func (s *Service) ReviewCandidate(ctx context.Context, in CandidateReview) (ReviewVerdict, error) {
	if strings.TrimSpace(in.TorrentTitle) == "" || strings.TrimSpace(in.Title) == "" {
		return ReviewVerdict{}, fmt.Errorf("%w: 没有条目或种子", ErrInvalid)
	}
	eff, err := s.effectiveSettings(ctx)
	if err != nil {
		return ReviewVerdict{}, err
	}
	var b strings.Builder
	what := "这部电影"
	if in.Kind == "tv" {
		what = "这部剧的这一季"
	}
	fmt.Fprintf(&b, "下面是要下载的条目和 PT 站点上的一个种子，请判断这个种子是不是%s。第一行只写「是」或「不是」，第二行用一句话说理由。\n\n", what)
	kind := "电影"
	if in.Kind == "tv" {
		kind = "剧集"
	}
	fmt.Fprintf(&b, "条目：%s：%s", kind, in.Title)
	var extra []string
	if in.OriginalTitle != "" && in.OriginalTitle != in.Title {
		extra = append(extra, in.OriginalTitle)
	}
	// 剧集的年份是这一季开播的年份，写在季的后面：不写在名字后面，模型就不会把它当成这部剧首播的年份，
	// 把标题里写着首播年份的种子说成不是
	season := in.Kind == "tv" && in.Season > 0
	if in.Year > 0 && !season {
		extra = append(extra, strconv.Itoa(in.Year))
	}
	if len(extra) > 0 {
		fmt.Fprintf(&b, "（%s）", strings.Join(extra, "，"))
	}
	if season {
		fmt.Fprintf(&b, "，第 %d 季", in.Season)
		if in.Year > 0 {
			fmt.Fprintf(&b, "（%d 年开播）", in.Year)
		}
	}
	b.WriteString("\n")
	var aliases []string
	for _, a := range in.Aliases {
		if a = strings.TrimSpace(a); a != "" && a != in.Title && a != in.OriginalTitle && len(aliases) < maxReviewAliases {
			aliases = append(aliases, a)
		}
	}
	if len(aliases) > 0 {
		fmt.Fprintf(&b, "别名：%s\n", strings.Join(aliases, "、"))
	}
	fmt.Fprintf(&b, "种子标题：%s\n", redact.Text(in.TorrentTitle))
	if t := strings.TrimSpace(in.TorrentSubtitle); t != "" {
		fmt.Fprintf(&b, "副标题：%s\n", redact.Text(t))
	}
	if p := strings.TrimSpace(in.Parsed); p != "" {
		fmt.Fprintf(&b, "从标题解析出：%s\n", redact.Text(p))
	}
	var used Usage
	ctx = WithUsageSink(ctx, func(u Usage) { used = u })
	reply, err := s.askOnce(ctx, eff.llm, eff.llm.Model, openai.UserMessage(b.String()), reviewTimeout)
	if err != nil {
		return ReviewVerdict{}, modelError(err, eff.llm.APIKey)
	}
	match, reason, ok := parseReview(reply)
	if !ok {
		return ReviewVerdict{Usage: used}, ErrReviewUnclear
	}
	return ReviewVerdict{Match: match, Reason: redact.Text(reason), Usage: used}, nil
}

// reviewWords 是回答第一行开头的结论：先比长的（「不是」要在「是」前面）。
var reviewWords = []struct {
	word  string
	match bool
}{{"不是", false}, {"否", false}, {"no", false}, {"是", true}, {"yes", true}}

// parseReview 从回答里取结论与理由：第一行开头写「不是」「否」（no）是不是，写「是」（yes）是是；都不是时 ok 为假。
func parseReview(reply string) (match bool, reason string, ok bool) {
	reply = strings.TrimSpace(stripThinking(reply))
	first, rest, _ := strings.Cut(reply, "\n")
	head := strings.TrimLeft(strings.TrimSpace(first), "*#>「\"' ")
	lower := strings.ToLower(head)
	for _, w := range reviewWords {
		if !strings.HasPrefix(lower, w.word) {
			continue
		}
		after := head[len(w.word):]
		// 英文的 yes、no 后面要是词的边界（不认 nothing、yesterday）
		if r := []rune(after); w.word[0] < 0x80 && len(r) > 0 && (r[0] >= 'a' && r[0] <= 'z' || r[0] >= 'A' && r[0] <= 'Z') {
			continue
		}
		match, ok = w.match, true
		reason = strings.TrimSpace(rest)
		if reason == "" {
			// 写在一行里的：去掉开头的结论
			reason = strings.TrimLeft(after, "*」\"' .、，,:：。!！")
		}
		return match, truncateRunes(strings.TrimSpace(reason), 200), ok
	}
	return false, "", false
}
