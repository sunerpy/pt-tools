package subscribe

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/sunerpy/pt-tools/internal/media/bangumi"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// 番剧日历（媒体中心 M9b）：探索页按星期列出 Bangumi 每日放送里这一季在播的动画；点一部时按中文名或原名加首播年份
// 在 TMDB 上找对应的剧集，找到了打开条目详情，找不到写明。

// BangumiAPI 是番剧日历用的 Bangumi 客户端（生产环境是 *bangumi.Client）。
type BangumiAPI interface {
	Calendar(ctx context.Context) ([]bangumi.CalendarDay, error)
}

// BangumiCalendar 取 Bangumi 的每日放送（缓存 6 小时）。
func (s *Service) BangumiCalendar(ctx context.Context) ([]bangumi.CalendarDay, error) {
	if s.cfg.Bangumi == nil {
		return nil, errors.New("番剧日历没有接上 Bangumi")
	}
	return s.cfg.Bangumi.Calendar(ctx)
}

// BangumiMatch 是番剧日历里一部动画在 TMDB 上对应的剧集。
type BangumiMatch struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	Title     string `json:"title"`
}

// seasonSuffixRe 是名字后面写的第几季（Bangumi 上一季是一个条目，TMDB 上一部剧是一个条目）：第二季、第2期、
// 2nd Season、Season 2、第2クール、Part 2 这些。
var seasonSuffixRe = regexp.MustCompile(`(?i)[\s:：\-–]*(?:第\s*[0-9一二三四五六七八九十百]+\s*(?:季|期|部|クール|シーズン)|` +
	`(?:[0-9]+(?:st|nd|rd|th)|second|third|final)\s+season|season\s*[0-9]+|part\s*[0-9]+)\s*$`)

// baseName 去掉名字后面写的第几季。
func baseName(name string) string {
	name = strings.TrimSpace(name)
	for {
		next := strings.TrimSpace(seasonSuffixRe.ReplaceAllString(name, ""))
		if next == name || next == "" {
			return name
		}
		name = next
	}
}

// BangumiToTMDB 在 TMDB 上找番剧日历里的一部动画（只找剧集）：先用去掉第几季的中文名、再用原名搜，在首播年份不晚于
// 这一季开播的下一年的动画里，名字（中文名或原名）完全相同的只有一个时用它；没有完全相同的、以这个名字开头（或名字以它
// 开头）的只有一个时也用它。拿不准或没搜到时返回 ErrNotFound（写明搜过的名字）。
func (s *Service) BangumiToTMDB(ctx context.Context, id int) (*BangumiMatch, error) {
	days, err := s.BangumiCalendar(ctx)
	if err != nil {
		return nil, err
	}
	var item *bangumi.CalendarItem
	for _, d := range days {
		if i := slices.IndexFunc(d.Items, func(it bangumi.CalendarItem) bool { return it.ID == id }); i >= 0 {
			item = &d.Items[i]
			break
		}
	}
	if item == nil {
		return nil, notFound("番剧日历里没有这一部（日历可能刚更新过，刷新一下再试）")
	}
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return nil, err
	}
	year := 0
	if len(item.AirDate) >= 4 {
		year, _ = strconv.Atoi(item.AirDate[:4])
	}
	var names []string
	for _, n := range []string{baseName(item.NameCN), baseName(item.Name)} {
		if n != "" && !slices.Contains(names, n) {
			names = append(names, n)
		}
	}
	keys := make([]string, 0, len(names))
	for _, n := range names {
		if k := normName(n); k != "" {
			keys = append(keys, k)
		}
	}
	for _, n := range names {
		results, err := c.Search(ctx, models.MediaKindTV, n, 0)
		if err != nil {
			return nil, err
		}
		if r := pickBangumi(results, keys, year); r != nil {
			return &BangumiMatch{MediaType: models.MediaKindTV, TMDBID: r.ID, Title: r.Title}, nil
		}
	}
	return nil, notFound(fmt.Sprintf("TMDB 上没找到对得上的剧集（搜过「%s」）", strings.Join(names, "」「")))
}

// notFoundError 是没找到（errors.Is 认作 ErrNotFound），Error() 只写原因（界面上直接显示）。
type notFoundError string

func notFound(msg string) error { return notFoundError(msg) }

func (e notFoundError) Error() string { return string(e) }

func (e notFoundError) Is(target error) bool { return target == ErrNotFound }

// pickBangumi 在 TMDB 的搜索结果里挑出对得上的动画（规则见 BangumiToTMDB）。
func pickBangumi(results []tmdb.Result, keys []string, year int) *tmdb.Result {
	var exact, prefix []*tmdb.Result
	for i := range results {
		r := &results[i]
		if !r.IsAnimation() || (year > 0 && r.Year > year+1) {
			continue
		}
		names := []string{normName(r.Title), normName(r.OriginalTitle)}
		if slices.ContainsFunc(names, func(n string) bool { return n != "" && slices.Contains(keys, n) }) {
			exact = append(exact, r)
			continue
		}
		if slices.ContainsFunc(names, func(n string) bool {
			return n != "" && slices.ContainsFunc(keys, func(k string) bool { return strings.HasPrefix(n, k) || strings.HasPrefix(k, n) })
		}) {
			prefix = append(prefix, r)
		}
	}
	switch {
	case len(exact) == 1:
		return exact[0]
	case len(exact) == 0 && len(prefix) == 1:
		return prefix[0]
	}
	return nil
}

// normName 去掉空格、标点与符号，转成小写（比名字用）。
func normName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
