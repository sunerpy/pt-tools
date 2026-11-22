package scheduler

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 一轮读种子列表的上限（见 listBrushItems）。
const (
	// brushMaxListPages 是每张列表最多读几页；brushMaxListReads 是一轮最多读几次列表
	brushMaxListPages = 2
	brushMaxListReads = 4
	// brushListBudget 是一轮里读列表一共能用多久，不算站点要求的读列表间隔（等的时间另算）；整轮仍是 brushRunTimeout
	brushListBudget = 3 * time.Minute
	// brushListMinRead 是读下一页之前至少要剩的时间：不够一次像样的请求就不读了
	brushListMinRead = 30 * time.Second
	// brushAdmitReserve 是读完列表以后至少要给加种（下载种子文件、推送）留的时间
	brushAdmitReserve = time.Minute
)

// brushNeed 是补页的条件：候选少于 slots 个时接着读；slots 为 0（没设同时下载上限）时候选为 0 才读。
// count 数一份列表里有几个候选。
type brushNeed struct {
	slots int
	count func(items []v2.TorrentItem) int
}

func (n *brushNeed) short(have int) bool {
	if n.slots > 0 {
		return have < n.slots
	}
	return have == 0
}

// reason 是补读一页的原因，写在运行记录那一步的前面。
func (n *brushNeed) reason(have int) string {
	if n.slots > 0 {
		return fmt.Sprintf("候选不够（%d 个，还能下 %d 个），", have, n.slots)
	}
	return "没有候选，"
}

// brushSlots 是这一轮还能同时下载几个；没设同时下载上限时为 0。
func brushSlots(task *models.BrushTask, downloading int) int {
	if task.MaxDownloading > 0 {
		return max(task.MaxDownloading-downloading, 0)
	}
	return 0
}

