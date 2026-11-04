package home

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/health"
	"github.com/sunerpy/pt-tools/models"
)

// 待处理的类别（TodoGroup.Kind、TodoItem.Kind），也是首页上 chip 的顺序。
const (
	TodoOrganizeFailed   = "organize_failed"
	TodoRecognize        = "recognize"
	TodoWaitingVersion   = "waiting_version"
	TodoAttendanceFailed = "attendance_failed"
	TodoLoginFailed      = "login_failed"
	TodoServerRemoved    = "server_removed"
	TodoLibraryMissing   = "library_missing"
	TodoBackup           = "backup"
	TodoHealth           = "health"
)

// chip 的颜色（TodoGroup.Tone），同界面的语义色。
const (
	ToneDanger  = "dang"
	ToneWarn    = "warn"
	TonePrimary = "primary"
	ToneInfo    = "info"
)

// TargetDrawer 是不去别的页面、在首页的抽屉里处理的那几类（媒体服务器里删了的与库文件不见了：每行有「处理」「忽略」）。
const TargetDrawer = "drawer"

// todoItemsPerKind 是抽屉里每类最多列几条（计数照样是全部的）
const todoItemsPerKind = 20

// TodoGroup 是待处理的一类：首页上的一个 chip，点了去 Target（带着筛选的页面，或者 drawer）。
type TodoGroup struct {
	Kind   string `json:"kind"`
	Count  int    `json:"count"`
	Label  string `json:"label"`
	Target string `json:"target"`
	Tone   string `json:"tone"`
}

