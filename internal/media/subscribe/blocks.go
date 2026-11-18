package subscribe

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/models"
)

// 种子黑名单：用户说「不要」的种子（站点上的种子编号），订阅不再选，按条目搜索标成已拉黑。按条目存（不按订阅编号），
// 删了订阅再建仍然有效；TMDB 编号为 0 的对所有条目生效。

const (
	// maxBlockReason 是原因最多几个字
	maxBlockReason = 100
	// maxBlockItems 是一次最多拉黑几个种子（同一资源在几个站点上的种子一起拉黑）
	maxBlockItems = 50
)

// BlockItem 是要拉黑的一个种子。
type BlockItem struct {
	Site      string `json:"site"`
	TorrentID string `json:"torrent_id"`
	Title     string `json:"title"`
}

// BlockInput 是一次拉黑：作用于一个条目（剧集是其中一季），TMDBID 为 0 时对所有条目生效。
type BlockInput struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	Season    int    `json:"season"`
	// MediaTitle 是条目的名字（带季），列表里写作用于哪个条目；为空时有订阅就用订阅的名字
	MediaTitle string      `json:"media_title"`
	Reason     string      `json:"reason"`
	Items      []BlockItem `json:"items"`
	// ExcludeGroup 为真时把第一个种子的制作组加进这一季订阅的标题排除词（「以后也不要这个制作组」）；要有订阅
	ExcludeGroup bool `json:"exclude_group"`
}

// BlockResult 是拉黑的结果：写好的行，以及加进标题排除词的制作组（没有加时为空）。
type BlockResult struct {
	Blocks        []models.MediaTorrentBlock `json:"blocks"`
	ExcludedGroup string                     `json:"excluded_group,omitempty"`
}

// blockKey 是认一个种子的键：站点名不分大小写。
func blockKey(site, torrentID string) string {
	return strings.ToLower(strings.TrimSpace(site)) + "|" + strings.TrimSpace(torrentID)
}

// BlockKey 是认一个种子的键（站点名不分大小写），按它查 GlobalBlocks 的结果。
func BlockKey(site, torrentID string) string { return blockKey(site, torrentID) }

// GlobalBlocks 是对所有条目拉黑的种子（关键字搜索里标「已拉黑」），按 BlockKey 查。
func (s *Service) GlobalBlocks(ctx context.Context) (map[string]models.MediaTorrentBlock, error) {
	var rows []models.MediaTorrentBlock
	if err := s.cfg.DB.WithContext(ctx).Where("tmdb_id = 0").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取黑名单失败: %w", err)
	}
	out := make(map[string]models.MediaTorrentBlock, len(rows))
	for _, r := range rows {
		out[blockKey(r.SiteName, r.TorrentID)] = r
	}
	return out, nil
}

// blockIndex 是一个条目适用的黑名单（这个条目的，加上对所有条目的；两种都有时用这个条目的）。
type blockIndex map[string]models.MediaTorrentBlock

func (b blockIndex) get(site, torrentID string) (models.MediaTorrentBlock, bool) {
	row, ok := b[blockKey(site, torrentID)]
	return row, ok
}

// blockWhy 是运行记录里拉黑的写法。
func blockWhy(b models.MediaTorrentBlock) string {
	if b.Reason == "" {
		return "已拉黑"
	}
	return "已拉黑：" + b.Reason
}

