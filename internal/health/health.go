// Package health 是系统体检（媒体中心 M8）：一次检查 TMDB、站点、下载器、媒体服务器、路径映射、媒体库、磁盘、时间、备份与密钥，
// 每项给出状态、说明与怎么修。结果存进 app_meta，体检页与首页读上一次的。不改任何设置；媒体库同「检查」，在库目录里试建一个文件再删掉。
package health

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/models"
)

// 一项检查的状态（Item.Status）。
const (
	StatusOK   = "ok"
	StatusWarn = "warn"
	StatusFail = "fail"
	StatusSkip = "skip"
)

// 检查的分组（Item.Group），也是体检页上的顺序。
const (
	GroupTMDB        = "tmdb"
	GroupSites       = "sites"
	GroupDownloaders = "downloaders"
	GroupServers     = "servers"
	GroupPaths       = "paths"
	GroupLibraries   = "libraries"
	GroupDisks       = "disks"
	GroupTime        = "time"
	GroupBackup      = "backup"
	GroupSecret      = "secret"
)

var groupOrder = []string{
	GroupTMDB, GroupSites, GroupDownloaders, GroupServers, GroupPaths, GroupLibraries, GroupDisks, GroupTime, GroupBackup, GroupSecret,
}

const (
	// parallel 是同时跑几组检查
	parallel = 4
	// DefaultTimeout 是一次体检最多等多久：到时还没跑完的那一组记成失败
	DefaultTimeout = 60 * time.Second
	// metaKey 是 app_meta 里存上一次结果的键
	metaKey = "system_check"
)

// Item 是一项检查的结果：Key 认这一项（tmdb、site:hdsky、downloader:1…），Detail 是现在的样子，Hint 是怎么修（没有问题时为空）。
type Item struct {
	Key    string `json:"key"`
	Group  string `json:"group"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

// Report 是一次体检：各项按分组排，后面是各种状态的项数。
type Report struct {
	At         time.Time `json:"at"`
	DurationMS int64     `json:"duration_ms"`
	Items      []Item    `json:"items"`
	OK         int       `json:"ok"`
	Warn       int       `json:"warn"`
	Fail       int       `json:"fail"`
	Skip       int       `json:"skip"`
}

// Check 是一组检查：Run 返回这一组的各项（出错也写成项）。Group、Title 是超时没跑完时那一项的分组与名字。
type Check struct {
	Group string
	Title string
	Run   func(ctx context.Context) []Item
}

// Runner 跑体检：同时最多 4 组，总时限 60 秒；同一时间只跑一次，跑着的时候再叫就等它跑完、用它的结果。
type Runner struct {
	DB  *gorm.DB
	Now func() time.Time
	// Checks 每次体检时建一遍（设置改了马上生效）
	Checks func() []Check
	// Timeout 是一次体检最多等多久，0 用 DefaultTimeout
	Timeout time.Duration

	mu  sync.Mutex
	cur *run
	// waiting 是在等跑着的那一次的请求数
	waiting int
}

type run struct {
	done chan struct{}
	rep  *Report
	err  error
}

// Run 跑一次体检，存下结果。
func (r *Runner) Run(ctx context.Context) (*Report, error) {
	r.mu.Lock()
	if c := r.cur; c != nil {
		r.waiting++
		r.mu.Unlock()
		defer func() {
			r.mu.Lock()
			r.waiting--
			r.mu.Unlock()
		}()
		select {
		case <-c.done:
			return c.rep, c.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	c := &run{done: make(chan struct{})}
	r.cur = c
	r.mu.Unlock()
	// 跑体检不受这一个请求的取消影响：别的请求也在等它
	c.rep, c.err = r.run(context.WithoutCancel(ctx))
	r.mu.Lock()
	r.cur = nil
	r.mu.Unlock()
	close(c.done)
	return c.rep, c.err
}

func (r *Runner) run(ctx context.Context) (*Report, error) {
	now := r.now()
	limit := r.Timeout
	if limit <= 0 {
		limit = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	checks := r.Checks()
	results := make([][]Item, len(checks))
	finished := make([]bool, len(checks))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, parallel)
	for i, c := range checks {
		wg.Go(func() {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()
			items := safeRun(ctx, c)
			mu.Lock()
			results[i], finished[i] = items, true
			mu.Unlock()
		})
	}
	waited := make(chan struct{})
	go func() {
		wg.Wait()
		close(waited)
	}()
	select {
	case <-waited:
	case <-ctx.Done():
	}
	rep := &Report{At: now, Items: []Item{}}
	mu.Lock()
	for i, c := range checks {
		if !finished[i] {
			rep.Items = append(rep.Items, Item{
				Key: c.Group, Group: c.Group, Title: c.Title, Status: StatusFail,
				Detail: "在时限内没检查完", Hint: "多半是连不上：检查网络与代理以后再体检一次",
			})
			continue
		}
		rep.Items = append(rep.Items, results[i]...)
	}
	mu.Unlock()
	sortItems(rep.Items)
	for _, it := range rep.Items {
		switch it.Status {
		case StatusOK:
			rep.OK++
		case StatusWarn:
			rep.Warn++
		case StatusFail:
			rep.Fail++
		default:
			rep.Skip++
		}
	}
	rep.DurationMS = r.now().Sub(now).Milliseconds()
	b, err := json.Marshal(rep)
	if err != nil {
		return nil, fmt.Errorf("记下体检结果失败: %w", err)
	}
	if err := models.WriteAppMeta(r.DB, metaKey, string(b)); err != nil {
		return nil, fmt.Errorf("记下体检结果失败: %w", err)
	}
	return rep, nil
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// safeRun 跑一组检查；这一组出了意外（panic）时写成一项失败，别的组照样跑。
func safeRun(ctx context.Context, c Check) (items []Item) {
	defer func() {
		if p := recover(); p != nil {
			items = []Item{{Key: c.Group, Group: c.Group, Title: c.Title, Status: StatusFail, Detail: fmt.Sprintf("检查出错了：%v", p)}}
		}
	}()
	return c.Run(ctx)
}

// sortItems 按分组的顺序排，同一组里照检查给出的顺序。
func sortItems(items []Item) {
	rank := func(g string) int {
		if i := slices.Index(groupOrder, g); i >= 0 {
			return i
		}
		return len(groupOrder)
	}
	slices.SortStableFunc(items, func(a, b Item) int { return cmp.Compare(rank(a.Group), rank(b.Group)) })
}

// Last 是上一次体检的结果；从没体检过时为 nil。
func Last(ctx context.Context, db *gorm.DB) (*Report, error) {
	raw, err := models.ReadAppMeta(db.WithContext(ctx), metaKey)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var rep Report
	if err := json.Unmarshal([]byte(raw), &rep); err != nil {
		return nil, fmt.Errorf("读取上一次体检的结果失败: %w", err)
	}
	return &rep, nil
}
