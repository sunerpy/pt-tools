package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/scheduler"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// newOrganizeService 构造并启动整理入库服务：媒体服务器的 Token 用 ConfigStore 的 Cookie 密钥加解密，
// 下载器取 DownloaderManager 的共享实例，入库通知写进监控通知日志、由现有的投递器发出。
func newOrganizeService(store *core.ConfigStore, mgr *scheduler.Manager, rec *recognize.Service, notifier *scheduler.MonitorNotifier) *organize.Service {
	if global.GlobalDB == nil || global.GlobalDB.DB == nil || store == nil || mgr == nil || rec == nil {
		global.GetSlogger().Warn("整理入库跳过初始化：数据库或媒体识别服务未就绪")
		return nil
	}
	svc := organize.New(organize.Config{
		DB:          global.GlobalDB.DB,
		Cipher:      storeCipher{store: store},
		Recognizer:  rec,
		Downloaders: organizeDownloaders{mgr: mgr},
		Notify:      organizeNotifier(notifier),
		Logger:      global.GetSlogger(),
	})
	svc.Start()
	global.GetSlogger().Info("整理入库服务已启动")
	return svc
}

// organizeNotifier 把入库通知写进监控通知日志（source=media），不直接调用 Push、也不自建 Router。
func organizeNotifier(n *scheduler.MonitorNotifier) organize.Notifier {
	if !n.Enabled() {
		return nil
	}
	return func(ctx context.Context, no organize.Notice) error {
		// Subject 是种子的 hash（列宽 64），EventKey 是这一批整理（hash 加第一条记录的编号）
		subject, _, _ := strings.Cut(no.Key, ":")
		if len(subject) > 64 {
			subject = subject[:64]
		}
		_, err := n.Enqueue(ctx, scheduler.MonitorNotifyEntry{
			Source: "media", Subject: subject, Kind: "organized", EventKey: no.Key,
			Title: no.Title, Text: no.Text, ConfIDs: no.ChannelIDs,
		})
		return err
	}
}

// organizeDownloaders 按下载器设置取 DownloaderManager 的共享实例（用完不能 Close）。
type organizeDownloaders struct {
	mgr *scheduler.Manager
}

var errOrganizeNoDownloader = errors.New("下载器不存在")

func (d organizeDownloaders) instance(ctx context.Context, s models.DownloaderSetting) (downloader.Downloader, models.DownloaderSetting, error) {
	if !s.Enabled {
		return nil, s, fmt.Errorf("下载器「%s」没有启用", s.Name)
	}
	dl, err := d.mgr.GetDownloaderManager().GetDownloaderContext(ctx, s.Name)
	if err != nil {
		return nil, s, fmt.Errorf("连接下载器「%s」失败: %w", s.Name, err)
	}
	return dl, s, nil
}

func (d organizeDownloaders) Get(ctx context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error) {
	var s models.DownloaderSetting
	if err := global.GlobalDB.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&s).Error; err != nil {
		return nil, s, fmt.Errorf("读取下载器失败: %w", err)
	}
	if s.ID == 0 {
		return nil, s, errOrganizeNoDownloader
	}
	return d.instance(ctx, s)
}

func (d organizeDownloaders) ByName(ctx context.Context, name string) (downloader.Downloader, models.DownloaderSetting, error) {
	var s models.DownloaderSetting
	if err := global.GlobalDB.DB.WithContext(ctx).Where("name = ?", name).Limit(1).Find(&s).Error; err != nil {
		return nil, s, fmt.Errorf("读取下载器失败: %w", err)
	}
	if s.ID == 0 {
		return nil, s, errOrganizeNoDownloader
	}
	return d.instance(ctx, s)
}

func (d organizeDownloaders) List(ctx context.Context) ([]models.DownloaderSetting, error) {
	var out []models.DownloaderSetting
	if err := global.GlobalDB.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&out).Error; err != nil {
		return nil, fmt.Errorf("读取下载器失败: %w", err)
	}
	return out, nil
}
