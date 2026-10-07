package models

import (
	"strings"
	"time"
)

// 转移做种任务的状态。TransferStateFinal 为真的是终态。
const (
	// TransferPending 是刚建好、还没拿到种子文件的任务。
	TransferPending = "pending"
	// TransferExported 是种子文件已经拿到，等着加入目标下载器。
	TransferExported = "exported"
	// TransferAdding 是正在把种子暂停加入目标：先记下这个状态再推送，进程在推送途中退出时，
	// 重启后按目标里有没有这个种子决定继续还是重推。
	TransferAdding = "adding"
	// TransferChecking 是已经暂停加入目标、正在校验数据。
	TransferChecking = "checking"
	// TransferVerified 是目标校验到了 100%，接下来恢复目标、从源移除。
	TransferVerified = "verified"
	// TransferDone 是目标已经开始做种、源里的种子已移除（数据保留）。
	TransferDone = "source_removed"
	// TransferRolledBack 是校验没到 100% 或超时：目标里的种子已移除（数据保留），源不动。
	TransferRolledBack = "rolled_back"
	// TransferFailed 是加入目标之前就失败了（拿不到种子文件、目标已有这个种子、闸门拒绝等），源不动。
	TransferFailed = "failed"
	// TransferCanceled 是用户在加入目标之前取消的任务。
	TransferCanceled = "canceled"
)

// TransferTag 是转移做种加入目标时带上的标签：恢复、回滚、收尾前都要确认目标里的种子带着它，
// 证明是这次转移加的，不去动用户自己加的同一个种子。
const TransferTag = "pt-tools-transfer"

// TransferStateFinal 报告状态是不是终态。
func TransferStateFinal(state string) bool {
	switch state {
	case TransferDone, TransferRolledBack, TransferFailed, TransferCanceled:
		return true
	}
	return false
}

// TransferActiveStates 是未结束的状态（同一个种子同一时间只能有一个这样的任务）。
var TransferActiveStates = []string{TransferPending, TransferExported, TransferAdding, TransferChecking, TransferVerified}

