package subscribe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/notify"
	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// pushSource 写进种子记录的下载来源。
const pushSource = "subscription"

// maxPushesPerRound 是每次评估最多推送几个种子（剧集一集一集补时，没推成的也算）；测试里会调小。
var maxPushesPerRound = 10

// 主动搜索每一拍最多搜几个订阅；推送下载种子文件的超时（站点驱动遇到一时的网络错误会自己重试几次，留够时间）；
// 同一站点这一轮连着几次没下载到种子文件以后，这一轮不再从它下载。
const (
	maxSearchPerTick   = 5
	fetchTimeout       = 2 * time.Minute
	searchTimeout      = 90 * time.Second
	searchJitter       = 30 * time.Minute
	siteFetchFailLimit = 2
)

// retrySoonBase 是一轮没做完（有种子文件没下载下来、时间用完了、推满了一轮的上限）以后多久再搜：
// 前面连着几次运行都没全成时翻倍，最多翻三次（4 小时）。平常的下次时间更早时按平常的。
const retrySoonBase = 30 * time.Minute

func siteHR(site string) (bool, *v2.SiteDefinition) {
	def, ok := v2.GetDefinitionRegistry().Get(site)
	if !ok {
		return false, nil
	}
	return def.HREnabled, def
}

// siteAllowed 报告订阅能不能用这个站点的种子。
func siteAllowed(sub *models.MediaSubscription, site string) bool {
	sites := decodeList(sub.Sites)
	return len(sites) == 0 || slices.ContainsFunc(sites, func(s string) bool { return strings.EqualFold(s, site) })
}

func (s *Service) activeSubs(ctx context.Context) ([]models.MediaSubscription, error) {
	var rows []models.MediaSubscription
	if err := s.cfg.DB.WithContext(ctx).Where("status = ?", models.MediaSubActive).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读取订阅失败: %w", err)
	}
	return rows, nil
}

// considerOffer 拿 RSS 交来的一个种子对所有在找的订阅。
func (s *Service) considerOffer(ctx context.Context, c Candidate) {
	set, err := s.Settings(ctx)
	if err != nil {
		return
	}
	subs, err := s.activeSubs(ctx)
	if err != nil || len(subs) == 0 {
		return
	}
	c.Meta = s.parse(ctx, c.Item.Title, c.Item.Subtitle)
	c.SiteHR, _ = siteHR(c.Site)
	for i := range subs {
		sub := &subs[i]
		if !siteAllowed(sub, c.Site) {
			continue
		}
		if _, ok := newMatcher(*sub).match(c.Meta, c.Item); !ok {
			continue
		}
		rctx, run := s.startRun(ctx, sub, models.MediaRunRSS)
		s.mu.Lock()
		msg := s.evaluate(rctx, sub, []Candidate{c}, set)
		s.mu.Unlock()
		// 种子文件没下载下来：不等平常的下次搜索，提前主动搜一次
		if soon, why := s.retrySoon(ctx, sub, runOf(rctx)); !soon.IsZero() && s.pullSearch(ctx, sub.ID, soon) {
			step(rctx, stepInfo, "%s，%s 主动搜一次", why, soon.In(time.Local).Format("01-02 15:04"))
		}
		// RSS 一天能送来很多个：只有真的做了什么（推送、拒绝、等版本）才留下这次的记录
		s.finishRun(rctx, sub, run, "RSS 里的 "+c.Item.Title+"："+msg, true)
		s.cfg.Logger.Infof("[订阅] RSS 里的 %s 对上了订阅「%s」：%s", c.Item.Title, sub.Title, msg)
	}
}

// searchDue 主动搜索到期的订阅（每一拍最多几个，按到期时间先后）。
func (s *Service) searchDue(ctx context.Context, set Settings) {
	var subs []models.MediaSubscription
	if err := s.cfg.DB.WithContext(ctx).
		Where("status = ? AND (next_search_at IS NULL OR next_search_at <= ?)", models.MediaSubActive, s.cfg.Now()).
		Order("next_search_at").Limit(maxSearchPerTick).Find(&subs).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 读取到期的订阅失败: %v", err)
		return
	}
	for i := range subs {
		if ctx.Err() != nil {
			return
		}
		s.searchOne(ctx, &subs[i], set, models.MediaRunSchedule)
	}
}

// SearchNow 立即搜索一个订阅，返回结果说明。
func (s *Service) SearchNow(ctx context.Context, id uint) (string, error) {
	sub, err := s.subRow(ctx, id)
	if err != nil {
		return "", err
	}
	if sub.Status != models.MediaSubActive {
		return "", fmt.Errorf("%w: 订阅没在找资源（暂停、待确认或已完成）", ErrInvalid)
	}
	set, err := s.Settings(ctx)
	if err != nil {
		return "", err
	}
	return s.searchOne(ctx, &sub, set, models.MediaRunManual), nil
}

