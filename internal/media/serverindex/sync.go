// Package serverindex 是媒体服务器索引：把服务器里的电影、剧与集全量同步进 media_server_items（拉全以后在一个事务里写，
// 没拉到的打删除标记、90 天后删行），同步时对 pt-tools 整理过、服务器里没有了的写待处理问题（media_issues 的
// server_removed），并提供查询：条目在哪台服务器里、一季的哪些集在服务器里、算作已入库的集、问题列表。
package serverindex

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/media/server"
	"github.com/sunerpy/pt-tools/models"
)

const (
	// MaxItems 是一台服务器最多同步多少项（电影、剧与集一起算）；超过时这一轮不写
	MaxItems = 200_000
	// SyncTimeout 是一次同步（列完全部与写库）的总时限
	SyncTimeout = 10 * time.Minute
	// PurgeAfter 是删除标记留多久：早于它、没有未处理问题的行才真正删掉
	PurgeAfter = 90 * 24 * time.Hour
	// writeBatch 是写库时一批多少行
	writeBatch = 200
)

// ErrTooMany 表示服务器里的条目超过 MaxItems。
var ErrTooMany = errors.New("媒体服务器里的条目超过 20 万项，没有同步")

// Result 是一次同步的结果。Movies、Series、Episodes 是同步以后服务器里有的；Partial 表示列的过程中服务器里的条目数变了，
// 这一轮没打删除标记；Reset 表示换了一台服务器（编号变了），旧的索引与问题清掉了。
type Result struct {
	Info     server.Info `json:"-"`
	Movies   int         `json:"movies"`
	Series   int         `json:"series"`
	Episodes int         `json:"episodes"`
	Added    int         `json:"added"`
	Updated  int         `json:"updated"`
	Removed  int         `json:"removed"`
	Restored int         `json:"restored"`
	Purged   int         `json:"purged"`
	Issues   int         `json:"issues"`
	Partial  bool        `json:"partial,omitempty"`
	Reset    bool        `json:"reset,omitempty"`
}

// Sync 同步一台服务器的索引：先 Test（读名字、版本、编号），再列到底，最后在一个事务里写库、写问题、清理、改服务器上的
// 同步结果。中途出错、超时、超量时不写索引，只把原因记进服务器的 index_error（被取消的不记）。调用方保证同一台服务器
// 不会同时同步。
func Sync(ctx context.Context, db *gorm.DB, srv models.MediaServer, c server.Client, now time.Time) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, SyncTimeout)
	defer cancel()
	res, err := doSync(ctx, db, srv, c, now)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("同步超过 %d 分钟: %w", int(SyncTimeout/time.Minute), err)
		}
		if !errors.Is(err, context.Canceled) {
			db.WithContext(context.WithoutCancel(ctx)).Model(&models.MediaServer{}).Where("id = ?", srv.ID).
				Update("index_error", truncate(err.Error(), 1024))
		}
		return res, err
	}
	return res, nil
}

func doSync(ctx context.Context, db *gorm.DB, srv models.MediaServer, c server.Client, now time.Time) (Result, error) {
	info, err := c.Test(ctx)
	if err != nil {
		return Result{}, err
	}
	items, partial, err := fetchAll(ctx, c)
	if err != nil {
		return Result{}, err
	}
	res := Result{Info: info, Partial: partial}
	rows, series := toRows(srv, items)
	for _, r := range rows {
		switch r.Kind {
		case models.MediaServerItemMovie:
			res.Movies++
		case models.MediaServerItemSeries:
			res.Series++
		case models.MediaServerItemEpisode:
			res.Episodes++
		}
	}
	txErr := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if srv.ServerUID != "" && info.ID != "" && srv.ServerUID != info.ID {
			// 换了一台服务器：旧的条目编号对不上，按删除算会把整台都当成删了
			if err := Forget(tx, srv.ID); err != nil {
				return err
			}
			res.Reset = true
		}
		var existing []models.MediaServerItem
		if err := tx.Where("server_id = ?", srv.ID).Find(&existing).Error; err != nil {
			return fmt.Errorf("读取索引失败: %w", err)
		}
		w := writer{tx: tx, srv: srv, now: now, series: series, res: &res}
		if err := w.apply(existing, rows); err != nil {
			return err
		}
		if err := w.purge(); err != nil {
			return err
		}
		upd := map[string]any{
			"indexed_at": now, "index_error": "", "index_movies": res.Movies, "index_series": res.Series, "index_episodes": res.Episodes,
		}
		if info.ID != "" {
			upd["server_uid"] = truncate(info.ID, 64)
		}
		if err := tx.Model(&models.MediaServer{}).Where("id = ?", srv.ID).Updates(upd).Error; err != nil {
			return fmt.Errorf("记下同步结果失败: %w", err)
		}
		return nil
	})
	if txErr != nil {
		return Result{}, txErr
	}
	return res, nil
}

