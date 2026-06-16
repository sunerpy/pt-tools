package internal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/requests"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/events"
	"github.com/sunerpy/pt-tools/internal/filter"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
	"github.com/sunerpy/pt-tools/utils"
)

// SkipRecheckHours defines how long a skipped non-free torrent should remain skipped before re-checking
// This allows torrents marked as non-free to be re-checked after becoming free during promotions
const SkipRecheckHours = 6

// buildSkipReason 构建种子跳过原因的可读描述
func buildSkipReason(isFree, canFinished, shouldDownloadByFilter bool) string {
	var reasons []string
	if !isFree {
		reasons = append(reasons, "非免费")
	} else if !canFinished {
		reasons = append(reasons, "免费期内无法完成")
	}
	if !shouldDownloadByFilter {
		reasons = append(reasons, "未匹配过滤规则")
	}
	if len(reasons) == 0 {
		return "未知原因"
	}
	return strings.Join(reasons, ", ")
}

// GetDownloaderForRSS 根据 RSS 配置获取下载器
// DownloaderInfo 下载器附加信息，用于记录推送来源
type DownloaderInfo struct {
	ID        uint
	Name      string
	AutoStart bool
	NeedClose bool
}

// GetDownloaderForRSS 根据 RSS 配置获取下载器
// 优先级：RSS 指定的下载器 > 默认下载器
// 返回下载器实例和下载器信息
func GetDownloaderForRSS(rssCfg models.RSSConfig) (downloader.Downloader, error) {
	dl, _, err := GetDownloaderForRSSWithInfo(rssCfg)
	return dl, err
}

// GetDownloaderForRSSWithInfo 根据 RSS 配置获取下载器及其信息（无站点上下文）。
// 优先级：RSS.DownloaderID > is_default。**不查站点绑定** —— 仅用于无站点
// 信息可用的旧调用点（CLI、cmd/rss 等）。新代码请用
// GetDownloaderForRSSAndSiteWithInfo。
func GetDownloaderForRSSWithInfo(rssCfg models.RSSConfig) (downloader.Downloader, *DownloaderInfo, error) {
	return getDownloaderForRSSImpl(rssCfg, "")
}

// GetDownloaderForRSSAndSiteWithInfo 根据 RSS 配置 + 站点名解析下载器。
//
// 优先级（新）：
//  1. rssCfg.DownloaderID（RSS 行级覆盖）
//  2. SiteSetting.DownloaderID（站点绑定，issue #373 修复点）
//  3. is_default=true（兜底）
//
// siteName 为空时退化为 GetDownloaderForRSSWithInfo 的旧行为。
func GetDownloaderForRSSAndSiteWithInfo(rssCfg models.RSSConfig, siteName string) (downloader.Downloader, *DownloaderInfo, error) {
	return getDownloaderForRSSImpl(rssCfg, strings.TrimSpace(siteName))
}

func getDownloaderForRSSImpl(rssCfg models.RSSConfig, siteName string) (downloader.Downloader, *DownloaderInfo, error) {
	if global.GlobalDB == nil {
		return nil, nil, errors.New("数据库未初始化")
	}

	var dlSetting models.DownloaderSetting

	switch {
	case rssCfg.DownloaderID != nil:
		// 优先级 1: RSS 行自己指定了下载器
		if err := global.GlobalDB.DB.First(&dlSetting, *rssCfg.DownloaderID).Error; err != nil {
			return nil, nil, fmt.Errorf("获取指定下载器失败: %w", err)
		}
		if !dlSetting.Enabled {
			return nil, nil, fmt.Errorf("指定的下载器 %s 未启用", dlSetting.Name)
		}
	case siteName != "":
		// 优先级 2: 站点绑定（issue #373）
		// 直接查 SiteSetting 而非 DownloaderManager.siteDownloaders 以避免内存
		// 状态过时（manager 仅在 scheduler init / Reload 时同步一次）。
		var site models.SiteSetting
		siteErr := global.GlobalDB.DB.Where("name = ?", siteName).First(&site).Error
		if siteErr == nil && site.DownloaderID != nil {
			if err := global.GlobalDB.DB.First(&dlSetting, *site.DownloaderID).Error; err == nil {
				if !dlSetting.Enabled {
					return nil, nil, fmt.Errorf("站点 %s 绑定的下载器 %s 未启用", siteName, dlSetting.Name)
				}
				break
			}
			// 站点 DownloaderID 指向已删除的下载器：fallthrough 到 is_default
			sLogger().Warnf("站点 %s 绑定的下载器 ID=%d 不存在，回退到 is_default", siteName, *site.DownloaderID)
		}
		// 站点未绑定 / 站点行不存在：fallthrough 到 is_default
		fallthrough
	default:
		// 优先级 3: is_default=true 兜底
		if dlSetting.ID == 0 {
			if err := global.GlobalDB.DB.Where("is_default = ?", true).First(&dlSetting).Error; err != nil {
				return nil, nil, fmt.Errorf("获取默认下载器失败: %w", err)
			}
		}
		if !dlSetting.Enabled {
			return nil, nil, fmt.Errorf("默认下载器 %s 未启用", dlSetting.Name)
		}
	}

	info := &DownloaderInfo{
		ID:        dlSetting.ID,
		Name:      dlSetting.Name,
		AutoStart: dlSetting.AutoStart,
		NeedClose: false,
	}

	dm := GetGlobalDownloaderManager()
	if dm != nil {
		dl, err := dm.GetDownloader(dlSetting.Name)
		if err == nil {
			return dl, info, nil
		}
		sLogger().Warnf("从 DownloaderManager 获取下载器失败，回退到直接创建: %v", err)
	}

	info.NeedClose = true
	dlType := downloader.DownloaderType(dlSetting.Type)

	if dm != nil && !dm.HasFactory(dlType) {
		return nil, nil, fmt.Errorf("不支持的下载器类型: %s", dlSetting.Type)
	}

	config := downloader.NewGenericConfig(dlType, dlSetting.URL, dlSetting.Username, dlSetting.Password, dlSetting.AutoStart)
	if dm != nil {
		dl, err := dm.CreateFromConfig(config, dlSetting.Name)
		if err != nil {
			return nil, nil, err
		}
		return dl, info, nil
	}

	return nil, nil, fmt.Errorf("DownloaderManager 未初始化且无法创建下载器: %s", dlSetting.Name)
}

// ProcessTorrentsWithDownloaderByRSS 根据 RSS 配置选择下载器并处理种子
func ProcessTorrentsWithDownloaderByRSS(
	ctx context.Context,
	rssCfg models.RSSConfig,
	dirPath, category, tags string,
	siteName models.SiteGroup,
) error {
	dl, dlInfo, err := GetDownloaderForRSSAndSiteWithInfo(rssCfg, string(siteName))
	if err != nil {
		return fmt.Errorf("获取下载器失败: %w", err)
	}
	if dlInfo.NeedClose {
		defer dl.Close()
	}

	sLogger().Infof("[种子推送开始] 站点=%s, 下载器=%s(%s), 目录=%s", siteName, dl.GetName(), dl.GetType(), dirPath)

	downloadPath := rssCfg.GetEffectiveDownloadPath()
	if downloadPath != "" {
		sLogger().Infof("使用自定义下载路径: %s", downloadPath)
	}

	filePaths, err := qbit.GetTorrentFilesPath(dirPath)
	if err != nil {
		sLogger().Error("无法读取目录", dirPath, err)
		return fmt.Errorf("无法读取目录: %v", err)
	}

	filePaths = filterTorrentFilesBySite(filePaths, siteName)
	if len(filePaths) == 0 {
		sLogger().Infof("[种子推送完成] 站点=%s, 总数=0, 成功=0, 失败=0", siteName)
		return nil
	}

	successCount := 0
	failCount := 0
	for i, file := range filePaths {
		err := processSingleTorrentWithDownloader(ctx, dl, dlInfo, file, category, tags, downloadPath, siteName, rssCfg.PauseOnFreeEnd)
		if err != nil {
			if errors.Is(err, downloader.ErrInsufficientSpace) {
				sLogger().Warnf("[磁盘保护] 空间不足，停止推送剩余 %d 个种子", len(filePaths)-i-1)
				break
			}
			sLogger().Errorf("处理种子失败: %s, %v", file, err)
			failCount++
		} else {
			successCount++
		}
	}

	sLogger().Infof("[种子推送完成] 站点=%s, 总数=%d, 成功=%d, 失败=%d", siteName, len(filePaths), successCount, failCount)
	return nil
}