// keywords 是主动搜索用的关键字：拉丁字母的名字（原名或英文别名）与中文名，最多两个。拉丁字母的名字剧集加季（S02）、
// 电影加年份；中文名不加（中文标题多写成「第二季」「第2期」，加了 S02 搜不到）。
func keywords(sub *models.MediaSubscription) []string {
	latin := ""
	for _, n := range append([]string{sub.OriginalTitle}, decodeList(sub.Aliases)...) {
		if n != "" && isLatin(n) {
			latin = n
			break
		}
	}
	var out []string
	for _, n := range []string{latin, sub.Title} {
		n = strings.TrimSpace(strings.NewReplacer(":", " ", "：", " ", "-", " ").Replace(n))
		if n == "" {
			continue
		}
		switch {
		case !isLatin(n):
		case sub.MediaType == models.MediaKindTV:
			n = fmt.Sprintf("%s S%02d", n, sub.Season)
		case sub.Year > 0:
			n = fmt.Sprintf("%s %d", n, sub.Year)
		}
		if !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

func isLatin(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) && r > unicode.MaxLatin1 {
			return false
		}
	}
	return true
}

// searchSites 是主动搜索的站点：订阅限定的站点（没有限定时是全部），去掉设置里不参与主动搜索的。nil 表示不限。
func (s *Service) searchSites(sub *models.MediaSubscription, set Settings) []string {
	sites := decodeList(sub.Sites)
	if len(set.SearchSkipSites) == 0 {
		return sites
	}
	if len(sites) == 0 && s.cfg.SiteNames != nil {
		sites = s.cfg.SiteNames()
	}
	out := []string{}
	for _, site := range sites {
		if !slices.ContainsFunc(set.SearchSkipSites, func(x string) bool { return strings.EqualFold(x, site) }) {
			out = append(out, site)
		}
	}
	return out
}

// searchOne 搜一个订阅并评估结果；先把下次搜索的时间排好（出错也不会每一拍都搜）。返回结果说明并写进订阅，
// 这次运行写进运行记录（trigger 是怎么开始的）。按播出时间的追剧搜完以后按新的台账重排下次时间；这一轮没做完的
// （种子文件没下载下来、时间用完了、推满了上限）提前再搜。
func (s *Service) searchOne(ctx context.Context, sub *models.MediaSubscription, set Settings, trigger string) string {
	now := s.cfg.Now()
	var lg *ledger
	if sub.MediaType == models.MediaKindTV && sub.ScheduleKind == models.MediaScheduleAirtime {
		lg, _ = s.ledgerRows(ctx, sub)
	}
	next := s.nextSearch(sub, set, lg, now)
	s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).
		Updates(map[string]any{"last_search_at": now, "next_search_at": next})
	rctx, run := s.startRun(ctx, sub, trigger)
	msg := s.search(rctx, sub, set)
	// 手动搜索有期限：期限到了这次的结果照样记下
	bctx := context.WithoutCancel(ctx)
	upd := map[string]any{"message": truncate(msg, 1024), "updated_at": s.cfg.Now()}
	if sub.MediaType == models.MediaKindTV && sub.ScheduleKind == models.MediaScheduleAirtime {
		if lg, err := s.ledgerRows(bctx, sub); err == nil {
			next = s.nextSearch(sub, set, lg, now)
			upd["next_search_at"] = next
		}
	}
	if soon, why := s.retrySoon(bctx, sub, runOf(rctx)); !soon.IsZero() && soon.Before(next) {
		upd["next_search_at"] = soon
		step(rctx, stepInfo, "%s，%s 再搜一次", why, soon.In(time.Local).Format("01-02 15:04"))
	}
	s.cfg.DB.WithContext(bctx).Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).Updates(upd)
	s.finishRun(rctx, sub, run, msg, false)
	return msg
}

// retrySoon 是这一轮没做完时提前的下次搜索时间与原因（retrySoonBase 起，前面连着没全成的运行每多一次翻一倍，最多翻三次）；
// 做完了的返回零值。
func (s *Service) retrySoon(ctx context.Context, sub *models.MediaSubscription, rl *runLog) (time.Time, string) {
	if rl == nil {
		return time.Time{}, ""
	}
	rl.mu.Lock()
	why := rl.unfinished
	if rl.fetchFailed > 0 {
		why = fmt.Sprintf("%d 个种子文件没下载下来", rl.fetchFailed)
	}
	rl.mu.Unlock()
	if why == "" {
		return time.Time{}, ""
	}
	// 这一次还没写完（状态为空）；手动改动不是搜索
	var last []models.MediaSubscriptionRun
	n := 0
	if err := s.cfg.DB.WithContext(ctx).Select("id", "status").
		Where("subscription_id = ? AND status NOT IN ?", sub.ID, []string{"", models.MediaRunChanged}).
		Order("id DESC").Limit(3).Find(&last).Error; err == nil {
		for _, r := range last {
			if r.Status != models.MediaRunFailed && r.Status != models.MediaRunPartial {
				break
			}
			n++
		}
	}
	return s.cfg.Now().Add(retrySoonBase << n), why
}

// pullSearch 把订阅的下次搜索提前到 at（已经更早的、马上要搜的不动），报告改了没有。时间按 julianday 比较
// （库里的时间是带时区的文本）。
func (s *Service) pullSearch(ctx context.Context, id uint, at time.Time) bool {
	res := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).
		Where("id = ? AND next_search_at IS NOT NULL AND julianday(next_search_at) > julianday(?)", id, at).
		Update("next_search_at", at)
	return res.Error == nil && res.RowsAffected > 0
}

