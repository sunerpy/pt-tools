package scheduler

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/sitelogin"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/utils"
)

const (
	dailyReportTick         = time.Minute
	dailyReportStartupDelay = 30 * time.Second
	dailyReportSource       = "daily_report"
	dailyReportKind         = "daily"
	// 战报里最多列出的站点数（按上传增量排序），其余合并成一句
	dailyReportMaxSites = 10
)

// DailyReportJobConfig 是 DailyReportJob 的依赖。
type DailyReportJobConfig struct {
	DB *gorm.DB
	// Settings 返回战报配置：是否开启、发送时刻（HH:MM，Location 时区）与接收通道（core.ConfigStore.DailyReportSettings）。
	Settings func() (enabled bool, hhmm string, channelIDs []uint, err error)
	// Offset 返回本安装固定的发送偏移（core.ConfigStore.DailyReportOffset）。
	Offset func() (time.Duration, error)
	// History 为空（用户数据仓库没起来）时不发战报，也不清理快照。
	History v2.UserInfoHistoryRepo
	// Notifier 为空时不发。
	Notifier *MonitorNotifier
	Clock    sitelogin.Clock
	Logger   *zap.SugaredLogger
	Tick     time.Duration
	// Location 是计算日期与发送时刻的时区，默认进程时区；须与 History 的时区一致。
	Location *time.Location
}

// DailyReportJob 每分钟检查一次：到了设定时刻加本安装固定的偏移后，若当天还没发，就读当天的快照算出增量，
// 连同登录状态异常的站点与签到结果写成一条战报，经 MonitorNotifier 按通道写日志行（同一天每个通道只一行，
// 重启也不重发），由投递器发送并按通道静默时段顺延。每天还顺带清理一次 400 天以前的快照。
type DailyReportJob struct {
	cfg DailyReportJobConfig

	mu        sync.Mutex
	running   bool
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	doneDay   string // 当天已经写过战报，避免每分钟重复计算
	prunedDay string
}

// NewDailyReportJob 构造战报任务，不启动；调用 Start 开始调度。
func NewDailyReportJob(cfg DailyReportJobConfig) *DailyReportJob {
	if cfg.Clock == nil {
		cfg.Clock = sitelogin.NewRealClock()
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop().Sugar()
	}
	if cfg.Tick <= 0 {
		cfg.Tick = dailyReportTick
	}
	if cfg.Location == nil {
		cfg.Location = time.Local
	}
	return &DailyReportJob{cfg: cfg}
}

// Start 启动调度循环；重复调用无效果。
func (j *DailyReportJob) Start() {
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.running {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	j.cancel = cancel
	j.running = true
	j.wg.Go(func() { j.loop(ctx) })
}

// Stop 停止调度循环并等它退出。
func (j *DailyReportJob) Stop() {
	if j == nil {
		return
	}
	j.mu.Lock()
	if !j.running {
		j.mu.Unlock()
		return
	}
	j.running = false
	cancel := j.cancel
	j.mu.Unlock()
	cancel()
	j.wg.Wait()
}

func (j *DailyReportJob) loop(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(dailyReportStartupDelay):
		j.RunOnce(ctx)
	}
	ticker := time.NewTicker(j.cfg.Tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			j.RunOnce(ctx)
		}
	}
}

// RunOnce 处理一轮：每天清理一次旧快照；到了发送时刻且当天还没发就写战报。
func (j *DailyReportJob) RunOnce(ctx context.Context) {
	if j == nil || j.cfg.History == nil {
		return
	}
	now := j.cfg.Clock.Now().In(j.cfg.Location)
	today := now.Format("2006-01-02")
	j.pruneOnce(ctx, today)

	if j.cfg.Settings == nil || j.cfg.Notifier == nil || !j.cfg.Notifier.Enabled() {
		return
	}
	j.mu.Lock()
	done := j.doneDay == today
	j.mu.Unlock()
	if done {
		return
	}
	enabled, hhmm, channels, err := j.cfg.Settings()
	if err != nil {
		j.cfg.Logger.Warnf("读取每日战报设置失败: %v", err)
		return
	}
	if !enabled || len(channels) == 0 {
		return
	}
	due, err := j.dueAt(now, hhmm)
	if err != nil {
		j.cfg.Logger.Warnf("每日战报时间无效: %v", err)
		return
	}
	if now.Before(due) {
		return
	}

	title, text, err := j.buildReport(ctx, today)
	if err != nil {
		j.cfg.Logger.Warnf("生成每日战报失败: %v", err)
		return
	}
	if _, err := j.cfg.Notifier.Enqueue(ctx, MonitorNotifyEntry{
		Source:   dailyReportSource,
		Subject:  "*",
		Kind:     dailyReportKind,
		EventKey: today,
		Title:    title,
		Text:     text,
		ConfIDs:  channels,
	}); err != nil {
		j.cfg.Logger.Warnf("写入每日战报失败: %v", err)
		return
	}
	j.mu.Lock()
	j.doneDay = today
	j.mu.Unlock()
}

// dueAt 是当天的发送时刻：设定时刻加本安装的偏移，最晚当天 23:59，偏移跨过午夜时不会漏掉这一天。
func (j *DailyReportJob) dueAt(now time.Time, hhmm string) (time.Time, error) {
	at, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, err
	}
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	due := day.Add(time.Duration(at.Hour())*time.Hour + time.Duration(at.Minute())*time.Minute)
	if j.cfg.Offset != nil {
		if offset, oerr := j.cfg.Offset(); oerr == nil {
			due = due.Add(offset)
		}
	}
	if latest := day.Add(23*time.Hour + 59*time.Minute); due.After(latest) {
		due = latest
	}
	return due, nil
}

