package scheduler

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 刷流的入场与删种判断都是纯函数：输入任务配置和一条搜索结果 / 一个下载器里的种子，输出判定和原因，便于表驱动测试。

// BrushDiscountLevels 是刷流入场可选的优惠类型（与 site/v2 的 DiscountLevel 取值一致）。
var BrushDiscountLevels = []v2.DiscountLevel{
	v2.DiscountFree, v2.Discount2xFree, v2.DiscountPercent30, v2.DiscountPercent50,
	v2.DiscountPercent70, v2.Discount2xUp, v2.Discount2x50,
}

// brushAllowedDiscounts 解析任务的 Discounts；为空时只收免费（FREE、2XFREE）。
func brushAllowedDiscounts(task models.BrushTask) map[v2.DiscountLevel]bool {
	out := map[v2.DiscountLevel]bool{}
	for raw := range strings.SplitSeq(task.Discounts, ",") {
		if lv := v2.DiscountLevel(strings.ToUpper(strings.TrimSpace(raw))); lv != "" {
			out[lv] = true
		}
	}
	if len(out) == 0 {
		for _, lv := range v2.FreeDiscountLevels {
			out[lv] = true
		}
	}
	return out
}

// splitKeywords 按换行与逗号拆关键词，去空、转小写。
func splitKeywords(s string) []string {
	var out []string
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == ',' || r == '，' }) {
		if kw := strings.ToLower(strings.TrimSpace(line)); kw != "" {
			out = append(out, kw)
		}
	}
	return out
}

const gib = 1024 * 1024 * 1024

// admitBrushItem 判断一条搜索结果能不能进入刷流。siteHR 是站点定义的整站 H&R（与 RSS 路径同一规则：
// hasHR = item.HasHR || def.HREnabled）。M-Team 的搜索忽略 FreeOnly、结果里会混入非免费种，所以优惠类型一律在这里判。
func admitBrushItem(task models.BrushTask, item v2.TorrentItem, siteHR bool, now time.Time) (bool, string) {
	if item.ID == "" {
		return false, "没有种子 ID"
	}
	allowed := brushAllowedDiscounts(task)
	if !allowed[item.DiscountLevel] {
		return false, fmt.Sprintf("优惠类型 %s 不在允许范围", item.DiscountLevel)
	}
	if !item.DiscountEndTime.IsZero() {
		if !item.DiscountEndTime.After(now) {
			return false, "优惠已到期"
		}
		if task.MinFreeRemainMin > 0 && item.DiscountEndTime.Sub(now) < time.Duration(task.MinFreeRemainMin)*time.Minute {
			return false, fmt.Sprintf("优惠剩余不足 %d 分钟", task.MinFreeRemainMin)
		}
	}
	if task.MinSizeGB > 0 || task.MaxSizeGB > 0 {
		if item.SizeBytes <= 0 {
			return false, "体积未知"
		}
		sizeGB := float64(item.SizeBytes) / gib
		if task.MinSizeGB > 0 && sizeGB < task.MinSizeGB {
			return false, fmt.Sprintf("体积 %.2f GB 小于 %.2f GB", sizeGB, task.MinSizeGB)
		}
		if task.MaxSizeGB > 0 && sizeGB > task.MaxSizeGB {
			return false, fmt.Sprintf("体积 %.2f GB 大于 %.2f GB", sizeGB, task.MaxSizeGB)
		}
	}
	if task.MaxSeeders > 0 && item.Seeders > task.MaxSeeders {
		return false, fmt.Sprintf("做种 %d 人多于 %d", item.Seeders, task.MaxSeeders)
	}
	if task.MinLeechers > 0 && item.Leechers < task.MinLeechers {
		return false, fmt.Sprintf("下载 %d 人少于 %d", item.Leechers, task.MinLeechers)
	}
	if task.MaxPublishAgeMin > 0 {
		if item.UploadedAt <= 0 {
			return false, "发布时间未知"
		}
		if age := now.Sub(time.Unix(item.UploadedAt, 0)); age > time.Duration(task.MaxPublishAgeMin)*time.Minute {
			return false, fmt.Sprintf("发布已 %d 分钟，超过 %d 分钟", int(age.Minutes()), task.MaxPublishAgeMin)
		}
	}
	if task.ExcludeHR && (item.HasHR || siteHR) {
		return false, "有 H&R"
	}
	text := strings.ToLower(item.Title + " " + item.Subtitle)
	if inc := splitKeywords(task.IncludeKeywords); len(inc) > 0 {
		hit := false
		for _, kw := range inc {
			if strings.Contains(text, kw) {
				hit = true
				break
			}
		}
		if !hit {
			return false, "标题不含任何包含词"
		}
	}
	for _, kw := range splitKeywords(task.ExcludeKeywords) {
		if strings.Contains(text, kw) {
			return false, fmt.Sprintf("标题含排除词 %s", kw)
		}
	}
	return true, ""
}

