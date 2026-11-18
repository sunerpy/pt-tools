package home

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/health"
	"github.com/sunerpy/pt-tools/models"
)

func kinds(groups []TodoGroup) []string {
	out := []string{}
	for _, g := range groups {
		out = append(out, g.Kind)
	}
	return out
}

func itemsOf(todo *Todo, kind string) []TodoItem {
	var out []TodoItem
	for _, it := range todo.Items {
		if it.Kind == kind {
			out = append(out, it)
		}
	}
	return out
}

func groupOf(todo *Todo, kind string) TodoGroup {
	for _, g := range todo.Groups {
		if g.Kind == kind {
			return g
		}
	}
	return TodoGroup{}
}

// 没有要处理的事时两个都是空的（不是 nil）。
func TestBuildTodoEmpty(t *testing.T) {
	db := newDB(t)
	todo, err := BuildTodo(context.Background(), TodoInput{DB: db, Now: time.Now()})
	require.NoError(t, err)
	assert.Equal(t, &Todo{Groups: []TodoGroup{}, Items: []TodoItem{}}, todo)
}

func TestBuildTodo(t *testing.T) {
	db := newDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.Local)
	later := now.Add(time.Hour)

	// 整理记录：两条失败的（一条在自动重试、不算），识别不出的失败与跳过各一条，一条整理成的
	for i, h := range []models.MediaTransferHistory{
		{Status: models.MediaTransferFailed, FailKind: models.MediaFailPath, TorrentName: "Movie.2024.1080p", Message: "pt-tools 里找不到文件"},
		{Status: models.MediaTransferFailed, FailKind: models.MediaFailFilesystem, Title: "现在就出发", MediaType: models.MediaKindTV, TMDBID: 9, Season: 2, Episode: 3, Message: "跨文件系统"},
		{Status: models.MediaTransferFailed, FailKind: models.MediaFailFilesystem, NextRetryAt: &later, TorrentName: "retrying"},
		{Status: models.MediaTransferFailed, FailKind: models.MediaFailRecognize, Message: "TMDB 上没有可靠的匹配"},
		{Status: models.MediaTransferSkipped, FailKind: models.MediaFailRecognize, Title: "沙丘2"},
		{Status: models.MediaTransferDone, Title: "done"},
	} {
		h.SourcePath = fmt.Sprintf("/dl/%d/file%d.mkv", i, i)
		h.UpdatedAt = now.Add(time.Duration(i) * time.Minute)
		require.NoError(t, db.Create(&h).Error)
	}

	// 订阅：在找的那个有两集等版本，暂停的那个不算
	active := models.MediaSubscription{MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Title: "最后生还者", Status: models.MediaSubActive}
	paused := models.MediaSubscription{MediaType: models.MediaKindTV, TMDBID: 200, Season: 1, Title: "暂停的剧", Status: models.MediaSubPaused}
	require.NoError(t, db.Create(&active).Error)
	require.NoError(t, db.Create(&paused).Error)
	for _, e := range []models.MediaSubscriptionEpisode{
		{SubscriptionID: active.ID, Episode: 9, State: models.MediaEpisodeWaitingVersion},
		{SubscriptionID: active.ID, Episode: 8, State: models.MediaEpisodeWaitingVersion},
		{SubscriptionID: active.ID, Episode: 7, State: models.MediaEpisodeLibrary},
		{SubscriptionID: paused.ID, Episode: 1, State: models.MediaEpisodeWaitingVersion},
	} {
		require.NoError(t, db.Create(&e).Error)
	}

	// 站点：甲签到失败、乙登录异常、丙关了探测、丁站点关着
	for _, s := range []models.SiteSetting{
		{Name: "aa", DisplayName: "甲站", Enabled: true, AuthMethod: "cookie"},
		{Name: "bb", DisplayName: "乙站", Enabled: true, AuthMethod: "cookie"},
		{Name: "cc", Enabled: true, AuthMethod: "cookie"},
		{Name: "dd", Enabled: false, AuthMethod: "cookie"},
	} {
		require.NoError(t, db.Create(&s).Error)
	}
	today := now.Format(time.DateOnly)
	for _, l := range []models.SiteAttendanceLog{
		{SiteName: "aa", Day: today, Status: models.AttendanceFailed, LastError: "签到页面改版了"},
		{SiteName: "bb", Day: today, Status: models.AttendanceSigned},
		{SiteName: "dd", Day: today, Status: models.AttendanceFailed},
		{SiteName: "cc", Day: now.AddDate(0, 0, -1).Format(time.DateOnly), Status: models.AttendanceFailed},
	} {
		require.NoError(t, db.Create(&l).Error)
	}
	for _, st := range []models.SiteLoginState{
		{SiteName: "BB", LastProbeStatus: "SESSION_EXPIRED", LastProbeError: "302 to login.php?passkey=0123456789abcdef0123456789abcdef"},
		{SiteName: "cc", LastProbeStatus: "NETWORK_ERROR", ProbeMode: "disabled"},
		{SiteName: "dd", LastProbeStatus: "KEY_ERROR"},
		{SiteName: "aa", LastProbeStatus: "NOT_CONFIGURED"},
	} {
		require.NoError(t, db.Create(&st).Error)
	}

	// 问题：媒体服务器里删了一部电影与一部剧的同一季两集；库文件不见了一个；忽略了的不算
	for i, is := range []models.MediaIssue{
		{Kind: models.MediaIssueServerRemoved, Status: models.MediaIssueOpen, ServerItemID: "m1", MediaType: models.MediaKindMovie, TMDBID: 603, Title: "黑客帝国", Detail: "Jellyfin 里删了"},
		{Kind: models.MediaIssueServerRemoved, Status: models.MediaIssueOpen, ServerItemID: "e1", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1, Episode: 1, Title: "最后生还者"},
		{Kind: models.MediaIssueServerRemoved, Status: models.MediaIssueOpen, ServerItemID: "e2", MediaType: models.MediaKindTV, TMDBID: 100088, Season: 1, Episode: 2, Title: "最后生还者"},
		{Kind: models.MediaIssueLibraryMissing, Status: models.MediaIssueOpen, HistoryID: 7, MediaType: models.MediaKindMovie, TMDBID: 872585, Title: "奥本海默", Detail: "/media/奥本海默.mkv 不见了"},
		{Kind: models.MediaIssueLibraryMissing, Status: models.MediaIssueDismissed, HistoryID: 8, MediaType: models.MediaKindMovie, TMDBID: 1, Title: "忽略了的"},
	} {
		is.UpdatedAt = now.Add(-time.Duration(i) * time.Minute)
		require.NoError(t, db.Create(&is).Error)
	}

	lastAt, okAt := now.Add(-5*time.Hour), now.Add(-80*time.Hour)
	in := TodoInput{
		DB: db, Now: now,
		Backup: func() (health.BackupState, error) {
			return health.BackupState{Enabled: true, LastAt: &lastAt, LastError: "目录写不进去", LastOKAt: &okAt}, nil
		},
		Health: &health.Report{Fail: 2, Items: []health.Item{
			{Key: "tmdb", Title: "TMDB 接口", Status: health.StatusFail, Detail: "API Key 无效"},
			{Key: "site:a", Title: "甲站", Status: health.StatusWarn},
			{Key: "downloader:1", Title: "qb", Status: health.StatusFail, Detail: "连不上"},
		}},
	}
	todo, err := BuildTodo(ctx, in)
	require.NoError(t, err)
	assert.Equal(t, []string{
		TodoOrganizeFailed, TodoRecognize, TodoWaitingVersion, TodoAttendanceFailed, TodoLoginFailed,
		TodoServerRemoved, TodoLibraryMissing, TodoBackup, TodoHealth,
	}, kinds(todo.Groups))

	assert.Equal(t, TodoGroup{Kind: TodoOrganizeFailed, Count: 2, Label: "整理失败 2", Target: "/media/history?status=failed", Tone: ToneDanger}, groupOf(todo, TodoOrganizeFailed))
	failed := itemsOf(todo, TodoOrganizeFailed)
	require.Len(t, failed, 2)
	assert.Equal(t, TodoItem{
		Kind: TodoOrganizeFailed, Title: "现在就出发 S02E03", Detail: "跨文件系统", Target: "/media/history?status=failed",
		MediaType: models.MediaKindTV, TMDBID: 9, Season: 2, Episode: 3,
	}, failed[0], "最近改过的在前")
	assert.Equal(t, "Movie.2024.1080p", failed[1].Title)

	assert.Equal(t, TodoGroup{Kind: TodoRecognize, Count: 2, Label: "识别不出 2", Target: "/media/history?kind=recognize", Tone: ToneWarn}, groupOf(todo, TodoRecognize))
	rec := itemsOf(todo, TodoRecognize)
	require.Len(t, rec, 2)
	assert.Equal(t, "沙丘2", rec[0].Title)
	assert.Equal(t, "file3.mkv", rec[1].Title, "没有标题与种子名时用文件名")

	assert.Equal(t, TodoGroup{Kind: TodoWaitingVersion, Count: 2, Label: "等版本 2", Target: "/media/subscriptions?state=waiting_version", Tone: TonePrimary}, groupOf(todo, TodoWaitingVersion))
	assert.Equal(t, []TodoItem{{
		Kind: TodoWaitingVersion, Title: "最后生还者 第 2 季", Detail: "第 8、9 集在等锁住的版本", Target: "/media/item/tv/100088?from=home&season=2&episode=8",
		MediaType: models.MediaKindTV, TMDBID: 100088, Season: 2, Episode: 8,
	}}, itemsOf(todo, TodoWaitingVersion))

	assert.Equal(t, TodoGroup{Kind: TodoAttendanceFailed, Count: 1, Label: "甲站 签到失败", Target: "/attendance", Tone: ToneWarn}, groupOf(todo, TodoAttendanceFailed))
	assert.Equal(t, "签到页面改版了", itemsOf(todo, TodoAttendanceFailed)[0].Detail)

	assert.Equal(t, TodoGroup{Kind: TodoLoginFailed, Count: 1, Label: "乙站 登录探测失败", Target: "/sites?status=abnormal", Tone: ToneWarn}, groupOf(todo, TodoLoginFailed))
	login := itemsOf(todo, TodoLoginFailed)[0]
	assert.Contains(t, login.Detail, "会话已过期：302 to login.php")
	assert.NotContains(t, login.Detail, "0123456789abcdef")

	assert.Equal(t, TodoGroup{Kind: TodoServerRemoved, Count: 2, Label: "媒体服务器里删了 2 部", Target: TargetDrawer, Tone: ToneInfo}, groupOf(todo, TodoServerRemoved))
	removed := itemsOf(todo, TodoServerRemoved)
	require.Len(t, removed, 2)
	assert.Equal(t, "黑客帝国", removed[0].Title)
	assert.Equal(t, "Jellyfin 里删了", removed[0].Detail)
	assert.Equal(t, "最后生还者 第 1 季", removed[1].Title)
	assert.Len(t, removed[1].IssueIDs, 2, "同一季的几集合成一行")
	assert.Equal(t, []int{1, 2}, removed[1].Episodes)
	assert.Empty(t, removed[0].Episodes, "电影没有集")
	assert.Equal(t, "媒体服务器里删了 2 项", removed[1].Detail)
	assert.Equal(t, 1, removed[1].Season)

	assert.Equal(t, TodoGroup{Kind: TodoLibraryMissing, Count: 1, Label: "库文件不见了 1 个", Target: TargetDrawer, Tone: ToneInfo}, groupOf(todo, TodoLibraryMissing))
	assert.Equal(t, "/media/奥本海默.mkv 不见了", itemsOf(todo, TodoLibraryMissing)[0].Detail)

	assert.Equal(t, TodoGroup{Kind: TodoBackup, Count: 1, Label: "备份 3 天没成功", Target: "/global?section=backup", Tone: ToneWarn}, groupOf(todo, TodoBackup))
	assert.Equal(t, "最近一次失败：目录写不进去", itemsOf(todo, TodoBackup)[0].Detail)

	assert.Equal(t, TodoGroup{Kind: TodoHealth, Count: 2, Label: "体检有 2 项失败", Target: "/system/check", Tone: ToneDanger}, groupOf(todo, TodoHealth))
	assert.Equal(t, []TodoItem{
		{Kind: TodoHealth, Title: "TMDB 接口", Detail: "API Key 无效", Target: "/system/check"},
		{Kind: TodoHealth, Title: "qb", Detail: "连不上", Target: "/system/check"},
	}, itemsOf(todo, TodoHealth))
}

