package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/maintenance"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// RSS 暂存推送（docs/design/free-expiry-safety.md 附录 B.2.3–B.2.7）：每条 RSS 只处理自己的暂存文件，
// 推送经共享推送服务（同一道门禁、同一套记账与监控登记），要求免费的暂存项推送前必须持有本次读取的资格。

// DetailRevalidator 按记录里的种子编号重新取一次站点详情（推送前验证资格用）。ctx 里带着 v2.WithFreshAfter。
type DetailRevalidator func(ctx context.Context, rec models.TorrentInfo) (*v2.TorrentItem, error)

type rssAttemptKey struct{}

// WithRSSAttempt 记下这一轮 RSS 的开始时间：取详情与推送两个阶段共用，推送阶段只认这一轮读到的资格。
func WithRSSAttempt(ctx context.Context, start time.Time) context.Context {
	return context.WithValue(ctx, rssAttemptKey{}, start)
}

// RSSAttemptStart 是 WithRSSAttempt 记下的开始时间。
func RSSAttemptStart(ctx context.Context) (time.Time, bool) {
	t, ok := ctx.Value(rssAttemptKey{}).(time.Time)
	return t, ok && !t.IsZero()
}

// contextDetailer 是能按调用方 ctx 取详情的站点：ctx 里的 v2.WithFreshAfter 才能传到驱动（缓存详情的驱动据此先刷新）。
type contextDetailer interface {
	GetTorrentDetailsContext(ctx context.Context, item *gofeed.Item) (*v2.TorrentItem, error)
}

// fetchDetail 取一个种子的详情。站点返回 (nil, nil) 也当作取详情失败：没有详情不能当成「不是免费」。
func fetchDetail(ctx context.Context, site UnifiedPTSite, item *gofeed.Item) (*v2.TorrentItem, error) {
	var (
		d   *v2.TorrentItem
		err error
	)
	if cd, ok := site.(contextDetailer); ok {
		d, err = cd.GetTorrentDetailsContext(ctx, item)
	} else {
		d, err = site.GetTorrentDetails(item)
	}
	if err == nil && d == nil {
		err = errors.New("站点没有返回详情")
	}
	return d, err
}

// detailSnapshot 把详情整理成资格快照。验证时间用底层数据实际取得的时间（缓存命中时是缓存刷新的时间），
// 驱动没给时用发起请求的时间；非空结束时间解析失败等问题记进日志（快照的 Err 也会写进记录）。
func detailSnapshot(siteName models.SiteGroup, torrentID string, d *v2.TorrentItem, requestedAt time.Time) v2.PromotionSnapshot {
	verified := d.FetchedAt
	if verified.IsZero() {
		verified = requestedAt
	}
	s := d.Promotion(verified)
	if s.Err != "" {
		sLogger().Warnf("[资格解析] site=%s torrent_id=%s validity=%s: %s", siteName, torrentID, s.Validity, s.Err)
	}
	return s
}

// SnapshotFromDetail 是 detailSnapshot 的导出形式（RSS 通知按钮等在别的包里取详情的入口用）。
func SnapshotFromDetail(siteName models.SiteGroup, torrentID string, d *v2.TorrentItem, requestedAt time.Time) v2.PromotionSnapshot {
	return detailSnapshot(siteName, torrentID, d, requestedAt)
}

// FreshPromotion 推送前按站点详情读一次免费资格，只认 attemptStart 以后取得的数据（缓存详情的驱动先刷新）。
// 站点不支持读详情、读取失败时返回 nil：要求免费的推送会被门禁拒绝。
func FreshPromotion(ctx context.Context, site v2.Site, siteName, torrentID string, attemptStart time.Time) *v2.PromotionSnapshot {
	provider, ok := site.(v2.DetailFetcherProvider)
	if !ok || provider.GetDetailFetcher() == nil {
		LogPromotionLookupFailure(siteName, torrentID, errors.New("站点不支持读取种子详情"))
		return nil
	}
	requestedAt := time.Now()
	item, err := provider.GetDetailFetcher().GetTorrentDetail(v2.WithFreshAfter(ctx, attemptStart), torrentID, "", "")
	if err == nil && item == nil {
		err = errors.New("站点没有返回详情")
	}
	if err != nil {
		LogPromotionLookupFailure(siteName, torrentID, err)
		return nil
	}
	s := detailSnapshot(models.SiteGroup(siteName), torrentID, item, requestedAt)
	return &s
}

