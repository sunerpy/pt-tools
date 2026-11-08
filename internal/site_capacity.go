// MIT License
// Copyright (c) 2025 pt-tools

package internal

import (
	"context"
	"errors"
	"strings"

	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// gibiByte 是 1 GiB 的字节数，用于 SeedingCapacityGB 与字节数之间换算。
const gibiByte = 1024 * 1024 * 1024

// ErrSiteCapacityExceeded 表示推送会让站点做种总量超过 SeedingCapacityGB，或读不到做种总量时 fail-closed 拒绝。
var ErrSiteCapacityExceeded = errors.New("站点做种容量已满")

// getSiteSeedingSizeBytes 聚合指定站点在其绑定下载器中当前做种/已推送种子的总体积（bytes），见 siteSeedingUsage。
func getSiteSeedingSizeBytes(ctx context.Context, siteName string, dl downloader.Downloader) (int64, error) {
	u, err := siteSeedingUsage(ctx, siteName, dl, "")
	return u.used, err
}

// siteUsage 是站点在一台下载器里的做种总量。
type siteUsage struct {
	used int64
	// sharedSource 不为空：要推送的种子是 pt-tools 辅种的种子，和这台下载器里的这个种子（小写 info hash）共用数据，
	// 加进去不多占空间，也不计入站点的做种总量
	sharedSource string
}

// siteSeedingUsage 聚合 siteName 在 dl 里的做种总量（bytes），并看要推送的种子 pushHash（可为空）是不是和下载器里已有的种子共用数据。
//
// 数据源：下载器实时 GetAllTorrents()。归属判断三条取其一：pt-tools 记录过这个站点、这个
// info hash 的种子（RSS 与手动推送都会记录，不依赖用户填的分类和标签）；Category/Tags
// 命中 siteName（覆盖 pt-tools 之外添加、但按站点打了标签的种子）；或主 tracker 属于这个站点
// （手动添加、什么标签都没打的种子）。按辅种记录认出共用同一份数据的种子只算一个（见 sharedDataSkips）：
// 辅种的种子在原种子还在下载器里时不计入。删种后下次拉取自动收敛。
// 失败语义：下载器不可达或查不了库时返回 error，调用方在容量闸门处 fail-closed（拒绝推送）。
func siteSeedingUsage(ctx context.Context, siteName string, dl downloader.Downloader, pushHash string) (siteUsage, error) {
	if dl == nil {
		return siteUsage{}, nil
	}
	torrents, err := dl.GetAllTorrents()
	if err != nil {
		return siteUsage{}, err
	}
	hashes, err := siteTorrentHashes(siteName)
	if err != nil {
		return siteUsage{}, err
	}
	links, err := reseedLinks()
	if err != nil {
		return siteUsage{}, err
	}
	skips, present := sharedDataSkips(torrents, links)
	u := siteUsage{used: sumSiteUsage(siteName, torrents, hashes, v2.NewTrackerResolver(), skips)}
	if h := strings.ToLower(strings.TrimSpace(pushHash)); h != "" {
		if src := links[h]; src != "" && src != h && present[src] {
			u.sharedSource = src
		}
	}
	return u, nil
}

// reseedLinks 返回 pt-tools 辅种加进下载器的种子与它的原种子（核对过文件、放在原种子的位置）：
// 辅种的种子 info hash → 原种子 info hash，都是小写。只取建成了辅种任务的记录（任务进行中或已完成）；
// 失败、站点已删除、回滚与取消的辅种没有留下种子。
func reseedLinks() (map[string]string, error) {
	if global.GlobalDB == nil {
		return nil, nil
	}
	var rows []models.ReseedRecord
	if err := global.GlobalDB.DB.Model(&models.ReseedRecord{}).Select("info_hash", "source_hash").
		Where("state IN ? AND source_hash != ''", []string{models.ReseedQueued, models.ReseedDone}).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	links := make(map[string]string, len(rows))
	for _, r := range rows {
		h, src := strings.ToLower(strings.TrimSpace(r.InfoHash)), strings.ToLower(strings.TrimSpace(r.SourceHash))
		if h != "" && src != "" {
			links[h] = src
		}
	}
	return links, nil
}

// sharedDataSkips 按辅种记录 links 把下载器里共用同一份数据的种子连成一组，每组只算一个种子的体积：
// 原来就有的那个（不是辅种加的）；都是辅种加的（原种子删了又被辅种加回来）时算最早加进下载器的那个。
// 原种子不在下载器里时，辅种的种子自己算（数据靠它留着）。返回不计入的种子与下载器里有哪些种子（都是小写 info hash）。
func sharedDataSkips(torrents []downloader.Torrent, links map[string]string) (skips, present map[string]bool) {
	present = make(map[string]bool, len(torrents))
	added := make(map[string]int64, len(torrents))
	for _, t := range torrents {
		if h := strings.ToLower(strings.TrimSpace(t.InfoHash)); h != "" {
			present[h] = true
			added[h] = t.DateAdded
		}
	}
	parent := map[string]string{}
	find := func(h string) string {
		for {
			p, ok := parent[h]
			if !ok || p == h {
				return h
			}
			h = p
		}
	}
	for h, src := range links {
		if h == src || !present[h] || !present[src] {
			continue
		}
		if _, ok := parent[h]; !ok {
			parent[h] = h
		}
		if _, ok := parent[src]; !ok {
			parent[src] = src
		}
		if a, b := find(h), find(src); a != b {
			parent[a] = b
		}
	}
	if len(parent) == 0 {
		return nil, present
	}
	// better 报告 a 是不是比 b 更该算体积：不是辅种加的优先，再看谁先加进下载器，最后按 hash（结果和遍历顺序无关）
	better := func(a, b string) bool {
		_, ra := links[a]
		_, rb := links[b]
		if ra != rb {
			return !ra
		}
		if added[a] != added[b] {
			return added[a] < added[b]
		}
		return a < b
	}
	owner := make(map[string]string, len(parent))
	for h := range parent {
		r := find(h)
		if cur, ok := owner[r]; !ok || better(h, cur) {
			owner[r] = h
		}
	}
	skips = make(map[string]bool, len(parent))
	for h := range parent {
		if owner[find(h)] != h {
			skips[h] = true
		}
	}
	return skips, present
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
	return sumSiteSeedingSizeWithHashes(siteName, torrents, nil, nil)
}

// sumSiteSeedingSizeWithHashes 求和归属该站点的种子体积：info hash 在 hashes 里、分类/标签命中站点名，
// 或主 tracker 经 resolver 识别为这个站点（resolver 为 nil 时不按 tracker 判断）。
func sumSiteSeedingSizeWithHashes(siteName string, torrents []downloader.Torrent, hashes map[string]struct{}, resolver *v2.TrackerResolver) int64 {
	return sumSiteUsage(siteName, torrents, hashes, resolver, nil)
}

// sumSiteUsage 和 sumSiteSeedingSizeWithHashes 一样求和，只是 skips 里的种子不算（和别的种子共用数据，见 sharedDataSkips）。
func sumSiteUsage(siteName string, torrents []downloader.Torrent, hashes map[string]struct{}, resolver *v2.TrackerResolver, skips map[string]bool) int64 {
	if siteName == "" {
		return 0
	}
	var total int64
	for _, t := range torrents {
		if skips[strings.ToLower(strings.TrimSpace(t.InfoHash))] {
			continue
		}
		_, recorded := hashes[strings.ToLower(t.InfoHash)]
		if (recorded && t.InfoHash != "") || torrentBelongsToSite(siteName, t) || trackerBelongsToSite(siteName, t, resolver) {
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

// trackerBelongsToSite 判断种子的主 tracker 是否属于指定站点。
func trackerBelongsToSite(siteName string, t downloader.Torrent, resolver *v2.TrackerResolver) bool {
	if resolver == nil || t.Tracker == "" {
		return false
	}
	id, ok := resolver.Resolve(t.Tracker)
	return ok && strings.EqualFold(id, siteName)
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
