// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/transmission"
)

// PushTorrentRequest 推送种子请求参数
type PushTorrentRequest struct {
	SiteID       string // 站点ID (如 hdsky, mteam)
	TorrentID    string // 种子ID
	TorrentData  []byte // 种子文件数据
	Title        string // 种子标题
	Category     string // 分类
	Tags         string // 标签
	SavePath     string // 保存路径（可选）
	DownloaderID uint   // 下载器ID
	// Source 写入 TorrentInfo.DownloadSource（如 brush）；为空时是 manual_push。
	Source string
	// Meta 是调用方已经知道的种子信息（刷流从搜索结果里带来）；为空时不覆盖库里已有的值。
	Meta *PushTorrentMeta
	// ReuseExistingData 用于转移做种与辅种：数据已经在盘上。强制暂停添加（校验完才开始），
	// 跳过磁盘预留，站点容量闸门照常；不写种子记录，成功后由调用方更新。
	ReuseExistingData bool
	// IMDbID、DoubanID 是调用方已经知道的外部编号（刷流、订阅从搜索结果带来）；为空的不覆盖库里已有的值。
	IMDbID   string
	DoubanID string
	// AddPaused 要求以暂停状态添加（qB 兼容入口的 paused/stopped）；下载器本身设成不自动开始时照样暂停。
	AddPaused bool
	// Rename 是加进下载器以后显示的名字（qB 的 rename；Transmission 没有这个选项，忽略）。
	Rename string
	// UpLimitKBs、DlLimitKBs 是调用方要的限速（KB/s，0 表示不限），和站点设置的限速取更严的。
	UpLimitKBs int
	DlLimitKBs int

	// Policy 是这次推送的入场策略（docs/design/free-expiry-safety.md）。零值是明确的手动推送：不要求免费、不承诺暂停。
	Policy AdmissionPolicy
	// Snapshot 是这次推送依据的资格快照；要求免费时必须是本次尝试取得的（见 CheckAdmission）。
	// 为空时用 Meta 里的优惠信息（只用于记录与是否需要暂停加入，不能当作要求免费的凭据）。
	Snapshot *v2.PromotionSnapshot
	// AdoptExisting：下载器里已经有这个种子时照样记账、按策略登记监控（RSS 暂存推送用）；否则只返回跳过。
	AdoptExisting bool
	// KeepRecord：种子记录由调用方维护（RSS），不覆盖标题、标签、分类与来源。
	KeepRecord bool
}

// PushTorrentMeta 是推送时一并写进 TorrentInfo 的种子信息。H&R 与体积会被自动清理的 H&R 保护用到
// （scheduler/cleanup_monitor.go 的 getHRInfoMap 按 has_hr、hr_seed_time_h、torrent_size 判断）。
// IsFree、FreeLevel、FreeEndTime 只在没有 Snapshot 时用来生成记录用的快照。
type PushTorrentMeta struct {
	SizeBytes   int64
	HasHR       bool
	HRSeedTimeH int
	IsFree      bool
	FreeLevel   string
	FreeEndTime *time.Time
}

// PushTorrentResult 推送种子结果
type PushTorrentResult struct {
	Success     bool
	Skipped     bool // 是否跳过（如已存在）
	TorrentHash string
	Message     string
	// Status 是机器可判定的结果（见 PushStatus*）
	Status PushStatus
}

// pushTarget 是这次推送的下载器。
type pushTarget struct {
	ID        uint
	Name      string
	AutoStart bool
}

// PushTorrentToDownloader 将种子推送到下载器：资格门禁、磁盘与站点容量闸门、加入、记账与免费结束监控登记都在这里。
// RSS 暂存推送用同一个实现（pushTorrent），只是带着已经连好的下载器。
func PushTorrentToDownloader(ctx context.Context, req PushTorrentRequest) (*PushTorrentResult, error) {
	if global.GlobalDB == nil {
		return nil, errors.New("数据库未初始化")
	}

	// 获取下载器配置
	var dlSetting models.DownloaderSetting
	if err := global.GlobalDB.DB.First(&dlSetting, req.DownloaderID).Error; err != nil {
		return nil, fmt.Errorf("获取下载器失败: %w", err)
	}
	if !dlSetting.Enabled {
		return nil, fmt.Errorf("下载器 %s 未启用", dlSetting.Name)
	}

	// 下载器实例在门禁放行后才建：每次推送单独建一个，用完关闭，批量推送时不再一路累积会话
	var dl downloader.Downloader
	defer func() {
		if dl != nil {
			_ = dl.Close()
		}
	}()
	connect := func() (downloader.Downloader, error) {
		d, err := newPushDownloader(dlSetting)
		if err != nil {
			return nil, fmt.Errorf("创建下载器实例失败: %w", err)
		}
		dl = d
		return d, nil
	}
	return pushTorrent(ctx, pushTarget{ID: dlSetting.ID, Name: dlSetting.Name, AutoStart: dlSetting.AutoStart}, connect, req)
}

