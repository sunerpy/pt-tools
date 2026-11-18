package health

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.Join(t.TempDir(), "t.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	require.NoError(t, db.AutoMigrate(
		&models.AppMeta{}, &models.SiteSetting{}, &models.SiteLoginState{}, &models.DownloaderSetting{},
		&models.MediaServer{}, &models.MediaPathMap{}, &models.MediaLibrary{},
	))
	return db
}

func items(group string, list ...Item) func(context.Context) []Item {
	return func(context.Context) []Item {
		for i := range list {
			list[i].Group = group
		}
		return list
	}
}

// 各组的项按分组排（同一组照检查给的顺序），记下各状态的项数与用时，存进 app_meta；Last 读回同一份。
func TestRunnerReport(t *testing.T) {
	db := newDB(t)
	start := time.Date(2026, 11, 4, 9, 0, 0, 0, time.Local)
	ticks := 0
	r := &Runner{DB: db, Now: func() time.Time { ticks++; return start.Add(time.Duration(ticks-1) * 1500 * time.Millisecond) }}
	r.Checks = func() []Check {
		return []Check{
			{Group: GroupSecret, Title: "密钥", Run: items(GroupSecret, Item{Key: "secret", Status: StatusOK})},
			{Group: GroupTMDB, Title: "TMDB", Run: func(context.Context) []Item {
				return []Item{
					{Key: "tmdb", Group: GroupTMDB, Status: StatusFail},
					{Key: "time", Group: GroupTime, Status: StatusWarn},
					{Key: "tmdb:image", Group: GroupTMDB, Status: StatusSkip},
				}
			}},
			{Group: GroupSites, Title: "站点", Run: items(GroupSites, Item{Key: "site:b", Status: StatusOK}, Item{Key: "site:a", Status: StatusWarn})},
			{Group: "other", Title: "别的", Run: items("other", Item{Key: "x", Status: "odd"})},
		}
	}
	rep, err := r.Run(context.Background())
	require.NoError(t, err)
	var keys []string
	for _, it := range rep.Items {
		keys = append(keys, it.Key)
	}
	assert.Equal(t, []string{"tmdb", "tmdb:image", "site:b", "site:a", "time", "secret", "x"}, keys)
	assert.Equal(t, [4]int{2, 2, 1, 2}, [4]int{rep.OK, rep.Warn, rep.Fail, rep.Skip}, "不认得的状态算跳过")
	assert.Equal(t, int64(1500), rep.DurationMS)
	assert.True(t, rep.At.Equal(start))

	last, err := Last(context.Background(), db)
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.Equal(t, rep.Items, last.Items)
	assert.Equal(t, rep.Fail, last.Fail)
	assert.True(t, last.At.Equal(start))
}