func (s *Service) search(ctx context.Context, sub *models.MediaSubscription, set Settings) string {
	if s.cfg.Search == nil {
		return "搜索服务没有启动"
	}
	sites := s.searchSites(sub, set)
	if sites != nil && len(sites) == 0 {
		return "没有可以主动搜索的站点（都在不参与主动搜索的名单里）"
	}
	var cands []Candidate
	seen := map[string]bool{}
	var errs []string
	for _, kw := range keywords(sub) {
		sctx, cancel := context.WithTimeout(ctx, searchTimeout)
		searchedAt := s.cfg.Now()
		res, err := s.cfg.Search.Search(sctx, v2.MultiSiteSearchQuery{SearchQuery: v2.SearchQuery{Keyword: kw}, Sites: sites, Timeout: searchTimeout, RawTitles: true})
		cancel()
		if err != nil {
			errs = append(errs, fmt.Sprintf("搜索「%s」失败：%v", kw, err))
			step(ctx, stepFail, "搜索「%s」失败：%v", kw, err)
			markRun(ctx, func(rl *runLog) { rl.searchFailed++ })
			continue
		}
		step(ctx, stepSearch, "搜索「%s」：%d 个结果", kw, len(res.Items))
		for _, e := range res.Errors {
			errs = append(errs, fmt.Sprintf("%s：%s", e.Site, e.Error))
			step(ctx, stepFail, "%s 搜索出错：%s", e.Site, e.Error)
		}
		for _, it := range res.Items {
			key := it.SourceSite + "|" + it.ID
			if seen[key] || !siteAllowed(sub, it.SourceSite) {
				continue
			}
			seen[key] = true
			hr, _ := siteHR(it.SourceSite)
			cands = append(cands, Candidate{Site: it.SourceSite, Item: it, Meta: s.parse(ctx, it.Title, it.Subtitle), From: fromSearch, SiteHR: hr, SeenAt: searchedAt})
		}
	}
	s.mu.Lock()
	msg := s.evaluate(ctx, sub, cands, set)
	s.mu.Unlock()
	if len(errs) > 0 {
		msg += "（" + strings.Join(errs[:min(len(errs), 3)], "；") + "）"
	}
	return msg
}

type scored struct {
	c  Candidate
	v  Verdict
	sp span
}

// evaluate 从候选种子里挑这个订阅要的并推送（调用方持有 s.mu），返回结果说明。剧集先重建台账（认领下载器里已有的种子），
// 候选的集号按集号映射对到 TMDB 的集。
func (s *Service) evaluate(ctx context.Context, sub *models.MediaSubscription, cands []Candidate, set Settings) string {
	fresh, err := s.subRow(ctx, sub.ID)
	if err != nil || fresh.Status != models.MediaSubActive {
		return "订阅没在找资源"
	}
	*sub = fresh
	profile := s.profileFor(ctx, sub, set)
	mt := newMatcher(*sub)
	var lg *ledger
	if sub.MediaType == models.MediaKindTV {
		if lg, err = s.rebuildLedger(ctx, sub, rebuildOpts{set: set, adopt: true}); err != nil {
			return "读不到这一季的分集：" + err.Error()
		}
		mt = mt.withMapper(s.mapperFor(ctx, sub, lg.season))
	}
	linked, err := s.torrents(ctx, sub.ID)
	if err != nil {
		return err.Error()
	}
	var ok []scored
	var rejects []string
	byKeywords := false
	for _, c := range cands {
		if slices.ContainsFunc(linked, func(t models.MediaSubscriptionTorrent) bool {
			return strings.EqualFold(t.SiteName, c.Site) && t.TorrentID == c.Item.ID
		}) {
			continue
		}
		sp, match, why := mt.matchMapped(c.Meta, c.Item)
		if !match {
			if why != "" {
				rejects = append(rejects, c.Item.Title+"："+why)
				step(ctx, stepReject, "%s：%s", c.Item.Title, why)
			}
			continue
		}
		if why := keywordReject(sub, c.Item); why != "" {
			byKeywords = true
			rejects = append(rejects, c.Item.Title+"："+why)
			step(ctx, stepReject, "%s：%s", c.Item.Title, why)
			continue
		}
		v := profile.Score(c, sp.count(sub.TotalEpisodes))
		if v.Reject != "" {
			rejects = append(rejects, c.Item.Title+"："+v.Reject)
			step(ctx, stepReject, "%s：%s", c.Item.Title, v.Reject)
			continue
		}
		ok = append(ok, scored{c: c, v: v, sp: sp})
	}
	if len(ok) == 0 {
		if len(rejects) > 0 {
			what := "质量档案"
			if byKeywords {
				what = "质量档案或标题关键词"
			}
			return fmt.Sprintf("找到 %d 个，都不符合%s：%s", len(rejects), what, strings.Join(rejects[:min(len(rejects), 3)], "；"))
		}
		return "没有找到对得上的资源"
	}
	sort.SliceStable(ok, func(i, j int) bool { return profile.better(ok[i].c, ok[j].c, ok[i].v, ok[j].v) })
	active := slices.ContainsFunc(linked, func(t models.MediaSubscriptionTorrent) bool {
		return t.Status == models.MediaSubTorrentDownloading || t.Status == models.MediaSubTorrentDone
	})
	downloading := slices.ContainsFunc(linked, func(t models.MediaSubscriptionTorrent) bool { return t.Status == models.MediaSubTorrentDownloading })
	if sub.MediaType == models.MediaKindMovie {
		vers, err := s.libraryVersions(ctx, sub)
		if err != nil {
			return err.Error()
		}
		if !sub.Upgrade {
			if active {
				return "已经下载过了"
			}
			if len(vers) > 0 {
				return "库里已经有了"
			}
			return s.pushOne(ctx, sub, ok[0], set, nil)
		}
		if downloading {
			return "洗版：上一个版本还在下载"
		}
		// 现在的版本（库里的与订阅下载的）按当前档案重算：失败的下载不算，档案改过也能比
		if base, has := s.baseline(ctx, profile, linked, vers); has && ok[0].v.Score <= base {
			return "洗版：没有比现在更好的版本"
		}
		return s.pushOne(ctx, sub, ok[0], set, nil)
	}
	msg := s.pickTV(ctx, sub, profile, ok, set, lg)
	// 推送了的话按新的种子重建台账（锁住的版本也可能刚写上）：详情与进度马上看得到这几集在下载
	if after, err := s.torrents(ctx, sub.ID); err == nil && len(after) > len(linked) {
		if fresh, err := s.subRow(ctx, sub.ID); err == nil {
			*sub = fresh
		}
		if _, err := s.rebuildLedger(ctx, sub, rebuildOpts{set: set}); err != nil {
			s.cfg.Logger.Debugf("[订阅] 推送以后重建「%s」的追剧台账失败: %v", sub.Title, err)
		}
	}
	return msg
}

