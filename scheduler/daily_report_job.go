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
	// EnabledSites 返回已启用站点名（小写）。战报只算这些站点：已禁用站点的增量、残留的登录状态与签到结果都不出现。
	// 为空时不过滤；返回错误时这一轮不发。
	EnabledSites func() (map[string]bool, error)
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
		j.cfg.Logger.Warnf("计算每日战报发送时刻失败: %v", err)
		return
	}
	if now.Before(due) {
		return
	}
	var enabledSites map[string]bool
	if j.cfg.EnabledSites != nil {
		if enabledSites, err = j.cfg.EnabledSites(); err != nil {
			j.cfg.Logger.Warnf("每日战报读取站点配置失败: %v", err)
			return
		}
	}

	title, text, err := j.buildReport(ctx, today, enabledSites)
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

// dueAt 是当天的发送时刻：设定时刻（当地墙上时间，夏令时切换日也不偏）加本安装的偏移，最晚当天 23:59，
// 偏移跨过午夜时不会漏掉这一天。偏移读不出来时返回错误，这一轮不发，不当成 0 偏移提前发。
func (j *DailyReportJob) dueAt(now time.Time, hhmm string) (time.Time, error) {
	at, err := time.Parse("15:04", hhmm)
	if err != nil {
		return time.Time{}, fmt.Errorf("发送时刻 %q 无效: %w", hhmm, err)
	}
	y, m, d := now.Date()
	due := time.Date(y, m, d, at.Hour(), at.Minute(), 0, 0, now.Location())
	if j.cfg.Offset != nil {
		offset, oerr := j.cfg.Offset()
		if oerr != nil {
			return time.Time{}, fmt.Errorf("读取发送偏移失败: %w", oerr)
		}
		due = due.Add(offset)
	}
	if latest := time.Date(y, m, d, 23, 59, 0, 0, now.Location()); due.After(latest) {
		due = latest
	}
	return due, nil
}

