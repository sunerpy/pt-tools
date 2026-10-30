package reseed

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
)

// ---------- 记录 ----------

// 记录的结果（页面上的分组；列表按它筛选）。记录自己的状态说了算的（失败、站点已删除，以及清除任务时写进记录的结果）直接用；
// 建了任务的看任务现在的状态。
const (
	// RecordActive 是任务还没结束（等待加入、加入中、校验中）
	RecordActive = "active"
	// RecordDone 是已辅种（校验通过、开始做种）
	RecordDone = "done"
	// RecordFailed 是失败：下载或核对没通过，或者任务失败
	RecordFailed = "failed"
	// RecordRolledBack 是已撤回（校验没到 100%，撤掉了加进去的种子）
	RecordRolledBack = "rolled_back"
	// RecordCanceled 是任务被取消
	RecordCanceled = "canceled"
	// RecordGone 是站点上已经没有这个种子
	RecordGone = "gone"
	// RecordMissing 是记录指向的任务已经不在了（被别的途径删掉，结果不知道）
	RecordMissing = "missing"
)

// recordOwnStates 是记录自己的状态就决定了结果、不用看任务的那几种。
var recordOwnStates = []string{models.ReseedFailed, models.ReseedGone, models.ReseedDone, models.ReseedRolledBack, models.ReseedCanceled}

// finalJobStates 是辅种任务结束时的状态。
var finalJobStates = []string{models.TransferDone, models.TransferRolledBack, models.TransferFailed, models.TransferCanceled}

// 一次最多重试、删除几条记录。
const (
	MaxRetryRecords  = 100
	MaxDeleteRecords = 500
)

var (
	// ErrRecordNotFound 表示辅种记录不存在。
	ErrRecordNotFound = errors.New("辅种记录不存在")
	// ErrNoPendingJob 表示记录没有还在等待加入下载器的任务（只有这种任务能取消）。
	ErrNoPendingJob = errors.New("这条记录没有等待加入下载器的任务")
	// ErrNothingToRetry 表示选中的记录都不能重试。
	ErrNothingToRetry = fmt.Errorf("%w: 选中的记录都不能重试：只有失败、站点已删除、已撤回、已取消和任务已删除的记录可以重试", ErrInvalid)
)

// 重试时跳过的原因（另见 run.go 里的跳过原因）。
const (
	SkipRecordMissing = "记录已经不在了"
	SkipNotRetryable  = "结果已经变了，不用重试"
	SkipNoDownloader  = "下载器不可用"
)

// RecordRetryable 报告这个结果的记录能不能重试。
func RecordRetryable(result string) bool {
	switch result {
	case RecordFailed, RecordGone, RecordRolledBack, RecordCanceled, RecordMissing:
		return true
	}
	return false
}

// recordResult 是一条记录在页面上的结果；与 whereRecordResult 的 SQL 是同一个规则。
func recordResult(state string, jobExists bool, jobState string) string {
	switch state {
	case models.ReseedFailed:
		return RecordFailed
	case models.ReseedGone:
		return RecordGone
	case models.ReseedDone:
		return RecordDone
	case models.ReseedRolledBack:
		return RecordRolledBack
	case models.ReseedCanceled:
		return RecordCanceled
	}
	if !jobExists {
		return RecordMissing
	}
	switch jobState {
	case models.TransferDone:
		return RecordDone
	case models.TransferRolledBack:
		return RecordRolledBack
	case models.TransferFailed:
		return RecordFailed
	case models.TransferCanceled:
		return RecordCanceled
	}
	return RecordActive
}

// whereRecordResult 按结果筛选（查询里记录是 rr、任务是 LEFT JOIN 的 j）；不认识的结果返回 false。
func whereRecordResult(q *gorm.DB, result string) (*gorm.DB, bool) {
	byJob := func(own, job string) *gorm.DB {
		return q.Where("(rr.state = ? OR (rr.state NOT IN ? AND j.id IS NOT NULL AND j.state = ?))", own, recordOwnStates, job)
	}
	switch result {
	case "":
		return q, true
	case RecordGone:
		return q.Where("rr.state = ?", models.ReseedGone), true
	case RecordDone:
		return byJob(models.ReseedDone, models.TransferDone), true
	case RecordFailed:
		return byJob(models.ReseedFailed, models.TransferFailed), true
	case RecordRolledBack:
		return byJob(models.ReseedRolledBack, models.TransferRolledBack), true
	case RecordCanceled:
		return byJob(models.ReseedCanceled, models.TransferCanceled), true
	case RecordMissing:
		return q.Where("rr.state NOT IN ? AND j.id IS NULL", recordOwnStates), true
	case RecordActive:
		return q.Where("rr.state NOT IN ? AND j.id IS NOT NULL AND j.state NOT IN ?", recordOwnStates, finalJobStates), true
	}
	return q, false
}

