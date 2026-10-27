package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 刷流种子的状态。active 之外都是终态。
const (
	BrushTorrentActive  = "active"
	BrushTorrentRemoved = "removed"
	// BrushTorrentGone 表示种子已经不在下载器里了（被人手动删掉，或被低空间紧急清理删掉），不是刷流删的。
	BrushTorrentGone = "gone"
)

// BrushTagAll 是所有刷流种子都带的标签；全局自动清理按它把刷流种子排除在常规规则之外。
const BrushTagAll = "pt-tools-brush"

// BrushTaskTag 是某个刷流任务专属的标签，删种只处理带这个标签、且有 BrushTorrent 记录的种子。
func BrushTaskTag(taskID uint) string {
	return fmt.Sprintf("pt-brush-%d", taskID)
}

// BrushTask 是一个刷流任务：从一个站点的免费列表里按入场条件挑种子推给一个下载器，再按删种规则删掉。
// 默认关闭。时长、体积的 0 一律表示「不限」。
// 列的默认值一律是零值：GORM 新建时会把零值换成 default，非零的默认值会让关掉的开关（如删种不删文件）存成打开；
// 新任务的默认取值在前端表单里。
type BrushTask struct {
	ID           uint   `gorm:"primaryKey" json:"id"`
	Name         string `gorm:"size:64;not null;uniqueIndex" json:"name"`
	Enabled      bool   `gorm:"not null;default:false" json:"enabled"`
	SiteName     string `gorm:"size:64;not null;index" json:"site_name"`
	DownloaderID uint   `gorm:"not null" json:"downloader_id"`
	SavePath     string `gorm:"size:512;default:''" json:"save_path"`
	Category     string `gorm:"size:128;default:''" json:"category"`
	// Tags 是用户额外加的标签（逗号分隔）；站点名、pt-tools-brush 和任务标签总会带上。
	Tags        string `gorm:"size:256;default:''" json:"tags"`
	IntervalMin int    `gorm:"not null;default:0" json:"interval_min"`
	// Section 是读哪个分区的种子列表，只对 M-Team：normal 综合区、adult 成人区、all 两个都读；空值等于 normal。
	// 别的站点保存为空。
	Section string `gorm:"size:16;default:''" json:"section"`

	// 入场条件
	// Discounts 是允许的优惠类型（逗号分隔的 DiscountLevel，如 FREE,2XFREE）；为空时只收免费（FREE、2XFREE）。
	Discounts        string  `gorm:"size:128;default:''" json:"discounts"`
	MinFreeRemainMin int     `gorm:"not null;default:0" json:"min_free_remain_min"`
	MinSizeGB        float64 `gorm:"not null;default:0" json:"min_size_gb"`
	MaxSizeGB        float64 `gorm:"not null;default:0" json:"max_size_gb"`
	MaxSeeders       int     `gorm:"not null;default:0" json:"max_seeders"`
	MinLeechers      int     `gorm:"not null;default:0" json:"min_leechers"`
	MaxPublishAgeMin int     `gorm:"not null;default:0" json:"max_publish_age_min"`
	ExcludeHR        bool    `gorm:"not null;default:false" json:"exclude_hr"`
	// IncludeKeywords / ExcludeKeywords 按行或逗号分隔，大小写不敏感地匹配标题与副标题。
	IncludeKeywords string `gorm:"size:1024;default:''" json:"include_keywords"`
	ExcludeKeywords string `gorm:"size:1024;default:''" json:"exclude_keywords"`

	// 限额
	MaxDownloading     int     `gorm:"not null;default:0" json:"max_downloading"`
	MaxTotalSizeGB     float64 `gorm:"not null;default:0" json:"max_total_size_gb"`
	MaxDailyDownloadGB float64 `gorm:"not null;default:0" json:"max_daily_download_gb"`

	// 删种规则（任一条满足即删）
	RemoveSeedTimeH float64 `gorm:"not null;default:0" json:"remove_seed_time_h"`
	RemoveRatio     float64 `gorm:"not null;default:0" json:"remove_ratio"`
	// RemoveLowSpeedKBs 与 RemoveLowSpeedWindowMin 一起用：已完成的种子最近 N 分钟的平均上传速度低于阈值时删。
	RemoveLowSpeedKBs           float64 `gorm:"not null;default:0" json:"remove_low_speed_kbs"`
	RemoveLowSpeedWindowMin     int     `gorm:"not null;default:0" json:"remove_low_speed_window_min"`
	RemoveInactiveH             float64 `gorm:"not null;default:0" json:"remove_inactive_h"`
	RemoveFreeExpiredIncomplete bool    `gorm:"not null;default:false" json:"remove_free_expired_incomplete"`
	RemoveWithData              bool    `gorm:"not null;default:false" json:"remove_with_data"`

	LastRunAt  *time.Time `json:"last_run_at,omitempty"`
	LastError  string     `gorm:"size:1024;default:''" json:"last_error"`
	LastResult string     `gorm:"size:512;default:''" json:"last_result"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// TagList 返回推送时给种子打的全部标签：站点名、pt-tools-brush、任务标签，再加用户标签（去重、保持顺序）。
func (t BrushTask) TagList() []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	add := func(tag string) {
		tag = strings.TrimSpace(tag)
		key := strings.ToLower(tag)
		if tag == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, tag)
	}
	add(t.SiteName)
	add(BrushTagAll)
	add(BrushTaskTag(t.ID))
	for tag := range strings.SplitSeq(t.Tags, ",") {
		add(tag)
	}
	return out
}

// BrushTorrent 是刷流任务推送过的一个种子；(task_id, info_hash) 唯一。
type BrushTorrent struct {
	ID       uint   `gorm:"primaryKey" json:"id"`
	TaskID   uint   `gorm:"not null;uniqueIndex:idx_brush_task_hash,priority:1;index" json:"task_id"`
	InfoHash string `gorm:"size:64;not null;uniqueIndex:idx_brush_task_hash,priority:2" json:"info_hash"`
	// (site_name, torrent_id) 唯一：同一个站点种子只由一个刷流任务加入一次（删掉之后也不再加回来）
	SiteName  string `gorm:"size:64;not null;uniqueIndex:idx_brush_site_torrent,priority:1" json:"site_name"`
	TorrentID string `gorm:"size:128;not null;uniqueIndex:idx_brush_site_torrent,priority:2" json:"torrent_id"`
	Title     string `gorm:"size:512;default:''" json:"title"`
	SizeBytes int64  `gorm:"not null;default:0" json:"size_bytes"`
	// Discount 是推送时的优惠类型（DiscountLevel）。
	Discount     string     `gorm:"size:16;default:''" json:"discount"`
	FreeEndAt    *time.Time `json:"free_end_at,omitempty"`
	HasHR        bool       `gorm:"not null;default:false" json:"has_hr"`
	HRSeedTimeH  int        `gorm:"not null;default:0" json:"hr_seed_time_h"`
	DownloaderID uint       `gorm:"not null" json:"downloader_id"`
	State        string     `gorm:"size:16;not null;default:'active';index" json:"state"`
	AddedAt      time.Time  `gorm:"not null" json:"added_at"`
	RemovedAt    *time.Time `json:"removed_at,omitempty"`
	RemoveReason string     `gorm:"size:256;default:''" json:"remove_reason"`

	// 最近一次采样时下载器报告的累计值，用于算每天的增量。
	Uploaded       int64      `gorm:"not null;default:0" json:"uploaded"`
	Downloaded     int64      `gorm:"not null;default:0" json:"downloaded"`
	Progress       float64    `gorm:"not null;default:0" json:"progress"`
	Ratio          float64    `gorm:"not null;default:0" json:"ratio"`
	SeedingTimeSec int64      `gorm:"not null;default:0" json:"seeding_time_sec"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`
	LastSampleAt   *time.Time `json:"last_sample_at,omitempty"`
}

// BrushTorrentSample 是一次采样：种子在下载器里的累计上传、下载量。只为算「最近 N 分钟的平均上传速度」保留，
// 超过最长窗口的旧行定期删掉。
type BrushTorrentSample struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	BrushTorrentID uint      `gorm:"not null;index:idx_brush_sample_torrent_at,priority:1" json:"brush_torrent_id"`
	At             time.Time `gorm:"not null;index:idx_brush_sample_torrent_at,priority:2;index" json:"at"`
	Uploaded       int64     `gorm:"not null" json:"uploaded"`
	Downloaded     int64     `gorm:"not null" json:"downloaded"`
}

// BrushDailyStat 是刷流任务每天的收益：(task_id, day) 唯一；上传、下载是当天采样差值的累加。
type BrushDailyStat struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	TaskID      uint   `gorm:"not null;uniqueIndex:idx_brush_stat_task_day,priority:1" json:"task_id"`
	Day         string `gorm:"size:10;not null;uniqueIndex:idx_brush_stat_task_day,priority:2;index" json:"day"`
	Uploaded    int64  `gorm:"not null;default:0" json:"uploaded"`
	Downloaded  int64  `gorm:"not null;default:0" json:"downloaded"`
	Added       int    `gorm:"not null;default:0" json:"added"`
	AddedBytes  int64  `gorm:"not null;default:0" json:"added_bytes"`
	Removed     int    `gorm:"not null;default:0" json:"removed"`
	LastUpdated time.Time
}

// 刷流运行的触发方式。
const (
	BrushRunSchedule = "schedule"
	BrushRunManual   = "manual"
)

// 刷流运行的结果。
const (
	BrushRunRunning = "running"
	BrushRunOK      = "ok"
	// BrushRunPartial 是这一轮跑完了，但有个别种子下载、推送、删除或记录失败
	BrushRunPartial = "partial"
	BrushRunFailed  = "failed"
	// BrushRunInterrupted 是这一轮没跑完进程就退出了（重启时补记）
	BrushRunInterrupted = "interrupted"
)

// BrushRun 是刷流任务的一轮运行（定时或手动「立即运行」）：先写一条进行中的记录，跑完再补上结果与步骤。
// 每个任务保留最近 500 条，且不超过 14 天（见 scheduler 的 brushRunKeep*）。
type BrushRun struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	TaskID     uint       `gorm:"not null;index" json:"task_id"`
	Trigger    string     `gorm:"size:16;not null;default:''" json:"trigger"`
	StartedAt  time.Time  `gorm:"not null;index" json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Status     string     `gorm:"size:16;not null;default:''" json:"status"`
	Summary    string     `gorm:"size:1024;not null;default:''" json:"summary"`
	Error      string     `gorm:"size:1024;not null;default:''" json:"error"`
	Listed     int        `gorm:"not null;default:0" json:"listed"`
	Eligible   int        `gorm:"not null;default:0" json:"eligible"`
	Added      int        `gorm:"not null;default:0" json:"added"`
	Removed    int        `gorm:"not null;default:0" json:"removed"`
	Gone       int        `gorm:"not null;default:0" json:"gone"`
	// Steps 是这一轮的步骤（BrushRunStep 的 JSON 数组，最多 16 KiB；超出时后面的不记，StepsTruncated 为真）
	Steps          string `gorm:"type:text;not null;default:''" json:"-"`
	StepsTruncated bool   `gorm:"not null;default:false" json:"steps_truncated"`
}