// pushTorrent 是推送的唯一实现。connect 在门禁放行后才调用，返回要用的下载器。
func pushTorrent(ctx context.Context, target pushTarget, connect func() (downloader.Downloader, error), req PushTorrentRequest) (*PushTorrentResult, error) {
	record := !req.ReuseExistingData
	snap := req.Snapshot
	if snap == nil && req.Meta != nil {
		snap = snapshotFromMeta(req.Meta)
	}

	// 门禁先于连接下载器：被拒绝的推送不用登录下载器
	decision := CheckAdmission(req.Policy, snap, req.Policy.Now())
	if !decision.Allow {
		return rejectPush(req, record, bestEffortHash(req.TorrentData), decision.Status, decision.Reason), nil
	}
	if record {
		hash := bestEffortHash(req.TorrentData)
		// 同一个种子的推送串行：「看别处有没有受监控的加入 → 加入 → 记账」整段不和另一次推送交错，
		// 否则两边都看不到对方、各加一份，后写的记账盖掉先写的，被盖掉的那一处没有监控
		unlock := placementLocks.lock(req.SiteID + "|" + cmp.Or(hash, "id:"+req.TorrentID))
		defer unlock()
		if other := monitoredElsewhere(req.SiteID, req.TorrentID, hash, target.ID); other != "" {
			return rejectPush(req, record, hash, PushStatusRejectedPlacement,
				fmt.Sprintf("这个种子已在下载器 %s 里并受免费结束监控，不再推到另一个下载器", other)), nil
		}
	}

	dl, err := connect()
	if err != nil {
		return nil, err
	}
	torrentHash, err := qbit.ComputeTorrentHash(req.TorrentData)
	if err != nil {
		return nil, fmt.Errorf("计算种子哈希失败: %w", err)
	}

	// 检查种子是否已存在于下载器中
	exists, err := dl.CheckTorrentExists(torrentHash)
	if err != nil {
		if req.AdoptExisting {
			// RSS 暂存推送：查不清下载器里有没有就不加，文件留到下一轮（可能已经加过、只差记账）
			return nil, fmt.Errorf("检查种子存在失败: %w", err)
		}
		sLogger().Warnf("检查种子存在失败: %v", err)
	}
	if exists {
		if req.AdoptExisting && record {
			return adoptExistingTorrent(dl, target, req, torrentHash, snap, decision, addedByEarlierAttempt(req, target))
		}
		sLogger().Infof("[PushTorrent] 种子已存在于下载器中，跳过: site=%s, id=%s, hash=%s, downloader=%s",
			req.SiteID, req.TorrentID, torrentHash, target.Name)
		// 只更新已有的记录：没有记录说明不是经 pt-tools 推送的，不为编号新建一条
		mergeTorrentExternalIDs(req.SiteID, req.TorrentID, req.IMDbID, req.DoubanID)
		return &PushTorrentResult{
			Success:     true,
			Skipped:     true,
			TorrentHash: torrentHash,
			Message:     "种子已存在于下载器中",
			Status:      PushStatusDuplicate,
		}, nil
	}

	// 创建或更新数据库记录（RSS 维护的记录不覆盖标题、标签与来源）。免费资格只在加入成功后写：被拒绝的尝试不改资格字段
	if record && !req.KeepRecord {
		now := time.Now()
		source := req.Source
		if source == "" {
			source = "manual_push"
		}
		torrentInfo := &models.TorrentInfo{
			SiteName:       req.SiteID,
			TorrentID:      req.TorrentID,
			TorrentHash:    &torrentHash,
			Title:          req.Title,
			Tag:            req.Tags,
			Category:       req.Category,
			IsDownloaded:   true,
			LastCheckTime:  &now,
			DownloadSource: source,
		}
		updateCols := []string{"torrent_hash", "title", "tag", "category", "is_downloaded", "last_check_time", "download_source"}
		if m := req.Meta; m != nil {
			torrentInfo.TorrentSize = m.SizeBytes
			torrentInfo.HasHR = m.HasHR
			torrentInfo.HRSeedTimeH = m.HRSeedTimeH
			updateCols = append(updateCols, "torrent_size", "has_hr", "hr_seed_time_h")
		}
		err = global.GlobalDB.WithTransaction(func(tx *gorm.DB) error {
			// 使用 upsert 创建或更新记录
			return tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "site_name"}, {Name: "torrent_id"}},
				DoUpdates: clause.AssignmentColumns(updateCols),
			}).Create(torrentInfo).Error
		})
		if err != nil {
			return nil, fmt.Errorf("保存种子记录失败: %w", err)
		}
	}
	// 外部编号不放进上面的更新列：不带编号的推送（手动、ChatOps）不能清掉已有的值
	mergeTorrentExternalIDs(req.SiteID, req.TorrentID, req.IMDbID, req.DoubanID)

	// 构建添加选项。承诺到期暂停的一律先暂停加入：监控记账成功以后才开始，结束时间不可信的就不自动开始
	pausePromised := decision.Monitor || decision.KeepPaused
	opts := downloader.AddTorrentOptions{
		AddAtPaused: !target.AutoStart || req.ReuseExistingData || req.AddPaused || pausePromised,
		SavePath:    req.SavePath,
		Category:    req.Category,
		Tags:        req.Tags,
	}
	startAfter := decision.Monitor && target.AutoStart && !req.AddPaused && !req.ReuseExistingData
	if name := strings.TrimSpace(req.Rename); name != "" {
		opts.AdvanceOptions = map[string]any{"rename": name}
	}

	// 按站点应用速度限制（从 SiteSetting 读取），调用方另给了限速时取更严的
	applySiteSpeedLimits(&opts, req.SiteID)
	opts.UploadSpeedLimitKBs = stricterLimit(opts.UploadSpeedLimitKBs, req.UpLimitKBs)
	opts.DownloadSpeedLimitKBs = stricterLimit(opts.DownloadSpeedLimitKBs, req.DlLimitKBs)

	glOnly, glErr := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	// 磁盘保护预检（修复 Issue #299）：
	// effective_free = client_free - in_flight_pending - pre_reserved
	// gate           = effective_free - thisTorrentSize >= threshold
	// 用全局互斥锁串行化整个 check + Reserve + push 临界区。
	var pushTorrentSize int64
	// 数据已经在盘上时不再占用空间，不走磁盘保护
	diskProtectOn := record && glErr == nil && glOnly.CleanupDiskProtect && glOnly.CleanupMinDiskSpaceGB > 0
	if diskProtectOn {
		mu := PushMutex()
		mu.Lock()
		defer mu.Unlock()

		freeSpace, spaceErr := dl.GetClientFreeSpace(ctx)
		if spaceErr != nil {
			sLogger().Warnf("[磁盘保护] %s: 获取磁盘空间失败，磁盘保护启用故拒绝推送: %v", target.Name, spaceErr)
			return &PushTorrentResult{
				Success:     false,
				TorrentHash: torrentHash,
				Message:     fmt.Sprintf("无法读取磁盘空间，已拒绝推送: %v", spaceErr),
				Status:      PushStatusRejectedDisk,
			}, nil
		}

		pendingBytes, pendingErr := dl.GetIncompletePendingBytes(ctx)
		if pendingErr != nil {
			// fail-closed：读不到下载中任务的待占用空间时，可用空间会被高估这部分体积
			sLogger().Warnf("[磁盘保护] %s: 查询下载中任务的待占用空间失败，磁盘保护启用故拒绝推送: %v", target.Name, pendingErr)
			return &PushTorrentResult{
				Success:     false,
				TorrentHash: torrentHash,
				Message:     fmt.Sprintf("无法读取下载中任务的待占用空间，已拒绝推送: %v", pendingErr),
				Status:      PushStatusRejectedDisk,
			}, nil
		}
		budget := GetDiskBudget()
		effectiveFreeBytes := freeSpace - pendingBytes - budget.Reserved()
		if effectiveFreeBytes < 0 {
			effectiveFreeBytes = 0
		}
		if size, sizeErr := qbit.ComputeTorrentSize(req.TorrentData); sizeErr == nil {
			pushTorrentSize = size
		}
		if pushTorrentSize <= 0 && req.Meta != nil && req.Meta.SizeBytes > 0 {
			pushTorrentSize = req.Meta.SizeBytes
		}
		minBytes := int64(glOnly.CleanupMinDiskSpaceGB * 1024 * 1024 * 1024)
		effGB := float64(effectiveFreeBytes) / (1024 * 1024 * 1024)
		tGB := float64(pushTorrentSize) / (1024 * 1024 * 1024)
		freeGB := float64(freeSpace) / (1024 * 1024 * 1024)
		pendingGB := float64(pendingBytes) / (1024 * 1024 * 1024)
		reservedGB := float64(budget.Reserved()) / (1024 * 1024 * 1024)
		switch {
		case effectiveFreeBytes <= minBytes:
			sLogger().Warnf("[磁盘保护] %s: 磁盘空间不足 (qBit可用 %.1f GB - 下载中待占用 %.1f GB - 本进程预留 %.1f GB = 有效 %.1f GB <= 保底 %.1f GB)，暂停推送: site=%s, id=%s",
				target.Name, freeGB, pendingGB, reservedGB, effGB, glOnly.CleanupMinDiskSpaceGB, req.SiteID, req.TorrentID)
			recordPushDiskProtectError(req.SiteID, req.TorrentID, "磁盘空间不足，暂停推送")
			if glOnly.CleanupEnabled {
				events.Publish(events.Event{Type: events.DiskSpaceLow, Source: "push", At: time.Now()})
			}
			return &PushTorrentResult{
				Success:     false,
				TorrentHash: torrentHash,
				Message:     fmt.Sprintf("磁盘空间不足 (有效 %.1f GB <= %.1f GB)，暂停推送", effGB, glOnly.CleanupMinDiskSpaceGB),
				Status:      PushStatusRejectedDisk,
			}, nil
		case pushTorrentSize <= 0:
			sLogger().Warnf("[磁盘保护] %s: 种子大小未知但有效空间高于保底线，放行推送: site=%s, id=%s", target.Name, req.SiteID, req.TorrentID)
		case effectiveFreeBytes-pushTorrentSize < minBytes:
			sLogger().Warnf("[磁盘保护] %s: 种子体积 %.1f GB 超过当前可用空间 %.1f GB (有效 - 种子 < 保底 %.1f GB)，已跳过: site=%s, id=%s",
				target.Name, tGB, effGB, glOnly.CleanupMinDiskSpaceGB, req.SiteID, req.TorrentID)
			recordPushDiskProtectError(req.SiteID, req.TorrentID,
				fmt.Sprintf("种子体积 %.1f GB 超过当前可用空间 %.1f GB，已跳过", tGB, effGB))
			return &PushTorrentResult{
				Success:     false,
				TorrentHash: torrentHash,
				Message:     fmt.Sprintf("种子体积 %.1f GB 超过当前可用空间 %.1f GB，已跳过", tGB, effGB),
				Status:      PushStatusRejectedTooLarge,
			}, nil
		default:
			budget.Reserve(pushTorrentSize)
		}
	}
	release := func() {
		if pushTorrentSize > 0 && diskProtectOn {
			GetDiskBudget().Release(pushTorrentSize)
		}
	}

	// 站点容量闸门（Issue #405）：超过该站点 SeedingCapacityGB 上限则拒绝推送。
	// 接在 disk-protect 之后、AddTorrentFileEx 之前；disk-protect 关闭时本闸门自行加锁。
	if capGB := siteSeedingCapacityGB(req.SiteID); capGB > 0 {
		if !diskProtectOn {
			mu := PushMutex()
			mu.Lock()
			defer mu.Unlock()
		}
		capSize := pushTorrentSize
		if capSize == 0 {
			if size, sizeErr := qbit.ComputeTorrentSize(req.TorrentData); sizeErr == nil {
				capSize = size
			}
		}
		if capSize <= 0 && req.Meta != nil && req.Meta.SizeBytes > 0 {
			capSize = req.Meta.SizeBytes
		}
		used, usedErr := getSiteSeedingSizeBytes(ctx, req.SiteID, dl)
		if usedErr != nil {
			sLogger().Warnf("[站点容量] %s: 获取站点做种总量失败，容量限制启用故拒绝推送: %v", req.SiteID, usedErr)
			release()
			recordPushDiskProtectError(req.SiteID, req.TorrentID, fmt.Sprintf("无法读取站点做种总量，已拒绝推送: %v", usedErr))
			return &PushTorrentResult{
				Success:     false,
				TorrentHash: torrentHash,
				Message:     fmt.Sprintf("无法读取站点做种总量，已拒绝推送: %v", usedErr),
				Status:      PushStatusRejectedCapacity,
			}, nil
		}
		capBytes := int64(capGB * gibiByte)
		if used+capSize > capBytes {
			usedGB := float64(used) / gibiByte
			tGB := float64(capSize) / gibiByte
			sLogger().Warnf("[站点容量] %s: 超过容量上限 (当前做种 %.1f GB + 本次种子 %.1f GB > 上限 %.1f GB)，跳过推送: id=%s",
				req.SiteID, usedGB, tGB, capGB, req.TorrentID)
			release()
			msg := fmt.Sprintf("站点容量已满 (当前 %.1f GB + 种子 %.1f GB > %.1f GB)", usedGB, tGB, capGB)
			recordPushDiskProtectError(req.SiteID, req.TorrentID, msg)
			return &PushTorrentResult{
				Success:     false,
				TorrentHash: torrentHash,
				Message:     msg,
				Status:      PushStatusRejectedCapacity,
			}, nil
		}
	}

	// 等锁、读空间的工夫可能跨过免费边界：真正加入前再看一次，跨过了就归还预留、拒绝
	if req.Policy.RequireFree {
		if again := CheckAdmission(req.Policy, snap, req.Policy.Now()); !again.Allow {
			release()
			return rejectPush(req, record, torrentHash, again.Status, again.Reason), nil
		}
	}

	// 承诺到期暂停的推送：加入前写下标记（下载器、策略与快照），加入后记账失败时对账能找回任务
	if record && pausePromised {
		if markErr := markPushPending(req, target, torrentHash, snap); markErr != nil {
			release()
			return nil, fmt.Errorf("写入推送标记失败: %w", markErr)
		}
	}

	// 推送种子到下载器
	// 下载器支持按 ctx 添加时用它：调用方的超时、关闭时的取消能让上传停下
	var result downloader.AddTorrentResult
	if ca, ok := dl.(downloader.ContextAdder); ok {
		result, err = ca.AddTorrentFileExContext(ctx, req.TorrentData, opts)
	} else {
		result, err = dl.AddTorrentFileEx(req.TorrentData, opts)
	}
	if err != nil {
		// 推送失败：归还预留配额，避免 budget 永久占用。标记留着：响应超时但其实加进去了时，对账按哈希找回（任务是暂停加入的）
		release()
		// 更新推送失败状态
		if record {
			_ = global.GlobalDB.DB.Model(&models.TorrentInfo{}).
				Where("site_name = ? AND torrent_id = ?", req.SiteID, req.TorrentID).
				Updates(map[string]any{
					"last_error":  err.Error(),
					"retry_count": gorm.Expr("retry_count + 1"),
				})
		}
		return nil, fmt.Errorf("推送种子失败: %w", err)
	}

	if !result.Success {
		// 业务层失败也需要归还预留。
		release()
		errMsg := fmt.Sprintf("%v", result.Message)
		if record {
			_ = global.GlobalDB.DB.Model(&models.TorrentInfo{}).
				Where("site_name = ? AND torrent_id = ?", req.SiteID, req.TorrentID).
				Updates(map[string]any{
					"last_error":      errMsg,
					"retry_count":     gorm.Expr("retry_count + 1"),
					"push_pending_at": nil,
				})
		}
		return &PushTorrentResult{
			Success:     false,
			TorrentHash: torrentHash,
			Message:     errMsg,
			Status:      PushStatusFailed,
		}, nil
	}
	if result.Duplicate {
		// 下载器说原来就有它（预检没查到：查询失败，或者刚好被别处加进去）：没有新加，归还预留
		release()
		if record && pausePromised {
			clearPushPending(req.SiteID, req.TorrentID)
		}
		if req.AdoptExisting && record {
			// RSS 暂存推送：和预检查到时一样记账、按策略登记监控；不是这次加入的，不改它的运行状态
			return adoptExistingTorrent(dl, target, req, torrentHash, snap, decision, false)
		}
		sLogger().Infof("[PushTorrent] 下载器里已有这个种子，跳过: site=%s, id=%s, hash=%s, downloader=%s",
			req.SiteID, req.TorrentID, torrentHash, target.Name)
		return &PushTorrentResult{
			Success:     true,
			Skipped:     true,
			TorrentHash: torrentHash,
			Message:     "种子已存在于下载器中",
			Status:      PushStatusDuplicate,
		}, nil
	}
	// 推送成功的预留由 scheduler/cleanup_monitor 周期 Reset 归还，避免与
	// downloader.GetIncompletePendingBytes 在 qBit 可见性窗口内双重计数
	// （Issue #299 race，详见 disk_budget.go 顶部注释）。

	if !record {
		sLogger().Infof("[PushTorrent] 种子已暂停加入（复用已有数据）: site=%s, hash=%s, downloader=%s",
			req.SiteID, torrentHash, target.Name)
		return &PushTorrentResult{Success: true, TorrentHash: torrentHash, Status: PushStatusPushed}, nil
	}

	// 推送成功，记账：下载器、任务编号、策略、资格快照与暂停计划在同一条 UPDATE 写入，监控从库里重建
	taskID := result.Hash
	if taskID == "" {
		taskID = torrentHash
	}
	err = global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_id = ?", req.SiteID, req.TorrentID).
		Updates(pushedColumns(target, taskID, req.Policy, snap, decision)).Error
	if err != nil {
		// 种子已经加进下载器，不能报推送失败；把本地记账失败明确告诉调用方。承诺暂停的任务是暂停加入的，保持暂停
		sLogger().Errorf("[PushTorrent] 种子已推送，但更新本地记录失败: site=%s, id=%s, %v", req.SiteID, req.TorrentID, err)
		status := PushStatusPushed
		msg := fmt.Sprintf("已推送到下载器，但本地记录更新失败: %v", err)
		if pausePromised {
			status = PushStatusMonitorDegraded
			msg += "；任务保持暂停，没有自动开始"
		}
		return &PushTorrentResult{Success: true, TorrentHash: result.Hash, Message: msg, Status: status}, nil
	}
	if decision.Monitor {
		scheduleFromDB(req.SiteID, req.TorrentID)
	}

	sLogger().Infof("[PushTorrent] 种子推送成功: site=%s, id=%s, hash=%s, downloader=%s, source_kind=%s, require_free=%v, pause_on_free_end=%v",
		req.SiteID, req.TorrentID, torrentHash, target.Name, req.Policy.SourceKind, req.Policy.RequireFree, req.Policy.PauseOnFreeEnd)

	res := &PushTorrentResult{Success: true, TorrentHash: result.Hash, Status: PushStatusPushed}
	switch {
	case decision.KeepPaused:
		res.Status, res.Message = PushStatusPushedPaused, decision.Reason
	case startAfter:
		if resumeErr := dl.ResumeTorrent(taskID); resumeErr != nil {
			sLogger().Warnf("[PushTorrent] 种子已暂停加入并登记了免费结束监控，但开始下载失败: site=%s, id=%s, %v", req.SiteID, req.TorrentID, resumeErr)
			res.Status, res.Message = PushStatusPushedPaused, fmt.Sprintf("已暂停加入，开始下载失败: %v", resumeErr)
		}
	}
	return res, nil
}