// sortBrushCandidates 把候选按下载人数多、做种人数少、发布新排在前面：抢在人多的时候进场，上传空间最大。
func sortBrushCandidates(items []v2.TorrentItem) {
	sort.SliceStable(items, func(a, b int) bool {
		x, y := items[a], items[b]
		if x.Leechers != y.Leechers {
			return x.Leechers > y.Leechers
		}
		if x.Seeders != y.Seeders {
			return x.Seeders < y.Seeders
		}
		return x.UploadedAt > y.UploadedAt
	})
}

// brushRemoval 判断一个刷流种子这一轮要不要删，返回原因。samples 是 SamplesSince(now - 窗口) 的结果
// （第一条是窗口起点之前最近的一条），只在低速规则里用到。
//
// H&R 保护与自动清理（cleanup_monitor 的 hrInfo）同一判断：有 H&R 且要求的做种时长还没满时一律不删，
// 包括免费到期还没下完的情况 —— 删掉未完成的 H&R 种子同样会被站点记一次 H&R。
func brushRemoval(task models.BrushTask, bt models.BrushTorrent, t downloader.Torrent, samples []models.BrushTorrentSample, now time.Time) (bool, string) {
	if bt.HasHR && bt.HRSeedTimeH > 0 && t.SeedingTime < int64(bt.HRSeedTimeH)*3600 {
		return false, ""
	}
	completed := t.IsCompleted || t.Progress >= 1
	if task.RemoveFreeExpiredIncomplete && !completed && bt.FreeEndAt != nil {
		// 下一轮检查之前就会到期的，这一轮就删：等到下一轮，到期之后那段已经按非免费计了下载
		interval := time.Duration(max(task.IntervalMin, 1)) * time.Minute
		if !now.Before(bt.FreeEndAt.Add(-interval)) {
			return true, "免费即将到期，还没下完"
		}
	}
	if task.RemoveSeedTimeH > 0 && completed && float64(t.SeedingTime) >= task.RemoveSeedTimeH*3600 {
		return true, fmt.Sprintf("做种 %.1f 小时，达到 %.1f 小时", float64(t.SeedingTime)/3600, task.RemoveSeedTimeH)
	}
	if task.RemoveRatio > 0 && t.Ratio >= task.RemoveRatio {
		return true, fmt.Sprintf("分享率 %.2f，达到 %.2f", t.Ratio, task.RemoveRatio)
	}
	if task.RemoveLowSpeedKBs > 0 && completed && task.RemoveLowSpeedWindowMin > 0 && len(samples) > 0 {
		window := time.Duration(task.RemoveLowSpeedWindowMin) * time.Minute
		start := samples[0]
		// 整个窗口都在做种、而且采样覆盖了整个窗口才判：刚下完或刚开始采样的种子不算
		if t.SeedingTime >= int64(window.Seconds()) && !start.At.After(now.Add(-window)) {
			elapsed := now.Sub(start.At).Seconds()
			if elapsed > 0 {
				speed := float64(t.TotalUploaded-start.Uploaded) / elapsed / 1024
				if speed < task.RemoveLowSpeedKBs {
					return true, fmt.Sprintf("最近 %d 分钟平均上传 %.1f KB/s，低于 %.1f KB/s", task.RemoveLowSpeedWindowMin, max(speed, 0), task.RemoveLowSpeedKBs)
				}
			}
		}
	}
	if task.RemoveInactiveH > 0 && bt.LastActivityAt != nil {
		idle := now.Sub(*bt.LastActivityAt)
		if idle >= time.Duration(task.RemoveInactiveH*float64(time.Hour)) {
			return true, fmt.Sprintf("已 %.1f 小时没有上传或下载", idle.Hours())
		}
	}
	return false, ""
}

// hasTag 报告种子的标签（qB tags / TR labels，逗号分隔）里是否有 tag（大小写不敏感）。
func hasTag(t downloader.Torrent, tag string) bool {
	for raw := range strings.SplitSeq(t.Tags+","+t.Label, ",") {
		if strings.EqualFold(strings.TrimSpace(raw), tag) {
			return true
		}
	}
	return false
}