// LogPromotionLookupFailure 记下推送前读详情失败（不写种子编号以外的内容，错误里不含密钥）。
func LogPromotionLookupFailure(siteName, torrentID string, err error) {
	sLogger().Warnf("[推送门禁] 读取免费资格失败: site=%s torrent_id=%s err=%v", siteName, torrentID, err)
}

// RevalidateFromSite 返回用站点详情接口重新验证资格的函数。种子编号用记录里不含密钥的 DetailID，没有时用 TorrentID。
func RevalidateFromSite(site UnifiedPTSite) DetailRevalidator {
	if site == nil {
		return nil
	}
	return func(ctx context.Context, rec models.TorrentInfo) (*v2.TorrentItem, error) {
		item := &gofeed.Item{GUID: rec.TorrentID, Title: rec.Title}
		// 只在有从链接取出的种子编号时给链接（各驱动都认 id=）；没有时按条目编号取（如 Rousi 用链接最后一段）
		if rec.DetailID != "" {
			item.Link = "details.php?id=" + rec.DetailID
		}
		return fetchDetail(ctx, site, item)
	}
}

// stagedPush 是一条 RSS 的一次暂存推送。
type stagedPush struct {
	rss          models.RSSConfig
	dl           downloader.Downloader
	dlInfo       *DownloaderInfo
	category     string
	tags         string
	downloadPath string
	siteName     models.SiteGroup
	attemptStart time.Time
	revalidate   DetailRevalidator
}

// ownershipEnforced：没有编号的 RSS（命令行一次性运行、测试）不按归属过滤
func (sp *stagedPush) ownershipEnforced() bool { return sp.rss.ID != 0 }

var (
	errStagedNotMine = errors.New("暂存文件属于另一条 RSS")
	errStagedHeld    = errors.New("暂存文件暂不推送")
)

// lookup 找暂存文件对应的记录：先按哈希，查不到再按文件名里的种子编号（取详情阶段刚下载、哈希还没写进记录的文件）。
func (sp *stagedPush) lookup(filePath string) (rec *models.TorrentInfo, hash string, err error) {
	hash, err = qbit.ComputeTorrentHashWithPath(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("计算种子哈希失败: %w", err)
	}
	rec, err = global.GlobalDB.GetTorrentBySiteAndHash(string(sp.siteName), hash)
	if err != nil {
		return nil, hash, fmt.Errorf("查询种子信息失败: %w", err)
	}
	if rec != nil {
		return rec, hash, nil
	}
	if id := torrentIDFromStagedName(filePath, sp.siteName); id != "" {
		byName, nameErr := global.GlobalDB.GetTorrentBySiteAndID(string(sp.siteName), id)
		if nameErr == nil && byName != nil && byName.SourceRSSID != nil && *byName.SourceRSSID != sp.rss.ID && rssActive(*byName.SourceRSSID) {
			return byName, hash, nil
		}
	}
	return nil, hash, nil
}

// torrentIDFromStagedName 从暂存文件名（<站点>-<种子编号>.torrent）取种子编号。
func torrentIDFromStagedName(filePath string, siteName models.SiteGroup) string {
	base := strings.TrimSuffix(filepath.Base(filePath), ".torrent")
	prefix := strings.ToLower(string(siteName)) + "-"
	if !strings.HasPrefix(strings.ToLower(base), prefix) {
		return ""
	}
	return base[len(prefix):]
}