// RecordView 是记录列表里的一项：记录加上结果与任务现在的状态。JobMissing 表示记录指向的任务已经不在了（结果不知道）。
type RecordView struct {
	models.ReseedRecord
	Result     string `json:"result"`
	JobState   string `json:"job_state,omitempty"`
	JobMessage string `json:"job_message,omitempty"`
	JobMissing bool   `json:"job_missing,omitempty"`
}

// recordRow 是记录与 LEFT JOIN 的任务读出来的一行。
type recordRow struct {
	models.ReseedRecord
	JobRowID   *uint   `gorm:"column:job_row_id"`
	JobState   *string `gorm:"column:job_state"`
	JobMessage *string `gorm:"column:job_message"`
}

func (r recordRow) view() RecordView {
	v := RecordView{ReseedRecord: r.ReseedRecord}
	if r.JobState != nil {
		v.JobState = *r.JobState
	}
	if r.JobMessage != nil {
		v.JobMessage = *r.JobMessage
	}
	v.Result = recordResult(r.State, r.JobRowID != nil, v.JobState)
	v.JobMissing = v.Result == RecordMissing
	return v
}

func (s *Service) recordQuery(ctx context.Context) *gorm.DB {
	return s.cfg.DB.WithContext(ctx).Table("reseed_records AS rr").
		Joins("LEFT JOIN torrent_transfer_jobs j ON j.id = rr.job_id")
}

// RecordFilter 是记录列表的筛选，都可以为空：Result 是上面的结果之一，Query 匹配种子名或 info hash。
type RecordFilter struct {
	Result string
	Site   string
	Query  string
}

// RecordPage 是一页记录。Sites 是记录里出现过的站点（筛选用）。
type RecordPage struct {
	Items    []RecordView `json:"items"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Sites    []string     `json:"sites"`
}

// Records 按最后一次尝试的时间从新到旧列出辅种记录（重试过的按最后一次尝试的时间排）。
func (s *Service) Records(ctx context.Context, f RecordFilter, page, size int) (RecordPage, error) {
	page, size = pageArgs(page, size)
	q, ok := whereRecordResult(s.recordQuery(ctx), strings.TrimSpace(f.Result))
	if !ok {
		return RecordPage{}, fmt.Errorf("%w: 不认识的结果 %q", ErrInvalid, f.Result)
	}
	if site := strings.TrimSpace(f.Site); site != "" {
		q = q.Where("rr.site_name = ?", site)
	}
	if kw := strings.TrimSpace(f.Query); kw != "" {
		if len(kw) > 200 {
			return RecordPage{}, fmt.Errorf("%w: 关键字太长", ErrInvalid)
		}
		like := "%" + strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(kw) + "%"
		q = q.Where(`(rr.name LIKE ? ESCAPE '\' OR rr.info_hash LIKE ? ESCAPE '\')`, like, strings.ToLower(like))
	}
	out := RecordPage{Items: []RecordView{}, Page: page, PageSize: size, Sites: []string{}}
	if err := q.Count(&out.Total).Error; err != nil {
		return RecordPage{}, fmt.Errorf("读取辅种记录失败: %w", err)
	}
	var rows []recordRow
	if err := q.Select("rr.*, j.id AS job_row_id, j.state AS job_state, j.message AS job_message").
		Order("rr.updated_at DESC, rr.id DESC").Offset((page - 1) * size).Limit(size).Scan(&rows).Error; err != nil {
		return RecordPage{}, fmt.Errorf("读取辅种记录失败: %w", err)
	}
	for _, r := range rows {
		out.Items = append(out.Items, r.view())
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.ReseedRecord{}).Distinct("site_name").
		Order("site_name").Pluck("site_name", &out.Sites).Error; err != nil {
		return RecordPage{}, fmt.Errorf("读取辅种记录的站点失败: %w", err)
	}
	return out, nil
}

// record 读出一条记录与它的结果。
func (s *Service) record(ctx context.Context, id uint) (RecordView, error) {
	var rows []recordRow
	if err := s.recordQuery(ctx).Select("rr.*, j.id AS job_row_id, j.state AS job_state, j.message AS job_message").
		Where("rr.id = ?", id).Limit(1).Scan(&rows).Error; err != nil {
		return RecordView{}, fmt.Errorf("读取辅种记录失败: %w", err)
	}
	if len(rows) == 0 {
		return RecordView{}, ErrRecordNotFound
	}
	return rows[0].view(), nil
}

