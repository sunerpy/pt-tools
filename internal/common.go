package internal

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
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

// GetDownloaderForRSSWithInfo 根据 RSS 配置获取下载器及其信息
// 优先使用全局 DownloaderManager（连接池复用），回退到直接创建实例
func GetDownloaderForRSSWithInfo(rssCfg models.RSSConfig) (downloader.Downloader, *DownloaderInfo, error) {
	if global.GlobalDB == nil {
		return nil, nil, errors.New("数据库未初始化")
	}

	var dlSetting models.DownloaderSetting

	if rssCfg.DownloaderID != nil {
		if err := global.GlobalDB.DB.First(&dlSetting, *rssCfg.DownloaderID).Error; err != nil {
			return nil, nil, fmt.Errorf("获取指定下载器失败: %w", err)
		}
		if !dlSetting.Enabled {
			return nil, nil, fmt.Errorf("指定的下载器 %s 未启用", dlSetting.Name)
		}
	} else {
		if err := global.GlobalDB.DB.Where("is_default = ?", true).First(&dlSetting).Error; err != nil {
			return nil, nil, fmt.Errorf("获取默认下载器失败: %w", err)
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
	dl, dlInfo, err := GetDownloaderForRSSWithInfo(rssCfg)
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

	// 磁盘空间预检查：空间低于保底阈值时拒绝推送
	if glOnly.CleanupDiskProtect && glOnly.CleanupMinDiskSpaceGB > 0 {
		freeSpace, spaceErr := dl.GetClientFreeSpace(ctx)
		if spaceErr != nil {
			sLogger().Warnf("[磁盘保护] 获取磁盘空间失败，继续推送: %v", spaceErr)
		} else {
			freeGB := float64(freeSpace) / (1024 * 1024 * 1024)
			if freeGB < glOnly.CleanupMinDiskSpaceGB {
				sLogger().Warnf("[磁盘保护] %s: 磁盘空间不足 (%.1f GB < %.1f GB)，跳过推送: %s",
					dl.GetName(), freeGB, glOnly.CleanupMinDiskSpaceGB, filePath)
				if glOnly.CleanupEnabled {
					events.Publish(events.Event{Type: events.DiskSpaceLow, Source: "rss", At: time.Now()})
				}
				return downloader.ErrInsufficientSpace
			}
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
	if downloadPath != "" {
		sLogger().Infof("使用自定义下载路径推送种子: %s -> %s", filePath, downloadPath)
	}
	result, pushErr := dl.AddTorrentFileEx(torrentData, opt)
	if pushErr != nil || !result.Success {
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
	// 获取响应体字节
	bodyBytes := resp.Bytes()
	// 写入文件
	_, err = file.Write(bodyBytes)
	if err != nil {
		return "", fmt.Errorf("写入种子文件失败: %v", err)
	}
	// 计算种子的 torrentHash
	torrentHash, err := qbit.ComputeTorrentHash(bodyBytes)
	if err != nil {
		return "", fmt.Errorf("计算种子哈希失败: %v", err)
	}
	// 下载成功
	return torrentHash, nil
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
	if def := v2.GetDefinitionRegistry().GetOrDefault(siteID); def != nil {
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
			// 如果种子已跳过或已推送，直接跳过
			if torrent != nil && (torrent.IsSkipped || torrent.IsPushed != nil) {
				sLogger().Infof("%s: 种子 %s 已跳过或已推送，直接跳过", title, item.GUID)
				stats.skipped.Add(1)
				continue
			}
			stats.total.Add(1)
			// 获取种子详情 (使用 UnifiedPTSite 接口，返回 *v2.TorrentItem)
			detail, err := site.GetTorrentDetails(item)
			if err != nil {
				sLogger().Errorf("[%s] %s: 获取种子详情失败, %v", siteName, title, err)
				stats.detailFailed.Add(1)
				continue
			}
			// 使用 v2.TorrentItem 的方法
			canFinished := detail.CanbeFinished(gl.DownloadLimitEnabled, gl.DownloadSpeedLimit, gl.TorrentSizeGB)
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

			// 检查过滤规则匹配
			var matchedRule *models.FilterRule
			var shouldDownloadByFilter bool
			downloadSource := "free_download"

			if filterSvc != nil && rssCfg.ID != 0 {
				// 构建匹配输入，包含标题和标签
				matchInput := filter.MatchInput{
					Title: title,
					Tag:   detailTag,
				}

				// 优先使用 RSS 关联的过滤规则（多对多关联）
				if hasAssociatedRules {
					shouldDownloadByFilter, matchedRule = filterSvc.ShouldDownloadForRSSWithInput(matchInput, isFree, rssCfg.ID)
					if matchedRule != nil {
						downloadSource = "filter_rule"
						sLogger().Infof("种子 %s (tag: %s) 匹配 RSS 关联过滤规则: %s (require_free=%v)", title, detailTag, matchedRule.Name, matchedRule.RequireFree)
					}
				} else {
					// RSS 没有关联规则时，不进行过滤规则匹配
					shouldDownloadByFilter = false
					matchedRule = nil
				}
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
					HRSeedTimeH:    siteHRSeedTimeH,
				}
				if matchedRule != nil {
					torrent.FilterRuleID = &matchedRule.ID
				}
			}

			// 决定是否下载：
			// 1. 通过过滤规则匹配且满足条件
			// 2. 或者通过免费下载逻辑（免费且可完成）
			shouldDownloadByFree := isFree && canFinished
			shouldDownload := shouldDownloadByFilter || shouldDownloadByFree

			if isFree {
				stats.free.Add(1)
			}

			if !shouldDownload {
				torrent.IsSkipped = true
				sLogger().Infof("种子: %s 不满足下载条件，跳过 (原因: %s)", title, buildSkipReason(isFree, canFinished, shouldDownloadByFilter))
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
	parser := gofeed.NewParser()
	feed, err := parser.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("解析 RSS 失败: %v", err)
	}
	return feed, nil
}

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
			// 如果种子已跳过或已推送，直接跳过
			if torrent != nil && (torrent.IsSkipped || torrent.IsPushed != nil) {
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
			canFinished := detail.CanbeFinished(global.GetSlogger(), gl.DownloadLimitEnabled, gl.DownloadSpeedLimit, gl.TorrentSizeGB)
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

			// 检查过滤规则匹配
			var matchedRule *models.FilterRule
			var shouldDownloadByFilter bool
			downloadSource := "free_download"

			if filterSvc != nil && rssCfg.ID != 0 {
				// 构建匹配输入，包含标题和标签
				matchInput := filter.MatchInput{
					Title: title,
					Tag:   detailTag,
				}

				// 优先使用 RSS 关联的过滤规则（多对多关联）
				if hasAssociatedRules {
					shouldDownloadByFilter, matchedRule = filterSvc.ShouldDownloadForRSSWithInput(matchInput, isFree, rssCfg.ID)
					if matchedRule != nil {
						downloadSource = "filter_rule"
						sLogger().Infof("种子 %s (tag: %s) 匹配 RSS 关联过滤规则: %s (require_free=%v)", title, detailTag, matchedRule.Name, matchedRule.RequireFree)
					}
				} else {
					// RSS 没有关联规则时，不进行过滤规则匹配
					shouldDownloadByFilter = false
					matchedRule = nil
				}
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

			// 决定是否下载：
			// 1. 通过过滤规则匹配且满足条件
			// 2. 或者通过免费下载逻辑（免费且可完成）
			shouldDownloadByFree := isFree && canFinished
			shouldDownload := shouldDownloadByFilter || shouldDownloadByFree

			if !shouldDownload {
				torrent.IsSkipped = true
				sLogger().Infof("种子: %s 不满足下载条件，跳过 (原因: %s)", title, buildSkipReason(isFree, canFinished, shouldDownloadByFilter))
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