// processFile 处理一个暂存的 .torrent。返回 downloader.ErrInsufficientSpace / ErrTorrentTooLarge /
// ErrSiteCapacityExceeded 时由推送循环决定停下还是跳过；errStagedNotMine、errStagedHeld 表示文件留着、不算失败。
func (sp *stagedPush) processFile(ctx context.Context, filePath string) error {
	torrent, torrentHash, err := sp.lookup(filePath)
	if err != nil {
		sLogger().Errorf("%s: %v", filePath, err)
		return err
	}
	if torrent == nil {
		sLogger().Warnf("数据库不存在记录，删除孤立种子文件: %s, hash: %s", filePath, torrentHash)
		if err = os.Remove(filePath); err != nil {
			sLogger().Errorf("删除孤立种子失败: %s, %v", filePath, err)
			return fmt.Errorf("删除孤立种子失败: %w", err)
		}
		return nil
	}
	if mine, reason := sp.owns(torrent); !mine {
		if reason != "" {
			recordDiskProtectError(sp.siteName, torrentHash, reason)
			sLogger().Infof("[暂存归属] %s: %s (RSS=%s)", filePath, reason, sp.rss.Name)
		}
		return errStagedNotMine
	}

	gl, _ := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	if gl.RetainHours > 0 && torrent.LastCheckTime != nil && !isPushed(torrent) &&
		torrent.LastCheckTime.Before(time.Now().Add(-time.Duration(gl.RetainHours)*time.Hour)) {
		sLogger().Infof("超过保留时长(%dh)，删除未推送种子: %s", gl.RetainHours, filePath)
		_ = os.Remove(filePath)
		return nil
	}
	if isPushed(torrent) {
		sLogger().Infof("种子已推送，删除本地文件: %s", filePath)
		if err = os.Remove(filePath); err != nil {
			return fmt.Errorf("删除已推送种子失败: %w", err)
		}
		return nil
	}
	if gl.MaxRetry > 0 && torrent.RetryCount >= gl.MaxRetry {
		sLogger().Warnf("超过最大重试次数(%d)，删除文件: %s", gl.MaxRetry, filePath)
		_ = os.Remove(filePath)
		return nil
	}
	if torrent.IsSkipped {
		// 所有者这一轮决定不下载了（不再免费、规则不再匹配）：暂存文件没用了，归属放开，之后别的 RSS 可以接手
		sLogger().Infof("种子已标记跳过，删除暂存文件: %s", filePath)
		_ = os.Remove(filePath)
		sp.releaseOwnership(torrent.ID)
		return nil
	}

	policy := sp.policyFor(*torrent, gl.FreeEndAdvanceMinutes)
	snap := snapshotFromRecord(*torrent)
	if (policy.RequireFree || policy.PauseOnFreeEnd) && !freshSnapshot(snap, sp.attemptStart, time.Now()) {
		snap = sp.revalidateSnapshot(ctx, *torrent, snap)
	}

	torrentData, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return fmt.Errorf("读取种子文件失败: %w", readErr)
	}
	target := pushTarget{ID: sp.dlInfo.ID, Name: sp.dlInfo.Name, AutoStart: sp.dlInfo.AutoStart}
	res, pushErr := pushTorrent(ctx, target, func() (downloader.Downloader, error) { return sp.dl, nil }, PushTorrentRequest{
		SiteID:        string(sp.siteName),
		TorrentID:     torrent.TorrentID,
		TorrentData:   torrentData,
		Title:         torrent.Title,
		Category:      sp.category,
		Tags:          sp.tags,
		SavePath:      sp.downloadPath,
		DownloaderID:  sp.dlInfo.ID,
		Source:        torrent.DownloadSource,
		Meta:          &PushTorrentMeta{SizeBytes: torrent.TorrentSize},
		Policy:        policy,
		Snapshot:      snap,
		AdoptExisting: true,
		KeepRecord:    true,
	})
	if pushErr != nil {
		sLogger().Errorf("推送种子失败: %s, %v", filePath, pushErr)
		return pushErr
	}
	switch res.Status {
	case PushStatusPushed, PushStatusPushedPaused, PushStatusDuplicate:
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("删除已推送种子失败: %w; torrentHash: %s", err, torrentHash)
		}
		sLogger().Infof("种子推送成功并删除: %s, torrentHash: %s, 下载器: %s, 状态: %s, PauseOnFreeEnd: %v",
			filePath, torrentHash, sp.dlInfo.Name, res.Status, sp.rss.PauseOnFreeEnd)
		return nil
	case PushStatusMonitorDegraded:
		// 已暂停加入，记账失败：文件留着，下一轮发现下载器里已有时补记账
		return fmt.Errorf("推送后记账失败: %s", res.Message)
	case PushStatusRejectedDisk:
		return downloader.ErrInsufficientSpace
	case PushStatusRejectedTooLarge:
		return downloader.ErrTorrentTooLarge
	case PushStatusRejectedCapacity:
		return ErrSiteCapacityExceeded
	case PushStatusRejectedNotFree:
		sp.dropStaged(torrent.ID, map[string]any{"is_skipped": true}, filePath, res.Message)
		return nil
	case PushStatusRejectedExpired:
		sp.dropStaged(torrent.ID, map[string]any{"is_expired": true}, filePath, res.Message)
		return nil
	case PushStatusHeldFreeUnknown, PushStatusRejectedStale, PushStatusRejectedPlacement:
		return errStagedHeld
	}
	return fmt.Errorf("推送种子失败: %s", res.Message)
}