// DeleteResult 是删除记录的结果：Active 是任务还没结束、没有删的条数。
type DeleteResult struct {
	Deleted int64 `json:"deleted"`
	Active  int64 `json:"active"`
}

// DeleteRecords 删除记录；任务还没结束的不删。删除以后这个站点的这个种子在以后的轮次里会再试一次；
// 尝试行不动（今天的名额、统计都不变）。判断与删除在同一条语句里，不会删掉刚建了任务的记录。
func (s *Service) DeleteRecords(ctx context.Context, ids []uint) (DeleteResult, error) {
	ids = dedupeUints(ids)
	if len(ids) == 0 {
		return DeleteResult{}, fmt.Errorf("%w: 没有选中记录", ErrInvalid)
	}
	if len(ids) > MaxDeleteRecords {
		return DeleteResult{}, fmt.Errorf("%w: 一次最多删除 %d 条", ErrInvalid, MaxDeleteRecords)
	}
	var out DeleteResult
	err := s.cfg.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var found int64
		if err := tx.Model(&models.ReseedRecord{}).Where("id IN ?", ids).Count(&found).Error; err != nil {
			return err
		}
		active := tx.Model(&models.TorrentTransferJob{}).Select("id").Where("state NOT IN ?", finalJobStates)
		res := tx.Where("id IN ?", ids).
			Where("(state IN ? OR job_id IS NULL OR job_id NOT IN (?))", recordOwnStates, active).
			Delete(&models.ReseedRecord{})
		if res.Error != nil {
			return res.Error
		}
		out.Deleted, out.Active = res.RowsAffected, found-res.RowsAffected
		return nil
	})
	if err != nil {
		return DeleteResult{}, fmt.Errorf("删除辅种记录失败: %w", err)
	}
	return out, nil
}

// CancelRecordJob 取消记录对应的、还在等待加入下载器的任务；已经加入的不能取消（transfer.ErrNotCancelable）。
func (s *Service) CancelRecordJob(ctx context.Context, id uint) error {
	v, err := s.record(ctx, id)
	if err != nil {
		return err
	}
	if v.Result != RecordActive || v.JobID == nil {
		return ErrNoPendingJob
	}
	err = s.cfg.Transfer.Cancel(ctx, *v.JobID)
	if errors.Is(err, transfer.ErrJobNotFound) {
		return ErrNoPendingJob
	}
	return err
}

// CheckRetry 检查选中的记录里有没有能重试的（重试轮次开始以前调用）。
func (s *Service) CheckRetry(ctx context.Context, ids []uint) error {
	ids = dedupeUints(ids)
	if len(ids) == 0 {
		return fmt.Errorf("%w: 没有选中记录", ErrInvalid)
	}
	if len(ids) > MaxRetryRecords {
		return fmt.Errorf("%w: 一次最多重试 %d 条", ErrInvalid, MaxRetryRecords)
	}
	var rows []recordRow
	if err := s.recordQuery(ctx).Select("rr.*, j.id AS job_row_id, j.state AS job_state, j.message AS job_message").
		Where("rr.id IN ?", ids).Scan(&rows).Error; err != nil {
		return fmt.Errorf("读取辅种记录失败: %w", err)
	}
	for _, r := range rows {
		if RecordRetryable(r.view().Result) {
			return nil
		}
	}
	return ErrNothingToRetry
}

// retryDownloader 是重试轮次里读过的一台下载器（每台只读一次）。
type retryDownloader struct {
	sources map[string]source
	present map[string]bool
}