func (BrushRun) TableName() string { return "brush_runs" }

// BrushRunStep 是一轮运行里的一步：Kind 是种类（list、reject、add、remove、gone、skip、stop、error、info），Text 给人看。
type BrushRunStep struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// StepList 解出这一轮的步骤；没有记下或解不开时为空。
func (r BrushRun) StepList() []BrushRunStep {
	if r.Steps == "" {
		return []BrushRunStep{}
	}
	var steps []BrushRunStep
	if err := json.Unmarshal([]byte(r.Steps), &steps); err != nil || steps == nil {
		return []BrushRunStep{}
	}
	return steps
}

// BrushRepository 封装刷流相关表的读写。
type BrushRepository struct {
	db *gorm.DB
}

func NewBrushRepository(db *gorm.DB) *BrushRepository {
	return &BrushRepository{db: db}
}

// ErrBrushTaskNotFound 表示任务不存在。
var ErrBrushTaskNotFound = errors.New("刷流任务不存在")

func (r *BrushRepository) ListTasks() ([]BrushTask, error) {
	var tasks []BrushTask
	if err := r.db.Order("id ASC").Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("读取刷流任务失败: %w", err)
	}
	return tasks, nil
}

func (r *BrushRepository) GetTask(id uint) (*BrushTask, error) {
	var task BrushTask
	if err := r.db.First(&task, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBrushTaskNotFound
		}
		return nil, fmt.Errorf("读取刷流任务失败: %w", err)
	}
	return &task, nil
}

