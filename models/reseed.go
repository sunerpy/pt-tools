package models

import (
	"strconv"
	"time"
)

// ReseedTag 是辅种加进下载器时带上的标签：恢复、回滚、收尾前确认目标里的种子带着它，证明是辅种加的。
const ReseedTag = "pt-tools-reseed"

// 转移任务的种类：转移做种（从源移除）与辅种（源与目标是同一台下载器，不移除任何种子）。
const (
	JobKindTransfer = ""
	JobKindReseed   = "reseed"
)

// JobOwnerTag 是这一种任务加进下载器时带上的通用标签（显示与分类用）。
func JobOwnerTag(kind string) string {
	if kind == JobKindReseed {
		return ReseedTag
	}
	return TransferTag
}

// JobTag 是任务 id 加进下载器时带上的独有标签（如 pt-tools-reseed-12）：通用标签谁都能加，证明不了种子是这个任务加的。
func JobTag(kind string, id uint) string {
	return JobOwnerTag(kind) + "-" + strconv.FormatUint(uint64(id), 10)
}

// ReseedSetting 是 IYUU 辅种的设置（只有一行，ID 为 1）。默认关闭；列的默认值一律是零值。
type ReseedSetting struct {
	ID      uint `gorm:"primaryKey" json:"id"`
	Enabled bool `gorm:"not null;default:false" json:"enabled"`
	// TokenEncrypted 是加密后的 IYUU token；接口只返回是否已设置。
	TokenEncrypted string `gorm:"type:text;default:''" json:"-"`
	// IntervalHours 是定时运行的间隔（0 时按 ReseedDefaultIntervalHours）。
	IntervalHours int `gorm:"not null;default:0" json:"interval_hours"`
	// DownloaderIDs 是参与辅种的下载器（JSON 数组，空表示全部启用的下载器）。
	DownloaderIDs string `gorm:"type:text;default:''" json:"-"`
	// SiteNames 是允许辅种的站点（JSON 数组）；SiteMode 为 ReseedSitesSelected 时只辅种这些站点。
	SiteNames string `gorm:"type:text;default:''" json:"-"`
	// SiteMode 是站点范围：ReseedSitesAll（全部已配置、IYUU 也支持的站点，以后新配置的也算）或 ReseedSitesSelected（只算 SiteNames）。
	// 旧数据为空串：SiteNames 为空时是全部，不为空时只算 SiteNames（见 ReseedSiteModeOf）。
	SiteMode string `gorm:"size:16;not null;default:''" json:"-"`
	// MaxPerSitePerDay 是每个站点每天最多加几个辅种（0 时按 ReseedDefaultMaxPerSitePerDay）。
	MaxPerSitePerDay int `gorm:"not null;default:0" json:"max_per_site_per_day"`
	// SidSha1 是 IYUU 的 reportExisting 返回的站点哈希，SidKey 是当时报告的站点 sid 列表；7 天内站点没变时复用。
	SidSha1   string     `gorm:"size:128;default:''" json:"-"`
	SidKey    string     `gorm:"type:text;default:''" json:"-"`
	SidAt     *time.Time `json:"-"`
	LastRunAt *time.Time `json:"last_run_at,omitempty"`
	// LastResult 是上一轮的一句话结果。
	LastResult string    `gorm:"type:text;default:''" json:"last_result"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (ReseedSetting) TableName() string { return "reseed_settings" }

// 辅种的站点范围。
const (
	ReseedSitesAll      = "all"
	ReseedSitesSelected = "selected"
)

// ReseedSiteModeOf 是设置里生效的站点范围：SiteMode 为空（旧数据）时，名单为空是全部，不为空是只算名单。
// siteNames 是解出来的名单。
func ReseedSiteModeOf(mode string, siteNames []string) string {
	switch mode {
	case ReseedSitesAll, ReseedSitesSelected:
		return mode
	}
	if len(siteNames) == 0 {
		return ReseedSitesAll
	}
	return ReseedSitesSelected
}

// 辅种设置的默认值与范围。
const (
	ReseedDefaultIntervalHours    = 12
	ReseedMinIntervalHours        = 1
	ReseedMaxIntervalHours        = 168
	ReseedDefaultMaxPerSitePerDay = 20
	ReseedMaxPerSitePerDay        = 500
	// ReseedSidTTL 是 sid_sha1 的有效期。
	ReseedSidTTL = 7 * 24 * time.Hour
	// ReseedRetryAfter 是暂时失败（下载不到种子等）之后多久再试。
	ReseedRetryAfter = 7 * 24 * time.Hour
)

// 辅种记录的结果。
const (
	// ReseedQueued 是已经建成辅种任务，结果看任务的状态。
	ReseedQueued = "queued"
	// ReseedFailed 是下载种子、核对 info hash 或文件列表失败；同一对不再尝试。
	ReseedFailed = "failed"
	// ReseedGone 是站点上已经没有这个种子（删了，IYUU 的索引里还有）：同一对不再尝试，也不算失败。
	ReseedGone = "gone"
	// 下面几个是清除已结束的辅种任务时，写进记录里的任务结果（任务失败的记为 ReseedFailed）。
	ReseedDone       = "done"
	ReseedRolledBack = "rolled_back"
	ReseedCanceled   = "canceled"
)

// ReseedRecord 记下对某个站点某个种子（info hash）的一次辅种尝试，同一对只尝试一次。
type ReseedRecord struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	InfoHash string `gorm:"size:64;not null;uniqueIndex:idx_reseed_hash_site" json:"info_hash"`
	SiteName string `gorm:"size:64;not null;uniqueIndex:idx_reseed_hash_site;index" json:"site_name"`
	// TorrentID 是站内种子 ID；SourceHash 是下载器里那个数据相同的种子。
	TorrentID    string `gorm:"size:128;default:''" json:"torrent_id"`
	SourceHash   string `gorm:"size:64;default:''" json:"source_hash"`
	DownloaderID uint   `gorm:"not null;default:0" json:"downloader_id"`
	Name         string `gorm:"size:512;default:''" json:"name"`
	State        string `gorm:"size:32;not null;index" json:"state"`
	Message      string `gorm:"size:1024;default:''" json:"message"`
	// Retryable 表示这次失败是暂时的（站点不可用、下载种子失败），过了 ReseedRetryAfter 可以再试；核对没通过的不再试。
	Retryable bool      `gorm:"not null;default:false" json:"retryable"`
	JobID     *uint     `gorm:"index" json:"job_id,omitempty"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
	UpdatedAt time.Time `gorm:"index" json:"updated_at"`
}

