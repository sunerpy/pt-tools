package reseed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sunerpy/pt-tools/models"
)

func intPtr(v int) *int { return &v }

func callsByEndpoint(t *testing.T, e *env) map[string]models.ReseedIYUUDaily {
	t.Helper()
	rows, err := e.svc.IYUUCallsToday(context.Background())
	require.NoError(t, err)
	out := map[string]models.ReseedIYUUDaily{}
	for _, r := range rows {
		out[r.Endpoint] = r
	}
	return out
}

// 最小间隔：没设置（NULL）时按 3 秒，0 是不隔。
func TestIYUUMinInterval(t *testing.T) {
	assert.Equal(t, 3*time.Second, minInterval(models.ReseedSetting{}))
	assert.Equal(t, time.Duration(0), minInterval(models.ReseedSetting{IYUUMinIntervalSeconds: intPtr(0)}))
	assert.Equal(t, 10*time.Second, minInterval(models.ReseedSetting{IYUUMinIntervalSeconds: intPtr(10)}))

	e := newEnv(t)
	got, err := e.svc.Settings(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, got.IYUUMinIntervalSeconds, "旧库升级后没设置：按 3 秒")
}

// 每个 IYUU 请求都经调用口：按最小间隔排队，记进当天各接口的计数与这一轮的请求次数。
func TestIYUUGateCountsAndPaces(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(func(u *SettingsUpdate) { u.IYUUMinIntervalSeconds = intPtr(3) })
	good := e.candidate("51", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	e.setResults(map[string]any{"sid": 1, "torrent_id": 51, "info_hash": good})
	res, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 3, res.IYUUCalls, "站点列表、报告站点、查询各一次")
	assert.Equal(t, []time.Duration{3 * time.Second, 3 * time.Second}, e.slept, "第二、三个请求各等 3 秒")
	calls := callsByEndpoint(t, e)
	assert.Equal(t, 1, calls[models.IYUUEndpointSites].Calls)
	assert.Equal(t, 1, calls[models.IYUUEndpointReport].OK)
	assert.Equal(t, 1, calls[models.IYUUEndpointQuery].Calls)
	assert.Equal(t, 1, calls[models.IYUUEndpointQuery].Hashes, "查询了一个种子")
	require.NotNil(t, calls[models.IYUUEndpointQuery].LastAt)

	// 下一轮：站点列表与站点哈希用缓存，只查询一次；离上一个请求已经过了 3 秒，不用等
	e.now = e.now.Add(time.Minute)
	e.slept = nil
	res, err = e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, res.IYUUCalls)
	assert.Empty(t, e.slept)
	assert.Equal(t, 2, callsByEndpoint(t, e)[models.IYUUEndpointQuery].Calls)

	got, err := e.svc.Settings(ctx)
	require.NoError(t, err)
	assert.Len(t, got.IYUUToday, 3)
	assert.Equal(t, 3, got.IYUUMinIntervalSeconds)

	// 第二天另起一行
	e.now = e.now.Add(24 * time.Hour)
	assert.Empty(t, callsByEndpoint(t, e))
}

// 429：冷却到 Retry-After 与 X-RateLimit-Reset 里晚的那个；冷却中不发请求、不跑定时轮次，「站点」页用旧的缓存；换 token 不再冷却。
func TestIYUUCooldown(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(nil)
	reset := e.now.Add(20 * time.Hour).Truncate(time.Second)
	e.iyuu.queryCodes, e.iyuu.retryAfter, e.iyuu.resetAt = []int{429}, 600, reset.Unix()
	res, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, EndLimited, res.End)
	assert.Contains(t, res.Stopped, reset.Format("01-02 15:04:05")+" 以后再试", "按冷却到的时间写，不照 Retry-After 的秒数")
	steps, _ := res.log.encoded()
	assert.Contains(t, steps, "之前不再请求")

	got, err := e.svc.Settings(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.IYUUCooldownUntil)
	assert.True(t, got.IYUUCooldownUntil.Equal(reset), "X-RateLimit-Reset 比 Retry-After 晚，按它冷却")
	require.NotNil(t, got.IYUULastLimit)
	assert.Equal(t, 600, got.IYUULastLimit.RetryAfter)
	assert.Equal(t, 100, got.IYUULastLimit.Limit)
	assert.Equal(t, models.IYUUEndpointQuery, got.IYUULastLimit.Endpoint)
	assert.Equal(t, 1, callsByEndpoint(t, e)[models.IYUUEndpointQuery].RateLimited)

	// 冷却中：不发请求、定时轮次不开始
	err = e.svc.IYUUReady(ctx)
	var ce *CoolingError
	require.ErrorAs(t, err, &ce)
	assert.True(t, IYUULimited(err))
	assert.False(t, e.svc.Due(ctx))
	queries := e.iyuu.queries
	res, err = e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err, "站点列表与站点哈希都有缓存，到查询才被拦下")
	assert.Equal(t, EndLimited, res.End)
	assert.Contains(t, res.Stopped, "冷却中")
	assert.Equal(t, queries, e.iyuu.queries, "冷却中没有发请求")

	// 站点列表的缓存过期了：冷却中照样用旧的
	e.now = e.now.Add(13 * time.Hour)
	items, err := e.svc.SiteMap(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, items)
	assert.Equal(t, 1, callsByEndpoint(t, e)[models.IYUUEndpointSites].Calls, "没有再请求站点列表")

	// 冷却结束
	e.now = reset
	require.NoError(t, e.svc.IYUUReady(ctx))

	// 重置时间离谱：最多冷却 24 小时
	e.iyuu.queryCodes, e.iyuu.resetAt = []int{429}, e.now.Add(72*time.Hour).Unix()
	_, err = e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	got, err = e.svc.Settings(ctx)
	require.NoError(t, err)
	require.NotNil(t, got.IYUUCooldownUntil)
	assert.Equal(t, models.ReseedMaxIYUUCooldown, got.IYUUCooldownUntil.Sub(e.now))

	// 换 token：不再冷却
	tok := "tok2"
	_, err = e.svc.SaveSettings(ctx, SettingsUpdate{Enabled: true, Token: &tok})
	require.NoError(t, err)
	require.NoError(t, e.svc.IYUUReady(ctx))
}

