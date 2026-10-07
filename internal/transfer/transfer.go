// Package transfer 实现转移做种：把下载器里已经下完的种子搬到另一个下载器继续做种，数据不动。
//
// 流程：拿到种子文件（qBittorrent 导出；否则按 pt-tools 的种子记录从站点重新下载，核对 info hash）
// → 暂停加入目标下载器，保存路径按两台下载器之间的路径映射换算（推送走 internal.PushTorrentToDownloader，
// 站点容量闸门照常，不占磁盘预算）→ 让目标校验 → 校验到 100% 恢复目标、从源移除（不删数据）；
// 没到 100% 或超时就从目标移除（不删数据），源不动。每一步的状态写进 TorrentTransferJob，进程重启后接着做。
package transfer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

const (
	// defaultCheckGrace 是加入目标之后等它开始校验的时间：下载器开始校验之前，种子可能还停在暂停、进度 0。
	defaultCheckGrace = 2 * time.Minute
	// 校验时限：30 分钟，加上按 20 MiB/s 读完全部数据的时间。
	checkBaseTimeout = 30 * time.Minute
	checkBytesPerSec = 20 << 20
	// MaxItems 是一次预览或建任务最多的种子数。
	MaxItems = 500
	// TransferSource 写进推送请求的来源。
	TransferSource = "transfer"
	// ReseedSource 是辅种的推送来源，也写进新种子记录的 download_source。
	ReseedSource = "reseed"
)

var (
	// ErrInvalid 表示请求本身有问题（没选种子、太多、下载器不对）。
	ErrInvalid = errors.New("转移请求无效")
	// ErrNotCancelable 表示任务已经加入目标下载器，不能再取消。
	ErrNotCancelable = errors.New("任务已经加入目标下载器，不能取消")
	// ErrJobNotFound 表示任务不存在。
	ErrJobNotFound = errors.New("转移任务不存在")
	// ErrTargetUnavailable 表示目标下载器不存在、未启用或连不上。
	ErrTargetUnavailable = errors.New("目标下载器不可用")
)

// Downloaders 按 ID 取下载器实例与配置；下载器不存在、未启用或连不上时返回错误。
type Downloaders interface {
	TransferDownloader(ctx context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error)
}

// DownloadersFunc 把函数适配为 Downloaders。
type DownloadersFunc func(ctx context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error)

// TransferDownloader 调用 f。
func (f DownloadersFunc) TransferDownloader(ctx context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error) {
	return f(ctx, id)
}

// SitesFunc 按站点名取共享的站点实例（与搜索、登录探测共用限速器）；拿不到时第二个返回值为 false。
type SitesFunc func(siteName string) (v2.Site, bool)

// PushFunc 是推送入口，默认 internal.PushTorrentToDownloader。
type PushFunc func(ctx context.Context, req ptinternal.PushTorrentRequest) (*ptinternal.PushTorrentResult, error)

// Config 是 Service 的依赖。
type Config struct {
	DB          *gorm.DB
	Downloaders Downloaders
	// Sites 为 nil 时不能从站点重新下载种子文件（源下载器不能导出时就转不了）。
	Sites    SitesFunc
	Push     PushFunc
	Resolver *v2.TrackerResolver
	Now      func() time.Time
	Logger   *zap.SugaredLogger
	// CheckGrace 默认 2 分钟。
	CheckGrace time.Duration
}

// Service 预览、建立并推进转移任务。
type Service struct {
	cfg Config
}

