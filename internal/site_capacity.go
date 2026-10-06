// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// gibiByte 是 1 GiB 的字节数，用于 SeedingCapacityGB 与字节数之间换算。
const gibiByte = 1024 * 1024 * 1024

// ErrSiteCapacityExceeded 表示推送会让站点做种总量超过 SeedingCapacityGB，或读不到做种总量时 fail-closed 拒绝。
var ErrSiteCapacityExceeded = errors.New("站点做种容量已满")

// getSiteSeedingSizeBytes 聚合指定站点在其绑定下载器中当前做种/已推送种子的总体积（bytes）。
//
// 数据源：下载器实时 GetAllTorrents()。归属判断两条取其一：pt-tools 记录过这个站点、这个
// info hash 的种子（RSS 与手动推送都会记录，不依赖用户填的分类和标签）；或 Category/Tags
// 命中 siteName（覆盖 pt-tools 之外添加、但按站点打了标签的种子）。删种后下次拉取自动收敛。
// 失败语义：下载器不可达或查不了库时返回 error，调用方在容量闸门处 fail-closed（拒绝推送）。
func getSiteSeedingSizeBytes(ctx context.Context, siteName string, dl downloader.Downloader) (int64, error) {
	if dl == nil {
		return 0, nil
	}
	torrents, err := dl.GetAllTorrents()
	if err != nil {
		return 0, err
	}
	hashes, err := siteTorrentHashes(siteName)
	if err != nil {
		return 0, err
	}
	return sumSiteSeedingSizeWithHashes(siteName, torrents, hashes), nil
}

// siteTorrentHashes 返回 pt-tools 为该站点记录过的种子 info hash（小写）。
func siteTorrentHashes(siteName string) (map[string]struct{}, error) {
	if siteName == "" || global.GlobalDB == nil {
		return nil, nil
	}
	var hashes []string
	if err := global.GlobalDB.DB.Model(&models.TorrentInfo{}).
		Where("site_name = ? AND torrent_hash IS NOT NULL AND torrent_hash != ''", siteName).
		Pluck("torrent_hash", &hashes).Error; err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(hashes))
	for _, h := range hashes {
		set[strings.ToLower(h)] = struct{}{}
	}
	return set, nil
}

// sumSiteSeedingSize 在 Go 侧按 Category/Tags 命中 siteName 求和 TotalSize（纯函数，便于测试）。
func sumSiteSeedingSize(siteName string, torrents []downloader.Torrent) int64 {
	return sumSiteSeedingSizeWithHashes(siteName, torrents, nil)
}

// sumSiteSeedingSizeWithHashes 求和归属该站点的种子体积：info hash 在 hashes 里，或分类/标签命中站点名。
func sumSiteSeedingSizeWithHashes(siteName string, torrents []downloader.Torrent, hashes map[string]struct{}) int64 {
	if siteName == "" {
		return 0
	}
	var total int64
	for _, t := range torrents {
		_, recorded := hashes[strings.ToLower(t.InfoHash)]
		if (recorded && t.InfoHash != "") || torrentBelongsToSite(siteName, t) {
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

// checkSiteCapacity 判断推送这个种子后站点做种总量是否会超过 capGB；读不到做种总量时同样返回
// ErrSiteCapacityExceeded（fail-closed）。torrentSize 未知时从种子文件解析。
func checkSiteCapacity(ctx context.Context, siteName string, dl downloader.Downloader, capGB float64, torrentSize int64, torrentFile string) error {
	if torrentSize <= 0 && torrentFile != "" {
		if data, err := os.ReadFile(torrentFile); err == nil {
			if size, sizeErr := qbit.ComputeTorrentSize(data); sizeErr == nil {
				torrentSize = size
			}
		}
	}
	used, err := getSiteSeedingSizeBytes(ctx, siteName, dl)
	if err != nil {
		return fmt.Errorf("%w：无法读取站点做种总量，已拒绝推送: %v", ErrSiteCapacityExceeded, err)
	}
	capBytes := int64(capGB * gibiByte)
	if used+torrentSize > capBytes {
		return fmt.Errorf("%w (当前 %.1f GB + 种子 %.1f GB > %.1f GB)", ErrSiteCapacityExceeded,
			float64(used)/gibiByte, float64(torrentSize)/gibiByte, capGB)
	}
	return nil
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
