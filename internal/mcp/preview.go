package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// 写工具的预览：不带 confirm=true 调写工具时不执行，回一份预览 —— 会改什么（改以前、改以后）、可能有什么影响。
// 内置 AI 助手拿它做确认卡片，外部的 MCP 客户端照它问用户，用户同意以后带 confirm=true 再调一次才执行。
// 预览只读：经 App API 的读接口取现状（Request.Preview，令牌有 mcp:write 就能读）。

// Change 是一项改动：改的是什么、改以前、改以后（新增的没有 Before，删掉的没有 After）。Secret 为真时是密钥，两边都不写原文。
type Change struct {
	Label  string `json:"label"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Secret bool   `json:"secret,omitempty"`
}

// Preview 是写工具的预览。Pending 总是真：这次没有执行。
type Preview struct {
	Pending bool     `json:"pending"`
	Tool    string   `json:"tool"`
	Summary string   `json:"summary"`
	Changes []Change `json:"changes"`
	Impact  []string `json:"impact"`
	Note    string   `json:"note"`
}

// previewNote 是预览里给调用方的那句话。
const previewNote = "没有执行：把上面的改动与影响告诉用户，用户同意以后带上 confirm=true 再调用一次才会执行。"

// previewer 是一个写工具的预览：按参数读现状，写出改动与影响。读不到现状时也给一份（影响里写明读不到），只有确定执行不了时才回错误。
type previewer[In any] func(ctx context.Context, d *Deps, c Caller, in In) (Preview, error)

// readJSON 经 App API 的读接口取 JSON（预览用：令牌有 mcp:write 就能读）。
func readJSON(ctx context.Context, d *Deps, c Caller, method, path string, body, out any) error {
	resp, err := d.Backend.Call(ctx, c, Request{Method: method, Path: path, Body: body, Preview: true})
	if err != nil {
		return err
	}
	if resp.Status < 200 || resp.Status >= 300 {
		e := &errAPI{Status: resp.Status}
		_ = json.Unmarshal(resp.Body, e)
		return e
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(resp.Body, out)
}

// notFound 报告 App API 回的是 404。
func notFound(err error) bool {
	var e *errAPI
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

// unreadable 是读不到现状时写进影响的一句话。
func unreadable(what string, err error) string {
	return fmt.Sprintf("读不到%s（%s）：确认以后照样会执行", what, oneLine(err.Error(), 120))
}

func oneLine(s string, limit int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return s
}

// sizeText 把字节数写成 1.5 GB 这样。
func sizeText(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	v, i := float64(n), 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return strconv.FormatFloat(v, 'f', 1, 64) + " " + units[i]
}

// ---- 下载器里的种子 ----

// torrentNow 是下载器里一个种子的现状（App API 的 AppTorrent 里预览要的几项）。
type torrentNow struct {
	DownloaderID uint    `json:"downloader_id"`
	Downloader   string  `json:"downloader"`
	TaskID       string  `json:"task_id"`
	InfoHash     string  `json:"info_hash"`
	Title        string  `json:"title"`
	Progress     float64 `json:"progress"`
	Size         int64   `json:"size"`
	State        string  `json:"state"`
	Ratio        float64 `json:"ratio"`
	SavePath     string  `json:"save_path"`
}

// stateNames 是种子状态的中文叫法。
var stateNames = map[string]string{
	"downloading": "下载中", "seeding": "做种中", "paused": "已暂停", "stopped": "已停止",
	"queued": "排队中", "checking": "校验中", "error": "出错", "unknown": "未知",
}

func stateName(s string) string {
	if n := stateNames[s]; n != "" {
		return n
	}
	return s
}

// finished 报告种子下载完了（App API 的进度是 0–1）。
func (t torrentNow) finished() bool { return t.Progress >= 0.9999 }

// percent 是下载进度的百分数。
func (t torrentNow) percent() float64 { return min(max(t.Progress, 0), 1) * 100 }

func (t torrentNow) label() string {
	return fmt.Sprintf("「%s」（%s，%s）", t.Title, sizeText(t.Size), stateName(t.State))
}

// findTorrent 在下载器里找任务：先按关键字（任务编号多半就是 info hash）找，找不到再看第一页。没有时回 nil。
func findTorrent(ctx context.Context, d *Deps, c Caller, downloaderID uint, taskID string) (*torrentNow, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, errors.New("task_id 要填（从 list_downloader_torrents 取）")
	}
	var page struct {
		Items    []torrentNow `json:"items"`
		Failures []struct {
			DownloaderID uint   `json:"downloader_id"`
			Error        string `json:"error"`
		} `json:"failures"`
	}
	for _, q := range []string{taskID, ""} {
		path := withQuery("/torrents", "downloader_id", utoa(downloaderID), "q", strings.ToLower(q), "page_size", "200")
		if err := readJSON(ctx, d, c, http.MethodGet, path, nil, &page); err != nil {
			return nil, err
		}
		for _, f := range page.Failures {
			if f.DownloaderID == downloaderID {
				// 下载器这次没读出来：不知道有没有这个种子
				return nil, errors.New(f.Error)
			}
		}
		for i := range page.Items {
			if t := page.Items[i]; t.TaskID == taskID || strings.EqualFold(t.InfoHash, taskID) {
				return &t, nil
			}
		}
	}
	return nil, nil
}

// torrentPreview 是暂停、继续、删除种子的预览。
func torrentPreview(ctx context.Context, d *Deps, c Caller, tool string, downloaderID uint, taskID string, removeData bool) (Preview, error) {
	p := Preview{Tool: tool}
	t, err := findTorrent(ctx, d, c, downloaderID, taskID)
	switch {
	case err != nil && notFound(err):
		return p, fmt.Errorf("没有编号 %d 的下载器", downloaderID)
	case err != nil:
		p.Summary = fmt.Sprintf("%s：下载器 %d 的任务 %s", toolVerb(tool), downloaderID, taskID)
		p.Impact = append(p.Impact, unreadable("这个种子的现状", err))
		return p, nil
	case t == nil:
		return p, fmt.Errorf("下载器 %d 里没有任务 %s：先用 list_downloader_torrents 查一下", downloaderID, taskID)
	}
	where := fmt.Sprintf("下载器「%s」里的%s", t.Downloader, t.label())
	switch tool {
	case "pause_torrent":
		p.Summary = "暂停" + where
		p.Changes = []Change{{Label: "状态", Before: stateName(t.State), After: "已暂停"}}
		if t.finished() {
			p.Impact = append(p.Impact, fmt.Sprintf("停止做种：不再上传，分享率（现在 %.2f）与做种时间不再增长", t.Ratio))
			p.Impact = append(p.Impact, "站点要求做种时间（H&R）的，暂停期间不算做种时间")
		} else {
			p.Impact = append(p.Impact, fmt.Sprintf("下载停在 %.1f%%，暂停期间不下载也不上传", t.percent()))
		}
		p.Impact = append(p.Impact, "随时可以用「继续」恢复")
	case "resume_torrent":
		p.Summary = "继续" + where
		after := "下载中"
		if t.finished() {
			after = "做种中"
		}
		p.Changes = []Change{{Label: "状态", Before: stateName(t.State), After: after}}
		if t.finished() {
			p.Impact = append(p.Impact, "接着做种，会占用上传带宽")
		} else {
			left := int64(float64(t.Size) * (1 - t.percent()/100))
			p.Impact = append(p.Impact, fmt.Sprintf("接着下载，还要写入约 %s", sizeText(left)))
		}
	default: // delete_torrent
		p.Summary = "从下载器删除" + where
		files := "保留在 " + orDash(t.SavePath)
		if removeData {
			files = "一起删除"
		}
		p.Changes = []Change{
			{Label: "种子", Before: t.Title, After: "从下载器「" + t.Downloader + "」删除"},
			{Label: "已下载的文件", Before: fmt.Sprintf("%s（%.1f%%）", sizeText(t.Size), t.percent()), After: files},
		}
		if removeData {
			p.Impact = append(p.Impact, fmt.Sprintf("文件删除以后不能恢复，释放约 %s", sizeText(int64(float64(t.Size)*t.percent()/100))))
		} else {
			p.Impact = append(p.Impact, "文件留在磁盘上，下载器里不再做种；要删文件得另外处理")
		}
		if t.finished() {
			p.Impact = append(p.Impact, "站点要求做种时间（H&R）而这个种子还没做够的，删除以后可能被记 H&R")
		}
		p.Impact = append(p.Impact, "已经整理进媒体库的硬链接文件不受影响")
	}
	return p, nil
}

func toolVerb(tool string) string {
	switch tool {
	case "pause_torrent":
		return "暂停种子"
	case "resume_torrent":
		return "继续种子"
	default:
		return "删除种子"
	}
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "原来的目录"
	}
	return s
}

// ---- 推送 ----

type downloaderNow struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	Default   bool   `json:"default"`
	Reachable bool   `json:"reachable"`
	Error     string `json:"error"`
	FreeSpace int64  `json:"free_space"`
}

// pushPreview 是推送种子的预览：推到哪台下载器、目录与分类，以及剩余空间。
func pushPreview(ctx context.Context, d *Deps, c Caller, site, id string, in pushIn) Preview {
	p := Preview{Tool: "push_torrent", Summary: fmt.Sprintf("把站点 %s 的种子 %s 推送到下载器", site, id)}
	var list struct {
		Items []downloaderNow `json:"items"`
	}
	var target *downloaderNow
	if err := readJSON(ctx, d, c, http.MethodGet, "/downloaders", nil, &list); err != nil {
		p.Impact = append(p.Impact, unreadable("下载器的状态", err))
	} else {
		for i := range list.Items {
			dl := list.Items[i]
			if (in.DownloaderID != 0 && dl.ID == in.DownloaderID) || (in.DownloaderID == 0 && dl.Default) {
				target = &dl
				break
			}
		}
	}
	to := "默认下载器"
	if target != nil {
		to = "「" + target.Name + "」"
	} else if in.DownloaderID != 0 {
		to = "下载器 " + utoa(in.DownloaderID)
	}
	p.Summary = fmt.Sprintf("把站点 %s 的种子 %s 推送到%s", site, id, to)
	p.Changes = []Change{
		{Label: "推送到", After: strings.Trim(to, "「」")},
		{Label: "种子", After: "站点 " + site + " 的种子 " + id},
	}
	if in.SavePath != "" {
		p.Changes = append(p.Changes, Change{Label: "保存目录", After: in.SavePath})
	}
	if in.Category != "" {
		p.Changes = append(p.Changes, Change{Label: "分类", After: in.Category})
	}
	if in.Tags != "" {
		p.Changes = append(p.Changes, Change{Label: "标签", After: in.Tags})
	}
	p.Impact = append(p.Impact, "pt-tools 按站点设置下载种子文件再推送；剩余空间不够或者站点做种数到了上限时不推送")
	if target != nil {
		if target.Reachable {
			p.Impact = append(p.Impact, fmt.Sprintf("下载器「%s」现在剩余 %s；下载会占用空间与带宽", target.Name, sizeText(target.FreeSpace)))
		} else {
			p.Impact = append(p.Impact, fmt.Sprintf("下载器「%s」现在连不上（%s），推送多半会失败", target.Name, oneLine(target.Error, 80)))
		}
	}
	p.Impact = append(p.Impact, "站点有 H&R 考核的，下完以后要按站点要求做种")
	return p
}

// ---- 订阅 ----

type subscriptionNow struct {
	ID        uint   `json:"id"`
	MediaType string `json:"media_type"`
	Season    int    `json:"season"`
	Title     string `json:"title"`
	Year      int    `json:"year"`
	Status    string `json:"status"`
	Upgrade   bool   `json:"upgrade"`
	Progress  struct {
		InLibrary int   `json:"in_library"`
		Missing   []int `json:"missing"`
	} `json:"progress"`
}

var subStatusNames = map[string]string{"active": "订阅中", "paused": "已暂停", "pending": "等待上映", "done": "已完成"}

func subStatusName(s string) string {
	if n := subStatusNames[s]; n != "" {
		return n
	}
	return s
}

func (s subscriptionNow) label() string {
	name := "「" + s.Title + "」"
	if s.Year > 0 {
		name += fmt.Sprintf("（%d）", s.Year)
	}
	if s.MediaType == "tv" && s.Season > 0 {
		name += fmt.Sprintf("第 %d 季", s.Season)
	}
	return name
}

// subscriptionPreview 是暂停、恢复、立即搜索、删除订阅的预览。
func subscriptionPreview(ctx context.Context, d *Deps, c Caller, tool string, id uint, status string) (Preview, error) {
	p := Preview{Tool: tool}
	var sub subscriptionNow
	err := readJSON(ctx, d, c, http.MethodGet, "/subscriptions/"+utoa(id), nil, &sub)
	switch {
	case err != nil && notFound(err):
		return p, fmt.Errorf("没有编号 %d 的订阅：先用 list_subscriptions 查一下", id)
	case err != nil:
		p.Summary = fmt.Sprintf("订阅 %d", id)
		p.Impact = append(p.Impact, unreadable("这个订阅", err))
		return p, nil
	}
	switch tool {
	case "set_subscription_status":
		if status == "paused" {
			p.Summary = "暂停订阅" + sub.label()
			p.Impact = append(p.Impact, "暂停以后不再自动搜索与下载；已经下载、整理进媒体库的不受影响")
		} else {
			p.Summary = "恢复订阅" + sub.label()
			p.Impact = append(p.Impact, "恢复以后按计划在各站搜索，搜到符合的就推送到下载器（占用空间）")
		}
		p.Changes = []Change{{Label: "状态", Before: subStatusName(sub.Status), After: subStatusName(status)}}
	case "search_subscription":
		p.Summary = "立即搜索订阅" + sub.label()
		missing := "还缺的"
		if n := len(sub.Progress.Missing); n > 0 {
			missing = fmt.Sprintf("还缺的 %d 集", n)
		}
		p.Changes = []Change{{Label: "下载器里的种子", After: "搜到符合的就新增"}}
		p.Impact = append(p.Impact, "现在去各站搜索"+missing+"，找到符合的会马上推送到下载器（占用空间与带宽）")
	default: // delete_subscription
		p.Summary = "删除订阅" + sub.label()
		p.Changes = []Change{{Label: "订阅", Before: sub.label() + "，" + subStatusName(sub.Status), After: "删除"}}
		p.Impact = append(p.Impact, "删除以后不再自动搜索与下载；已经下载的种子与媒体库里的文件留着")
		if sub.Upgrade {
			p.Impact = append(p.Impact, "这个订阅开着洗版：删掉以后不再找更好的版本")
		}
	}
	return p, nil
}

// addSubscriptionPreview 是添加订阅的预览（订阅还不存在，只能按参数写）。
func addSubscriptionPreview(in addSubIn) Preview {
	what := "电影"
	if in.MediaType == "tv" {
		what = "剧集"
		if in.Season > 0 {
			what = fmt.Sprintf("剧集第 %d 季", in.Season)
		}
	}
	p := Preview{
		Tool: "add_subscription", Summary: fmt.Sprintf("订阅 TMDB %d 的%s", in.TMDBID, what),
		Changes: []Change{{Label: "订阅", After: fmt.Sprintf("新增：TMDB %d（%s）", in.TMDBID, what)}},
		Impact:  []string{"pt-tools 会按计划在各站搜索，找到符合质量要求的种子自动推送到下载器（占用空间与带宽）"},
	}
	if in.Upgrade {
		p.Changes = append(p.Changes, Change{Label: "洗版", After: "开"})
		p.Impact = append(p.Impact, "开着洗版：下完以后还会找更好的版本，找到就替换媒体库里的文件")
	}
	return p
}

// ---- 媒体库 ----

type organizePlan struct {
	Downloader string `json:"downloader"`
	Name       string `json:"name"`
	Library    *struct {
		Name string `json:"name"`
	} `json:"library"`
	Mode  string `json:"mode"`
	Items []struct {
		Rel     string `json:"rel"`
		Size    int64  `json:"size"`
		Target  string `json:"target"`
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"items"`
	Skipped int    `json:"skipped"`
	Problem string `json:"problem"`
}