// 冷却不到 1 分钟：等冷却结束再查一次。
func TestIYUUShortCooldownWaits(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(nil)
	good := e.candidate("52", map[string]int64{"a.mkv": 100, "sub/b.srt": 1})
	e.setResults(map[string]any{"sid": 1, "torrent_id": 52, "info_hash": good})
	e.iyuu.queryCodes, e.iyuu.retryAfter, e.iyuu.resetAt = []int{429}, 5, e.now.Add(20*time.Second).Unix()
	res, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{20 * time.Second}, e.slept, "按重置时间等")
	assert.Equal(t, 1, res.Created)
	q := callsByEndpoint(t, e)[models.IYUUEndpointQuery]
	assert.Equal(t, 2, q.Calls)
	assert.Equal(t, 1, q.RateLimited)
	assert.Equal(t, 1, q.OK)
}

// 每天的上限：到了以后不再请求，这一轮提前停下；不跑定时轮次；第二天重新计数。
func TestIYUUDailyCap(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.enable(func(u *SettingsUpdate) { u.IYUUMaxCallsPerDay = 2 })
	res, err := e.svc.Run(ctx, RunOptions{})
	require.NoError(t, err)
	assert.Equal(t, EndLimited, res.End)
	assert.Contains(t, res.Stopped, "到了设置的上限")
	assert.Zero(t, e.iyuu.queries)
	assert.Equal(t, 2, res.IYUUCalls)
	err = e.svc.IYUUReady(ctx)
	require.ErrorIs(t, err, ErrIYUUDailyCap)
	assert.True(t, IYUULimited(err))
	assert.False(t, e.svc.Due(ctx))

	e.now = e.now.Add(24 * time.Hour)
	require.NoError(t, e.svc.IYUUReady(ctx))
	assert.True(t, e.svc.Due(ctx))
}

// 设置：间隔 0–300 秒、每天 0–10000 次；间隔不传时保留原来的。
func TestIYUUSettings(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	for _, u := range []SettingsUpdate{
		{IYUUMinIntervalSeconds: intPtr(-1)},
		{IYUUMinIntervalSeconds: intPtr(301)},
		{IYUUMaxCallsPerDay: -1},
		{IYUUMaxCallsPerDay: 10001},
	} {
		_, err := e.svc.SaveSettings(ctx, u)
		require.ErrorIs(t, err, ErrInvalid)
	}
	got, err := e.svc.SaveSettings(ctx, SettingsUpdate{IYUUMinIntervalSeconds: intPtr(10), IYUUMaxCallsPerDay: 50})
	require.NoError(t, err)
	assert.Equal(t, 10, got.IYUUMinIntervalSeconds)
	assert.Equal(t, 50, got.IYUUMaxCallsPerDay)
	got, err = e.svc.SaveSettings(ctx, SettingsUpdate{IYUUMaxCallsPerDay: 0})
	require.NoError(t, err)
	assert.Equal(t, 10, got.IYUUMinIntervalSeconds, "不传时保留")
	assert.Zero(t, got.IYUUMaxCallsPerDay)
	assert.Empty(t, got.IYUUToday)
}

// 等最小间隔时这一轮被停止：不等，也不算一次请求。
func TestIYUUGateWaitCanceled(t *testing.T) {
	e := newEnv(t)
	e.enable(func(u *SettingsUpdate) { u.IYUUMinIntervalSeconds = intPtr(30) })
	ctx, cancel := context.WithCancel(context.Background())
	e.svc.cfg.Sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}
	_, err := e.svc.SiteMap(context.Background())
	require.NoError(t, err)
	err = e.svc.callIYUU(ctx, nil, models.IYUUEndpointReport, 0, func(context.Context) error { return errors.New("不该发") })
	require.ErrorIs(t, err, context.Canceled)
	assert.Zero(t, callsByEndpoint(t, e)[models.IYUUEndpointReport].Calls)
	assert.Zero(t, e.svc.gate.pending)
}