// policyFor 是暂存项的入场策略：是否要求免费跟着记录（取详情阶段按免费通道或规则写入），暂停开关跟着本 RSS。
func (sp *stagedPush) policyFor(t models.TorrentInfo, advanceMinutes int) AdmissionPolicy {
	p := AdmissionPolicy{
		RequireFree:    t.RequireFree,
		PauseOnFreeEnd: sp.rss.PauseOnFreeEnd,
		SafetyMargin:   SafetyMarginFor(advanceMinutes),
		SourceKind:     SourceKindRSS,
		AttemptStart:   sp.attemptStart,
	}
	if sp.rss.ID != 0 {
		id := sp.rss.ID
		p.SourceRSSID = &id
	}
	return p
}

// dropStaged 在要求免费的推送被拒（不再免费、免费已结束）时更新记录、删除暂存文件并放开归属。
func (sp *stagedPush) dropStaged(id uint, cols map[string]any, filePath, reason string) {
	if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).Where("id = ?", id).Updates(cols).Error; err != nil {
		sLogger().Errorf("[暂存推送] 更新记录失败 (ID:%d): %v", id, err)
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		sLogger().Errorf("[暂存推送] 删除暂存文件失败: %s, %v", filePath, err)
	}
	sp.releaseOwnership(id)
	sLogger().Infof("[暂存推送] 删除暂存文件 %s: %s", filePath, reason)
}

// releaseOwnership 放开本 RSS 对一个没推送的种子的归属（条件更新，只改自己的）。
func (sp *stagedPush) releaseOwnership(recordID uint) {
	if !sp.ownershipEnforced() {
		return
	}
	releaseRSSOwnership(recordID, sp.rss.ID)
}

func releaseRSSOwnership(recordID, rssID uint) {
	if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("id = ? AND source_rss_id = ? AND (is_pushed IS NULL OR is_pushed = ?)", recordID, rssID, false).
		Update("source_rss_id", nil).Error; err != nil {
		sLogger().Warnf("[暂存归属] 放开归属失败 (ID:%d): %v", recordID, err)
	}
}

