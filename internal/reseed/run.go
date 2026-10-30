package reseed

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/internal/transfer"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// ---------- 运行 ----------

// RunResult 是一轮辅种的结果。
type RunResult struct {
	Downloaders int `json:"downloaders"`
	// Hashes 是这一轮要查询的种子数（接着上一轮查时只算后面的那些；重试轮次是选中的记录数）
	Hashes int `json:"hashes"`
	// Batches 是要查询 IYUU 的批数，BatchesDone 是查完并处理完的批数（重试轮次没有批）
	Batches     int `json:"batches"`
	BatchesDone int `json:"batches_done"`
	Candidates  int `json:"candidates"`
	Created     int `json:"created"`
	Failed      int `json:"failed"`
	// Gone 是站点上已经没有的种子（不算失败，以后不再试）
	Gone int `json:"gone"`
	// Interrupted 是被停止、超时或退出打断的尝试（不记失败，以后照常再试）
	Interrupted int `json:"interrupted"`
	// IYUUCalls 是这一轮请求 IYUU 的次数
	IYUUCalls int            `json:"iyuu_calls"`
	Skipped   map[string]int `json:"skipped,omitempty"`
	Errors    []string       `json:"errors,omitempty"`
	// Stopped 说明这一轮为什么提前停下（限流、被停止等）。
	Stopped string `json:"stopped,omitempty"`
	// End 是提前停下的种类（EndStopped、EndLimited、EndShutdown），跑完时为空。
	End string `json:"end,omitempty"`
	// Cursor 是最后一个完整处理完的批的最后一个 info hash（接着查时从它之后开始）。
	Cursor string `json:"-"`
	// Retry 表示这是重试轮次（汇总的写法不同）。
	Retry bool `json:"-"`

	log *runLog
	// daySkips 是按站点记的、下载以前就跳过的个数（这一轮结束时写进 reseed_daily_skips）
	daySkips map[string]*models.ReseedDailySkip
}

// 提前停下的种类。
const (
	// EndStopped 是被停止（手动停止、暂停太久、运行中换了 token）
	EndStopped = "stopped"
	// EndLimited 是超时、IYUU 限流或到了每天的请求上限
	EndLimited = "limited"
	// EndShutdown 是 pt-tools 正在退出
	EndShutdown = "shutdown"
)

// 跳过的原因。
const (
	SkipNotSelected = "站点不在辅种范围"
	SkipExists      = "下载器里已经有"
	SkipTried       = "已经尝试过"
	SkipDailyLimit  = "今天这个站点已经加满"
	SkipActive      = "已有进行中的任务"
	// SkipNoSource 是重试时原种子已经不在下载器里了（或者还没下完）
	SkipNoSource = "原种子不在下载器里"
)

func (r *RunResult) skip(why string) {
	if r.Skipped == nil {
		r.Skipped = map[string]int{}
	}
	r.Skipped[why]++
}

// skipAt 记一个下载以前就跳过的候选，并按站点计进每天的跳过统计（site 为空串是不在辅种范围的 IYUU 站点）。
func (r *RunResult) skipAt(site, why string) {
	r.skip(why)
	if r.daySkips == nil {
		r.daySkips = map[string]*models.ReseedDailySkip{}
	}
	c := r.daySkips[site]
	if c == nil {
		c = &models.ReseedDailySkip{SiteName: site}
		r.daySkips[site] = c
	}
	switch why {
	case SkipExists:
		c.Present++
	case SkipTried:
		c.Tried++
	case SkipDailyLimit:
		c.DailyLimit++
	case SkipNotSelected:
		c.NotSelected++
	}
}

func (r *RunResult) addError(text string) {
	r.Errors = append(r.Errors, text)
	r.log.add(stepError, "%s", text)
}