// TodoItem 是待处理抽屉里的一行。IssueIDs 是媒体服务器里删了的与库文件不见了的问题（「忽略」把它们都忽略掉）；
// MediaType 到 Episode 是这一行说的条目（「处理」打开它的删除弹窗、「看版本」打开它的详情）。
type TodoItem struct {
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Detail    string `json:"detail,omitempty"`
	Target    string `json:"target,omitempty"`
	IssueIDs  []uint `json:"issue_ids,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	TMDBID    int    `json:"tmdb_id,omitempty"`
	Season    int    `json:"season,omitempty"`
	Episode   int    `json:"episode,omitempty"`
	// Episodes 是问题说的那几集（剧集；有说整季的问题时为空，删除弹窗按整季）
	Episodes []int `json:"episodes,omitempty"`
}

// Todo 是首页的待处理：每类一组，另给抽屉里的各行（每类最多 20 行）。都没有时两个都是空的。
type Todo struct {
	Groups []TodoGroup `json:"groups"`
	Items  []TodoItem  `json:"items"`
}

// TodoInput 是算待处理要用的：Backup 是备份的设置与最近一次的结果，Health 是上一次体检（没体检过时为 nil）。
type TodoInput struct {
	DB     *gorm.DB
	Now    time.Time
	Backup func() (health.BackupState, error)
	Health *health.Report
}

type todoBuilder struct {
	ctx context.Context
	in  TodoInput
	db  *gorm.DB
	out *Todo
}

// BuildTodo 算首页的待处理（只读库，不发请求）：整理失败（不在自动重试的）、识别不出、等版本的集、今天签到失败的站点、
// 登录探测失败的站点、媒体服务器里删了的、库文件不见了、备份没成功、体检失败的项。
func BuildTodo(ctx context.Context, in TodoInput) (*Todo, error) {
	b := &todoBuilder{ctx: ctx, in: in, db: in.DB.WithContext(ctx), out: &Todo{Groups: []TodoGroup{}, Items: []TodoItem{}}}
	for _, step := range []func() error{
		b.organizeFailed, b.recognize, b.waitingVersion, b.attendance, b.login, b.issues, b.backup,
	} {
		if err := step(); err != nil {
			return nil, err
		}
	}
	b.health()
	return b.out, nil
}

func (b *todoBuilder) group(g TodoGroup, items []TodoItem) {
	if g.Count <= 0 {
		return
	}
	b.out.Groups = append(b.out.Groups, g)
	if len(items) > todoItemsPerKind {
		items = items[:todoItemsPerKind]
	}
	b.out.Items = append(b.out.Items, items...)
}

// historyName 是整理记录给人看的名字：识别出的标题（剧集带上季与集），其次种子名，再其次文件名。
func historyName(r models.MediaTransferHistory) string {
	if r.Title != "" {
		if r.MediaType == models.MediaKindTV && r.Season > 0 && r.Episode > 0 {
			return fmt.Sprintf("%s S%02dE%02d", r.Title, r.Season, r.Episode)
		}
		return r.Title
	}
	if r.TorrentName != "" {
		return r.TorrentName
	}
	return filepath.Base(r.SourcePath)
}

func (b *todoBuilder) histories(kind, target string, q *gorm.DB) ([]TodoItem, int, error) {
	var n int64
	if err := q.Session(&gorm.Session{}).Count(&n).Error; err != nil {
		return nil, 0, fmt.Errorf("读取整理记录失败: %w", err)
	}
	var rows []models.MediaTransferHistory
	if err := q.Session(&gorm.Session{}).Order("updated_at DESC, id DESC").Limit(todoItemsPerKind).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("读取整理记录失败: %w", err)
	}
	items := make([]TodoItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, TodoItem{
			Kind: kind, Title: historyName(r), Detail: r.Message, Target: target,
			MediaType: r.MediaType, TMDBID: r.TMDBID, Season: r.Season, Episode: r.Episode,
		})
	}
	return items, int(n), nil
}

func (b *todoBuilder) organizeFailed() error {
	const target = "/media/history?status=failed"
	q := b.db.Model(&models.MediaTransferHistory{}).
		Where("status = ? AND fail_kind <> ? AND next_retry_at IS NULL", models.MediaTransferFailed, models.MediaFailRecognize)
	items, n, err := b.histories(TodoOrganizeFailed, target, q)
	if err != nil {
		return err
	}
	b.group(TodoGroup{Kind: TodoOrganizeFailed, Count: n, Label: fmt.Sprintf("整理失败 %d", n), Target: target, Tone: ToneDanger}, items)
	return nil
}

func (b *todoBuilder) recognize() error {
	const target = "/media/history?kind=recognize"
	q := b.db.Model(&models.MediaTransferHistory{}).
		Where("status IN ? AND fail_kind = ?", []string{models.MediaTransferFailed, models.MediaTransferSkipped}, models.MediaFailRecognize)
	items, n, err := b.histories(TodoRecognize, target, q)
	if err != nil {
		return err
	}
	b.group(TodoGroup{Kind: TodoRecognize, Count: n, Label: fmt.Sprintf("识别不出 %d", n), Target: target, Tone: ToneWarn}, items)
	return nil
}

// waitingVersion 是在找的订阅里在等锁住的版本的集：每个订阅一行，写出是哪几集。
func (b *todoBuilder) waitingVersion() error {
	type row struct {
		SubscriptionID uint
		Episode        int
	}
	var rows []row
	if err := b.db.Table("media_subscription_episodes AS e").Select("e.subscription_id, e.episode").
		Joins("JOIN media_subscriptions AS s ON s.id = e.subscription_id").
		Where("e.state = ? AND s.status = ?", models.MediaEpisodeWaitingVersion, models.MediaSubActive).
		Order("e.subscription_id, e.episode").Scan(&rows).Error; err != nil {
		return fmt.Errorf("读取追剧台账失败: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}
	episodes := map[uint][]int{}
	var order []uint
	for _, r := range rows {
		if _, ok := episodes[r.SubscriptionID]; !ok {
			order = append(order, r.SubscriptionID)
		}
		episodes[r.SubscriptionID] = append(episodes[r.SubscriptionID], r.Episode)
	}
	var subs []models.MediaSubscription
	if err := b.db.Where("id IN ?", order).Find(&subs).Error; err != nil {
		return fmt.Errorf("读取订阅失败: %w", err)
	}
	byID := make(map[uint]models.MediaSubscription, len(subs))
	for _, s := range subs {
		byID[s.ID] = s
	}
	items := make([]TodoItem, 0, len(order))
	for _, id := range order {
		s := byID[id]
		eps := episodes[id]
		nums := make([]string, 0, len(eps))
		for _, e := range eps {
			nums = append(nums, strconv.Itoa(e))
		}
		title := s.Title
		if s.Season > 0 {
			title = fmt.Sprintf("%s 第 %d 季", s.Title, s.Season)
		}
		items = append(items, TodoItem{
			Kind: TodoWaitingVersion, Title: title, Detail: "第 " + strings.Join(nums, "、") + " 集在等锁住的版本",
			Target:    fmt.Sprintf("/media/item/%s/%d?from=home&season=%d&episode=%d", s.MediaType, s.TMDBID, s.Season, eps[0]),
			MediaType: s.MediaType, TMDBID: s.TMDBID, Season: s.Season, Episode: eps[0],
		})
	}
	b.group(TodoGroup{
		Kind: TodoWaitingVersion, Count: len(rows), Label: fmt.Sprintf("等版本 %d", len(rows)),
		Target: "/media/subscriptions?state=waiting_version", Tone: TonePrimary,
	}, items)
	return nil
}

// enabledSites 是启用的站点（编号小写 → 站点设置）。
func (b *todoBuilder) enabledSites() (map[string]models.SiteSetting, error) {
	var sites []models.SiteSetting
	if err := b.db.Where("enabled = ?", true).Find(&sites).Error; err != nil {
		return nil, fmt.Errorf("读取站点失败: %w", err)
	}
	out := make(map[string]models.SiteSetting, len(sites))
	for _, s := range sites {
		out[strings.ToLower(s.Name)] = s
	}
	return out, nil
}

// siteLabel 是站点那几类的 chip：一个站点时写站点名，几个时写个数。
func siteLabel(items []TodoItem, what string) string {
	if len(items) == 1 {
		return items[0].Title + " " + what
	}
	return fmt.Sprintf("%d 个站点%s", len(items), what)
}

// attendance 是今天签到失败的站点（启用的）。
func (b *todoBuilder) attendance() error {
	sites, err := b.enabledSites()
	if err != nil {
		return err
	}
	var logs []models.SiteAttendanceLog
	day := b.in.Now.In(time.Local).Format(time.DateOnly)
	if err := b.db.Where("day = ? AND status = ?", day, models.AttendanceFailed).Order("site_name").Find(&logs).Error; err != nil {
		return fmt.Errorf("读取签到结果失败: %w", err)
	}
	var items []TodoItem
	for _, l := range logs {
		s, ok := sites[strings.ToLower(l.SiteName)]
		if !ok {
			continue
		}
		detail := l.LastError
		if detail == "" {
			detail = l.Message
		}
		items = append(items, TodoItem{Kind: TodoAttendanceFailed, Title: health.SiteName(s), Detail: clean(detail), Target: "/attendance"})
	}
	if len(items) > 0 {
		b.group(TodoGroup{Kind: TodoAttendanceFailed, Count: len(items), Label: siteLabel(items, "签到失败"), Target: "/attendance", Tone: ToneWarn}, items)
	}
	return nil
}

// login 是登录探测失败的站点（启用的、没关探测的，同每日战报的「登录状态异常」）。
func (b *todoBuilder) login() error {
	sites, err := b.enabledSites()
	if err != nil {
		return err
	}
	var states []models.SiteLoginState
	if err := b.db.Where("last_probe_status IN ? AND (probe_mode IS NULL OR probe_mode <> ?)", health.AbnormalProbes(), "disabled").
		Order("site_name").Find(&states).Error; err != nil {
		return fmt.Errorf("读取登录状态失败: %w", err)
	}
	const target = "/sites?status=abnormal"
	var items []TodoItem
	for _, st := range states {
		s, ok := sites[strings.ToLower(st.SiteName)]
		if !ok {
			continue
		}
		detail := health.ProbeLabel(st.LastProbeStatus)
		if e := clean(st.LastProbeError); e != "" {
			detail += "：" + e
		}
		items = append(items, TodoItem{Kind: TodoLoginFailed, Title: health.SiteName(s), Detail: detail, Target: target})
	}
	if len(items) > 0 {
		b.group(TodoGroup{Kind: TodoLoginFailed, Count: len(items), Label: siteLabel(items, "登录探测失败"), Target: target, Tone: ToneWarn}, items)
	}
	return nil
}

type issueKey struct {
	kind   string
	tmdbID int
	season int
}

// issues 是开着的媒体服务器里删了的（按部计数）与库文件不见了的（按文件计数）：抽屉里同一部（剧集按季）合成一行。
func (b *todoBuilder) issues() error {
	var rows []models.MediaIssue
	if err := b.db.Where("status = ? AND kind IN ?", models.MediaIssueOpen, []string{models.MediaIssueServerRemoved, models.MediaIssueLibraryMissing}).
		Order("updated_at DESC, id DESC").Find(&rows).Error; err != nil {
		return fmt.Errorf("读取待处理问题失败: %w", err)
	}
	for _, kind := range []string{models.MediaIssueServerRemoved, models.MediaIssueLibraryMissing} {
		var items []TodoItem
		var episodes [][]int
		index := map[issueKey]int{}
		titles := map[[2]any]bool{}
		files := 0
		for _, r := range rows {
			if r.Kind != kind {
				continue
			}
			files++
			titles[[2]any{r.MediaType, r.TMDBID}] = true
			key := issueKey{r.MediaType, r.TMDBID, r.Season}
			if r.MediaType != models.MediaKindTV {
				key.season = 0
			}
			if i, ok := index[key]; ok && r.TMDBID > 0 {
				items[i].IssueIDs = append(items[i].IssueIDs, r.ID)
				episodes[i] = append(episodes[i], r.Episode)
				continue
			}
			index[key] = len(items)
			episodes = append(episodes, []int{r.Episode})
			title := r.Title
			if r.MediaType == models.MediaKindTV && r.Season > 0 {
				title = fmt.Sprintf("%s 第 %d 季", r.Title, r.Season)
			}
			items = append(items, TodoItem{
				Kind: kind, Title: title, Detail: r.Detail, Target: TargetDrawer, IssueIDs: []uint{r.ID},
				MediaType: r.MediaType, TMDBID: r.TMDBID, Season: key.season,
			})
		}
		for i := range items {
			if items[i].MediaType == models.MediaKindTV {
				items[i].Episodes = issueEpisodes(episodes[i])
			}
			if n := len(items[i].IssueIDs); n > 1 {
				if kind == models.MediaIssueServerRemoved {
					items[i].Detail = fmt.Sprintf("媒体服务器里删了 %d 项", n)
				} else {
					items[i].Detail = fmt.Sprintf("库里有 %d 个文件不见了", n)
				}
			}
		}
		if kind == models.MediaIssueServerRemoved {
			b.group(TodoGroup{Kind: TodoServerRemoved, Count: len(titles), Label: fmt.Sprintf("媒体服务器里删了 %d 部", len(titles)), Target: TargetDrawer, Tone: ToneInfo}, items)
		} else {
			b.group(TodoGroup{Kind: TodoLibraryMissing, Count: files, Label: fmt.Sprintf("库文件不见了 %d 个", files), Target: TargetDrawer, Tone: ToneInfo}, items)
		}
	}
	return nil
}

// issueEpisodes 是几个问题说的集（去重、排好）；有说整季的（集号为 0）时为空。
func issueEpisodes(eps []int) []int {
	var out []int
	for _, e := range eps {
		if e <= 0 {
			return nil
		}
		if !slices.Contains(out, e) {
			out = append(out, e)
		}
	}
	slices.Sort(out)
	return out
}

// backup 是备份没成功（开着定时备份时，最近一次失败或两天没成功）。
func (b *todoBuilder) backup() error {
	if b.in.Backup == nil {
		return nil
	}
	st, err := b.in.Backup()
	if err != nil {
		return err
	}
	days, late := health.BackupLate(st, b.in.Now)
	if !late {
		return nil
	}
	const target = "/global?section=backup"
	label, detail := "最近一次备份失败", clean(st.LastError)
	if days >= 2 {
		label = fmt.Sprintf("备份 %d 天没成功", days)
		if st.LastError != "" {
			detail = "最近一次失败：" + detail
		} else {
			ok := st.LastOKAt
			if ok == nil {
				ok = st.LastAt
			}
			detail = "最近一次成功在 " + ok.In(time.Local).Format("01-02 15:04")
		}
	}
	b.group(TodoGroup{Kind: TodoBackup, Count: 1, Label: label, Target: target, Tone: ToneWarn},
		[]TodoItem{{Kind: TodoBackup, Title: label, Detail: detail, Target: target}})
	return nil
}

// health 是上一次体检失败的项。
func (b *todoBuilder) health() {
	rep := b.in.Health
	if rep == nil || rep.Fail == 0 {
		return
	}
	const target = "/system/check"
	var items []TodoItem
	for _, it := range rep.Items {
		if it.Status == health.StatusFail {
			items = append(items, TodoItem{Kind: TodoHealth, Title: it.Title, Detail: it.Detail, Target: target})
		}
	}
	b.group(TodoGroup{Kind: TodoHealth, Count: rep.Fail, Label: fmt.Sprintf("体检有 %d 项失败", rep.Fail), Target: target, Tone: ToneDanger}, items)
}
