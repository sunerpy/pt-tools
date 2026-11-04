package subscribe

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	ptinternal "github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/serverindex"
	"github.com/sunerpy/pt-tools/models"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// 条目详情里的「删除一部片」（媒体中心 M7）：预览一部电影、剧集的一季（或其中几集）在库里的文件、关联的种子与订阅，
// 再按用户勾的删。种子按（下载器编号, hash）认，确认也按这一对；执行时不信预览：重新读下载器、重新算保护（和自动删种共用的规则），
// 连数据删时同一台下载器里还有别的种子用着这份数据的只删种子。每一步单独给结果，不自动删任何东西。

// 保护规则（Protection.Rule），同 scheduler 的 GuardKeepTag 等（cmd 里接 SeedGuard 时原样转过来）。
const (
	GuardKeepTag     = "keep_tag"
	GuardProtectTag  = "protect_tag"
	GuardHR          = "hr"
	GuardDownloading = "downloading"
	GuardMinRetain   = "min_retain"
)

// Protection 是种子命中的一条保护规则（同 scheduler.Protection）：Rule 为空表示不受保护；Text 是完整的原因；Tag 是命中的标签；
// Hours 是 H&R 要求的做种小时数（0 是要求不明）或最短保种时间；Left 是还差的秒数。
type Protection struct {
	Rule  string
	Text  string
	Tag   string
	Hours int
	Left  int64
}

// Guard 是删种的保护规则（生产环境是 scheduler.SeedGuard：和自动删种共用全局设置里「自动删种」的保护项）。
type Guard interface {
	Check(t downloader.Torrent) Protection
	// Rules 是开着的规则（条目详情的说明用）
	Rules() []string
}

// GuardFunc 按现在的设置建保护规则；返回错误时分不清哪些种子受保护，一律当作受保护、不删。
type GuardFunc func(ctx context.Context) (Guard, error)

// LibraryDeleter 是删除一部片要用的整理入库的动作（生产环境是 *organize.Service）。
type LibraryDeleter interface {
	LibraryFiles(ctx context.Context, q organize.FileQuery) ([]organize.LibraryFile, error)
	DeleteHistoryWith(ctx context.Context, id uint, opt organize.DeleteOptions) ([]string, error)
	KeepLinks(ctx context.Context, ids []uint) error
}

func (s *Service) guard(ctx context.Context) (Guard, error) {
	if s.cfg.Guard == nil {
		return nil, errors.New("保护规则没有接上")
	}
	return s.cfg.Guard(ctx)
}

// guardView 按保护规则填种子能不能删（g 为空时一律受保护）：Reason 是「能不能删」那一列的短字，Protection 是完整的原因。
func guardView(v *ItemTorrent, t downloader.Torrent, g Guard) Protection {
	if g == nil {
		v.Deletable, v.Reason, v.Protection = false, "受保护：读不到保护规则", "读不到删种的保护规则，不知道能不能删"
		return Protection{Rule: "unknown", Text: v.Protection}
	}
	p := g.Check(t)
	v.Deletable, v.Protection = p.Rule == "", p.Text
	switch p.Rule {
	case "":
		v.Reason = "可以删"
		if v.HR && v.HRHours > 0 && v.SeedingSeconds >= int64(v.HRHours)*3600 {
			v.Reason = "H&R 已满足"
		}
	case GuardKeepTag, GuardProtectTag:
		v.Reason = "受保护：标签 " + p.Tag
	case GuardHR:
		v.Reason = "受保护：H&R 要求不明"
		if p.Hours > 0 && p.Left > 0 {
			v.Reason = "H&R 还差 " + leftText(p.Left)
		}
	case GuardDownloading:
		v.Reason = "受保护：下载中"
	case GuardMinRetain:
		v.Reason = fmt.Sprintf("受保护：加进来不到 %d 小时", p.Hours)
	default:
		v.Reason = "受保护"
	}
	return p
}

// DeleteSelector 选要删的：一部电影；剧集的一季，Episodes 不为空时只是其中几集。
type DeleteSelector struct {
	MediaType string `json:"media_type"`
	TMDBID    int    `json:"tmdb_id"`
	Season    int    `json:"season"`
	Episodes  []int  `json:"episodes"`
}

