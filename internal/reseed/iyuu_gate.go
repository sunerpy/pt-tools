package reseed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sunerpy/pt-tools/internal/iyuu"
	"github.com/sunerpy/pt-tools/models"
)

// ---------- IYUU 调用口 ----------
//
// 所有 IYUU 请求（站点列表、报告站点、查询，包括「站点」页读站点列表）都经这里：冷却中、今天的次数到了上限时不发；
// 按设置的最小间隔排队；请求以后记进当天的计数；IYUU 回 429 时按 Retry-After 与 X-RateLimit-Reset 里晚的那个冷却。
// IYUU 的说明（https://doc.iyuu.cn/guide/qa-reseed）：推荐每天辅种一次，短时间内多次运行会被拒绝（辅种太频繁）。

// ErrIYUUDailyCap 表示今天请求 IYUU 的次数到了设置的上限。
var ErrIYUUDailyCap = errors.New("今天请求 IYUU 的次数到了设置的上限，明天再试（可以在设置里调高或改成不限）")

// CoolingError 表示 IYUU 限流以后还在冷却：Until 之前不发请求。
type CoolingError struct {
	Until time.Time
}

func (e *CoolingError) Error() string {
	return "IYUU 限流冷却中，" + e.Until.Local().Format("01-02 15:04:05") + " 以后再试"
}

// IYUULimited 报告 err 是不是 IYUU 的限流（429、冷却中或到了每天的上限）。
func IYUULimited(err error) bool {
	var rl *iyuu.RateLimitError
	var ce *CoolingError
	return errors.As(err, &rl) || errors.As(err, &ce) || errors.Is(err, ErrIYUUDailyCap)
}

// limitError 是 IYUU 回 429 以后的错误：消息写冷却到什么时候（Retry-After 与 X-RateLimit-Reset 里晚的那个，见 coolDown），
// 不照 Retry-After 写秒数；errors.As 仍然取得到原来的 *iyuu.RateLimitError。
type limitError struct {
	rl    *iyuu.RateLimitError
	until time.Time
}

func (e *limitError) Error() string {
	return fmt.Sprintf("IYUU 限流（%s），%s 以后再试", e.rl.Msg, e.until.Local().Format("01-02 15:04:05"))
}

func (e *limitError) Unwrap() error { return e.rl }

// limitErr 把 IYUU 的 429 换成写明冷却时间的错误（冷却已经记下时）；别的错误原样返回。
func (s *Service) limitErr(ctx context.Context, err error) error {
	var rl *iyuu.RateLimitError
	if !errors.As(err, &rl) {
		return err
	}
	r, rerr := s.row(ctx)
	if rerr != nil {
		return err
	}
	ce := cooling(r, s.cfg.Now())
	if ce == nil {
		return err
	}
	return &limitError{rl: rl, until: ce.Until}
}

// IYUULimit 是最近一次限流的情况。
type IYUULimit struct {
	At       time.Time `json:"at"`
	Endpoint string    `json:"endpoint"`
	Message  string    `json:"message"`
	// RetryAfter 是 IYUU 给的 Retry-After（秒）；ResetAt、Limit 是 X-RateLimit-Reset 与 X-RateLimit-Limit（没给时不写）
	RetryAfter int        `json:"retry_after"`
	ResetAt    *time.Time `json:"reset_at,omitempty"`
	Limit      int        `json:"limit,omitempty"`
	// Until 是冷却到什么时候
	Until time.Time `json:"until"`
}

// iyuuGate 是调用口在内存里的状态。
type iyuuGate struct {
	mu sync.Mutex
	// next 是下一个请求最早什么时候发（按最小间隔预约）
	next time.Time
	// pending 是放行了、还没记进计数的请求（检查上限时一起算，同时有两个请求时不会多发）
	pending int
}

// minInterval 是两次请求之间至少隔多久：没设置（NULL）时按默认的 3 秒。
func minInterval(r models.ReseedSetting) time.Duration {
	sec := models.ReseedDefaultIYUUMinInterval
	if r.IYUUMinIntervalSeconds != nil {
		sec = *r.IYUUMinIntervalSeconds
	}
	return time.Duration(sec) * time.Second
}