// placementLocks 按站点与种子哈希串行化推送（见 pushTorrent）。
var placementLocks keyedMutex

// keyedMutex 是按键的互斥锁；没人用的键会被清掉。
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*keyedEntry
}

type keyedEntry struct {
	mu   sync.Mutex
	refs int
}

func (k *keyedMutex) lock(key string) (unlock func()) {
	k.mu.Lock()
	if k.locks == nil {
		k.locks = map[string]*keyedEntry{}
	}
	e := k.locks[key]
	if e == nil {
		e = &keyedEntry{}
		k.locks[key] = e
	}
	e.refs++
	k.mu.Unlock()
	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		k.mu.Lock()
		e.refs--
		if e.refs == 0 {
			delete(k.locks, key)
		}
		k.mu.Unlock()
	}
}

// bestEffortHash 是被拒绝时结果里带的哈希；种子文件解析不了时为空。
func bestEffortHash(data []byte) string {
	h, err := qbit.ComputeTorrentHash(data)
	if err != nil {
		return ""
	}
	return h
}

// rejectPush 记下拒绝原因（不累加推送重试次数，拒绝不是种子本身的问题）并返回结果。
func rejectPush(req PushTorrentRequest, record bool, hash string, status PushStatus, reason string) *PushTorrentResult {
	if record {
		recordPushDiskProtectError(req.SiteID, req.TorrentID, reason)
	}
	rssID := "-"
	if req.Policy.SourceRSSID != nil {
		rssID = fmt.Sprint(*req.Policy.SourceRSSID)
	}
	sLogger().Infof("[推送门禁] 拒绝推送: site=%s torrent_id=%s source_kind=%s source_rss_id=%s status=%s reason=%s",
		req.SiteID, req.TorrentID, req.Policy.SourceKind, rssID, status, reason)
	return &PushTorrentResult{Success: false, TorrentHash: hash, Message: reason, Status: status}
}