// 几个站点时写个数；抽屉里每类最多 20 行，计数照样是全部的。
func TestTodoLimitsAndLabels(t *testing.T) {
	db := newDB(t)
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.Local)
	for i := range 25 {
		require.NoError(t, db.Create(&models.MediaTransferHistory{
			Status: models.MediaTransferFailed, FailKind: models.MediaFailOther, SourcePath: fmt.Sprintf("/dl/%d.mkv", i),
		}).Error)
	}
	for _, name := range []string{"aa", "bb"} {
		require.NoError(t, db.Create(&models.SiteSetting{Name: name, Enabled: true, AuthMethod: "cookie"}).Error)
		require.NoError(t, db.Create(&models.SiteAttendanceLog{SiteName: name, Day: now.Format(time.DateOnly), Status: models.AttendanceFailed, Message: "没签上"}).Error)
		require.NoError(t, db.Create(&models.SiteLoginState{SiteName: name, LastProbeStatus: "CHALLENGE"}).Error)
	}
	todo, err := BuildTodo(context.Background(), TodoInput{DB: db, Now: now})
	require.NoError(t, err)
	assert.Equal(t, 25, groupOf(todo, TodoOrganizeFailed).Count)
	assert.Len(t, itemsOf(todo, TodoOrganizeFailed), todoItemsPerKind)
	assert.Equal(t, "2 个站点签到失败", groupOf(todo, TodoAttendanceFailed).Label)
	assert.Equal(t, "没签上", itemsOf(todo, TodoAttendanceFailed)[0].Detail, "没有错误时用签到的说明")
	assert.Equal(t, "2 个站点登录探测失败", groupOf(todo, TodoLoginFailed).Label)
	assert.Equal(t, "被反爬拦截", itemsOf(todo, TodoLoginFailed)[0].Detail)

	// 不是查库时限好条数的类（这里是体检失败的项）也只列 20 条，计数照样是全部的
	rep := &health.Report{Fail: 25}
	for i := range 25 {
		rep.Items = append(rep.Items, health.Item{Key: fmt.Sprintf("x:%d", i), Title: fmt.Sprintf("项 %d", i), Status: health.StatusFail})
	}
	todo, err = BuildTodo(context.Background(), TodoInput{DB: db, Now: now, Health: rep})
	require.NoError(t, err)
	assert.Equal(t, 25, groupOf(todo, TodoHealth).Count)
	assert.Len(t, itemsOf(todo, TodoHealth), todoItemsPerKind)
}