// SaveTask 新建（ID 为 0）或整行更新一个任务的配置；运行状态列（last_*）由 MarkRun 单独写，这里不覆盖。
func (r *BrushRepository) SaveTask(task *BrushTask) error {
	if task.ID == 0 {
		if err := r.db.Create(task).Error; err != nil {
			return fmt.Errorf("保存刷流任务失败: %w", err)
		}
		return nil
	}
	err := r.db.Model(&BrushTask{}).Where("id = ?", task.ID).
		Select("*").Omit("id", "created_at", "last_run_at", "last_error", "last_result").
		Updates(task).Error
	if err != nil {
		return fmt.Errorf("保存刷流任务失败: %w", err)
	}
	return nil
}

// DeleteTask 删除任务和它的统计、采样、种子与运行记录。下载器里的种子不动。
func (r *BrushRepository) DeleteTask(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var ids []uint
		if err := tx.Model(&BrushTorrent{}).Where("task_id = ?", id).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) > 0 {
			if err := tx.Where("brush_torrent_id IN ?", ids).Delete(&BrushTorrentSample{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("task_id = ?", id).Delete(&BrushTorrent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", id).Delete(&BrushDailyStat{}).Error; err != nil {
			return err
		}
		if err := tx.Where("task_id = ?", id).Delete(&BrushRun{}).Error; err != nil {
			return err
		}
		res := tx.Delete(&BrushTask{}, id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrBrushTaskNotFound
		}
		return nil
	})
}