// maxDeleteEpisodes 是一次最多选几集
const maxDeleteEpisodes = 500

func (sel DeleteSelector) normalized() (DeleteSelector, error) {
	switch sel.MediaType {
	case models.MediaKindMovie:
		if sel.Season != 0 || len(sel.Episodes) > 0 {
			return sel, fmt.Errorf("%w: 电影没有季与集", ErrInvalid)
		}
	case models.MediaKindTV:
		if sel.Season < 0 || sel.Season > 200 {
			return sel, fmt.Errorf("%w: 季号不对", ErrInvalid)
		}
	default:
		return sel, fmt.Errorf("%w: 类型要选电影或剧集", ErrInvalid)
	}
	if sel.TMDBID <= 0 {
		return sel, fmt.Errorf("%w: 要指定 TMDB 编号", ErrInvalid)
	}
	if len(sel.Episodes) > maxDeleteEpisodes {
		return sel, fmt.Errorf("%w: 一次最多选 %d 集", ErrInvalid, maxDeleteEpisodes)
	}
	eps := slices.Clone(sel.Episodes)
	for _, n := range eps {
		if n <= 0 || n > 10000 {
			return sel, fmt.Errorf("%w: 集号不对", ErrInvalid)
		}
	}
	slices.Sort(eps)
	sel.Episodes = slices.Compact(eps)
	return sel, nil
}

func (sel DeleteSelector) files() organize.FileQuery {
	return organize.FileQuery{MediaType: sel.MediaType, TMDBID: sel.TMDBID, Season: sel.Season, Episodes: sel.Episodes}
}

// inScope 报告关联的种子在不在选的范围里：剧集是这一季（不知道是哪一季的也算），选了集时种子里要有其中的集（整季包、
// 不知道是哪几集的也算）。
func (sel DeleteSelector) inScope(r *torrentRef) bool {
	if sel.MediaType != models.MediaKindTV {
		return true
	}
	if r.season != sel.Season && r.season != 0 {
		return false
	}
	if len(sel.Episodes) == 0 || r.whole || len(r.eps) == 0 {
		return true
	}
	return slices.ContainsFunc(r.eps, func(n int) bool { return slices.Contains(sel.Episodes, n) })
}

// coversMore 报告种子里是不是还有这次没选的集（或者说不准）：这样的种子预览里默认不勾，返回说明。
func (sel DeleteSelector) coversMore(r *torrentRef) (bool, string) {
	if sel.MediaType != models.MediaKindTV {
		return false, ""
	}
	if r.season == 0 {
		return true, "不知道是哪一季的种子"
	}
	if len(sel.Episodes) == 0 {
		return false, ""
	}
	switch {
	case r.whole:
		return true, "整季包，还有这次没选的集"
	case len(r.eps) == 0:
		return true, "不知道种子里是哪几集"
	}
	var more []int
	for _, n := range r.eps {
		if !slices.Contains(sel.Episodes, n) {
			more = append(more, n)
		}
	}
	if len(more) == 0 {
		return false, ""
	}
	return true, "种子里还有 " + spanLabel(more, false)
}

// issueInScope 报告问题是不是选的范围里的（剧集是这一季，选了集时是其中的集）。
func (sel DeleteSelector) issueInScope(is models.MediaIssue) bool {
	if sel.MediaType != models.MediaKindTV {
		return true
	}
	return is.Season == sel.Season && (len(sel.Episodes) == 0 || slices.Contains(sel.Episodes, is.Episode))
}

// DeletePreview 是删除预览：库里的文件（整理记录）、关联的种子、覆盖这一部的订阅与范围里还开着的问题。Errors 是没读到的部分：
// 读不到的下载器里的种子不列，读不到保护规则时种子一律当作受保护。
type DeletePreview struct {
	Files         []organize.LibraryFile `json:"files"`
	Torrents      []DeleteTorrent        `json:"torrents"`
	Subscriptions []ItemSubscription     `json:"subscriptions"`
	Issues        []models.MediaIssue    `json:"issues"`
	Errors        []string               `json:"errors"`
}

