package serverindex

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/models"
)

// Servers 是能用索引的服务器：启用的、索引开着、同步成功过，按编号排。countOnly 为真时只要开了「算作已入库」的。
func Servers(ctx context.Context, db *gorm.DB, countOnly bool) ([]models.MediaServer, error) {
	q := db.WithContext(ctx).Where("enabled = ? AND index_off = ? AND indexed_at IS NOT NULL", true, false)
	if countOnly {
		q = q.Where("count_library = ?", true)
	}
	var out []models.MediaServer
	if err := q.Order("id").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("读取媒体服务器失败: %w", err)
	}
	return out, nil
}

// OpenURL 是服务器里一项的打开链接（「打开链接用的地址」没填时用服务器地址）。
func OpenURL(srv models.MediaServer, itemID string) string {
	base := srv.PlayURL
	if base == "" {
		base = srv.URL
	}
	return server.ItemURL(srv.Kind, base, srv.ServerUID, itemID)
}

// Episode 是服务器里的一集。
type Episode struct {
	ItemID     string
	Season     int
	Episode    int
	EpisodeEnd int
}

// Presence 是一台服务器里有没有一部电影或剧：ItemID 是电影或剧在服务器里的编号（没有时为空），Episodes 是剧在服务器里
// 有的集（按季、集排）。
type Presence struct {
	Server   models.MediaServer
	ItemID   string
	Episodes []Episode
}

