package qbitcompat

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sunerpy/pt-tools/internal"
	"github.com/sunerpy/pt-tools/internal/app"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

const (
	maxTorrentFile = 10 << 20
	maxAddItems    = 50
	addTimeout     = 2 * time.Minute
)

// 添加失败的原因（记进审计，也决定回 Fails. 还是 415）。
var (
	errInvalidTorrent = errors.New("种子文件不对")
	errMagnet         = errors.New("不接受磁力链接")
	errUnknownURL     = errors.New("认不出是哪个已启用站点的种子链接")
	errSiteDisabled   = errors.New("站点没有启用")
)

// addOptions 是 torrents/add 的选项（字段名按 qB）。
type addOptions struct {
	savePath, category, rename string
	tags                       []string
	paused                     bool
	upKBs, dlKBs               int
}

func parseAddOptions(f url.Values) addOptions {
	o := addOptions{
		savePath: strings.TrimSpace(f.Get("savepath")), category: strings.TrimSpace(f.Get("category")),
		rename: strings.TrimSpace(f.Get("rename")), tags: splitTags(f.Get("tags")),
		paused: formBool(f.Get("paused")) || formBool(f.Get("stopped")),
	}
	// upLimit、dlLimit 是字节每秒；推送用 KB/s，不足 1 KB/s 的按 1 KB/s
	toKBs := func(v string) int {
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil || n <= 0 {
			return 0
		}
		return int(max((n+1023)/1024, 1))
	}
	o.upKBs, o.dlKBs = toKBs(f.Get("upLimit")), toKBs(f.Get("dlLimit"))
	return o
}

// addOutcome 是一个种子的结果。
type addOutcome struct {
	ok, skipped bool
	// hash 是加进去的种子的 info hash（小写）
	hash string
	err  error
}

// add 是 POST /api/v2/torrents/add：种子文件（multipart 里上传的文件，可以有多个）与链接（urls，一行一个）。
// 种子文件直接用；链接只认能解析成已启用站点种子编号的，由 pt-tools 经站点下载（不请求客户端给的地址），磁力链接一律拒绝。
// 都经 internal.PushTorrentToDownloader 推到绑定的下载器（磁盘空间保护、站点做种容量照常）。
// 有一个加进去或者已经在下载器里就回 Ok.，全失败回 Fails.；只有不对的种子文件时回 415（qB 的行为）。
func (s *Server) add(w http.ResponseWriter, r *http.Request, c *call) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(maxAddBody); err != nil {
			text(w, http.StatusBadRequest, "Bad Request")
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()
	} else if err := r.ParseForm(); err != nil {
		text(w, http.StatusBadRequest, "Bad Request")
		return
	}
	opts := parseAddOptions(r.Form)
	var files [][]byte
	var fileErrs []error
	if r.MultipartForm != nil {
		// qB 收所有上传的文件，不看字段名：qbittorrent-api（MoviePilot 用的）拿文件名当字段名，不是 torrents
		for _, field := range slices.Sorted(maps.Keys(r.MultipartForm.File)) {
			for _, fh := range r.MultipartForm.File[field] {
				data, err := readPart(fh)
				if err != nil {
					fileErrs = append(fileErrs, err)
					continue
				}
				files = append(files, data)
			}
		}
	}
	var links []string
	for _, l := range strings.Split(r.Form.Get("urls"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			links = append(links, l)
		}
	}
	total := len(files) + len(fileErrs) + len(links)
	if total == 0 || total > maxAddItems {
		reason := "error:nothing_added"
		if total > maxAddItems {
			reason = "error:too_many"
		}
		s.recordWrite(r, c, "torrents/add", reason, selection{}, map[string]any{"files": len(files) + len(fileErrs), "urls": len(links)})
		text(w, http.StatusOK, "Fails.")
		return
	}
	b, ok := s.withBackend(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), addTimeout)
	defer cancel()
	defer s.invalidate(b.setting.ID)
	outcomes := make([]addOutcome, 0, total)
	for _, err := range fileErrs {
		outcomes = append(outcomes, addOutcome{err: err})
	}
	// 每一个之前看一下上下文：超时或者 pt-tools 在关闭时，剩下的不再处理
	stopped := func() (addOutcome, bool) {
		if err := ctx.Err(); err != nil {
			return addOutcome{err: fmt.Errorf("没有处理（请求已结束）: %w", err)}, true
		}
		return addOutcome{}, false
	}
	for _, data := range files {
		if o, stop := stopped(); stop {
			outcomes = append(outcomes, o)
			continue
		}
		outcomes = append(outcomes, s.pushData(ctx, b, data, "", "", opts))
	}
	for _, l := range links {
		if o, stop := stopped(); stop {
			outcomes = append(outcomes, o)
			continue
		}
		outcomes = append(outcomes, s.pushLink(ctx, b, l, opts))
	}
	// 确实是这次加进去的才记所有权（原来就在下载器里的不归兼容入口）：全部推完以后整批一起等下载器列出来
	var fresh []string
	for _, o := range outcomes {
		if o.ok && !o.skipped {
			fresh = append(fresh, o.hash)
		}
	}
	if len(fresh) > 0 {
		s.ownAdded(ctx, b, fresh)
	}
	s.recordAdd(r, c, outcomes, len(files)+len(fileErrs), len(links), opts)

	invalid := 0
	for _, o := range outcomes {
		if o.ok {
			text(w, http.StatusOK, "Ok.")
			return
		}
		if errors.Is(o.err, errInvalidTorrent) {
			invalid++
		}
	}
	if invalid == len(outcomes) {
		text(w, http.StatusUnsupportedMediaType, "Torrent file is not valid")
		return
	}
	text(w, http.StatusOK, "Fails.")
}

