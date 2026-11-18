package transfer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// ErrRuleInvalid 表示规则的设置不对。
var ErrRuleInvalid = errors.New("转移规则无效")

// NormalizeRule 校验并整理规则：名称必填，源与目标是两台不同的下载器，数值不能为负；0 的间隔和上限换成默认值。
func NormalizeRule(r *models.TransferRule) error {
	r.Name = strings.TrimSpace(r.Name)
	r.Category = strings.TrimSpace(r.Category)
	r.Tag = strings.TrimSpace(r.Tag)
	r.SiteName = strings.TrimSpace(r.SiteName)
	switch {
	case r.Name == "":
		return fmt.Errorf("%w：名称不能为空", ErrRuleInvalid)
	case r.SourceDownloaderID == 0 || r.TargetDownloaderID == 0:
		return fmt.Errorf("%w：要选源下载器和目标下载器", ErrRuleInvalid)
	case r.SourceDownloaderID == r.TargetDownloaderID:
		return fmt.Errorf("%w：源和目标不能是同一台下载器", ErrRuleInvalid)
	case r.MinSeedingHours < 0 || r.MaxPerRun < 0 || r.IntervalMin < 0:
		return fmt.Errorf("%w：数值不能为负", ErrRuleInvalid)
	case r.MaxPerRun > models.TransferRuleMaxPerRun:
		return fmt.Errorf("%w：每轮最多 %d 个", ErrRuleInvalid, models.TransferRuleMaxPerRun)
	case r.IntervalMin > 0 && r.IntervalMin < models.TransferRuleMinIntervalMin:
		return fmt.Errorf("%w：间隔至少 %d 分钟", ErrRuleInvalid, models.TransferRuleMinIntervalMin)
	}
	if r.MaxPerRun == 0 {
		r.MaxPerRun = models.TransferRuleDefaultMaxPerRun
	}
	if r.IntervalMin == 0 {
		r.IntervalMin = models.TransferRuleDefaultIntervalMin
	}
	return nil
}

// RuleDue 报告规则这一刻该不该运行。
func RuleDue(r models.TransferRule, now time.Time) bool {
	if !r.Enabled {
		return false
	}
	if r.LastRunAt == nil {
		return true
	}
	interval := r.IntervalMin
	if interval <= 0 {
		interval = models.TransferRuleDefaultIntervalMin
	}
	return !now.Before(r.LastRunAt.Add(time.Duration(interval) * time.Minute))
}

// RuleResult 是规则一轮运行的结果。
type RuleResult struct {
	Matched int      `json:"matched"`
	Created int      `json:"created"`
	Skipped []string `json:"skipped,omitempty"`
}

// Summary 是写进规则 last_result 的一句话。
func (r RuleResult) Summary() string {
	s := fmt.Sprintf("符合条件 %d 个，建了 %d 个任务", r.Matched, r.Created)
	if len(r.Skipped) > 0 {
		s += fmt.Sprintf("，跳过 %d 个（%s）", len(r.Skipped), r.Skipped[0])
	}
	return s
}

// RunRule 按规则在源下载器里挑种子建转移任务：已下完、不是刷流的种子，分类、标签、站点、做种时长都符合，
// 目标里还没有、也没有进行中的任务；按添加时间从早到晚，最多 MaxPerRun 个。
func (s *Service) RunRule(ctx context.Context, r models.TransferRule) (RuleResult, error) {
	res := RuleResult{}
	if r.SourceDownloaderID == r.TargetDownloaderID {
		return res, fmt.Errorf("%w：源和目标不能是同一台下载器", ErrRuleInvalid)
	}
	src, _, err := s.cfg.Downloaders.TransferDownloader(ctx, r.SourceDownloaderID)
	if err != nil {
		return res, fmt.Errorf("源下载器不可用: %w", err)
	}
	target, _, err := s.cfg.Downloaders.TransferDownloader(ctx, r.TargetDownloaderID)
	if err != nil {
		return res, fmt.Errorf("目标下载器不可用: %w", err)
	}
	all, err := src.GetAllTorrents()
	if err != nil {
		return res, fmt.Errorf("读取源下载器的种子失败: %w", err)
	}
	inTargetList, err := target.GetAllTorrents()
	if err != nil {
		return res, fmt.Errorf("读取目标下载器的种子失败: %w", err)
	}
	inTarget := make(map[string]bool, len(inTargetList))
	for _, t := range inTargetList {
		inTarget[strings.ToLower(t.InfoHash)] = true
	}
	hashes := make([]string, 0, len(all))
	for _, t := range all {
		hashes = append(hashes, strings.ToLower(t.InfoHash))
	}
	records, err := s.records(ctx, hashes)
	if err != nil {
		return res, err
	}
	active, err := s.activeHashes(ctx, hashes)
	if err != nil {
		return res, err
	}

	matched := make([]downloader.Torrent, 0)
	for _, t := range all {
		h := strings.ToLower(t.InfoHash)
		if h == "" || !completed(t) || hasTag(t.Tags, models.BrushTagAll) {
			continue
		}
		if r.Category != "" && !strings.EqualFold(t.Category, r.Category) {
			continue
		}
		if r.Tag != "" && !hasTag(t.Tags, r.Tag) {
			continue
		}
		if r.SiteName != "" && !strings.EqualFold(s.torrentSite(t, records), r.SiteName) {
			continue
		}
		if r.MinSeedingHours > 0 && t.SeedingTime < int64(r.MinSeedingHours)*3600 {
			continue
		}
		matched = append(matched, t)
	}
	res.Matched = len(matched)
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].DateAdded < matched[j].DateAdded })

	limit := r.MaxPerRun
	if limit <= 0 {
		limit = models.TransferRuleDefaultMaxPerRun
	}
	items := make([]Item, 0, limit)
	for _, t := range matched {
		h := strings.ToLower(t.InfoHash)
		if inTarget[h] || active[h] {
			continue
		}
		items = append(items, Item{SourceID: r.SourceDownloaderID, Hash: h})
		if len(items) >= limit {
			break
		}
	}
	if len(items) == 0 {
		return res, nil
	}
	ruleID := r.ID
	created, skipped, err := s.Create(ctx, r.TargetDownloaderID, items, &ruleID)
	if err != nil {
		return res, err
	}
	res.Created = len(created)
	for _, sk := range skipped {
		res.Skipped = append(res.Skipped, fmt.Sprintf("%s：%s", sk.Name, sk.Reason))
	}
	return res, nil
}

// torrentSite 是种子所属的站点：pt-tools 有记录时用记录，否则按 tracker 认。
func (s *Service) torrentSite(t downloader.Torrent, records map[string]models.TorrentInfo) string {
	if rec, ok := records[strings.ToLower(t.InfoHash)]; ok && rec.SiteName != "" {
		return rec.SiteName
	}
	site, _ := s.cfg.Resolver.Resolve(t.Tracker)
	return site
}

// hasTag 报告逗号分隔的标签里有没有 tag（大小写不敏感）。
func hasTag(tags, tag string) bool {
	for _, t := range strings.Split(tags, ",") {
		if strings.EqualFold(strings.TrimSpace(t), tag) {
			return true
		}
	}
	return false
}