// revalidateSnapshot 按站点详情重新验证资格并写回记录；取不到时返回原来的快照（陈旧的，要求免费时会被门禁拒绝）。
func (sp *stagedPush) revalidateSnapshot(ctx context.Context, rec models.TorrentInfo, old *v2.PromotionSnapshot) *v2.PromotionSnapshot {
	if sp.revalidate == nil {
		return old
	}
	requestedAt := time.Now()
	item, err := sp.revalidate(v2.WithFreshAfter(ctx, sp.attemptStart), rec)
	if err != nil {
		sLogger().Warnf("[暂存推送] 重新验证资格失败，不沿用旧资格: site=%s torrent_id=%s source_rss_id=%d result=failed err=%v",
			sp.siteName, rec.TorrentID, sp.rss.ID, err)
		return old
	}
	s := detailSnapshot(sp.siteName, rec.TorrentID, item, requestedAt)
	cols := SnapshotColumns(s)
	cols["is_expired"] = promotionEnded(s)
	if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).Where("id = ?", rec.ID).Updates(cols).Error; err != nil {
		sLogger().Warnf("[暂存推送] 写回资格失败 (ID:%d): %v", rec.ID, err)
	}
	sLogger().Infof("[暂存推送] 已重新验证资格: site=%s torrent_id=%s source_rss_id=%d result=%s",
		sp.siteName, rec.TorrentID, sp.rss.ID, s.Validity)
	return &s
}

// owns 判断这个暂存文件是不是本 RSS 的；不是时返回要写进记录的原因（另一条 RSS 的文件不写，原因为空）。
// 没有所有者的旧文件：本站点只有这一条 RSS 用这个目录时用条件更新接管，否则留着不动。
func (sp *stagedPush) owns(t *models.TorrentInfo) (bool, string) {
	if !sp.ownershipEnforced() {
		return true, ""
	}
	if t.SourceRSSID != nil {
		if *t.SourceRSSID == sp.rss.ID {
			return true, ""
		}
		if rssActive(*t.SourceRSSID) {
			return false, ""
		}
	}
	if !soleRSSForDir(sp.rss, string(sp.siteName)) {
		if t.SourceRSSID == nil {
			return false, "无法确定归属的旧暂存文件（同站点有多条 RSS 用这个目录），未推送"
		}
		return false, ""
	}
	if !claimOwnership(t.ID, t.SourceRSSID, sp.rss.ID) {
		return false, ""
	}
	id := sp.rss.ID
	t.SourceRSSID = &id
	return true, ""
}

// mayDelete 报告暂存清理能不能动这个文件：没有记录的孤立文件、本 RSS 的文件，以及只有本 RSS 用这个目录时没有所有者的旧文件。
func (sp *stagedPush) mayDelete(t *models.TorrentInfo) bool {
	if !sp.ownershipEnforced() || t == nil {
		return true
	}
	if t.SourceRSSID != nil {
		if *t.SourceRSSID == sp.rss.ID {
			return true
		}
		if rssActive(*t.SourceRSSID) {
			return false
		}
	}
	return soleRSSForDir(sp.rss, string(sp.siteName))
}

// sweep 按文件时间与记录状态清理本 RSS 暂存目录里已无意义的 .torrent（已推送、孤立、已达最大重试、超过保留期）。
// retainHours <= 0 时不启用。另一条 RSS 的文件不动。
func (sp *stagedPush) sweep(dirPath string, retainHours int) {
	if retainHours <= 0 || global.GlobalDB == nil {
		return
	}
	files, err := qbit.GetTorrentFilesPath(dirPath)
	if err != nil {
		return
	}
	gl, _ := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	for _, f := range filterTorrentFilesBySite(files, sp.siteName) {
		rec, _, lookErr := sp.lookup(f)
		if lookErr != nil || !sp.mayDelete(rec) || !maintenance.ShouldSweepRecord(rec, f, retainHours, gl.MaxRetry) {
			continue
		}
		if rmErr := os.Remove(f); rmErr != nil && !os.IsNotExist(rmErr) {
			sLogger().Errorf("[暂存清理] 删除失败: %s, %v", f, rmErr)
			continue
		}
		if rec != nil {
			sp.releaseOwnership(rec.ID)
		}
		sLogger().Infof("[暂存清理] 删除已无意义的 .torrent: %s", f)
	}
}