// pickTV 挑剧集的种子：这一季（必需集）播完以后，缺一半以上或缺的集没有单集可补时下载整季包，否则一集一集补；
// 洗版时（必需集都下载过了）只看比现在分数高的整季包。开着版本锁时补缺的集只要锁住的版本（见 lock.go）。
func (s *Service) pickTV(ctx context.Context, sub *models.MediaSubscription, profile Profile, ok []scored, set Settings, lg *ledger) string {
	var packs, eps []scored
	for _, o := range ok {
		if o.sp.Whole {
			packs = append(packs, o)
		} else {
			eps = append(eps, o)
		}
	}
	airedAll := lg.requiredAired()
	if len(lg.missing) == 0 {
		downloadingN := 0
		for _, n := range lg.required {
			if r := lg.row(n); r != nil && r.State == models.MediaEpisodeDownloading {
				downloadingN++
			}
		}
		if !sub.Upgrade || !airedAll || downloadingN > 0 {
			if downloadingN > 0 {
				step(ctx, stepInfo, "缺的集都在下载")
				return "缺的集都在下载"
			}
			return "没有缺的集"
		}
		// 剧集各集的版本可能有好有差：整季包只要能让一集变好就换（比它好的集入库以后留着）；洗版的整季包不受版本锁限制
		vers, err := s.libraryVersions(ctx, sub)
		if err != nil {
			return err.Error()
		}
		if len(packs) == 0 || packs[0].v.Score <= s.episodeFloor(ctx, profile, vers, lg.required) {
			return "洗版：没有比现在更好的整季包"
		}
		return s.pushOne(ctx, sub, packs[0], set, nil)
	}
	if lg.listErr != "" {
		step(ctx, stepWait, "这一轮不推缺的集（免得重复下载正在下载的集）：%s", lg.listErr)
		markRun(ctx, func(rl *runLog) { rl.waiting = true })
		return "读不到下载器里的种子，这一轮不推缺的集：" + lg.listErr
	}
	missing := map[int]bool{}
	for _, n := range lg.missing {
		missing[n] = true
	}
	// 「修改」里改锁时给用户看：这次搜到的哪些版本能补上缺的集（锁住的版本等不到时选一个）
	s.recordSeenVersions(ctx, sub, ok, missing)
	coversMissing := func(o scored) bool {
		for n := range missing {
			if o.sp.covers(n) {
				return true
			}
		}
		return false
	}
	// 这一集换版本（用户放开了锁）：单集的候选只覆盖放开了的集时不受锁限制
	unlockedOnly := func(o scored) bool {
		if o.sp.Whole {
			return false
		}
		hit := false
		for n := range missing {
			if !o.sp.covers(n) {
				continue
			}
			if r := lg.row(n); r == nil || !r.ManualUnlock {
				return false
			}
			hit = true
		}
		return hit
	}
	// 整季包只在这一季播完以后要（播出中的整季包多半不全）：缺一半以上，或者缺的集没有单集可补
	wantPack := func(packs, eps []scored) bool {
		return len(packs) > 0 && airedAll && (len(missing)*2 >= lg.aired || !slices.ContainsFunc(eps, coversMissing))
	}
	var lockG, lockR string
	locked, round := false, false
	if sub.LockVersion {
		s.ensureLock(ctx, sub)
		lockG, lockR, locked = lockOf(sub)
		if !locked {
			// 还没有版本：按这一轮第一个会推送的候选定一个本轮锁（推成功以后才写进库）
			var first *scored
			if wantPack(packs, eps) {
				first = &packs[0]
			} else if i := slices.IndexFunc(eps, coversMissing); i >= 0 {
				first = &eps[i]
			}
			if first != nil {
				lockG, lockR, locked = versionOf(first.c.Meta)
				round = locked
			}
		}
	}
	filterLock := func(list []scored, g, r string) []scored {
		out := list[:0:0]
		for _, o := range list {
			if lockAllows(g, r, o.c.Meta) || unlockedOnly(o) {
				out = append(out, o)
				continue
			}
			if coversMissing(o) {
				step(ctx, stepLock, "%s：版本锁，要 %s", o.c.Item.Title, lockText(g, r))
			}
		}
		return out
	}
	if locked {
		before := len(packs) + len(eps)
		packs, eps = filterLock(packs, lockG, lockR), filterLock(eps, lockG, lockR)
		s.markVersionSeen(ctx, sub, lockG, lockR, missing, packs, eps)
		if len(packs)+len(eps) == 0 && before > 0 {
			markRun(ctx, func(rl *runLog) { rl.waiting = true })
			return fmt.Sprintf("缺 %d 集，找到的都不是锁住的版本（%s），接着等", len(missing), lockText(lockG, lockR))
		}
	}
	// pushed 推送成功以后：本轮锁写进库；之后推送前重读库里的锁（用户中途改了锁时立即生效）
	pushed := func() {
		if round {
			s.writeLock(ctx, sub, lockG, lockR)
			round = false
		}
	}
	allowedNow := func(o scored) bool {
		if !sub.LockVersion {
			return true
		}
		s.reloadLock(ctx, sub)
		g, r, ok := lockOf(sub)
		if !ok {
			return true
		}
		if lockAllows(g, r, o.c.Meta) || unlockedOnly(o) {
			return true
		}
		step(ctx, stepLock, "%s：版本锁改成了 %s，不推这个", o.c.Item.Title, lockText(g, r))
		return false
	}
	if wantPack(packs, eps) && allowedNow(packs[0]) {
		msg := s.pushOne(ctx, sub, packs[0], set, missing)
		if strings.HasPrefix(msg, "下载了") {
			pushed()
		}
		return msg
	}
	var msgs []string
	down := map[string]bool{}
	for _, o := range eps {
		if len(missing) == 0 {
			break
		}
		if len(msgs) >= maxPushesPerRound {
			if slices.ContainsFunc(eps, coversMissing) {
				markRun(ctx, func(rl *runLog) { rl.unfinished = fmt.Sprintf("这一轮推满了 %d 个", maxPushesPerRound) })
			}
			break
		}
		if ctx.Err() != nil {
			step(ctx, stepWait, "这一轮的时间用完了，没推的集下次再推")
			markRun(ctx, func(rl *runLog) { rl.unfinished = "这一轮的时间用完了" })
			break
		}
		if !coversMissing(o) || !allowedNow(o) {
			continue
		}
		if siteFetchDown(ctx, o.c.Site) {
			if !down[o.c.Site] {
				down[o.c.Site] = true
				step(ctx, stepWait, "%s 连着 %d 次没下载到种子文件，这一轮先不从它下载", o.c.Site, siteFetchFailLimit)
			}
			continue
		}
		msg := s.pushOne(ctx, sub, o, set, missing)
		msgs = append(msgs, msg)
		if strings.HasPrefix(msg, "下载了") {
			pushed()
			for n := range missing {
				if o.sp.covers(n) {
					delete(missing, n)
				}
			}
		}
	}
	if len(msgs) == 0 {
		return fmt.Sprintf("缺 %d 集，没有找到这些集的资源", len(lg.missing))
	}
	return strings.Join(msgs, "；")
}