func filterTorrentFilesBySite(filePaths []string, siteName models.SiteGroup) []string {
	prefix := strings.ToLower(string(siteName)) + "-"
	filtered := make([]string, 0, len(filePaths))
	for _, file := range filePaths {
		if strings.HasPrefix(strings.ToLower(filepath.Base(file)), prefix) {
			filtered = append(filtered, file)
		}
	}
	if len(filtered) > 0 {
		return filtered
	}

	return filePaths
}

func processSingleTorrentWithDownloader(
	ctx context.Context,
	dl downloader.Downloader,
	dlInfo *DownloaderInfo,
	filePath, category, tags, downloadPath string,
	siteName models.SiteGroup,
	pauseOnFreeEnd bool,
) error {
	torrentHash, err := qbit.ComputeTorrentHashWithPath(filePath)
	if err != nil {
		sLogger().Errorf("计算种子哈希失败: %s, %v", filePath, err)
		return fmt.Errorf("计算种子哈希失败: %w", err)
	}

	torrent, err := global.GlobalDB.GetTorrentBySiteAndHash(string(siteName), torrentHash)
	if err != nil {
		sLogger().Errorf("查询种子信息失败: %s, %v", filePath, err)
		return fmt.Errorf("查询种子信息失败: %w", err)
	}

	if torrent == nil {
		sLogger().Warnf("数据库不存在记录，删除孤立种子文件: %s, hash: %s", filePath, torrentHash)
		if err = os.Remove(filePath); err != nil {
			sLogger().Errorf("删除孤立种子失败: %s, %v", filePath, err)
			return fmt.Errorf("删除孤立种子失败: %w", err)
		}
		return nil
	}

	skipExpireCheck := false
	if torrent.DownloadSource == "filter_rule" && torrent.FilterRuleID != nil {
		var filterRule models.FilterRule
		if txErr := global.GlobalDB.DB.First(&filterRule, *torrent.FilterRuleID).Error; txErr == nil {
			if !filterRule.RequireFree {
				skipExpireCheck = true
				sLogger().Infof("[过期检查] 种子 %s 通过过滤规则匹配且不要求免费，跳过过期检查", torrent.Title)
			}
		}
	}

	isExpired := torrent.GetExpired()
	sLogger().Infof("[过期检查] 种子: %s, hash: %s, FreeEndTime: %v, IsExpired(DB): %v, GetExpired(): %v, SkipExpireCheck: %v",
		torrent.Title, torrentHash,
		torrent.FreeEndTime, torrent.IsExpired, isExpired, skipExpireCheck)

	if isExpired && !skipExpireCheck {
		sLogger().Warnf("[过期] 种子免费期已过期，标记并删除: %s, FreeEndTime: %v", filePath, torrent.FreeEndTime)
		if err = global.GlobalDB.DB.Model(&models.TorrentInfo{}).
			Where("site_name = ? AND torrent_hash = ?", siteName, torrentHash).
			Updates(map[string]any{
				"is_expired": true,
				"last_error": "种子已过期，未推送",
			}).Error; err != nil {
			return fmt.Errorf("标记过期状态失败: %w", err)
		}
		if err = os.Remove(filePath); err != nil {
			sLogger().Errorf("[过期] 删除过期种子失败: %s, %v", filePath, err)
		} else {
			sLogger().Infof("[过期] 已删除过期种子: %s", filePath)
		}
		return nil
	}

	gl, _ := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	if gl.RetainHours > 0 && torrent.LastCheckTime != nil {
		cutoff := time.Now().Add(-time.Duration(gl.RetainHours) * time.Hour)
		if torrent.IsPushed == nil || !*torrent.IsPushed {
			if torrent.LastCheckTime.Before(cutoff) {
				sLogger().Infof("超过保留时长(%dh)，删除未推送种子: %s", gl.RetainHours, filePath)
				_ = os.Remove(filePath)
				return nil
			}
		}
	}

	if torrent.IsPushed != nil && *torrent.IsPushed {
		sLogger().Infof("种子已推送，删除本地文件: %s", filePath)
		if err = os.Remove(filePath); err != nil {
			return fmt.Errorf("删除已推送种子失败: %w", err)
		}
		return nil
	}

	if torrent.GetExpired() && !skipExpireCheck {
		sLogger().Infof("种子已过期，删除: %s", filePath)
		_ = os.Remove(filePath)
		return nil
	}

	exists, err := dl.CheckTorrentExists(torrentHash)
	if err != nil {
		return fmt.Errorf("检查种子存在失败: %w", err)
	}

	if exists {
		sLogger().Infof("种子已存在于下载器 %s: %s", dl.GetName(), filePath)
		pushed := true
		if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
			Where("site_name = ? AND torrent_hash = ?", string(siteName), torrentHash).
			Updates(map[string]any{
				"is_pushed":          &pushed,
				"push_time":          time.Now(),
				"downloader_id":      &dlInfo.ID,
				"downloader_name":    dlInfo.Name,
				"downloader_task_id": torrentHash,
				"pause_on_free_end":  pauseOnFreeEnd,
			}).Error; err != nil {
			return fmt.Errorf("更新数据库状态失败: %w", err)
		}
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("删除已存在种子失败: %w", err)
		}
		if pauseOnFreeEnd && torrent.FreeEndTime != nil {
			torrent.DownloaderTaskID = torrentHash
			torrent.DownloaderName = dlInfo.Name
			torrent.DownloaderID = &dlInfo.ID
			torrent.PauseOnFreeEnd = true
			ScheduleTorrentForMonitoring(*torrent)
		}
		return nil
	}

	sLogger().Infof("推送新种子到下载器 %s: %s", dl.GetName(), filePath)

	glOnly, _ := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	if glOnly.MaxRetry > 0 && torrent.RetryCount >= glOnly.MaxRetry {
		sLogger().Warnf("超过最大重试次数(%d)，删除文件: %s", glOnly.MaxRetry, filePath)
		_ = os.Remove(filePath)
		return nil
	}

	// 磁盘空间预检查（修复 Issue #299）
	//
	// 旧实现仅比较 freeGB < threshold，存在两个 race：
	//  1. 没扣除即将推送种子自身大小 —— 一次性多种子推送各自看到相同 free。
	//  2. qBit 默认 preallocate_all=false，新推送种子直到下载完成才反映在
	//     free_space_on_disk —— 多 RSS worker 并发推送基于过时数据通过检查。
	// 修复策略 = 三层减法 + 全局互斥锁串行化：
	//   effective_free = client_free - in_flight_pending - pre_reserved
	//   gate           = effective_free - thisTorrentSize >= threshold
	var reservedTorrentSize int64
	if glOnly.CleanupDiskProtect && glOnly.CleanupMinDiskSpaceGB > 0 {
		mu := PushMutex()
		mu.Lock()
		defer mu.Unlock()

		freeSpace, spaceErr := dl.GetClientFreeSpace(ctx)
		if spaceErr != nil {
			// fail-closed：磁盘保护启用时若无法读取空间，拒绝推送而非放行。
			// 旧实现 fail-open ("继续推送")是 Issue #299 的次因之一。
			sLogger().Warnf("[磁盘保护] %s: 获取磁盘空间失败，磁盘保护启用故拒绝推送: %v", dl.GetName(), spaceErr)
			return downloader.ErrInsufficientSpace
		}

		pendingBytes, pendingErr := dl.GetIncompletePendingBytes(ctx)
		if pendingErr != nil {
			sLogger().Warnf("[磁盘保护] %s: 查询 in-flight pending 失败，仅以 reserved 推算: %v", dl.GetName(), pendingErr)
			pendingBytes = 0
		}
		budget := GetDiskBudget()
		effectiveFreeBytes := freeSpace - pendingBytes - budget.Reserved()
		if effectiveFreeBytes < 0 {
			effectiveFreeBytes = 0
		}
		var torrentSize int64
		if torrent != nil {
			torrentSize = torrent.TorrentSize
		}
		if torrentSize <= 0 {
			if torrentData, readErr := os.ReadFile(filePath); readErr == nil {
				if parsedSize, sizeErr := qbit.ComputeTorrentSize(torrentData); sizeErr == nil {
					torrentSize = parsedSize
				}
			}
		}
		minBytes := int64(glOnly.CleanupMinDiskSpaceGB * 1024 * 1024 * 1024)
		if effectiveFreeBytes-torrentSize < minBytes {
			effGB := float64(effectiveFreeBytes) / (1024 * 1024 * 1024)
			tGB := float64(torrentSize) / (1024 * 1024 * 1024)
			freeGB := float64(freeSpace) / (1024 * 1024 * 1024)
			pendingGB := float64(pendingBytes) / (1024 * 1024 * 1024)
			reservedGB := float64(budget.Reserved()) / (1024 * 1024 * 1024)
			sLogger().Warnf("[磁盘保护] %s: 空间不足 (qBit可用 %.1f GB - 下载中待占用 %.1f GB - 本进程预留 %.1f GB = 有效 %.1f GB；有效 - 种子 %.1f GB < 保底 %.1f GB)，跳过推送: %s",
				dl.GetName(), freeGB, pendingGB, reservedGB, effGB, tGB, glOnly.CleanupMinDiskSpaceGB, filePath)
			if glOnly.CleanupEnabled {
				events.Publish(events.Event{Type: events.DiskSpaceLow, Source: "rss", At: time.Now()})
			}
			return downloader.ErrInsufficientSpace
		}
		// 通过检查后立刻预留，确保后续 worker 能看到本次扣减。
		// 推送失败会在下方失败分支 Release 归还。
		if torrentSize > 0 {
			budget.Reserve(torrentSize)
			reservedTorrentSize = torrentSize
		}
	}

	torrentData, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return fmt.Errorf("读取种子文件失败: %w", readErr)
	}

	opt := downloader.AddTorrentOptions{
		AddAtPaused: !dlInfo.AutoStart,
		SavePath:    downloadPath,
		Category:    category,
		Tags:        tags,
	}
	applySiteSpeedLimits(&opt, string(siteName))
	if downloadPath != "" {
		sLogger().Infof("使用自定义下载路径推送种子: %s -> %s", filePath, downloadPath)
	}
	result, pushErr := dl.AddTorrentFileEx(torrentData, opt)
	if pushErr != nil || !result.Success {
		// 推送失败：归还预留配额，避免 budget 被永久占用。
		// 注：torrent 可能为 nil（极少数情况下 GetTorrentBySiteAndHash 失败但仍走到这）；
		// 由 Reserve 的零值守卫保证 Release(0) no-op。
		if reservedTorrentSize > 0 {
			GetDiskBudget().Release(reservedTorrentSize)
		}
		errMsg := ""
		if pushErr != nil {
			errMsg = pushErr.Error()
		} else if result.Message != nil {
			errMsg = fmt.Sprintf("%v", result.Message)
		}
		np := false
		if updateErr := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
			Where("site_name = ? AND torrent_hash = ?", string(siteName), torrentHash).
			Updates(map[string]any{
				"is_pushed":   &np,
				"retry_count": gorm.Expr("retry_count + 1"),
				"last_error":  errMsg,
			}).Error; updateErr != nil {
			sLogger().Errorf("更新推送失败状态出错: %s, %v", filePath, updateErr)
		}
		if pushErr != nil {
			return fmt.Errorf("推送种子失败: %w", pushErr)
		}
		return fmt.Errorf("推送种子失败: %s", errMsg)
	}
	// 推送成功的预留**不**在此处归还。
	// 原因：AddTorrentFileEx 返回成功后，qBit 通常仍需 1~2 秒才会把新种子计入
	// torrents/info 的 amount_left（in-flight pending）。如果此处立刻 Release，
	// 这段窗口内并发 worker 看到 pre_reserved=0 且 pending 也不含本种子，会
	// 把同一份磁盘空间重复借给两个推送 —— 即 Issue #299 的原始 race。
	// 预留由 scheduler/cleanup_monitor.runOnce 的周期 Reset 归还（默认 30 分钟），
	// 期间 effective_free 略偏保守但不会越界。详见 disk_budget.go 顶部注释。

	pushed2 := true
	now := time.Now()
	taskID := result.Hash
	if taskID == "" {
		taskID = torrentHash
	}
	if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_hash = ?", string(siteName), torrentHash).
		Updates(map[string]any{
			"is_pushed":          &pushed2,
			"push_time":          now,
			"downloader_id":      &dlInfo.ID,
			"downloader_name":    dlInfo.Name,
			"downloader_task_id": taskID,
			"pause_on_free_end":  pauseOnFreeEnd,
		}).Error; err != nil {
		return fmt.Errorf("更新推送状态失败: %w", err)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("删除已推送种子失败: %w; torrentHash: %s", err, torrentHash)
	}

	sLogger().Infof("种子推送成功并删除: %s, torrentHash: %s, 下载器: %s, PauseOnFreeEnd: %v",
		filePath, torrentHash, dl.GetName(), pauseOnFreeEnd)

	if pauseOnFreeEnd && torrent.FreeEndTime != nil {
		torrent.DownloaderTaskID = taskID
		torrent.DownloaderName = dlInfo.Name
		torrent.DownloaderID = &dlInfo.ID
		torrent.PauseOnFreeEnd = true
		ScheduleTorrentForMonitoring(*torrent)
	}

	return nil
}