// cooling 是冷却中的错误；没在冷却时为 nil。
func cooling(r models.ReseedSetting, now time.Time) *CoolingError {
	if r.IYUUCooldownUntil == nil || !now.Before(*r.IYUUCooldownUntil) {
		return nil
	}
	return &CoolingError{Until: *r.IYUUCooldownUntil}
}

// iyuuDay 是计数用的日期（进程时区，与每天的名额同一个时区）。
func iyuuDay(t time.Time) string { return t.Format(time.DateOnly) }

// iyuuCallsOn 是 day 这一天各接口的请求数之和。
func (s *Service) iyuuCallsOn(ctx context.Context, day string) (int, error) {
	var n int
	if err := s.cfg.DB.WithContext(ctx).Model(&models.ReseedIYUUDaily{}).Where("day = ?", day).
		Select("COALESCE(SUM(calls), 0)").Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("读取 IYUU 请求计数失败: %w", err)
	}
	return n, nil
}

// IYUUReady 报告现在能不能请求 IYUU：冷却中返回 *CoolingError，今天到了上限返回 ErrIYUUDailyCap。
// 「立即运行」「继续」开始以前、定时轮次到点时用它判断，免得开始一轮马上又停下。
func (s *Service) IYUUReady(ctx context.Context) error {
	r, err := s.row(ctx)
	if err != nil {
		return err
	}
	now := s.cfg.Now()
	if ce := cooling(r, now); ce != nil {
		return ce
	}
	if r.IYUUMaxCallsPerDay > 0 {
		n, err := s.iyuuCallsOn(ctx, iyuuDay(now))
		if err != nil {
			return err
		}
		if n >= r.IYUUMaxCallsPerDay {
			return fmt.Errorf("%w（%d 次）", ErrIYUUDailyCap, r.IYUUMaxCallsPerDay)
		}
	}
	return nil
}

// admit 放行一个请求：冷却中、到了上限时返回错误；否则预约一个时刻（与上一个请求至少隔最小间隔），等到那时再返回。
// ctx 取消时不等。放行以后一定要 recordCall。
func (s *Service) admit(ctx context.Context) error {
	r, err := s.row(ctx)
	if err != nil {
		return err
	}
	now := s.cfg.Now()
	if ce := cooling(r, now); ce != nil {
		return ce
	}
	g := &s.gate
	g.mu.Lock()
	if r.IYUUMaxCallsPerDay > 0 {
		n, cerr := s.iyuuCallsOn(ctx, iyuuDay(now))
		if cerr != nil {
			g.mu.Unlock()
			return cerr
		}
		if n+g.pending >= r.IYUUMaxCallsPerDay {
			g.mu.Unlock()
			return fmt.Errorf("%w（%d 次）", ErrIYUUDailyCap, r.IYUUMaxCallsPerDay)
		}
	}
	slot := now
	if g.next.After(slot) {
		slot = g.next
	}
	g.next = slot.Add(minInterval(r))
	g.pending++
	g.mu.Unlock()
	if wait := slot.Sub(now); wait > 0 {
		if err := s.cfg.Sleep(ctx, wait); err != nil {
			g.mu.Lock()
			g.pending--
			g.mu.Unlock()
			return err
		}
	}
	return nil
}

// callIYUU 经调用口请求一次 IYUU。res 不为 nil 时把这次请求记进这一轮（次数、限流的步骤）。
func (s *Service) callIYUU(ctx context.Context, res *RunResult, endpoint string, hashes int, fn func(context.Context) error) error {
	if err := s.admit(ctx); err != nil {
		return err
	}
	err := fn(ctx)
	s.recordCall(ctx, res, endpoint, hashes, err)
	return err
}

