package qbitcompat

import (
	"cmp"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/internal/dlassistant"
	"github.com/sunerpy/pt-tools/thirdpart/downloader"
)

// qB 用 8640000 秒（100 天）表示 ETA 无穷大。
const etaInfinite = 8640000

// qbTorrent 是 torrents/info 的一项，字段按 qB WebUI API v2。后端是 qB 时，接口里没有的字段取 qB 原样的值。
type qbTorrent struct {
	AddedOn           int64   `json:"added_on"`
	AmountLeft        int64   `json:"amount_left"`
	AutoTMM           bool    `json:"auto_tmm"`
	Availability      float64 `json:"availability"`
	Category          string  `json:"category"`
	Completed         int64   `json:"completed"`
	CompletionOn      int64   `json:"completion_on"`
	ContentPath       string  `json:"content_path"`
	DlLimit           int64   `json:"dl_limit"`
	Dlspeed           int64   `json:"dlspeed"`
	Downloaded        int64   `json:"downloaded"`
	DownloadedSession int64   `json:"downloaded_session"`
	ETA               int64   `json:"eta"`
	FLPiecePrio       bool    `json:"f_l_piece_prio"`
	ForceStart        bool    `json:"force_start"`
	Hash              string  `json:"hash"`
	IsPrivate         bool    `json:"isPrivate"`
	LastActivity      int64   `json:"last_activity"`
	MagnetURI         string  `json:"magnet_uri"`
	MaxRatio          float64 `json:"max_ratio"`
	MaxSeedingTime    int64   `json:"max_seeding_time"`
	Name              string  `json:"name"`
	NumComplete       int64   `json:"num_complete"`
	NumIncomplete     int64   `json:"num_incomplete"`
	NumLeechs         int64   `json:"num_leechs"`
	NumSeeds          int64   `json:"num_seeds"`
	Priority          int64   `json:"priority"`
	Progress          float64 `json:"progress"`
	Ratio             float64 `json:"ratio"`
	RatioLimit        float64 `json:"ratio_limit"`
	SavePath          string  `json:"save_path"`
	SeedingTime       int64   `json:"seeding_time"`
	SeedingTimeLimit  int64   `json:"seeding_time_limit"`
	SeenComplete      int64   `json:"seen_complete"`
	SeqDl             bool    `json:"seq_dl"`
	Size              int64   `json:"size"`
	State             string  `json:"state"`
	SuperSeeding      bool    `json:"super_seeding"`
	Tags              string  `json:"tags"`
	TimeActive        int64   `json:"time_active"`
	TotalSize         int64   `json:"total_size"`
	Tracker           string  `json:"tracker"`
	UpLimit           int64   `json:"up_limit"`
	Uploaded          int64   `json:"uploaded"`
	UploadedSession   int64   `json:"uploaded_session"`
	Upspeed           int64   `json:"upspeed"`
}

// rawMap 是后端 qB 原样的字段（别的下载器没有）。
func rawMap(t downloader.Torrent) map[string]any {
	m, _ := t.Raw.(map[string]any)
	return m
}

func rawInt(m map[string]any, key string, def int64) int64 {
	if v, ok := m[key].(float64); ok {
		return int64(v)
	}
	return def
}

func rawFloat(m map[string]any, key string, def float64) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return def
}

func rawBool(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}