func attemptDownload(url, title, downloadDir string) (string, error) {
	return attemptDownloadWithContext(context.Background(), url, title, downloadDir)
}

func attemptDownloadWithContext(ctx context.Context, url, title, downloadDir string) (string, error) {
	// 使用 requests 库下载，支持 context
	resp, err := requests.Get(url, requests.WithContext(ctx), requests.WithTimeout(30*time.Second))
	if err != nil {
		return "", fmt.Errorf("下载种子失败: %v", err)
	}
	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP 状态码错误: %d", resp.StatusCode)
	}
	// 获取响应体字节
	bodyBytes := resp.Bytes()
	// 先计算种子的 torrentHash，确认响应体确实是合法 .torrent 后再落盘。
	// 旧逻辑先写文件再算 hash，站点返回 HTML/JSON 错误页时会留下无效 .torrent，后续推送扫描反复报错。
	torrentHash, err := qbit.ComputeTorrentHash(bodyBytes)
	if err != nil {
		return "", fmt.Errorf("下载到的内容不是有效种子文件，无法解析 info hash: status=%d, size=%d, preview=%q, err=%w", resp.StatusCode, len(bodyBytes), invalidTorrentPreview(bodyBytes), err)
	}
	// 创建下载目录
	if err = os.MkdirAll(downloadDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("创建下载目录失败: %v", err)
	}
	// 生成文件路径
	fileName := fmt.Sprintf("%s/%s.torrent", downloadDir, sanitizeTitle(title))
	file, err := os.Create(fileName)
	if err != nil {
		return "", fmt.Errorf("创建种子文件失败: %v", err)
	}
	defer file.Close()
	// 写入文件
	_, err = file.Write(bodyBytes)
	if err != nil {
		return "", fmt.Errorf("写入种子文件失败: %v", err)
	}
	// 下载成功
	return torrentHash, nil
}