// readPart 读一个上传的种子文件，最多 10 MiB。
func readPart(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errInvalidTorrent, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxTorrentFile+1))
	if err != nil || len(data) > maxTorrentFile {
		return nil, fmt.Errorf("%w: 读不了或超过 10 MiB", errInvalidTorrent)
	}
	return data, nil
}

// pushLink 处理一条链接：磁力链接拒绝；能解析成已启用站点的种子编号时经站点下载，再推送。
func (s *Server) pushLink(ctx context.Context, b *backend, link string, o addOptions) addOutcome {
	if strings.HasPrefix(strings.ToLower(link), "magnet:") {
		return addOutcome{err: errMagnet}
	}
	siteID, torrentID, ok := s.deps.Resolver.ResolveDownloadURL(link)
	if !ok {
		return addOutcome{err: errUnknownURL}
	}
	var site v2.Site
	if s.deps.Site != nil {
		site = s.deps.Site(siteID)
	}
	if site == nil {
		return addOutcome{err: fmt.Errorf("%w: %s", errSiteDisabled, siteID)}
	}
	data, err := site.Download(ctx, torrentID)
	if err != nil {
		return addOutcome{err: fmt.Errorf("经站点 %s 下载种子 %s 失败: %w", siteID, torrentID, err)}
	}
	return s.pushData(ctx, b, data, siteID, torrentID, o)
}

