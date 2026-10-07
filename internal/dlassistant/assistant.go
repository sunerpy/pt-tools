// Package dlassistant 是下载器助手：给缺站点标签的种子补标签、批量替换 tracker 地址、找出 tracker 报告
// 未注册或不存在的失效种子。每项都分「预览」与「执行」两步，执行前重新核对，预览之后种子变了就跳过。
//
// tracker 地址里带 passkey，返回给界面的一律脱敏（见 RedactTrackerURL）；替换时用的是下载器里的原地址。
package dlassistant

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
	"github.com/sunerpy/pt-tools/utils"
)

// trackerFetchConcurrency 是同时读取种子 tracker 列表的请求数；下载器多在本机或局域网，4 个足够快也不压垮它。
const trackerFetchConcurrency = 4

// trackerFetchTimeout 是读取单个种子 tracker 列表的时限。
const trackerFetchTimeout = 15 * time.Second

// minTrackerPatternLen 是替换 tracker 时「原内容」的最短长度，太短容易误伤（比如只写一个点）。
const minTrackerPatternLen = 3

// MaxScanTorrents 是一次扫描或预览最多检查的种子数：种子更多时只检查前这么多个，结果里写明。
const MaxScanTorrents = 20000

// ScanInfo 说明一次扫描检查了多少种子：Scanned 小于 Total 时说明种子太多，只检查了一部分。
type ScanInfo struct {
	Total   int `json:"total"`
	Scanned int `json:"scanned"`
}

func limitTorrents(all []downloader.Torrent) ([]downloader.Torrent, ScanInfo) {
	info := ScanInfo{Total: len(all), Scanned: len(all)}
	if len(all) > MaxScanTorrents {
		info.Scanned = MaxScanTorrents
		return all[:MaxScanTorrents], info
	}
	return all, info
}

// ItemError 是执行时没成功的一项。
type ItemError struct {
	Hash  string `json:"hash"`
	Name  string `json:"name,omitempty"`
	Error string `json:"error"`
}

// ApplyResult 是一次执行的结果。
type ApplyResult struct {
	Done    int         `json:"done"`
	Skipped []ItemError `json:"skipped"`
	Failed  []ItemError `json:"failed"`
	// KeptData 是删除时要求删数据、但数据还被别的种子用着（如辅种），所以只删了种子的个数（算在 Done 里）。
	KeptData int `json:"kept_data,omitempty"`
}

func (r *ApplyResult) skip(hash, name, why string) {
	r.Skipped = append(r.Skipped, ItemError{Hash: hash, Name: name, Error: why})
}

func (r *ApplyResult) fail(hash, name string, err error) {
	r.Failed = append(r.Failed, ItemError{Hash: hash, Name: name, Error: err.Error()})
}

func newResult() ApplyResult {
	return ApplyResult{Skipped: []ItemError{}, Failed: []ItemError{}}
}

// ---------- 补站点标签 ----------