// endWith 按这一轮 ctx 的 cause 记下为什么提前停下。
func (r *RunResult) endWith(ctx context.Context) {
	cause := context.Cause(ctx)
	switch {
	case errors.Is(cause, ErrRunStopped), errors.Is(cause, ErrPausedTooLong):
		r.End, r.Stopped = EndStopped, cause.Error()
	case errors.Is(cause, ErrRunTimeout), errors.Is(cause, context.DeadlineExceeded):
		r.End, r.Stopped = EndLimited, ErrRunTimeout.Error()
	default:
		r.End, r.Stopped = EndShutdown, "pt-tools 正在退出，这一轮没有跑完"
	}
	r.log.add(stepStop, "%s", r.Stopped)
}

// Summary 是写进设置 last_result 与轮次记录的一句话。
func (r RunResult) Summary() string {
	var parts []string
	if r.Retry {
		parts = append(parts, fmt.Sprintf("重试 %d 条记录，加入 %d 个", r.Hashes, r.Created))
	} else {
		parts = append(parts, fmt.Sprintf("查询 %d 个种子，找到 %d 个可辅种，加入 %d 个", r.Hashes, r.Candidates, r.Created))
	}
	if r.Failed > 0 {
		parts = append(parts, fmt.Sprintf("失败 %d 个", r.Failed))
	}
	if r.Gone > 0 {
		parts = append(parts, fmt.Sprintf("站点已删除 %d 个", r.Gone))
	}
	if n := r.SkippedTotal(); n > 0 {
		parts = append(parts, fmt.Sprintf("跳过 %d 个", n))
	}
	if r.Interrupted > 0 {
		parts = append(parts, fmt.Sprintf("被打断 %d 个", r.Interrupted))
	}
	if r.Stopped != "" {
		parts = append(parts, r.Stopped)
	}
	if len(r.Errors) > 0 {
		parts = append(parts, r.Errors[0])
	}
	return strings.Join(parts, "，")
}

// SkippedTotal 是跳过的总数。
func (r RunResult) SkippedTotal() int {
	n := 0
	for _, v := range r.Skipped {
		n += v
	}
	return n
}

// snapshot 是给页面看实时进度的副本（不和这一轮的 goroutine 共用 map 与切片）。
func (r RunResult) snapshot() RunResult {
	out := r
	out.Skipped = maps.Clone(r.Skipped)
	out.Errors = append([]string(nil), r.Errors...)
	out.log, out.daySkips = nil, nil
	return out
}

// source 是下载器里一个已经下完的种子。
type source struct {
	dlID    uint
	dl      downloader.Downloader
	torrent downloader.Torrent
}

// RunOptions 是一轮的选项。
type RunOptions struct {
	// RunID 是这一轮的轮次记录：尝试记在它名下、每查完一批写一次进度；0 时不记（单测直接调 Run）。
	RunID uint
	// After 是从哪个 info hash 之后开始查（接着上一轮查）；空串是从头。
	After string
	// Control 在检查点上暂停这一轮；nil 时不暂停。
	Control *Control
	// Progress 在每查完一批、每个尝试以后收到结果的副本（页面上的实时进度）。
	Progress func(RunResult)
}

// pass 是一轮里几个函数共用的状态（只在这一轮的 goroutine 里用）。
type pass struct {
	opts  RunOptions
	res   *RunResult
	today map[string]int
	limit int
}

func (p *pass) runID() *uint {
	if p.opts.RunID == 0 {
		return nil
	}
	id := p.opts.RunID
	return &id
}

// wait 是检查点：暂停时停在这里；这一轮被取消时返回错误。
func (p *pass) wait(ctx context.Context) error {
	if p.opts.Control == nil {
		return ctx.Err()
	}
	paused, err := p.opts.Control.Wait(ctx)
	if paused > 0 && err == nil {
		p.res.log.add(stepInfo, "暂停了 %s，继续", humanDuration(paused))
	}
	return err
}

func (p *pass) progress() {
	if p.opts.Progress != nil {
		p.opts.Progress(p.res.snapshot())
	}
}