// TorrentTransferJob 是把一个种子从源下载器转移到目标下载器的任务：导出种子 → 暂停加入目标（数据路径按映射换算）
// → 校验到 100% → 恢复目标 → 从源移除（不删数据）。状态持久化，进程重启后接着做。
// 列的默认值一律是零值（GORM 新建时会把零值换成 default）。
type TorrentTransferJob struct {
	ID                 uint   `gorm:"primaryKey" json:"id"`
	SourceDownloaderID uint   `gorm:"not null;index" json:"source_downloader_id"`
	TargetDownloaderID uint   `gorm:"not null;index" json:"target_downloader_id"`
	InfoHash           string `gorm:"size:64;not null;index" json:"info_hash"`
	Name               string `gorm:"size:512;default:''" json:"name"`
	TotalSize          int64  `gorm:"not null;default:0" json:"total_size"`
	// SiteName 与 TorrentID 来自 pt-tools 的种子记录；没有记录时 SiteName 按 tracker 认出，TorrentID 为空。
	SiteName       string `gorm:"size:64;default:''" json:"site_name"`
	TorrentID      string `gorm:"size:128;default:''" json:"torrent_id"`
	SourceSavePath string `gorm:"size:1024;default:''" json:"source_save_path"`
	TargetSavePath string `gorm:"size:1024;default:''" json:"target_save_path"`
	Category       string `gorm:"size:128;default:''" json:"category"`
	Tags           string `gorm:"size:512;default:''" json:"tags"`
	State          string `gorm:"size:32;not null;index" json:"state"`
	Message        string `gorm:"size:1024;default:''" json:"message"`
	// Progress 是最近一次看到的目标校验进度（0–1）。
	Progress float64 `gorm:"not null;default:0" json:"progress"`
	// RecheckIssued 表示已经让目标下载器开始校验。
	RecheckIssued bool `gorm:"not null;default:false" json:"recheck_issued"`
	// TorrentData 是导出的种子文件；任务结束时清空。
	TorrentData []byte `gorm:"type:blob" json:"-"`
	RuleID      *uint  `gorm:"index" json:"rule_id,omitempty"`
	// CheckStartedAt 是加入目标的时间；Deadline 是校验时限。
	CheckStartedAt *time.Time `json:"check_started_at,omitempty"`
	Deadline       *time.Time `json:"deadline,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (TorrentTransferJob) TableName() string { return "torrent_transfer_jobs" }

// DownloaderPathMap 是两个下载器之间的路径对应：源下载器看到的 SourcePrefix，在目标下载器里是 TargetPrefix。
// 换算时取最长的匹配前缀；没有匹配时路径不变。
type DownloaderPathMap struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	SourceDownloaderID uint      `gorm:"not null;uniqueIndex:idx_downloader_path_map" json:"source_downloader_id"`
	TargetDownloaderID uint      `gorm:"not null;uniqueIndex:idx_downloader_path_map" json:"target_downloader_id"`
	SourcePrefix       string    `gorm:"size:512;not null;uniqueIndex:idx_downloader_path_map" json:"source_prefix"`
	TargetPrefix       string    `gorm:"size:512;not null" json:"target_prefix"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (DownloaderPathMap) TableName() string { return "downloader_path_maps" }

// MapTransferPath 把源下载器里的路径换算成目标下载器里的路径：取最长的匹配前缀（按路径分段匹配，
// /data 不匹配 /database）；没有匹配时原样返回，第二个返回值为 false。
// Windows 路径（盘符或 \\ 开头）不区分大小写，/ 与 \ 都认；Unix 路径区分大小写。
// 前缀两边一个是 Windows 路径、一个是 Unix 路径时，余下部分的分隔符换成目标的写法。
func MapTransferPath(maps []DownloaderPathMap, src string) (string, bool) {
	best, bestLen := -1, -1
	for i, m := range maps {
		raw := strings.TrimSpace(m.SourcePrefix)
		if raw == "" {
			continue
		}
		if n, ok := matchPathPrefix(src, raw); ok && n > bestLen {
			best, bestLen = i, n
		}
	}
	if best < 0 {
		return src, false
	}
	target := strings.TrimRight(strings.TrimSpace(maps[best].TargetPrefix), "/\\")
	rest := src[bestLen:]
	switch {
	case strings.Contains(target, "\\") && !strings.Contains(target, "/"):
		rest = strings.ReplaceAll(rest, "/", "\\")
	case strings.Contains(target, "/") || target == "":
		rest = strings.ReplaceAll(rest, "\\", "/")
	}
	return target + rest, true
}

// matchPathPrefix 报告 prefix 是不是 src 的整段目录前缀，返回 src 里被前缀占去的字节数。
func matchPathPrefix(src, prefix string) (int, bool) {
	win := isWindowsPath(prefix) || isWindowsPath(src)
	if win {
		// 换分隔符不改变字节数，算出的长度可以直接用在原串上
		src, prefix = strings.ReplaceAll(src, "\\", "/"), strings.ReplaceAll(prefix, "\\", "/")
	}
	p := strings.TrimRight(prefix, "/")
	if p == "" { // 前缀是根目录，匹配所有绝对路径
		if strings.HasPrefix(src, "/") {
			return 0, true
		}
		return 0, false
	}
	if len(src) < len(p) {
		return 0, false
	}
	head := src[:len(p)]
	same := head == p
	if win {
		same = strings.EqualFold(head, p)
	}
	if !same || (len(src) > len(p) && src[len(p)] != '/') {
		return 0, false
	}
	return len(p), true
}

// isWindowsPath 报告路径是不是 Windows 写法：盘符（D:）或网络路径（\\server）。
func isWindowsPath(p string) bool {
	if strings.HasPrefix(p, "\\\\") {
		return true
	}
	return len(p) >= 2 && p[1] == ':' && (p[0]|0x20) >= 'a' && (p[0]|0x20) <= 'z'
}

// TransferRule 是定时转移规则：按间隔把源下载器里符合条件、已经做完种的种子建成转移任务。默认关闭。
// 条件为空或 0 表示不限。
type TransferRule struct {
	ID                 uint   `gorm:"primaryKey" json:"id"`
	Name               string `gorm:"size:64;not null;uniqueIndex" json:"name"`
	Enabled            bool   `gorm:"not null;default:false" json:"enabled"`
	SourceDownloaderID uint   `gorm:"not null" json:"source_downloader_id"`
	TargetDownloaderID uint   `gorm:"not null" json:"target_downloader_id"`
	// Category 是分类等于；Tag 是标签里含有（大小写不敏感）；SiteName 是 pt-tools 记录的站点或按 tracker 认出的站点。
	Category        string `gorm:"size:128;default:''" json:"category"`
	Tag             string `gorm:"size:128;default:''" json:"tag"`
	SiteName        string `gorm:"size:64;default:''" json:"site_name"`
	MinSeedingHours int    `gorm:"not null;default:0" json:"min_seeding_hours"`
	// MaxPerRun 是每一轮最多建几个任务（0 时按 TransferRuleDefaultMaxPerRun）。
	MaxPerRun int `gorm:"not null;default:0" json:"max_per_run"`
	// IntervalMin 是运行间隔（分钟，0 时按 TransferRuleDefaultIntervalMin）。
	IntervalMin int        `gorm:"not null;default:0" json:"interval_min"`
	LastRunAt   *time.Time `json:"last_run_at,omitempty"`
	LastResult  string     `gorm:"size:512;default:''" json:"last_result"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (TransferRule) TableName() string { return "transfer_rules" }

// 定时规则的默认值与范围。
const (
	TransferRuleDefaultMaxPerRun   = 10
	TransferRuleDefaultIntervalMin = 60
	TransferRuleMinIntervalMin     = 10
	TransferRuleMaxPerRun          = 100
)
