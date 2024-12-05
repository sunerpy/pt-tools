package internal

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/qbit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	cfg       = global.GlobalCfg
	trueFlag  = true
	falseFlag = false
)

func attemptDownload(url, title, downloadDir string) (string, error) {
	// 尝试下载
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("下载种子失败: %v", err)
	}
	defer resp.Body.Close()
	// 检查 HTTP 状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP 状态码错误: %d", resp.StatusCode)
	}
	// 创建下载目录
	if err := os.MkdirAll(downloadDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("创建下载目录失败: %v", err)
	}
	// 生成文件路径
	fileName := fmt.Sprintf("%s/%s.torrent", downloadDir, sanitizeTitle(title))
	file, err := os.Create(fileName)
	if err != nil {
		return "", fmt.Errorf("创建种子文件失败: %v", err)
	}
	defer file.Close()
	// 创建一个多写入器，同时写入文件和内存缓冲区
	var buffer bytes.Buffer
	multiWriter := io.MultiWriter(file, &buffer)
	// 下载并保存种子文件
	_, err = io.Copy(multiWriter, resp.Body)
	if err != nil {
		return "", fmt.Errorf("写入种子文件失败: %v", err)
	}
	// 计算种子的 torrentHash
	torrentHash, err := qbit.ComputeTorrentHash(buffer.Bytes())
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