// normalize 检查并整理拉黑的条目：剧集要具体的季；对所有条目的类型为空、季为 0。
func (in *BlockInput) normalize() error {
	in.Reason = strings.TrimSpace(in.Reason)
	if utf8.RuneCountInString(in.Reason) > maxBlockReason {
		return fmt.Errorf("%w: 原因最多 %d 个字", ErrInvalid, maxBlockReason)
	}
	in.MediaTitle = truncate(strings.TrimSpace(in.MediaTitle), 255)
	switch {
	case in.TMDBID < 0:
		return fmt.Errorf("%w: TMDB 编号不对", ErrInvalid)
	case in.TMDBID == 0:
		in.MediaType, in.Season, in.MediaTitle, in.ExcludeGroup = "", 0, "", false
	case in.MediaType == models.MediaKindMovie:
		in.Season = 0
	case in.MediaType == models.MediaKindTV:
		if in.Season <= 0 || in.Season > 200 {
			return fmt.Errorf("%w: 要指定第几季", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: 类型要选电影或剧集", ErrInvalid)
	}
	if len(in.Items) == 0 {
		return fmt.Errorf("%w: 没有要拉黑的种子", ErrInvalid)
	}
	if len(in.Items) > maxBlockItems {
		return fmt.Errorf("%w: 一次最多拉黑 %d 个种子", ErrInvalid, maxBlockItems)
	}
	for i := range in.Items {
		it := &in.Items[i]
		it.Site, it.TorrentID, it.Title = strings.TrimSpace(it.Site), strings.TrimSpace(it.TorrentID), truncate(strings.TrimSpace(it.Title), 512)
		if it.Site == "" || it.TorrentID == "" || len(it.Site) > 64 || len(it.TorrentID) > 128 {
			return fmt.Errorf("%w: 种子的站点或编号不对", ErrInvalid)
		}
		if strings.HasPrefix(it.Site, models.MediaAdoptedSitePrefix) {
			return fmt.Errorf("%w: 下载器里认领的种子不是站点上的种子，不能拉黑", ErrInvalid)
		}
	}
	return nil
}

// Blocks 列出全部拉黑的种子，新的在前。
func (s *Service) Blocks(ctx context.Context) ([]models.MediaTorrentBlock, error) {
	var rows []models.MediaTorrentBlock
	if err := s.cfg.DB.WithContext(ctx).Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取黑名单失败: %w", err)
	}
	return orEmpty(rows), nil
}

// AddBlocks 拉黑几个种子（见 BlockInput）。同一个种子对同一个条目已经拉黑过时改成这次的原因，不报错。
// 这个条目有订阅时在订阅的运行记录里记一笔。
func (s *Service) AddBlocks(ctx context.Context, in BlockInput) (*BlockResult, error) {
	res, sub, err := s.addBlocks(ctx, in)
	if err != nil || sub == nil {
		return res, err
	}
	msg := "不要这个种子：" + in.Items[0].Title
	if n := len(res.Blocks); n > 1 {
		msg = fmt.Sprintf("不要这个资源（%d 个站点上的种子）：%s", n, in.Items[0].Title)
	}
	s.manualRun(ctx, sub, msg+blockNote(in.Reason, res.ExcludedGroup))
	return res, nil
}

// blockNote 是运行记录里拉黑的补充：原因与加进标题排除词的制作组。
func blockNote(reason, group string) string {
	out := ""
	if r := strings.TrimSpace(reason); r != "" {
		out += "（" + r + "）"
	}
	if group != "" {
		out += "；以后不要制作组 " + group + "（加进了标题排除词）"
	}
	return out
}

// addBlocks 写黑名单、按需加标题排除词，返回结果与这个条目的订阅（没有时为空）；不记运行记录。
func (s *Service) addBlocks(ctx context.Context, in BlockInput) (*BlockResult, *models.MediaSubscription, error) {
	if err := in.normalize(); err != nil {
		return nil, nil, err
	}
	var sub *models.MediaSubscription
	if in.TMDBID > 0 {
		var row models.MediaSubscription
		if err := s.cfg.DB.WithContext(ctx).Where("media_type = ? AND tmdb_id = ? AND season = ?", in.MediaType, in.TMDBID, in.Season).
			Order("id").Limit(1).Find(&row).Error; err != nil {
			return nil, nil, fmt.Errorf("读取订阅失败: %w", err)
		}
		if row.ID != 0 {
			sub = &row
			if in.MediaTitle == "" {
				in.MediaTitle = subTitle(sub)
			}
		}
	}
	group := ""
	if in.ExcludeGroup {
		if sub == nil {
			return nil, nil, fmt.Errorf("%w: 这个条目没有订阅，不能加标题排除词", ErrInvalid)
		}
		if group = s.parse(ctx, in.Items[0].Title, "").Group; group == "" {
			return nil, nil, fmt.Errorf("%w: 认不出这个种子的制作组", ErrInvalid)
		}
	}
	rows, err := s.saveBlocks(ctx, in)
	if err != nil {
		return nil, nil, err
	}
	res := &BlockResult{Blocks: rows}
	if group != "" {
		added, err := s.excludeGroup(ctx, sub.ID, group)
		if err != nil {
			return res, sub, err
		}
		if added {
			res.ExcludedGroup = group
		}
	}
	return res, sub, nil
}

// saveBlocks 写黑名单（同一个键已有时更新标题、原因与条目名），返回写好的行。
func (s *Service) saveBlocks(ctx context.Context, in BlockInput) ([]models.MediaTorrentBlock, error) {
	now := s.cfg.Now()
	rows := make([]models.MediaTorrentBlock, 0, len(in.Items))
	seen := map[string]bool{}
	for _, it := range in.Items {
		if k := blockKey(it.Site, it.TorrentID); !seen[k] {
			seen[k] = true
			rows = append(rows, models.MediaTorrentBlock{
				SiteName: it.Site, TorrentID: it.TorrentID, MediaType: in.MediaType, TMDBID: in.TMDBID, Season: in.Season,
				MediaTitle: in.MediaTitle, Title: it.Title, Reason: in.Reason, CreatedAt: now,
			})
		}
	}
	db := s.cfg.DB.WithContext(ctx)
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "site_name"}, {Name: "torrent_id"}, {Name: "media_type"}, {Name: "tmdb_id"}, {Name: "season"}},
		DoUpdates: clause.AssignmentColumns([]string{"title", "reason", "media_title"}),
	}).Create(&rows).Error; err != nil {
		return nil, fmt.Errorf("保存黑名单失败: %w", err)
	}
	// 更新已有的行时 Create 回填的编号不可靠：按键读回
	out := make([]models.MediaTorrentBlock, 0, len(rows))
	for _, r := range rows {
		var got models.MediaTorrentBlock
		if err := db.Where("site_name = ? AND torrent_id = ? AND media_type = ? AND tmdb_id = ? AND season = ?",
			r.SiteName, r.TorrentID, r.MediaType, r.TMDBID, r.Season).Limit(1).Find(&got).Error; err != nil {
			return nil, fmt.Errorf("读取黑名单失败: %w", err)
		}
		out = append(out, got)
	}
	return out, nil
}