// MarkRun 记下一轮运行的时间、结果摘要与错误（错误为空表示这一轮正常）。
func (r *BrushRepository) MarkRun(id uint, at time.Time, result, lastErr string) error {
	return r.db.Model(&BrushTask{}).Where("id = ?", id).Updates(map[string]any{
		"last_run_at": at.UTC(),
		"last_result": truncateRunes(result, 500),
		"last_error":  truncateRunes(lastErr, 1000),
	}).Error
}

// ActiveTorrents 返回任务名下仍在做的种子。
func (r *BrushRepository) ActiveTorrents(taskID uint) ([]BrushTorrent, error) {
	var rows []BrushTorrent
	if err := r.db.Where("task_id = ? AND state = ?", taskID, BrushTorrentActive).Order("id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取刷流种子失败: %w", err)
	}
	return rows, nil
}

// CountActive 返回任务名下仍在做的种子数。
func (r *BrushRepository) CountActive(taskID uint) (int64, error) {
	var n int64
	err := r.db.Model(&BrushTorrent{}).Where("task_id = ? AND state = ?", taskID, BrushTorrentActive).Count(&n).Error
	return n, err
}

// ListTorrents 分页列出任务的种子；state 为空时列全部，新的在前。
func (r *BrushRepository) ListTorrents(taskID uint, state string, page, pageSize int) ([]BrushTorrent, int64, error) {
	q := r.db.Model(&BrushTorrent{}).Where("task_id = ?", taskID)
	if state != "" {
		q = q.Where("state = ?", state)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	var rows []BrushTorrent
	if err := q.Order("added_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// SeenSiteTorrentIDs 返回所有刷流任务在这个站点推送过的种子 ID（不论哪个任务、现在什么状态）：
// 同一个站点种子不会被两个任务各加一份（两份会争用同一条 TorrentInfo 记录）。
func (r *BrushRepository) SeenSiteTorrentIDs(siteName string) (map[string]bool, error) {
	var ids []string
	if err := r.db.Model(&BrushTorrent{}).Where("site_name = ?", siteName).Pluck("torrent_id", &ids).Error; err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// ErrBrushTorrentTaken 表示这个站点种子已经由别的刷流任务加入过（site_name + torrent_id 唯一）。
var ErrBrushTorrentTaken = errors.New("这个种子已经由别的刷流任务加入")

// RecordAdded 记下一个推送成功的种子，并把当天的加入数与体积记进统计。同一任务同一 hash 已有记录时不重复记；
// 这个站点种子已经归别的任务时返回 ErrBrushTorrentTaken（调用方应撤回刚加的这一份）。
func (r *BrushRepository) RecordAdded(bt *BrushTorrent, day string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(bt)
		if res.Error != nil {
			return fmt.Errorf("记录刷流种子失败: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			var mine int64
			if err := tx.Model(&BrushTorrent{}).Where("task_id = ? AND info_hash = ?", bt.TaskID, bt.InfoHash).Count(&mine).Error; err != nil {
				return fmt.Errorf("记录刷流种子失败: %w", err)
			}
			if mine > 0 {
				return nil
			}
			return ErrBrushTorrentTaken
		}
		return addDailyStat(tx, bt.TaskID, day, BrushDailyStat{Added: 1, AddedBytes: bt.SizeBytes})
	})
}

// RecordSample 写一次采样，更新种子的最近状态，并把与上次采样的差值（只计正数）累加到每日统计里。
// 差值按时间比例分摊到它跨过的每一天（loc 是分天的时区）：23:55 与次日 00:05 之间的量不会全记到次日。
// 第一次采样从加入时刻算起：推送前下载器里没有这个种子，累计值本身就是这段时间的量。
func (r *BrushRepository) RecordSample(bt *BrushTorrent, sample BrushTorrentSample, progress, ratio float64, seedingSec int64, loc *time.Location) error {
	if loc == nil {
		loc = time.Local
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		up := sample.Uploaded - bt.Uploaded
		down := sample.Downloaded - bt.Downloaded
		from := bt.AddedAt
		if bt.LastSampleAt != nil {
			from = *bt.LastSampleAt
		} else {
			up, down = sample.Uploaded, sample.Downloaded
		}
		if up < 0 {
			up = 0
		}
		if down < 0 {
			down = 0
		}
		sample.BrushTorrentID = bt.ID
		sample.ID = 0
		if err := tx.Create(&sample).Error; err != nil {
			return fmt.Errorf("写刷流采样失败: %w", err)
		}
		at := sample.At.UTC()
		updates := map[string]any{
			"uploaded":         sample.Uploaded,
			"downloaded":       sample.Downloaded,
			"progress":         progress,
			"ratio":            ratio,
			"seeding_time_sec": seedingSec,
			"last_sample_at":   at,
		}
		if up > 0 || down > 0 || bt.LastActivityAt == nil {
			updates["last_activity_at"] = at
		}
		if err := tx.Model(&BrushTorrent{}).Where("id = ?", bt.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("更新刷流种子失败: %w", err)
		}
		bt.Uploaded, bt.Downloaded, bt.Progress, bt.Ratio, bt.SeedingTimeSec = sample.Uploaded, sample.Downloaded, progress, ratio, seedingSec
		bt.LastSampleAt = &at
		if v, ok := updates["last_activity_at"]; ok {
			t := v.(time.Time)
			bt.LastActivityAt = &t
		}
		if up == 0 && down == 0 {
			return nil
		}
		for _, part := range splitByDay(from, sample.At, up, down, loc) {
			if err := addDailyStat(tx, bt.TaskID, part.day, BrushDailyStat{Uploaded: part.up, Downloaded: part.down}); err != nil {
				return err
			}
		}
		return nil
	})
}

type dayShare struct {
	day      string
	up, down int64
}

// splitByDay 把 [from, to] 这段时间里的 up、down 按每天占的时长分摊（时区 loc）；to 不晚于 from 时整段记在 to 那天。
// 舍入的零头都归到最后一天，合计与输入相等。
func splitByDay(from, to time.Time, up, down int64, loc *time.Location) []dayShare {
	from, to = from.In(loc), to.In(loc)
	if !to.After(from) || from.Format(dateLayout) == to.Format(dateLayout) {
		return []dayShare{{day: to.Format(dateLayout), up: up, down: down}}
	}
	total := to.Sub(from).Seconds()
	var out []dayShare
	var usedUp, usedDown int64
	cur := from
	for cur.Before(to) {
		y, m, d := cur.Date()
		next := time.Date(y, m, d+1, 0, 0, 0, 0, loc)
		if next.After(to) {
			next = to
		}
		frac := next.Sub(cur).Seconds() / total
		share := dayShare{day: cur.Format(dateLayout), up: int64(float64(up) * frac), down: int64(float64(down) * frac)}
		usedUp += share.up
		usedDown += share.down
		out = append(out, share)
		cur = next
	}
	out[len(out)-1].up += up - usedUp
	out[len(out)-1].down += down - usedDown
	return out
}

const dateLayout = "2006-01-02"

// MarkEnded 把种子记为终态（removed 或 gone）；removed 时计入当天的删除数。
func (r *BrushRepository) MarkEnded(bt *BrushTorrent, state, reason string, at time.Time, day string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		at = at.UTC()
		res := tx.Model(&BrushTorrent{}).Where("id = ? AND state = ?", bt.ID, BrushTorrentActive).Updates(map[string]any{
			"state":         state,
			"removed_at":    at,
			"remove_reason": truncateRunes(reason, 250),
		})
		if res.Error != nil {
			return fmt.Errorf("更新刷流种子失败: %w", res.Error)
		}
		bt.State, bt.RemovedAt, bt.RemoveReason = state, &at, reason
		if res.RowsAffected == 0 || state != BrushTorrentRemoved {
			return nil
		}
		return addDailyStat(tx, bt.TaskID, day, BrushDailyStat{Removed: 1})
	})
}