// New 构造 Service。
func New(cfg Config) *Service {
	if cfg.Push == nil {
		cfg.Push = ptinternal.PushTorrentToDownloader
	}
	if cfg.Resolver == nil {
		cfg.Resolver = v2.NewTrackerResolver()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.CheckGrace <= 0 {
		cfg.CheckGrace = defaultCheckGrace
	}
	return &Service{cfg: cfg}
}

// Item 是要转移的一个种子：在哪台源下载器、info hash 是什么。
type Item struct {
	SourceID uint   `json:"source_id"`
	Hash     string `json:"hash"`
}

// 种子文件的来源。
const (
	SourceExport = "export"
	SourceSite   = "site"
)

// PreviewItem 是预览里的一行：能不能转、换算后的路径、种子文件从哪来。
type PreviewItem struct {
	SourceID   uint   `json:"source_id"`
	SourceName string `json:"source_name"`
	Hash       string `json:"hash"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	SavePath   string `json:"save_path"`
	TargetPath string `json:"target_path"`
	// Mapped 表示路径按映射换算过；为 false 时目标用同一个路径。
	Mapped   bool   `json:"mapped"`
	SiteName string `json:"site_name,omitempty"`
	// Source 是种子文件的来源：export（从源下载器导出）或 site（按 pt-tools 的记录从站点重新下载）。
	Source string `json:"source,omitempty"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`

	torrent   downloader.Torrent
	torrentID string
}

// Preview 列出这些种子能不能转到目标下载器，以及换算后的路径。
func (s *Service) Preview(ctx context.Context, targetID uint, items []Item) ([]PreviewItem, error) {
	return s.preview(ctx, targetID, items)
}

func (s *Service) preview(ctx context.Context, targetID uint, items []Item) ([]PreviewItem, error) {
	items = normalizeItems(items)
	switch {
	case targetID == 0:
		return nil, fmt.Errorf("%w：要选目标下载器", ErrInvalid)
	case len(items) == 0:
		return nil, fmt.Errorf("%w：没有选中任何种子", ErrInvalid)
	case len(items) > MaxItems:
		return nil, fmt.Errorf("%w：一次最多转移 %d 个种子", ErrInvalid, MaxItems)
	}
	target, _, err := s.cfg.Downloaders.TransferDownloader(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrTargetUnavailable, err)
	}
	hashes := make([]string, 0, len(items))
	for _, it := range items {
		hashes = append(hashes, it.Hash)
	}
	active, err := s.activeHashes(ctx, hashes)
	if err != nil {
		return nil, err
	}
	records, err := s.records(ctx, hashes)
	if err != nil {
		return nil, err
	}

	out := make([]PreviewItem, len(items))
	for i, it := range items {
		out[i] = PreviewItem{SourceID: it.SourceID, Hash: it.Hash}
	}
	for _, sourceID := range sourceOrder(items) {
		s.previewSource(ctx, sourceID, targetID, target, out, active, records)
	}
	return out, nil
}

// previewSource 填好 out 里来自 sourceID 的那几行。
func (s *Service) previewSource(ctx context.Context, sourceID, targetID uint, target downloader.Downloader,
	out []PreviewItem, active map[string]bool, records map[string]models.TorrentInfo,
) {
	rows := make([]*PreviewItem, 0)
	hashes := make([]string, 0)
	for i := range out {
		if out[i].SourceID == sourceID {
			rows = append(rows, &out[i])
			hashes = append(hashes, out[i].Hash)
		}
	}
	setReason := func(reason string) {
		for _, r := range rows {
			r.Reason = reason
		}
	}
	if sourceID == targetID {
		setReason("源和目标是同一台下载器")
		return
	}
	src, srcSet, err := s.cfg.Downloaders.TransferDownloader(ctx, sourceID)
	if err != nil {
		setReason("源下载器不可用: " + err.Error())
		return
	}
	torrents, err := src.GetTorrentsBy(downloader.TorrentFilter{Hashes: hashes})
	if err != nil {
		setReason("读取源下载器的种子失败: " + err.Error())
		return
	}
	byHash := make(map[string]downloader.Torrent, len(torrents))
	for _, t := range torrents {
		byHash[strings.ToLower(t.InfoHash)] = t
	}
	maps, err := s.pathMaps(ctx, sourceID, targetID)
	if err != nil {
		setReason(err.Error())
		return
	}
	_, canExport := src.(downloader.TorrentExporter)
	for _, r := range rows {
		r.SourceName = srcSet.Name
		t, ok := byHash[r.Hash]
		if !ok {
			r.Reason = "源下载器里没有这个种子"
			continue
		}
		r.torrent = t
		r.Name, r.Size, r.SavePath = t.Name, t.TotalSize, t.SavePath
		r.TargetPath, r.Mapped = models.MapTransferPath(maps, t.SavePath)
		rec, hasRec := records[r.Hash]
		r.SiteName = rec.SiteName
		if r.SiteName == "" {
			r.SiteName, _ = s.cfg.Resolver.Resolve(t.Tracker)
		}
		if hasRec {
			r.torrentID = rec.TorrentID
		}
		switch {
		case !completed(t):
			r.Reason = "还没下载完"
		case active[r.Hash]:
			r.Reason = "已经有进行中的转移任务"
		case canExport:
			r.Source = SourceExport
		case hasRec && rec.TorrentID != "" && s.siteAvailable(rec.SiteName):
			r.Source = SourceSite
		default:
			r.Reason = "拿不到种子文件：这台下载器不能导出种子，pt-tools 也没有它可用的站点记录"
		}
		if r.Reason != "" {
			continue
		}
		exists, err := target.CheckTorrentExists(r.Hash)
		switch {
		case err != nil:
			r.Reason = "检查目标下载器失败: " + err.Error()
		case exists:
			r.Reason = "目标下载器里已经有这个种子"
		}
	}
	for _, r := range rows {
		r.OK = r.Reason == ""
	}
}

// Create 按预览的结果为能转的种子建任务，返回建好的任务与跳过的种子（带原因）。
func (s *Service) Create(ctx context.Context, targetID uint, items []Item, ruleID *uint) ([]models.TorrentTransferJob, []PreviewItem, error) {
	rows, err := s.preview(ctx, targetID, items)
	if err != nil {
		return nil, nil, err
	}
	created := make([]models.TorrentTransferJob, 0)
	skipped := make([]PreviewItem, 0)
	err = s.cfg.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			if !r.OK {
				skipped = append(skipped, r)
				continue
			}
			// 预览之后可能有别的请求建了同一个种子的任务
			var n int64
			if qErr := tx.Model(&models.TorrentTransferJob{}).
				Where("info_hash = ? AND state IN ?", r.Hash, models.TransferActiveStates).Count(&n).Error; qErr != nil {
				return fmt.Errorf("检查进行中的任务失败: %w", qErr)
			}
			if n > 0 {
				r.OK, r.Reason = false, "已经有进行中的转移任务"
				skipped = append(skipped, r)
				continue
			}
			job := models.TorrentTransferJob{
				SourceDownloaderID: r.SourceID,
				TargetDownloaderID: targetID,
				InfoHash:           r.Hash,
				Name:               r.Name,
				TotalSize:          r.Size,
				SiteName:           r.SiteName,
				TorrentID:          r.torrentID,
				SourceSavePath:     r.SavePath,
				TargetSavePath:     r.TargetPath,
				Category:           r.torrent.Category,
				Tags:               r.torrent.Tags,
				State:              models.TransferPending,
				RuleID:             ruleID,
			}
			if cErr := tx.Create(&job).Error; cErr != nil {
				return fmt.Errorf("保存转移任务失败: %w", cErr)
			}
			created = append(created, job)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return created, skipped, nil
}

// Cancel 取消还没加入目标下载器的任务。
func (s *Service) Cancel(ctx context.Context, id uint) error {
	now := s.cfg.Now()
	res := s.cfg.DB.WithContext(ctx).Model(&models.TorrentTransferJob{}).
		Where("id = ? AND state IN ?", id, []string{models.TransferPending, models.TransferExported}).
		Updates(map[string]any{"state": models.TransferCanceled, "message": "已取消", "finished_at": now, "torrent_data": nil})
	if res.Error != nil {
		return fmt.Errorf("取消任务失败: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		return nil
	}
	var n int64
	if err := s.cfg.DB.WithContext(ctx).Model(&models.TorrentTransferJob{}).Where("id = ?", id).Count(&n).Error; err != nil {
		return fmt.Errorf("读取任务失败: %w", err)
	}
	if n == 0 {
		return ErrJobNotFound
	}
	return ErrNotCancelable
}

// ClearFinished 删除这一种已经结束的任务记录，返回删除的条数。
func (s *Service) ClearFinished(ctx context.Context, kind string) (int64, error) {
	res := s.cfg.DB.WithContext(ctx).Where("kind = ?", kind).
		Where("state IN ?", []string{models.TransferDone, models.TransferRolledBack, models.TransferFailed, models.TransferCanceled}).
		Delete(&models.TorrentTransferJob{})
	if res.Error != nil {
		return 0, fmt.Errorf("清除已结束的任务失败: %w", res.Error)
	}
	return res.RowsAffected, nil
}

// ---------- 推进任务 ----------

// RunOnce 把所有没结束的任务各推进一步，返回状态有变化的任务数。
// 每台目标下载器同一时间只有一个任务在加入或校验，免得一次校验很多种子把硬盘占满。
func (s *Service) RunOnce(ctx context.Context) int {
	var jobs []models.TorrentTransferJob
	if err := s.cfg.DB.WithContext(ctx).Where("state IN ?", models.TransferActiveStates).Order("id").Find(&jobs).Error; err != nil {
		s.cfg.Logger.Warnf("[转移做种] 读取任务失败: %v", err)
		return 0
	}
	busy := map[uint]bool{}
	for _, j := range jobs {
		if j.State == models.TransferAdding || j.State == models.TransferChecking {
			busy[j.TargetDownloaderID] = true
		}
	}
	changed := 0
	for i := range jobs {
		if ctx.Err() != nil {
			break
		}
		j := &jobs[i]
		if j.State == models.TransferExported && busy[j.TargetDownloaderID] {
			continue
		}
		before := j.State
		s.step(ctx, j)
		if j.State != before {
			changed++
		}
		if j.State == models.TransferAdding || j.State == models.TransferChecking {
			busy[j.TargetDownloaderID] = true
		}
	}
	return changed
}

// step 推进一步。返回的错误是暂时的（连不上下载器等）：任务留在当前状态，消息写明原因，下一轮再试。
func (s *Service) step(ctx context.Context, j *models.TorrentTransferJob) {
	var err error
	switch j.State {
	case models.TransferPending:
		err = s.export(ctx, j)
	case models.TransferExported:
		err = s.add(ctx, j)
	case models.TransferAdding:
		err = s.resumeAdding(ctx, j)
	case models.TransferChecking:
		err = s.check(ctx, j)
	case models.TransferVerified:
		err = s.finish(ctx, j)
	}
	if err != nil && !errors.Is(err, errSuperseded) {
		s.cfg.Logger.Warnf("[转移做种] 任务 %d（%s）: %v", j.ID, j.Name, err)
		s.note(ctx, j, err.Error())
	}
}

// errSuperseded 表示任务在这一步期间被别人改了状态（例如被取消），这一步的结果作废。
var errSuperseded = errors.New("任务状态已被改变")

// export 拿到种子文件：能导出就导出，否则按记录从站点重新下载；核对 info hash。
func (s *Service) export(ctx context.Context, j *models.TorrentTransferJob) error {
	src, _, err := s.cfg.Downloaders.TransferDownloader(ctx, j.SourceDownloaderID)
	if err != nil {
		return fmt.Errorf("源下载器不可用: %w", err)
	}
	t, found, err := findTorrent(src, j.InfoHash)
	if err != nil {
		return fmt.Errorf("读取源下载器的种子失败: %w", err)
	}
	if !found {
		return s.fail(ctx, j, "源下载器里已经没有这个种子")
	}
	if !completed(t) {
		return s.fail(ctx, j, "源种子还没下载完")
	}
	data, why := s.fetchTorrent(ctx, src, j)
	if why != "" {
		return s.fail(ctx, j, why)
	}
	if hash, hashErr := qbit.ComputeTorrentHash(data); hashErr != nil || !strings.EqualFold(hash, j.InfoHash) {
		return s.fail(ctx, j, "拿到的种子文件与要转移的种子不符")
	}
	// 路径以导出这一刻的为准（建任务之后可能改过保存路径）
	maps, err := s.pathMaps(ctx, j.SourceDownloaderID, j.TargetDownloaderID)
	if err != nil {
		return err
	}
	j.SourceSavePath = t.SavePath
	j.TargetSavePath, _ = models.MapTransferPath(maps, t.SavePath)
	j.TorrentData = data
	return s.moveTo(ctx, j, models.TransferExported, "")
}

// fetchTorrent 返回种子文件；拿不到时第二个返回值写明原因。
func (s *Service) fetchTorrent(ctx context.Context, src downloader.Downloader, j *models.TorrentTransferJob) ([]byte, string) {
	exportErr := ""
	if ex, ok := src.(downloader.TorrentExporter); ok {
		data, err := ex.ExportTorrent(ctx, j.InfoHash)
		if err == nil {
			return data, ""
		}
		exportErr = "导出种子失败: " + err.Error()
	}
	if j.TorrentID == "" || j.SiteName == "" {
		if exportErr != "" {
			return nil, exportErr
		}
		return nil, "拿不到种子文件：这台下载器不能导出种子，pt-tools 也没有它的站点记录"
	}
	if !s.siteAvailable(j.SiteName) {
		return nil, fmt.Sprintf("拿不到种子文件：站点 %s 不可用", j.SiteName)
	}
	site, _ := s.cfg.Sites(j.SiteName)
	data, err := site.Download(ctx, j.TorrentID)
	if err != nil {
		return nil, fmt.Sprintf("从站点 %s 下载种子失败: %v", j.SiteName, err)
	}
	return data, ""
}

// add 把种子暂停加入目标下载器。先记下 adding 再推送：进程在推送途中退出时，重启后按目标里有没有这个种子继续。
func (s *Service) add(ctx context.Context, j *models.TorrentTransferJob) error {
	target, _, err := s.cfg.Downloaders.TransferDownloader(ctx, j.TargetDownloaderID)
	if err != nil {
		return fmt.Errorf("目标下载器不可用: %w", err)
	}
	exists, err := target.CheckTorrentExists(j.InfoHash)
	if err != nil {
		return fmt.Errorf("检查目标下载器失败: %w", err)
	}
	if exists {
		return s.fail(ctx, j, "目标下载器里已经有这个种子")
	}
	if j.OwnerTag == "" {
		j.OwnerTag = models.JobTag(j.Kind, j.ID)
	}
	if err := s.moveTo(ctx, j, models.TransferAdding, "正在加入目标下载器"); err != nil {
		return err
	}
	res, pushErr := s.cfg.Push(ctx, ptinternal.PushTorrentRequest{
		SiteID:            j.SiteName,
		TorrentID:         j.TorrentID,
		TorrentData:       j.TorrentData,
		Title:             j.Name,
		Category:          j.Category,
		Tags:              withOwnerTags(j.Tags, j),
		SavePath:          j.TargetSavePath,
		DownloaderID:      j.TargetDownloaderID,
		Source:            pushSource(j.Kind),
		ReuseExistingData: true,
	})
	switch {
	case pushErr != nil:
		// 推送报错时种子也可能已经加进去了（例如响应超时）：目标里有带标签的这个种子才接着校验
		if t, found, err := findTorrent(target, j.InfoHash); err == nil && found {
			if !owned(j, t) {
				return s.fail(ctx, j, notOurs(j))
			}
			return s.startChecking(ctx, j)
		}
		return s.fail(ctx, j, "加入目标下载器失败: "+pushErr.Error())
	case res == nil:
		return s.fail(ctx, j, "加入目标下载器失败")
	case res.Skipped:
		return s.fail(ctx, j, "目标下载器里已经有这个种子")
	case !res.Success:
		return s.fail(ctx, j, "目标下载器拒绝加入: "+res.Message)
	}
	return s.startChecking(ctx, j)
}

func (s *Service) startChecking(ctx context.Context, j *models.TorrentTransferJob) error {
	now := s.cfg.Now()
	deadline := now.Add(checkTimeout(j.TotalSize))
	j.CheckStartedAt, j.Deadline, j.RecheckIssued, j.Progress = &now, &deadline, false, 0
	return s.moveTo(ctx, j, models.TransferChecking, "等待目标开始校验")
}

// resumeAdding 处理推送途中进程退出的任务：目标里有这个种子就接着校验，没有就重新推送。
func (s *Service) resumeAdding(ctx context.Context, j *models.TorrentTransferJob) error {
	target, _, err := s.cfg.Downloaders.TransferDownloader(ctx, j.TargetDownloaderID)
	if err != nil {
		return fmt.Errorf("目标下载器不可用: %w", err)
	}
	t, found, err := findTorrent(target, j.InfoHash)
	if err != nil {
		return fmt.Errorf("检查目标下载器失败: %w", err)
	}
	switch {
	case found && owned(j, t):
		return s.startChecking(ctx, j)
	case found:
		return s.fail(ctx, j, notOurs(j))
	}
	return s.moveTo(ctx, j, models.TransferExported, "")
}

// check 让目标校验并看进度：到 100% 记为 verified；没到 100% 或超时就回滚。
func (s *Service) check(ctx context.Context, j *models.TorrentTransferJob) error {
	target, _, err := s.cfg.Downloaders.TransferDownloader(ctx, j.TargetDownloaderID)
	if err != nil {
		return fmt.Errorf("目标下载器不可用: %w", err)
	}
	t, found, err := findTorrent(target, j.InfoHash)
	if err != nil {
		return fmt.Errorf("读取目标下载器的种子失败: %w", err)
	}
	now := s.cfg.Now()
	started := now
	if j.CheckStartedAt != nil {
		started = *j.CheckStartedAt
	}
	if !found {
		if now.Sub(started) < s.cfg.CheckGrace {
			return nil // 刚加入，下载器可能还没列出来
		}
		return s.fail(ctx, j, "目标下载器里找不到加入的种子；源下载器里的种子没动")
	}
	if !owned(j, t) {
		return s.fail(ctx, j, notOurs(j))
	}
	if !j.RecheckIssued {
		if err := target.RecheckTorrent(t.ID); err != nil {
			return fmt.Errorf("让目标校验失败: %w", err)
		}
		j.RecheckIssued = true
		return s.save(ctx, j, "正在校验")
	}
	j.Progress = t.Progress
	pct := t.Progress * 100
	switch {
	case checkDone(t):
		return s.moveTo(ctx, j, models.TransferVerified, "校验完成")
	case stillChecking(t):
		if j.Deadline != nil && now.After(*j.Deadline) {
			return s.rollback(ctx, j, target, t, fmt.Sprintf("校验超过时限（到 %.1f%%）", pct))
		}
		return s.save(ctx, j, fmt.Sprintf("正在校验（%.1f%%）", pct))
	case now.Sub(started) < s.cfg.CheckGrace:
		return s.save(ctx, j, "等待目标开始校验")
	default:
		return s.rollback(ctx, j, target, t, fmt.Sprintf("校验只到 %.1f%%：目标路径下的数据与种子对不上，请检查路径映射", pct))
	}
}

// rollback 从目标移除种子（不删数据），源不动。
func (s *Service) rollback(ctx context.Context, j *models.TorrentTransferJob, target downloader.Downloader, t downloader.Torrent, why string) error {
	if err := target.RemoveTorrent(t.ID, false); err != nil && !errors.Is(err, downloader.ErrTorrentNotFound) {
		return fmt.Errorf("%s；从目标移除失败，下一轮再试: %w", why, err)
	}
	return s.moveTo(ctx, j, models.TransferRolledBack, why+"；已从目标移除（数据保留），源下载器里的种子没动")
}

// finish 恢复目标里的种子，再从源移除（不删数据），把 pt-tools 的种子记录改到目标下载器。
func (s *Service) finish(ctx context.Context, j *models.TorrentTransferJob) error {
	target, targetSet, err := s.cfg.Downloaders.TransferDownloader(ctx, j.TargetDownloaderID)
	if err != nil {
		return fmt.Errorf("目标下载器不可用: %w", err)
	}
	t, found, err := findTorrent(target, j.InfoHash)
	if err != nil {
		return fmt.Errorf("读取目标下载器的种子失败: %w", err)
	}
	if !found {
		return s.fail(ctx, j, "目标下载器里的种子不见了；源下载器里的种子没动")
	}
	if !owned(j, t) {
		return s.fail(ctx, j, notOurs(j))
	}
	if !checkDone(t) {
		// 校验完成之后目标又在校验，或者数据不完整了：不收尾，退回校验重新计时；源绝不移除
		now := s.cfg.Now()
		deadline := now.Add(checkTimeout(j.TotalSize))
		j.CheckStartedAt, j.Deadline, j.RecheckIssued, j.Progress = &now, &deadline, true, t.Progress
		return s.moveTo(ctx, j, models.TransferChecking, "目标里的种子不再是 100%，重新等待校验")
	}
	if rErr := target.ResumeTorrent(t.ID); rErr != nil {
		return fmt.Errorf("恢复目标里的种子失败: %w", rErr)
	}
	if j.Kind == models.JobKindReseed {
		// 辅种：数据与原来的种子共用，不移除任何种子；记下这个新种子，站点容量与清理都认得它
		s.recordReseed(ctx, j, targetSet)
		return s.moveTo(ctx, j, models.TransferDone, "")
	}
	src, srcSet, err := s.cfg.Downloaders.TransferDownloader(ctx, j.SourceDownloaderID)
	if err != nil {
		return fmt.Errorf("目标已开始做种，源下载器不可用，下一轮再从源移除: %w", err)
	}
	st, found, err := findTorrent(src, j.InfoHash)
	if err != nil {
		return fmt.Errorf("目标已开始做种，读取源下载器失败，下一轮再从源移除: %w", err)
	}
	if found {
		if err := src.RemoveTorrent(st.ID, false); err != nil && !errors.Is(err, downloader.ErrTorrentNotFound) {
			return fmt.Errorf("目标已开始做种，从源移除失败，下一轮再试: %w", err)
		}
	}
	// 只改源下载器上的记录：同一个 hash 在别的下载器上（如另一站的辅种）的记录不动；
	// 早期只记了下载器名称、没记 ID 的记录按名称认
	taskID := j.InfoHash
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Model(&models.TorrentInfo{}).
		Where("LOWER(torrent_hash) = ?", j.InfoHash).
		Where("downloader_id = ? OR (downloader_id IS NULL AND downloader_name = ?)", j.SourceDownloaderID, srcSet.Name).
		Updates(map[string]any{"downloader_id": targetSet.ID, "downloader_name": targetSet.Name, "downloader_task_id": taskID}).Error; err != nil {
		s.cfg.Logger.Warnf("[转移做种] 任务 %d 已完成，更新种子记录的下载器失败: %v", j.ID, err)
	}
	return s.moveTo(ctx, j, models.TransferDone, "")
}

// ---------- 持久化 ----------

// moveTo 把任务改成 state 写回库：只有库里的状态仍是任务当前状态时才写（期间被取消的不覆盖）。
// 写库不随 ctx 取消：进程退出时已经做完的这一步也要记下来。
func (s *Service) moveTo(ctx context.Context, j *models.TorrentTransferJob, state, msg string) error {
	from := j.State
	j.State, j.Message = state, msg
	if models.TransferStateFinal(state) {
		now := s.cfg.Now()
		j.FinishedAt = &now
		j.TorrentData = nil
	}
	res := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Model(&models.TorrentTransferJob{}).
		Where("id = ? AND state = ?", j.ID, from).
		Select("*").Omit("id", "created_at").Updates(j)
	if res.Error != nil {
		j.State = from
		return fmt.Errorf("保存任务状态失败: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		j.State = from
		return errSuperseded
	}
	if models.TransferStateFinal(state) && from != state && j.OwnerTag != "" {
		s.dropOwnerTag(ctx, j)
	}
	return nil
}

// save 不改状态，只写回进度、消息等字段。
func (s *Service) save(ctx context.Context, j *models.TorrentTransferJob, msg string) error {
	return s.moveTo(ctx, j, j.State, msg)
}

// note 在暂时失败时只更新消息。
func (s *Service) note(ctx context.Context, j *models.TorrentTransferJob, msg string) {
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Model(&models.TorrentTransferJob{}).
		Where("id = ? AND state = ?", j.ID, j.State).Update("message", msg).Error; err != nil {
		s.cfg.Logger.Warnf("[转移做种] 任务 %d 写入消息失败: %v", j.ID, err)
	}
	j.Message = msg
}

// fail 把任务记为失败（还没加入目标，源不动）。
func (s *Service) fail(ctx context.Context, j *models.TorrentTransferJob, why string) error {
	return s.moveTo(ctx, j, models.TransferFailed, why)
}

// ---------- 查询 ----------

func (s *Service) activeHashes(ctx context.Context, hashes []string) (map[string]bool, error) {
	var got []string
	if err := s.cfg.DB.WithContext(ctx).Model(&models.TorrentTransferJob{}).
		Where("info_hash IN ? AND state IN ?", hashes, models.TransferActiveStates).Pluck("info_hash", &got).Error; err != nil {
		return nil, fmt.Errorf("读取进行中的转移任务失败: %w", err)
	}
	out := make(map[string]bool, len(got))
	for _, h := range got {
		out[h] = true
	}
	return out, nil
}

// records 返回 pt-tools 对这些种子的记录（按小写 info hash）；同一个 hash 有多条时取有种子 ID 的那条。
func (s *Service) records(ctx context.Context, hashes []string) (map[string]models.TorrentInfo, error) {
	var rows []models.TorrentInfo
	if err := s.cfg.DB.WithContext(ctx).Where("LOWER(torrent_hash) IN ?", hashes).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取种子记录失败: %w", err)
	}
	out := make(map[string]models.TorrentInfo, len(rows))
	for _, r := range rows {
		if r.TorrentHash == nil {
			continue
		}
		h := strings.ToLower(*r.TorrentHash)
		if prev, ok := out[h]; ok && prev.TorrentID != "" {
			continue
		}
		out[h] = r
	}
	return out, nil
}

func (s *Service) pathMaps(ctx context.Context, sourceID, targetID uint) ([]models.DownloaderPathMap, error) {
	var maps []models.DownloaderPathMap
	if err := s.cfg.DB.WithContext(ctx).
		Where("source_downloader_id = ? AND target_downloader_id = ?", sourceID, targetID).Find(&maps).Error; err != nil {
		return nil, fmt.Errorf("读取路径映射失败: %w", err)
	}
	return maps, nil
}

func (s *Service) siteAvailable(name string) bool {
	if s.cfg.Sites == nil || name == "" {
		return false
	}
	_, ok := s.cfg.Sites(name)
	return ok
}

// ---------- 小工具 ----------

func normalizeItems(items []Item) []Item {
	seen := map[Item]bool{}
	out := make([]Item, 0, len(items))
	for _, it := range items {
		it.Hash = strings.ToLower(strings.TrimSpace(it.Hash))
		if it.Hash == "" || it.SourceID == 0 || seen[it] {
			continue
		}
		seen[it] = true
		out = append(out, it)
	}
	return out
}

func sourceOrder(items []Item) []uint {
	seen := map[uint]bool{}
	out := make([]uint, 0)
	for _, it := range items {
		if !seen[it.SourceID] {
			seen[it.SourceID] = true
			out = append(out, it.SourceID)
		}
	}
	return out
}

func findTorrent(dl downloader.Downloader, hash string) (downloader.Torrent, bool, error) {
	torrents, err := dl.GetTorrentsBy(downloader.TorrentFilter{Hashes: []string{hash}})
	if err != nil {
		return downloader.Torrent{}, false, err
	}
	for _, t := range torrents {
		if strings.EqualFold(t.InfoHash, hash) {
			return t, true, nil
		}
	}
	return downloader.Torrent{}, false, nil
}

func completed(t downloader.Torrent) bool {
	return t.IsCompleted || t.Progress >= 1
}

// checkDone 报告目标校验完了并且数据完整：100%，而且不在校验、状态说得清（排队做种的 100% 种子也算）。
func checkDone(t downloader.Torrent) bool {
	return completed(t) && t.State != downloader.TorrentChecking && t.State != downloader.TorrentUnknown
}

// stillChecking 报告目标是不是还在校验或排队等校验：正在校验、状态说不清，或者不到 100% 还在排队。
// 这几种都等到时限，不按「校验完不到 100%」回滚。
func stillChecking(t downloader.Torrent) bool {
	switch t.State {
	case downloader.TorrentChecking, downloader.TorrentUnknown:
		return true
	case downloader.TorrentQueued:
		return !completed(t)
	}
	return false
}

// notOurs 是目标里有同一个种子、却不是这个任务加的（没有它的归属标签）时的说明。
func notOurs(j *models.TorrentTransferJob) string {
	if j.Kind == models.JobKindReseed {
		return "下载器里已经有这个种子，但不是这个辅种任务加的（没有 " + ownerTag(j) + " 标签）；没有动它"
	}
	return "目标下载器里已经有这个种子，但不是这次转移加的（没有 " + ownerTag(j) + " 标签）；没有动它，源下载器里的种子也没动"
}

// ownerTag 是证明归属用的标签：任务独有的标签；早期版本建的任务没有，用这一种任务的通用标签。
func ownerTag(j *models.TorrentTransferJob) string {
	if j.OwnerTag != "" {
		return j.OwnerTag
	}
	return models.JobOwnerTag(j.Kind)
}

// owned 报告目标里的种子是不是这个任务加的（带它的归属标签）。
func owned(j *models.TorrentTransferJob, t downloader.Torrent) bool {
	return hasTag(t.Tags, ownerTag(j))
}

// withOwnerTags 在原有标签后面加上这一种任务的通用标签和任务独有的标签。
func withOwnerTags(tags string, j *models.TorrentTransferJob) string {
	tags = withTag(tags, models.JobOwnerTag(j.Kind))
	if j.OwnerTag != "" {
		tags = withTag(tags, j.OwnerTag)
	}
	return tags
}

// withTag 在原有标签后面加上 tag（已经有就不重复加）。
func withTag(tags, tag string) string {
	if hasTag(tags, tag) {
		return tags
	}
	if strings.TrimSpace(tags) == "" {
		return tag
	}
	return tags + "," + tag
}

// dropOwnerTag 在任务结束后从目标下载器里去掉任务独有的标签（qBittorrent 的标签列表里不留下用过的任务标签）。
// 做不到时只记日志：留下的标签不影响什么。
func (s *Service) dropOwnerTag(ctx context.Context, j *models.TorrentTransferJob) {
	target, _, err := s.cfg.Downloaders.TransferDownloader(ctx, j.TargetDownloaderID)
	if err != nil {
		return
	}
	r, ok := target.(downloader.TagRemover)
	if !ok {
		return
	}
	if err := r.RemoveTag(j.InfoHash, j.OwnerTag); err != nil {
		s.cfg.Logger.Warnf("[转移做种] 任务 %d 已结束，去掉标签 %s 失败: %v", j.ID, j.OwnerTag, err)
	}
}

// pushSource 是写进推送请求的来源。
func pushSource(kind string) string {
	if kind == models.JobKindReseed {
		return ReseedSource
	}
	return TransferSource
}

func checkTimeout(size int64) time.Duration {
	if size < 0 {
		size = 0
	}
	return checkBaseTimeout + time.Duration(size/checkBytesPerSec)*time.Second
}