func invalidTorrentPreview(data []byte) string {
	preview := strings.ToValidUTF8(string(data), "")
	preview = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, preview)
	preview = strings.TrimSpace(preview)
	preview = regexp.MustCompile(`\s+`).ReplaceAllString(preview, " ")
	runes := []rune(preview)
	if len(runes) > 160 {
		preview = string(runes[:160])
	}
	return preview
}

// 下载种子文件，包含重试机制
func downloadTorrent(url, title, downloadDir string, maxRetries int, retryDelay time.Duration) (string, error) {
	if err := os.MkdirAll(downloadDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("创建下载目录失败: %v", err)
	}
	var lastError error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		hash, err := attemptDownload(url, title, downloadDir)
		if err == nil {
			return hash, nil
		}
		lastError = err
		if attempt < maxRetries {
			sLogger().Infof("下载失败,重试中... (attempt: %d/%d), 错误: %v", attempt, maxRetries, lastError)
			time.Sleep(retryDelay)
		}
	}
	// 所有重试均失败
	return "", fmt.Errorf("下载失败: %v", lastError)
}

type rssTaskStats struct {
	total          atomic.Int64
	free           atomic.Int64
	downloaded     atomic.Int64
	skipped        atomic.Int64
	detailFailed   atomic.Int64
	downloadFailed atomic.Int64
}

func shouldSkipSiteDownload(torrent *models.TorrentInfo, downloadPath, fileBase string, maxRetry int) (bool, string) {
	if torrent == nil {
		return false, ""
	}
	if torrent.IsPushed != nil && *torrent.IsPushed {
		return true, "已推送，跳过重新下载"
	}
	if maxRetry > 0 && torrent.RetryCount >= maxRetry {
		return true, fmt.Sprintf("超过最大重试次数 %d", maxRetry)
	}
	if torrent.IsDownloaded {
		if _, statErr := os.Stat(filepath.Join(downloadPath, fileBase+".torrent")); statErr == nil {
			return true, "已下载且本地文件存在"
		}
	}
	return false, ""
}

func shouldSkipExistingTorrent(torrent *models.TorrentInfo) bool {
	if torrent == nil {
		return false
	}

	// If skipped and non-free, allow re-check after SkipRecheckHours
	if torrent.IsSkipped && !torrent.IsFree {
		if torrent.LastCheckTime != nil {
			elapsed := time.Since(*torrent.LastCheckTime)
			if elapsed >= SkipRecheckHours*time.Hour {
				return false // Allow re-check
			}
		}
		return true
	}

	if torrent.IsSkipped {
		return true
	}

	return torrent.IsPushed != nil && *torrent.IsPushed
}