// markVersionSeen 记下这一轮在站点上找到了锁住的版本的缺集（不算等版本，见 rebuildLedger）。
func (s *Service) markVersionSeen(ctx context.Context, sub *models.MediaSubscription, g, r string, missing map[int]bool, lists ...[]scored) {
	var eps []int
	for n := range missing {
		for _, list := range lists {
			if slices.ContainsFunc(list, func(o scored) bool { return o.sp.covers(n) && lockAllows(g, r, o.c.Meta) }) {
				eps = append(eps, n)
				break
			}
		}
	}
	if len(eps) == 0 {
		return
	}
	seen := lockText(g, r)
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscriptionEpisode{}).
		Where("subscription_id = ? AND episode IN ? AND version_seen <> ?", sub.ID, eps, seen).
		Update("version_seen", seen).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 记下找到的版本失败: %v", err)
	}
}

// downloaderFor 是推送用的下载器：订阅自己选的，没有时用设置里的默认下载器，再没有时用 pt-tools 的默认下载器。
func (s *Service) downloaderFor(ctx context.Context, sub *models.MediaSubscription, set Settings) (uint, error) {
	for _, id := range []uint{sub.DownloaderID, set.DefaultDownloaderID} {
		if id == 0 {
			continue
		}
		var n int64
		if err := s.cfg.DB.WithContext(ctx).Model(&models.DownloaderSetting{}).Where("id = ? AND enabled = ?", id, true).Count(&n).Error; err == nil && n > 0 {
			return id, nil
		}
	}
	var ds models.DownloaderSetting
	if err := s.cfg.DB.WithContext(ctx).Where("enabled = ?", true).Order("is_default DESC, id").Limit(1).Find(&ds).Error; err != nil {
		return 0, fmt.Errorf("读取下载器失败: %w", err)
	}
	if ds.ID == 0 {
		return 0, errors.New("没有启用的下载器")
	}
	return ds.ID, nil
}