// SiteTagSuggestion 是一个按 tracker 认出了站点、但没打这个站点分类或标签的种子。
type SiteTagSuggestion struct {
	Hash        string `json:"hash"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	Site        string `json:"site"`
	SiteName    string `json:"site_name"`
	TrackerHost string `json:"tracker_host"`
	Tags        string `json:"tags"`
	Category    string `json:"category"`
}

// SiteTagItem 是要补的一项：种子与站点。
type SiteTagItem struct {
	Hash string `json:"hash"`
	Site string `json:"site"`
}

// FindMissingSiteTags 列出按 tracker 能认出站点、但分类和标签里都没有这个站点的种子。
func FindMissingSiteTags(ctx context.Context, dl downloader.Downloader, r *v2.TrackerResolver) ([]SiteTagSuggestion, ScanInfo, error) {
	all, err := dl.GetAllTorrents()
	if err != nil {
		return nil, ScanInfo{}, fmt.Errorf("读取下载器种子失败: %w", err)
	}
	torrents, info := limitTorrents(all)
	trackers := fetchTrackers(ctx, dl, torrents, func(t downloader.Torrent) bool { return t.Tracker == "" })
	if err := ctx.Err(); err != nil {
		return nil, info, err
	}
	out := make([]SiteTagSuggestion, 0)
	for _, t := range torrents {
		site, host, ok := resolveSite(r, t, trackers[hashKey(t)])
		if !ok || hasSiteMark(t, site) {
			continue
		}
		s := SiteTagSuggestion{Hash: hashKey(t), Name: t.Name, Size: t.TotalSize, Site: site, SiteName: site, TrackerHost: host, Tags: t.Tags, Category: t.Category}
		if def, ok := r.Definition(site); ok && def.Name != "" {
			s.SiteName = def.Name
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Site != out[j].Site {
			return out[i].Site < out[j].Site
		}
		return out[i].Name < out[j].Name
	})
	return out, info, nil
}

// ApplySiteTags 给种子加上站点标签。执行前重新核对：种子还在、tracker 仍属于这个站点、还没有这个标签。
// 传给下载器的是原有标签加上站点名（qBittorrent 的 addTags 只会新增，Transmission 的 labels 是整份替换）。
func ApplySiteTags(ctx context.Context, dl downloader.Downloader, r *v2.TrackerResolver, items []SiteTagItem) (ApplyResult, error) {
	res := newResult()
	torrents, err := dl.GetAllTorrents()
	if err != nil {
		return res, fmt.Errorf("读取下载器种子失败: %w", err)
	}
	byHash := indexTorrents(torrents)
	seen := map[string]bool{}
	for _, it := range items {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		hash := strings.ToLower(strings.TrimSpace(it.Hash))
		if seen[hash] {
			continue
		}
		seen[hash] = true
		t, ok := byHash[hash]
		if !ok {
			res.skip(hash, "", "种子已不在下载器里")
			continue
		}
		var urls []string
		if t.Tracker == "" {
			urls = trackerURLs(ctx, dl, t)
		}
		site, _, ok := resolveSite(r, t, urls)
		if !ok || !strings.EqualFold(site, it.Site) {
			res.skip(hash, t.Name, "tracker 不属于这个站点")
			continue
		}
		if hasSiteMark(t, site) {
			res.skip(hash, t.Name, "已经有这个站点的标签")
			continue
		}
		if err := dl.SetTorrentTags(t.ID, joinTags(t.Tags, site)); err != nil {
			res.fail(hash, t.Name, err)
			continue
		}
		res.Done++
	}
	return res, nil
}

// ---------- 替换 tracker ----------

// TrackerMatch 是一个要改的 tracker 地址（脱敏后展示）。ID 是这条「原地址 → 新地址」的指纹，
// 执行时只改指纹仍然对得上的地址：预览之后新加的、被改过的地址都不会被动到。
// 替换后不是有效地址的那条 ID 为空、Error 写明原因，不能执行。
type TrackerMatch struct {
	ID    string `json:"id"`
	Hash  string `json:"hash"`
	Name  string `json:"name"`
	Old   string `json:"old"`
	New   string `json:"new"`
	Error string `json:"error,omitempty"`
}

// TrackerSelection 是执行替换时选中的一条：种子与预览时那条地址的指纹。
type TrackerSelection struct {
	Hash string `json:"hash"`
	ID   string `json:"id"`
}

// trackerFingerprint 是「种子、原地址、新地址」的指纹；不含明文，不会把 passkey 带出去。
func trackerFingerprint(hash, oldURL, newURL string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(hash) + "\n" + oldURL + "\n" + newURL))
	return hex.EncodeToString(sum[:12])
}

// ValidateTrackerReplace 校验替换参数：原内容至少 3 个字，替换后要仍是 http(s) 或 udp 地址（在 ReplaceTrackerURL 里逐个核对）。
func ValidateTrackerReplace(from, to string) error {
	if len([]rune(strings.TrimSpace(from))) < minTrackerPatternLen {
		return fmt.Errorf("要替换的内容至少 %d 个字", minTrackerPatternLen)
	}
	if strings.TrimSpace(to) == "" {
		return errors.New("替换成的内容不能为空")
	}
	if strings.TrimSpace(from) == strings.TrimSpace(to) {
		return errors.New("替换前后的内容相同")
	}
	return nil
}

// ReplaceTrackerURL 把地址里第一处 from 换成 to；结果不是 http(s) 或 udp 地址时返回错误。
func ReplaceTrackerURL(raw, from, to string) (string, error) {
	out := strings.Replace(raw, from, to, 1)
	u, err := url.Parse(out)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "udp") {
		return "", fmt.Errorf("替换后不是有效的 tracker 地址: %s", RedactTrackerURL(out))
	}
	return out, nil
}

// PreviewTrackerReplace 列出 tracker 地址里含 from 的种子和替换后的地址（读每个种子完整的 tracker 列表）。
func PreviewTrackerReplace(ctx context.Context, dl downloader.Downloader, from, to string) ([]TrackerMatch, ScanInfo, error) {
	if err := ValidateTrackerReplace(from, to); err != nil {
		return nil, ScanInfo{}, err
	}
	all, err := dl.GetAllTorrents()
	if err != nil {
		return nil, ScanInfo{}, fmt.Errorf("读取下载器种子失败: %w", err)
	}
	torrents, info := limitTorrents(all)
	stats := fetchTrackerStats(ctx, dl, torrents)
	if err := ctx.Err(); err != nil {
		return nil, info, err
	}
	out := make([]TrackerMatch, 0)
	for _, t := range torrents {
		for _, u := range uniqueTrackerURLs(stats[hashKey(t)]) {
			if !strings.Contains(u, from) {
				continue
			}
			m := TrackerMatch{Hash: hashKey(t), Name: t.Name, Old: RedactTrackerURL(u)}
			if nu, err := ReplaceTrackerURL(u, from, to); err != nil {
				m.Error = err.Error() // 照样列出，但没有指纹，执行时不会改
			} else {
				m.ID, m.New = trackerFingerprint(hashKey(t), u, nu), RedactTrackerURL(nu)
			}
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, info, nil
}

// ApplyTrackerReplace 只改预览时选中的那几条地址：逐个种子重新读 tracker 列表，地址与替换结果的指纹
// 仍和预览时一致才改；对不上的（预览之后地址变了）跳过。同一种子改到一半失败时写明已经改了几条。
func ApplyTrackerReplace(ctx context.Context, dl downloader.Downloader, from, to string, selected []TrackerSelection) (ApplyResult, error) {
	res := newResult()
	if err := ValidateTrackerReplace(from, to); err != nil {
		return res, err
	}
	editor, ok := dl.(downloader.TrackerEditor)
	if !ok {
		return res, downloader.ErrCapabilityUnsupported
	}
	torrents, err := dl.GetAllTorrents()
	if err != nil {
		return res, fmt.Errorf("读取下载器种子失败: %w", err)
	}
	byHash := indexTorrents(torrents)
	want := map[string]map[string]bool{}
	order := make([]string, 0)
	for _, sel := range selected {
		hash := strings.ToLower(strings.TrimSpace(sel.Hash))
		if hash == "" || sel.ID == "" {
			continue
		}
		if want[hash] == nil {
			want[hash] = map[string]bool{}
			order = append(order, hash)
		}
		want[hash][sel.ID] = true
	}
	for _, hash := range order {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		t, ok := byHash[hash]
		if !ok {
			res.skip(hash, "", "种子已不在下载器里")
			continue
		}
		fctx, cancel := context.WithTimeout(ctx, trackerFetchTimeout)
		trs, err := getTrackers(fctx, dl, t.ID)
		cancel()
		if err != nil {
			res.fail(hash, t.Name, fmt.Errorf("读取 tracker 失败: %w", err))
			continue
		}
		changed, matched := 0, 0
		var itemErr error
		for _, u := range uniqueTrackerURLs(trs) {
			if !strings.Contains(u, from) {
				continue
			}
			nu, err := ReplaceTrackerURL(u, from, to)
			if err != nil || !want[hash][trackerFingerprint(hash, u, nu)] {
				continue
			}
			matched++
			if err := editor.EditTracker(ctx, t.ID, u, nu); err != nil {
				itemErr = err
				break
			}
			changed++
		}
		switch {
		case itemErr != nil && changed > 0:
			res.fail(hash, t.Name, fmt.Errorf("已改 %d 个地址，之后失败: %w", changed, itemErr))
		case itemErr != nil:
			res.fail(hash, t.Name, itemErr)
		case matched == 0:
			res.skip(hash, t.Name, "预览之后 tracker 地址变了，没有修改")
		default:
			res.Done++
			if matched < len(want[hash]) {
				res.skip(hash, t.Name, fmt.Sprintf("有 %d 个选中的地址预览之后变了，没有修改", len(want[hash])-matched))
			}
		}
	}
	return res, nil
}

// ---------- 失效种子 ----------

// DeadReason 是失效的原因。
type DeadReason string

const (
	// DeadUnregistered 是 tracker 说这个种子没有注册（站点删了种或换了种）。
	DeadUnregistered DeadReason = "unregistered"
	// DeadNotFound 是 tracker 说种子不存在或已被删除。
	DeadNotFound DeadReason = "not_found"
)

// DeadTorrent 是一个失效种子。
type DeadTorrent struct {
	Hash        string     `json:"hash"`
	Name        string     `json:"name"`
	Size        int64      `json:"size"`
	Progress    float64    `json:"progress"`
	Site        string     `json:"site,omitempty"`
	SiteName    string     `json:"site_name,omitempty"`
	TrackerHost string     `json:"tracker_host"`
	Reason      DeadReason `json:"reason"`
	Message     string     `json:"message"`
}

var (
	unregisteredMarks = []string{"unregistered torrent", "torrent not registered", "torrent is not registered", "not registered with this tracker", "未注册"}
	notFoundMarks     = []string{
		"torrent not found", "torrent does not exist", "torrent not exist", "infohash not found", "info_hash not found",
		"torrent has been deleted", "torrent deleted", "种子不存在", "该种子不存在", "种子已被删除", "种子已删除", "种子被删除",
	}
	// accountMarks 出现时是账号或 passkey 的问题，不是种子失效，不能删种
	accountMarks = []string{"passkey", "user", "用户", "账号", "帐号", "banned", "封禁", "流量", "ratio", "client", "客户端"}
)

// classifyTrackerMessage 判断一条 tracker 消息是不是在说种子失效。
func classifyTrackerMessage(msg string) (DeadReason, bool) {
	// 消息里带的地址（常含 passkey=…）不参与判断，免得把地址里的字当成账号问题
	m := strings.ToLower(strings.TrimSpace(messageURL.ReplaceAllString(msg, " ")))
	if m == "" {
		return "", false
	}
	for _, a := range accountMarks {
		if strings.Contains(m, a) {
			return "", false
		}
	}
	for _, k := range notFoundMarks {
		if strings.Contains(m, k) {
			return DeadNotFound, true
		}
	}
	for _, k := range unregisteredMarks {
		if strings.Contains(m, k) {
			return DeadUnregistered, true
		}
	}
	return "", false
}

// classifyDead 判断一个种子是否失效：至少一个真实 tracker 报告失效，并且没有任何真实 tracker 在正常工作。
func classifyDead(trackers []downloader.TorrentTracker) (DeadReason, string, bool) {
	var reason DeadReason
	var message string
	real := 0
	for _, tr := range trackers {
		if !isRealTracker(tr.URL) {
			continue
		}
		real++
		if tr.Status == 2 || tr.Status == 3 {
			return "", "", false
		}
		if r, ok := classifyTrackerMessage(tr.Message); ok && reason == "" {
			reason, message = r, tr.Message
		}
	}
	return reason, message, real > 0 && reason != ""
}

// ScanDeadTorrents 逐个读取种子的 tracker 状态，列出失效种子。
func ScanDeadTorrents(ctx context.Context, dl downloader.Downloader, r *v2.TrackerResolver) ([]DeadTorrent, ScanInfo, error) {
	all, err := dl.GetAllTorrents()
	if err != nil {
		return nil, ScanInfo{}, fmt.Errorf("读取下载器种子失败: %w", err)
	}
	torrents, info := limitTorrents(all)
	stats := fetchTrackerStats(ctx, dl, torrents)
	if err := ctx.Err(); err != nil {
		return nil, info, err
	}
	out := make([]DeadTorrent, 0)
	for _, t := range torrents {
		trs := stats[hashKey(t)]
		reason, msg, dead := classifyDead(trs)
		if !dead {
			continue
		}
		d := DeadTorrent{Hash: hashKey(t), Name: t.Name, Size: t.TotalSize, Progress: t.Progress, Reason: reason, Message: RedactTrackerMessage(msg)}
		urls := make([]string, 0, len(trs))
		for _, tr := range trs {
			urls = append(urls, tr.URL)
		}
		if site, host, ok := resolveSite(r, t, urls); ok {
			d.Site, d.SiteName, d.TrackerHost = site, site, host
			if def, ok := r.Definition(site); ok && def.Name != "" {
				d.SiteName = def.Name
			}
		} else {
			d.TrackerHost = firstHost(urls)
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, info, nil
}

// DeleteDeadTorrents 删除选中的失效种子；删除前逐个重新读取 tracker 状态，已经恢复正常的不删。
// 要求删数据时，数据还被别的种子用着的（如在别的站点辅种的同一份文件）只删种子、保留数据。
func DeleteDeadTorrents(ctx context.Context, dl downloader.Downloader, hashes []string, removeData bool) (ApplyResult, error) {
	res := newResult()
	torrents, err := dl.GetAllTorrents()
	if err != nil {
		return res, fmt.Errorf("读取下载器种子失败: %w", err)
	}
	byHash := indexTorrents(torrents)
	removed := map[string]bool{} // 已经删掉的不算“别的种子”，共用数据的最后一个连数据删
	seen := map[string]bool{}
	for _, h := range hashes {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		hash := strings.ToLower(strings.TrimSpace(h))
		if seen[hash] {
			continue
		}
		seen[hash] = true
		t, ok := byHash[hash]
		if !ok {
			res.skip(hash, "", "种子已不在下载器里")
			continue
		}
		fctx, cancel := context.WithTimeout(ctx, trackerFetchTimeout)
		trs, err := getTrackers(fctx, dl, t.ID)
		cancel()
		if err != nil {
			res.fail(hash, t.Name, fmt.Errorf("读取 tracker 状态失败: %w", err))
			continue
		}
		if _, _, dead := classifyDead(trs); !dead {
			res.skip(hash, t.Name, "tracker 已经恢复正常，没有删除")
			continue
		}
		withData := removeData && !downloader.SharesData(torrents, t, removed)
		if err := dl.RemoveTorrent(t.ID, withData); err != nil {
			res.fail(hash, t.Name, err)
			continue
		}
		removed[downloader.TorrentKey(t)] = true
		if removeData && !withData {
			res.KeptData++
		}
		res.Done++
	}
	return res, nil
}

// ---------- 共用 ----------

// RedactTrackerURL 把 tracker 地址里的 passkey 之类遮掉：敏感查询参数与路径里像密钥的长段。
func RedactTrackerURL(raw string) string {
	return strings.ReplaceAll(redactTrackerURL(raw), "%2A%2A%2A", "***")
}

func redactTrackerURL(raw string) string {
	s := utils.SanitizeURL(raw)
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	segs := strings.Split(u.Path, "/")
	changed := false
	for i, seg := range segs {
		if secretSegment.MatchString(seg) {
			segs[i] = "***"
			changed = true
		}
	}
	if changed {
		u.Path = strings.Join(segs, "/")
		u.RawPath = ""
		return u.String()
	}
	return s
}

// secretSegment 是 Unit3D 一类把 passkey 放在路径里的长串（16 位以上的字母数字）。
var secretSegment = regexp.MustCompile(`^[A-Za-z0-9]{16,}$`)

var (
	messageURL    = regexp.MustCompile(`(?i)\b(?:https?|udp)://[^\s"'<>]+`)
	messageSecret = regexp.MustCompile(`(?i)\b(passkey|authkey|torrent_pass|credential|token|apikey|api_key|sign|secret|rsskey|key)=([^&\s"'<>(),;]+)`)
	messageLong   = regexp.MustCompile(`\b[A-Za-z0-9]{24,}\b`)
)