// pruneOnce 每天清理一次旧快照；失败时下一轮再试，成功之后当天不再清理。RunOnce 只在调度循环里串行调用。
func (j *DailyReportJob) pruneOnce(ctx context.Context, today string) {
	j.mu.Lock()
	done := j.prunedDay == today
	j.mu.Unlock()
	if done {
		return
	}
	before, err := v2.AddDays(today, -v2.SnapshotRetentionDays)
	if err != nil {
		return
	}
	n, err := j.cfg.History.PruneSnapshots(ctx, before)
	if err != nil {
		j.cfg.Logger.Warnf("清理用户数据快照失败，下一轮重试: %v", err)
		return
	}
	if n > 0 {
		j.cfg.Logger.Infof("清理了 %d 条 %s 以前的用户数据快照", n, before)
	}
	j.mu.Lock()
	j.prunedDay = today
	j.mu.Unlock()
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

// buildReport 生成当天的战报；enabled 不为 nil 时只算其中的站点。
func (j *DailyReportJob) buildReport(ctx context.Context, today string, enabled map[string]bool) (string, string, error) {
	sum, err := v2.LoadDeltaSummary(ctx, j.cfg.History, "today")
	if err != nil {
		return "", "", err
	}
	sum = sum.Filter(enabled)
	title := fmt.Sprintf("📊 每日战报 %s", today)
	var b strings.Builder

	switch {
	case len(sum.Sites) == 0:
		b.WriteString("今天还没有同步到站点数据（登录探测成功或手动同步之后才有）。")
	case countWithBaseline(sum.Sites) == 0:
		b.WriteString("今天暂无可比的数据：站点要有今天之前的一份快照才算得出增量，明天起就有。")
	default:
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

	if abnormal := j.abnormalSites(ctx, enabled); len(abnormal) > 0 {
		fmt.Fprintf(&b, "\n⚠️ 登录状态异常：%s", strings.Join(abnormal, "、"))
	}
	if line := j.attendanceLine(ctx, today, enabled); line != "" {
		b.WriteString("\n")
		b.WriteString(line)
	}
	if lines := j.brushLines(ctx, today); len(lines) > 0 {
		b.WriteString("\n")
		b.WriteString(strings.Join(lines, "\n"))
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

// inSites 报告 site 是否在 enabled 里；enabled 为 nil 表示不过滤。
func inSites(enabled map[string]bool, site string) bool {
	return enabled == nil || enabled[strings.ToLower(site)]
}

// abnormalSites 列出登录状态异常的站点。探测模式为「禁用」的站点不再探测，残留的旧状态不算。
func (j *DailyReportJob) abnormalSites(ctx context.Context, enabled map[string]bool) []string {
	if j.cfg.DB == nil {
		return nil
	}
	var states []models.SiteLoginState
	statuses := make([]string, 0, len(probeStatusText))
	for k := range probeStatusText {
		statuses = append(statuses, k)
	}
	if err := j.cfg.DB.WithContext(ctx).
		Where("last_probe_status IN ? AND (probe_mode IS NULL OR probe_mode <> ?)", statuses, ProbeModeDisabled).
		Order("site_name").Find(&states).Error; err != nil {
		j.cfg.Logger.Warnf("每日战报读取登录状态失败: %v", err)
		return nil
	}
	out := make([]string, 0, len(states))
	for _, st := range states {
		if !inSites(enabled, st.SiteName) {
			continue
		}
		out = append(out, fmt.Sprintf("%s（%s）", siteDisplayName(st.SiteName), probeStatusText[st.LastProbeStatus]))
	}
	return out
}

func (j *DailyReportJob) attendanceLine(ctx context.Context, today string, enabled map[string]bool) string {
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
		if inSites(enabled, l.SiteName) {
			counts[l.Status]++
		}
	}
	if counts[models.AttendanceSigned]+counts[models.AttendanceAlready]+counts[models.AttendanceFailed] == 0 {
		return ""
	}
	return fmt.Sprintf("✅ 今日签到：成功 %d · 已签 %d · 失败 %d",
		counts[models.AttendanceSigned], counts[models.AttendanceAlready], counts[models.AttendanceFailed])
}

// dailyReportMaxBrushTasks 是战报里逐个列出的刷流任务数（按上传排序），其余只算进合计。
const dailyReportMaxBrushTasks = 5

// brushLines 是当天刷流的收益：合计一行，任务多于一个时再逐个列出（最多 5 个）。没有刷流表或当天没有收益时为空。
func (j *DailyReportJob) brushLines(ctx context.Context, today string) []string {
	if j.cfg.DB == nil || !j.cfg.DB.Migrator().HasTable(&models.BrushDailyStat{}) {
		return nil
	}
	var stats []models.BrushDailyStat
	if err := j.cfg.DB.WithContext(ctx).Where("day = ?", today).Find(&stats).Error; err != nil {
		j.cfg.Logger.Warnf("每日战报读取刷流收益失败: %v", err)
		return nil
	}
	var total models.BrushDailyStat
	active := stats[:0]
	for _, st := range stats {
		if st.Uploaded == 0 && st.Downloaded == 0 && st.Added == 0 && st.Removed == 0 {
			continue
		}
		total.Uploaded += st.Uploaded
		total.Downloaded += st.Downloaded
		total.Added += st.Added
		total.Removed += st.Removed
		active = append(active, st)
	}
	if len(active) == 0 {
		return nil
	}
	lines := []string{fmt.Sprintf("🚀 今日刷流：上传 %s · 下载 %s · 加入 %d · 删除 %d",
		utils.FormatBytes(total.Uploaded), utils.FormatBytes(total.Downloaded), total.Added, total.Removed)}
	if len(active) < 2 {
		return lines
	}
	names := map[uint]string{}
	var tasks []models.BrushTask
	if err := j.cfg.DB.WithContext(ctx).Select("id", "name").Find(&tasks).Error; err == nil {
		for _, t := range tasks {
			names[t.ID] = t.Name
		}
	}
	sort.SliceStable(active, func(a, c int) bool { return active[a].Uploaded > active[c].Uploaded })
	for i, st := range active {
		if i == dailyReportMaxBrushTasks {
			lines = append(lines, fmt.Sprintf("· 其余 %d 个刷流任务省略", len(active)-i))
			break
		}
		name := names[st.TaskID]
		if name == "" {
			name = fmt.Sprintf("任务 %d", st.TaskID)
		}
		lines = append(lines, fmt.Sprintf("· %s：上传 %s · 下载 %s", name, utils.FormatBytes(st.Uploaded), utils.FormatBytes(st.Downloaded)))
	}
	return lines
}

// DailyReportRand 是生产环境分配偏移用的随机数。
func DailyReportRand(n int64) int64 {
	if n <= 0 {
		return 0
	}
	return rand.Int63n(n)
}