// addedByEarlierAttempt 报告记录上留着之前一次加入的标记（暂停加入以后记账没写成），而且是加到这个下载器的。
func addedByEarlierAttempt(req PushTorrentRequest, target pushTarget) bool {
	var cur models.TorrentInfo
	if err := global.GlobalDB.DB.Where("site_name = ? AND torrent_id = ?", req.SiteID, req.TorrentID).First(&cur).Error; err != nil {
		return false
	}
	return cur.PushPendingAt != nil && cur.DownloaderID != nil && *cur.DownloaderID == target.ID
}

// adoptExistingTorrent 处理下载器里已经有这个种子的 RSS 推送：记账、按策略登记监控。
// ours 为真（上次暂停加入后记账失败）时补上记账并按设置开始；否则是下载器里原本就有的，不改它的运行状态。
func adoptExistingTorrent(dl downloader.Downloader, target pushTarget, req PushTorrentRequest, hash string, snap *v2.PromotionSnapshot, decision AdmissionDecision, ours bool) (*PushTorrentResult, error) {
	if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_id = ?", req.SiteID, req.TorrentID).
		Updates(pushedColumns(target, hash, req.Policy, snap, decision)).Error; err != nil {
		return nil, fmt.Errorf("更新数据库状态失败: %w", err)
	}
	if decision.Monitor {
		scheduleFromDB(req.SiteID, req.TorrentID)
	}
	res := &PushTorrentResult{Success: true, Skipped: true, TorrentHash: hash, Message: "种子已存在于下载器中，已记账", Status: PushStatusDuplicate}
	if ours && decision.Monitor && target.AutoStart && !req.AddPaused {
		if err := dl.ResumeTorrent(hash); err != nil {
			sLogger().Warnf("[PushTorrent] 补记账后开始下载失败: site=%s, id=%s, %v", req.SiteID, req.TorrentID, err)
		}
	}
	sLogger().Infof("[PushTorrent] 种子已存在于下载器 %s，已记账: site=%s, id=%s, hash=%s, 补上次的记账=%v", target.Name, req.SiteID, req.TorrentID, hash, ours)
	return res, nil
}

