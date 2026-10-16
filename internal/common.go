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
	"github.com/sunerpy/pt-tools/internal/filter"
	"github.com/sunerpy/pt-tools/internal/maintenance"
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

// ProcessTorrentsWithDownloaderByRSS 根据 RSS 配置选择下载器，推送这条 RSS 暂存目录里属于它的种子。
// revalidate 用于推送前重新读详情（要求免费、承诺到期暂停而记录里的资格不是这一轮读到的时候）；为空时只能等下一轮取详情。
func ProcessTorrentsWithDownloaderByRSS(
	ctx context.Context,
	rssCfg models.RSSConfig,
	dirPath, category, tags string,
	siteName models.SiteGroup,
	revalidate DetailRevalidator,
) error {
	attemptStart, ok := RSSAttemptStart(ctx)
	if !ok {
		attemptStart = time.Now()
	}
	dl, dlInfo, err := GetDownloaderForRSSAndSiteWithInfo(rssCfg, string(siteName))
	if err != nil {
		return fmt.Errorf("获取下载器失败: %w", err)
	}
	if dlInfo.NeedClose {
		defer dl.Close()
	}

	sLogger().Infof("[种子推送开始] 站点=%s, RSS=%s, 下载器=%s(%s), 目录=%s", siteName, rssCfg.Name, dl.GetName(), dl.GetType(), dirPath)

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
		sLogger().Infof("[种子推送完成] 站点=%s, RSS=%s, 总数=0", siteName, rssCfg.Name)
		return nil
	}

	gl, _ := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	sp := &stagedPush{
		rss: rssCfg, dl: dl, dlInfo: dlInfo, category: category, tags: tags, downloadPath: downloadPath,
		siteName: siteName, attemptStart: attemptStart, revalidate: revalidate,
	}
	result := runPushLoop(ctx, sp, dirPath, filePaths, gl.RetainHours)

	sLogger().Infof("[种子推送完成] 站点=%s, RSS=%s, 总数=%d, 成功=%d, 失败=%d, 暂不推送=%d, 其他 RSS 的=%d, 跳过(过大)=%d, 磁盘满=%v",
		siteName, rssCfg.Name, len(filePaths), result.success, result.failed, result.held, result.others, result.skippedTooLarge, result.diskFull)
	return nil
}

// pushLoopResult 汇总一次推送循环的处理结果，供日志与测试断言使用。
type pushLoopResult struct {
	success         int
	failed          int
	held            int // 资格还不能确认（结束时间未知、没有本次读到的资格），文件留着
	others          int // 属于另一条 RSS 的文件，不动
	skippedTooLarge int
	diskFull        bool
	capacityFull    bool
}

// runPushLoop 遍历暂存目录中的 .torrent 逐个推送，并按磁盘保护语义分类处理：
// 磁盘整体不足则停止本轮（break），单个种子过大则跳过继续，其余错误计入 failed。
// defer 的暂存清理注册在循环前，确保正常结束/磁盘满 break/过大 continue
// 三种退出路径都执行暂存目录兜底清理（只清理本 RSS 的文件）。
func runPushLoop(ctx context.Context, sp *stagedPush, dirPath string, filePaths []string, retainHours int) pushLoopResult {
	defer sp.sweep(dirPath, retainHours)

	var result pushLoopResult
LOOP:
	for i, file := range filePaths {
		err := sp.processFile(ctx, file)
		switch {
		case err == nil:
			result.success++
		case errors.Is(err, errStagedHeld):
			result.held++
		case errors.Is(err, errStagedNotMine):
			result.others++
		case errors.Is(err, downloader.ErrInsufficientSpace):
			sLogger().Warnf("[磁盘保护] 磁盘空间已达保底线，停止本轮推送剩余 %d 个种子", len(filePaths)-i-1)
			result.diskFull = true
			break LOOP
		case errors.Is(err, downloader.ErrTorrentTooLarge):
			sLogger().Warnf("[磁盘保护] 种子过大跳过，继续处理后续: %s", file)
			result.skippedTooLarge++
			continue
		case errors.Is(err, ErrSiteCapacityExceeded):
			// 站点容量满了，同一站点后面的种子也推不进去；文件留在暂存目录，下一轮再试
			sLogger().Warnf("[站点容量] %s: 已达容量上限，停止本轮推送剩余 %d 个种子", sp.siteName, len(filePaths)-i-1)
			result.capacityFull = true
			break LOOP
		default:
			sLogger().Errorf("处理种子失败: %s, %v", file, err)
			result.failed++
		}
	}
	return result
}

