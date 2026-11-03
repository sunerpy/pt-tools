package organize

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// HistoryGroup 是按条目分的一组整理记录：识别出条目的按类型、TMDB 编号与季分，没识别出的按种子名分。
type HistoryGroup struct {
	MediaType string `json:"media_type,omitempty"`
	TMDBID    int    `json:"tmdb_id,omitempty"`
	Season    int    `json:"season,omitempty"`
	// Torrent 是没识别出条目的这一组的种子名（监控目录的项、指定的路径是最后一段）
	Torrent string `json:"torrent,omitempty"`
	Title   string `json:"title,omitempty"`
	Year    int    `json:"year,omitempty"`
	Total   int    `json:"total"`
	Done    int    `json:"done"`
	Failed  int    `json:"failed"`
	Skipped int    `json:"skipped"`
	Removed int    `json:"removed"`
	// LatestAt 是组里最近更新的那条记录的时间
	LatestAt time.Time `json:"latest_at"`
}

// HistoryGroupPage 是一页按条目分的组。
type HistoryGroupPage struct {
	Groups []HistoryGroup `json:"groups"`
	Total  int64          `json:"total"`
}

// groupSelect 是按条目分组的列：没识别出条目的类型、季记成空，按种子名分。
const groupSelect = `CASE WHEN tmdb_id > 0 THEN media_type ELSE '' END AS g_type, tmdb_id AS g_tmdb,
	CASE WHEN tmdb_id > 0 THEN season ELSE 0 END AS g_season, CASE WHEN tmdb_id > 0 THEN '' ELSE torrent_name END AS g_torrent,
	MAX(title) AS title, MAX(year) AS year, COUNT(*) AS total,
	SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS done, SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS failed,
	SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS skipped, SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS removed,
	MAX(julianday(updated_at)) AS latest`

// HistoryGroups 按条目把整理记录分组（筛选条件同 History，组的条件不看），按组里最近的时间倒序分页。
// 时间按时刻比较（julianday），不按文本：记录里的时间带各自的时区。
func (s *Service) HistoryGroups(ctx context.Context, q HistoryQuery) (HistoryGroupPage, error) {
	if q.Limit <= 0 || q.Limit > 200 {
		q.Limit = 50
	}
	q.Offset = max(q.Offset, 0)
	q.MediaType, q.TMDBID, q.Season, q.Torrent = "", 0, 0, ""
	grouped := func() (*gorm.DB, error) {
		db, err := s.historyDB(ctx, q)
		if err != nil {
			return nil, err
		}
		return db.Select(groupSelect, models.MediaTransferDone, models.MediaTransferFailed, models.MediaTransferSkipped, models.MediaTransferRemoved).
			Group("g_type, g_tmdb, g_season, g_torrent"), nil
	}
	sub, err := grouped()
	if err != nil {
		return HistoryGroupPage{}, err
	}
	var page HistoryGroupPage
	if err = s.cfg.DB.WithContext(ctx).Table("(?) AS g", sub).Count(&page.Total).Error; err != nil {
		return HistoryGroupPage{}, fmt.Errorf("读取整理记录失败: %w", err)
	}
	var rows []struct {
		GType    string  `gorm:"column:g_type"`
		GTMDB    int     `gorm:"column:g_tmdb"`
		GSeason  int     `gorm:"column:g_season"`
		GTorrent string  `gorm:"column:g_torrent"`
		Title    string  `gorm:"column:title"`
		Year     int     `gorm:"column:year"`
		Total    int     `gorm:"column:total"`
		Done     int     `gorm:"column:done"`
		Failed   int     `gorm:"column:failed"`
		Skipped  int     `gorm:"column:skipped"`
		Removed  int     `gorm:"column:removed"`
		Latest   float64 `gorm:"column:latest"`
	}
	if sub, err = grouped(); err != nil {
		return HistoryGroupPage{}, err
	}
	if err = sub.Order("latest DESC, g_tmdb DESC, g_torrent").Limit(q.Limit).Offset(q.Offset).Scan(&rows).Error; err != nil {
		return HistoryGroupPage{}, fmt.Errorf("读取整理记录失败: %w", err)
	}
	page.Groups = make([]HistoryGroup, 0, len(rows))
	for _, r := range rows {
		page.Groups = append(page.Groups, HistoryGroup{
			MediaType: r.GType, TMDBID: r.GTMDB, Season: r.GSeason, Torrent: r.GTorrent, Title: r.Title, Year: r.Year,
			Total: r.Total, Done: r.Done, Failed: r.Failed, Skipped: r.Skipped, Removed: r.Removed, LatestAt: fromJulian(r.Latest),
		})
	}
	return page, nil
}

// fromJulian 把 SQLite 的 julianday（UTC）换成本地时间，精确到毫秒。
func fromJulian(jd float64) time.Time {
	return time.UnixMilli(int64(math.Round((jd - 2440587.5) * 86400000))).In(time.Local)
}

// 批量操作（Batch）的动作。
const (
	BatchRetry       = "retry"
	BatchDelete      = "delete"
	BatchDeleteFiles = "delete_files"
	BatchRescrape    = "rescrape"
	// maxBatch 是一次批量操作最多几条记录
	maxBatch = 200
)

// BatchItem 是批量操作里一条记录的结果。
type BatchItem struct {
	ID      uint   `json:"id"`
	OK      bool   `json:"ok"`
	Message string `json:"message,omitempty"`
}

// BatchResult 是一次批量操作的结果：每条记录一项，按请求里的顺序（重复的编号只算一次）。
type BatchResult struct {
	Items  []BatchItem `json:"items"`
	OK     int         `json:"ok"`
	Failed int         `json:"failed"`
}

