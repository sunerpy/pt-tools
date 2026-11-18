package organize

import (
	"context"
	"time"

	"github.com/sunerpy/pt-tools/models"
)

// 新的一集常常在播出后几个小时就整理入库了，那时 TMDB 还没有这一集的截图、简介，有的连集标题都没有：刮削只能写
// 「第 N 集」、没有简介与截图，记成部分刮削。以前只有开了「每周补一次缺的刮削」的媒体库才会再刮，没开的就一直缺着
// （2026-11-11《伟大的长征》第 21–55 集里 27 集没有截图与简介）。现在部分刮削与刮削失败的记录在整理以后的
// 1、3、7、14 天各自动补刮一次，不看那个开关；之后还缺的，开着每周补刮的媒体库照旧每周再补。

// retryStages 是部分刮削与刮削失败的记录在整理（记下记录）以后自动补刮的时间点。
var retryStages = []time.Duration{24 * time.Hour, 3 * 24 * time.Hour, 7 * 24 * time.Hour, 14 * 24 * time.Hour}

// retryMax 是一个媒体库一次最多自动补刮几个条目（剩下的下一次再补）。
const retryMax = fillMax

// scrapeRetryDue 报告一条部分刮削或刮削失败的记录这时候该不该自动补刮：到了某个补刮的时间点，上次刮削又在这个时间点以前。
func scrapeRetryDue(created time.Time, scraped *time.Time, now time.Time) bool {
	var at time.Time
	for _, d := range retryStages {
		if t := created.Add(d); !t.After(now) {
			at = t
		}
	}
	if at.IsZero() {
		return false
	}
	return scraped == nil || scraped.Before(at)
}

// scrapeRetry 把到了补刮时间的部分刮削、刮削失败的已整理记录所在的条目排进队列重新刮削（只补没有的文件，见 rescrapeWith）。
// 只看开着刮削的媒体库；一个条目排一次（重新刮削整部电影或剧集），每个库一次最多 retryMax 个，先补刮削得最早的。
func (s *Service) scrapeRetry(ctx context.Context) {
	now := s.cfg.Now()
	var libs []models.MediaLibrary
	if err := s.cfg.DB.WithContext(ctx).Where("scrape = ?", true).Order("id").Find(&libs).Error; err != nil {
		s.cfg.Logger.Warnf("[整理入库] 读取要自动补刮的媒体库失败: %v", err)
		return
	}
	last := retryStages[len(retryStages)-1]
	for _, lib := range libs {
		var rows []models.MediaTransferHistory
		// 先在库里粗筛：整理满了第一个时间点、最后一个时间点以后还没刮过的；准确的判断在下面
		if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
			Select("id", "media_type", "tmdb_id", "created_at", "scraped_at").
			Where("library_id = ? AND status = ? AND tmdb_id > 0 AND target_path <> '' AND scrape_status IN ?",
				lib.ID, models.MediaTransferDone, []string{models.MediaScrapePartial, models.MediaScrapeFailed}).
			Where("julianday(created_at) <= julianday(?)", now.Add(-retryStages[0]).UTC()).
			Where("julianday(COALESCE(scraped_at, created_at)) < julianday(created_at) + ?", last.Hours()/24).
			Order("julianday(COALESCE(scraped_at, created_at))").Find(&rows).Error; err != nil {
			s.cfg.Logger.Warnf("[整理入库] 读取媒体库「%s」要自动补刮的记录失败: %v", lib.Name, err)
			continue
		}
		type show struct {
			kind string
			id   int
		}
		picked := map[show]bool{}
		queued := 0
		for _, r := range rows {
			k := show{r.MediaType, r.TMDBID}
			if picked[k] || !scrapeRetryDue(r.CreatedAt, r.ScrapedAt, now) {
				continue
			}
			if queued >= retryMax {
				break
			}
			if s.put(job{rescrape: r.ID, fill: true}) == queueFull {
				s.cfg.Logger.Warnf("[整理入库] 整理队列已满，媒体库「%s」的自动补刮排了 %d 个条目，剩下的下次再补", lib.Name, queued)
				return
			}
			picked[k] = true
			queued++
		}
		if queued > 0 {
			s.cfg.Logger.Infof("[整理入库] 媒体库「%s」自动补刮 %d 个条目（部分刮削的，整理后 1、3、7、14 天各补一次）", lib.Name, queued)
		}
	}
}