// pushOne 下载种子文件并推送到下载器，记下种子并发通知。返回结果说明（以「下载了」开头表示成功）；结果写进这次运行的步骤。
// pushOne 推送一个候选。want 是这一轮要的集（补缺的集时）：种子里有几集时只下这些集；为 nil 时整个种子都下（电影、洗版）。
func (s *Service) pushOne(ctx context.Context, sub *models.MediaSubscription, o scored, set Settings, want map[int]bool) string {
	msg := s.pushTorrent(ctx, sub, o, set, want)
	switch {
	case strings.HasPrefix(msg, "下载了"):
		step(ctx, stepPush, "%s", msg)
		markRun(ctx, func(rl *runLog) { rl.pushed = append(rl.pushed, o.c.Item.Title) })
	case strings.HasPrefix(msg, noWantedPrefix):
		step(ctx, stepReject, "%s", msg)
	default:
		step(ctx, stepFail, "%s", msg)
		markRun(ctx, func(rl *runLog) { rl.pushFailed++ })
	}
	return msg
}

func (s *Service) pushTorrent(ctx context.Context, sub *models.MediaSubscription, o scored, set Settings, want map[int]bool) string {
	if s.cfg.Sites == nil || s.cfg.Push == nil {
		return "推送服务没有启动"
	}
	site, ok := s.cfg.Sites(o.c.Site)
	if !ok {
		return fmt.Sprintf("站点 %s 没有启用，没有下载 %s", o.c.Site, o.c.Item.Title)
	}
	dlID, err := s.downloaderFor(ctx, sub, set)
	if err != nil {
		return err.Error()
	}
	data, err := fetchTorrent(ctx, site, o.c.Item)
	markRun(ctx, func(rl *runLog) { rl.fetched(o.c.Site, err == nil) })
	if err != nil {
		return fmt.Sprintf("下载 %s 的种子文件失败：%v", o.c.Item.Title, err)
	}
	hash, err := qbit.ComputeTorrentHash(data)
	if err != nil {
		return fmt.Sprintf("%s 的种子文件无法解析：%v", o.c.Item.Title, err)
	}
	hash = strings.ToLower(hash)
	// 种子里有几集（整季包、多集）时：视频都对得出集、却没有一集是要的，不推；推送以后把不要的集设成不下载。
	// 开着洗版的订阅整个下：库里已有的集也按每一集比版本，好的换掉旧版本
	var plan *filePlan
	if want != nil && !sub.Upgrade && sub.MediaType == models.MediaKindTV && (o.sp.Whole || o.sp.End > o.sp.Start) {
		if plan = s.planFiles(ctx, sub, data, want); plan != nil && plan.none() {
			return fmt.Sprintf("%s%s：种子里没有缺的集（种子里是 %s）", noWantedPrefix, o.c.Item.Title, episodeList(plan.all))
		}
	}
	size := o.c.Item.SizeBytes
	if size <= 0 {
		if n, serr := qbit.ComputeTorrentSize(data); serr == nil {
			size = n
		}
	}
	hasHR, def := siteHR(o.c.Site)
	hasHR = hasHR || o.c.Item.HasHR
	hrHours := 0
	if hasHR {
		hrHours = ptinternal.HRSeedTimeH(o.c.Site, def, size)
	}
	// 只有免费的结束时间才是免费到期：半价、2X 这类优惠的结束时间写进去，免费到期清理会删掉没下完的种子
	var freeEnd *time.Time
	if o.c.Item.IsFree() && !o.c.Item.DiscountEndTime.IsZero() {
		fe := o.c.Item.DiscountEndTime.UTC()
		freeEnd = &fe
	}
	// 下载种子文件可能要一会儿：推送前再看一眼，这期间订阅被暂停或删掉了就不推
	if cur, cerr := s.subRow(ctx, sub.ID); cerr != nil || cur.Status != models.MediaSubActive {
		return fmt.Sprintf("订阅已经暂停或删除，没有推送 %s", o.c.Item.Title)
	}
	policy, snap := s.admission(ctx, sub, set, site, o.c)
	res, err := s.cfg.Push(ctx, ptinternal.PushTorrentRequest{
		SiteID: o.c.Site, TorrentID: o.c.Item.ID, TorrentData: data, Title: o.c.Item.Title,
		Category: sub.Category, Tags: sub.Tags, SavePath: sub.SavePath, DownloaderID: dlID, Source: pushSource,
		IMDbID: o.c.Item.IMDbID, DoubanID: o.c.Item.DoubanID,
		Meta: &ptinternal.PushTorrentMeta{
			SizeBytes: size, HasHR: hasHR, HRSeedTimeH: hrHours,
			IsFree: o.c.Item.IsFree(), FreeLevel: string(o.c.Item.DiscountLevel), FreeEndTime: freeEnd,
		},
		Policy:   policy,
		Snapshot: snap,
	})
	if err != nil {
		return fmt.Sprintf("推送 %s 失败：%v", o.c.Item.Title, err)
	}
	// 磁盘空间、站点做种容量这些闸门拒绝时不报错，只是没有成功：和失败一样不记下，下次还能再试
	if res == nil || !res.Success {
		reason := "下载器没有接收"
		if res != nil && res.Message != "" {
			reason = res.Message
		}
		return fmt.Sprintf("推送 %s 失败：%s", o.c.Item.Title, reason)
	}
	var kept []int
	if plan != nil && len(plan.unwanted) > 0 && !res.Skipped {
		got, n, serr := s.selectFiles(ctx, dlID, hash, plan.mp, want)
		switch {
		case serr == nil && n > 0:
			kept = got
			step(ctx, stepInfo, "只下缺的 %s：种子里另外 %d 个视频文件（%s）设成了不下载", episodeList(got), n, episodeList(plan.unwanted))
		case errors.Is(serr, downloader.ErrCapabilityUnsupported):
			step(ctx, stepInfo, "下载器不支持只下种子里的部分文件，整个种子都会下载（%s 不缺）", episodeList(plan.unwanted))
		case serr != nil:
			step(ctx, stepWait, "没能把不缺的集设成不下载（%v），整个种子都会下载", serr)
		}
	}
	now := s.cfg.Now()
	row := models.MediaSubscriptionTorrent{
		SubscriptionID: sub.ID, SiteName: o.c.Site, TorrentID: o.c.Item.ID, InfoHash: hash, Title: truncate(o.c.Item.Title, 512),
		Subtitle: truncate(o.c.Item.Subtitle, 512), Score: o.v.Score, Episode: o.sp.Start, EpisodeEnd: o.sp.End, Complete: o.sp.Whole,
		SizeBytes: size, DownloaderID: dlID, HasHR: hasHR, Adopted: res.Skipped,
		Status: models.MediaSubTorrentDownloading, CreatedAt: now, UpdatedAt: now,
	}
	if res.Skipped {
		row.Message = "下载器里已经有这个种子"
	}
	switch {
	case len(kept) > 0:
		// 只下了缺的集：种子里只算这几集
		b, _ := json.Marshal(kept)
		row.CoveredEpisodes = string(b)
	case o.sp.Whole && sub.MediaType == models.MediaKindTV:
		// 整季包按种子的文件列表核对实际有哪几集：缺的集下一轮照常一集一集补
		eps := []int(nil)
		if plan != nil {
			eps = plan.all
		} else {
			eps = s.packEpisodes(ctx, sub, data)
		}
		if len(eps) > 0 {
			b, _ := json.Marshal(eps)
			row.CoveredEpisodes = string(b)
		}
	}
	if err := s.cfg.DB.WithContext(ctx).Create(&row).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 记下订阅的种子失败: %v", err)
	}
	s.notify(ctx, sub, o, set, kept)
	if len(kept) > 0 {
		return "下载了 " + o.c.Item.Title + "（只下 " + episodeList(kept) + "）"
	}
	return "下载了 " + o.c.Item.Title
}