func (r *BatchResult) add(it BatchItem) {
	r.Items = append(r.Items, it)
	if it.OK {
		r.OK++
	} else {
		r.Failed++
	}
}

// itemError 是批量操作里一条记录出错的说明（去掉「参数无效: 」）。
func itemError(id uint, err error) BatchItem {
	return BatchItem{ID: id, Message: strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")}
}

// Batch 对选中的整理记录做同一个操作，逐条回结果：
//   - retry：重新整理记录所在的种子（监控目录的项、指定的路径），同一个只排一次；排进整理队列就回，不等整理完
//     （后台没在跑时直接整理）；之前手动指定过条目的沿用那个条目；
//   - delete 只删记录；delete_files 连库里整理出的文件一起删（同 DeleteHistory，移动整理的不删）；
//   - rescrape：重新刮削记录所在的电影或剧集（同 Rescrape，只收整理好的记录），同一个媒体库里的同一部只刮一次，也是排进队列就回。
func (s *Service) Batch(ctx context.Context, ids []uint, action string) (*BatchResult, error) {
	seen := map[uint]bool{}
	ids = slices.DeleteFunc(slices.Clone(ids), func(id uint) bool {
		dup := id == 0 || seen[id]
		seen[id] = true
		return dup
	})
	switch {
	case len(ids) == 0:
		return nil, fmt.Errorf("%w: 要选整理记录", ErrInvalid)
	case len(ids) > maxBatch:
		return nil, fmt.Errorf("%w: 一次最多 %d 条", ErrInvalid, maxBatch)
	}
	res := &BatchResult{Items: make([]BatchItem, 0, len(ids))}
	switch action {
	case BatchRetry:
		s.batchRetry(ctx, ids, res)
	case BatchDelete, BatchDeleteFiles:
		for _, id := range ids {
			kept, err := s.DeleteHistory(ctx, id, action == BatchDeleteFiles)
			switch {
			case err != nil:
				res.add(itemError(id, err))
			case len(kept) > 0:
				res.add(BatchItem{ID: id, OK: true, Message: "这些文件留着：" + strings.Join(kept, "；")})
			default:
				res.add(BatchItem{ID: id, OK: true})
			}
		}
	case BatchRescrape:
		s.batchRescrape(ctx, ids, res)
	default:
		return nil, fmt.Errorf("%w: 操作要选 retry、delete、delete_files 或 rescrape", ErrInvalid)
	}
	return res, nil
}

// batchRetry 按排队的键（下载器与种子、监控目录的项、路径）去重，每个只重试一次。
func (s *Service) batchRetry(ctx context.Context, ids []uint, res *BatchResult) {
	first := map[string]uint{}
	for _, id := range ids {
		row, err := s.historyRow(ctx, id)
		if err != nil {
			res.add(itemError(id, err))
			continue
		}
		req, err := retryRequest(row).normalized()
		if err != nil {
			res.add(itemError(id, err))
			continue
		}
		key := jobKey(req.DownloaderID, "", req.queueHash())
		if prev, ok := first[key]; ok {
			res.add(BatchItem{ID: id, OK: true, Message: fmt.Sprintf("和记录 %d 是同一个种子（或同一项），一起重试", prev)})
			continue
		}
		first[key] = id
		if !s.isRunning() {
			s.mu.Lock()
			got, oerr := s.organize(ctx, req, models.MediaTriggerManual)
			s.mu.Unlock()
			if oerr != nil {
				res.add(itemError(id, oerr))
				continue
			}
			res.add(BatchItem{ID: id, OK: true, Message: resultNote(got)})
			continue
		}
		res.add(queued(id, s.put(job{req: req, trigger: models.MediaTriggerManual})))
	}
}

// batchRescrape 按（媒体库、类型、TMDB 编号）去重，每一部只重新刮削一次。
func (s *Service) batchRescrape(ctx context.Context, ids []uint, res *BatchResult) {
	first := map[string]uint{}
	for _, id := range ids {
		row, lib, err := s.rescrapeBase(ctx, id)
		if err != nil {
			res.add(itemError(id, err))
			continue
		}
		key := fmt.Sprintf("%d|%s|%d", lib.ID, row.MediaType, row.TMDBID)
		if prev, ok := first[key]; ok {
			res.add(BatchItem{ID: id, OK: true, Message: fmt.Sprintf("和记录 %d 是同一部，一起重新刮削", prev)})
			continue
		}
		first[key] = id
		if !s.isRunning() {
			s.mu.Lock()
			got, rerr := s.rescrape(ctx, id)
			s.mu.Unlock()
			if rerr != nil {
				res.add(itemError(id, rerr))
				continue
			}
			res.add(BatchItem{ID: id, OK: true, Message: fmt.Sprintf("刮削了 %d 条记录，写了 %d 个文件", got.Records, got.Written)})
			continue
		}
		res.add(queued(id, s.put(job{rescrape: id})))
	}
}

// queued 是排进队列的结果。
func queued(id uint, put int) BatchItem {
	switch put {
	case queueFull:
		return BatchItem{ID: id, Message: "整理队列已满，稍后再试"}
	case queueDup:
		return BatchItem{ID: id, OK: true, Message: "已经在整理队列里"}
	}
	return BatchItem{ID: id, OK: true, Message: "排进了整理队列"}
}

// resultNote 是直接整理完的一句话说明。
func resultNote(r *Result) string {
	if r.Plan != nil && r.Plan.Problem != "" {
		return r.Plan.Problem
	}
	return fmt.Sprintf("新入库 %d 个，已在库里 %d 个，跳过 %d 个，失败 %d 个", r.Created, r.Done, r.Skipped, r.Failed)
}