// pushData 推送一个种子文件。站点没给时按 tracker 认；编号没给时（上传的文件）用 hash:<info hash>。
// 认出了站点时标签里追加站点 ID（站点标签），没给保存目录时用 createCategory 记下的分类目录。
func (s *Server) pushData(ctx context.Context, b *backend, data []byte, siteID, torrentID string, o addOptions) addOutcome {
	parsed, err := v2.ParseTorrent(data)
	if err != nil {
		return addOutcome{err: fmt.Errorf("%w: %v", errInvalidTorrent, err)}
	}
	hash := strings.ToLower(parsed.InfoHash)
	if siteID == "" {
		siteID = s.siteOf(parsed)
	}
	// 上传的种子文件内容由客户端给（comment 里的详情页地址也能随便写），编号一律用 hash:<info hash>，
	// 不按 comment 里的编号去 upsert，免得盖掉站点上真有的那条记录
	if torrentID == "" {
		torrentID = "hash:" + hash
	}
	tags := visibleTags(o.tags)
	if siteID != "" && !slices.Contains(tags, siteID) {
		tags = append(tags, siteID)
	}
	// 所有权的标签（见 OwnerTag）：原来就在下载器里的种子不会因为这次添加带上它
	tags = append(tags, OwnerTag)
	save := o.savePath
	if save == "" && o.category != "" {
		save = categoryMap(b.cfg)[o.category]
	}
	title := o.rename
	if title == "" {
		title = parsed.Name
	}
	res, err := s.deps.Push(ctx, internal.PushTorrentRequest{
		SiteID: siteID, TorrentID: torrentID, TorrentData: data, Title: title, Category: o.category, Tags: strings.Join(tags, ","),
		SavePath: save, DownloaderID: b.setting.ID, Source: Source, AddPaused: o.paused, Rename: o.rename,
		UpLimitKBs: o.upKBs, DlLimitKBs: o.dlKBs,
	})
	switch {
	case err != nil:
		return addOutcome{err: err}
	case res == nil || !res.Success:
		msg := "下载器没有接收"
		if res != nil && res.Message != "" {
			msg = res.Message
		}
		return addOutcome{err: errors.New(msg)}
	}
	return addOutcome{ok: true, skipped: res.Skipped, hash: hash}
}

// siteOf 按种子里的 tracker 认站点；认不出时是空的。
func (s *Server) siteOf(p *v2.ParsedTorrent) string {
	urls := []string{p.Announce}
	for _, tier := range p.AnnounceList {
		urls = append(urls, tier...)
	}
	for _, u := range urls {
		if id, ok := s.deps.Resolver.Resolve(u); ok {
			return id
		}
	}
	return ""
}

// recordAdd 记一条添加的审计：成功 success；有失败的 error:partial；全失败时按原因（denied:magnet、denied:url 或 error:add_failed）。
func (s *Server) recordAdd(r *http.Request, c *call, outcomes []addOutcome, files, links int, o addOptions) {
	c.audited = true
	var added, skipped int
	var reasons []string
	policy := 0
	for _, x := range outcomes {
		switch {
		case x.ok && x.skipped:
			skipped++
		case x.ok:
			added++
		default:
			reasons = append(reasons, redact(x.err.Error()))
			if errors.Is(x.err, errMagnet) || errors.Is(x.err, errUnknownURL) || errors.Is(x.err, errSiteDisabled) {
				policy++
			}
		}
	}
	failed := len(reasons)
	result := "success"
	switch {
	case failed > 0 && added+skipped > 0:
		result = "error:partial"
	case failed > 0 && policy == failed && errorsAll(outcomes, errMagnet):
		result = "denied:magnet"
	case failed > 0 && policy == failed:
		result = "denied:url"
	case failed > 0:
		result = "error:add_failed"
	}
	if len(reasons) > 10 {
		reasons = reasons[:10]
	}
	s.record(r, app.AuditEntry{
		ChannelUserID: strconv.FormatUint(uint64(c.token.ID), 10), Command: "POST /api/v2/torrents/add", Result: result,
		LatencyMs: time.Since(c.start).Milliseconds(),
		Args: map[string]any{
			"name": c.token.Name, "username": c.username, "files": files, "urls": links, "added": added, "skipped": skipped,
			"failed": failed, "reasons": reasons, "category": o.category,
		},
	})
}

func errorsAll(outcomes []addOutcome, target error) bool {
	for _, o := range outcomes {
		if !o.ok && !errors.Is(o.err, target) {
			return false
		}
	}
	return true
}