// fetchAll 一页页列到底。返回的 partial 表示有一页报了列的过程中条目数变了。
func fetchAll(ctx context.Context, c server.Client) ([]server.Item, bool, error) {
	var all []server.Item
	cursor, partial := "", false
	for {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		page, err := c.ListItems(ctx, cursor)
		if err != nil {
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			return nil, false, err
		}
		all = append(all, page.Items...)
		if len(all) > MaxItems {
			return nil, false, ErrTooMany
		}
		partial = partial || page.Changed
		if page.Next == "" || page.Next == cursor {
			return all, partial, nil
		}
		cursor = page.Next
	}
}

// toRows 把列到的项换成索引的行（按条目编号去重，后列到的为准）：集的编号换成所在剧的，路径按服务器的路径前缀换成
// pt-tools 里的路径。第二个返回值是剧（按条目编号），写问题时取剧名。
func toRows(srv models.MediaServer, items []server.Item) (map[string]models.MediaServerItem, map[string]server.Item) {
	series := map[string]server.Item{}
	for _, it := range items {
		if it.Kind == models.MediaServerItemSeries {
			series[it.ID] = it
		}
	}
	rows := make(map[string]models.MediaServerItem, len(items))
	for _, it := range items {
		if it.ID == "" || len(it.ID) > 64 || (it.Kind == models.MediaServerItemEpisode && len(it.SeriesID) > 64) {
			continue
		}
		r := models.MediaServerItem{
			ServerID: srv.ID, ItemID: it.ID, Kind: it.Kind, TMDBID: it.TMDBID, IMDbID: truncate(it.IMDbID, 16), TVDBID: truncate(it.TVDBID, 16),
			Title: truncate(it.Title, 255), Year: it.Year, Path: it.Path, LocalPath: LocalPath(srv, it.Path),
		}
		if !it.AddedAt.IsZero() {
			at := it.AddedAt
			r.AddedAt = &at
		}
		if it.Kind == models.MediaServerItemEpisode {
			r.SeriesItemID, r.Season, r.Episode, r.EpisodeEnd = it.SeriesID, it.Season, it.Episode, it.EpisodeEnd
			r.TMDBID, r.IMDbID, r.TVDBID = 0, "", ""
			if s, ok := series[it.SeriesID]; ok {
				r.TMDBID, r.IMDbID, r.TVDBID = s.TMDBID, truncate(s.IMDbID, 16), truncate(s.TVDBID, 16)
			}
		}
		rows[it.ID] = r
	}
	return rows, series
}

// LocalPath 把媒体服务器里的路径换成 pt-tools 里的路径（服务器设置的两个前缀反过来用）；没设前缀时两边一样；
// 设了前缀而路径不在里面时为空。
func LocalPath(srv models.MediaServer, p string) string {
	if p == "" || srv.ServerPrefix == "" {
		return p
	}
	local, ok := models.MapTransferPath([]models.DownloaderPathMap{{SourcePrefix: srv.ServerPrefix, TargetPrefix: srv.LocalPrefix}}, p)
	if !ok {
		return ""
	}
	return local
}

// writer 是一次同步在事务里的写入。
type writer struct {
	tx     *gorm.DB
	srv    models.MediaServer
	now    time.Time
	series map[string]server.Item
	res    *Result
}

// sameItem 报告索引的一行有没有变（不看编号、写入时间与删除标记）。
func sameItem(a, b models.MediaServerItem) bool {
	at := func(t *time.Time) int64 {
		if t == nil {
			return 0
		}
		return t.Unix()
	}
	return a.Kind == b.Kind && a.SeriesItemID == b.SeriesItemID && a.TMDBID == b.TMDBID && a.IMDbID == b.IMDbID && a.TVDBID == b.TVDBID &&
		a.Season == b.Season && a.Episode == b.Episode && a.EpisodeEnd == b.EpisodeEnd && a.Title == b.Title && a.Year == b.Year &&
		a.Path == b.Path && a.LocalPath == b.LocalPath && at(a.AddedAt) == at(b.AddedAt)
}