// DeleteTorrent 是删除预览里的一个关联种子：同条目详情的种子（能不能删按保护规则），另写命中的规则、种子里是不是还有这次没选的集
// （CoversMore，默认不勾），以及同一台下载器里和它共用数据的种子（Shares：要连数据删，得这几个也一起删）。
type DeleteTorrent struct {
	ItemTorrent
	Rule string `json:"rule,omitempty"`
	// Why 是受保护的短句，弹窗写在「受保护：」后面（完整的原因在 Protection）
	Why        string     `json:"why,omitempty"`
	CoversMore bool       `json:"covers_more"`
	More       string     `json:"more,omitempty"`
	Shares     []ShareRef `json:"shares"`
}

// protectWhy 是删除弹窗里受保护的短句（画板 03 的片段）：「H&R 要求做种 3 天，还差 2 天」「还在下载（63%）」。不受保护时为空。
func protectWhy(p Protection, t downloader.Torrent) string {
	switch p.Rule {
	case "":
		return ""
	case GuardKeepTag:
		return "带保留标签 " + p.Tag
	case GuardProtectTag:
		return "带保护标签 " + p.Tag
	case GuardHR:
		if p.Hours <= 0 {
			return "H&R 要求不明，一直保护"
		}
		need := fmt.Sprintf("%d 小时", p.Hours)
		if p.Hours%24 == 0 {
			need = fmt.Sprintf("%d 天", p.Hours/24)
		}
		if p.Left <= 0 {
			return "H&R 要求做种 " + need
		}
		return "H&R 要求做种 " + need + "，还差 " + leftText(p.Left)
	case GuardDownloading:
		if t.Progress < 1 {
			return fmt.Sprintf("还在下载（%d%%）", int(t.Progress*100))
		}
		return "还在校验"
	case GuardMinRetain:
		if p.Left <= 0 {
			return fmt.Sprintf("加进来不到 %d 小时", p.Hours)
		}
		return fmt.Sprintf("加进来不到 %d 小时，还差 %s", p.Hours, leftText(p.Left))
	}
	return p.Text
}

// ShareRef 是和一个种子共用数据的种子（同一台下载器）。
type ShareRef struct {
	Hash string `json:"hash"`
	Name string `json:"name"`
}

// deleteScope 是选的范围里的订阅与关联的种子（剧集只看这一季的订阅）。
func (s *Service) deleteScope(ctx context.Context, sel DeleteSelector) ([]models.MediaSubscription, []*torrentRef, error) {
	q := s.cfg.DB.WithContext(ctx).Where("media_type = ? AND tmdb_id = ?", sel.MediaType, sel.TMDBID)
	if sel.MediaType == models.MediaKindTV {
		q = q.Where("season = ?", sel.Season)
	}
	var subs []models.MediaSubscription
	if err := q.Order("id").Find(&subs).Error; err != nil {
		return nil, nil, fmt.Errorf("读取订阅失败: %w", err)
	}
	// 关联的种子按整部算（订阅的种子、推送记录、整理记录），再按季集筛：不知道是哪一季的种子也要列出来
	var all []models.MediaSubscription
	if err := s.cfg.DB.WithContext(ctx).Where("media_type = ? AND tmdb_id = ?", sel.MediaType, sel.TMDBID).Order("id").Find(&all).Error; err != nil {
		return nil, nil, fmt.Errorf("读取订阅失败: %w", err)
	}
	hist, err := s.itemHistory(ctx, sel.MediaType, sel.TMDBID)
	if err != nil {
		return nil, nil, err
	}
	refs, err := s.itemRefs(ctx, sel.MediaType, sel.TMDBID, all, hist)
	if err != nil {
		return nil, nil, err
	}
	out := refs[:0:0]
	for _, r := range refs {
		if sel.inScope(r) {
			out = append(out, r)
		}
	}
	return subs, out, nil
}