// SamplesSince 返回种子在 since 及之后的采样（按时间升序），外加 since 之前最近的一条（作为窗口起点）。
func (r *BrushRepository) SamplesSince(brushTorrentID uint, since time.Time) ([]BrushTorrentSample, error) {
	var before BrushTorrentSample
	err := r.db.Where("brush_torrent_id = ? AND at < ?", brushTorrentID, since.UTC()).Order("at DESC").First(&before).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var rows []BrushTorrentSample
	if err := r.db.Where("brush_torrent_id = ? AND at >= ?", brushTorrentID, since.UTC()).Order("at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	if before.ID != 0 {
		rows = append([]BrushTorrentSample{before}, rows...)
	}
	return rows, nil
}

// PruneSamples 删掉 before 之前的采样；返回删掉的行数。
func (r *BrushRepository) PruneSamples(before time.Time) (int64, error) {
	res := r.db.Where("at < ?", before.UTC()).Delete(&BrushTorrentSample{})
	return res.RowsAffected, res.Error
}

// StartRun 写一条进行中的运行记录（进程中途退出时也看得到这一轮开始过）。
func (r *BrushRepository) StartRun(taskID uint, trigger string, at time.Time) (*BrushRun, error) {
	run := &BrushRun{TaskID: taskID, Trigger: trigger, StartedAt: at.UTC(), Status: BrushRunRunning}
	if err := r.db.Create(run).Error; err != nil {
		return nil, fmt.Errorf("记录刷流运行失败: %w", err)
	}
	return run, nil
}