func TestIssueEpisodes(t *testing.T) {
	assert.Equal(t, []int{2, 3}, issueEpisodes([]int{3, 2, 3}))
	assert.Nil(t, issueEpisodes([]int{3, 0}), "有说整季的问题时按整季")
	assert.Nil(t, issueEpisodes(nil))
}

func TestTodoBackupWording(t *testing.T) {
	db := newDB(t)
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, time.Local)
	ago := func(d time.Duration) *time.Time { v := now.Add(-d); return &v }
	run := func(st health.BackupState, err error) (*Todo, error) {
		return BuildTodo(context.Background(), TodoInput{DB: db, Now: now, Backup: func() (health.BackupState, error) { return st, err }})
	}
	todo, err := run(health.BackupState{Enabled: true, LastAt: ago(5 * time.Hour), LastError: "磁盘满了", LastOKAt: ago(28 * time.Hour)}, nil)
	require.NoError(t, err)
	assert.Equal(t, "最近一次备份失败", groupOf(todo, TodoBackup).Label)
	assert.Equal(t, "磁盘满了", itemsOf(todo, TodoBackup)[0].Detail)

	todo, err = run(health.BackupState{Enabled: true, LastAt: ago(50 * time.Hour), LastOKAt: ago(50 * time.Hour)}, nil)
	require.NoError(t, err)
	assert.Equal(t, "备份 2 天没成功", groupOf(todo, TodoBackup).Label)
	assert.Equal(t, "最近一次成功在 "+ago(50*time.Hour).Format("01-02 15:04"), itemsOf(todo, TodoBackup)[0].Detail)

	todo, err = run(health.BackupState{Enabled: true, LastAt: ago(50 * time.Hour)}, nil)
	require.NoError(t, err)
	assert.Equal(t, "备份 2 天没成功", groupOf(todo, TodoBackup).Label, "旧版本没记成功时间")

	todo, err = run(health.BackupState{Enabled: true, LastAt: ago(time.Hour), LastOKAt: ago(time.Hour)}, nil)
	require.NoError(t, err)
	assert.Empty(t, todo.Groups, "按时备份成功的不提醒")

	_, err = run(health.BackupState{}, errors.New("读取备份设置失败"))
	require.EqualError(t, err, "读取备份设置失败")
}