// apply 写新的与变了的行，又出现的清掉删除标记（问题改成解决了），没拉到的打删除标记（对 pt-tools 整理过的写问题）。
// 这一轮列得不全（Partial）时不打删除标记。
func (w *writer) apply(existing []models.MediaServerItem, rows map[string]models.MediaServerItem) error {
	byItem := make(map[string]models.MediaServerItem, len(existing))
	for _, e := range existing {
		byItem[e.ItemID] = e
	}
	var creates []models.MediaServerItem
	for id, r := range rows {
		e, ok := byItem[id]
		if !ok {
			r.SyncedAt = w.now
			creates = append(creates, r)
			continue
		}
		restored := e.RemovedAt != nil
		if !restored && sameItem(e, r) {
			continue
		}
		r.ID, r.SyncedAt, r.RemovedAt = e.ID, w.now, nil
		if err := w.tx.Select("*").Omit("id").Where("id = ?", e.ID).Updates(&r).Error; err != nil {
			return fmt.Errorf("更新索引失败: %w", err)
		}
		if restored {
			w.res.Restored++
			if err := Clear(w.tx, removedKey(w.srv.ID, id), "媒体服务器里又有了", w.now); err != nil {
				return err
			}
		} else {
			w.res.Updated++
		}
	}
	if len(creates) > 0 {
		if err := w.tx.CreateInBatches(creates, writeBatch).Error; err != nil {
			return fmt.Errorf("写入索引失败: %w", err)
		}
		w.res.Added = len(creates)
	}
	if w.res.Partial {
		return nil
	}
	var gone []models.MediaServerItem
	for _, e := range existing {
		if _, ok := rows[e.ItemID]; !ok && e.RemovedAt == nil {
			gone = append(gone, e)
		}
	}
	for i := 0; i < len(gone); i += writeBatch {
		chunk := gone[i:min(i+writeBatch, len(gone))]
		ids := make([]uint, 0, len(chunk))
		for _, g := range chunk {
			ids = append(ids, g.ID)
		}
		if err := w.tx.Model(&models.MediaServerItem{}).Where("id IN ?", ids).Update("removed_at", w.now).Error; err != nil {
			return fmt.Errorf("记下删除标记失败: %w", err)
		}
	}
	w.res.Removed = len(gone)
	return w.raiseRemoved(gone)
}

// raiseRemoved 对没有了的电影与集里 pt-tools 整理过的写 server_removed 问题。
func (w *writer) raiseRemoved(gone []models.MediaServerItem) error {
	var withPath []string
	for _, g := range gone {
		if g.LocalPath != "" && (g.Kind == models.MediaServerItemMovie || g.Kind == models.MediaServerItemEpisode) {
			withPath = append(withPath, g.LocalPath)
		}
	}
	ours := map[string]bool{}
	for i := 0; i < len(withPath); i += 500 {
		var paths []string
		if err := w.tx.Model(&models.MediaTransferHistory{}).Where("status = ? AND target_path IN ?", models.MediaTransferDone, withPath[i:min(i+500, len(withPath))]).
			Distinct().Pluck("target_path", &paths).Error; err != nil {
			return fmt.Errorf("读取整理记录失败: %w", err)
		}
		for _, p := range paths {
			ours[p] = true
		}
	}
	for _, g := range gone {
		var mediaType, title string
		switch g.Kind {
		case models.MediaServerItemMovie:
			mediaType, title = models.MediaKindMovie, g.Title
		case models.MediaServerItemEpisode:
			mediaType, title = models.MediaKindTV, g.Title
			if s, ok := w.series[g.SeriesItemID]; ok && s.Title != "" {
				title = s.Title
			} else if name := w.seriesTitle(g.SeriesItemID); name != "" {
				title = name
			}
		default:
			continue
		}
		var hit bool
		if g.LocalPath != "" {
			hit = ours[g.LocalPath]
		} else {
			var err error
			if hit, err = w.recordedByID(g, mediaType); err != nil {
				return err
			}
		}
		if !hit {
			continue
		}
		detail := fmt.Sprintf("媒体服务器「%s」里没有了", w.srv.Name)
		if g.Path != "" {
			detail += "：" + g.Path
		}
		info := IssueInfo{MediaType: mediaType, TMDBID: g.TMDBID, Season: g.Season, Episode: g.Episode, Title: truncate(title, 255), Detail: truncate(detail, 1024)}
		if err := Raise(w.tx, removedKey(w.srv.ID, g.ItemID), info, w.now); err != nil {
			return err
		}
		w.res.Issues++
	}
	return nil
}