// pushedColumns 是加入成功后写进种子记录的列：下载器与任务编号、策略、资格快照与暂停计划。
func pushedColumns(target pushTarget, taskID string, policy AdmissionPolicy, snap *v2.PromotionSnapshot, decision AdmissionDecision) map[string]any {
	pushed := true
	dlID := target.ID
	cols := map[string]any{
		"is_pushed":          &pushed,
		"push_time":          time.Now(),
		"last_error":         nil,
		"downloader_id":      &dlID,
		"downloader_name":    target.Name,
		"downloader_task_id": taskID,
		"push_pending_at":    nil,
		"pause_on_free_end":  policy.PauseOnFreeEnd,
		"require_free":       policy.RequireFree,
		"pause_planned_at":   nil,
		"free_end_failures":  0,
	}
	if policy.SourceRSSID != nil {
		// 已经有所有者 RSS 的不改（通知按钮推的是另一条 RSS 暂存过的种子时，暂存文件仍由所有者清理）
		cols["source_rss_id"] = gorm.Expr("COALESCE(source_rss_id, ?)", *policy.SourceRSSID)
	}
	if snap != nil {
		maps.Copy(cols, SnapshotColumns(*snap))
	}
	if decision.Monitor && snap != nil && snap.EndsAt != nil {
		// 计划暂停时间与监控器一致：结束时间减提前量（0 到 60 分钟）
		advance := min(max(time.Duration(freeEndAdvanceMinutes())*time.Minute, 0), maxSafetyMargin)
		cols["pause_planned_at"] = snap.EndsAt.Add(-advance)
	}
	return cols
}