// toQB 把下载器的种子换成 qB 的字段。tracker 地址里的 passkey 遮住，magnet 只留 xt 与 dn。
// labels 为真时后端是 Transmission：分类是第一个 label，标签是其余的。
func toQB(t downloader.Torrent, labels bool) qbTorrent {
	raw := rawMap(t)
	hash := strings.ToLower(t.InfoHash)
	if hash == "" {
		hash = strings.ToLower(t.ID)
	}
	completed := t.TotalSize - t.AmountLeft
	if t.AmountLeft <= 0 {
		completed = int64(float64(t.TotalSize) * t.Progress)
	}
	eta := t.ETA
	if eta < 0 {
		eta = etaInfinite
	}
	q := qbTorrent{
		AddedOn: t.DateAdded, AmountLeft: max(t.AmountLeft, 0), Availability: t.Availability, Category: t.Category,
		Completed: completed, CompletionOn: t.CompletionOn, ContentPath: t.ContentPath,
		DlLimit: rawInt(raw, "dl_limit", -1), Dlspeed: t.DownloadSpeed, Downloaded: t.TotalDownloaded,
		DownloadedSession: rawInt(raw, "downloaded_session", 0), ETA: eta,
		FLPiecePrio: rawBool(raw, "f_l_piece_prio"), ForceStart: rawBool(raw, "force_start"), Hash: hash,
		IsPrivate: true, LastActivity: rawInt(raw, "last_activity", 0), MagnetURI: magnetOf(hash, t.Name),
		MaxRatio: rawFloat(raw, "max_ratio", -1), MaxSeedingTime: rawInt(raw, "max_seeding_time", -1), Name: t.Name,
		NumComplete: rawInt(raw, "num_complete", -1), NumIncomplete: rawInt(raw, "num_incomplete", -1),
		NumLeechs: int64(t.NumPeers), NumSeeds: int64(t.NumSeeds), Priority: rawInt(raw, "priority", 0),
		Progress: t.Progress, Ratio: t.Ratio, RatioLimit: rawFloat(raw, "ratio_limit", -2), SavePath: t.SavePath,
		SeedingTime: t.SeedingTime, SeedingTimeLimit: rawInt(raw, "seeding_time_limit", -2),
		SeenComplete: rawInt(raw, "seen_complete", t.CompletionOn), SeqDl: rawBool(raw, "seq_dl"),
		Size: rawInt(raw, "size", t.TotalSize), State: qbState(t), SuperSeeding: rawBool(raw, "super_seeding"),
		Tags: t.Tags, TimeActive: rawInt(raw, "time_active", 0), TotalSize: rawInt(raw, "total_size", t.TotalSize),
		Tracker: redactTracker(t.Tracker), UpLimit: rawInt(raw, "up_limit", -1), Uploaded: t.TotalUploaded,
		UploadedSession: rawInt(raw, "uploaded_session", 0), Upspeed: t.UploadSpeed,
	}
	if v, ok := raw["auto_tmm"].(bool); ok {
		q.AutoTMM = v
	}
	if v, ok := raw["isPrivate"].(bool); ok {
		q.IsPrivate = v
	}
	if labels {
		_, tags := splitLabels(t)
		q.Tags = strings.Join(tags, ",")
	}
	return q
}

// splitLabels 把 Transmission 的 labels 拆成分类（第一个）与标签（其余）。
func splitLabels(t downloader.Torrent) (string, []string) {
	labels := splitTags(t.Tags)
	if len(labels) > 0 && labels[0] == t.Category {
		return t.Category, labels[1:]
	}
	return t.Category, labels
}