// recordDiskProtectError 将磁盘保护拒绝原因写入 last_error，使状态在 Web/ChatOps
// 可见，且不累加 retry_count（拒绝非本种子之过，避免误达 MaxRetry 被删）。
func recordDiskProtectError(siteName models.SiteGroup, torrentHash, msg string) {
	if global.GlobalDB == nil {
		return
	}
	if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_hash = ?", string(siteName), torrentHash).
		Update("last_error", msg).Error; err != nil {
		sLogger().Errorf("记录磁盘保护状态失败: hash=%s, %v", torrentHash, err)
	}
}

// sweepStagingDir 按文件系统 mtime + DB 状态兜底清理暂存目录里已无意义的 .torrent
// （已推送/孤立/已达最大重试/超过保留期未推送），不按归属过滤（没有 RSS 上下文的旧调用点）。
// retainHours <= 0 时不启用。
func sweepStagingDir(dirPath string, siteName models.SiteGroup, retainHours int) {
	(&stagedPush{siteName: siteName}).sweep(dirPath, retainHours)
}

// shouldSweep 判定一个暂存 .torrent 是否应被清理（保守，避免误删还有用的）。
// 决策规则复用 internal/maintenance.ShouldSweepStaging（Issue #450 与特性 D 共享的
// 唯一 sweep 规则），避免两处逻辑分叉。MaxRetry 从全局配置读取后透传。
func shouldSweep(filePath string, siteName models.SiteGroup, retainHours int) bool {
	if global.GlobalDB == nil {
		return false
	}
	gl, _ := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	return maintenance.ShouldSweepStaging(global.GlobalDB, filePath, siteName, retainHours, gl.MaxRetry)
}

// filterTorrentFilesBySite 只留本站点的暂存文件（文件名以「站点-」开头）。没有匹配时返回空：
// 暂存目录按标签共用，别的站点的文件不归这个站点处理。
func filterTorrentFilesBySite(filePaths []string, siteName models.SiteGroup) []string {
	prefix := strings.ToLower(string(siteName)) + "-"
	filtered := make([]string, 0, len(filePaths))
	for _, file := range filePaths {
		if strings.HasPrefix(strings.ToLower(filepath.Base(file)), prefix) {
			filtered = append(filtered, file)
		}
	}
	return filtered
}

// processSingleTorrentWithDownloader 推送一个暂存文件（没有 RSS 编号：不按归属过滤，本次尝试从现在算）。
func processSingleTorrentWithDownloader(
	ctx context.Context,
	dl downloader.Downloader,
	dlInfo *DownloaderInfo,
	filePath, category, tags, downloadPath string,
	siteName models.SiteGroup,
	pauseOnFreeEnd bool,
) error {
	sp := &stagedPush{
		rss: models.RSSConfig{PauseOnFreeEnd: pauseOnFreeEnd}, dl: dl, dlInfo: dlInfo,
		category: category, tags: tags, downloadPath: downloadPath, siteName: siteName, attemptStart: time.Now(),
	}
	return sp.processFile(ctx, filePath)
}

func attemptDownload(url, title, downloadDir string) (string, error) {
	return attemptDownloadWithContext(context.Background(), url, title, downloadDir)
}