// packEpisodes 是整季包里实际有的集：按种子的文件列表里每个视频文件的文件名解析（含集号映射）。读不出时为空（按整季算）。
func (s *Service) packEpisodes(ctx context.Context, sub *models.MediaSubscription, data []byte) []int {
	files, err := reseed.TorrentFiles(data)
	if err != nil {
		return nil
	}
	mp := s.fileMapper(ctx, sub)
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return s.fileEpisodes(ctx, mp, paths)
}

// fileEpisodes 是种子里这些文件（相对路径）对得上的集：只看视频文件，按文件名认集号（综艺经集号映射，mp 可以为空）；
// 对不上的、写的是整季的不算。
func (s *Service) fileEpisodes(ctx context.Context, mp *episodeMapper, paths []string) []int {
	got := map[int]bool{}
	for _, p := range paths {
		eps, _ := s.videoEpisodes(ctx, mp, p)
		for _, n := range eps {
			got[n] = true
		}
	}
	return slices.Sorted(maps.Keys(got))
}

// fetchTorrent 经站点实例下载种子文件（走站点限速）；HDDolby 这类要 hash 的站点从下载地址里取 downhash。
// admission 是这次推送的入场策略与资格快照。档案要求免费时推送要求免费：搜索结果可能来自缓存，
// 所以推送前按站点详情重新读一次（读不到就没有快照，门禁会拒绝）；RSS 送来的候选用取详情时的资格。
func (s *Service) admission(ctx context.Context, sub *models.MediaSubscription, set Settings, site v2.Site, c Candidate) (ptinternal.AdmissionPolicy, *v2.PromotionSnapshot) {
	policy := ptinternal.AdmissionPolicy{
		RequireFree:  s.profileFor(ctx, sub, set).Free == models.MediaPrefRequire,
		SafetyMargin: ptinternal.DefaultSafetyMargin(),
		SourceKind:   ptinternal.SourceKindSubscribe,
		AttemptStart: c.SeenAt,
		Clock:        s.cfg.Now,
	}
	if policy.RequireFree && c.From == fromSearch {
		policy.AttemptStart = s.cfg.Now()
		return policy, ptinternal.FreshPromotion(ctx, site, c.Site, c.Item.ID, policy.AttemptStart)
	}
	verified := c.Item.FetchedAt
	if verified.IsZero() {
		verified = c.SeenAt
	}
	snap := c.Item.Promotion(verified)
	return policy, &snap
}