// rssActive 报告这条 RSS 还在（没有删除，也不是示例配置）。
func rssActive(id uint) bool {
	var n int64
	if err := global.GlobalDB.DB.Model(&models.RSSSubscription{}).Where("id = ? AND is_example = ?", id, false).Count(&n).Error; err != nil {
		return true // 读不到时按还在算：宁可不接管
	}
	return n > 0
}

// soleRSSForDir 报告同站点里只有这一条（非示例）RSS 使用这个暂存目录（同一个标签）。
func soleRSSForDir(rss models.RSSConfig, siteName string) bool {
	var n int64
	err := global.GlobalDB.DB.Model(&models.RSSSubscription{}).
		Joins("JOIN site_settings ON site_settings.id = rss_subscriptions.site_id").
		Where("site_settings.name = ? AND rss_subscriptions.is_example = ? AND TRIM(rss_subscriptions.tag) = ?",
			siteName, false, strings.TrimSpace(rss.Tag)).
		Count(&n).Error
	return err == nil && n == 1
}

// claimOwnership 用条件更新确定所有者：原来没有所有者，或者就是 prev（已删除的所有者）时才改，看影响行数。
func claimOwnership(recordID uint, prev *uint, rssID uint) bool {
	q := global.GlobalDB.DB.Model(&models.TorrentInfo{}).Where("id = ?", recordID)
	if prev == nil {
		q = q.Where("source_rss_id IS NULL")
	} else {
		q = q.Where("source_rss_id = ?", *prev)
	}
	res := q.Update("source_rss_id", rssID)
	return res.Error == nil && res.RowsAffected == 1
}

// snapshotFromRecord 把记录里的资格整理成快照。没有规范状态的记录（理论上 v12 迁移后不会有）按
// is_free / free_level / free_end_time 推导，没有结束时间的免费是 unknown；验证时间为空（不算本次读取的）。
func snapshotFromRecord(t models.TorrentInfo) *v2.PromotionSnapshot {
	level := v2.DiscountLevel(t.FreeLevel)
	if level == "" {
		level = v2.DiscountNone
	}
	if t.IsFree && !v2.IsFreeTorrent(level) {
		level = v2.DiscountFree
	}
	s := &v2.PromotionSnapshot{Level: level, Source: t.FreeStatusSource, Err: t.FreeStatusError}
	if t.FreeEndTime != nil {
		e := t.FreeEndTime.UTC()
		s.EndsAt = &e
	}
	if t.FreeVerifiedAt != nil {
		s.VerifiedAt = *t.FreeVerifiedAt
	}
	switch v := v2.FreeValidity(t.FreeValidity); v {
	case v2.FreeUntil, v2.FreePermanent, v2.FreeUnknown, v2.FreeNotFree:
		s.Validity = v
		if v == v2.FreePermanent {
			s.EndsAt = nil
		}
		return s
	}
	item := v2.TorrentItem{DiscountLevel: level}
	if s.EndsAt != nil {
		item.DiscountEndTime = *s.EndsAt
	}
	s.Validity = item.Promotion(time.Now()).Validity
	return s
}

// freshSnapshot 报告快照是不是本次尝试读到、而且还不旧。
func freshSnapshot(s *v2.PromotionSnapshot, attemptStart, now time.Time) bool {
	return s != nil && !s.VerifiedAt.IsZero() && !attemptStart.IsZero() && !s.VerifiedAt.Before(attemptStart) &&
		now.Sub(s.VerifiedAt) <= SnapshotMaxAge
}