// PreviewDelete 是删除的预览（见 DeletePreview）：下载器的种子列表现读，不用缓存。
func (s *Service) PreviewDelete(ctx context.Context, in DeleteSelector) (*DeletePreview, error) {
	sel, err := in.normalized()
	if err != nil {
		return nil, err
	}
	if s.cfg.Deleter == nil {
		return nil, errors.New("整理入库服务没有启动")
	}
	out := &DeletePreview{Torrents: []DeleteTorrent{}, Subscriptions: []ItemSubscription{}, Issues: []models.MediaIssue{}, Errors: []string{}}
	if out.Files, err = s.cfg.Deleter.LibraryFiles(ctx, sel.files()); err != nil {
		return nil, err
	}
	subs, refs, err := s.deleteScope(ctx, sel)
	if err != nil {
		return nil, err
	}
	for i := range subs {
		sub := &subs[i]
		v := ItemSubscription{ID: sub.ID, Status: sub.Status, Season: sub.Season, Follow: isFollow(sub)}
		if g, r, ok := lockOf(sub); ok {
			v.Lock = &MediaSearchLock{Group: g, Resolution: r}
		}
		out.Subscriptions = append(out.Subscriptions, v)
	}
	g, gerr := s.guard(ctx)
	if gerr != nil {
		out.Errors = append(out.Errors, "读不到删种的保护规则："+gerr.Error()+"，种子一律当作受保护")
	}
	byDL, ids := refsByDownloader(refs)
	for _, id := range ids {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		live, lerr := s.freshTorrents(ctx, id)
		if lerr != nil {
			out.Errors = append(out.Errors, lerr.Error())
			continue
		}
		for _, r := range byDL[id] {
			t, ok := live.byHash[r.hash]
			if !ok {
				continue
			}
			v := torrentView(r, live.name)
			liveView(&v, t)
			p := guardView(&v, t, g)
			v.Version = versionLabel(s.parse(ctx, v.Title, ""))
			dt := DeleteTorrent{ItemTorrent: v, Rule: p.Rule, Why: protectWhy(p, t), Shares: []ShareRef{}}
			dt.CoversMore, dt.More = sel.coversMore(r)
			for _, o := range live.sharing.Sharers(t) {
				dt.Shares = append(dt.Shares, ShareRef{Hash: strings.ToLower(o.InfoHash), Name: o.Name})
			}
			out.Torrents = append(out.Torrents, dt)
		}
	}
	sortTorrents(out.Torrents, func(d DeleteTorrent) ItemTorrent { return d.ItemTorrent })
	issues, err := serverindex.OpenIssues(ctx, s.cfg.DB, sel.MediaType, sel.TMDBID)
	if err != nil {
		return nil, err
	}
	for _, is := range issues {
		if sel.issueInScope(is) {
			out.Issues = append(out.Issues, is)
		}
	}
	return out, nil
}

// refsByDownloader 把关联的种子按下载器分组（下载器按第一次出现的顺序）。
func refsByDownloader(refs []*torrentRef) (map[uint][]*torrentRef, []uint) {
	byDL := map[uint][]*torrentRef{}
	var ids []uint
	for _, r := range refs {
		if _, ok := byDL[r.downloaderID]; !ok {
			ids = append(ids, r.downloaderID)
		}
		byDL[r.downloaderID] = append(byDL[r.downloaderID], r)
	}
	return byDL, ids
}

// liveList 是现读的一台下载器的种子：按 hash（小写）查，带上判断共用数据的 DataSharing。
type liveList struct {
	dl      downloader.Downloader
	name    string
	byHash  map[string]downloader.Torrent
	sharing *downloader.DataSharing
}

// freshTorrents 现读一台下载器的全部种子（不用缓存）；连不上、读不到时返回写明下载器的错误。
func (s *Service) freshTorrents(ctx context.Context, id uint) (*liveList, error) {
	if s.cfg.Downloaders == nil {
		return nil, errors.New("下载器服务没有启动")
	}
	dl, setting, err := s.cfg.Downloaders.Get(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("连不上下载器（编号 %d）：%w", id, err)
	}
	all, err := dl.GetAllTorrents()
	if err != nil {
		return nil, fmt.Errorf("读不到下载器「%s」里的种子：%w", setting.Name, err)
	}
	m := make(map[string]downloader.Torrent, len(all))
	for _, t := range all {
		m[strings.ToLower(t.InfoHash)] = t
	}
	return &liveList{dl: dl, name: setting.Name, byHash: m, sharing: downloader.NewDataSharing(all, downloader.FilesOf(dl))}, nil
}