// RedactTrackerMessage 遮住 tracker 回复里可能带的凭证：里面的地址按 RedactTrackerURL 处理，
// 再遮 passkey=… 一类参数和 24 位以上的长串。
func RedactTrackerMessage(msg string) string {
	msg = messageURL.ReplaceAllStringFunc(msg, RedactTrackerURL)
	msg = messageSecret.ReplaceAllString(msg, "$1=***")
	return messageLong.ReplaceAllString(msg, "***")
}

func isRealTracker(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	switch u.Scheme {
	case "http", "https", "udp":
		return true
	}
	return false
}

func hashKey(t downloader.Torrent) string {
	if t.InfoHash != "" {
		return strings.ToLower(t.InfoHash)
	}
	return strings.ToLower(t.ID)
}

func indexTorrents(torrents []downloader.Torrent) map[string]downloader.Torrent {
	m := make(map[string]downloader.Torrent, len(torrents))
	for _, t := range torrents {
		m[hashKey(t)] = t
	}
	return m
}

func hasSiteMark(t downloader.Torrent, site string) bool {
	if strings.EqualFold(strings.TrimSpace(t.Category), site) {
		return true
	}
	for _, tag := range strings.Split(t.Tags, ",") {
		if strings.EqualFold(strings.TrimSpace(tag), site) {
			return true
		}
	}
	return false
}