// TableName 指定表名。
func (ReseedRecord) TableName() string { return "reseed_records" }

// 辅种轮次的触发方式。
const (
	ReseedTriggerSchedule = "schedule"
	ReseedTriggerManual   = "manual"
	// ReseedTriggerRetry 是在记录里勾选重试（不查 IYUU，只重试选中的记录）
	ReseedTriggerRetry = "retry"
	// ReseedTriggerContinue 是接着一轮没跑完的往下查
	ReseedTriggerContinue = "continue"
)

// 辅种轮次的状态。
const (
	ReseedRunRunning = "running"
	ReseedRunPaused  = "paused"
	ReseedRunOK      = "ok"
	// ReseedRunPartial 是这一轮跑完了，但有下载器读不到、查询 IYUU 出错之类的问题
	ReseedRunPartial = "partial"
	// ReseedRunStopped 是被停止（手动停止、暂停太久自动停止、运行中换了 token）
	ReseedRunStopped = "stopped"
	// ReseedRunLimited 是超时、IYUU 限流或到了每天的请求上限而提前停下
	ReseedRunLimited = "limited"
	// ReseedRunFailed 是没能开始（没有 token、没有站点等）
	ReseedRunFailed = "failed"
	// ReseedRunInterrupted 是 pt-tools 退出时这一轮还没跑完（启动时补记）
	ReseedRunInterrupted = "interrupted"
)

// ReseedRunEndedEarly 报告这一轮是不是提前停下的（可以接着往下查）。
func ReseedRunEndedEarly(status string) bool {
	return status == ReseedRunStopped || status == ReseedRunLimited || status == ReseedRunInterrupted
}