// TorrentKey 认一个下载器里的种子：（下载器编号, hash）。同一个 hash 在两台下载器上是两个种子。
type TorrentKey struct {
	DownloaderID uint   `json:"downloader_id"`
	Hash         string `json:"hash"`
}

func (k TorrentKey) normalized() TorrentKey {
	return TorrentKey{DownloaderID: k.DownloaderID, Hash: strings.ToLower(strings.TrimSpace(k.Hash))}
}

// DeleteTorrentReq 是要删的一个种子；DeleteData 为真时连数据删（还有别的种子用着这份数据时只删种子）。
type DeleteTorrentReq struct {
	TorrentKey
	DeleteData bool `json:"delete_data"`
}

// 订阅怎么办（DeleteRequest.Subscription）。
const (
	// DeleteSubPause 暂停（默认）：不暂停的话，台账会把删掉的集当成缺的，再去下一遍
	DeleteSubPause = "pause"
	// DeleteSubDelete 删除订阅：以后不再找
	DeleteSubDelete = "delete"
	// DeleteSubKeep 保留、继续追
	DeleteSubKeep = "keep"
)

// DeleteRequest 是删除的请求：选择器、要连文件删的整理记录（预览里看到的那些，空的不删库文件）、导入的已有文件也删、
// 要删的种子与受保护的种子里确认了要删的（按下载器编号与 hash）、订阅怎么办（为空时暂停）。
type DeleteRequest struct {
	DeleteSelector
	HistoryIDs       []uint             `json:"history_ids"`
	AllowExisting    bool               `json:"allow_existing"`
	Torrents         []DeleteTorrentReq `json:"torrents"`
	ConfirmProtected []TorrentKey       `json:"confirm_protected"`
	Subscription     string             `json:"subscription"`
}

// 删除的一步（DeleteStep.Kind）。
const (
	DeleteStepSubscription = "subscription"
	DeleteStepTorrent      = "torrent"
	DeleteStepFile         = "file"
)

// DeleteStep 是删除的一步：一个订阅、一个种子或一条整理记录（连库里的文件），各自成功或失败。
type DeleteStep struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	// ID 是订阅或整理记录的编号；种子写 DownloaderID 与 Hash
	ID           uint   `json:"id,omitempty"`
	DownloaderID uint   `json:"downloader_id,omitempty"`
	Hash         string `json:"hash,omitempty"`
	OK           bool   `json:"ok"`
	Message      string `json:"message"`
}

// DeleteResult 是删除的结果：每一步（订阅、种子、库文件的顺序）与失败了几步。
type DeleteResult struct {
	Steps  []DeleteStep `json:"steps"`
	Failed int          `json:"failed"`
}

func (r *DeleteResult) add(st DeleteStep) {
	r.Steps = append(r.Steps, st)
	if !st.OK {
		r.Failed++
	}
}