// SnapshotColumns 是一份资格快照对应的种子记录列（每次读到站点详情都整套写入，不留旧值）。
func SnapshotColumns(s v2.PromotionSnapshot) map[string]any {
	f := snapshotFieldsOf(s)
	return map[string]any{
		"is_free":            f.isFree,
		"free_level":         f.level,
		"free_end_time":      f.end,
		"free_validity":      f.validity,
		"free_verified_at":   f.verified,
		"free_status_source": f.source,
		"free_status_error":  f.errText,
	}
}

// snapshotFields 是快照落到种子记录上的取值：is_free 包括结束时间未知的免费（是否放行看 free_validity），
// 结束时间只记免费级别的（半价、2X 的结束不触发免费结束处理）。
type snapshotFields struct {
	isFree   bool
	level    string
	end      *time.Time
	validity string
	verified *time.Time
	source   string
	errText  string
}

func snapshotFieldsOf(s v2.PromotionSnapshot) snapshotFields {
	f := snapshotFields{
		isFree:   s.Validity == v2.FreeUntil || s.Validity == v2.FreePermanent || s.Validity == v2.FreeUnknown,
		level:    string(s.Level),
		validity: string(s.Validity),
		source:   s.Source,
		errText:  s.Err,
	}
	if f.level == "" {
		f.level = string(v2.DiscountNone)
	}
	if v2.IsFreeTorrent(s.Level) && s.EndsAt != nil {
		e := s.EndsAt.UTC()
		f.end = &e
	}
	if !s.VerifiedAt.IsZero() {
		v := s.VerifiedAt
		f.verified = &v
	}
	if len(f.errText) > 250 {
		f.errText = truncateUTF8(f.errText, 250)
	}
	return f
}