// FinishRun 补上一轮运行的结果、计数与步骤。
func (r *BrushRepository) FinishRun(run *BrushRun) error {
	var finished *time.Time
	if run.FinishedAt != nil {
		at := run.FinishedAt.UTC()
		finished = &at
	}
	err := r.db.Model(&BrushRun{}).Where("id = ?", run.ID).Updates(map[string]any{
		"finished_at":     finished,
		"status":          run.Status,
		"summary":         truncateRunes(run.Summary, 1000),
		"error":           truncateRunes(run.Error, 1000),
		"listed":          run.Listed,
		"eligible":        run.Eligible,
		"added":           run.Added,
		"removed":         run.Removed,
		"gone":            run.Gone,
		"steps":           run.Steps,
		"steps_truncated": run.StepsTruncated,
	}).Error
	if err != nil {
		return fmt.Errorf("记录刷流运行结果失败: %w", err)
	}
	return nil
}

// ListRuns 分页列出任务的运行记录，新的在前；同时给出总条数。
func (r *BrushRepository) ListRuns(taskID uint, limit, offset int) ([]BrushRun, int64, error) {
	q := r.db.Model(&BrushRun{}).Where("task_id = ?", taskID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("读取刷流运行记录失败: %w", err)
	}
	var rows []BrushRun
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("读取刷流运行记录失败: %w", err)
	}
	return rows, total, nil
}

// MarkInterruptedRuns 把 before 以前开始、还写着「进行中」的运行记成没跑完（启动时调用：上一个进程在一轮中途退出了）。
// before 取这个进程的刷流监控建好的时刻，这个进程里刚开始的一轮不受影响。
func (r *BrushRepository) MarkInterruptedRuns(before, at time.Time) (int64, error) {
	res := r.db.Model(&BrushRun{}).Where("status = ? AND julianday(started_at) < julianday(?)", BrushRunRunning, before.UTC()).Updates(map[string]any{
		"status":      BrushRunInterrupted,
		"finished_at": at.UTC(),
		"error":       "pt-tools 在这一轮中途退出了，这一轮没有跑完",
	})
	return res.RowsAffected, res.Error
}

