package subscribe

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader/qbit"
)

// pushSource 写进种子记录的下载来源。
const pushSource = "subscription"

// 每次评估最多推送几个种子（剧集一集一集补时）；主动搜索每一拍最多搜几个订阅；推送下载种子文件的超时。
const (
	maxPushesPerRound = 10
	maxSearchPerTick  = 5
	fetchTimeout      = 60 * time.Second
	searchTimeout     = 90 * time.Second
	searchJitter      = 30 * time.Minute
)

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
		s.mu.Lock()
		msg := s.evaluate(ctx, sub, []Candidate{c}, set)
		s.mu.Unlock()
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
		s.searchOne(ctx, &subs[i], set)
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
	return s.searchOne(ctx, &sub, set), nil
}

// keywords 是主动搜索用的关键字：拉丁字母的名字（原名或英文别名）与中文名，剧集加季（S02），电影加年份；最多两个。
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
		case sub.MediaType == models.MediaKindTV:
			n = fmt.Sprintf("%s S%02d", n, sub.Season)
		case sub.Year > 0 && isLatin(n):
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

// searchOne 搜一个订阅并评估结果；先把下次搜索的时间排好（出错也不会每一拍都搜）。返回结果说明并写进订阅。
func (s *Service) searchOne(ctx context.Context, sub *models.MediaSubscription, set Settings) string {
	now := s.cfg.Now()
	next := now.Add(time.Duration(set.SearchIntervalHours)*time.Hour + s.cfg.Jitter(searchJitter))
	s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).
		Updates(map[string]any{"last_search_at": now, "next_search_at": next})
	msg := s.search(ctx, sub, set)
	s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscription{}).Where("id = ?", sub.ID).
		Updates(map[string]any{"message": truncate(msg, 1024), "updated_at": s.cfg.Now()})
	return msg
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
		res, err := s.cfg.Search.Search(sctx, v2.MultiSiteSearchQuery{SearchQuery: v2.SearchQuery{Keyword: kw}, Sites: sites, Timeout: searchTimeout, RawTitles: true})
		cancel()
		if err != nil {
			errs = append(errs, fmt.Sprintf("搜索「%s」失败：%v", kw, err))
			continue
		}
		for _, e := range res.Errors {
			errs = append(errs, fmt.Sprintf("%s：%s", e.Site, e.Error))
		}
		for _, it := range res.Items {
			key := it.SourceSite + "|" + it.ID
			if seen[key] || !siteAllowed(sub, it.SourceSite) {
				continue
			}
			seen[key] = true
			hr, _ := siteHR(it.SourceSite)
			cands = append(cands, Candidate{Site: it.SourceSite, Item: it, Meta: s.parse(ctx, it.Title, it.Subtitle), From: fromSearch, SiteHR: hr})
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

// evaluate 从候选种子里挑这个订阅要的并推送（调用方持有 s.mu），返回结果说明。
func (s *Service) evaluate(ctx context.Context, sub *models.MediaSubscription, cands []Candidate, set Settings) string {
	fresh, err := s.subRow(ctx, sub.ID)
	if err != nil || fresh.Status != models.MediaSubActive {
		return "订阅没在找资源"
	}
	*sub = fresh
	profile := s.profileFor(ctx, sub, set)
	mt := newMatcher(*sub)
	linked, err := s.torrents(ctx, sub.ID)
	if err != nil {
		return err.Error()
	}
	var ok []scored
	var rejects []string
	for _, c := range cands {
		if slices.ContainsFunc(linked, func(t models.MediaSubscriptionTorrent) bool {
			return strings.EqualFold(t.SiteName, c.Site) && t.TorrentID == c.Item.ID
		}) {
			continue
		}
		sp, match := mt.match(c.Meta, c.Item)
		if !match {
			continue
		}
		v := profile.Score(c, sp.count(sub.TotalEpisodes))
		if v.Reject != "" {
			rejects = append(rejects, c.Item.Title+"："+v.Reject)
			continue
		}
		ok = append(ok, scored{c: c, v: v, sp: sp})
	}
	if len(ok) == 0 {
		if len(rejects) > 0 {
			return fmt.Sprintf("找到 %d 个，都不符合质量档案：%s", len(rejects), strings.Join(rejects[:min(len(rejects), 3)], "；"))
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
			return s.pushOne(ctx, sub, ok[0], set)
		}
		if downloading {
			return "洗版：上一个版本还在下载"
		}
		// 现在的版本（库里的与订阅下载的）按当前档案重算：失败的下载不算，档案改过也能比
		if base, has := s.baseline(ctx, profile, linked, vers); has && ok[0].v.Score <= base {
			return "洗版：没有比现在更好的版本"
		}
		return s.pushOne(ctx, sub, ok[0], set)
	}
	return s.pickTV(ctx, sub, profile, ok, linked, set)
}

// pickTV 挑剧集的种子：这一季播完以后，缺一半以上或缺的集没有单集可补时下载整季包，否则一集一集补；
// 洗版时（这一季都下载过了）只看比现在分数高的整季包。
func (s *Service) pickTV(ctx context.Context, sub *models.MediaSubscription, profile Profile, ok []scored, linked []models.MediaSubscriptionTorrent, set Settings) string {
	p, err := s.progress(ctx, sub, false)
	if err != nil {
		return "读不到这一季的分集：" + err.Error()
	}
	var packs, eps []scored
	for _, o := range ok {
		if o.sp.Whole {
			packs = append(packs, o)
		} else {
			eps = append(eps, o)
		}
	}
	airedAll := p.Total > 0 && p.Aired == p.Total
	if len(p.Missing) == 0 {
		if !sub.Upgrade || !airedAll || p.Downloading > 0 {
			if p.Downloading > 0 {
				return "缺的集都在下载"
			}
			return "没有缺的集"
		}
		// 剧集各集的版本可能有好有差：整季包只要能让一集变好就换（比它好的集入库以后留着）
		vers, err := s.libraryVersions(ctx, sub)
		if err != nil {
			return err.Error()
		}
		if len(packs) == 0 || packs[0].v.Score <= s.episodeFloor(ctx, profile, vers, p.Total) {
			return "洗版：没有比现在更好的整季包"
		}
		return s.pushOne(ctx, sub, packs[0], set)
	}
	missing := map[int]bool{}
	for _, n := range p.Missing {
		missing[n] = true
	}
	coversMissing := func(o scored) bool {
		for n := range missing {
			if o.sp.covers(n) {
				return true
			}
		}
		return false
	}
	// 整季包只在这一季播完以后要（播出中的整季包多半不全）：缺一半以上，或者缺的集没有单集可补
	epAvailable := slices.ContainsFunc(eps, coversMissing)
	if len(packs) > 0 && airedAll && (len(p.Missing)*2 >= p.Aired || !epAvailable) {
		return s.pushOne(ctx, sub, packs[0], set)
	}
	var msgs []string
	for _, o := range eps {
		if len(msgs) >= maxPushesPerRound || len(missing) == 0 {
			break
		}
		if !coversMissing(o) {
			continue
		}
		msg := s.pushOne(ctx, sub, o, set)
		msgs = append(msgs, msg)
		if strings.HasPrefix(msg, "下载了") {
			for n := range missing {
				if o.sp.covers(n) {
					delete(missing, n)
				}
			}
		}
	}
	if len(msgs) == 0 {
		return fmt.Sprintf("缺 %d 集，没有找到这些集的资源", len(p.Missing))
	}
	return strings.Join(msgs, "；")
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

// pushOne 下载种子文件并推送到下载器，记下种子并发通知。返回结果说明（以「下载了」开头表示成功）。
func (s *Service) pushOne(ctx context.Context, sub *models.MediaSubscription, o scored, set Settings) string {
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
	if err != nil {
		return fmt.Sprintf("下载 %s 的种子文件失败：%v", o.c.Item.Title, err)
	}
	hash, err := qbit.ComputeTorrentHash(data)
	if err != nil {
		return fmt.Sprintf("%s 的种子文件无法解析：%v", o.c.Item.Title, err)
	}
	hash = strings.ToLower(hash)
	size := o.c.Item.SizeBytes
	if size <= 0 {
		if n, serr := qbit.ComputeTorrentSize(data); serr == nil {
			size = n
		}
	}
	hasHR, def := siteHR(o.c.Site)
	hasHR = hasHR || o.c.Item.HasHR
	hrHours := 0
	if hasHR && def != nil {
		hrHours = def.CalcHRSeedTimeH(size)
	}
	var freeEnd *time.Time
	if !o.c.Item.DiscountEndTime.IsZero() {
		fe := o.c.Item.DiscountEndTime.UTC()
		freeEnd = &fe
	}
	res, err := s.cfg.Push(ctx, ptinternal.PushTorrentRequest{
		SiteID: o.c.Site, TorrentID: o.c.Item.ID, TorrentData: data, Title: o.c.Item.Title,
		Category: sub.Category, Tags: sub.Tags, SavePath: sub.SavePath, DownloaderID: dlID, Source: pushSource,
		IMDbID: o.c.Item.IMDbID, DoubanID: o.c.Item.DoubanID,
		Meta: &ptinternal.PushTorrentMeta{
			SizeBytes: size, HasHR: hasHR, HRSeedTimeH: hrHours,
			IsFree: o.c.Item.IsFree(), FreeLevel: string(o.c.Item.DiscountLevel), FreeEndTime: freeEnd,
		},
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
	if err := s.cfg.DB.WithContext(ctx).Create(&row).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 记下订阅的种子失败: %v", err)
	}
	s.notify(ctx, sub, o, set)
	return "下载了 " + o.c.Item.Title
}

// fetchTorrent 经站点实例下载种子文件（走站点限速）；HDDolby 这类要 hash 的站点从下载地址里取 downhash。
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

// notify 下载了订阅的资源时发一条通知（设置里没选通道时不发）。
func (s *Service) notify(ctx context.Context, sub *models.MediaSubscription, o scored, set Settings) {
	if s.cfg.Notify == nil || len(set.NotifyChannels) == 0 {
		return
	}
	title := sub.Title
	if sub.Year > 0 {
		title = fmt.Sprintf("%s (%d)", title, sub.Year)
	}
	if sub.MediaType == models.MediaKindTV {
		title += fmt.Sprintf(" 第 %d 季", sub.Season)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "种子：%s\n站点：%s", o.c.Item.Title, o.c.Site)
	switch {
	case o.sp.Whole:
		b.WriteString("\n整季")
	case o.sp.Start > 0 && o.sp.End > o.sp.Start:
		fmt.Fprintf(&b, "\n第 %d–%d 集", o.sp.Start, o.sp.End)
	case o.sp.Start > 0:
		fmt.Fprintf(&b, "\n第 %d 集", o.sp.Start)
	}
	if q := strings.TrimSpace(strings.Join([]string{o.c.Meta.Resolution, o.c.Meta.Source, o.c.Meta.VideoCodec}, " ")); q != "" {
		fmt.Fprintf(&b, "\n质量：%s", q)
	}
	n := Notice{
		Channels: set.NotifyChannels, Kind: NoticeDownloaded, Subject: o.c.Site + "|" + o.c.Item.ID,
		EventKey: fmt.Sprintf("sub-%d-%s-%s", sub.ID, o.c.Site, o.c.Item.ID), Title: "订阅下载：" + title, Body: b.String(),
	}
	if sub.PosterPath != "" {
		n.ImageURL = tmdb.PosterURL(sub.PosterPath, "w500")
	}
	if err := s.cfg.Notify(ctx, n); err != nil {
		s.cfg.Logger.Warnf("[订阅] 发通知失败: %v", err)
	}
}