// Run 运行一轮辅种。返回的错误是这一轮根本没法开始（没有 token、没有站点等）；单个下载器或站点的问题记在结果里。
func (s *Service) Run(ctx context.Context, opts RunOptions) (RunResult, error) {
	res := RunResult{log: &runLog{}}
	r, err := s.row(ctx)
	if err != nil {
		return res, err
	}
	token, err := s.token(ctx, r)
	if err != nil {
		return res, err
	}
	client := s.cfg.NewClient(token)
	sites, err := s.iyuuSites(ctx, &res, client, token)
	if err != nil {
		return res, fmt.Errorf("读取 IYUU 站点列表失败: %w", s.limitErr(ctx, err))
	}
	bySID := map[int]string{}
	sids := make([]int, 0)
	for _, it := range s.mapSites(sites, r) {
		if it.Selected {
			bySID[it.SID] = it.SiteName
			sids = append(sids, it.SID)
		}
	}
	if len(sids) == 0 {
		return res, ErrNoSites
	}
	sort.Ints(sids)
	sidSha1, err := s.sidSha1(ctx, &res, client, &r, sids, false)
	if err != nil {
		return res, fmt.Errorf("向 IYUU 报告站点失败: %w", s.limitErr(ctx, err))
	}

	sources, present := s.collect(ctx, r, &res)
	hashes := make([]string, 0, len(sources))
	for h := range sources {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)
	if opts.After != "" {
		i := sort.Search(len(hashes), func(k int) bool { return hashes[k] > opts.After })
		if i > 0 {
			res.log.add(stepInfo, "接着上一轮往下查：排在前面的 %d 个种子上一轮已经查过", i)
		}
		hashes = hashes[i:]
	}
	res.Hashes = len(hashes)
	res.Batches = (len(hashes) + iyuu.MaxBatch - 1) / iyuu.MaxBatch

	p := &pass{opts: opts, res: &res, limit: dailyLimit(r)}
	if p.today, err = s.todayCounts(ctx); err != nil {
		return res, err
	}
	// 跳过的个数这一轮结束时写一次（停止、超时时也写）；进程在一轮中间退出时这一轮的不记
	defer func() { s.saveDailySkips(ctx, res.daySkips) }()
	p.progress()
	refreshed := false
	for start := 0; start < len(hashes); start += iyuu.MaxBatch {
		if p.wait(ctx) != nil {
			res.endWith(ctx)
			break
		}
		// token 在这一轮里被清除或更换时停下，不再用旧 token 发送 info hash
		if cur, err := s.row(ctx); err != nil {
			res.addError(err.Error())
			break
		} else if cur.TokenEncrypted != r.TokenEncrypted {
			res.End, res.Stopped = EndStopped, "IYUU token 已清除或更换，这一轮停下"
			res.log.add(stepStop, "%s", res.Stopped)
			break
		}
		batch := hashes[start:min(start+iyuu.MaxBatch, len(hashes))]
		found, err := s.query(ctx, &res, client, batch, &sidSha1, &r, sids, &refreshed)
		if err != nil {
			switch {
			case ctx.Err() != nil:
				res.endWith(ctx)
			case IYUULimited(err):
				// 限流、冷却中或到了每天的上限：停下，下一轮接着这里往下查
				res.End, res.Stopped = EndLimited, s.limitErr(ctx, err).Error()
				res.log.add(stepStop, "%s", res.Stopped)
			default:
				res.addError("查询 IYUU 失败: " + err.Error())
			}
			break
		}
		res.log.add(stepQuery, "查询第 %d/%d 批（%d 个种子）：%d 个种子在别的站点上有可辅种的",
			res.BatchesDone+1, res.Batches, len(batch), len(found))
		s.handle(ctx, p, found, sources, present, bySID)
		if ctx.Err() != nil {
			// 这一批没处理完：游标不前进，接着查时这一批再查一次（试过的种子照常跳过）
			res.endWith(ctx)
			break
		}
		res.BatchesDone++
		res.Cursor = batch[len(batch)-1]
		s.saveRunCursor(ctx, opts.RunID, &res)
		p.progress()
	}
	return res, nil
}

