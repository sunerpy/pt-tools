package scheduler

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 删种的保护规则（见 SeedGuard），自动删种、做种竞争度、免费结束时的删除与媒体库的「删除一部片」共用（刷流删种用其中的标签部分）。
//
// hard 保护任何自动删除都遵守：保留标签、保护标签、H&R 没做满。
// soft 保护只挡常规删除条件、竞争度和紧急清理：保护下载中、最短保种时间。免费到期没下完的兜底不受 soft 保护挡。

type hrInfo struct {
	HasHR       bool
	HRSeedTimeH int
	// UploadRatio 是站点设置里填的「H&R 上传量达标」（#542）：下完以后上传量达到体积的这么多倍也算达标，0 是不按上传量
	UploadRatio float64
	// SizeBytes 是记录里的种子体积（0 是没记），按上传量判断时用
	SizeBytes int64
}

// hrUploadMet 报告种子是不是按上传量做满了 H&R（#542）：站点设置里填了上传量要求（ratio > 0）、体积已知、
// 种子下完了，并且累计上传不少于体积的 ratio 倍。自动删种（hrGuard）与刷流删种（brushRemoval）共用这一个判断。
func hrUploadMet(ratio float64, sizeBytes, uploaded int64, progress float64) bool {
	if ratio <= 0 || sizeBytes <= 0 || progress < 1 {
		return false
	}
	return float64(uploaded) >= ratio*float64(sizeBytes)
}

// formatUploadedRatio 是给人看的已传倍数：最多两位小数、去掉末尾的 0，向下取（还差一点时不显示成已经够了）。
func formatUploadedRatio(v float64) string {
	return strconv.FormatFloat(math.Floor(v*100+1e-9)/100, 'f', -1, 64)
}

// formatRequiredRatio 是给人看的要求倍数：按填的值写（最多三位小数）。
func formatRequiredRatio(v float64) string {
	return strconv.FormatFloat(math.Round(v*1000)/1000, 'f', -1, 64)
}

// loadHRInfo 读出每个种子（info hash，小写）的 H&R 要求：记录上记的、站点定义的整站 H&R，或站点设置里填的做种要求（#727），
// 以及站点设置里填的上传量要求（#542）和记录里的体积。
// 读库出错时返回错误：调用方按读不到处理，这一轮不删（宁可不删也不删掉 H&R 没做满的种子）。
func loadHRInfo(db *gorm.DB) (map[string]hrInfo, error) {
	result := make(map[string]hrInfo)

	defs := make(map[string]*v2.SiteDefinition)
	for _, def := range v2.GetDefinitionRegistry().GetAll() {
		defs[def.ID] = def
	}
	// 站点设置里填的 H&R 做种要求（#727，站点定义里没有时用它）与上传量要求（#542）
	settingH := make(map[string]int)
	settingRatio := make(map[string]float64)
	var settings []struct {
		Name          string
		HRSeedTimeH   int
		HRUploadRatio float64
	}
	if err := db.Model(&models.SiteSetting{}).Select("name, hr_seed_time_h, hr_upload_ratio").
		Where("hr_seed_time_h > 0 OR hr_upload_ratio > 0").Scan(&settings).Error; err != nil {
		return nil, fmt.Errorf("读取站点的 H&R 设置失败: %w", err)
	}
	for _, s := range settings {
		settingH[s.Name] = max(s.HRSeedTimeH, 0)
		settingRatio[s.Name] = ptinternal.UsableHRUploadRatio(s.HRUploadRatio)
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
		info.UploadRatio, info.SizeBytes = settingRatio[r.SiteName], max(r.TorrentSize, 0)
		if cur, seen := result[hash]; seen {
			info = mergeHRInfo(cur, info)
		}
		result[hash] = info
	}
	return result, nil
}

// mergeHRInfo 合并同一个哈希的几条记录（跨站辅种等），取最严的：有一条做种要求未知就按未知；
// 有一条的站点没填上传量要求就不按上传量放行，都填了取最大的倍数；体积取最大的。
func mergeHRInfo(a, b hrInfo) hrInfo {
	out := hrInfo{HasHR: true, SizeBytes: max(a.SizeBytes, b.SizeBytes)}
	if a.HRSeedTimeH > 0 && b.HRSeedTimeH > 0 {
		out.HRSeedTimeH = max(a.HRSeedTimeH, b.HRSeedTimeH)
	}
	if a.UploadRatio > 0 && b.UploadRatio > 0 {
		out.UploadRatio = max(a.UploadRatio, b.UploadRatio)
	}
	return out
}