// PruneRuns 删掉 before 以前开始的运行记录，并且每个任务只留最新的 keepPerTask 条；返回删掉的行数。
// 时间按时刻比较（julianday），不按文本，存的时区不同也不会差几个小时。
func (r *BrushRepository) PruneRuns(before time.Time, keepPerTask int) (int64, error) {
	res := r.db.Where("julianday(started_at) < julianday(?)", before.UTC()).Delete(&BrushRun{})
	if res.Error != nil {
		return 0, fmt.Errorf("清理刷流运行记录失败: %w", res.Error)
	}
	removed := res.RowsAffected
	if keepPerTask <= 0 {
		return removed, nil
	}
	var over []struct {
		TaskID uint
		N      int64
	}
	if err := r.db.Model(&BrushRun{}).Select("task_id, COUNT(*) AS n").Group("task_id").
		Having("COUNT(*) > ?", keepPerTask).Scan(&over).Error; err != nil {
		return removed, fmt.Errorf("清理刷流运行记录失败: %w", err)
	}
	for _, o := range over {
		// 第 keepPerTask+1 新的那一条及更旧的都删（编号按开始顺序递增）
		var cut BrushRun
		if err := r.db.Where("task_id = ?", o.TaskID).Order("id DESC").Offset(keepPerTask).Limit(1).Take(&cut).Error; err != nil {
			return removed, fmt.Errorf("清理刷流运行记录失败: %w", err)
		}
		res := r.db.Where("task_id = ? AND id <= ?", o.TaskID, cut.ID).Delete(&BrushRun{})
		if res.Error != nil {
			return removed, fmt.Errorf("清理刷流运行记录失败: %w", res.Error)
		}
		removed += res.RowsAffected
	}
	return removed, nil
}

// DailyStats 返回 [fromDay, toDay] 的每日统计；taskID 为 0 时返回全部任务。
func (r *BrushRepository) DailyStats(taskID uint, fromDay, toDay string) ([]BrushDailyStat, error) {
	q := r.db.Where("day >= ? AND day <= ?", fromDay, toDay)
	if taskID != 0 {
		q = q.Where("task_id = ?", taskID)
	}
	var rows []BrushDailyStat
	if err := q.Order("day ASC, task_id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取刷流统计失败: %w", err)
	}
	return rows, nil
}

// TaskTotals 返回每个任务累计的上传与下载（全部日期）。
func (r *BrushRepository) TaskTotals() (map[uint]BrushDailyStat, error) {
	var rows []struct {
		TaskID     uint
		Uploaded   int64
		Downloaded int64
		Added      int
		Removed    int
	}
	if err := r.db.Model(&BrushDailyStat{}).
		Select("task_id, SUM(uploaded) AS uploaded, SUM(downloaded) AS downloaded, SUM(added) AS added, SUM(removed) AS removed").
		Group("task_id").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("汇总刷流统计失败: %w", err)
	}
	out := make(map[uint]BrushDailyStat, len(rows))
	for _, row := range rows {
		out[row.TaskID] = BrushDailyStat{TaskID: row.TaskID, Uploaded: row.Uploaded, Downloaded: row.Downloaded, Added: row.Added, Removed: row.Removed}
	}
	return out, nil
}

// addDailyStat 把增量累加到 (task, day) 那一行，没有就建。
func addDailyStat(tx *gorm.DB, taskID uint, day string, d BrushDailyStat) error {
	row := BrushDailyStat{TaskID: taskID, Day: day, LastUpdated: time.Now().UTC()}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "task_id"}, {Name: "day"}},
		DoNothing: true,
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("初始化刷流统计失败: %w", err)
	}
	err := tx.Model(&BrushDailyStat{}).Where("task_id = ? AND day = ?", taskID, day).Updates(map[string]any{
		"uploaded":     gorm.Expr("uploaded + ?", d.Uploaded),
		"downloaded":   gorm.Expr("downloaded + ?", d.Downloaded),
		"added":        gorm.Expr("added + ?", d.Added),
		"added_bytes":  gorm.Expr("added_bytes + ?", d.AddedBytes),
		"removed":      gorm.Expr("removed + ?", d.Removed),
		"last_updated": time.Now().UTC(),
	}).Error
	if err != nil {
		return fmt.Errorf("更新刷流统计失败: %w", err)
	}
	return nil
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