func joinTags(existing, add string) string {
	tags := make([]string, 0)
	for _, tag := range strings.Split(existing, ",") {
		if tag = strings.TrimSpace(tag); tag != "" {
			tags = append(tags, tag)
		}
	}
	return strings.Join(append(tags, add), ",")
}

// resolveSite 先看主 tracker，再看 tracker 列表，返回站点 ID 与 tracker 主机名。
func resolveSite(r *v2.TrackerResolver, t downloader.Torrent, urls []string) (string, string, bool) {
	candidates := append([]string{t.Tracker}, urls...)
	for _, u := range candidates {
		if !isRealTracker(u) {
			continue
		}
		if site, ok := r.Resolve(u); ok {
			return site, hostOnly(u), true
		}
	}
	return "", "", false
}

func hostOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func firstHost(urls []string) string {
	for _, u := range urls {
		if isRealTracker(u) {
			return hostOnly(u)
		}
	}
	return ""
}

func getTrackers(ctx context.Context, dl downloader.Downloader, id string) ([]downloader.TorrentTracker, error) {
	if r, ok := dl.(downloader.TrackerReader); ok {
		return r.GetTorrentTrackersContext(ctx, id)
	}
	// 不支持 ctx 的下载器：ctx 到期时先返回，底层请求自己跑完
	type result struct {
		trs []downloader.TorrentTracker
		err error
	}
	ch := make(chan result, 1)
	go func() {
		trs, err := dl.GetTorrentTrackers(id)
		ch <- result{trs, err}
	}()
	select {
	case r := <-ch:
		return r.trs, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// trackerURLs 返回种子的 tracker 地址（主 tracker 已知时也读列表，替换时要改的是列表里的原地址）。
func trackerURLs(ctx context.Context, dl downloader.Downloader, t downloader.Torrent) []string {
	fctx, cancel := context.WithTimeout(ctx, trackerFetchTimeout)
	defer cancel()
	trs, err := getTrackers(fctx, dl, t.ID)
	if err != nil {
		if t.Tracker != "" {
			return []string{t.Tracker}
		}
		return nil
	}
	urls := make([]string, 0, len(trs))
	for _, tr := range trs {
		if isRealTracker(tr.URL) {
			urls = append(urls, tr.URL)
		}
	}
	return urls
}

// fetchTrackers 对 need 为真的种子读取 tracker 地址（能一次读全部的下载器一次读）；其他种子用主 tracker。
func fetchTrackers(ctx context.Context, dl downloader.Downloader, torrents []downloader.Torrent, need func(downloader.Torrent) bool) map[string][]string {
	if _, ok := dl.(downloader.BulkTrackerReader); ok {
		needed := make([]downloader.Torrent, 0)
		out := make(map[string][]string, len(torrents))
		for _, t := range torrents {
			if need(t) {
				needed = append(needed, t)
			} else if t.Tracker != "" {
				out[hashKey(t)] = []string{t.Tracker}
			}
		}
		if len(needed) > 0 {
			for h, trs := range fetchTrackerStats(ctx, dl, needed) {
				out[h] = uniqueTrackerURLs(trs)
			}
		}
		return out
	}
	out := make(map[string][]string, len(torrents))
	var mu sync.Mutex
	forEachLimited(ctx, torrents, func(t downloader.Torrent) {
		var urls []string
		if need(t) {
			urls = trackerURLs(ctx, dl, t)
		} else if t.Tracker != "" {
			urls = []string{t.Tracker}
		}
		mu.Lock()
		out[hashKey(t)] = urls
		mu.Unlock()
	})
	return out
}

// uniqueTrackerURLs 返回 tracker 列表里去重后的真实地址（不含 DHT、PeX、LSD）。
func uniqueTrackerURLs(trs []downloader.TorrentTracker) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(trs))
	for _, tr := range trs {
		u := strings.TrimSpace(tr.URL)
		if !isRealTracker(u) || seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
	}
	return out
}