// 读库出错时整个待处理报错（首页那一块写原因）。
func TestTodoErrors(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		drop any
		msg  string
	}{
		{&models.MediaTransferHistory{}, "读取整理记录失败"},
		{&models.MediaSubscriptionEpisode{}, "读取追剧台账失败"},
		{&models.MediaSubscription{}, "读取追剧台账失败"},
		{&models.SiteSetting{}, "读取站点失败"},
		{&models.SiteAttendanceLog{}, "读取签到结果失败"},
		{&models.SiteLoginState{}, "读取登录状态失败"},
		{&models.MediaIssue{}, "读取待处理问题失败"},
	} {
		db := newDB(t)
		require.NoError(t, db.Migrator().DropTable(c.drop))
		_, err := BuildTodo(ctx, TodoInput{DB: db, Now: time.Now()})
		require.ErrorContains(t, err, c.msg, "%T", c.drop)
	}

	// 等版本的订阅读不到
	db := newDB(t)
	sub := models.MediaSubscription{MediaType: models.MediaKindTV, TMDBID: 1, Season: 1, Title: "x", Status: models.MediaSubActive}
	require.NoError(t, db.Create(&sub).Error)
	require.NoError(t, db.Create(&models.MediaSubscriptionEpisode{SubscriptionID: sub.ID, Episode: 1, State: models.MediaEpisodeWaitingVersion}).Error)
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("fail_subs", func(tx *gorm.DB) {
		if tx.Statement.Table == "media_subscriptions" {
			_ = tx.AddError(errors.New("坏了"))
		}
	}))
	_, err := BuildTodo(ctx, TodoInput{DB: db, Now: time.Now()})
	require.ErrorContains(t, err, "读取订阅失败")
}