func downloadWorkerUnified(
	ctx context.Context,
	wg *sync.WaitGroup,
	site UnifiedPTSite,
	torrentChan <-chan *gofeed.Item,
	rssCfg models.RSSConfig,
	stats *rssTaskStats,
) {
	defer wg.Done()
	siteName := site.SiteGroup()

	var gl models.SettingsGlobal
	if global.GlobalDB != nil {
		if g, err := core.NewConfigStore(global.GlobalDB).GetGlobalOnly(); err == nil {
			gl = g
		}
	}

	siteID := string(siteName)
	var siteHR bool
	var siteHRSeedTimeH int
	var siteDef *v2.SiteDefinition
	if def := v2.GetDefinitionRegistry().GetOrDefault(siteID); def != nil {
		siteDef = def
		siteHR = def.HREnabled
		siteHRSeedTimeH = def.HRSeedTimeHours
	}

	var filterSvc filter.FilterService
	if global.GlobalDB != nil {
		filterSvc = filter.NewFilterService(global.GlobalDB.DB)
	}

	// 检查 RSS 是否有关联的过滤规则
	hasAssociatedRules := false
	if filterSvc != nil && rssCfg.ID != 0 {
		rules, err := filterSvc.GetRulesForRSS(rssCfg.ID)
		if err == nil && len(rules) > 0 {
			hasAssociatedRules = true
			sLogger().Infof("RSS %s 关联了 %d 个过滤规则", rssCfg.Name, len(rules))
		}
	}

	for {
		select {
		case <-ctx.Done():
			sLogger().Warn("下载任务取消")
			return
		case item, ok := <-torrentChan:
			if !ok {
				return
			}
			var torrentURL string
			if len(item.Enclosures) > 0 {
				torrentURL = item.Enclosures[0].URL
			} else {
				torrentURL = ""
			}
			title := item.Title
			// 查询数据库记录
			torrent, err := global.GlobalDB.GetTorrentBySiteAndID(string(siteName), item.GUID)
			if err != nil {
				sLogger().Errorf("从数据库获取种子: %s 详情失败, %v", title, err)
				continue
			}
			if shouldSkipExistingTorrent(torrent) {
				sLogger().Infof("%s: 种子 %s 已跳过或已推送，直接跳过", title, item.GUID)
				stats.skipped.Add(1)
				continue
			}
			if notifier := getRSSNotifier(); notifier != nil {
				if rssCfg.NotifyMode == "all" || rssCfg.NotifyMode == "both" {
					_, torrentRef := extractTorrentRef(item)
					if torrentRef == "" {
						torrentRef = item.GUID
					}
					if torrentRef != "" {
						_ = notifier.NotifyNewItem(ctx, RSSItemNotice{
							RSS:       &rssCfg,
							FeedItem:  item,
							SiteName:  string(siteName),
							TorrentID: torrentRef,
						})
					}
				}
			}
			stats.total.Add(1)
			// 获取种子详情 (使用 UnifiedPTSite 接口，返回 *v2.TorrentItem)
			detail, err := site.GetTorrentDetails(item)
			if err != nil {
				sLogger().Errorf("[%s] %s: 获取种子详情失败, %v", siteName, title, err)
				stats.detailFailed.Add(1)
				continue
			}
			// 使用 v2.TorrentItem 的方法（此处传 0 给 sizeLimitGB，全局大小硬上限由 filter.Decide 统一检查，
			// 避免过滤规则通道绕过全局限制）
			canFinished := detail.CanbeFinished(gl.DownloadLimitEnabled, gl.DownloadSpeedLimit, 0)
			isFree := detail.IsFree()

			freeEndTime := detail.GetFreeEndTime()
			if isFree && canFinished && freeEndTime != nil && gl.MinFreeMinutes > 0 {
				remaining := time.Until(*freeEndTime)
				if remaining > 0 && remaining < time.Duration(gl.MinFreeMinutes)*time.Minute {
					canFinished = false
					sLogger().Infof("种子: %s 免费剩余时间不足 (%.0f分钟 < %d分钟)，跳过",
						title, remaining.Minutes(), gl.MinFreeMinutes)
				}
			}

			// 获取种子的标签/副标题用于过滤匹配
			detailTag := detail.GetSubTitle()
			sizeGB := float64(detail.SizeBytes) / 1024 / 1024 / 1024

			// Sprint 2: 'filtered' 模式通知钩子。需要详情后才能匹配（subtitle/size）
			// 与渲染模板。复用 GetTorrentDetails 已有的站点级 PersistentRateLimiter，
			// 因此不会引入额外站点压力。
			if notifier := getRSSNotifier(); notifier != nil &&
				(rssCfg.NotifyMode == "filtered" || rssCfg.NotifyMode == "both") &&
				filterSvc != nil && rssCfg.ID != 0 {
				matched, rule := filterSvc.ShouldNotifyForRSSWithInput(
					filter.MatchInput{Title: title, Tag: detailTag, SizeGB: sizeGB},
					isFree, rssCfg.ID,
				)
				if matched {
					_, torrentRef := extractTorrentRef(item)
					if torrentRef == "" {
						torrentRef = item.GUID
					}
					if torrentRef != "" {
						_ = notifier.NotifyFilteredItem(ctx, RSSFilteredNotice{
							RSS:       &rssCfg,
							Torrent:   detail,
							Rule:      rule,
							SiteName:  string(siteName),
							TorrentID: torrentRef,
						})
					}
				}
			}

			// 统一通过 filter.Decide 做完整决策：全局大小硬上限 → 过滤规则通道 → 免费通道
			var decision filter.Decision
			if filterSvc != nil && rssCfg.ID != 0 && hasAssociatedRules {
				decision = filterSvc.Decide(filter.DecisionContext{
					Input:      filter.MatchInput{Title: title, Tag: detailTag, SizeGB: sizeGB},
					IsFree:     isFree,
					CanFinish:  canFinished,
					GlobalSize: gl.TorrentSizeGB,
					FilterMode: rssCfg.GetEffectiveFilterMode(&gl),
				}, rssCfg.ID)
			} else {
				decision = filter.DecideWithoutRules(filter.DecisionContext{
					Input:      filter.MatchInput{Title: title, Tag: detailTag, SizeGB: sizeGB},
					IsFree:     isFree,
					CanFinish:  canFinished,
					GlobalSize: gl.TorrentSizeGB,
					FilterMode: rssCfg.GetEffectiveFilterMode(&gl),
				})
			}

			matchedRule := decision.MatchedRule
			downloadSource := decision.Source
			if downloadSource == "" {
				downloadSource = filter.SourceFreeDownload
			}
			if matchedRule != nil && decision.Source == filter.SourceFilterRule {
				sLogger().Infof("种子 %s (tag: %s) 匹配 RSS 关联过滤规则: %s (require_free=%v, min=%d, max=%d)", title, detailTag, matchedRule.Name, matchedRule.RequireFree, matchedRule.MinSizeGB, matchedRule.MaxSizeGB)
			}

			// 更新种子状态（标记跳过或继续下载）
			if torrent == nil {
				now := time.Now()
				cat := ""
				if len(item.Categories) > 0 {
					cat = strings.Join(item.Categories, "/")
				}
				// 使用 v2.TorrentItem 的方法获取 FreeLevel 和 FreeEndTime
				freeLevel := detail.GetFreeLevel()
				freeEndTime := detail.GetFreeEndTime()
				torrent = &models.TorrentInfo{
					SiteName:       string(siteName),
					TorrentID:      item.GUID,
					FreeLevel:      freeLevel,
					FreeEndTime:    freeEndTime,
					Title:          title,
					Category:       cat,
					Tag:            rssCfg.Tag,
					LastCheckTime:  &now,
					DownloadSource: downloadSource,
					TorrentSize:    detail.SizeBytes,
					HasHR:          detail.HasHR || siteHR,
					HRSeedTimeH:    calcHRSeedTimeForTorrent(siteDef, siteHRSeedTimeH, detail.SizeBytes),
				}
				if matchedRule != nil {
					torrent.FilterRuleID = &matchedRule.ID
				}
			}

			shouldDownload := decision.ShouldDownload

			if isFree {
				stats.free.Add(1)
			}

			if !shouldDownload {
				torrent.IsSkipped = true
				reason := decision.Reason
				if reason == "" {
					reason = buildSkipReason(isFree, canFinished, false)
				}
				sLogger().Infof("种子: %s 不满足下载条件，跳过 (原因: %s)", title, reason)
			} else {
				torrent.IsSkipped = false
			}
			torrent.IsFree = isFree

			err = global.GlobalDB.WithTransaction(func(tx *gorm.DB) error {
				// 使用 GORM 的 upsert 功能
				err = tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "site_name"}, {Name: "torrent_id"}},
					DoUpdates: clause.AssignmentColumns([]string{"is_skipped", "free_level", "free_end_time", "title", "category", "tag", "last_check_time", "is_free", "download_source", "filter_rule_id"}),
				}).Create(torrent).Error
				return err
			})
			if err != nil {
				sLogger().Errorf("更新种子:%s 状态失败, %v", title, err)
				continue
			}
			if torrent.IsSkipped {
				continue
			}
			// 下载种子并更新哈希值
			if shouldDownload {
				// 先在事务外执行 HTTP 下载操作
				homeDir, _ := os.UserHomeDir()
				base, berr := utils.ResolveDownloadBase(homeDir, models.WorkDir, gl.DownloadDir)
				if berr != nil {
					sLogger().Errorf("%s: 解析下载路径失败, %v", title, berr)
					continue
				}
				sub := utils.SubPathFromTag(rssCfg.Tag)
				downloadPath := filepath.Join(base, sub)
				if _, mkErr := os.Stat(downloadPath); os.IsNotExist(mkErr) {
					sLogger().Infof("创建下载目录: %s", downloadPath)
					_ = os.MkdirAll(downloadPath, 0o755)
				}
				// 文件命名统一为 siteName-torrentID.torrent，避免重复与歧义
				fileBase := fmt.Sprintf("%s-%s", strings.ToLower(string(siteName)), item.GUID)
				if skip, reason := shouldSkipSiteDownload(torrent, downloadPath, fileBase, gl.MaxRetry); skip {
					sLogger().Infof("种子: %s 跳过重新下载 (原因: %s)", title, reason)
					stats.skipped.Add(1)
					continue
				}
				hash, downloadErr := site.DownloadTorrent(torrentURL, fileBase, downloadPath)
				if downloadErr != nil {
					sLogger().Errorf("%s: 种子下载失败, %v", title, downloadErr)
					stats.downloadFailed.Add(1)
					continue
				}
				torrentFile := filepath.Join(downloadPath, fileBase+".torrent")
				if _, statErr := os.Stat(torrentFile); os.IsNotExist(statErr) {
					sLogger().Warnf("种子文件不存在但标记已下载: %s", title)
					// 修正数据库状态
					torrent.IsDownloaded = false
					torrent.TorrentHash = nil
					global.GlobalDB.DB.Save(torrent)
					sLogger().Infof("已更新数据库记录: %s", title)
					continue
				}
				// 更新数据库记录
				torrent.IsDownloaded = true
				torrent.TorrentHash = &hash
				// 更新指定字段
				now := time.Now()
				err = global.GlobalDB.DB.Model(&models.TorrentInfo{}).
					Where("site_name = ? AND torrent_id = ?", torrent.SiteName, torrent.TorrentID).
					Updates(map[string]any{
						"torrent_hash":    torrent.TorrentHash,
						"is_downloaded":   torrent.IsDownloaded,
						"is_free":         torrent.IsFree,
						"last_check_time": &now,
						"download_source": downloadSource,
						"filter_rule_id":  torrent.FilterRuleID,
					}).Error
				if err != nil {
					sLogger().Errorf("%s: 数据库更新失败, %v", title, err)
				} else {
					sLogger().Info("种子下载成功并记录到数据库 ", title)
					stats.downloaded.Add(1)
				}
			}
		}
	}
}