// 保护规则（Protection.Rule）：前两条与 H&R 是 hard，后两条是 soft。
const (
	GuardKeepTag     = "keep_tag"
	GuardProtectTag  = "protect_tag"
	GuardHR          = "hr"
	GuardDownloading = "downloading"
	GuardMinRetain   = "min_retain"
)

// Protection 是种子命中的一条保护规则；Rule 为空表示不受保护。Text 是给人看的原因（自动删种的预览、删除弹窗里写的就是它）；
// Tag 是标签规则命中的标签；Hours 是 H&R 要求的做种小时数（0 是要求不明）或最短保种时间；Left 是 H&R 或最短保种还差的秒数。
type Protection struct {
	Rule  string
	Text  string
	Tag   string
	Hours int
	Left  int64
}

// tagGuard 是种子命中的标签保护：保留标签（pt-tools-keep，内置）或保护标签。看 qB 的标签与分类、Transmission 的 labels。
func tagGuard(t downloader.Torrent, protectTags []string) Protection {
	tTags := splitTags(t.Tags + "," + t.Category + "," + t.Label)
	if containsIgnoreCase(tTags, models.KeepTag) {
		return Protection{Rule: GuardKeepTag, Text: "带保留标签（" + models.KeepTag + "）", Tag: models.KeepTag}
	}
	for _, pt := range protectTags {
		if containsIgnoreCase(tTags, pt) {
			return Protection{Rule: GuardProtectTag, Text: "带保护标签「" + pt + "」", Tag: pt}
		}
	}
	return Protection{}
}

// tagProtection 是 tagGuard 的原因文字（空串表示没命中）。
func tagProtection(t downloader.Torrent, protectTags []string) string {
	return tagGuard(t, protectTags).Text
}

// hrGuard 是 H&R 保护：有 H&R 而做种时长没满，或要求未知（站点定义里没有、站点设置也没填，#727）。
// 站点设置里填了「H&R 上传量达标」（#542）时，下完、上传量够了也不保护；体积用记录里的，没有时用下载器给的。
func hrGuard(t downloader.Torrent, hr map[string]hrInfo) Protection {
	info, ok := hr[strings.ToLower(t.InfoHash)]
	if !ok || !info.HasHR {
		return Protection{}
	}
	requiredSeedTimeS := int64(info.HRSeedTimeH) * 3600
	if requiredSeedTimeS > 0 && t.SeedingTime >= requiredSeedTimeS {
		return Protection{}
	}
	size := info.SizeBytes
	if size <= 0 {
		size = t.TotalSize
	}
	if hrUploadMet(info.UploadRatio, size, t.TotalUploaded, t.Progress) {
		return Protection{}
	}
	byUpload := info.UploadRatio > 0 && size > 0
	uploaded := ""
	if byUpload {
		uploaded = "已传 " + formatUploadedRatio(float64(max(t.TotalUploaded, 0))/float64(size)) + " 倍"
		if t.Progress < 1 {
			uploaded += "（还没下完，下完才按上传量算）"
		}
	}
	switch {
	case requiredSeedTimeS <= 0 && byUpload:
		return Protection{Rule: GuardHR, Text: fmt.Sprintf("H&R：站点没有给出做种时长，要求上传 %s 倍体积，%s", formatRequiredRatio(info.UploadRatio), uploaded)}
	case requiredSeedTimeS <= 0:
		return Protection{Rule: GuardHR, Text: "H&R：站点没有给出做种要求，一直保护（可以在站点设置里填「H&R 做种要求」）"}
	}
	text := fmt.Sprintf("H&R：要求做种 %d 小时，已做 %.1f 小时", info.HRSeedTimeH, float64(t.SeedingTime)/3600)
	if byUpload {
		text = fmt.Sprintf("H&R：要求做种 %d 小时或上传 %s 倍体积，已做 %.1f 小时、%s",
			info.HRSeedTimeH, formatRequiredRatio(info.UploadRatio), float64(t.SeedingTime)/3600, uploaded)
	}
	return Protection{Rule: GuardHR, Text: text, Hours: info.HRSeedTimeH, Left: requiredSeedTimeS - max(t.SeedingTime, 0)}
}

// hardGuard 是任何自动删除都遵守的保护：保留标签 → 保护标签 → H&R（打开了「保护 H&R 种子」时）。
func hardGuard(cfg *models.SettingsGlobal, t downloader.Torrent, hr map[string]hrInfo, protectTags []string) Protection {
	if p := tagGuard(t, protectTags); p.Rule != "" {
		return p
	}
	if cfg.CleanupProtectHR {
		return hrGuard(t, hr)
	}
	return Protection{}
}