// truncateUTF8 截到不超过 n 字节，不截断多字节字符。
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// snapshotFromMeta 把调用方给的优惠信息整理成记录用的快照；没有取得时间，不能当作要求免费的凭据。
func snapshotFromMeta(m *PushTorrentMeta) *v2.PromotionSnapshot {
	level := v2.DiscountLevel(m.FreeLevel)
	if level == "" {
		level = v2.DiscountNone
		if m.IsFree {
			level = v2.DiscountFree
		}
	}
	item := v2.TorrentItem{DiscountLevel: level}
	if m.FreeEndTime != nil {
		item.DiscountEndTime = *m.FreeEndTime
	}
	s := item.Promotion(time.Now())
	s.VerifiedAt = time.Time{}
	s.Source = "caller"
	return &s
}

// monitoredElsewhere 返回这个种子正在另一个下载器里受免费结束监控时那个下载器的名字。
// 同一个种子可能有两条记录（RSS 按条目编号、通知按钮与手动推送按详情编号），所以也按哈希找。
func monitoredElsewhere(siteID, torrentID, hash string, downloaderID uint) string {
	q := global.GlobalDB.DB.Where("site_name = ?", siteID)
	if hash != "" {
		q = q.Where("(torrent_id = ? OR torrent_hash = ?)", torrentID, hash)
	} else {
		q = q.Where("torrent_id = ?", torrentID)
	}
	var rows []models.TorrentInfo
	if err := q.Find(&rows).Error; err != nil {
		return ""
	}
	for _, cur := range rows {
		if cur.DownloaderID == nil || *cur.DownloaderID == downloaderID {
			continue
		}
		pushed := cur.IsPushed != nil && *cur.IsPushed
		if pushed && cur.PauseOnFreeEnd && cur.DownloaderTaskID != "" && !cur.IsCompleted && !cur.IsPausedBySystem &&
			cur.FreeValidity == string(v2.FreeUntil) {
			return cur.DownloaderName
		}
	}
	return ""
}

// markPushPending 在承诺到期暂停的推送加入前写下标记、下载器与策略（任务编号等加入后补）。
func markPushPending(req PushTorrentRequest, target pushTarget, hash string, snap *v2.PromotionSnapshot) error {
	dlID := target.ID
	cols := map[string]any{
		"push_pending_at":   time.Now(),
		"torrent_hash":      hash,
		"downloader_id":     &dlID,
		"downloader_name":   target.Name,
		"pause_on_free_end": req.Policy.PauseOnFreeEnd,
		"require_free":      req.Policy.RequireFree,
	}
	if req.Policy.SourceRSSID != nil {
		cols["source_rss_id"] = gorm.Expr("COALESCE(source_rss_id, ?)", *req.Policy.SourceRSSID)
	}
	if snap != nil {
		maps.Copy(cols, SnapshotColumns(*snap))
	}
	return global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_id = ?", req.SiteID, req.TorrentID).Updates(cols).Error
}

