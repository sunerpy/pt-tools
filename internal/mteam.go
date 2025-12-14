package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mmcdole/gofeed"
	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/qbit"
	"github.com/sunerpy/pt-tools/utils"
)

type MteamImpl struct {
	ctx        context.Context
	maxRetries int
	retryDelay time.Duration
	qbitClient *qbit.QbitClient
}

func NewMteamImpl(ctx context.Context) (*MteamImpl, error) {
	qbc, _ := core.NewConfigStore(global.GlobalDB).GetQbitOnly()
	client, err := qbit.NewQbitClient(qbc.URL, qbc.User, qbc.Password, time.Second*10)
	if err != nil {
		sLogger().Error("MTEAM-qbit认证失败", err)
		return nil, fmt.Errorf("MTEAM-qbit认证失败: %w", err)
	}
	return &MteamImpl{
		ctx:        ctx,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		qbitClient: client,
	}, nil
}

func (m *MteamImpl) Context() context.Context {
	return m.ctx
}

func (m *MteamImpl) IsEnabled() bool {
	scMap, _ := core.NewConfigStore(global.GlobalDB).ListSites()
	if scMap[models.MTEAM].Enabled != nil {
		return *scMap[models.MTEAM].Enabled
	}
	return false
}

func (m *MteamImpl) DownloadTorrent(url, title, downloadDir string) (string, error) {
	return downloadTorrent(url, title, downloadDir, m.maxRetries, m.retryDelay)
}

func (m *MteamImpl) CanbeFinished(detail models.MTTorrentDetail) bool {
	gl, _ := core.NewConfigStore(global.GlobalDB).GetGlobalOnly()
	if !gl.DownloadLimitEnabled {
		return true
	} else {
		timeEnd, err := time.Parse("2006-01-02 15:04:05", detail.Status.DiscountEndTime)
		if err != nil {
			sLogger().Error("解析时间失败", err)
			return false
		}
		torrentSizeMB, err := strconv.Atoi(detail.Size)
		if err != nil {
			sLogger().Error("解析种子大小失败", err)
			return false
		}
		duration := time.Until(timeEnd)
		secondsDiff := int(duration.Seconds())
		if secondsDiff*gl.DownloadSpeedLimit < (torrentSizeMB / 1024 / 1024) {
			sLogger().Warn("种子免费时间不足以完成下载,跳过...", detail.ID)
			return false
		}
		return true
	}
}

func (m *MteamImpl) GetTorrentDetails(item *gofeed.Item) (*models.APIResponse[models.MTTorrentDetail], error) {
	if !m.IsEnabled() {
		return nil, errors.New(enableError)
	}
	if item == nil {
		return nil, fmt.Errorf("RSS item 为空")
	}
	data := fmt.Appendf(nil, "id=%s", item.GUID)
	cfg, _ := core.NewConfigStore(global.GlobalDB).Load()
	sc := cfg.Sites[models.MTEAM]
	if sc.APIUrl == "" || sc.APIKey == "" {
		return nil, fmt.Errorf("站点 API 未配置")
	}
	apiPath := fmt.Sprintf("%s%s", sc.APIUrl, torrentDetailPath)
	req, err := http.NewRequest("POST", apiPath, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %v", err)
	}
	req.Header.Set("x-api-key", sc.APIKey)
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

func (m *MteamImpl) SendTorrentToQbit(ctx context.Context, rssCfg models.RSSConfig) error {
	homeDir, _ := os.UserHomeDir()
	store := core.NewConfigStore(global.GlobalDB)
	gl, _ := store.GetGlobalOnly()
	base, berr := utils.ResolveDownloadBase(homeDir, models.WorkDir, gl.DownloadDir)
	if berr != nil {
		return berr
	}
	sub := utils.SubPathFromTag(rssCfg.Tag)
	dirPath := filepath.Join(base, sub)
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		_ = os.MkdirAll(dirPath, 0o755)
	}
	exists, empty, err := utils.CheckDirectory(dirPath)
	if err != nil {
		sLogger().Error("检查目录失败", err)
		return err
	}
	if !exists {
		sLogger().Info("下载目录不存在(未下载种子,跳过)", dirPath)
		return nil
	}
	if empty {
		sLogger().Info("下载目录为空(未下载种子,跳过)", dirPath)
		return nil
	}
	err = ProcessTorrentsWithDBUpdate(ctx, m.qbitClient, dirPath, rssCfg.Category, rssCfg.Tag, models.MTEAM)
	if err != nil {
		sLogger().Error("发送种子到 qBittorrent 失败", err)
		return err
	}
	sLogger().Info("种子处理完成并更新数据库记录")
	return nil
}
