package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/pt-tools/config"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/qbit"
	"go.uber.org/zap"
)

type MteamImpl struct {
	ctx        context.Context
	maxRetries int
	retryDelay time.Duration
	qbitClient *qbit.QbitClient
}

func NewMteamImpl(ctx context.Context) *MteamImpl {
	client, err := qbit.NewQbitClient(global.GetGlobalConfig().Qbit.URL, global.GetGlobalConfig().Qbit.User, global.GetGlobalConfig().Qbit.Password, time.Second*10)
	if err != nil {
		global.GlobalLogger.Fatal("认证失败", zap.Error(err))
	}
	return &MteamImpl{
		ctx:        ctx,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		qbitClient: client,
	}
}

func (m *MteamImpl) Context() context.Context {
	return m.ctx
}

func (m *MteamImpl) IsEnabled() bool {
	if global.GetGlobalConfig().Sites[models.MTEAM].Enabled != nil {
		return *global.GetGlobalConfig().Sites[models.MTEAM].Enabled
	}
	return false
}

func (m *MteamImpl) DownloadTorrent(url, title, downloadDir string) (string, error) {
	return downloadTorrent(url, title, downloadDir, m.maxRetries, m.retryDelay)
}

func (m *MteamImpl) CanbeFinished(detail models.MTTorrentDetail) bool {
	if !global.GetGlobalConfig().Global.DownloadLimitEnabled {
		return true
	} else {
		timeEnd, err := time.Parse("2006-01-02 15:04:05", detail.Status.DiscountEndTime)
		if err != nil {
			global.GlobalLogger.Error("解析时间失败", zap.Error(err))
			return false
		}
		torrentSizeMB, err := strconv.Atoi(detail.Size)
		if err != nil {
			global.GlobalLogger.Error("解析种子大小失败", zap.Error(err))
			return false
		}
		duration := timeEnd.Sub(time.Now())
		secondsDiff := int(duration.Seconds())
		if secondsDiff*global.GetGlobalConfig().Global.DownloadSpeedLimit < (torrentSizeMB / 1024 / 1024) {
			global.GlobalLogger.Warn("种子免费时间不足以完成下载,跳过...", zap.String("torrent_id", detail.ID))
			return false
		}
		return true
	}
}

func (m *MteamImpl) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.MTTorrentDetail], error) {
	if !m.IsEnabled() {
		return nil, fmt.Errorf(enableError)
	}
	data := []byte(fmt.Sprintf("id=%s", item.GUID))
	apiPath := fmt.Sprintf("%s%s", global.GetGlobalConfig().Sites[models.MTEAM].APIUrl, torrentDetailPath)
	req, err := http.NewRequest("POST", apiPath, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("x-api-key", global.GetGlobalConfig().Sites[models.MTEAM].APIKey)
	req.Header.Set("Content-Type", mteamContentType)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %v", err)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应体失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求失败，状态码: %d", resp.StatusCode)
	}
	// var responseData models.TorrentDetail
	// 保存原始 JSON 响应字符串
	// var formattedJSON bytes.Buffer
	// if err := json.Indent(&formattedJSON, bodyBytes, "", "  "); err != nil {
	// 	return nil, fmt.Errorf("格式化 JSON 失败: %v", err)
	// }
	// 解析 JSON 到结构体
	var responseData *models.APIResponse[models.MTTorrentDetail]
	if err := json.Unmarshal(bodyBytes, &responseData); err != nil {
		return nil, fmt.Errorf("解析 JSON 失败: %v", err)
	}
	return responseData, nil
}

func (m *MteamImpl) IsFree(detail models.MTTorrentDetail) bool {
	return detail.IsFree()
}

func (m *MteamImpl) MaxRetries() int {
	return m.maxRetries
}

func (m *MteamImpl) RetryDelay() time.Duration {
	return m.retryDelay
}

func (m *MteamImpl) SendTorrentToQbit(ctx context.Context, rssCfg config.RSSConfig) error {
	dirPath := filepath.Join(global.GlobalDirCfg.DownloadDir, rssCfg.DownloadSubPath)
	err := ProcessTorrentsWithDBUpdate(ctx, m.qbitClient, dirPath, rssCfg.Category, rssCfg.Tag, models.MTEAM)
	if err != nil {
		global.GlobalLogger.Fatal("发送种子到 qBittorrent 失败", zap.Error(err))
		return err
	}
	global.GlobalLogger.Info("种子处理完成并更新数据库记录")
	return nil
}
