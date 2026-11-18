package organize

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal/media/csf"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
)

// csfTimeout 是一次整理里交给 ChineseSubFinder 最多用多久（每个视频一个请求，单个请求 15 秒超时）。
const csfTimeout = time.Minute

// csfClient 按媒体库保存的地址与 API Key 建 ChineseSubFinder 的客户端。
func (s *Service) csfClient(lib *models.MediaLibrary) (*csf.Client, error) {
	key, err := s.cfg.Cipher.Decrypt(lib.CSFKeyEncrypted)
	if err != nil {
		return nil, fmt.Errorf("解不开保存的 ChineseSubFinder 的 API Key（%v）：到媒体库里重新填", err)
	}
	return csf.New(lib.CSFURL, key, s.cfg.CSFHTTP)
}

// csfNote 是交不上时写进整理记录的话：找不到文件与 Key 不对时写上怎么改；剧集没开刮削时 ChineseSubFinder 读不到这一集的 NFO。
func csfNote(err error, series bool, lib *models.MediaLibrary) string {
	switch {
	case errors.Is(err, csf.ErrFileNotFound):
		return err.Error() + "：看看媒体库里「ChineseSubFinder 里的库目录」填得对不对"
	case errors.Is(err, csf.ErrUnauthorized):
		return err.Error() + "：到媒体库里重新填"
	case series && !lib.Scrape && !errors.Is(err, csf.ErrUnavailable) && !errors.Is(err, csf.ErrBadConfig):
		return err.Error() + "（剧集要开着刮削：ChineseSubFinder 读每一集的 NFO）"
	}
	return err.Error()
}

// chineseSubFinder 把这次新放进库的视频交给 ChineseSubFinder 下载字幕（媒体库填了它的地址与 API Key 时；原盘不交）。
// 路径按「ChineseSubFinder 里的库目录」换成它那边看到的路径。交不上的写进那条整理记录的说明（每一句都写着 ChineseSubFinder），
// 不影响整理的结果；连不上、Key 不对、设置不对时后面的不再试。返回给这次整理的消息。
func (s *Service) chineseSubFinder(ctx context.Context, p *Plan, created []*PlanItem) []string {
	lib := p.Library
	if lib == nil || lib.CSFURL == "" || lib.CSFKeyEncrypted == "" {
		return nil
	}
	var items []*PlanItem
	for _, it := range created {
		if it.disc == nil && it.HistoryID > 0 {
			items = append(items, it)
		}
	}
	if len(items) == 0 {
		return nil
	}
	series := p.Match != nil && p.Match.MediaType == tmdb.KindTV
	notes := map[*PlanItem]string{}
	var msgs []string
	c, err := s.csfClient(lib)
	if err != nil {
		for _, it := range items {
			notes[it] = csfNote(err, series, lib)
		}
		msgs = append(msgs, csfNote(err, series, lib))
	} else {
		cctx, cancel := context.WithTimeout(ctx, csfTimeout)
		defer cancel()
		kind := csf.VideoMovie
		if series {
			kind = csf.VideoSeries
		}
		for i, it := range items {
			phys, ok := csf.PathFor(lib.Path, lib.CSFPath, it.videoPath())
			if !ok {
				notes[it] = "文件不在库目录里，没有交给 ChineseSubFinder"
				continue
			}
			_, err := c.AddJob(cctx, csf.Job{VideoType: kind, Path: phys})
			if err == nil {
				continue
			}
			note := csfNote(err, series, lib)
			if errors.Is(err, csf.ErrUnavailable) || errors.Is(err, csf.ErrUnauthorized) || errors.Is(err, csf.ErrBadConfig) || cctx.Err() != nil {
				// 后面的多半也交不上：不再试，都写上
				for _, rest := range items[i:] {
					notes[rest] = note
				}
				msgs = append(msgs, note)
				break
			}
			notes[it] = note
			msgs = append(msgs, fmt.Sprintf("%s：%s", filepath.Base(it.Target), note))
		}
	}
	for _, it := range items {
		if n := notes[it]; n != "" {
			if err := s.noteRecord(ctx, it.HistoryID, []string{n}, nil); err != nil {
				s.cfg.Logger.Warnf("[整理入库] 记下 ChineseSubFinder 的结果失败 (%s): %v", it.Target, err)
			}
		}
	}
	if len(msgs) == 0 {
		s.cfg.Logger.Infof("[整理入库] 交给 ChineseSubFinder 下载字幕：%d 个视频", len(items))
	}
	for _, m := range msgs {
		s.cfg.Logger.Warnf("[整理入库] %s", strings.TrimSpace(m))
	}
	return msgs
}

// CSFTestInput 是测试 ChineseSubFinder 的请求：ID 不为 0 且没填 Key 时用这个媒体库保存的 Key（可以先改地址再测）。
type CSFTestInput struct {
	ID  uint    `json:"id"`
	URL string  `json:"url"`
	Key *string `json:"key,omitempty"`
}

// TestCSF 访问一次 ChineseSubFinder（不建任务），确认地址与 API Key。
func (s *Service) TestCSF(ctx context.Context, in CSFTestInput) error {
	key := ""
	if in.Key != nil {
		key = strings.TrimSpace(*in.Key)
	}
	if key == "" && in.ID > 0 {
		var lib models.MediaLibrary
		if err := s.cfg.DB.WithContext(ctx).Where("id = ?", in.ID).Limit(1).Find(&lib).Error; err != nil {
			return fmt.Errorf("读取媒体库失败: %w", err)
		}
		if lib.ID == 0 {
			return ErrNotFound
		}
		if lib.CSFKeyEncrypted != "" {
			var err error
			if key, err = s.cfg.Cipher.Decrypt(lib.CSFKeyEncrypted); err != nil {
				return fmt.Errorf("解密 API Key 失败: %w", err)
			}
		}
	}
	c, err := csf.New(in.URL, key, s.cfg.CSFHTTP)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return c.Test(ctx)
}