// 一组出了意外（panic）时写成这一组的一项失败，别的组照样跑；超时没跑完的组写成失败，已经跑完的照常给。
func TestRunnerPanicAndTimeout(t *testing.T) {
	db := newDB(t)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	r := &Runner{DB: db, Timeout: 500 * time.Millisecond, Checks: func() []Check {
		return []Check{
			{Group: GroupSites, Title: "站点", Run: func(context.Context) []Item { panic("坏了") }},
			{Group: GroupServers, Title: "媒体服务器", Run: func(context.Context) []Item {
				<-release // 不理会 ctx 的检查：体检照样按时结束
				return nil
			}},
			{Group: GroupBackup, Title: "备份", Run: items(GroupBackup, Item{Key: "backup", Status: StatusOK})},
		}
	}}
	start := time.Now()
	rep, err := r.Run(context.Background())
	require.NoError(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
	require.Len(t, rep.Items, 3)
	assert.Equal(t, Item{Key: GroupSites, Group: GroupSites, Title: "站点", Status: StatusFail, Detail: "检查出错了：坏了"}, rep.Items[0])
	assert.Equal(t, GroupServers, rep.Items[1].Key)
	assert.Equal(t, StatusFail, rep.Items[1].Status)
	assert.Equal(t, "在时限内没检查完", rep.Items[1].Detail)
	assert.NotEmpty(t, rep.Items[1].Hint)
	assert.Equal(t, "backup", rep.Items[2].Key)
	assert.Equal(t, 2, rep.Fail)
}

// 同时最多跑 4 组；没轮到的组在超时以后不再开始。
func TestRunnerParallel(t *testing.T) {
	db := newDB(t)
	var running, peak atomic.Int32
	var ran atomic.Int32
	gate := make(chan struct{})
	checks := make([]Check, 8)
	for i := range checks {
		checks[i] = Check{Group: GroupDisks, Title: "磁盘", Run: func(context.Context) []Item {
			ran.Add(1)
			n := running.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			<-gate
			running.Add(-1)
			return []Item{{Key: "d", Group: GroupDisks, Status: StatusOK}}
		}}
	}
	r := &Runner{DB: db, Checks: func() []Check { return checks }}
	done := make(chan *Report, 1)
	go func() {
		rep, err := r.Run(context.Background())
		assert.NoError(t, err)
		done <- rep
	}()
	require.Eventually(t, func() bool { return running.Load() == parallel }, 5*time.Second, time.Millisecond)
	close(gate)
	rep := <-done
	assert.Equal(t, int32(parallel), peak.Load())
	assert.Equal(t, int32(8), ran.Load())
	assert.Len(t, rep.Items, 8)

	// 时限到了时，排队的组不再开始
	ran.Store(0)
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	slow := make([]Check, 6)
	for i := range slow {
		slow[i] = Check{Group: GroupDisks, Title: "磁盘", Run: func(context.Context) []Item {
			ran.Add(1)
			<-block
			return nil
		}}
	}
	r2 := &Runner{DB: db, Timeout: 50 * time.Millisecond, Checks: func() []Check { return slow }}
	rep, err := r2.Run(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 6, rep.Fail)
	// 占着名额的 4 组一直不回来，排队的组只能等到时限、不再开始
	assert.LessOrEqual(t, ran.Load(), int32(parallel))
}

// 体检跑着的时候再叫：等它跑完、用它的结果，不再跑一遍；等的人自己的请求取消了就先回去。
func TestRunnerSingleFlight(t *testing.T) {
	db := newDB(t)
	var runs atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	r := &Runner{DB: db, Checks: func() []Check {
		return []Check{{Group: GroupBackup, Title: "备份", Run: func(context.Context) []Item {
			if runs.Add(1) == 1 {
				close(started)
			}
			<-release
			return []Item{{Key: "backup", Group: GroupBackup, Status: StatusOK}}
		}}}
	}}
	reports := make([]*Report, 2)
	var wg sync.WaitGroup
	wg.Go(func() {
		var err error
		reports[0], err = r.Run(context.Background())
		assert.NoError(t, err)
	})
	<-started
	wg.Go(func() {
		var err error
		reports[1], err = r.Run(context.Background())
		assert.NoError(t, err)
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := r.Run(ctx)
	require.ErrorIs(t, err, context.Canceled, "等的人取消了就先回去")
	// 第二个请求在等（拿着同一次体检）以后再放行
	require.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.waiting == 1
	}, 5*time.Second, time.Millisecond)
	close(release)
	wg.Wait()
	assert.Equal(t, int32(1), runs.Load())
	require.NotNil(t, reports[0])
	assert.Same(t, reports[0], reports[1])

	rep, err := r.Run(context.Background())
	require.NoError(t, err)
	assert.NotSame(t, reports[0], rep, "跑完以后再叫是新的一次")
	assert.Equal(t, int32(2), runs.Load())
}

// 发起体检的请求取消了（关了页面）体检照样跑完、存下结果：别的请求可能在等它。
func TestRunnerIgnoresCallerCancel(t *testing.T) {
	db := newDB(t)
	started := make(chan struct{})
	release := make(chan struct{})
	r := &Runner{DB: db, Checks: func() []Check {
		return []Check{{Group: GroupBackup, Title: "备份", Run: func(ctx context.Context) []Item {
			close(started)
			<-release
			if ctx.Err() != nil {
				return []Item{{Key: "backup", Group: GroupBackup, Status: StatusFail, Detail: "被取消了"}}
			}
			return []Item{{Key: "backup", Group: GroupBackup, Status: StatusOK}}
		}}}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *Report, 1)
	go func() {
		rep, err := r.Run(ctx)
		assert.NoError(t, err)
		done <- rep
	}()
	<-started
	cancel()
	close(release)
	rep := <-done
	require.NotNil(t, rep)
	assert.Equal(t, 1, rep.OK, "请求取消了体检照样跑完")
	last, err := Last(context.Background(), db)
	require.NoError(t, err)
	require.NotNil(t, last)
	assert.Equal(t, 1, last.OK)
}

// 从没体检过时 Last 为空；存的结果坏了时报错；写不进 app_meta 时体检报错。
func TestLastAndWriteErrors(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	rep, err := Last(ctx, db)
	require.NoError(t, err)
	assert.Nil(t, rep)

	require.NoError(t, models.WriteAppMeta(db, metaKey, "{"))
	_, err = Last(ctx, db)
	require.ErrorContains(t, err, "读取上一次体检的结果失败")

	require.NoError(t, db.Migrator().DropTable(&models.AppMeta{}))
	r := &Runner{DB: db, Checks: func() []Check { return nil }}
	_, err = r.Run(ctx)
	require.ErrorContains(t, err, "记下体检结果失败")
}
