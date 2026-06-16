// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"strings"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// gibiByte 是 1 GiB 的字节数，用于 SeedingCapacityGB 与字节数之间换算。
const gibiByte = 1024 * 1024 * 1024

// getSiteSeedingSizeBytes 聚合指定站点在其绑定下载器中当前做种/已推送种子的总体积（bytes）。
//
// 数据源：下载器实时 GetAllTorrents()，按 Category/Tags 命中 siteName 后对 TotalSize 求和
// （Downloader 接口不下推 tag/category 过滤，只能在 Go 侧本地匹配）。删种后下次拉取自动收敛。
// 失败语义：下载器不可达时返回 error，调用方在容量闸门处 fail-closed（拒绝推送）。
func getSiteSeedingSizeBytes(ctx context.Context, siteName string, dl downloader.Downloader) (int64, error) {
	if dl == nil {
		return 0, nil
	}
	torrents, err := dl.GetAllTorrents()
	if err != nil {
		return 0, err
	}
	return sumSiteSeedingSize(siteName, torrents), nil
}

// sumSiteSeedingSize 在 Go 侧按 Category/Tags 命中 siteName 求和 TotalSize（纯函数，便于测试）。
func sumSiteSeedingSize(siteName string, torrents []downloader.Torrent) int64 {
	if siteName == "" {
		return 0
	}
	var total int64
	for _, t := range torrents {
		if torrentBelongsToSite(siteName, t) {
			total += t.TotalSize
		}
	}
	return total
}

// torrentBelongsToSite 判断种子是否归属指定站点：Category 等于 siteName，或 Tags 中含 siteName。
// Tags 为逗号分隔的字符串（qBit tags / TR labels joined），逐项 trim 后大小写不敏感比较。
func torrentBelongsToSite(siteName string, t downloader.Torrent) bool {
	if strings.EqualFold(strings.TrimSpace(t.Category), siteName) {
		return true
	}
	for _, tag := range strings.Split(t.Tags, ",") {
		if strings.EqualFold(strings.TrimSpace(tag), siteName) {
			return true
		}
	}
	return false
}

// siteSeedingCapacityGB 读取指定站点的 SeedingCapacityGB 配置；未配置/查不到/DB 不可用时返回 0（不限制）。
func siteSeedingCapacityGB(siteName string) float64 {
	if siteName == "" || global.GlobalDB == nil {
		return 0
	}
	var site models.SiteSetting
	if err := global.GlobalDB.DB.Where("name = ?", siteName).First(&site).Error; err != nil {
		return 0
	}
	return site.SeedingCapacityGB
}