// RemoveBlock 把一个种子移出黑名单。
func (s *Service) RemoveBlock(ctx context.Context, id uint) error {
	res := s.cfg.DB.WithContext(ctx).Delete(&models.MediaTorrentBlock{}, id)
	if res.Error != nil {
		return fmt.Errorf("移出黑名单失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// blocksFor 是一个条目适用的黑名单：这个条目的（剧集是这一季），加上对所有条目的。
func (s *Service) blocksFor(ctx context.Context, mediaType string, tmdbID, season int) (blockIndex, error) {
	var rows []models.MediaTorrentBlock
	if err := s.cfg.DB.WithContext(ctx).Where("(media_type = ? AND tmdb_id = ? AND season = ?) OR tmdb_id = 0", mediaType, tmdbID, season).
		Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取黑名单失败: %w", err)
	}
	idx := blockIndex{}
	for _, r := range rows {
		k := blockKey(r.SiteName, r.TorrentID)
		if old, ok := idx[k]; ok && old.TMDBID != 0 {
			continue
		}
		idx[k] = r
	}
	return idx, nil
}

// excludeGroup 把制作组加进订阅的标题排除词；已经有了时不重复加（返回假）。
func (s *Service) excludeGroup(ctx context.Context, subID uint, group string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, err := s.subRow(ctx, subID)
	if err != nil {
		return false, err
	}
	kw := normalizeKeywordText(group)
	for _, old := range splitKeywords(sub.ExcludeKeywords) {
		if old == kw {
			return false, nil
		}
	}
	next := group
	if strings.TrimSpace(sub.ExcludeKeywords) != "" {
		next = strings.TrimRight(sub.ExcludeKeywords, "\n") + "\n" + group
	}
	if len(next) > 1024 {
		return false, fmt.Errorf("%w: 标题排除词太长了，先在订阅的「修改」里删掉一些", ErrInvalid)
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", subID).
		Updates(map[string]any{"exclude_keywords": next, "updated_at": s.cfg.Now()}).Error; err != nil {
		return false, fmt.Errorf("保存订阅失败: %w", err)
	}
	return true, nil
}

// BlockTorrentInput 是追剧详情里的「不要这个种子」。
type BlockTorrentInput struct {
	Reason string `json:"reason"`
	// ExcludeGroup 为真时把这个种子的制作组加进订阅的标题排除词
	ExcludeGroup bool `json:"exclude_group"`
}

// BlockSubscriptionTorrent 是追剧详情里「不要这个种子」：按订阅的条目拉黑这个种子；种子还在下载时改成失败，它的集重新算缺的，
// 订阅在找资源时马上搜一次。已整理入库的不改（库里的文件不动）。下载器里的种子不在这里删（页面按用户的选择调删种接口）。
func (s *Service) BlockSubscriptionTorrent(ctx context.Context, subID, rowID uint, in BlockTorrentInput) (*BlockResult, error) {
	sub, err := s.subRow(ctx, subID)
	if err != nil {
		return nil, err
	}
	var t models.MediaSubscriptionTorrent
	if err = s.cfg.DB.WithContext(ctx).Where("id = ? AND subscription_id = ?", rowID, subID).Limit(1).Find(&t).Error; err != nil {
		return nil, fmt.Errorf("读取订阅的种子失败: %w", err)
	}
	if t.ID == 0 {
		return nil, ErrNotFound
	}
	if t.Adopted {
		return nil, fmt.Errorf("%w: 下载器里认领的种子不是站点上的种子，不能拉黑", ErrInvalid)
	}
	res, _, err := s.addBlocks(ctx, BlockInput{
		MediaType: sub.MediaType, TMDBID: sub.TMDBID, Season: sub.Season, MediaTitle: subTitle(&sub), Reason: in.Reason,
		Items: []BlockItem{{Site: t.SiteName, TorrentID: t.TorrentID, Title: t.Title}}, ExcludeGroup: in.ExcludeGroup,
	})
	if err != nil {
		return res, err
	}
	msg := "不要这个种子"
	if r := strings.TrimSpace(in.Reason); r != "" {
		msg += "：" + r
	}
	run := "不要这个种子：" + t.Title + blockNote(in.Reason, res.ExcludedGroup)
	if t.Status == models.MediaSubTorrentDownloading {
		run += "；重新找资源"
		s.mu.Lock()
		s.setTorrent(ctx, t.ID, models.MediaSubTorrentFailed, msg)
		if sub.MediaType == models.MediaKindTV {
			set, _ := s.Settings(ctx)
			if _, lerr := s.rebuildLedger(ctx, &sub, rebuildOpts{set: set}); lerr != nil {
				s.cfg.Logger.Debugf("[订阅] 拉黑以后重建「%s」的追剧台账失败: %v", sub.Title, lerr)
			}
		}
		s.mu.Unlock()
		if sub.Status == models.MediaSubActive {
			s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Update("next_search_at", s.cfg.Now())
		}
	}
	s.manualRun(ctx, &sub, run)
	return res, nil
}