// ReseedRun 是一轮辅种（定时、立即运行、继续或重试）：开始时写一行，跑完补上结果、计数与步骤。
// 保留最近 ReseedRunKeep 条且不超过 ReseedRunKeepFor。
type ReseedRun struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	Trigger    string     `gorm:"size:16;not null;default:''" json:"trigger"`
	StartedAt  time.Time  `gorm:"not null;index" json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Status     string     `gorm:"size:16;not null;default:'';index" json:"status"`
	Summary    string     `gorm:"size:1024;not null;default:''" json:"summary"`
	Error      string     `gorm:"size:1024;not null;default:''" json:"error"`
	// 计数：读到的下载器、查询的种子（重试轮次是选中的记录数）、IYUU 查询的批数与完成的批数、候选与各个结果
	Downloaders int `gorm:"not null;default:0" json:"downloaders"`
	Hashes      int `gorm:"not null;default:0" json:"hashes"`
	Batches     int `gorm:"not null;default:0" json:"batches"`
	BatchesDone int `gorm:"not null;default:0" json:"batches_done"`
	Candidates  int `gorm:"not null;default:0" json:"candidates"`
	Created     int `gorm:"not null;default:0" json:"created"`
	Failed      int `gorm:"not null;default:0" json:"failed"`
	Gone        int `gorm:"not null;default:0" json:"gone"`
	Interrupted int `gorm:"not null;default:0" json:"interrupted"`
	Skipped     int `gorm:"not null;default:0" json:"skipped"`
	// SkippedDetail 是跳过的原因与个数（JSON 对象）
	SkippedDetail string `gorm:"type:text;not null;default:''" json:"-"`
	IYUUCalls     int    `gorm:"column:iyuu_calls;not null;default:0" json:"iyuu_calls"`
	// After 是这一轮从哪个 info hash 之后开始查（空串是从头）；Cursor 是最后一个完整处理完的批的最后一个 info hash（一批都没处理完时为空）。
	After         string `gorm:"size:64;not null;default:''" json:"-"`
	Cursor        string `gorm:"size:64;not null;default:''" json:"-"`
	ContinuedFrom *uint  `json:"continued_from,omitempty"`
	// PausedAt 是这一次暂停的开始时间（暂停中才有）；PausedSeconds 是已经暂停过的总秒数
	PausedAt      *time.Time `json:"paused_at,omitempty"`
	PausedSeconds int        `gorm:"not null;default:0" json:"paused_seconds"`
	// Steps 是这一轮的步骤（ReseedRunStep 的 JSON 数组，最多 16 KiB；超出时后面的不记，StepsTruncated 为真）
	Steps          string `gorm:"type:text;not null;default:''" json:"-"`
	StepsTruncated bool   `gorm:"not null;default:false" json:"steps_truncated"`
}

// TableName 指定表名。
func (ReseedRun) TableName() string { return "reseed_runs" }

// 轮次记录的保留：最近 500 条、不超过 180 天。
const (
	ReseedRunKeep    = 500
	ReseedRunKeepFor = 180 * 24 * time.Hour
)

// ReseedRunStep 是一轮里的一步：Kind 是种类（list、query、add、fail、gone、stop、error、info），Text 给人看。
type ReseedRunStep struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// 辅种尝试的结局。
const (
	// ReseedAttemptRunning 是正在尝试（写入时）；pt-tools 退出时留下的，启动时改成 interrupted
	ReseedAttemptRunning = "running"
	// ReseedAttemptCreated 是建成了辅种任务（与任务在同一个事务里写）
	ReseedAttemptCreated = "created"
	// ReseedAttemptFailed 是站点不可用、下载失败、info hash 或文件列表对不上、建任务失败
	ReseedAttemptFailed = "failed"
	// ReseedAttemptGone 是站点上已经没有这个种子
	ReseedAttemptGone = "gone"
	// ReseedAttemptActive 是种子下载、核对过了，但这个 info hash 已有进行中的任务（统计里算跳过）
	ReseedAttemptActive = "active"
	// ReseedAttemptInterrupted 是这一轮被停止、超时或 pt-tools 退出打断了（不改辅种记录，以后照常再试）
	ReseedAttemptInterrupted = "interrupted"
)

// ReseedAttempt 是一次辅种尝试，只追加：开始下载种子文件之前写入（从这一刻起占今天这个站点的名额），结束时写结局。
// 每站每天的名额、轮次里的尝试与统计都按它算；删除或重试辅种记录不改它。
// JobState、JobFinishedAt 只在清除已结束的辅种任务时（同一个事务）从任务抄过来，任务还在时看任务表。
type ReseedAttempt struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	RunID      *uint      `gorm:"index" json:"run_id,omitempty"`
	StartedAt  time.Time  `gorm:"not null;index;index:idx_reseed_attempt_site_at,priority:2" json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	SiteName   string     `gorm:"size:64;not null;index:idx_reseed_attempt_site_at,priority:1" json:"site_name"`
	InfoHash   string     `gorm:"size:64;not null" json:"info_hash"`
	TorrentID  string     `gorm:"size:128;not null;default:''" json:"torrent_id"`
	Name       string     `gorm:"size:512;not null;default:''" json:"name"`
	// DownloaderID 是原种子所在、辅种要加入的下载器
	DownloaderID  uint       `gorm:"not null;default:0" json:"downloader_id"`
	Outcome       string     `gorm:"size:16;not null;index" json:"outcome"`
	Message       string     `gorm:"size:1024;not null;default:''" json:"message"`
	JobID         *uint      `gorm:"index" json:"job_id,omitempty"`
	JobState      string     `gorm:"size:32;not null;default:''" json:"job_state,omitempty"`
	JobFinishedAt *time.Time `gorm:"index" json:"job_finished_at,omitempty"`
	// Backfilled 是升级时按旧的辅种记录补的（v15）
	Backfilled bool `gorm:"not null;default:false;index" json:"backfilled"`
}

// TableName 指定表名。
func (ReseedAttempt) TableName() string { return "reseed_attempts" }

// ReseedAttemptKeepFor 是尝试行的保留时间（统计的自定义范围最长 5 年）。
const ReseedAttemptKeepFor = 5 * 366 * 24 * time.Hour