// offerSeenAt 是 RSS 送来的候选的取得时间：详情带了取得时间就用它，否则用送来的时间。
func offerSeenAt(item v2.TorrentItem, now time.Time) time.Time {
	if !item.FetchedAt.IsZero() {
		return item.FetchedAt
	}
	return now
}

func fetchTorrent(ctx context.Context, site v2.Site, it v2.TorrentItem) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	if hd, ok := site.(v2.HashDownloader); ok && it.DownloadURL != "" {
		if u, err := url.Parse(it.DownloadURL); err == nil {
			if h := u.Query().Get("downhash"); h != "" {
				return hd.DownloadWithHash(ctx, it.ID, h)
			}
		}
	}
	return site.Download(ctx, it.ID)
}

// notify 下载了订阅的资源时发一条通知（设置里没选通道时不发）。追剧（剧集按自己的定时搜索的）发「追剧更新」，其余发「订阅下载」。
// notify 发下载了的通知；kept 不为空时是种子里只下了这几集。
func (s *Service) notify(ctx context.Context, sub *models.MediaSubscription, o scored, set Settings, kept []int) {
	if s.cfg.Notify == nil || len(set.NotifyChannels) == 0 {
		return
	}
	title := subTitle(sub)
	var b strings.Builder
	fmt.Fprintf(&b, "种子：%s\n站点：%s", o.c.Item.Title, o.c.Site)
	episodes := ""
	switch {
	case len(kept) > 0 && o.sp.Whole:
		episodes = episodesText(kept) + "（整季包里只下这几集）"
	case len(kept) > 0:
		episodes = episodesText(kept) + "（种子里只下这几集）"
	case o.sp.Whole:
		episodes = "整季"
	case o.sp.Start > 0 && o.sp.End > o.sp.Start:
		episodes = fmt.Sprintf("第 %d–%d 集", o.sp.Start, o.sp.End)
	case o.sp.Start > 0:
		episodes = fmt.Sprintf("第 %d 集", o.sp.Start)
	}
	if episodes != "" {
		b.WriteString("\n" + episodes)
	}
	q := strings.TrimSpace(strings.Join([]string{o.c.Meta.Resolution, o.c.Meta.Source, o.c.Meta.VideoCodec}, " "))
	if q != "" {
		fmt.Fprintf(&b, "\n质量：%s", q)
	}
	n := Notice{
		Channels: set.NotifyChannels, Kind: NoticeDownloaded, Subject: o.c.Site + "|" + o.c.Item.ID,
		EventKey: fmt.Sprintf("sub-%d-%s-%s", sub.ID, o.c.Site, o.c.Item.ID), Title: "订阅下载：" + title, Body: b.String(),
	}
	if sub.PosterPath != "" {
		// 用户换了 TMDB 图片地址时，通知里的海报也用换的地址
		if c, err := s.cfg.Recognizer.TMDB(ctx); err == nil {
			n.ImageURL = c.PosterURL(sub.PosterPath, "w500")
		} else {
			n.ImageURL = tmdb.PosterURL(sub.PosterPath, "w500")
		}
	}
	if isFollow(sub) {
		version := ""
		if g, r, ok := lockOf(sub); ok {
			version = lockText(g, r)
		}
		var fb strings.Builder
		if episodes != "" {
			fb.WriteString(episodes + "\n")
		}
		fmt.Fprintf(&fb, "种子：%s\n站点：%s", o.c.Item.Title, o.c.Site)
		if q != "" {
			fmt.Fprintf(&fb, "\n质量：%s", q)
		}
		if version != "" {
			fmt.Fprintf(&fb, "\n版本：%s", version)
		}
		n.Kind, n.Title, n.Body = NoticeFollowFound, "追剧更新："+title, fb.String()
		n.Event = notify.NewEvent(notify.EventFollowFound, s.cfg.Now(), map[string]any{
			"subscription": title, "season": sub.Season, "episodes": episodes, "torrent_title": o.c.Item.Title,
			"site": o.c.Site, "quality": q, "version": version, "poster": n.ImageURL,
		})
	} else {
		n.Event = notify.NewEvent(notify.EventSubscribeDownloaded, s.cfg.Now(), map[string]any{
			"subscription": title, "media_type": sub.MediaType, "year": sub.Year, "season": sub.Season, "episodes": episodes,
			"torrent_title": o.c.Item.Title, "site": o.c.Site, "quality": q, "poster": n.ImageURL,
		})
	}
	if err := s.cfg.Notify(ctx, n); err != nil {
		s.cfg.Logger.Warnf("[订阅] 发通知失败: %v", err)
	}
}