// Lookup 是能用索引的服务器里各有没有这部电影或剧（每台一项，没有的 ItemID 为空）。
func Lookup(ctx context.Context, db *gorm.DB, mediaType string, tmdbID int) ([]Presence, error) {
	servers, err := Servers(ctx, db, false)
	if err != nil || len(servers) == 0 || tmdbID <= 0 {
		return nil, err
	}
	ids := make([]uint, 0, len(servers))
	for _, s := range servers {
		ids = append(ids, s.ID)
	}
	kind := models.MediaServerItemMovie
	if mediaType == models.MediaKindTV {
		kind = models.MediaServerItemSeries
	}
	var rows []models.MediaServerItem
	if err := db.WithContext(ctx).Select("server_id", "item_id", "kind", "season", "episode", "episode_end").
		Where("server_id IN ? AND tmdb_id = ? AND removed_at IS NULL AND kind IN ?", ids, tmdbID, []string{kind, models.MediaServerItemEpisode}).
		Order("server_id, season, episode, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取索引失败: %w", err)
	}
	out := make([]Presence, 0, len(servers))
	for _, s := range servers {
		p := Presence{Server: s}
		for _, r := range rows {
			if r.ServerID != s.ID {
				continue
			}
			switch {
			case r.Kind == kind && p.ItemID == "":
				p.ItemID = r.ItemID
			case r.Kind == models.MediaServerItemEpisode && mediaType == models.MediaKindTV:
				p.Episodes = append(p.Episodes, Episode{ItemID: r.ItemID, Season: r.Season, Episode: r.Episode, EpisodeEnd: r.EpisodeEnd})
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// SeasonHit 是一集在哪台服务器里：第一台有这一集的（按服务器编号）。
type SeasonHit struct {
	Server models.MediaServer
	ItemID string
}

// SeasonEpisodes 是能用索引的服务器里一部剧一季的各集（集号 → 第一台有它的服务器与条目编号）。countOnly 为真时只看开了
// 「算作已入库」的服务器。
func SeasonEpisodes(ctx context.Context, db *gorm.DB, tmdbID, season int, countOnly bool) (map[int]SeasonHit, error) {
	servers, err := Servers(ctx, db, countOnly)
	if err != nil || len(servers) == 0 || tmdbID <= 0 {
		return map[int]SeasonHit{}, err
	}
	byID := map[uint]models.MediaServer{}
	ids := make([]uint, 0, len(servers))
	for _, s := range servers {
		byID[s.ID] = s
		ids = append(ids, s.ID)
	}
	var rows []models.MediaServerItem
	if err := db.WithContext(ctx).Select("server_id", "item_id", "episode", "episode_end").
		Where("server_id IN ? AND kind = ? AND tmdb_id = ? AND season = ? AND removed_at IS NULL AND episode > 0", ids, models.MediaServerItemEpisode, tmdbID, season).
		Order("server_id, episode, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取索引失败: %w", err)
	}
	out := map[int]SeasonHit{}
	for _, r := range rows {
		for n := r.Episode; n <= max(r.EpisodeEnd, r.Episode); n++ {
			if _, ok := out[n]; !ok {
				out[n] = SeasonHit{Server: byID[r.ServerID], ItemID: r.ItemID}
			}
		}
	}
	return out, nil
}

// Ref 是算作已入库的一集在台账里记的出处：服务器编号:条目编号。
func Ref(h SeasonHit) string {
	return strconv.FormatUint(uint64(h.Server.ID), 10) + ":" + h.ItemID
}

// EpisodeKey 是一集：剧的 TMDB 编号、季、集。
type EpisodeKey struct{ TMDBID, Season, Episode int }

// CountedEpisodes 是开了「算作已入库」的服务器里这些剧的全部集（日历用）。
func CountedEpisodes(ctx context.Context, db *gorm.DB, tmdbIDs []int) (map[EpisodeKey]bool, error) {
	out := map[EpisodeKey]bool{}
	if len(tmdbIDs) == 0 {
		return out, nil
	}
	servers, err := Servers(ctx, db, true)
	if err != nil || len(servers) == 0 {
		return out, err
	}
	ids := make([]uint, 0, len(servers))
	for _, s := range servers {
		ids = append(ids, s.ID)
	}
	var rows []models.MediaServerItem
	if err := db.WithContext(ctx).Select("tmdb_id", "season", "episode", "episode_end").
		Where("server_id IN ? AND kind = ? AND tmdb_id IN ? AND removed_at IS NULL AND episode > 0", ids, models.MediaServerItemEpisode, tmdbIDs).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取索引失败: %w", err)
	}
	for _, r := range rows {
		for n := r.Episode; n <= max(r.EpisodeEnd, r.Episode); n++ {
			out[EpisodeKey{r.TMDBID, r.Season, n}] = true
		}
	}
	return out, nil
}

// SeriesTMDB 是服务器里一部剧（按条目编号）的 TMDB 编号；索引里没有时为 0。
func SeriesTMDB(ctx context.Context, db *gorm.DB, serverID uint, seriesItemID string) (int, error) {
	var ids []int
	if err := db.WithContext(ctx).Model(&models.MediaServerItem{}).Where("server_id = ? AND item_id = ? AND kind = ?", serverID, seriesItemID, models.MediaServerItemSeries).
		Limit(1).Pluck("tmdb_id", &ids).Error; err != nil {
		return 0, fmt.Errorf("读取索引失败: %w", err)
	}
	if len(ids) == 0 {
		return 0, nil
	}
	return ids[0], nil
}

// ItemIssues 是一部电影或剧还开着的 server_removed 问题（按季、集排）。
func ItemIssues(ctx context.Context, db *gorm.DB, mediaType string, tmdbID int) ([]models.MediaIssue, error) {
	out := []models.MediaIssue{}
	if tmdbID <= 0 {
		return out, nil
	}
	if err := db.WithContext(ctx).Where("kind = ? AND status = ? AND media_type = ? AND tmdb_id = ?", models.MediaIssueServerRemoved, models.MediaIssueOpen, mediaType, tmdbID).
		Order("season, episode, id").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("读取问题失败: %w", err)
	}
	return out, nil
}

// SeasonsOf 是一台服务器里一部剧有的各季与每季的集数（季号从小到大，特别篇第 0 季排最后）。
func SeasonsOf(eps []Episode) [][2]int {
	count := map[int]map[int]bool{}
	for _, e := range eps {
		if count[e.Season] == nil {
			count[e.Season] = map[int]bool{}
		}
		for n := e.Episode; n > 0 && n <= max(e.EpisodeEnd, e.Episode); n++ {
			count[e.Season][n] = true
		}
	}
	out := make([][2]int, 0, len(count))
	for s, set := range count {
		out = append(out, [2]int{s, len(set)})
	}
	slices.SortFunc(out, func(a, b [2]int) int { return seasonOrder(a[0]) - seasonOrder(b[0]) })
	return out
}

// EpisodeCount 是这些项一共几集（同一集有几个文件时算一集）。
func EpisodeCount(eps []Episode) int {
	n := 0
	for _, s := range SeasonsOf(eps) {
		n += s[1]
	}
	return n
}

func seasonOrder(n int) int {
	if n == 0 {
		return 1 << 20
	}
	return n
}
