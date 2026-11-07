package subscribe

import (
	"context"
	"fmt"

	"github.com/sunerpy/pt-tools/models"
)

// SubscriptionDraft 是新建订阅前给编辑页看的条目（只读，不写库）。
type SubscriptionDraft struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	// Season 是这一季：请求的 0 换成最新一季；电影是 0
	Season int `json:"season"`
	// Seasons 是一共几季，TotalEpisodes 是这一季的集数（电影都是 0）
	Seasons       int    `json:"seasons"`
	TotalEpisodes int    `json:"total_episodes"`
	Title         string `json:"title"`
	OriginalTitle string `json:"original_title"`
	// Year 是年份（剧集是这一季的）
	Year       int    `json:"year"`
	PosterPath string `json:"poster_path"`
	// Anime 是 TMDB 类型里有没有「动画」
	Anime bool `json:"anime"`
	// AutoLibraryID 是媒体库选「自动」时整理会选到的库（选法同整理，见 models.ChooseLibrary）；没有能用的库时是 0
	AutoLibraryID uint `json:"auto_library_id"`
	// SubscriptionID 是这一季已有的订阅；没有时是 0
	SubscriptionID uint `json:"subscription_id"`
}

// Draft 读新建订阅前要显示的条目：名字、年份、一共几季与这一季的集数（剧集的 season 为 0 时取最新一季）、是不是动画、
// 媒体库选「自动」时整理会选到的库，以及这一季是不是已经在追。只读，不写库；条目与季在 TMDB 上没有时报错，同新建订阅。
func (s *Service) Draft(ctx context.Context, kind string, id, season int) (*SubscriptionDraft, error) {
	if kind != models.MediaKindMovie && kind != models.MediaKindTV {
		return nil, fmt.Errorf("%w: 类型要选电影或剧集", ErrInvalid)
	}
	if id <= 0 {
		return nil, fmt.Errorf("%w: 要指定 TMDB 编号", ErrInvalid)
	}
	if kind == models.MediaKindMovie {
		season = 0
	} else if season < 0 || season > 200 {
		return nil, fmt.Errorf("%w: 季号不对", ErrInvalid)
	}
	row, d, err := s.draftWith(ctx, kind, id, season)
	if err != nil {
		return nil, err
	}
	out := &SubscriptionDraft{
		MediaType: kind, TMDBID: id, Season: row.Season, TotalEpisodes: row.TotalEpisodes, Title: row.Title,
		OriginalTitle: row.OriginalTitle, Year: row.Year, PosterPath: row.PosterPath, Anime: d.IsAnimation(),
	}
	if kind == models.MediaKindTV {
		out.Seasons = d.Seasons
	}
	db := s.cfg.DB.WithContext(ctx)
	var libs []models.MediaLibrary
	if err := db.Where("enabled = ?", true).Order("id").Find(&libs).Error; err != nil {
		return nil, fmt.Errorf("读取媒体库失败: %w", err)
	}
	if lib := models.ChooseLibrary(libs, kind, out.Anime); lib != nil {
		out.AutoLibraryID = lib.ID
	}
	var sub models.MediaSubscription
	if err := db.Select("id").Where("media_type = ? AND tmdb_id = ? AND season = ?", kind, id, out.Season).Limit(1).Find(&sub).Error; err != nil {
		return nil, fmt.Errorf("读取订阅失败: %w", err)
	}
	out.SubscriptionID = sub.ID
	return out, nil
}