var modeNames = map[string]string{"hardlink": "硬链接", "copy": "复制", "symlink": "软链接", "move": "移动"}

// modeImpact 是整理方式的影响。
func modeImpact(mode string, size int64) string {
	switch mode {
	case "copy":
		return fmt.Sprintf("复制：媒体库里多占约 %s，原文件留着继续做种", sizeText(size))
	case "move":
		return "移动：原文件搬走以后下载器里的种子找不到文件，不能再做种"
	case "symlink":
		return "软链接：不占额外空间；原文件删了媒体库里的也没了"
	default:
		return "硬链接：不占额外空间，原文件留着继续做种"
	}
}

// planPreview 把整理计划写成预览：每个视频文件从哪儿到哪儿。
func planPreview(tool string, plan organizePlan) (Preview, error) {
	p := Preview{Tool: tool, Summary: fmt.Sprintf("把「%s」整理进媒体库", plan.Name)}
	if plan.Problem != "" {
		return p, fmt.Errorf("这个种子整理不了：%s", plan.Problem)
	}
	lib := "按类型选的媒体库"
	if plan.Library != nil && plan.Library.Name != "" {
		lib = "媒体库「" + plan.Library.Name + "」"
	}
	p.Summary = fmt.Sprintf("把下载器「%s」里的「%s」整理进%s", plan.Downloader, plan.Name, lib)
	var total int64
	exists := 0
	for _, it := range plan.Items {
		switch it.Status {
		case "done":
			continue
		case "exists":
			exists++
			continue
		}
		total += it.Size
		after := it.Target
		if it.Status == "replace" {
			after += "（替换已有的文件）"
		}
		if it.Status == "failed" {
			after = "整理不了：" + it.Message
		}
		if len(p.Changes) < 20 {
			p.Changes = append(p.Changes, Change{Label: it.Rel, Before: sizeText(it.Size), After: after})
		}
	}
	if n := len(plan.Items) - exists; n > 20 {
		p.Changes = append(p.Changes, Change{Label: "…", After: fmt.Sprintf("还有 %d 个文件", n-20)})
	}
	if len(p.Changes) == 0 {
		return p, errors.New("这个种子的文件都已经在媒体库里了，没有要整理的")
	}
	p.Changes = append([]Change{{Label: "整理方式", After: modeNames[orDefaultMode(plan.Mode)]}}, p.Changes...)
	p.Impact = append(p.Impact, modeImpact(orDefaultMode(plan.Mode), total))
	if exists > 0 {
		p.Impact = append(p.Impact, fmt.Sprintf("有 %d 个文件的目标位置已经有同名文件：不覆盖，记为已存在", exists))
	}
	p.Impact = append(p.Impact, "按设置写 NFO 与海报，并通知媒体服务器扫描")
	return p, nil
}

