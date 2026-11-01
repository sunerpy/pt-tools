package scheduler

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 删种的保护规则，自动删种、做种竞争度、免费结束时的删除共用（刷流删种用其中的标签部分）。
//
// hard 保护任何自动删除都遵守：保留标签、保护标签、H&R 没做满。
// soft 保护只挡常规删除条件、竞争度和紧急清理：保护下载中、最短保种时间。免费到期没下完的兜底不受 soft 保护挡。

type hrInfo struct {
	HasHR       bool
	HRSeedTimeH int
}

// loadHRInfo 读出每个种子（info hash，小写）的 H&R 要求：记录上记的、站点定义的整站 H&R，或站点设置里填的做种要求（#727）。
// 读库出错时返回错误：调用方按读不到处理，这一轮不删（宁可不删也不删掉 H&R 没做满的种子）。
func loadHRInfo(db *gorm.DB) (map[string]hrInfo, error) {
	result := make(map[string]hrInfo)

	defs := make(map[string]*v2.SiteDefinition)
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		defs[def.ID] = def
	}
	// 站点设置里填的 H&R 做种要求（#727）：站点定义里没有时用它
	settingH := make(map[string]int)
	var settings []struct {
		Name        string
		HRSeedTimeH int
	}
	if err := db.Model(&models.SiteSetting{}).Select("name, hr_seed_time_h").Where("hr_seed_time_h > 0").Scan(&settings).Error; err != nil {
		return nil, fmt.Errorf("读取站点的 H&R 设置失败: %w", err)
	}
	for _, s := range settings {
		settingH[s.Name] = s.HRSeedTimeH
	}

	type hrRecord struct {
		TorrentHash string
		SiteName    string
		HasHR       bool
		HRSeedTimeH int
		TorrentSize int64
	}
	// 活动表与归档表都看：记录归档以后，下载器里的种子照样要按 H&R 保护
	var records, archived []hrRecord
	const cols = "torrent_hash, site_name, has_hr, hr_seed_time_h, torrent_size"
	if err := db.Model(&models.TorrentInfo{}).Select(cols).Where("torrent_hash IS NOT NULL AND torrent_hash != ''").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("读取种子的 H&R 信息失败: %w", err)
	}
	if err := db.Model(&models.TorrentInfoArchive{}).Select(cols).Where("torrent_hash IS NOT NULL AND torrent_hash != ''").Find(&archived).Error; err != nil {
		return nil, fmt.Errorf("读取归档种子的 H&R 信息失败: %w", err)
	}

	for _, r := range append(records, archived...) {
		hash := strings.ToLower(r.TorrentHash)
		def := defs[r.SiteName]
		var info hrInfo
		switch {
		case r.HasHR:
			h := r.HRSeedTimeH
			if h <= 0 {
				// 记录上没记下要求：按站点定义算，定义里没有时用站点设置（记录写成以后才填的设置也算）
				h = v2.ResolveHRSeedTimeH(def, r.TorrentSize, settingH[r.SiteName])
			}
			info = hrInfo{HasHR: true, HRSeedTimeH: h}
		case def != nil && def.HREnabled:
			// 整站 H&R：按体积算要求
			info = hrInfo{HasHR: true, HRSeedTimeH: v2.ResolveHRSeedTimeH(def, r.TorrentSize, settingH[r.SiteName])}
		default:
			continue
		}
		cur, seen := result[hash]
		switch {
		case !seen:
			result[hash] = info
		case cur.HRSeedTimeH <= 0 || info.HRSeedTimeH <= 0:
			// 同一个哈希有几条记录（跨站辅种等）：取最严的——有一条要求未知就按未知保护
			result[hash] = hrInfo{HasHR: true}
		default:
			result[hash] = hrInfo{HasHR: true, HRSeedTimeH: max(cur.HRSeedTimeH, info.HRSeedTimeH)}
		}
	}
	return result, nil
}

// tagProtection 是种子命中的标签保护（空串表示没命中）：保留标签（pt-tools-keep，内置）或保护标签。看 qB 的标签与分类、Transmission 的 labels。
func tagProtection(t downloader.Torrent, protectTags []string) string {
	tTags := splitTags(t.Tags + "," + t.Category + "," + t.Label)
	if containsIgnoreCase(tTags, models.KeepTag) {
		return "带保留标签（" + models.KeepTag + "）"
	}
	for _, pt := range protectTags {
		if containsIgnoreCase(tTags, pt) {
			return "带保护标签「" + pt + "」"
		}
	}
	return ""
}

// hrProtection 是 H&R 保护（空串表示不受保护）：有 H&R 而做种时长没满，或要求未知（站点定义里没有、站点设置也没填，#727）。
func hrProtection(t downloader.Torrent, hr map[string]hrInfo) string {
	info, ok := hr[strings.ToLower(t.InfoHash)]
	if !ok || !info.HasHR {
		return ""
	}
	requiredSeedTimeS := int64(info.HRSeedTimeH) * 3600
	if requiredSeedTimeS > 0 && t.SeedingTime >= requiredSeedTimeS {
		return ""
	}
	if requiredSeedTimeS <= 0 {
		return "H&R：站点没有给出做种要求，一直保护（可以在站点设置里填「H&R 做种要求」）"
	}
	return fmt.Sprintf("H&R：要求做种 %d 小时，已做 %.1f 小时", info.HRSeedTimeH, float64(t.SeedingTime)/3600)
}

// hardProtection 是任何自动删除都遵守的保护（空串表示不受保护）：保留标签 → 保护标签 → H&R（打开了「保护 H&R 种子」时）。
func hardProtection(cfg *models.SettingsGlobal, t downloader.Torrent, hr map[string]hrInfo, protectTags []string) string {
	if why := tagProtection(t, protectTags); why != "" {
		return why
	}
	if cfg.CleanupProtectHR {
		return hrProtection(t, hr)
	}
	return ""
}

// softProtection 是只挡常规删除条件、竞争度与紧急清理的保护（空串表示不受保护）：保护下载中 → 最短保种时间。
func softProtection(cfg *models.SettingsGlobal, t downloader.Torrent, now int64) string {
	if cfg.CleanupProtectDL && (t.State == downloader.TorrentDownloading || t.State == downloader.TorrentChecking) {
		return "正在下载或校验（打开了「保护下载中的种子」）"
	}
	if cfg.CleanupMinRetainH > 0 && t.DateAdded > 0 {
		retainUntil := t.DateAdded + int64(cfg.CleanupMinRetainH*3600)
		if now < retainUntil {
			return fmt.Sprintf("加进下载器不到 %d 小时（最短保种时间），还差 %.1f 小时", cfg.CleanupMinRetainH, float64(retainUntil-now)/3600)
		}
	}
	return ""
}