func ProcessTorrentsWithDBUpdate(
	ctx context.Context,
	qbitClient *qbit.QbitClient,
	dirPath, category, tags string,
	siteName models.SiteGroup,
) error {
	// 获取目录下的所有种子文件（移出事务）
	filePaths, err := qbit.GetTorrentFilesPath(dirPath)
	if err != nil {
		sLogger().Error("无法读取目录", dirPath, err)
		return fmt.Errorf("无法读取目录: %v", err)
	}
	// 为每个种子创建独立处理
	for _, file := range filePaths {
		// 使用独立事务处理每个种子
		err := processSingleTorrent(ctx, qbitClient, file, category, tags, siteName)
		if err != nil {
			sLogger().Errorf("处理种子失败: %s, %v", file, err)
			// 记录错误但继续处理其他种子
		}
	}
	sLogger().Info("所有种子处理完成")
	return nil
}

func processSingleTorrent(
	ctx context.Context,
	qbitClient *qbit.QbitClient,
	filePath, category, tags string,
	siteName models.SiteGroup,
) error {
	torrentHash, err := qbit.ComputeTorrentHashWithPath(filePath)
	if err != nil {
		sLogger().Errorf("计算种子哈希失败: %s, %v", filePath, err)
		return fmt.Errorf("计算种子哈希失败: %w", err)
	}

	torrent, err := global.GlobalDB.GetTorrentBySiteAndHash(string(siteName), torrentHash)
	if err != nil {
		sLogger().Errorf("查询种子信息失败: %s, %v", filePath, err)
		return fmt.Errorf("查询种子信息失败: %w", err)
	}
	if torrent == nil {
		sLogger().Warnf("数据库不存在记录，删除孤立种子文件: %s, hash: %s", filePath, torrentHash)
		if err = os.Remove(filePath); err != nil {
			sLogger().Errorf("删除孤立种子失败: %s, %v", filePath, err)
			return fmt.Errorf("删除孤立种子失败: %w", err)
		}
		return nil
	}
	isExpired := torrent.GetExpired()
	sLogger().Infof("[过期检查] 种子: %s, hash: %s, FreeEndTime: %v, IsExpired(DB): %v, GetExpired(): %v",
		torrent.Title, torrentHash,
		torrent.FreeEndTime, torrent.IsExpired, isExpired)
	if isExpired {
		sLogger().Warnf("[过期] 种子免费期已过期，标记并删除: %s, FreeEndTime: %v", filePath, torrent.FreeEndTime)
		if err = global.GlobalDB.DB.Model(&models.TorrentInfo{}).
			Where("site_name = ? AND torrent_hash = ?", siteName, torrentHash).
			Updates(map[string]any{
				"is_expired": true,
				"last_error": "种子已过期，未推送",
			}).Error; err != nil {
			return fmt.Errorf("标记过期状态失败: %w", err)
		}
		if err = os.Remove(filePath); err != nil {
			sLogger().Errorf("[过期] 删除过期种子失败: %s, %v", filePath, err)
		} else {
			sLogger().Infof("[过期] 已删除过期种子: %s", filePath)
		}
		return nil
	}

	gl, _ := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	if gl.RetainHours > 0 && torrent.LastCheckTime != nil {
		cutoff := time.Now().Add(-time.Duration(gl.RetainHours) * time.Hour)
		if torrent.IsPushed == nil || !*torrent.IsPushed {
			if torrent.LastCheckTime.Before(cutoff) {
				sLogger().Infof("超过保留时长(%dh)，删除未推送种子: %s", gl.RetainHours, filePath)
				_ = os.Remove(filePath)
				return nil
			}
		}
	}

	if torrent.IsPushed != nil && *torrent.IsPushed {
		sLogger().Infof("种子已推送，删除本地文件: %s", filePath)
		if err = os.Remove(filePath); err != nil {
			return fmt.Errorf("删除已推送种子失败: %w", err)
		}
		return nil
	}

	if torrent.GetExpired() {
		sLogger().Infof("种子已过期，删除: %s", filePath)
		_ = os.Remove(filePath)
		return nil
	}

	exists, err := qbitClient.CheckTorrentExists(torrentHash)
	if err != nil {
		return fmt.Errorf("检查种子存在失败: %w", err)
	}
	if exists {
		sLogger().Infof("种子已存在于 qBittorrent: %s", filePath)
		pushed := true
		if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
			Where("site_name = ? AND torrent_hash = ?", string(siteName), torrentHash).
			Updates(map[string]any{
				"is_pushed": &pushed,
				"push_time": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("更新数据库状态失败: %w", err)
		}
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("删除已存在种子失败: %w", err)
		}
		return nil
	}

	sLogger().Infof("推送新种子到 qBittorrent: %s\n", filePath)

	glOnly, _ := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	if glOnly.MaxRetry > 0 && torrent.RetryCount >= glOnly.MaxRetry {
		sLogger().Warnf("超过最大重试次数(%d)，删除文件: %s", glOnly.MaxRetry, filePath)
		_ = os.Remove(filePath)
		return nil
	}
	if pushErr := qbitClient.ProcessSingleTorrentFile(ctx, filePath, category, tags); pushErr != nil {
		np := false
		if updateErr := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
			Where("site_name = ? AND torrent_hash = ?", string(siteName), torrentHash).
			Updates(map[string]any{
				"is_pushed":   &np,
				"retry_count": gorm.Expr("retry_count + 1"),
				"last_error":  pushErr.Error(),
			}).Error; updateErr != nil {
			sLogger().Errorf("更新推送失败状态出错: %s, %v", filePath, updateErr)
		}
		return fmt.Errorf("推送种子失败: %w", pushErr)
	}

	pushed2 := true
	if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_hash = ?", string(siteName), torrentHash).
		Updates(map[string]any{
			"is_pushed": &pushed2,
			"push_time": time.Now(),
		}).Error; err != nil {
		return fmt.Errorf("更新推送状态失败: %w", err)
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("删除已推送种子失败: %w; torrentHash: %s", err, torrentHash)
	}
	sLogger().Infof("种子推送成功并删除: %s, torrentHash: %s", filePath, torrentHash)
	return nil
}

func sanitizeTitle(title string) string {
	// 定义允许的字符（字母、数字、空格、下划线、短横线）
	re := regexp.MustCompile(`[^a-zA-Z0-9\s_-]`)
	// 替换非法字符为空
	sanitized := re.ReplaceAllString(title, "")
	// 替换连续空格为单个空格
	sanitized = strings.Join(strings.Fields(sanitized), " ")
	return strings.TrimSpace(sanitized)
}