// collect 读出范围内各下载器里已经下完的种子（按小写 info hash），以及全部下载器里已经有的 info hash。
func (s *Service) collect(ctx context.Context, r models.ReseedSetting, res *RunResult) (map[string]source, map[string]bool) {
	ids := decodeUints(r.DownloaderIDs)
	if len(ids) == 0 {
		var rows []models.DownloaderSetting
		if err := s.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&rows).Error; err != nil {
			res.addError("读取下载器失败: " + err.Error())
		}
		for _, d := range rows {
			ids = append(ids, d.ID)
		}
	}
	sources := map[string]source{}
	present := map[string]bool{}
	for _, id := range ids {
		dl, set, err := s.downloader(ctx, id)
		if err != nil {
			res.addError(fmt.Sprintf("下载器 %d 不可用: %v", id, err))
			continue
		}
		torrents, err := dl.GetAllTorrents()
		if err != nil {
			res.addError(fmt.Sprintf("读取 %s 的种子失败: %v", set.Name, err))
			continue
		}
		res.Downloaders++
		done := addTorrents(id, dl, torrents, sources, present)
		res.log.add(stepList, "读取下载器 %s：%d 个种子，%d 个已经下完", set.Name, len(torrents), done)
	}
	return sources, present
}

// downloader 取一台下载器；没有配置下载器来源时（测试里）返回错误。
func (s *Service) downloader(ctx context.Context, id uint) (downloader.Downloader, models.DownloaderSetting, error) {
	if s.cfg.Downloaders == nil {
		return nil, models.DownloaderSetting{}, errors.New("没有可用的下载器")
	}
	return s.cfg.Downloaders.TransferDownloader(ctx, id)
}

// addTorrents 把一台下载器里的种子记进 present（全部）与 sources（已经下完的，按小写 info hash，先读到的为准），
// 返回新记进 sources 的个数。
func addTorrents(id uint, dl downloader.Downloader, torrents []downloader.Torrent, sources map[string]source, present map[string]bool) int {
	done := 0
	for _, t := range torrents {
		h := strings.ToLower(t.InfoHash)
		if h == "" {
			continue
		}
		present[h] = true
		if !(t.IsCompleted || t.Progress >= 1) || t.State == downloader.TorrentError || t.State == downloader.TorrentChecking {
			continue
		}
		if _, ok := sources[h]; !ok {
			sources[h] = source{dlID: id, dl: dl, torrent: t}
			done++
		}
	}
	return done
}

// sidSha1 返回查询要带的站点哈希：报告的站点没变、7 天之内就用缓存的；force 时重新报告。
func (s *Service) sidSha1(ctx context.Context, res *RunResult, client *iyuu.Client, r *models.ReseedSetting, sids []int, force bool) (string, error) {
	key := joinInts(sids)
	now := s.cfg.Now()
	if !force && r.SidSha1 != "" && r.SidKey == key && r.SidAt != nil && now.Sub(*r.SidAt) < models.ReseedSidTTL {
		return r.SidSha1, nil
	}
	var sum string
	err := s.callIYUU(ctx, res, models.IYUUEndpointReport, 0, func(ctx context.Context) error {
		var cerr error
		sum, cerr = client.ReportExisting(ctx, sids)
		return cerr
	})
	if err != nil {
		return "", err
	}
	now = s.cfg.Now()
	r.SidSha1, r.SidKey, r.SidAt = sum, key, &now
	// 只写缓存列，并且只在 token 没换过时写：这一轮读出设置之后，token 可能已经被清除或更换
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Model(&models.ReseedSetting{}).
		Where("id = 1 AND token_encrypted = ?", r.TokenEncrypted).
		Updates(map[string]any{"sid_sha1": sum, "sid_key": key, "sid_at": now}).Error; err != nil {
		s.cfg.Logger.Warnf("[辅种] 保存 sid_sha1 失败: %v", err)
	}
	return sum, nil
}