// joinLabels 拼回 Transmission 的 labels：分类在第一个。
func joinLabels(category string, tags []string) string {
	out := make([]string, 0, len(tags)+1)
	if category != "" {
		out = append(out, category)
	}
	for _, t := range tags {
		if t != category && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return strings.Join(out, ",")
}

// usesLabels 报告后端是不是 Transmission（分类与标签都在 labels 里）。
func usesLabels(b *backend) bool { return b.setting.Type == string(downloader.DownloaderTransmission) }

// qbState 是 qB 的状态值。后端是 qB 时用它原样的状态（5.x 的 stopped* 换回 4.x 的 paused*，和报的版本一致）。
func qbState(t downloader.Torrent) string {
	if s, ok := rawMap(t)["state"].(string); ok && s != "" {
		switch s {
		case "stoppedDL":
			return "pausedDL"
		case "stoppedUP":
			return "pausedUP"
		}
		return s
	}
	done := t.IsCompleted || t.Progress >= 1
	pick := func(dl, up string) string {
		if done {
			return up
		}
		return dl
	}
	switch t.State {
	case downloader.TorrentDownloading:
		if t.DownloadSpeed > 0 {
			return "downloading"
		}
		return "stalledDL"
	case downloader.TorrentSeeding:
		if t.UploadSpeed > 0 {
			return "uploading"
		}
		return "stalledUP"
	case downloader.TorrentPaused, downloader.TorrentStopped:
		return pick("pausedDL", "pausedUP")
	case downloader.TorrentQueued:
		return pick("queuedDL", "queuedUP")
	case downloader.TorrentChecking:
		return pick("checkingDL", "checkingUP")
	case downloader.TorrentError:
		return "error"
	}
	return "unknown"
}

// matchFilter 是 torrents/info 的 filter（按 qB 的分组）。不认识的 filter 当成 all。
func matchFilter(q qbTorrent, filter string) bool {
	s := q.State
	in := func(states ...string) bool { return slices.Contains(states, s) }
	paused := in("pausedDL", "pausedUP")
	active := q.Dlspeed > 0 || q.Upspeed > 0 || in("downloading", "uploading", "forcedDL", "forcedUP", "metaDL", "moving")
	switch filter {
	case "downloading":
		return in("downloading", "metaDL", "forcedMetaDL", "stalledDL", "checkingDL", "pausedDL", "queuedDL", "forcedDL", "allocating")
	case "seeding":
		return in("uploading", "stalledUP", "checkingUP", "queuedUP", "forcedUP")
	case "completed":
		return in("uploading", "stalledUP", "checkingUP", "pausedUP", "queuedUP", "forcedUP")
	case "paused", "stopped":
		return paused
	case "resumed", "running":
		return !paused
	case "active":
		return active
	case "inactive":
		return !active
	case "stalled":
		return in("stalledUP", "stalledDL")
	case "stalled_uploading":
		return s == "stalledUP"
	case "stalled_downloading":
		return s == "stalledDL"
	case "checking":
		return in("checkingUP", "checkingDL", "checkingResumeData")
	case "moving":
		return s == "moving"
	case "errored":
		return in("error", "missingFiles")
	}
	return true
}

func magnetOf(hash, name string) string {
	m := "magnet:?xt=urn:btih:" + hash
	if name != "" {
		m += "&dn=" + url.QueryEscape(name)
	}
	return m
}

// redactTracker 遮住 tracker 地址里的 passkey（查询串与路径里的长串）；客户端靠主机认站点，够用。
func redactTracker(raw string) string {
	if raw == "" {
		return ""
	}
	return dlassistant.RedactTrackerURL(raw)
}

// redact 遮住要交给客户端的文本里的凭证（错误信息、tracker 回复）。
func redact(s string) string { return dlassistant.RedactTrackerMessage(s) }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// list 取绑定下载器的全部种子，换成 qB 的字段；同时给出 hash → 下载器种子 的索引（写接口要用下载器自己的编号）。
func (s *Server) list(b *backend) ([]qbTorrent, map[string]downloader.Torrent, error) {
	all, err := b.dl.GetAllTorrents()
	if err != nil {
		return nil, nil, err
	}
	out := make([]qbTorrent, 0, len(all))
	byHash := make(map[string]downloader.Torrent, len(all))
	labels := usesLabels(b)
	for _, t := range all {
		q := toQB(t, labels)
		out = append(out, q)
		byHash[q.Hash] = t
	}
	return out, byHash, nil
}

// torrentsInfo 是 GET /api/v2/torrents/info（filter、category、tag、sort、reverse、limit、offset、hashes）。
func (s *Server) torrentsInfo(w http.ResponseWriter, r *http.Request, _ *call) {
	b, ok := s.withBackend(w, r)
	if !ok {
		return
	}
	items, _, err := s.list(b)
	if err != nil {
		text(w, http.StatusServiceUnavailable, redact(err.Error()))
		return
	}
	_ = r.ParseForm()
	q := r.Form
	filter := q.Get("filter")
	var hashes map[string]bool
	if h := strings.TrimSpace(q.Get("hashes")); h != "" && h != "all" {
		hashes = splitHashes(h)
	}
	out := make([]qbTorrent, 0, len(items))
	for _, t := range items {
		if !matchFilter(t, filter) {
			continue
		}
		if hashes != nil && !hashes[t.Hash] {
			continue
		}
		if c, has := q["category"]; has && t.Category != c[0] {
			continue
		}
		if tg, has := q["tag"]; has && !hasTag(t.Tags, tg[0]) {
			continue
		}
		out = append(out, t)
	}
	if key := q.Get("sort"); key != "" {
		sortTorrents(out, key, formBool(q.Get("reverse")))
	}
	out = page(out, q.Get("offset"), q.Get("limit"))
	writeJSON(w, out)
}

func splitHashes(s string) map[string]bool {
	out := map[string]bool{}
	for _, h := range strings.Split(s, "|") {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			out[h] = true
		}
	}
	return out
}

// hasTag：tag 为空时找没有标签的种子，否则逗号分隔的标签里要有它。
func hasTag(tags, tag string) bool {
	if tag == "" {
		return strings.TrimSpace(tags) == ""
	}
	for _, t := range strings.Split(tags, ",") {
		if strings.TrimSpace(t) == tag {
			return true
		}
	}
	return false
}