// DeleteItem 按请求删除（见 DeleteRequest）：先处理订阅（删的过程中订阅不会把删掉的集当成缺的再去下），再删种子，再删库里的文件。
// 种子在删除锁里（同自动删种）现读下载器、重新算保护：受保护又没确认的不删（预览以后才变成受保护的也一样）；不是这一部的不删。
// 每一步单独给结果，失败的不影响已经做完的。
func (s *Service) DeleteItem(ctx context.Context, req DeleteRequest) (*DeleteResult, error) {
	sel, err := req.normalized()
	if err != nil {
		return nil, err
	}
	action := req.Subscription
	if action == "" {
		action = DeleteSubPause
	}
	if action != DeleteSubPause && action != DeleteSubDelete && action != DeleteSubKeep {
		return nil, fmt.Errorf("%w: 订阅要选 pause、delete 或 keep", ErrInvalid)
	}
	if s.cfg.Deleter == nil {
		return nil, errors.New("整理入库服务没有启动")
	}
	var torrents []DeleteTorrentReq
	seen := map[TorrentKey]bool{}
	for _, t := range req.Torrents {
		t.TorrentKey = t.normalized()
		if t.DownloaderID == 0 || t.Hash == "" || len(t.Hash) > 128 {
			return nil, fmt.Errorf("%w: 种子要写下载器编号与 hash", ErrInvalid)
		}
		if !seen[t.TorrentKey] {
			seen[t.TorrentKey] = true
			torrents = append(torrents, t)
		}
	}
	confirmed := map[TorrentKey]bool{}
	for _, k := range req.ConfirmProtected {
		confirmed[k.normalized()] = true
	}
	ids := slices.Clone(req.HistoryIDs)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	subs, refs, err := s.deleteScope(ctx, sel)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 && len(torrents) == 0 && (action == DeleteSubKeep || len(subs) == 0) {
		return nil, fmt.Errorf("%w: 没有选要删的", ErrInvalid)
	}
	res := &DeleteResult{Steps: []DeleteStep{}}
	if action != DeleteSubKeep {
		for i := range subs {
			res.add(s.deleteSubStep(ctx, &subs[i], action))
		}
	}
	if len(torrents) > 0 {
		s.deleteTorrents(ctx, refs, torrents, confirmed, ids, res)
	}
	q := sel.files()
	for _, id := range ids {
		res.add(s.deleteFileStep(ctx, q, id, req.AllowExisting))
	}
	s.cfg.Logger.Infof("[媒体库] 网页上删除 %s %d%s：%s", sel.MediaType, sel.TMDBID, scopeLabel(sel), stepsSummary(res))
	return res, nil
}

// scopeLabel 是日志里写的范围：第几季、哪几集。
func scopeLabel(sel DeleteSelector) string {
	if sel.MediaType != models.MediaKindTV {
		return ""
	}
	out := fmt.Sprintf(" 第 %d 季", sel.Season)
	if len(sel.Episodes) > 0 {
		out += " " + spanLabel(sel.Episodes, false)
	}
	return out
}

// stepsSummary 是日志里写的每一步。
func stepsSummary(res *DeleteResult) string {
	parts := make([]string, 0, len(res.Steps))
	for _, st := range res.Steps {
		mark := "成功"
		if !st.OK {
			mark = "失败"
		}
		parts = append(parts, fmt.Sprintf("%s「%s」%s（%s）", st.Kind, st.Target, mark, st.Message))
	}
	return strings.Join(parts, "；")
}

// deleteSubStep 暂停或删除一个订阅。暂停的是在找的与完成的追剧（完成以后 30 天内 TMDB 加了集会重新打开，删了库里的文件以后它会把删掉的集
// 当成缺的）；原来就暂停的不动，待确认的（豆瓣想看建的）不动，完成的电影订阅不会再找，也不动。
func (s *Service) deleteSubStep(ctx context.Context, sub *models.MediaSubscription, action string) DeleteStep {
	st := DeleteStep{Kind: DeleteStepSubscription, Target: subscriptionTitle(sub), ID: sub.ID}
	if action == DeleteSubDelete {
		if err := s.DeleteSubscription(ctx, sub.ID); err != nil {
			st.Message = err.Error()
			return st
		}
		st.OK, st.Message = true, "删除了订阅"
		return st
	}
	switch {
	case sub.Status == models.MediaSubPaused:
		st.OK, st.Message = true, "原来就是暂停的"
	case sub.Status == models.MediaSubPending:
		st.OK, st.Message = true, "还没确认的订阅，没有动"
	case sub.Status == models.MediaSubDone && sub.MediaType != models.MediaKindTV:
		st.OK, st.Message = true, "已经完成了，不会再去下，没有动"
	default:
		if _, err := s.SetStatus(ctx, sub.ID, models.MediaSubPaused); err != nil {
			st.Message = err.Error()
			return st
		}
		st.OK, st.Message = true, "暂停了"
	}
	return st
}