// query 查询一批：限流且冷却不长（不超过 1 分钟）时等冷却结束再查一次；IYUU 报错时重新报告站点、再查一次（每轮最多一次）。
func (s *Service) query(ctx context.Context, res *RunResult, client *iyuu.Client, batch []string, sidSha1 *string,
	r *models.ReseedSetting, sids []int, refreshed *bool,
) (map[string][]iyuu.Candidate, error) {
	once := func() (map[string][]iyuu.Candidate, error) {
		var found map[string][]iyuu.Candidate
		err := s.callIYUU(ctx, res, models.IYUUEndpointQuery, len(batch), func(ctx context.Context) error {
			var cerr error
			found, cerr = client.Query(ctx, batch, *sidSha1)
			return cerr
		})
		return found, err
	}
	found, err := once()
	var rl *iyuu.RateLimitError
	if errors.As(err, &rl) {
		if wait := s.cooldownLeft(ctx); wait > 0 && wait <= maxRateLimitWait {
			if serr := s.cfg.Sleep(ctx, wait); serr != nil {
				return nil, err
			}
			found, err = once()
		}
	}
	var apiErr *iyuu.APIError
	if errors.As(err, &apiErr) && !*refreshed {
		*refreshed = true
		sum, rerr := s.sidSha1(ctx, res, client, r, sids, true)
		if rerr != nil {
			return nil, err
		}
		*sidSha1 = sum
		found, err = once()
	}
	return found, err
}

// dailyLimit 是每个站点每天最多尝试几次。
func dailyLimit(r models.ReseedSetting) int {
	if r.MaxPerSitePerDay == 0 {
		return models.ReseedDefaultMaxPerSitePerDay
	}
	return r.MaxPerSitePerDay
}

// todayCounts 返回今天（进程时区）每个站点已经开始的尝试次数（尝试行只追加：删除或重试记录不会让它变小）。
func (s *Service) todayCounts(ctx context.Context) (map[string]int, error) {
	now := s.cfg.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var rows []struct {
		SiteName string
		N        int
	}
	if err := s.cfg.DB.WithContext(ctx).Model(&models.ReseedAttempt{}).Select("site_name, COUNT(*) AS n").
		Where("julianday(started_at) >= julianday(?)", start.UTC()).Group("site_name").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("统计今天的辅种失败: %w", err)
	}
	out := map[string]int{}
	for _, r := range rows {
		out[r.SiteName] = r.N
	}
	return out, nil
}

// handle 处理一批查询结果：逐个可辅种的种子判断、下载、核对，然后建任务或记下失败。
func (s *Service) handle(ctx context.Context, p *pass, found map[string][]iyuu.Candidate, sources map[string]source,
	present map[string]bool, bySID map[int]string,
) {
	res := p.res
	srcHashes := make([]string, 0, len(found))
	for h := range found {
		srcHashes = append(srcHashes, h)
	}
	sort.Strings(srcHashes)
	for _, srcHash := range srcHashes {
		src, ok := sources[srcHash]
		if !ok {
			continue
		}
		for _, c := range found[srcHash] {
			if p.wait(ctx) != nil {
				return
			}
			res.Candidates++
			site, ok := bySID[c.SID]
			switch {
			case !ok:
				res.skipAt("", SkipNotSelected)
				continue
			case c.InfoHash == "" || present[c.InfoHash]:
				res.skipAt(site, SkipExists)
				continue
			case s.tried(ctx, c.InfoHash, site):
				res.skipAt(site, SkipTried)
				continue
			case p.today[site] >= p.limit:
				res.skipAt(site, SkipDailyLimit)
				continue
			}
			present[c.InfoHash] = true
			s.attempt(ctx, p, src, c, site)
			p.progress()
		}
	}
}

// tried 报告这个站点的这个种子是不是已经尝试过、这次不该再试：暂时失败的过了 ReseedRetryAfter 可以再试。
func (s *Service) tried(ctx context.Context, hash, site string) bool {
	var rec models.ReseedRecord
	err := s.cfg.DB.WithContext(ctx).Where("info_hash = ? AND site_name = ?", hash, site).First(&rec).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return false
	case err != nil:
		return true // 读不到就当尝试过，不冒险重复
	}
	retryNow := rec.State == models.ReseedFailed && rec.Retryable && s.cfg.Now().Sub(rec.UpdatedAt) >= models.ReseedRetryAfter
	return !retryNow
}