// sortTorrents 按回应里的任意字段排序（qB 的 sort）：数字比大小、字符串比字典序、布尔假在前。
func sortTorrents(list []qbTorrent, key string, reverse bool) {
	vals := make([]map[string]any, len(list))
	for i, t := range list {
		b, _ := json.Marshal(t)
		_ = json.Unmarshal(b, &vals[i])
	}
	idx := make([]int, len(list))
	for i := range idx {
		idx[i] = i
	}
	slices.SortStableFunc(idx, func(a, b int) int {
		c := compareAny(vals[a][key], vals[b][key])
		if reverse {
			return -c
		}
		return c
	})
	sorted := make([]qbTorrent, len(list))
	for i, j := range idx {
		sorted[i] = list[j]
	}
	copy(list, sorted)
}

func compareAny(a, b any) int {
	switch x := a.(type) {
	case float64:
		y, _ := b.(float64)
		return cmp.Compare(x, y)
	case string:
		y, _ := b.(string)
		return strings.Compare(strings.ToLower(x), strings.ToLower(y))
	case bool:
		y, _ := b.(bool)
		switch {
		case x == y:
			return 0
		case !x:
			return -1
		}
		return 1
	}
	return 0
}

// page 是 offset（小于 0 时从末尾数）与 limit。
func page(list []qbTorrent, offsetStr, limitStr string) []qbTorrent {
	offset, _ := strconv.Atoi(offsetStr)
	if offset < 0 {
		offset = max(len(list)+offset, 0)
	}
	if offset > len(list) {
		offset = len(list)
	}
	list = list[offset:]
	if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit < len(list) {
		list = list[:limit]
	}
	return list
}

// find 按 hash 找种子；找不到时回 404（qB 的 Torrent hash was not found）。
func (s *Server) find(w http.ResponseWriter, r *http.Request) (*backend, downloader.Torrent, bool) {
	b, ok := s.withBackend(w, r)
	if !ok {
		return nil, downloader.Torrent{}, false
	}
	_ = r.ParseForm()
	hash := strings.ToLower(strings.TrimSpace(r.Form.Get("hash")))
	_, byHash, err := s.list(b)
	if err != nil {
		text(w, http.StatusServiceUnavailable, redact(err.Error()))
		return nil, downloader.Torrent{}, false
	}
	t, found := byHash[hash]
	if hash == "" || !found {
		text(w, http.StatusNotFound, "Torrent hash was not found")
		return nil, downloader.Torrent{}, false
	}
	return b, t, true
}

// properties 是 GET /api/v2/torrents/properties?hash=。
func (s *Server) properties(w http.ResponseWriter, r *http.Request, _ *call) {
	b, t, ok := s.find(w, r)
	if !ok {
		return
	}
	q := toQB(t, usesLabels(b))
	completion := q.CompletionOn
	if completion <= 0 {
		completion = -1
	}
	writeJSON(w, map[string]any{
		"hash": q.Hash, "name": q.Name, "save_path": q.SavePath, "creation_date": -1, "piece_size": -1, "comment": "",
		"total_wasted": 0, "total_uploaded": q.Uploaded, "total_uploaded_session": q.UploadedSession,
		"total_downloaded": q.Downloaded, "total_downloaded_session": q.DownloadedSession, "up_limit": q.UpLimit,
		"dl_limit": q.DlLimit, "time_elapsed": q.TimeActive, "seeding_time": q.SeedingTime,
		"nb_connections": q.NumSeeds + q.NumLeechs, "nb_connections_limit": -1, "share_ratio": q.Ratio,
		"addition_date": q.AddedOn, "completion_date": completion, "created_by": "", "dl_speed_avg": 0,
		"dl_speed": q.Dlspeed, "eta": q.ETA, "last_seen": q.SeenComplete, "peers": q.NumLeechs,
		"peers_total": q.NumIncomplete, "pieces_have": -1, "pieces_num": -1, "reannounce": 0, "seeds": q.NumSeeds,
		"seeds_total": q.NumComplete, "total_size": q.TotalSize, "up_speed_avg": 0, "up_speed": q.Upspeed,
		"isPrivate": q.IsPrivate,
	})
}

// files 是 GET /api/v2/torrents/files?hash=。
func (s *Server) files(w http.ResponseWriter, r *http.Request, _ *call) {
	b, t, ok := s.find(w, r)
	if !ok {
		return
	}
	files, err := b.dl.GetTorrentFiles(t.ID)
	if err != nil {
		text(w, http.StatusServiceUnavailable, redact(err.Error()))
		return
	}
	out := make([]map[string]any, 0, len(files))
	for _, f := range files {
		out = append(out, map[string]any{
			"index": f.Index, "name": f.Name, "size": f.Size, "progress": f.Progress, "priority": f.Priority,
			"is_seed": f.Progress >= 1, "piece_range": []int{0, 0}, "availability": -1,
		})
	}
	writeJSON(w, out)
}