// subscriptionTitle 是订阅给人看的名字（剧集带季）。
func subscriptionTitle(sub *models.MediaSubscription) string {
	if sub.MediaType == models.MediaKindTV {
		return fmt.Sprintf("%s 第 %d 季", sub.Title, sub.Season)
	}
	return sub.Title
}

// deleteTorrents 一台台下载器删种子：在删除锁里现读下载器的种子、重新算保护与共用数据，再删。
func (s *Service) deleteTorrents(ctx context.Context, refs []*torrentRef, torrents []DeleteTorrentReq, confirmed map[TorrentKey]bool, keepIDs []uint, res *DeleteResult) {
	related := map[TorrentKey]*torrentRef{}
	for _, r := range refs {
		related[TorrentKey{DownloaderID: r.downloaderID, Hash: r.hash}] = r
	}
	var order []uint
	byDL := map[uint][]DeleteTorrentReq{}
	for _, t := range torrents {
		if _, ok := byDL[t.DownloaderID]; !ok {
			order = append(order, t.DownloaderID)
		}
		byDL[t.DownloaderID] = append(byDL[t.DownloaderID], t)
	}
	g, gerr := s.guard(ctx)
	for _, id := range order {
		s.deleteOnDownloader(ctx, id, byDL[id], related, confirmed, keepIDs, g, gerr, res)
	}
}

// doomedTorrent 是复查以后要删的一个种子。
type doomedTorrent struct {
	req   DeleteTorrentReq
	t     downloader.Torrent
	title string
	note  string
}

func (s *Service) deleteOnDownloader(ctx context.Context, id uint, list []DeleteTorrentReq, related map[TorrentKey]*torrentRef,
	confirmed map[TorrentKey]bool, keepIDs []uint, g Guard, gerr error, res *DeleteResult,
) {
	step := func(t DeleteTorrentReq, title string) DeleteStep {
		if title == "" {
			title = t.Hash
		}
		return DeleteStep{Kind: DeleteStepTorrent, Target: title, DownloaderID: t.DownloaderID, Hash: t.Hash}
	}
	titleOf := func(t DeleteTorrentReq) string {
		if r := related[t.TorrentKey]; r != nil {
			return r.title
		}
		return ""
	}
	// 删除锁：和自动删种、刷流删种、网页上的「保留」串行（见 internal.RemovalMutex），锁里只做现读、复查、删
	mu := ptinternal.RemovalMutex()
	mu.Lock()
	defer mu.Unlock()
	live, err := s.freshTorrents(ctx, id)
	if err != nil {
		for _, t := range list {
			st := step(t, titleOf(t))
			st.Message = err.Error() + "，没有删"
			res.add(st)
		}
		return
	}
	var doomed []doomedTorrent
	for _, t := range list {
		title := titleOf(t)
		cur, ok := live.byHash[t.Hash]
		if ok && title == "" {
			title = cur.Name
		}
		st := step(t, title)
		switch {
		case related[t.TorrentKey] == nil:
			st.Message = "不是这一部的种子，没有删"
		case !ok:
			st.OK, st.Message = true, "下载器里已经没有这个种子了"
		case g == nil:
			st.Message = "读不到删种的保护规则，没有删"
			if gerr != nil {
				st.Message += "（" + gerr.Error() + "）"
			}
		default:
			p := g.Check(cur)
			if p.Rule != "" && !confirmed[t.TorrentKey] {
				st.Message = "受保护，没有删：" + protectWhy(p, cur)
				break
			}
			d := doomedTorrent{req: t, t: cur, title: st.Target}
			if p.Rule != "" {
				d.note = "受保护（" + protectWhy(p, cur) + "），确认了删除"
			}
			doomed = append(doomed, d)
			continue
		}
		res.add(st)
	}
	deleting := make(map[string]bool, len(doomed))
	for _, d := range doomed {
		deleting[downloader.TorrentKey(d.t)] = true
	}
	for _, d := range doomed {
		res.add(s.removeTorrent(ctx, live, d, deleting, keepIDs))
	}
}