// attempt 下载一个可辅种的种子，核对 info hash 与文件列表，通过就建任务。开始时写一行尝试（占今天这个站点的名额），
// 结束时写结局；结果也记进 ReseedRecord（被打断时不改记录）。
func (s *Service) attempt(ctx context.Context, p *pass, src source, c iyuu.Candidate, site string) {
	res := p.res
	db := s.cfg.DB.WithContext(context.WithoutCancel(ctx))
	att := models.ReseedAttempt{
		RunID: p.runID(), StartedAt: s.cfg.Now(), SiteName: site, InfoHash: c.InfoHash, TorrentID: c.TorrentID,
		Name: src.torrent.Name, DownloaderID: src.dlID, Outcome: models.ReseedAttemptRunning,
	}
	if err := db.Create(&att).Error; err != nil {
		// 记不下尝试就不下载：名额与轮次里的尝试都按它算
		res.addError(fmt.Sprintf("记录辅种尝试失败（%s 站内 ID %s，这次没有下载）: %v", site, c.TorrentID, err))
		return
	}
	p.today[site]++
	label := fmt.Sprintf("%s（%s %s）", src.torrent.Name, site, c.TorrentID)
	rec := models.ReseedRecord{
		InfoHash: c.InfoHash, SiteName: site, TorrentID: c.TorrentID, SourceHash: strings.ToLower(src.torrent.InfoHash),
		DownloaderID: src.dlID, Name: src.torrent.Name,
	}
	finish := func(outcome, msg string) { s.finishAttempt(ctx, att.ID, outcome, msg) }
	// interrupted：停止、超时或退出打断了这一次，不记失败，以后照常再试（名额照占：请求可能已经发到站点）
	interrupted := func() bool {
		if ctx.Err() == nil {
			return false
		}
		res.Interrupted++
		finish(models.ReseedAttemptInterrupted, "这一轮被打断，没有试完")
		return true
	}
	fail := func(why string) {
		res.Failed++
		rec.State, rec.Message = models.ReseedFailed, why
		s.saveRecord(ctx, &rec)
		finish(models.ReseedAttemptFailed, why)
		res.log.add(stepFail, "%s：%s", label, why)
	}
	// 站点不可用、下载不到种子、读不到原种子的文件列表是暂时的：记为可重试，过一段时间再试
	retryLater := func(why string) {
		rec.Retryable = true
		fail(why)
	}
	inst, ok := s.cfg.Sites(site)
	if !ok {
		retryLater("站点 " + site + " 不可用")
		return
	}
	data, err := inst.Download(ctx, c.TorrentID)
	if err != nil && interrupted() {
		return
	}
	if errors.Is(err, v2.ErrTorrentNotFound) {
		// 站点上删了（IYUU 的索引里还有）：不是失败，也不会过几天就有，记下来不再试
		const why = "站点上已经没有这个种子（多半已被删除）"
		res.Gone++
		rec.State, rec.Message = models.ReseedGone, why
		s.saveRecord(ctx, &rec)
		finish(models.ReseedAttemptGone, why)
		res.log.add(stepGone, "%s：%s", label, why)
		return
	}
	if err != nil {
		retryLater("下载种子失败: " + err.Error())
		return
	}
	if h, hashErr := qbit.ComputeTorrentHash(data); hashErr != nil || !strings.EqualFold(h, c.InfoHash) {
		fail("下载到的种子与 IYUU 给的 info hash 不符")
		return
	}
	newFiles, err := TorrentFiles(data)
	if err != nil {
		fail(err.Error())
		return
	}
	oldFiles, err := src.dl.GetTorrentFiles(src.torrent.ID)
	if err != nil {
		// 下载器暂时读不到（重启中、连不上）：还没核对，过一段时间再试
		retryLater("读取原种子的文件列表失败: " + err.Error())
		return
	}
	if same, why := SameFiles(newFiles, DownloaderFiles(oldFiles)); !same {
		fail("文件列表与原种子不一致：" + why)
		return
	}
	job := models.TorrentTransferJob{
		TargetDownloaderID: src.dlID, InfoHash: c.InfoHash, Name: src.torrent.Name, TotalSize: src.torrent.TotalSize,
		SiteName: site, TorrentID: c.TorrentID, SourceSavePath: src.torrent.SavePath, TargetSavePath: src.torrent.SavePath,
		Category: src.torrent.Category, Tags: site, TorrentData: data,
	}
	// 任务、辅种记录与尝试的结局在同一个事务里提交：不会出现「任务建成了、记录或尝试没写上」
	err = s.cfg.Transfer.EnqueueReseed(ctx, &job, func(tx *gorm.DB) error {
		created := rec
		created.State, created.JobID = models.ReseedQueued, &job.ID
		if uerr := upsertRecord(tx, &created, s.cfg.Now()); uerr != nil {
			return uerr
		}
		return tx.Model(&models.ReseedAttempt{}).Where("id = ?", att.ID).
			Updates(map[string]any{"outcome": models.ReseedAttemptCreated, "job_id": job.ID, "finished_at": s.cfg.Now()}).Error
	})
	if err != nil {
		switch {
		case interrupted():
		case errors.Is(err, transfer.ErrJobActive):
			// 种子已经下载、核对过了：名额照占，统计里算跳过，不算失败
			res.skip(SkipActive)
			finish(models.ReseedAttemptActive, SkipActive)
		case errors.Is(err, transfer.ErrInvalid):
			fail(err.Error())
		default:
			retryLater("建辅种任务失败: " + err.Error()) // 写库失败等暂时的问题
		}
		return
	}
	res.Created++
	res.log.add(stepAdd, "%s：已建辅种任务（任务 %d）", label, job.ID)
}