func attemptDownloadWithContext(ctx context.Context, url, title, downloadDir string) (string, error) {
	// 使用 requests 库下载，支持 context
	resp, err := requests.Get(url, requests.WithContext(ctx), requests.WithTimeout(30*time.Second))
	if err != nil {
		return "", fmt.Errorf("下载种子失败: %v", utils.RedactURLError(err))
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

	// 检查 RSS 是否关联了下载规则：只关联了通知规则时，下载照旧按没有规则处理（免费种子走免费通道）
	hasAssociatedRules := false
	if filterSvc != nil && rssCfg.ID != 0 {
		rules, err := filterSvc.GetRulesForRSS(rssCfg.ID)
		if err == nil && filter.HasRulesFor(rules, filter.PurposeDownload) {
			hasAssociatedRules = true
			sLogger().Infof("RSS %s 关联了 %d 个过滤规则", rssCfg.Name, len(rules))
		}
	}

	attemptStart, ok := RSSAttemptStart(ctx)
	if !ok {
		attemptStart = time.Now()
	}
	margin := SafetyMarginFor(gl.FreeEndAdvanceMinutes)

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
				torrentURL = item.Link
			}
			title := item.Title
			// 查询数据库记录
			torrent, err := global.GlobalDB.GetTorrentBySiteAndID(string(siteName), item.GUID)
			if err != nil {
				sLogger().Errorf("从数据库获取种子: %s 详情失败, %v", title, err)
				continue
			}
			if torrent != nil && isPushed(torrent) {
				// 已推送：读一次详情刷新资格，不再推送。免费缩短、延长或取消时，自动删种的「免费到期未完成删除」
				// 与到期暂停都按新的事实处理（受到期监控的把计划暂停改到新的时刻，监控器据此暂停）；
				// 这条 RSS 推的还按这一轮的判断刷新来源与命中的过滤规则
				requestedAt := time.Now()
				d, derr := fetchDetail(v2.WithFreshAfter(ctx, attemptStart), site, item)
				if derr != nil {
					sLogger().Warnf("%s: 刷新已推送种子的资格失败，保持原来的记录: %v", title, derr)
				} else {
					snap := detailSnapshot(siteName, item.GUID, d, requestedAt)
					// 来源与规则只跟着同一份资格一起写：这份资格没写成（读到的比库里旧、写库失败）就不写，不留新旧混合的记录
					if applied, rerr := refreshPushedRecord(torrent, snap, time.Now()); rerr != nil {
						sLogger().Warnf("%s: 写回已推送种子的资格失败: %v", title, rerr)
					} else if applied {
						dec := decideItem(title, d, snap, gl, rssCfg, filterSvc, hasAssociatedRules, margin)
						if rerr := refreshPushedSource(torrent, rssCfg, snap, dec.source, dec.ruleID); rerr != nil {
							sLogger().Warnf("%s: 写回已推送种子的来源失败: %v", title, rerr)
						}
					}
				}
				stats.skipped.Add(1)
				continue
			}
			// 已跳过的记录也每轮读详情、整套刷新并按这一轮重新判断（不再等 6 小时）
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
			// 获取种子详情：只认这一轮开始以后取得的数据（缓存详情的驱动据此先刷新）
			requestedAt := time.Now()
			detail, err := fetchDetail(v2.WithFreshAfter(ctx, attemptStart), site, item)
			if err != nil {
				sLogger().Errorf("[%s] %s: 获取种子详情失败, %v", siteName, title, err)
				stats.detailFailed.Add(1)
				continue
			}
			if detail.FetchedAt.IsZero() {
				// 驱动没给取得时间：按发起请求的时间算（订阅拿到的候选也带着它）。复制一份再改，不动驱动的结果
				stamped := *detail
				stamped.FetchedAt = requestedAt
				detail = &stamped
			}
			snap := detailSnapshot(siteName, item.GUID, detail, requestedAt)
			// 订阅也看 RSS 拉到的种子（不阻塞；对上了由订阅自己推送，和这里的过滤规则各管各的）
			offerToSubscriptions(siteID, detail, item)
			dec := decideItem(title, detail, snap, gl, rssCfg, filterSvc, hasAssociatedRules, margin)
			isFree, canFinished, detailTag, sizeGB := dec.isFree, dec.canFinished, dec.tag, dec.sizeGB
			if dec.freeShort {
				sLogger().Infof("种子: %s 免费剩余时间不足 (%.0f分钟 < %d分钟)，跳过",
					title, dec.freeLeft.Minutes(), gl.MinFreeMinutes)
			}

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

			decision, matchedRule, downloadSource, ruleID := dec.decision, dec.rule, dec.source, dec.ruleID
			if matchedRule != nil && decision.Source == filter.SourceFilterRule {
				sLogger().Infof("种子 %s (tag: %s) 匹配 RSS 关联过滤规则: %s (require_free=%v, min=%d, max=%d)", title, detailTag, matchedRule.Name, matchedRule.RequireFree, matchedRule.MinSizeGB, matchedRule.MaxSizeGB)
			}
			shouldDownload := decision.ShouldDownload
			requireFree := dec.requireFree

			if isFree {
				stats.free.Add(1)
			}
			if !shouldDownload {
				reason := decision.Reason
				if reason == "" {
					reason = buildSkipReason(isFree, canFinished, false)
				}
				sLogger().Infof("种子: %s 不满足下载条件，跳过 (原因: %s)", title, reason)
			}

			// 资格快照是站点事实，每轮整套写入（不用旧结构体回写旧值）；下载策略只由所有者 RSS 写
			cat := ""
			if len(item.Categories) > 0 {
				cat = strings.Join(item.Categories, "/")
			}
			_, detailRef := extractTorrentRef(item)
			now := time.Now()
			facts := SnapshotColumns(snap)
			facts["is_expired"] = promotionEnded(snap)
			facts["title"] = title
			facts["last_check_time"] = &now
			if detailRef != "" {
				facts["detail_id"] = detailRef
			}
			if detail.SizeBytes > 0 {
				facts["torrent_size"] = detail.SizeBytes
			}
			// 外部编号：详情里有就写上，没有时保留库里已有的值
			if detail.IMDbID != "" {
				facts["imdb_id"] = detail.IMDbID
			}
			if detail.DoubanID != "" {
				facts["douban_id"] = detail.DoubanID
			}
			policy := map[string]any{
				"is_skipped":      !shouldDownload,
				"download_source": downloadSource,
				"filter_rule_id":  ruleID,
				"require_free":    requireFree,
				"tag":             rssCfg.Tag,
			}

			stage := false
			if torrent == nil {
				fresh := &models.TorrentInfo{
					SiteName:       string(siteName),
					TorrentID:      item.GUID,
					Title:          title,
					Category:       cat,
					Tag:            rssCfg.Tag,
					LastCheckTime:  &now,
					DownloadSource: downloadSource,
					FilterRuleID:   ruleID,
					IsSkipped:      !shouldDownload,
					RequireFree:    requireFree,
					DetailID:       detailRef,
					TorrentSize:    detail.SizeBytes,
					HasHR:          detail.HasHR || siteHR,
					HRSeedTimeH:    calcHRSeedTimeForTorrent(siteDef, siteHRSeedTimeH, detail.SizeBytes),
					IMDbID:         detail.IMDbID,
					DoubanID:       detail.DoubanID,
				}
				applySnapshotToRecord(fresh, snap)
				if shouldDownload && rssCfg.ID != 0 {
					id := rssCfg.ID
					fresh.SourceRSSID = &id
				}
				created, createErr := createTorrentRecord(fresh)
				if createErr != nil {
					sLogger().Errorf("保存种子:%s 记录失败, %v", title, createErr)
					continue
				}
				if created {
					torrent, stage = fresh, shouldDownload
				} else {
					// 另一条 RSS 同时建了这条记录：按已有记录处理
					torrent, err = global.GlobalDB.GetTorrentBySiteAndID(string(siteName), item.GUID)
					if err != nil || torrent == nil {
						sLogger().Errorf("读取种子:%s 记录失败, %v", title, err)
						continue
					}
				}
			}
			if !stage {
				stage, err = recordFetchDecision(torrent, rssCfg, facts, policy, shouldDownload)
				if err != nil {
					sLogger().Errorf("更新种子:%s 状态失败, %v", title, err)
					continue
				}
				if shouldDownload && !stage {
					sLogger().Infof("种子: %s 已由另一条 RSS 暂存，这条 RSS 不重复处理", title)
					stats.skipped.Add(1)
					continue
				}
			}
			if !stage {
				continue
			}
			// 下载种子并更新哈希值
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
				// 修正数据库状态（只改这两列，不回写整条旧记录）
				if fixErr := global.GlobalDB.DB.Model(&models.TorrentInfo{}).Where("id = ?", torrent.ID).
					Updates(map[string]any{"is_downloaded": false, "torrent_hash": nil}).Error; fixErr != nil {
					sLogger().Errorf("%s: 修正下载状态失败, %v", title, fixErr)
				}
				continue
			}
			// 更新指定字段
			doneAt := time.Now()
			err = global.GlobalDB.DB.Model(&models.TorrentInfo{}).
				Where("id = ?", torrent.ID).
				Updates(map[string]any{
					"torrent_hash":    &hash,
					"is_downloaded":   true,
					"last_check_time": &doneAt,
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

// itemDecision 是 RSS 按这一轮的详情对一个条目做的下载决定（见 decideItem）。
type itemDecision struct {
	isFree, canFinished bool
	// freeShort：免费剩余时间不足 MinFreeMinutes（freeLeft 是剩余时间），这一轮不下载
	freeShort   bool
	freeLeft    time.Duration
	tag         string
	sizeGB      float64
	decision    filter.Decision
	rule        *models.FilterRule
	ruleID      *uint
	source      string
	requireFree bool
}

// decideItem 按这一轮的详情做下载决定：全局大小硬限（上下限）→ 过滤规则通道 → 免费通道（filter.Decide）。
// 决定下载时的「免费」：免费到某时（剩余超过安全余量）、长期免费，或免费但结束时间未知
// （未知的照样暂存，推送前要求免费的会被拦下，等下一轮读到结束时间）。
func decideItem(title string, detail *v2.TorrentItem, snap v2.PromotionSnapshot, gl models.SettingsGlobal, rssCfg models.RSSConfig,
	filterSvc filter.FilterService, hasRules bool, margin time.Duration,
) itemDecision {
	d := itemDecision{tag: detail.GetSubTitle(), sizeGB: float64(detail.SizeBytes) / 1024 / 1024 / 1024}
	// 此处传 0 给 sizeLimitGB：全局大小硬上限由 filter.Decide 统一检查，避免过滤规则通道绕过全局限制
	d.canFinished = detail.CanbeFinished(gl.DownloadLimitEnabled, gl.DownloadSpeedLimit, 0)
	d.isFree = freeForDecision(snap, time.Now(), margin)
	if d.isFree && d.canFinished && snap.EndsAt != nil && gl.MinFreeMinutes > 0 {
		if left := time.Until(*snap.EndsAt); left < time.Duration(gl.MinFreeMinutes)*time.Minute {
			d.canFinished, d.freeShort, d.freeLeft = false, true, left
		}
	}
	dc := filter.DecisionContext{
		Input:         filter.MatchInput{Title: title, Tag: d.tag, SizeGB: d.sizeGB},
		IsFree:        d.isFree,
		CanFinish:     d.canFinished,
		GlobalSize:    gl.TorrentSizeGB,
		GlobalMinSize: gl.TorrentMinSizeGB,
		FilterMode:    rssCfg.GetEffectiveFilterMode(&gl),
	}
	if filterSvc != nil && rssCfg.ID != 0 && hasRules {
		d.decision = filterSvc.Decide(dc, rssCfg.ID)
	} else {
		d.decision = filter.DecideWithoutRules(dc)
	}
	d.rule = d.decision.MatchedRule
	if d.rule != nil {
		d.ruleID = &d.rule.ID
	}
	d.source = d.decision.Source
	if d.source == "" {
		d.source = filter.SourceFreeDownload
	}
	// 免费通道与要求免费的规则：推送前必须持有本次读取、仍在免费期内的资格
	d.requireFree = d.source == filter.SourceFreeDownload || (d.rule != nil && d.rule.RequireFree)
	return d
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

// freeEndForRecord 是写进种子记录的免费到期时间：只有免费（FREE、2XFREE）的结束时间才算。半价、2X 这类优惠也有结束时间，
// 记下来的话，免费结束暂停与「删除免费到期未完成的种子」会在优惠结束时处理一个本来就不要求免费的种子。没有结束时间时也不记。
func freeEndForRecord(isFree bool, end *time.Time) *time.Time {
	if !isFree || end == nil || end.IsZero() {
		return nil
	}
	return end
}

// FetchAndDownloadFreeRSSUnified 使用 UnifiedPTSite 接口获取并下载免费 RSS 种子
func FetchAndDownloadFreeRSSUnified(ctx context.Context, m UnifiedPTSite, rssCfg models.RSSConfig) error {
	startTime := time.Now()
	if _, ok := RSSAttemptStart(ctx); !ok {
		ctx = WithRSSAttempt(ctx, startTime)
	}
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

	var discarded int
	for _, item := range feed.Items {
		if len(item.Enclosures) == 0 && strings.TrimSpace(item.Link) == "" {
			discarded++
			continue
		}
		select {
		case <-ctxWithTimeout.Done():
			sLogger().Info("任务被取消")
			close(torrentChan)
			wg.Wait()
			return ctxWithTimeout.Err()
		case torrentChan <- item:
		}
	}
	if discarded > 0 {
		sLogger().Warnf("[RSS] 站点=%s, RSS=%s, 丢弃 %d 个无下载链接条目", siteName, rssCfg.Name, discarded)
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
		return nil, fmt.Errorf("解析 RSS 失败: %w", utils.RedactURLError(err))
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
	var discarded int
	for _, item := range feed.Items {
		if len(item.Enclosures) == 0 && strings.TrimSpace(item.Link) == "" {
			discarded++
			continue
		}
		select {
		case <-ctxWithTimeout.Done():
			sLogger().Info("任务被取消")
			close(torrentChan)
			wg.Wait()
			return ctxWithTimeout.Err()
		case torrentChan <- item:
		}
	}
	if discarded > 0 {
		sLogger().Warnf("[RSS] 站点=%s, RSS=%s, 丢弃 %d 个无下载链接条目", siteName, rssCfg.Name, discarded)
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

	// 检查 RSS 是否关联了下载规则：只关联了通知规则时，下载照旧按没有规则处理（免费种子走免费通道）
	hasAssociatedRules := false
	if filterSvc != nil && rssCfg.ID != 0 {
		rules, err := filterSvc.GetRulesForRSS(rssCfg.ID)
		if err == nil && filter.HasRulesFor(rules, filter.PurposeDownload) {
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
				torrentURL = item.Link
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
					Input:         filter.MatchInput{Title: title, Tag: detailTag, SizeGB: sizeGB},
					IsFree:        isFree,
					CanFinish:     canFinished,
					GlobalSize:    gl.TorrentSizeGB,
					GlobalMinSize: gl.TorrentMinSizeGB,
					FilterMode:    rssCfg.GetEffectiveFilterMode(&gl),
				}, rssCfg.ID)
			} else {
				decision = filter.DecideWithoutRules(filter.DecisionContext{
					Input:         filter.MatchInput{Title: title, Tag: detailTag, SizeGB: sizeGB},
					IsFree:        isFree,
					CanFinish:     canFinished,
					GlobalSize:    gl.TorrentSizeGB,
					GlobalMinSize: gl.TorrentMinSizeGB,
					FilterMode:    rssCfg.GetEffectiveFilterMode(&gl),
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
					FreeEndTime:    freeEndForRecord(isFree, detail.GetFreeEndTime()),
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