// fetchTrackerStats 读取种子的 tracker 状态：下载器能一次读全部的（Transmission）就一次读，否则逐个并发读；
// 读不到的种子没有条目（不会被当成失效）。
func fetchTrackerStats(ctx context.Context, dl downloader.Downloader, torrents []downloader.Torrent) map[string][]downloader.TorrentTracker {
	if bulk, ok := dl.(downloader.BulkTrackerReader); ok {
		all, err := bulk.GetAllTorrentTrackers(ctx)
		if err == nil {
			out := make(map[string][]downloader.TorrentTracker, len(torrents))
			for _, t := range torrents {
				if trs, ok := all[hashKey(t)]; ok {
					out[hashKey(t)] = trs
				}
			}
			return out
		}
	}
	out := make(map[string][]downloader.TorrentTracker, len(torrents))
	var mu sync.Mutex
	forEachLimited(ctx, torrents, func(t downloader.Torrent) {
		fctx, cancel := context.WithTimeout(ctx, trackerFetchTimeout)
		defer cancel()
		trs, err := getTrackers(fctx, dl, t.ID)
		if err != nil {
			return
		}
		mu.Lock()
		out[hashKey(t)] = trs
		mu.Unlock()
	})
	return out
}

func forEachLimited(ctx context.Context, torrents []downloader.Torrent, fn func(downloader.Torrent)) {
	sem := make(chan struct{}, trackerFetchConcurrency)
	var wg sync.WaitGroup
	for _, t := range torrents {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(t downloader.Torrent) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(t)
		}(t)
	}
	wg.Wait()
}