func orDefaultMode(m string) string {
	if m == "" {
		return "hardlink"
	}
	return m
}

// organizePreview 是整理进媒体库的预览：照 preview_media_organize 读出整理计划。
func organizePreview(ctx context.Context, d *Deps, c Caller, in organizeIn) (Preview, error) {
	body, err := organizeBody(in.DownloaderID, in.Hash, in.MediaType, in.TMDBID, in.LibraryID)
	if err != nil {
		return Preview{}, err
	}
	var plan organizePlan
	if err := readJSON(ctx, d, c, http.MethodPost, "/media/organize/preview", body, &plan); err != nil {
		if notFound(err) {
			return Preview{}, err
		}
		return Preview{
			Tool: "organize_media", Summary: fmt.Sprintf("整理下载器 %d 的种子 %s", in.DownloaderID, in.Hash),
			Impact: []string{unreadable("整理计划", err)},
		}, nil
	}
	return planPreview("organize_media", plan)
}

// retryPreview 是重新整理一条记录的预览：记录的种子、目标位置与当时的原因。
func retryPreview(ctx context.Context, d *Deps, c Caller, id uint) (Preview, error) {
	p := Preview{Tool: "retry_media_organize", Summary: fmt.Sprintf("重新整理记录 %d", id)}
	var page struct {
		Items []struct {
			ID          uint   `json:"id"`
			Title       string `json:"title"`
			TorrentName string `json:"torrent_name"`
			Library     string `json:"library"`
			TargetPath  string `json:"target_path"`
			Mode        string `json:"mode"`
			Size        int64  `json:"size"`
			Status      string `json:"status"`
			Message     string `json:"message"`
		} `json:"items"`
	}
	if err := readJSON(ctx, d, c, http.MethodGet, withQuery("/media/history", "page_size", "100"), nil, &page); err != nil {
		p.Impact = append(p.Impact, unreadable("整理记录", err))
		return p, nil
	}
	for _, it := range page.Items {
		if it.ID != id {
			continue
		}
		p.Summary = fmt.Sprintf("重新整理「%s」（%s）", it.Title, it.TorrentName)
		before := "上次：" + it.Status
		if it.Message != "" {
			before += "，" + oneLine(it.Message, 80)
		}
		p.Changes = []Change{{Label: "整理记录 " + utoa(id), Before: before, After: "按现在的设置重新整理"}}
		if it.TargetPath != "" {
			p.Changes = append(p.Changes, Change{Label: "目标位置", After: it.TargetPath})
		}
		p.Impact = append(p.Impact, modeImpact(orDefaultMode(it.Mode), it.Size))
		p.Impact = append(p.Impact, "目标位置已经有别的文件时不覆盖")
		return p, nil
	}
	p.Impact = append(p.Impact, "最近 100 条整理记录里没有这一条：确认以后照样会按编号重新整理")
	return p, nil
}