// brushStopsSite 报告读列表遇到的这个错误是不是这一轮都不该再请求这个站点：登录失效、二次验证、Cloudflare 质询、限流。
func brushStopsSite(err error) bool {
	for _, target := range []error{
		v2.ErrSessionExpired, v2.ErrInvalidCredentials, v2.ErrAuthFailed,
		v2.ErrCloudflareChallenge, v2.ErrRateLimited, v2.ErrCircuitOpen,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// brushSleep 等 d；ctx 先结束时提前返回它的错误。
func brushSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// brushListLabel 是运行记录里的一张列表的一页：「hdsky 的免费列表第 1 页」「hdsky 的 2x 免费列表第 2 页」
// （「的」后面是字母或数字时空一格）。
func brushListLabel(site, name string, page int) string {
	sep := ""
	if r, _ := utf8.DecodeRuneInString(name); r < utf8.RuneSelf {
		sep = " "
	}
	return fmt.Sprintf("%s 的%s%s第 %d 页", site, sep, name, page)
}

// brushListState 是一张列表这一轮读到了哪里。
type brushListState struct {
	list  brushList
	pages int
	// done 表示不再往后读：空页、和上一页一样、读失败了
	done bool
	// ids 是上一页的种子编号
	ids map[string]bool
}

// brushListFailure 是读失败的一页。
type brushListFailure struct {
	// label 是失败的是哪张列表、第几页（「综合区」「免费列表第 1 页」）；只读一张列表的第 1 页时为空
	label string
	err   error
	// stop 表示站点不让再读了（见 brushStopsSite）
	stop bool
}

func (f brushListFailure) error() error {
	if f.label == "" {
		return f.err
	}
	return fmt.Errorf("%s: %w", f.label, f.err)
}

// brushLister 是一轮读列表的进度；一轮在一个 goroutine 里跑，不用加锁。
type brushLister struct {
	m        *BrushMonitor
	site     v2.Site
	siteName string
	res      *BrushRunResult
	// mteam 为真时按分区读：运行记录写成一步（res.Scope），不写每一页
	mteam bool
	// multi 为真时计划里不止一张列表（失败时写明是哪一张）
	multi    bool
	interval time.Duration
	// start 是读列表开始的时刻，waited 是一共等了多久（不算进 brushListBudget），lastRead 是上一次读完的时刻
	start    time.Time
	waited   time.Duration
	lastRead time.Time
	// reads 是读了几次（含失败的），okReads 是读成功几次；halted 为真时这一轮不再读
	reads, okReads int
	halted         bool

	items    []v2.TorrentItem
	index    map[string]int
	sections []string
	failed   []brushListFailure
}

// listBrushItems 读这一轮的种子列表（计划见 brushListPlan），返回按种子编号去重后的种子（同一个种子留最近读到的那份）。
//
// 先读每张列表的第 1 页；need 不为空、并且它说候选还不够时，按顺序给能翻页、第 1 页不是空的列表补读下一页，每读一页重新数，
// 够了就停。每张列表最多 brushMaxListPages 页，一轮最多 brushMaxListReads 次。站点要求的读列表间隔（BrushList.MinInterval）
// 在两次读之间等够；读列表一共最多 brushListBudget（不算等的），整轮剩下的时间不够等完间隔、读一页、再留出加种的时间时不再读。
// 一张列表、一页读失败时记下是哪张、第几页，用读到的接着往下走；全都读失败才让这一轮失败。登录失效、质询、限流时这一轮
// 不再读这个站点。
func (m *BrushMonitor) listBrushItems(ctx context.Context, site v2.Site, task models.BrushTask, plan []brushList, need *brushNeed, res *BrushRunResult) ([]v2.TorrentItem, error) {
	l := &brushLister{
		m: m, site: site, siteName: task.SiteName, res: res,
		mteam: site.Kind() == v2.SiteMTorrent, multi: len(plan) > 1,
		start: m.cfg.Clock.Now(), index: map[string]int{},
	}
	if spec := v2.BrushListOf(site); spec != nil {
		l.interval = spec.MinInterval
	}
	states := make([]*brushListState, len(plan))
	for i, list := range plan {
		states[i] = &brushListState{list: list}
	}
	for _, st := range states {
		if !l.readPage(ctx, st, "") {
			break
		}
	}
	if need != nil && !l.halted {
	paging:
		for _, st := range states {
			for !st.done && st.list.pageable && st.pages < brushMaxListPages {
				have := need.count(l.items)
				if !need.short(have) {
					break paging
				}
				if !l.readPage(ctx, st, need.reason(have)) {
					break paging
				}
			}
		}
	}
	return l.finish()
}

// readPage 读 st 的下一页；why 是补页的原因（第 1 页为空）。返回 false（并记下 halted）表示这一轮不再读列表：站点不让读了、
// 时间不够、读满了次数、这一轮结束了；调用方收到 false 以后不再调它。
func (l *brushLister) readPage(ctx context.Context, st *brushListState, why string) bool {
	page := st.pages + 1
	if l.reads >= brushMaxListReads {
		l.halted = true
		l.res.step(brushStepInfo, "这一轮已经读了 %d 次列表，不再读 %s", l.reads, brushListLabel(l.siteName, st.list.name, page))
		return false
	}
	timeout, ok := l.ready(ctx, st, page)
	if !ok {
		l.halted = true
		return false
	}
	q := st.list.query
	if page > 1 {
		q.Page = page
	}
	searchCtx, cancel := context.WithTimeout(ctx, timeout)
	got, err := l.site.Search(searchCtx, q)
	cancel()
	l.reads++
	l.lastRead = l.m.cfg.Clock.Now()
	st.pages = page
	if err != nil {
		st.done = true
		f := brushListFailure{err: err, stop: brushStopsSite(err)}
		switch {
		case l.mteam && l.multi:
			f.label = st.list.name
		case !l.mteam && (l.multi || page > 1):
			f.label = fmt.Sprintf("%s第 %d 页", st.list.name, page)
		}
		l.failed = append(l.failed, f)
		if f.stop || ctx.Err() != nil {
			l.halted = true
			return false
		}
		return true
	}
	l.okReads++
	ids := make(map[string]bool, len(got))
	for _, it := range got {
		ids[it.ID] = true
	}
	same := page > 1 && len(got) > 0 && maps.Equal(ids, st.ids)
	st.ids = ids
	if len(got) == 0 || same {
		st.done = true
	}
	for _, it := range got {
		if i, seen := l.index[it.ID]; seen {
			l.items[i] = it // 同一个种子留最近读到的那份（优惠状态更新）
			continue
		}
		l.index[it.ID] = len(l.items)
		l.items = append(l.items, it)
	}
	if l.mteam {
		l.sections = append(l.sections, st.list.name)
		return true
	}
	suffix, note := "", ""
	if st.list.local {
		suffix = "（本地按优惠类型挑）"
	}
	if same {
		note = fmt.Sprintf("，和第 %d 页的一样", page-1)
	}
	l.res.step(brushStepList, "%s读取 %s%s：%d 个%s", why, brushListLabel(l.siteName, st.list.name, page), suffix, len(got), note)
	return true
}

// ready 在读下一页之前等够站点要求的间隔（从上一次读完算起），返回这一次读的时限；时间不够时记一步、返回 false（不等）。
// 时限不超过读列表剩下的时间（brushListBudget 减去已经读掉的，等的不算），也不超过整轮剩下的时间减去等的与留给加种的
// （brushAdmitReserve）；不到 brushListMinRead 就不读。第一次读不用等，时限与以前每轮读一次一样。
func (l *brushLister) ready(ctx context.Context, st *brushListState, page int) (time.Duration, bool) {
	if l.reads == 0 {
		return min(brushSearchTimeout, brushListBudget), true
	}
	if ctx.Err() != nil {
		return 0, false
	}
	now := l.m.cfg.Clock.Now()
	var gap time.Duration
	if l.interval > 0 {
		gap = max(l.interval-now.Sub(l.lastRead), 0)
	}
	timeout := min(brushSearchTimeout, brushListBudget-(now.Sub(l.start)-l.waited))
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline)-gap-brushAdmitReserve)
	}
	if timeout < brushListMinRead {
		l.res.step(brushStepInfo, "这一轮剩下的时间不够，不再读 %s", brushListLabel(l.siteName, st.list.name, page))
		return 0, false
	}
	if gap > 0 {
		before := l.m.cfg.Clock.Now()
		if err := l.m.cfg.Sleep(ctx, gap); err != nil {
			return 0, false
		}
		l.waited += l.m.cfg.Clock.Now().Sub(before)
		l.res.step(brushStepInfo, "等 %d 秒再读（%s 两次读列表至少隔 %d 秒）", ceilSeconds(gap), l.siteName, ceilSeconds(l.interval))
	}
	return timeout, true
}