// recordCall 把一次请求记进当天的计数；429 时设冷却。
func (s *Service) recordCall(ctx context.Context, res *RunResult, endpoint string, hashes int, err error) {
	now := s.cfg.Now()
	ok, limited, failed := 0, 0, 0
	var rl *iyuu.RateLimitError
	switch {
	case err == nil:
		ok = 1
	case errors.As(err, &rl):
		limited = 1
	default:
		failed = 1
	}
	db := s.cfg.DB.WithContext(context.WithoutCancel(ctx))
	row := models.ReseedIYUUDaily{
		Day: iyuuDay(now), Endpoint: endpoint, Calls: 1, OK: ok, RateLimited: limited, Failed: failed, Hashes: hashes, LastAt: &now,
	}
	if werr := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "day"}, {Name: "endpoint"}},
		DoUpdates: clause.Assignments(map[string]any{
			"calls":        gorm.Expr("reseed_iyuu_daily.calls + 1"),
			"ok":           gorm.Expr("reseed_iyuu_daily.ok + ?", ok),
			"rate_limited": gorm.Expr("reseed_iyuu_daily.rate_limited + ?", limited),
			"failed":       gorm.Expr("reseed_iyuu_daily.failed + ?", failed),
			"hashes":       gorm.Expr("reseed_iyuu_daily.hashes + ?", hashes),
			"last_at":      now,
		}),
	}).Create(&row).Error; werr != nil {
		s.cfg.Logger.Warnf("[辅种] 记录 IYUU 请求计数失败: %v", werr)
	}
	s.gate.mu.Lock()
	s.gate.pending--
	s.gate.mu.Unlock()
	if res != nil {
		res.IYUUCalls++
	}
	if rl != nil {
		s.coolDown(db, res, endpoint, rl, now)
	}
}

// coolDown 记下限流：冷却到 Retry-After 与 X-RateLimit-Reset 里晚的那个（最多 24 小时）。
func (s *Service) coolDown(db *gorm.DB, res *RunResult, endpoint string, rl *iyuu.RateLimitError, now time.Time) {
	until := now.Add(rl.RetryAfter)
	if rl.ResetAt.After(until) {
		until = rl.ResetAt
	}
	if limit := now.Add(models.ReseedMaxIYUUCooldown); until.After(limit) {
		until = limit
	}
	info := IYUULimit{At: now, Endpoint: endpoint, Message: rl.Msg, RetryAfter: int(rl.RetryAfter.Seconds()), Limit: rl.Limit, Until: until}
	if !rl.ResetAt.IsZero() {
		reset := rl.ResetAt
		info.ResetAt = &reset
	}
	if err := db.Model(&models.ReseedSetting{}).Where("id = 1").
		Updates(map[string]any{"iyuu_cooldown_until": until, "iyuu_last_limit": encode(info)}).Error; err != nil {
		s.cfg.Logger.Warnf("[辅种] 记录 IYUU 限流失败: %v", err)
	}
	s.cfg.Logger.Warnf("[辅种] IYUU 限流（%s）：%s 之前不再请求", rl.Msg, until.Local().Format("01-02 15:04:05"))
	if res != nil {
		res.log.add(stepInfo, "IYUU 限流（%s）：%s 之前不再请求", rl.Msg, until.Local().Format("01-02 15:04:05"))
	}
}

// cooldownLeft 是冷却还剩多久（没在冷却时为 0）。
func (s *Service) cooldownLeft(ctx context.Context) time.Duration {
	r, err := s.row(ctx)
	if err != nil || r.IYUUCooldownUntil == nil {
		return 0
	}
	return max(r.IYUUCooldownUntil.Sub(s.cfg.Now()), 0)
}

// lastLimit 解出最近一次限流的情况。
func lastLimit(r models.ReseedSetting) *IYUULimit {
	if strings.TrimSpace(r.IYUULastLimit) == "" {
		return nil
	}
	var info IYUULimit
	if json.Unmarshal([]byte(r.IYUULastLimit), &info) != nil {
		return nil
	}
	return &info
}

// IYUUCallsToday 是今天各接口的请求计数（按接口名排）。
func (s *Service) IYUUCallsToday(ctx context.Context) ([]models.ReseedIYUUDaily, error) {
	rows := []models.ReseedIYUUDaily{}
	if err := s.cfg.DB.WithContext(ctx).Where("day = ?", iyuuDay(s.cfg.Now())).Order("endpoint").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取 IYUU 请求计数失败: %w", err)
	}
	return rows, nil
}