func (j *DailyReportJob) pruneOnce(ctx context.Context, today string) {
	j.mu.Lock()
	if j.prunedDay == today {
		j.mu.Unlock()
		return
	}
	j.prunedDay = today
	j.mu.Unlock()
	before, err := v2.AddDays(today, -v2.SnapshotRetentionDays)
	if err != nil {
		return
	}
	if n, err := j.cfg.History.PruneSnapshots(ctx, before); err != nil {
		j.cfg.Logger.Warnf("清理用户数据快照失败: %v", err)
	} else if n > 0 {
		j.cfg.Logger.Infof("清理了 %d 条 %s 以前的用户数据快照", n, before)
	}
}

func siteDisplayName(id string) string {
	if def, ok := v2.GetDefinitionRegistry().Get(id); ok && def.Name != "" {
		return def.Name
	}
	return id
}

var probeStatusText = map[string]string{
	string(sitelogin.SESSION_EXPIRED): "会话过期",
	string(sitelogin.KEY_ERROR):       "密钥错误",
	string(sitelogin.CHALLENGE):       "被拦截",
	string(sitelogin.NETWORK_ERROR):   "网络错误",
	string(sitelogin.RATE_LIMITED):    "限流",
	string(sitelogin.PARSE_ERROR):     "解析失败",
	string(sitelogin.UNKNOWN):         "未知",
}

func formatBonus(v float64) string {
	return fmt.Sprintf("+%.0f", v)
}

// buildReport 生成当天的战报。
func (j *DailyReportJob) buildReport(ctx context.Context, today string) (string, string, error) {
	sum, err := v2.LoadDeltaSummary(ctx, j.cfg.History, "today")
	if err != nil {
		return "", "", err
	}
	title := fmt.Sprintf("📊 每日战报 %s", today)
	var b strings.Builder

	if len(sum.Sites) == 0 {
		b.WriteString("今天还没有同步到站点数据（登录探测成功或手动同步之后才有）。")
	} else {
		fmt.Fprintf(&b, "今日合计：上传 %s · 下载 %s · 魔力 %s",
			utils.FormatBytes(sum.TotalUploaded), utils.FormatBytes(sum.TotalDownloaded), formatBonus(sum.TotalBonus))
		sites := append([]v2.SiteDelta(nil), sum.Sites...)
		sort.SliceStable(sites, func(a, c int) bool { return sites[a].Uploaded > sites[c].Uploaded })
		var negative []string
		shown := 0
		for _, d := range sites {
			if d.Negative {
				negative = append(negative, siteDisplayName(d.Site))
			}
			if !d.HasBaseline {
				continue
			}
			if shown == dailyReportMaxSites {
				continue
			}
			shown++
			fmt.Fprintf(&b, "\n· %s：上传 %s · 下载 %s · 魔力 %s", siteDisplayName(d.Site),
				utils.FormatBytes(d.Uploaded), utils.FormatBytes(d.Downloaded), formatBonus(d.Bonus))
		}
		if rest := countWithBaseline(sites) - shown; rest > 0 {
			fmt.Fprintf(&b, "\n· 其余 %d 个站点省略", rest)
		}
		if len(negative) > 0 {
			fmt.Fprintf(&b, "\nℹ️ 数据回退、按 0 计入：%s", strings.Join(negative, "、"))
		}
	}

	if abnormal := j.abnormalSites(ctx); len(abnormal) > 0 {
		fmt.Fprintf(&b, "\n⚠️ 登录状态异常：%s", strings.Join(abnormal, "、"))
	}
	if line := j.attendanceLine(ctx, today); line != "" {
		b.WriteString("\n")
		b.WriteString(line)
	}
	return title, b.String(), nil
}

func countWithBaseline(sites []v2.SiteDelta) int {
	n := 0
	for _, d := range sites {
		if d.HasBaseline {
			n++
		}
	}
	return n
}

func (j *DailyReportJob) abnormalSites(ctx context.Context) []string {
	if j.cfg.DB == nil {
		return nil
	}
	var states []models.SiteLoginState
	statuses := make([]string, 0, len(probeStatusText))
	for k := range probeStatusText {
		statuses = append(statuses, k)
	}
	if err := j.cfg.DB.WithContext(ctx).Where("last_probe_status IN ?", statuses).
		Order("site_name").Find(&states).Error; err != nil {
		j.cfg.Logger.Warnf("每日战报读取登录状态失败: %v", err)
		return nil
	}
	out := make([]string, 0, len(states))
	for _, st := range states {
		out = append(out, fmt.Sprintf("%s（%s）", siteDisplayName(st.SiteName), probeStatusText[st.LastProbeStatus]))
	}
	return out
}

func (j *DailyReportJob) attendanceLine(ctx context.Context, today string) string {
	if j.cfg.DB == nil || !j.cfg.DB.Migrator().HasTable(&models.SiteAttendanceLog{}) {
		return ""
	}
	var logs []models.SiteAttendanceLog
	if err := j.cfg.DB.WithContext(ctx).Where("day = ?", today).Find(&logs).Error; err != nil {
		j.cfg.Logger.Warnf("每日战报读取签到结果失败: %v", err)
		return ""
	}
	counts := map[string]int{}
	for _, l := range logs {
		counts[l.Status]++
	}
	if counts[models.AttendanceSigned]+counts[models.AttendanceAlready]+counts[models.AttendanceFailed] == 0 {
		return ""
	}
	return fmt.Sprintf("✅ 今日签到：成功 %d · 已签 %d · 失败 %d",
		counts[models.AttendanceSigned], counts[models.AttendanceAlready], counts[models.AttendanceFailed])
}

// DailyReportRand 是生产环境分配偏移用的随机数。
func DailyReportRand(n int64) int64 {
	if n <= 0 {
		return 0
	}
	return rand.Int63n(n)
}