// trackers 是 GET /api/v2/torrents/trackers?hash=：地址里的 passkey 遮住，tracker 回复也过一遍遮挡。
func (s *Server) trackers(w http.ResponseWriter, r *http.Request, _ *call) {
	b, t, ok := s.find(w, r)
	if !ok {
		return
	}
	trackers, err := b.dl.GetTorrentTrackers(t.ID)
	if err != nil {
		text(w, http.StatusServiceUnavailable, redact(err.Error()))
		return
	}
	out := make([]map[string]any, 0, len(trackers))
	for _, tr := range trackers {
		out = append(out, map[string]any{
			"url": redactTracker(tr.URL), "status": tr.Status, "tier": 0, "num_peers": tr.Peers, "num_seeds": tr.Seeds,
			"num_leeches": tr.Leeches, "num_downloaded": -1, "msg": redact(tr.Message),
		})
	}
	writeJSON(w, out)
}

// categoryView 是 torrents/categories 的一项。
type categoryView struct {
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
}

// allCategories 是分类：兼容入口记下的、下载器自己的、种子上用到的。
func (s *Server) allCategories(b *backend, items []qbTorrent) map[string]categoryView {
	out := map[string]categoryView{}
	// qB 的 GetClientLabels 是它的分类；Transmission 的是标签，不算分类
	if b.setting.Type == string(downloader.DownloaderQBittorrent) {
		if labels, err := b.dl.GetClientLabels(); err == nil {
			for _, l := range labels {
				if l != "" {
					out[l] = categoryView{Name: l}
				}
			}
		}
	}
	for _, t := range items {
		if t.Category != "" {
			out[t.Category] = categoryView{Name: t.Category}
		}
	}
	for name, path := range categoryMap(b.cfg) {
		out[name] = categoryView{Name: name, SavePath: path}
	}
	return out
}

func allTags(b *backend, items []qbTorrent) []string {
	seen := map[string]bool{}
	for _, t := range tagList(b.cfg) {
		seen[t] = true
	}
	for _, t := range items {
		for _, tag := range strings.Split(t.Tags, ",") {
			if tag = strings.TrimSpace(tag); tag != "" {
				seen[tag] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	slices.Sort(out)
	return out
}

// categories 是 GET /api/v2/torrents/categories。
func (s *Server) categories(w http.ResponseWriter, r *http.Request, _ *call) {
	b, ok := s.withBackend(w, r)
	if !ok {
		return
	}
	items, _, err := s.list(b)
	if err != nil {
		text(w, http.StatusServiceUnavailable, redact(err.Error()))
		return
	}
	writeJSON(w, s.allCategories(b, items))
}

// tags 是 GET /api/v2/torrents/tags。
func (s *Server) tags(w http.ResponseWriter, r *http.Request, _ *call) {
	b, ok := s.withBackend(w, r)
	if !ok {
		return
	}
	items, _, err := s.list(b)
	if err != nil {
		text(w, http.StatusServiceUnavailable, redact(err.Error()))
		return
	}
	writeJSON(w, allTags(b, items))
}

// maindata 是 GET /api/v2/sync/maindata：每次都给全量（full_update），rid 递增，客户端照常能用。
func (s *Server) maindata(w http.ResponseWriter, r *http.Request, _ *call) {
	b, ok := s.withBackend(w, r)
	if !ok {
		return
	}
	items, _, err := s.list(b)
	if err != nil {
		text(w, http.StatusServiceUnavailable, redact(err.Error()))
		return
	}
	torrents := make(map[string]qbTorrent, len(items))
	for _, t := range items {
		torrents[t.Hash] = t
	}
	state := s.transfer(b)
	if free, ferr := b.dl.GetClientFreeSpace(r.Context()); ferr == nil {
		state["free_space_on_disk"] = free
	}
	state["queueing"], state["use_alt_speed_limits"], state["refresh_interval"] = false, false, 1500
	writeJSON(w, map[string]any{
		"rid": s.rid.Add(1), "full_update": true, "torrents": torrents, "categories": s.allCategories(b, items),
		"tags": allTags(b, items), "server_state": state,
	})
}