func downloadWorker[T models.ResType](
	ctx context.Context,
	siteName models.SiteGroup,
	wg *sync.WaitGroup,
	site PTSiteInter[T],
	torrentChan <-chan *gofeed.Item,
	downloadSubPath string,
) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			sLogger().Warn("下载任务取消")
			return
		case item, ok := <-torrentChan:
			if !ok {
				return
			}
			torrentURL := item.Enclosures[0].URL
			title := item.Title
			// 查询数据库记录
			torrent, err := global.GlobalDB.GetTorrentBySiteAndID(string(siteName), item.GUID)
			if err != nil {
				sLogger().Errorf("%s: 获取种子详情失败, %v", title, err)
				continue
			}
			// 如果种子已跳过或已推送，直接跳过
			if torrent != nil && (torrent.IsSkipped || torrent.IsPushed != nil) {
				sLogger().Infof("%s: 种子已跳过或已推送，直接跳过", title)
				continue
			}
			// 获取种子详情
			resDetail, err := site.GetTorrentDetails(item)
			if err != nil {
				sLogger().Errorf("%s: 获取种子详情失败, %v", title, err)
				continue
			}
			detail := resDetail.Data
			canFinished := detail.CanbeFinished(global.GetSlogger(), global.GetGlobalConfig().Global.DownloadLimitEnabled, global.GetGlobalConfig().Global.DownloadSpeedLimit, global.GetGlobalConfig().Global.TorrentSizeGB)
			isFree := detail.IsFree()
			// 更新种子状态（标记跳过或继续下载）
			if torrent == nil {
				torrent = &models.TorrentInfo{
					SiteName:    string(siteName),
					TorrentID:   item.GUID,
					FreeLevel:   detail.GetFreeLevel(),
					FreeEndTime: detail.GetFreeEndTime(),
				}
			}
			err = global.GlobalDB.WithTransaction(func(tx *gorm.DB) error {
				// 标记跳过或更新状态
				if !isFree || !canFinished {
					sLogger().Infof("种子: %s, ID: %s, free: %v, canbefinish: %v 为收费或无法完成，跳过", title, item.GUID, isFree, canFinished)
					torrent.IsSkipped = true
				} else {
					torrent.IsSkipped = false
				}
				// 使用 GORM 的 upsert 功能
				err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "site_name"}, {Name: "torrent_id"}},
					DoUpdates: clause.AssignmentColumns([]string{"is_skipped", "free_level", "free_end_time"}),
				}).Create(torrent).Error
				return err
			})
			if err != nil {
				sLogger().Errorf("更新种子:%s 状态失败, %v", title, err)
				continue
			}
			// 如果标记为跳过，直接跳过
			if torrent.IsSkipped {
				sLogger().Infof("种子: %s 为收费或无法完成，跳过", title)
				continue
			}
			// 下载种子并更新哈希值
			err = global.GlobalDB.WithTransaction(func(tx *gorm.DB) error {
				downloadPath := filepath.Join(global.GlobalDirCfg.DownloadDir, downloadSubPath)
				hash, err := site.DownloadTorrent(torrentURL, title, downloadPath)
				if err != nil {
					return fmt.Errorf("种子下载失败: %w", err)
				}
				// 更新数据库记录
				torrent.IsDownloaded = true
				torrent.TorrentHash = &hash
				// 更新指定字段
				err = tx.Model(&models.TorrentInfo{}).
					Where("site_name = ? AND torrent_id = ?", torrent.SiteName, torrent.TorrentID).
					Updates(map[string]interface{}{
						"torrent_hash":  torrent.TorrentHash,
						"is_downloaded": torrent.IsDownloaded,
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

func ProcessTorrentsWithDBUpdate(
	ctx context.Context,
	qbitClient *qbit.QbitClient,
	dirPath, category, tags string,
	siteName models.SiteGroup,
) error {
	// 使用事务处理目录和更新数据库
	return global.GlobalDB.WithTransaction(func(tx *gorm.DB) error {
		// 获取目录下的所有种子文件
		filePaths, err := qbit.GetTorrentFilesPath(dirPath)
		if err != nil {
			sLogger().Error("无法读取目录", dirPath)
			return fmt.Errorf("无法读取目录: %v", err)
		}
		for _, file := range filePaths {
			// 计算种子哈希
			torrentHash, err := qbit.ComputeTorrentHashWithPath(file)
			if err != nil {
				return fmt.Errorf("计算种子哈希失败: %w", err)
			}
			// 查询数据库中的种子信息
			torrent, err := global.GlobalDB.GetTorrentBySiteAndHash(string(siteName), torrentHash)
			if err != nil {
				return fmt.Errorf("查询种子信息失败: %w", err)
			}
			// 如果种子已推送，跳过并删除本地文件
			if torrent != nil && (torrent.IsPushed != nil && *torrent.IsPushed) {
				if err := os.Remove(file); err != nil {
					sLogger().Error("种子已推送，删除本地文件失败", file, err)
				} else {
					sLogger().Info("种子已推送,本地文件删除成功", file)
				}
				continue
			}
			// 检查种子是否已存在于 qBittorrent 中
			exists, err := qbitClient.CheckTorrentExists(torrentHash)
			if err != nil {
				return fmt.Errorf("检查种子失败: %w", err)
			}
			if exists {
				// 更新数据库为已推送
				err := tx.Model(&models.TorrentInfo{}).
					Where("site_name = ? AND torrent_hash = ?", siteName, torrentHash).
					Updates(map[string]interface{}{
						"is_pushed": true,
						"push_time": time.Now(),
					}).Error
				if err != nil {
					return fmt.Errorf("更新数据库记录失败: %w", err)
				}
				// 删除本地文件
				if err := os.Remove(file); err != nil {
					return fmt.Errorf("种子已存在，但删除本地文件失败: %w", err)
				}
				sLogger().Info("种子已存在于 qBittorrent,本地文件删除成功", file)
				continue
			}
			// 推送种子到 qBittorrent
			err = qbitClient.ProcessSingleTorrentFile(ctx, file, category, tags)
			if err != nil {
				// 更新数据库为失败状态
				err := tx.Model(&models.TorrentInfo{}).
					Where("site_name = ? AND torrent_hash = ?", siteName, torrentHash).
					Update("is_pushed", false).Error
				if err != nil {
					sLogger().Error("更新数据库失败", torrentHash, err)
				}
				return fmt.Errorf("处理种子文件失败: %w", err)
			}
			// 更新数据库为成功状态
			err = tx.Model(&models.TorrentInfo{}).
				Where("site_name = ? AND torrent_hash = ?", siteName, torrentHash).
				Updates(map[string]interface{}{
					"is_pushed": true,
					"push_time": time.Now(),
				}).Error
			if err != nil {
				return fmt.Errorf("更新数据库记录失败 (torrent_hash: %s): %w", torrentHash, err)
			}
		}
		return nil
	})
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

func FetchAndDownloadFreeRSS[T models.ResType](ctx context.Context, siteName models.SiteGroup, m PTSiteInter[T], rssCfg config.RSSConfig) error {
	if !m.IsEnabled() {
		return fmt.Errorf(enableError)
	}
	if rssCfg.DownloadSubPath == "" {
		return fmt.Errorf("下载目录为空")
	}
	feed, err := fetchRSSFeed(rssCfg.URL)
	if err != nil {
		return err
	}
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	var wg sync.WaitGroup
	torrentChan := make(chan *gofeed.Item, len(feed.Items))
	// 启动多个下载 Worker
	for i := 0; i < maxGoroutine; i++ {
		wg.Add(1)
		go downloadWorker(
			ctxWithTimeout,
			siteName,
			&wg,
			m,
			torrentChan,
			rssCfg.DownloadSubPath,
		)
	}
	// 将种子发送到下载队列
	for _, item := range feed.Items {
		if item.Enclosures != nil && len(item.Enclosures) > 0 {
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

func fetchRSSFeed(url string) (*gofeed.Feed, error) {
	parser := gofeed.NewParser()
	feed, err := parser.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("解析 RSS 失败: %v", err)
	}
	return feed, nil
}