// Retry 是重试轮次：重试选中的记录。不查 IYUU，用记录里的站点、站内 ID 与 info hash；在记录的下载器里找原种子，
// 目标里已经有这个种子或者站点今天的名额用完时跳过，其余走与普通轮次同一个尝试（写尝试行、占名额、更新记录）。
// 不看站点开关与「已经尝试过」：重试是在记录里点名要试的。
func (s *Service) Retry(ctx context.Context, opts RunOptions, ids []uint) (RunResult, error) {
	res := RunResult{Retry: true, log: &runLog{}}
	ids = dedupeUints(ids)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	res.Hashes = len(ids)
	r, err := s.row(ctx)
	if err != nil {
		return res, err
	}
	p := &pass{opts: opts, res: &res, limit: dailyLimit(r)}
	if p.today, err = s.todayCounts(ctx); err != nil {
		return res, err
	}
	res.log.add(stepInfo, "重试 %d 条记录", len(ids))
	p.progress()
	dls := map[uint]*retryDownloader{}
	for _, id := range ids {
		if p.wait(ctx) != nil {
			res.endWith(ctx)
			break
		}
		v, rerr := s.record(ctx, id)
		if errors.Is(rerr, ErrRecordNotFound) {
			res.skip(SkipRecordMissing)
			res.log.add(stepSkip, "记录 %d：%s", id, SkipRecordMissing)
			continue
		}
		if rerr != nil {
			res.addError(rerr.Error())
			continue
		}
		label := fmt.Sprintf("%s（%s %s）", v.Name, v.SiteName, v.TorrentID)
		skip := func(why string) {
			res.skip(why)
			res.log.add(stepSkip, "%s：%s", label, why)
		}
		if !RecordRetryable(v.Result) {
			skip(SkipNotRetryable)
			continue
		}
		res.Candidates++
		d := s.retryDownloader(ctx, dls, v.DownloaderID, &res)
		if d == nil {
			skip(SkipNoDownloader)
			continue
		}
		hash := strings.ToLower(v.InfoHash)
		src, ok := d.sources[strings.ToLower(v.SourceHash)]
		switch {
		case !ok:
			skip(SkipNoSource)
			continue
		case d.present[hash]:
			skip(SkipExists)
			continue
		case p.today[v.SiteName] >= p.limit:
			skip(SkipDailyLimit)
			continue
		}
		d.present[hash] = true
		s.attempt(ctx, p, src, iyuu.Candidate{InfoHash: hash, TorrentID: v.TorrentID}, v.SiteName)
		p.progress()
	}
	return res, nil
}

// retryDownloader 读出一台下载器里的种子（每轮每台只读一次）；读不到时返回 nil，并在结果里记一次错误。
func (s *Service) retryDownloader(ctx context.Context, cache map[uint]*retryDownloader, id uint, res *RunResult) *retryDownloader {
	if d, ok := cache[id]; ok {
		return d
	}
	cache[id] = nil
	dl, set, err := s.downloader(ctx, id)
	if err != nil {
		res.addError(fmt.Sprintf("下载器 %d 不可用: %v", id, err))
		return nil
	}
	torrents, err := dl.GetAllTorrents()
	if err != nil {
		res.addError(fmt.Sprintf("读取 %s 的种子失败: %v", set.Name, err))
		return nil
	}
	d := &retryDownloader{sources: map[string]source{}, present: map[string]bool{}}
	done := addTorrents(id, dl, torrents, d.sources, d.present)
	res.Downloaders++
	res.log.add(stepList, "读取下载器 %s：%d 个种子，%d 个已经下完", set.Name, len(torrents), done)
	cache[id] = d
	return d
}

// ClearFinishedJobs 删除已经结束的辅种任务，返回删除的条数。删除前把任务的结果写进引用它的辅种记录
// （记录不再指向任务；不改记录的更新时间，重试间隔按它算），并把结果与完成时间抄进建成它的尝试行（统计按它算）。
// 都在同一个事务里：不会出现任务删了、结果没抄上。
func (s *Service) ClearFinishedJobs(ctx context.Context) (int64, error) {
	var n int64
	err := s.cfg.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var jobs []models.TorrentTransferJob
		if err := tx.Omit("torrent_data").Where("kind = ? AND state IN ?", models.JobKindReseed, finalJobStates).
			Find(&jobs).Error; err != nil {
			return err
		}
		if len(jobs) == 0 {
			return nil
		}
		ids := make([]uint, 0, len(jobs))
		for _, j := range jobs {
			ids = append(ids, j.ID)
			state, msg := jobRecordState(j)
			if err := tx.Model(&models.ReseedRecord{}).Where("job_id = ?", j.ID).
				UpdateColumns(map[string]any{"state": state, "message": msg, "job_id": nil}).Error; err != nil {
				return err
			}
			finished := j.FinishedAt
			if finished == nil {
				at := j.UpdatedAt
				finished = &at
			}
			if err := tx.Model(&models.ReseedAttempt{}).Where("job_id = ? AND job_state = ''", j.ID).
				UpdateColumns(map[string]any{"job_state": j.State, "job_finished_at": finished}).Error; err != nil {
				return err
			}
		}
		res := tx.Where("id IN ? AND state IN ?", ids, finalJobStates).Delete(&models.TorrentTransferJob{})
		n = res.RowsAffected
		return res.Error
	})
	if err != nil {
		return 0, fmt.Errorf("清除已结束的辅种任务失败: %w", err)
	}
	return n, nil
}

// jobRecordState 是结束的辅种任务写进记录里的结果和说明。
func jobRecordState(j models.TorrentTransferJob) (string, string) {
	switch j.State {
	case models.TransferDone:
		return models.ReseedDone, j.Message
	case models.TransferRolledBack:
		return models.ReseedRolledBack, j.Message
	case models.TransferCanceled:
		return models.ReseedCanceled, j.Message
	default:
		return models.ReseedFailed, j.Message
	}
}