// FetchAndDownloadFreeRSSUnified 使用 UnifiedPTSite 接口获取并下载免费 RSS 种子
func FetchAndDownloadFreeRSSUnified(ctx context.Context, m UnifiedPTSite, rssCfg models.RSSConfig) error {
	startTime := time.Now()
	siteName := m.SiteGroup()
	sLogger().Infof("[RSS任务开始] 站点=%s, RSS=%s, URL=%s", siteName, rssCfg.Name, utils.SanitizeURL(rssCfg.URL))

	if global.GlobalDB == nil {
		return errors.New("配置未就绪: DB 不可用")
	}
	store := core.NewConfigStore(global.GlobalDB)
	gl, err := store.GetGlobalOnly()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	base := gl.DownloadDir
	if strings.TrimSpace(base) == "" {
		return errors.New("配置未就绪: 下载目录为空")
	}
	if !m.IsEnabled() {
		return errors.New(enableError)
	}

	feed, err := fetchRSSFeed(rssCfg.URL)
	if err != nil {
		sLogger().Errorf("[RSS任务失败] 站点=%s, RSS=%s, 错误=%v", siteName, rssCfg.Name, err)
		return err
	}
	sLogger().Infof("[RSS解析完成] 站点=%s, RSS=%s, 种子数量=%d", siteName, rssCfg.Name, len(feed.Items))

	ctxWithTimeout, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var wg sync.WaitGroup
	torrentChan := make(chan *gofeed.Item, len(feed.Items))

	var stats rssTaskStats

	concurrency := rssCfg.GetEffectiveConcurrency(&gl)
	sLogger().Infof("RSS %s 使用并发数: %d", rssCfg.Name, concurrency)

	for range concurrency {
		wg.Add(1)
		go downloadWorkerUnified(
			ctxWithTimeout,
			&wg,
			m,
			torrentChan,
			rssCfg,
			&stats,
		)
	}

	for _, item := range feed.Items {
		if len(item.Enclosures) > 0 {
			select {
			case <-ctxWithTimeout.Done():
				sLogger().Info("任务被取消")
				close(torrentChan)
				wg.Wait()
				return ctxWithTimeout.Err()
			case torrentChan <- item:
			}
		}
	}
	close(torrentChan)
	wg.Wait()

	duration := time.Since(startTime)
	sLogger().Infof("[RSS任务完成] 站点=%s, RSS=%s, 耗时=%v, 总数=%d, 免费=%d, 已下载=%d, 跳过=%d, 详情失败=%d, 下载失败=%d",
		siteName, rssCfg.Name, duration.Round(time.Millisecond),
		stats.total.Load(), stats.free.Load(), stats.downloaded.Load(),
		stats.skipped.Load(), stats.detailFailed.Load(), stats.downloadFailed.Load())
	return nil
}

func fetchRSSFeed(url string) (*gofeed.Feed, error) {
	return fetchRSSFeedWithContext(context.Background(), url)
}

// fetchRSSFeedWithContext fetches an RSS feed with a real browser User-Agent so
// Cloudflare-fronted PT trackers (e.g. gtkpw, agsvpt) don't drop the TLS handshake.
// gofeed's default ParseURL sets a generic UA that is regularly RST'd by these CDNs.
func fetchRSSFeedWithContext(ctx context.Context, url string) (*gofeed.Feed, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("构造 RSS 请求失败: %w", err)
	}
	req.Header.Set("User-Agent", rssUserAgent)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.5")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("解析 RSS 失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("解析 RSS 失败: HTTP %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	parser := gofeed.NewParser()
	feed, err := parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("解析 RSS 失败: %w", err)
	}
	return feed, nil
}

const rssUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"