func clearPushPending(siteID, torrentID string) {
	if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_id = ?", siteID, torrentID).Update("push_pending_at", nil).Error; err != nil {
		sLogger().Warnf("[PushTorrent] 清除推送标记失败: site=%s, id=%s: %v", siteID, torrentID, err)
	}
}

// scheduleFromDB 按库里的记录登记免费结束监控（内存定时器只是加速，周期对账同样从库里重建）。
func scheduleFromDB(siteID, torrentID string) {
	var ti models.TorrentInfo
	if err := global.GlobalDB.DB.Where("site_name = ? AND torrent_id = ?", siteID, torrentID).First(&ti).Error; err != nil {
		sLogger().Warnf("[PushTorrent] 读取记录登记监控失败（周期对账会补上）: site=%s, id=%s: %v", siteID, torrentID, err)
		return
	}
	ScheduleTorrentForMonitoring(ti)
}

// freeEndAdvanceMinutes 读全局设置里的免费结束提前量（读不到时按 0）。
func freeEndAdvanceMinutes() int {
	if global.GlobalDB == nil {
		return 0
	}
	gl, err := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	if err != nil {
		return 0
	}
	return gl.FreeEndAdvanceMinutes
}

// newPushDownloader 创建手动推送用的下载器实例；测试里替换为假实现。
var newPushDownloader = createDownloaderInstanceForPush

// SwapPushDownloaderFactory 只给其他包的测试用：把推送用的下载器实例换成假实现，返回还原函数。
// 刷流等调用 PushTorrentToDownloader 的后台任务靠它在测试里走真实的磁盘与站点容量闸门。
func SwapPushDownloaderFactory(f func(models.DownloaderSetting) (downloader.Downloader, error)) (restore func()) {
	orig := newPushDownloader
	newPushDownloader = f
	return func() { newPushDownloader = orig }
}

// recordPushDiskProtectError 将手动推送的磁盘保护拒绝原因写入 last_error，
// 与 RSS 路径记账口径一致；不累加 retry_count（拒绝非本种子之过）。
func recordPushDiskProtectError(siteID, torrentID, msg string) {
	if global.GlobalDB == nil {
		return
	}
	if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_id = ?", siteID, torrentID).
		Update("last_error", msg).Error; err != nil {
		sLogger().Errorf("记录磁盘保护状态失败: site=%s, id=%s, %v", siteID, torrentID, err)
	}
}

// createDownloaderInstanceForPush 根据配置创建下载器实例
func createDownloaderInstanceForPush(config models.DownloaderSetting) (downloader.Downloader, error) {
	switch strings.ToLower(config.Type) {
	case "qbittorrent":
		dlConfig := qbit.NewQBitConfig(config.URL, config.Username, config.Password)
		return qbit.NewQbitClient(dlConfig, config.Name)
	case "transmission":
		dlConfig := transmission.NewTransmissionConfigWithAutoStart(config.URL, config.Username, config.Password, config.AutoStart)
		return transmission.NewTransmissionClient(dlConfig, config.Name)
	default:
		return nil, fmt.Errorf("不支持的下载器类型: %s", config.Type)
	}
}

// applySiteSpeedLimits fills opts.UploadSpeedLimitKBs / DownloadSpeedLimitKBs
// from the SiteSetting row keyed by siteName. No-op when siteName is empty
// or the lookup fails (e.g. unknown site or DB error) — the torrent is added
// with unlimited speed in that case, matching pre-v0.26 behavior.
func applySiteSpeedLimits(opts *downloader.AddTorrentOptions, siteName string) {
	if opts == nil || siteName == "" || global.GlobalDB == nil {
		return
	}
	var site models.SiteSetting
	if err := global.GlobalDB.DB.Where("name = ?", siteName).First(&site).Error; err != nil {
		return
	}
	opts.UploadSpeedLimitKBs = site.UploadLimitKBs
	opts.DownloadSpeedLimitKBs = site.DownloadLimitKBs
}

// stricterLimit 是两个限速（KB/s，0 表示不限）里更严的那个。
func stricterLimit(a, b int) int {
	switch {
	case a <= 0:
		return max(b, 0)
	case b <= 0:
		return a
	default:
		return min(a, b)
	}
}

// mergeTorrentExternalIDs 把非空的外部编号写进已有的种子记录；没有记录时什么也不做（不为编号新建记录）。
func mergeTorrentExternalIDs(siteID, torrentID, imdbID, doubanID string) {
	updates := map[string]any{}
	if id := v2.NormalizeIMDbID(imdbID); id != "" {
		updates["imdb_id"] = id
	}
	if id := v2.NormalizeDoubanID(doubanID); id != "" {
		updates["douban_id"] = id
	}
	if len(updates) == 0 || global.GlobalDB == nil {
		return
	}
	if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_id = ?", siteID, torrentID).Updates(updates).Error; err != nil {
		sLogger().Warnf("[PushTorrent] 写入外部编号失败: site=%s, id=%s: %v", siteID, torrentID, err)
	}
}