// saveDailySkips 把这一轮按站点记的跳过个数加进今天的统计。
func (s *Service) saveDailySkips(ctx context.Context, skips map[string]*models.ReseedDailySkip) {
	if len(skips) == 0 {
		return
	}
	day := iyuuDay(s.cfg.Now())
	db := s.cfg.DB.WithContext(context.WithoutCancel(ctx))
	for site, c := range skips {
		row := models.ReseedDailySkip{Day: day, SiteName: site, Present: c.Present, Tried: c.Tried, DailyLimit: c.DailyLimit, NotSelected: c.NotSelected}
		if err := db.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "day"}, {Name: "site_name"}},
			DoUpdates: clause.Assignments(map[string]any{
				"present":      gorm.Expr("reseed_daily_skips.present + ?", c.Present),
				"tried":        gorm.Expr("reseed_daily_skips.tried + ?", c.Tried),
				"daily_limit":  gorm.Expr("reseed_daily_skips.daily_limit + ?", c.DailyLimit),
				"not_selected": gorm.Expr("reseed_daily_skips.not_selected + ?", c.NotSelected),
			}),
		}).Create(&row).Error; err != nil {
			s.cfg.Logger.Warnf("[辅种] 记录跳过的统计失败: %v", err)
		}
	}
}

// finishAttempt 写尝试的结局（只从 running 改一次）。
func (s *Service) finishAttempt(ctx context.Context, id uint, outcome, msg string) {
	if err := s.cfg.DB.WithContext(context.WithoutCancel(ctx)).Model(&models.ReseedAttempt{}).
		Where("id = ? AND outcome = ?", id, models.ReseedAttemptRunning).
		Updates(map[string]any{"outcome": outcome, "message": msg, "finished_at": s.cfg.Now()}).Error; err != nil {
		s.cfg.Logger.Warnf("[辅种] 记录尝试 %d 的结果失败: %v", id, err)
	}
}

// saveRecord 写入（或在重试时更新）这个站点这个种子的记录。
func (s *Service) saveRecord(ctx context.Context, rec *models.ReseedRecord) {
	if err := upsertRecord(s.cfg.DB.WithContext(context.WithoutCancel(ctx)), rec, s.cfg.Now()); err != nil {
		s.cfg.Logger.Warnf("[辅种] 记录 %s@%s 失败: %v", rec.InfoHash, rec.SiteName, err)
	}
}

func upsertRecord(db *gorm.DB, rec *models.ReseedRecord, now time.Time) error {
	rec.CreatedAt, rec.UpdatedAt = now, now
	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "info_hash"}, {Name: "site_name"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"torrent_id", "source_hash", "downloader_id", "name", "state", "message", "retryable", "job_id", "updated_at",
		}),
	}).Create(rec).Error
}
