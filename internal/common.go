package internal

import (
	"bytes"
	"context"
	"errors"
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
			if isFree {
				err = global.GlobalDB.WithTransaction(func(tx *gorm.DB) error {
					downloadPath := filepath.Join(global.GlobalDirCfg.DownloadDir, downloadSubPath)
					hash, err := site.DownloadTorrent(torrentURL, title, downloadPath)
					if err != nil {
						return fmt.Errorf("种子下载失败: %w", err)
					}
					cleanTitle := sanitizeTitle(title)
					torrentFile := filepath.Join(downloadPath, cleanTitle+".torrent")
					if _, err := os.Stat(torrentFile); os.IsNotExist(err) {
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
	// 每个种子使用独立事务
	return global.GlobalDB.WithTransaction(func(tx *gorm.DB) error {
		// 计算种子哈希
		torrentHash, err := qbit.ComputeTorrentHashWithPath(filePath)
		if err != nil {
			sLogger().Errorf("计算种子哈希失败: %s, %v", filePath, err)
			return fmt.Errorf("计算种子哈希失败: %w", err)
		}
		// 查询数据库中的种子信息
		torrent, err := global.GlobalDB.GetTorrentBySiteAndHash(string(siteName), torrentHash)
		if err != nil {
			sLogger().Errorf("查询种子信息失败: %s, %v", filePath, err)
			return fmt.Errorf("查询种子信息失败: %w", err)
		}
		if torrent == nil {
			if err := os.Remove(filePath); err != nil {
				sLogger().Errorf("删除过期种子失败: %s, %v", filePath, err)
				return fmt.Errorf("删除过期种子失败: %w", err)
			}
			sLogger().Infof("数据库不存在记录,删除过期种子成功: %s", filePath)
			return nil
		}
		// 0. 处理过期种子
		if torrent.GetExpired() {
			sLogger().Warnf("种子免费期已过期，标记并删除: %s", filePath)
			// 更新数据库标记过期
			if err := tx.Model(&models.TorrentInfo{}).
				Where("site_name = ? AND torrent_hash = ?", siteName, torrentHash).
				Update("is_expired", true).Error; err != nil {
				return fmt.Errorf("标记过期状态失败: %w", err)
			}
			sLogger().Warnf("种子已过免费期，删除本地文件: %s", filePath)
			if err := os.Remove(filePath); err != nil {
				sLogger().Errorf("删除过期种子失败: %s, %v", filePath, err)
			}
			// 直接返回不进行后续处理
			return nil
		}
		// 2. 已推送种子的处理
		if torrent.IsPushed != nil && *torrent.IsPushed {
			sLogger().Infof("种子已推送，删除本地文件: %s", filePath)
			if err := os.Remove(filePath); err != nil {
				return fmt.Errorf("删除已推送种子失败: %w", err)
			}
			return nil
		}
		// 3. 检查是否已在 qBittorrent 中存在
		exists, err := qbitClient.CheckTorrentExists(torrentHash)
		if err != nil {
			return fmt.Errorf("检查种子存在失败: %w", err)
		}
		if exists {
			sLogger().Infof("种子已存在于 qBittorrent: %s", filePath)
			// 更新数据库状态
			if err := tx.Model(&models.TorrentInfo{}).
				Where("site_name = ? AND torrent_hash = ?", siteName, torrentHash).
				Updates(map[string]interface{}{
					"is_pushed": true,
					"push_time": time.Now(),
				}).Error; err != nil {
				return fmt.Errorf("更新数据库状态失败: %w", err)
			}
			// 删除本地文件
			if err := os.Remove(filePath); err != nil {
				return fmt.Errorf("删除已存在种子失败: %w", err)
			}
			return nil
		}
		// 4. 新种子推送处理
		sLogger().Infof("推送新种子到 qBittorrent: %s\n", filePath)
		if err := qbitClient.ProcessSingleTorrentFile(ctx, filePath, category, tags); err != nil {
			// 推送失败只更新状态不删除文件
			if updateErr := tx.Model(&models.TorrentInfo{}).
				Where("site_name = ? AND torrent_hash = ?", siteName, torrentHash).
				Update("is_pushed", false).Error; updateErr != nil {
				sLogger().Errorf("更新推送失败状态出错: %s, %v", filePath, updateErr)
			}
			return fmt.Errorf("推送种子失败: %w", err)
		}
		// 5. 推送成功处理
		if err := tx.Model(&models.TorrentInfo{}).
			Where("site_name = ? AND torrent_hash = ?", siteName, torrentHash).
			Updates(map[string]interface{}{
				"is_pushed": true,
				"push_time": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("更新推送状态失败: %w", err)
		}
		// 推送成功后删除本地文件
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("删除已推送种子失败: %w; torrentHash: %s", err, torrentHash)
		}
		sLogger().Infof("种子推送成功并删除: %s, torrentHash: %s", filePath, torrentHash)
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
		return errors.New(enableError)
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