// seriesTitle 是索引里一部剧的名字（这一轮没列到这部剧时，例如整部剧删掉了）。
func (w *writer) seriesTitle(itemID string) string {
	if itemID == "" {
		return ""
	}
	var titles []string
	w.tx.Model(&models.MediaServerItem{}).Where("server_id = ? AND item_id = ?", w.srv.ID, itemID).Limit(1).Pluck("title", &titles)
	if len(titles) == 0 {
		return ""
	}
	return titles[0]
}

// recordedByID 是换不了路径时按编号找 pt-tools 的整理记录：电影按 TMDB 编号，集按剧的 TMDB 编号、季与集。
func (w *writer) recordedByID(g models.MediaServerItem, mediaType string) (bool, error) {
	return recordedByID(w.tx, g, mediaType)
}

func recordedByID(tx *gorm.DB, g models.MediaServerItem, mediaType string) (bool, error) {
	if g.TMDBID <= 0 {
		return false, nil
	}
	q := tx.Model(&models.MediaTransferHistory{}).Where("status = ? AND media_type = ? AND tmdb_id = ?", models.MediaTransferDone, mediaType, g.TMDBID)
	if g.Kind == models.MediaServerItemEpisode {
		if g.Episode <= 0 {
			return false, nil
		}
		last := max(g.EpisodeEnd, g.Episode)
		q = q.Where("season = ? AND episode > 0 AND episode <= ? AND MAX(episode_end, episode) >= ?", g.Season, last, g.Episode)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return false, fmt.Errorf("读取整理记录失败: %w", err)
	}
	return n > 0, nil
}

// recorded 报告服务器里的这一项（已经标了删除的也算）还有没有对应的 pt-tools 整理记录，规则同同步时写问题（raiseRemoved）：
// 有 pt-tools 的路径时按路径，换不了路径时按编号。
func recorded(tx *gorm.DB, g models.MediaServerItem, mediaType string) (bool, error) {
	if g.LocalPath == "" {
		return recordedByID(tx, g, mediaType)
	}
	var n int64
	if err := tx.Model(&models.MediaTransferHistory{}).Where("status = ? AND target_path = ?", models.MediaTransferDone, g.LocalPath).Count(&n).Error; err != nil {
		return false, fmt.Errorf("读取整理记录失败: %w", err)
	}
	return n > 0, nil
}

// purge 删掉删除标记早于 PurgeAfter、没有未处理问题的行。
func (w *writer) purge() error {
	res := w.tx.Where("server_id = ? AND removed_at IS NOT NULL AND julianday(removed_at) < julianday(?)", w.srv.ID, w.now.Add(-PurgeAfter)).
		Where("NOT EXISTS (SELECT 1 FROM media_issues i WHERE i.kind = ? AND i.status = ? AND i.server_id = media_server_items.server_id AND i.server_item_id = media_server_items.item_id)",
			models.MediaIssueServerRemoved, models.MediaIssueOpen).
		Delete(&models.MediaServerItem{})
	if res.Error != nil {
		return fmt.Errorf("清理索引失败: %w", res.Error)
	}
	w.res.Purged = int(res.RowsAffected)
	return nil
}

// Forget 删掉一台服务器的索引与 server_removed 问题（删除服务器、换了一台服务器时）。
func Forget(tx *gorm.DB, serverID uint) error {
	if err := tx.Where("server_id = ?", serverID).Delete(&models.MediaServerItem{}).Error; err != nil {
		return fmt.Errorf("清掉索引失败: %w", err)
	}
	if err := tx.Where("kind = ? AND server_id = ?", models.MediaIssueServerRemoved, serverID).Delete(&models.MediaIssue{}).Error; err != nil {
		return fmt.Errorf("清掉问题失败: %w", err)
	}
	return nil
}

// truncate 按字节截断，不截断多字节字符。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}
