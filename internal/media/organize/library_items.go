package organize

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// 媒体库的内容（App 的「媒体库」页）：一个库里整理好的记录按条目合并，电影一部一项，剧集一季一项。

const (
	// libraryPosterWorkers 是同时取几个海报
	libraryPosterWorkers = 4
	// libraryPosterBudget 是一页的海报最多取多久，超过的留空
	libraryPosterBudget = 8 * time.Second
	// libraryMaxSpan 是一条记录最多算几集：episode_end 异常大时只算 episode
	libraryMaxSpan = 2000
)

// LibraryItemsQuery 是媒体库内容的查询。
type LibraryItemsQuery struct {
	LibraryID uint
	// Keyword 按整理时记下的片名搜索：组里有一条记录的标题含它就整组列出
	Keyword string
	Limit   int
	Offset  int
}

// LibraryItem 是媒体库里的一项：一部电影，或剧的一季。
type LibraryItem struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	Title     string `json:"title"`
	Year      int    `json:"year,omitempty"`
	Season    int    `json:"season,omitempty"`
	// Episodes 是库里有的集号（升序、去重），电影为空
	Episodes []int `json:"episodes"`
	// Files 是这一项整理好的记录数（一个视频一条）
	Files int   `json:"files"`
	Size  int64 `json:"size"`
	// PosterPath 是 TMDB 的海报（相对路径），取不到时为空
	PosterPath string `json:"poster_path,omitempty"`
	// LatestAt 是这一项最近一次入库的时间
	LatestAt time.Time `json:"latest_at"`
}

// LibraryItemPage 是一页媒体库的内容。
type LibraryItemPage struct {
	Items []LibraryItem `json:"items"`
	Total int64         `json:"total"`
}

// libraryItemSelect 是按条目合并的列：电影的季记成 0；集号记成「episode-episode_end」，在 Go 里展开。
const libraryItemSelect = `media_type AS g_type, tmdb_id AS g_tmdb, CASE WHEN media_type = ? THEN season ELSE 0 END AS g_season,
	MAX(title) AS title, MAX(year) AS year, COUNT(*) AS files, SUM(size) AS size,
	GROUP_CONCAT(DISTINCT episode || '-' || episode_end) AS episodes, MAX(julianday(created_at)) AS latest`

// LibraryItems 列出一个媒体库里整理好（status=done）、认出条目的内容，按最近入库的时刻倒序分页（时间按时刻比较，
// 不按文本：记录里的时间带各自的时区）。导入的已有文件也算。停用的库照样能看；库不存在时返回 ErrNotFound。
// 海报从 TMDB 取（有缓存），TMDB 读不到或超时的项留空，不报错；记录里没有标题的用 TMDB 的标题补上（只用于显示，不参与搜索）。
func (s *Service) LibraryItems(ctx context.Context, q LibraryItemsQuery) (LibraryItemPage, error) {
	if q.Limit <= 0 || q.Limit > 100 {
		q.Limit = 30
	}
	q.Offset = max(q.Offset, 0)
	kw := strings.TrimSpace(q.Keyword)
	if len(kw) > 200 {
		return LibraryItemPage{}, fmt.Errorf("%w: 关键字太长", ErrInvalid)
	}
	db := s.cfg.DB.WithContext(ctx)
	var lib models.MediaLibrary
	if q.LibraryID == 0 {
		return LibraryItemPage{}, fmt.Errorf("%w: 媒体库", ErrNotFound)
	}
	if err := db.Where("id = ?", q.LibraryID).Limit(1).Find(&lib).Error; err != nil {
		return LibraryItemPage{}, fmt.Errorf("读取媒体库失败: %w", err)
	}
	if lib.ID == 0 {
		return LibraryItemPage{}, fmt.Errorf("%w: 媒体库", ErrNotFound)
	}
	grouped := func() *gorm.DB {
		g := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
			Where("library_id = ? AND status = ? AND tmdb_id > 0", lib.ID, models.MediaTransferDone).
			Select(libraryItemSelect, models.MediaKindTV).
			Group("g_type, g_tmdb, g_season")
		if kw != "" {
			like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(kw) + "%"
			g = g.Having(`MAX(CASE WHEN title LIKE ? ESCAPE '\' THEN 1 ELSE 0 END) = 1`, like)
		}
		return g
	}
	var page LibraryItemPage
	if err := s.cfg.DB.WithContext(ctx).Table("(?) AS g", grouped()).Count(&page.Total).Error; err != nil {
		return LibraryItemPage{}, fmt.Errorf("读取整理记录失败: %w", err)
	}
	var rows []struct {
		GType    string  `gorm:"column:g_type"`
		GTMDB    int     `gorm:"column:g_tmdb"`
		GSeason  int     `gorm:"column:g_season"`
		Title    string  `gorm:"column:title"`
		Year     int     `gorm:"column:year"`
		Files    int     `gorm:"column:files"`
		Size     int64   `gorm:"column:size"`
		Episodes string  `gorm:"column:episodes"`
		Latest   float64 `gorm:"column:latest"`
	}
	if err := grouped().Order("latest DESC, g_tmdb DESC, g_season DESC").Limit(q.Limit).Offset(q.Offset).Scan(&rows).Error; err != nil {
		return LibraryItemPage{}, fmt.Errorf("读取整理记录失败: %w", err)
	}
	page.Items = make([]LibraryItem, 0, len(rows))
	for _, r := range rows {
		it := LibraryItem{
			MediaType: r.GType, TMDBID: r.GTMDB, Title: r.Title, Year: r.Year, Season: r.GSeason, Episodes: []int{},
			Files: r.Files, Size: r.Size, LatestAt: fromJulian(r.Latest),
		}
		if r.GType == models.MediaKindTV {
			it.Episodes = libraryEpisodes(r.Episodes)
		}
		page.Items = append(page.Items, it)
	}
	s.libraryPosters(ctx, page.Items)
	return page, nil
}

// libraryEpisodes 把「episode-episode_end」的列表（逗号分隔）展开成升序、去重的集号；集号为 0 的不算，
// episode_end 小于 episode 或比 episode 大出 libraryMaxSpan 以上时只算 episode。
func libraryEpisodes(raw string) []int {
	seen := map[int]bool{}
	out := []int{}
	add := func(n int) {
		if n > 0 && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for part := range strings.SplitSeq(raw, ",") {
		a, b, ok := strings.Cut(strings.TrimSpace(part), "-")
		if !ok {
			continue
		}
		start, err1 := strconv.Atoi(a)
		end, err2 := strconv.Atoi(b)
		if err1 != nil || err2 != nil || start <= 0 {
			continue
		}
		if end <= start || end-start > libraryMaxSpan {
			add(start)
			continue
		}
		for n := start; n <= end; n++ {
			add(n)
		}
	}
	slices.Sort(out)
	return out
}

// libraryPosters 并发从 TMDB 补上海报（标题没记下时也补上），最多 libraryPosterBudget；读不到的留空。
func (s *Service) libraryPosters(ctx context.Context, items []LibraryItem) {
	if len(items) == 0 || s.cfg.Recognizer == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, libraryPosterBudget)
	defer cancel()
	c, err := s.cfg.Recognizer.TMDB(ctx)
	if err != nil {
		return
	}
	sem := make(chan struct{}, libraryPosterWorkers)
	var wg sync.WaitGroup
	for i := range items {
		it := &items[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			d, err := c.Details(ctx, it.MediaType, it.TMDBID)
			if err != nil {
				return
			}
			it.PosterPath = d.PosterPath
			if it.Title == "" {
				it.Title = d.Title
			}
		}()
	}
	wg.Wait()
}