// hardProtection 是 hardGuard 的原因文字（空串表示不受保护）。
func hardProtection(cfg *models.SettingsGlobal, t downloader.Torrent, hr map[string]hrInfo, protectTags []string) string {
	return hardGuard(cfg, t, hr, protectTags).Text
}

// softGuard 是只挡常规删除条件、竞争度与紧急清理的保护：保护下载中 → 最短保种时间。
func softGuard(cfg *models.SettingsGlobal, t downloader.Torrent, now int64) Protection {
	if cfg.CleanupProtectDL && (t.State == downloader.TorrentDownloading || t.State == downloader.TorrentChecking) {
		return Protection{Rule: GuardDownloading, Text: "正在下载或校验（打开了「保护下载中的种子」）"}
	}
	if cfg.CleanupMinRetainH > 0 && t.DateAdded > 0 {
		retainUntil := t.DateAdded + int64(cfg.CleanupMinRetainH*3600)
		if now < retainUntil {
			return Protection{
				Rule: GuardMinRetain, Hours: cfg.CleanupMinRetainH, Left: retainUntil - now,
				Text: fmt.Sprintf("加进下载器不到 %d 小时（最短保种时间），还差 %.1f 小时", cfg.CleanupMinRetainH, float64(retainUntil-now)/3600),
			}
		}
	}
	return Protection{}
}

// softProtection 是 softGuard 的原因文字（空串表示不受保护）。
func softProtection(cfg *models.SettingsGlobal, t downloader.Torrent, now int64) string {
	return softGuard(cfg, t, now).Text
}

// SeedGuard 是删种的保护规则：自动删种（含做种竞争度与紧急清理）、免费结束时的删除，以及媒体库里的「删除一部片」共用。
// 建的时候按全局设置读出 H&R 要求、记下当时的时间，之后只做判断，不读库。
type SeedGuard struct {
	cfg         models.SettingsGlobal
	hr          map[string]hrInfo
	protectTags []string
	now         int64
}

// NewSeedGuard 按全局设置 cfg 建保护规则，读出每个种子的 H&R 要求。读库出错时返回错误：分不清哪些种子受保护，调用方这一次不删。
func NewSeedGuard(db *gorm.DB, cfg *models.SettingsGlobal) (*SeedGuard, error) {
	hr, err := loadHRInfo(db)
	if err != nil {
		return nil, err
	}
	return &SeedGuard{cfg: *cfg, hr: hr, protectTags: splitTags(cfg.CleanupProtectTags), now: time.Now().Unix()}, nil
}

// LoadSeedGuard 按库里的全局设置建保护规则；还没有全局设置时按设置的默认值（保护 H&R、最短保种 24 小时）。
// 读库出错时返回错误（调用方这一次不删）。
func LoadSeedGuard(db *gorm.DB) (*SeedGuard, error) {
	var cfg models.SettingsGlobal
	if err := db.Limit(1).Find(&cfg).Error; err != nil {
		return nil, fmt.Errorf("读取全局设置失败: %w", err)
	}
	if cfg.ID == 0 {
		cfg = models.SettingsGlobal{CleanupProtectHR: true, CleanupMinRetainH: 24}
	}
	return NewSeedGuard(db, &cfg)
}

// Check 是种子命中的第一条保护规则：先 hard（保留标签、保护标签、H&R），再 soft（下载中、最短保种时间）。
func (g *SeedGuard) Check(t downloader.Torrent) Protection {
	if p := g.Hard(t); p.Rule != "" {
		return p
	}
	return softGuard(&g.cfg, t, g.now)
}

// Hard 只看 hard 保护（免费过了还没下完的兜底、免费结束时的删除不等最短保种时间、不管「保护下载中」）。
func (g *SeedGuard) Hard(t downloader.Torrent) Protection {
	return hardGuard(&g.cfg, t, g.hr, g.protectTags)
}

// Why 是 Check 的原因文字（空串表示不受保护）。
func (g *SeedGuard) Why(t downloader.Torrent) string {
	return g.Check(t).Text
}

// Rules 是现在开着的保护规则，按「下载中、最短保种、H&R、标签」的顺序（条目详情的说明用）；保留标签一直开着。
func (g *SeedGuard) Rules() []string {
	var out []string
	if g.cfg.CleanupProtectDL {
		out = append(out, "下载中")
	}
	if g.cfg.CleanupMinRetainH > 0 {
		out = append(out, fmt.Sprintf("最短保种 %d 小时", g.cfg.CleanupMinRetainH))
	}
	if g.cfg.CleanupProtectHR {
		out = append(out, "H&R")
	}
	if len(g.protectTags) > 0 {
		out = append(out, "保留与保护标签")
	} else {
		out = append(out, "保留标签")
	}
	return out
}