// FetchAndDownloadFreeRSS 旧的泛型版本（已废弃，请使用 FetchAndDownloadFreeRSSUnified）
// Deprecated: Use FetchAndDownloadFreeRSSUnified instead for new implementations
func FetchAndDownloadFreeRSS[T models.ResType](ctx context.Context, siteName models.SiteGroup, m PTSiteInter[T], rssCfg models.RSSConfig) error {
	// 每次运行从 DB 读取最新配置并更新目录缓存（以 DB 为唯一配置源）
	if global.GlobalDB == nil {
		return errors.New("配置未就绪: DB 不可用")
	}
	store := core.NewConfigStore(global.GlobalDB)
	gl, err := store.GetGlobalOnly()
	if err != nil {
		return fmt.Errorf("加载配置失败: %w", err)
	}
	base := gl.DownloadDir
	if strings.TrimSpace(base) == "" {
		return errors.New("配置未就绪: 下载目录为空")
	}
	if !m.IsEnabled() {
		return errors.New(enableError)
	}
	// DownloadSubPath 前端移除，允许为空；使用 Tag 作为子目录
	feed, err := fetchRSSFeed(rssCfg.URL)
	if err != nil {
		return err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var wg sync.WaitGroup
	torrentChan := make(chan *gofeed.Item, len(feed.Items))

	// 获取有效的并发数（RSS 配置优先，否则使用全局配置）
	concurrency := rssCfg.GetEffectiveConcurrency(&gl)
	sLogger().Infof("RSS %s 使用并发数: %d", rssCfg.Name, concurrency)

	// 启动多个下载 Worker
	for range concurrency {
		wg.Add(1)
		go downloadWorker(
			ctxWithTimeout,
			siteName,
			&wg,
			m,
			torrentChan,
			rssCfg,
		)
	}
	// 将种子发送到下载队列
	for _, item := range feed.Items {
		if len(item.Enclosures) > 0 {
			select {
			case <-ctxWithTimeout.Done():
				sLogger().Info("任务被取消")
				close(torrentChan)
				wg.Wait()
				return ctxWithTimeout.Err()
			case torrentChan <- item:
			}
		}
	}
	close(torrentChan)
	wg.Wait()
	return nil
}

// downloadWorker 旧的泛型版本（已废弃，请使用 downloadWorkerUnified）
// Deprecated: Use downloadWorkerUnified instead for new implementations
func downloadWorker[T models.ResType](
	ctx context.Context,
	siteName models.SiteGroup,
	wg *sync.WaitGroup,
	site PTSiteInter[T],
	torrentChan <-chan *gofeed.Item,
	rssCfg models.RSSConfig,
) {
	defer wg.Done()
	// 读取一次全局限制配置，用于 CanbeFinished
	var gl models.SettingsGlobal
	if global.GlobalDB != nil {
		if g, err := core.NewConfigStore(global.GlobalDB).GetGlobalOnly(); err == nil {
			gl = g
		}
	}

	// 初始化过滤服务
	var filterSvc filter.FilterService
	if global.GlobalDB != nil {
		filterSvc = filter.NewFilterService(global.GlobalDB.DB)
	}

	// 检查 RSS 是否有关联的过滤规则
	hasAssociatedRules := false
	if filterSvc != nil && rssCfg.ID != 0 {
		rules, err := filterSvc.GetRulesForRSS(rssCfg.ID)
		if err == nil && len(rules) > 0 {
			hasAssociatedRules = true
			sLogger().Infof("RSS %s 关联了 %d 个过滤规则", rssCfg.Name, len(rules))
		}
	}

	for {
		select {
		case <-ctx.Done():
			sLogger().Warn("下载任务取消")
			return
		case item, ok := <-torrentChan:
			if !ok {
				return
			}
			var torrentURL string
			if len(item.Enclosures) > 0 {
				torrentURL = item.Enclosures[0].URL
			} else {
				torrentURL = ""
			}
			title := item.Title
			// 查询数据库记录
			torrent, err := global.GlobalDB.GetTorrentBySiteAndID(string(siteName), item.GUID)
			if err != nil {
				sLogger().Errorf("从数据库获取种子: %s 详情失败, %v", title, err)
				continue
			}
			if shouldSkipExistingTorrent(torrent) {
				sLogger().Infof("%s: 种子 %s 已跳过或已推送，直接跳过", title, item.GUID)
				continue
			}
			// 获取种子详情
			resDetail, err := site.GetTorrentDetails(item)
			if err != nil {
				sLogger().Errorf("[%s] %s: 获取种子详情失败, %v", siteName, title, err)
				continue
			}
			detail := resDetail.Data
			// 全局大小硬上限由 filter.Decide 统一处理，此处传 0 避免重复检查
			canFinished := detail.CanbeFinished(global.GetSlogger(), gl.DownloadLimitEnabled, gl.DownloadSpeedLimit, 0)
			isFree := detail.IsFree()

			legacyFreeEndTime := detail.GetFreeEndTime()
			if isFree && canFinished && legacyFreeEndTime != nil && gl.MinFreeMinutes > 0 {
				remaining := time.Until(*legacyFreeEndTime)
				if remaining > 0 && remaining < time.Duration(gl.MinFreeMinutes)*time.Minute {
					canFinished = false
					sLogger().Infof("种子: %s 免费剩余时间不足 (%.0f分钟 < %d分钟)，跳过",
						title, remaining.Minutes(), gl.MinFreeMinutes)
				}
			}

			// 获取种子的标签/副标题用于过滤匹配
			detailTag := detail.GetSubTitle()
			sizeGB := float64(detail.GetSizeBytes()) / 1024 / 1024 / 1024

			var decision filter.Decision
			if filterSvc != nil && rssCfg.ID != 0 && hasAssociatedRules {
				decision = filterSvc.Decide(filter.DecisionContext{
					Input:      filter.MatchInput{Title: title, Tag: detailTag, SizeGB: sizeGB},
					IsFree:     isFree,
					CanFinish:  canFinished,
					GlobalSize: gl.TorrentSizeGB,
					FilterMode: rssCfg.GetEffectiveFilterMode(&gl),
				}, rssCfg.ID)
			} else {
				decision = filter.DecideWithoutRules(filter.DecisionContext{
					Input:      filter.MatchInput{Title: title, Tag: detailTag, SizeGB: sizeGB},
					IsFree:     isFree,
					CanFinish:  canFinished,
					GlobalSize: gl.TorrentSizeGB,
					FilterMode: rssCfg.GetEffectiveFilterMode(&gl),
				})
			}

			matchedRule := decision.MatchedRule
			downloadSource := decision.Source
			if downloadSource == "" {
				downloadSource = filter.SourceFreeDownload
			}
			if matchedRule != nil && decision.Source == filter.SourceFilterRule {
				sLogger().Infof("种子 %s (tag: %s) 匹配 RSS 关联过滤规则: %s (require_free=%v, min=%d, max=%d)", title, detailTag, matchedRule.Name, matchedRule.RequireFree, matchedRule.MinSizeGB, matchedRule.MaxSizeGB)
			}

			// 更新种子状态（标记跳过或继续下载）
			if torrent == nil {
				now := time.Now()
				cat := ""
				if len(item.Categories) > 0 {
					cat = strings.Join(item.Categories, "/")
				}
				torrent = &models.TorrentInfo{
					SiteName:       string(siteName),
					TorrentID:      item.GUID,
					FreeLevel:      detail.GetFreeLevel(),
					FreeEndTime:    detail.GetFreeEndTime(),
					Title:          title,
					Category:       cat,
					Tag:            rssCfg.Tag,
					LastCheckTime:  &now,
					DownloadSource: downloadSource,
				}
				if matchedRule != nil {
					torrent.FilterRuleID = &matchedRule.ID
				}
			}

			shouldDownload := decision.ShouldDownload

			if !shouldDownload {
				torrent.IsSkipped = true
				reason := decision.Reason
				if reason == "" {
					reason = buildSkipReason(isFree, canFinished, false)
				}
				sLogger().Infof("种子: %s 不满足下载条件，跳过 (原因: %s)", title, reason)
			} else {
				torrent.IsSkipped = false
			}
			torrent.IsFree = isFree

			err = global.GlobalDB.WithTransaction(func(tx *gorm.DB) error {
				// 使用 GORM 的 upsert 功能
				err = tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "site_name"}, {Name: "torrent_id"}},
					DoUpdates: clause.AssignmentColumns([]string{"is_skipped", "free_level", "free_end_time", "title", "category", "tag", "last_check_time", "is_free", "download_source", "filter_rule_id"}),
				}).Create(torrent).Error
				return err
			})
			if err != nil {
				sLogger().Errorf("更新种子:%s 状态失败, %v", title, err)
				continue
			}
			if torrent.IsSkipped {
				continue
			}
			// 下载种子并更新哈希值
			if shouldDownload {
				err = global.GlobalDB.WithTransaction(func(tx *gorm.DB) error {
					homeDir, _ := os.UserHomeDir()
					base, berr := utils.ResolveDownloadBase(homeDir, models.WorkDir, gl.DownloadDir)
					if berr != nil {
						return berr
					}
					sub := utils.SubPathFromTag(rssCfg.Tag)
					downloadPath := filepath.Join(base, sub)
					if _, mkErr := os.Stat(downloadPath); os.IsNotExist(mkErr) {
						sLogger().Infof("创建下载目录: %s", downloadPath)
						_ = os.MkdirAll(downloadPath, 0o755)
					}
					// 文件命名统一为 siteName-torrentID.torrent，避免重复与歧义
					fileBase := fmt.Sprintf("%s-%s", strings.ToLower(string(siteName)), item.GUID)
					if skip, reason := shouldSkipSiteDownload(torrent, downloadPath, fileBase, gl.MaxRetry); skip {
						sLogger().Infof("种子: %s 跳过重新下载 (原因: %s)", title, reason)
						return nil
					}
					hash, downloadErr := site.DownloadTorrent(torrentURL, fileBase, downloadPath)
					if downloadErr != nil {
						return fmt.Errorf("种子下载失败: %w", downloadErr)
					}
					torrentFile := filepath.Join(downloadPath, fileBase+".torrent")
					if _, err = os.Stat(torrentFile); os.IsNotExist(err) {
						sLogger().Warnf("种子文件不存在但标记已下载: %s", title)
						// 修正数据库状态
						torrent.IsDownloaded = false
						torrent.TorrentHash = nil
						tx.Save(torrent)
						sLogger().Infof("已更新数据库记录: %s", title)
						return nil
					}
					// 更新数据库记录
					torrent.IsDownloaded = true
					torrent.TorrentHash = &hash
					// 更新指定字段
					now := time.Now()
					err = tx.Model(&models.TorrentInfo{}).
						Where("site_name = ? AND torrent_id = ?", torrent.SiteName, torrent.TorrentID).
						Updates(map[string]any{
							"torrent_hash":    torrent.TorrentHash,
							"is_downloaded":   torrent.IsDownloaded,
							"is_free":         torrent.IsFree,
							"last_check_time": &now,
							"download_source": downloadSource,
							"filter_rule_id":  torrent.FilterRuleID,
						}).Error
					return err
				})
				if err != nil {
					sLogger().Errorf("%s: 事务执行失败, %v", title, err)
				} else {
					sLogger().Info("种子下载成功并记录到数据库 ", title)
				}
			}
		}
	}
}

// calcHRSeedTimeForTorrent returns the per-torrent HR seed time (hours).
// If the site definition has size-tiered rules (HRSeedTimeRules), it calculates
// based on the torrent size; otherwise falls back to the flat site-wide value.
func calcHRSeedTimeForTorrent(def *v2.SiteDefinition, fallbackH int, sizeBytes int64) int {
	if def != nil && len(def.HRSeedTimeRules) > 0 {
		return def.CalcHRSeedTimeH(sizeBytes)
	}
	return fallbackH
}

func extractTorrentRef(item *gofeed.Item) (siteName, torrentID string) {
	if item == nil || item.Link == "" {
		return "", ""
	}
	u, err := url.Parse(item.Link)
	if err != nil {
		return "", ""
	}
	siteName = u.Host
	if id := u.Query().Get("id"); id != "" {
		return siteName, id
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err == nil && p != "" {
			return siteName, p
		}
	}
	return siteName, ""
}