// promotionEnded 报告快照显示免费已经结束（免费级别、结束时间不晚于读取时间）。
func promotionEnded(s v2.PromotionSnapshot) bool {
	return v2.IsFreeTorrent(s.Level) && s.EndsAt != nil && !s.EndsAt.After(s.VerifiedAt)
}

func isPushed(t *models.TorrentInfo) bool { return t.IsPushed != nil && *t.IsPushed }

// freeForDecision 报告决定下载时按免费算：免费到某时且剩余超过安全余量、长期免费，或免费但结束时间未知。
func freeForDecision(s v2.PromotionSnapshot, now time.Time, margin time.Duration) bool {
	switch s.Validity {
	case v2.FreePermanent, v2.FreeUnknown:
		return true
	case v2.FreeUntil:
		return s.FreeFor(now, margin)
	}
	return false
}

// createTorrentRecord 新建种子记录；同一个种子的记录已经被别处建好时返回 false，不覆盖它。
func createTorrentRecord(t *models.TorrentInfo) (bool, error) {
	res := global.GlobalDB.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "site_name"}, {Name: "torrent_id"}},
		DoNothing: true,
	}).Create(t)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// recordFetchDecision 写一条已有记录这一轮的结果。facts（资格快照等站点事实）每轮都写；
// policy（是否跳过、来源、规则、是否要求免费、标签）只有本 RSS 是所有者、或还没有所有者时才写。
// 决定下载时先用条件更新取得归属，归另一条还在的 RSS 时不写策略。返回这条 RSS 能不能暂存它。
func recordFetchDecision(rec *models.TorrentInfo, rss models.RSSConfig, facts, policy map[string]any, download bool) (bool, error) {
	db := global.GlobalDB.DB
	if err := db.Model(&models.TorrentInfo{}).Where("id = ?", rec.ID).Updates(facts).Error; err != nil {
		return false, err
	}
	if rss.ID == 0 {
		// 没有编号的 RSS（命令行一次性运行、测试）：不按归属区分
		return download, db.Model(&models.TorrentInfo{}).Where("id = ?", rec.ID).Updates(policy).Error
	}
	owner := rec.SourceRSSID
	ownerGone := owner != nil && *owner != rss.ID && !rssActive(*owner)
	if download {
		claimed := owner != nil && *owner == rss.ID
		if !claimed && (owner == nil || ownerGone) {
			claimed = claimOwnership(rec.ID, owner, rss.ID)
		}
		if !claimed {
			return false, nil
		}
		res := db.Model(&models.TorrentInfo{}).Where("id = ? AND source_rss_id = ?", rec.ID, rss.ID).Updates(policy)
		if res.Error != nil {
			return false, res.Error
		}
		id := rss.ID
		rec.SourceRSSID = &id
		return res.RowsAffected == 1, nil
	}
	// 不下载：自己是所有者或还没有所有者时记下跳过；所有者已删除时一并放开归属
	q := db.Model(&models.TorrentInfo{}).Where("id = ?", rec.ID)
	switch {
	case owner == nil:
		q = q.Where("source_rss_id IS NULL")
	case *owner == rss.ID:
		q = q.Where("source_rss_id = ?", rss.ID)
	case ownerGone:
		q = q.Where("source_rss_id = ?", *owner)
		policy["source_rss_id"] = nil
	default:
		return false, nil
	}
	return false, q.Updates(policy).Error
}

// applySnapshotToRecord 把快照写进新建的记录（与 SnapshotColumns 同一套取值）。
func applySnapshotToRecord(t *models.TorrentInfo, s v2.PromotionSnapshot) {
	f := snapshotFieldsOf(s)
	t.IsFree, t.FreeLevel, t.FreeEndTime = f.isFree, f.level, f.end
	t.FreeValidity, t.FreeVerifiedAt, t.FreeStatusSource, t.FreeStatusError = f.validity, f.verified, f.source, f.errText
	t.IsExpired = promotionEnded(s)
}