// removeTorrent 删一个复查过的种子：要连数据删、但同一台下载器里还有别的种子（不算这次一起删的）用着这份数据时只删种子；
// 连数据删以前，这个种子整理出的、这次不删的硬链接记录标上留着（打开了「删种时一并删除入库链接」也不删）。
// 删掉的订阅种子还没入库的改成失败：「保留，继续追」时这几集马上重新算缺的。
func (s *Service) removeTorrent(ctx context.Context, live *liveList, d doomedTorrent, deleting map[string]bool, keepIDs []uint) DeleteStep {
	st := DeleteStep{Kind: DeleteStepTorrent, Target: d.title, DownloaderID: d.req.DownloaderID, Hash: d.req.Hash}
	withData, shared := d.req.DeleteData, false
	if withData && live.sharing.Shares(d.t, deleting) {
		withData, shared = false, true
	}
	if withData {
		if err := s.keepLinksOf(ctx, d.req.TorrentKey, keepIDs); err != nil {
			st.Message = "没有删：" + err.Error()
			return st
		}
	}
	if err := live.dl.RemoveTorrent(d.t.ID, withData); err != nil && !errors.Is(err, downloader.ErrTorrentNotFound) {
		st.Message = "删除失败：" + err.Error()
		return st
	}
	msg := "只删了种子，数据留着"
	switch {
	case withData:
		msg = "连数据删了"
	case shared:
		msg = "只删了种子：数据还有别的种子在用，留着"
	}
	if d.note != "" {
		msg += "；" + d.note
	}
	st.OK, st.Message = true, msg
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaSubscriptionTorrent{}).
		Where("downloader_id = ? AND lower(info_hash) = ? AND status = ?", d.req.DownloaderID, d.req.Hash, models.MediaSubTorrentDownloading).
		Updates(map[string]any{"status": models.MediaSubTorrentFailed, "message": "在条目详情里删了这个种子", "updated_at": s.cfg.Now()}).Error; err != nil {
		s.cfg.Logger.Warnf("[订阅] 更新删掉的种子的记录失败: %v", err)
	}
	return st
}

// keepLinksOf 把这个种子整理出的、这次不删的已整理记录标上留着库里的文件（见 organize.KeepLinks）。
func (s *Service) keepLinksOf(ctx context.Context, k TorrentKey, deleting []uint) error {
	var ids []uint
	if err := s.cfg.DB.WithContext(ctx).Model(&models.MediaTransferHistory{}).
		Where("downloader_id = ? AND lower(info_hash) = ? AND status = ?", k.DownloaderID, k.Hash, models.MediaTransferDone).
		Pluck("id", &ids).Error; err != nil {
		return fmt.Errorf("读取整理记录失败: %w", err)
	}
	ids = slices.DeleteFunc(ids, func(id uint) bool { return slices.Contains(deleting, id) })
	return s.cfg.Deleter.KeepLinks(ctx, ids)
}

// deleteFileStep 连库里的文件删一条整理记录：要是这一部（这一季、这几集）的；移动整理的照旧拒绝，导入的已有文件要 allowExisting。
func (s *Service) deleteFileStep(ctx context.Context, q organize.FileQuery, id uint, allowExisting bool) DeleteStep {
	st := DeleteStep{Kind: DeleteStepFile, ID: id, Target: fmt.Sprintf("整理记录 %d", id)}
	var row models.MediaTransferHistory
	if err := s.cfg.DB.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&row).Error; err != nil {
		st.Message = "读取整理记录失败：" + err.Error()
		return st
	}
	if row.ID == 0 {
		st.Message = "整理记录已经不在了"
		return st
	}
	if row.TargetPath != "" {
		st.Target = row.TargetPath
	}
	if !q.Matches(row) {
		st.Message = "不是这一部的记录，没有删"
		return st
	}
	kept, err := s.cfg.Deleter.DeleteHistoryWith(ctx, id, organize.DeleteOptions{Files: true, AllowExisting: allowExisting})
	if err != nil {
		st.Message = strings.TrimPrefix(err.Error(), organize.ErrInvalid.Error()+": ")
		return st
	}
	st.OK, st.Message = true, "删了库里的文件与整理记录"
	if len(kept) > 0 {
		st.Message += "；" + strings.Join(kept, "；")
	}
	return st
}