func ceilSeconds(d time.Duration) int {
	return int((d + time.Second - 1) / time.Second)
}

// finish 收尾：一页都没读到时这一轮失败；读到了就把失败的页记成错误（这一轮算部分失败），写上读到的范围与去重后的个数。
func (l *brushLister) finish() ([]v2.TorrentItem, error) {
	// 第一次读一定会发出去（见 ready），所以一页都没读到时至少有一个失败
	if l.okReads == 0 {
		if len(l.failed) == 1 {
			return nil, fmt.Errorf("读取 %s 的种子列表失败: %w", l.siteName, l.failed[0].error())
		}
		msgs := make([]string, 0, len(l.failed))
		for _, f := range l.failed {
			msgs = append(msgs, f.error().Error())
		}
		return nil, fmt.Errorf("读取 %s 的种子列表失败: %s", l.siteName, strings.Join(msgs, "；"))
	}
	for _, f := range l.failed {
		then := "这一轮只用读到的列表"
		switch {
		case f.stop:
			then = "这一轮不再读这个站点的列表，只用已经读到的"
		case l.mteam:
			then = "这一轮只用读到的分区"
		}
		l.res.addError(fmt.Sprintf("读取 %s 的种子列表失败（%v），%s", l.siteName, f.error(), then))
	}
	switch {
	case l.mteam:
		l.res.Scope = "M-Team " + strings.Join(l.sections, "、") + "第 1 页"
		l.res.step(brushStepList, "读取 %s：%d 个", l.res.Scope, len(l.items))
	case l.okReads > 1:
		l.res.step(brushStepList, "去重后共 %d 个", len(l.items))
	}
	return l.items, nil
}
