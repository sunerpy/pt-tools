package health

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/sunerpy/pt-tools/internal/fsutil"
	"github.com/sunerpy/pt-tools/internal/media/tmdb"
	"github.com/sunerpy/pt-tools/internal/redact"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
	"github.com/sunerpy/pt-tools/utils"
)

const (
	// probeTimeout 是一台下载器、一台媒体服务器最多等多久
	probeTimeout = 20 * time.Second
	// probeParallel 是一组里同时探测几台
	probeParallel = 4
	// clockSkew 是本机时间和 TMDB 差多少算不对
	clockSkew = 5 * time.Minute
	// backupStale 是开着定时备份时，最近一次成功距今超过多久要提醒（两个周期）
	backupStale = 48 * time.Hour
	// maxDetail 是写进说明的错误最多多长（按字符）
	maxDetail = 300
)

// diskUsage 读一个目录所在的盘（测试里换掉）。
var diskUsage = fsutil.DiskUsage

// stamp 是说明里写的时间（本地时间，月-日 时:分）。
func stamp(t time.Time) string { return t.In(time.Local).Format("01-02 15:04") }

// short 把错误整理成说明里写的样子：去掉凭证，太长时截断。
func short(s string) string {
	s = strings.TrimSpace(redact.Secrets(s))
	if r := []rune(s); len(r) > maxDetail {
		s = string(r[:maxDetail]) + "…"
	}
	return s
}

// TMDB 检查 TMDB：接口能不能用（API Key 与代理）、图片地址能不能取图，再拿响应的时间比本机的时钟。
// client 按现在的设置建客户端；poster 给一张拿来试的海报（订阅里的，没有时为空，接口能用时另取一部电影的海报）。
func TMDB(client func(ctx context.Context) (*tmdb.Client, error), poster func(ctx context.Context) string, now func() time.Time) Check {
	return Check{Group: GroupTMDB, Title: "TMDB", Run: func(ctx context.Context) []Item {
		api := Item{Key: "tmdb", Group: GroupTMDB, Title: "TMDB 接口"}
		img := Item{Key: "tmdb:image", Group: GroupTMDB, Title: "TMDB 图片"}
		clock := Item{Key: "time", Group: GroupTime, Title: "本机时间"}
		c, err := client(ctx)
		if err != nil {
			api.Status, api.Detail = StatusFail, short(err.Error())
			api.Hint = "在「媒体识别」里检查 TMDB 的设置"
			if errors.Is(err, tmdb.ErrNoKey) {
				api.Status, api.Detail = StatusWarn, "没有 TMDB API Key：识别、整理入库与订阅都用不了"
				api.Hint = "在「媒体识别」里填 TMDB API Key"
			}
			img.Status, img.Detail = StatusSkip, "TMDB 用不了，没法试图片地址"
			clock.Status, clock.Detail = StatusSkip, "TMDB 用不了，没法比本机时间"
			return []Item{api, img, clock}
		}
		at, err := c.ServerTime(ctx)
		switch {
		case err == nil:
			api.Status, api.Detail = StatusOK, "API Key 可用"
		case errors.Is(err, tmdb.ErrUnauthorized):
			api.Status, api.Detail, api.Hint = StatusFail, "API Key 无效", "在「媒体识别」里换一个 TMDB API Key"
		case errors.Is(err, tmdb.ErrRateLimited):
			api.Status, api.Detail, api.Hint = StatusWarn, "请求太频繁，TMDB 暂时拒绝了", "过几分钟再体检一次"
		default:
			api.Status, api.Detail = StatusFail, short(err.Error())
			api.Hint = "连不上 TMDB：检查网络，或在「媒体识别」里设置代理"
		}
		clock = clockItem(clock, at, err, now())
		path := poster(ctx)
		if path == "" && err == nil {
			// 订阅里没有海报时拿一部电影（黑客帝国）的海报来试
			if d, derr := c.Details(ctx, tmdb.KindMovie, 603); derr == nil {
				path = d.PosterPath
			}
		}
		if path == "" {
			img.Status, img.Detail = StatusSkip, "没有拿来试的海报"
			return []Item{api, img, clock}
		}
		if _, err := c.Image(ctx, path, "w92"); err != nil {
			img.Status, img.Detail = StatusWarn, short(err.Error())
			img.Hint = "海报与刮削的图片取不到：在「媒体识别」里把图片地址换回官方的，或检查代理"
		} else {
			img.Status, img.Detail = StatusOK, "能取到海报"
		}
		return []Item{api, img, clock}
	}}
}

// clockItem 比本机时间和 TMDB 响应的时间。
func clockItem(it Item, server time.Time, err error, now time.Time) Item {
	if err != nil {
		it.Status, it.Detail = StatusSkip, "TMDB 接口出错，没法比本机时间"
		return it
	}
	if server.IsZero() {
		it.Status, it.Detail = StatusSkip, "TMDB 的响应里没有时间，没法比"
		return it
	}
	diff := now.Sub(server)
	way := "快"
	if diff < 0 {
		diff, way = -diff, "慢"
	}
	if diff <= clockSkew {
		it.Status, it.Detail = StatusOK, "和 TMDB 的时间相差 "+span(diff)
		return it
	}
	it.Status = StatusWarn
	it.Detail = fmt.Sprintf("本机时间比 TMDB 的%s %s", way, span(diff))
	it.Hint = "校准系统时间（打开 NTP 自动对时；Docker 里的时间跟宿主机走）：时间不对时签到、免费到期与定时任务都会算错"
	return it
}

// span 是时间差的说法：一分钟以内写秒，一小时以内写分钟，再长写小时。
func span(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d 秒", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟", int(d/time.Minute))
	default:
		return fmt.Sprintf("%d 小时", int(d/time.Hour))
	}
}

// probeText 是登录探测状态的说法（同网页上的 probeStatus.ts）。
var probeText = map[string]string{
	"SESSION_EXPIRED": "会话已过期",
	"CHALLENGE":       "被反爬拦截",
	"RATE_LIMITED":    "请求过于频繁",
	"KEY_ERROR":       "密钥错误",
	"NETWORK_ERROR":   "网络错误",
	"PARSE_ERROR":     "解析失败",
	"UNKNOWN":         "未知状态",
	"NOT_CONFIGURED":  "未配置凭证",
	"NOT_APPLICABLE":  "不适用",
	"UNSUPPORTED":     "暂不支持探测",
}

var probeHint = map[string]string{
	"SESSION_EXPIRED": "Cookie 过期了：在浏览器里重新登录站点，再用扩展同步 Cookie，或在站点设置里换上新的",
	"CHALLENGE":       "站点的防护拦下了请求：在浏览器里打开站点通过验证后重新同步 Cookie；常被拦的站点可以配置 CloakBrowser",
	"RATE_LIMITED":    "请求太频繁被站点限流了：稍后会自动再探测",
	"KEY_ERROR":       "API Key 不对或过期了：在站点设置里重新填",
	"NETWORK_ERROR":   "连不上站点：检查网络与站点代理",
	"PARSE_ERROR":     "认不出站点返回的页面（可能改版了）：在站点的登录状态里看详情",
	"UNKNOWN":         "在站点的登录状态里看详情",
	"NOT_CONFIGURED":  "没填 Cookie 或 API Key：在站点设置里填上",
}

// probeModeDisabled 是关掉登录探测的站点（scheduler.ProbeModeDisabled）。
const probeModeDisabled = "disabled"

// abnormalProbes 是算登录异常的探测状态（同每日战报的「登录状态异常」）。
var abnormalProbes = []string{"SESSION_EXPIRED", "KEY_ERROR", "CHALLENGE", "NETWORK_ERROR", "RATE_LIMITED", "PARSE_ERROR", "UNKNOWN"}

// AbnormalProbes 是算登录异常的探测状态（同每日战报）：没配凭证、不适用、暂不支持探测的不算。
func AbnormalProbes() []string { return slices.Clone(abnormalProbes) }

// ProbeLabel 是登录探测状态的说法（同网页上的 probeStatus.ts）；不认得的写「未知状态」。
func ProbeLabel(status string) string {
	if t, ok := probeText[status]; ok {
		return t
	}
	return probeText["UNKNOWN"]
}

// Sites 看各个启用的站点最近一次登录探测的结果，不发新的请求：正常的算通过，其余的提醒（探测关着、没探测过的跳过）。
func Sites(db *gorm.DB) Check {
	return Check{Group: GroupSites, Title: "站点", Run: func(ctx context.Context) []Item {
		var sites []models.SiteSetting
		if err := db.WithContext(ctx).Where("enabled = ?", true).Order("name").Find(&sites).Error; err != nil {
			return []Item{{Key: "sites", Group: GroupSites, Title: "站点", Status: StatusFail, Detail: "读取站点失败：" + short(err.Error())}}
		}
		if len(sites) == 0 {
			return []Item{{Key: "sites", Group: GroupSites, Title: "站点", Status: StatusSkip, Detail: "没有启用的站点"}}
		}
		var states []models.SiteLoginState
		if err := db.WithContext(ctx).Find(&states).Error; err != nil {
			return []Item{{Key: "sites", Group: GroupSites, Title: "站点", Status: StatusFail, Detail: "读取登录状态失败：" + short(err.Error())}}
		}
		byName := make(map[string]models.SiteLoginState, len(states))
		for _, st := range states {
			byName[strings.ToLower(st.SiteName)] = st
		}
		items := make([]Item, 0, len(sites))
		for _, s := range sites {
			items = append(items, siteItem(s, byName[strings.ToLower(s.Name)]))
		}
		return items
	}}
}

func siteItem(s models.SiteSetting, st models.SiteLoginState) Item {
	it := Item{Key: "site:" + strings.ToLower(s.Name), Group: GroupSites, Title: SiteName(s)}
	switch {
	case st.ProbeMode == probeModeDisabled:
		it.Status, it.Detail = StatusSkip, "登录探测关着"
		return it
	case st.LastProbeAt == nil || st.LastProbeStatus == "":
		it.Status, it.Detail = StatusSkip, "没探测过登录状态"
		return it
	}
	when := stamp(*st.LastProbeAt)
	switch st.LastProbeStatus {
	case "OK":
		it.Status, it.Detail = StatusOK, "最近一次探测（"+when+"）正常"
	case "NOT_APPLICABLE", "UNSUPPORTED":
		it.Status, it.Detail = StatusSkip, probeText[st.LastProbeStatus]
	default:
		it.Status, it.Detail = StatusWarn, fmt.Sprintf("最近一次探测（%s）%s", when, ProbeLabel(st.LastProbeStatus))
		if e := short(st.LastProbeError); e != "" {
			it.Detail += "：" + e
		}
		it.Hint = probeHint[st.LastProbeStatus]
		if it.Hint == "" {
			it.Hint = probeHint["UNKNOWN"]
		}
	}
	return it
}

// SiteName 是站点给人看的名字：站点设置里的显示名，其次站点定义里的名字，都没有时用站点编号。
func SiteName(s models.SiteSetting) string {
	if n := strings.TrimSpace(s.DisplayName); n != "" {
		return n
	}
	if def, ok := v2.GetDefinitionRegistry().Get(s.Name); ok && def.Name != "" {
		return def.Name
	}
	return s.Name
}

// DownloaderClient 是体检用到的下载器方法（thirdpart/downloader.Downloader 都有）。
type DownloaderClient interface {
	GetClientVersion() (string, error)
	GetClientFreeSpace(ctx context.Context) (int64, error)
}

// DownloaderTarget 是一台启用的下载器：Get 取它的实例（连不上时报错），几台同时取。
type DownloaderTarget struct {
	ID   uint
	Name string
	Get  func(ctx context.Context) (DownloaderClient, error)
}

// Downloaders 检查每台启用的下载器：连不连得上、版本、剩余空间（不到 20 GB 提醒）。几台同时探测，每台最多等 20 秒。
func Downloaders(list func(ctx context.Context) ([]DownloaderTarget, error)) Check {
	return Check{Group: GroupDownloaders, Title: "下载器", Run: func(ctx context.Context) []Item {
		targets, err := list(ctx)
		if err != nil {
			return []Item{{Key: "downloaders", Group: GroupDownloaders, Title: "下载器", Status: StatusFail, Detail: "读取下载器失败：" + short(err.Error())}}
		}
		if len(targets) == 0 {
			return []Item{{
				Key: "downloaders", Group: GroupDownloaders, Title: "下载器", Status: StatusWarn,
				Detail: "没有启用的下载器：RSS、订阅与搜索都推送不了", Hint: "在「下载器设置」里添加并启用一台",
			}}
		}
		return each(ctx, targets, downloaderItem)
	}}
}

func downloaderItem(ctx context.Context, t DownloaderTarget) Item {
	it := Item{Key: fmt.Sprintf("downloader:%d", t.ID), Group: GroupDownloaders, Title: t.Name}
	unreachable := func(err error) Item {
		it.Status, it.Detail = StatusFail, "连不上："+short(err.Error())
		it.Hint = "检查下载器是不是开着，以及地址、用户名与密码"
		return it
	}
	if t.Get == nil {
		return unreachable(errors.New("取不到下载器"))
	}
	c, err := within(ctx, probeTimeout, t.Get)
	if err == nil && c == nil {
		err = errors.New("取不到下载器")
	}
	if err != nil {
		return unreachable(err)
	}
	version, err := within(ctx, probeTimeout, func(context.Context) (string, error) { return c.GetClientVersion() })
	if err != nil {
		return unreachable(err)
	}
	free, err := within(ctx, probeTimeout, c.GetClientFreeSpace)
	if err != nil {
		it.Status, it.Detail = StatusWarn, joinDetail(version, "读不到剩余空间："+short(err.Error()))
		it.Hint = "下载器没给剩余空间：开着磁盘保护时，推送前的空间检查会拒绝推送"
		return it
	}
	free = max(free, 0)
	it.Detail = joinDetail(version, "剩余 "+utils.FormatBytes(free))
	if fsutil.LowSpace(uint64(free), 0) {
		it.Status, it.Hint = StatusWarn, "下载盘快满了：清理做种的种子或换一块更大的盘；开着磁盘保护时，空间不够就不再推送"
		return it
	}
	it.Status = StatusOK
	return it
}

// joinDetail 用「 · 」连起说明里不为空的几段。
func joinDetail(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// within 跑 f，最多等 d：到时还没回来就不等了（f 自己的请求有各自的超时）。
func within[T any](ctx context.Context, d time.Duration, f func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	type result struct {
		v   T
		err error
	}
	ch := make(chan result, 1)
	go func() {
		v, err := f(ctx)
		ch <- result{v, err}
	}()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, fmt.Errorf("%d 秒内没有回应", int(d/time.Second))
	}
}

// each 同时（最多 4 个）对每一项跑 f，结果的顺序和 in 一样。
func each[T any](ctx context.Context, in []T, f func(context.Context, T) Item) []Item {
	out := make([]Item, len(in))
	sem := make(chan struct{}, probeParallel)
	var wg sync.WaitGroup
	for i, v := range in {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = f(ctx, v)
		})
	}
	wg.Wait()
	return out
}

// ServerInfo 是测试连接媒体服务器得到的：服务器种类给的版本。
type ServerInfo struct {
	Version string
}

// Servers 测试每台启用的媒体服务器能不能连上（用保存的地址与 API Key），再看上次同步索引有没有出错。
func Servers(db *gorm.DB, test func(ctx context.Context, srv models.MediaServer) (ServerInfo, error)) Check {
	return Check{Group: GroupServers, Title: "媒体服务器", Run: func(ctx context.Context) []Item {
		var rows []models.MediaServer
		if err := db.WithContext(ctx).Where("enabled = ?", true).Order("id").Find(&rows).Error; err != nil {
			return []Item{{Key: "servers", Group: GroupServers, Title: "媒体服务器", Status: StatusFail, Detail: "读取媒体服务器失败：" + short(err.Error())}}
		}
		if len(rows) == 0 {
			return []Item{{Key: "servers", Group: GroupServers, Title: "媒体服务器", Status: StatusSkip, Detail: "没有连媒体服务器"}}
		}
		return each(ctx, rows, func(ctx context.Context, srv models.MediaServer) Item {
			return serverItem(ctx, srv, test)
		})
	}}
}

// serverKinds 是媒体服务器种类给人看的名字。
var serverKinds = map[string]string{models.MediaServerEmby: "Emby", models.MediaServerJellyfin: "Jellyfin", models.MediaServerPlex: "Plex"}

func serverItem(ctx context.Context, srv models.MediaServer, test func(ctx context.Context, srv models.MediaServer) (ServerInfo, error)) Item {
	it := Item{Key: fmt.Sprintf("server:%d", srv.ID), Group: GroupServers, Title: srv.Name}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	info, err := test(ctx, srv)
	if err != nil {
		it.Status, it.Detail = StatusFail, "连不上："+short(err.Error())
		it.Hint = "在「媒体库 → 媒体服务器」里检查地址与 API Key"
		return it
	}
	kind := serverKinds[srv.Kind]
	if kind == "" {
		kind = srv.Kind
	}
	it.Detail = strings.TrimSpace(kind + " " + info.Version)
	switch {
	case srv.IndexOff:
	case srv.IndexError != "":
		it.Status = StatusWarn
		it.Detail = joinDetail(it.Detail, "上次同步索引失败："+short(srv.IndexError))
		it.Hint = "在「媒体库 → 媒体服务器」里点「同步」看原因"
		return it
	case srv.IndexedAt != nil:
		it.Detail = joinDetail(it.Detail, fmt.Sprintf("索引 %d 部电影、%d 部剧，%s 同步", srv.IndexMovies, srv.IndexSeries, stamp(*srv.IndexedAt)))
	}
	it.Status = StatusOK
	return it
}

// PathMaps 检查每条路径映射在 pt-tools 里看到的目录：在不在、能不能读。
func PathMaps(db *gorm.DB) Check {
	return Check{Group: GroupPaths, Title: "路径映射", Run: func(ctx context.Context) []Item {
		var maps []models.MediaPathMap
		if err := db.WithContext(ctx).Order("downloader_id, id").Find(&maps).Error; err != nil {
			return []Item{{Key: "paths", Group: GroupPaths, Title: "路径映射", Status: StatusFail, Detail: "读取路径映射失败：" + short(err.Error())}}
		}
		if len(maps) == 0 {
			return []Item{{Key: "paths", Group: GroupPaths, Title: "路径映射", Status: StatusSkip, Detail: "没有路径映射（下载器和 pt-tools 看到的路径相同时不用配）"}}
		}
		names := downloaderNames(ctx, db)
		items := make([]Item, 0, len(maps))
		for _, m := range maps {
			name := names[m.DownloaderID]
			if name == "" {
				name = fmt.Sprintf("#%d", m.DownloaderID)
			}
			it := Item{Key: fmt.Sprintf("path:%d", m.ID), Group: GroupPaths, Title: "下载器「" + name + "」的路径映射", Detail: m.DownloaderPrefix + " → " + m.LocalPrefix}
			if msg := readableDir(m.LocalPrefix); msg != "" {
				it.Status, it.Detail = StatusFail, it.Detail+"："+msg
				it.Hint = "Docker 里要把下载目录挂进容器，挂载的位置和这里填的 pt-tools 路径一样；没有读权限时给 pt-tools 加上"
			} else {
				it.Status = StatusOK
			}
			items = append(items, it)
		}
		return items
	}}
}

// downloaderNames 是下载器编号到名字（读不到时为空的表）。
func downloaderNames(ctx context.Context, db *gorm.DB) map[uint]string {
	var rows []models.DownloaderSetting
	out := map[uint]string{}
	if err := db.WithContext(ctx).Select("id", "name").Find(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		out[r.ID] = r.Name
	}
	return out
}

// readableDir 报告目录有什么问题（不在、不是目录、读不了）；没问题时为空。
func readableDir(dir string) string {
	info, err := os.Stat(dir)
	switch {
	case err != nil:
		return "pt-tools 里找不到这个目录"
	case !info.IsDir():
		return "不是目录"
	}
	f, err := os.Open(dir)
	if err == nil {
		_, err = f.Readdirnames(1)
		_ = f.Close()
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "读不了：" + short(err.Error())
	}
	return ""
}

// Probe 是检查媒体库目录的一步（organize.CheckItem）。
type Probe struct {
	Name    string
	OK      bool
	Message string
}

// SavePath 是一台下载器的默认保存路径（已按路径映射换成 pt-tools 里看到的路径）。
type SavePath struct {
	Downloader string
	Path       string
}

// Libraries 检查每个媒体库的目录（在不在、能不能写，硬链接库试建一个硬链接，见 organize.CheckLibrary）；
// 硬链接库另外比一比各下载器的默认保存路径是不是和库在同一个文件系统。
func Libraries(db *gorm.DB, check func(ctx context.Context, path, mode string) ([]Probe, error), savePaths func(ctx context.Context) []SavePath) Check {
	return Check{Group: GroupLibraries, Title: "媒体库", Run: func(ctx context.Context) []Item {
		var libs []models.MediaLibrary
		if err := db.WithContext(ctx).Order("id").Find(&libs).Error; err != nil {
			return []Item{{Key: "libraries", Group: GroupLibraries, Title: "媒体库", Status: StatusFail, Detail: "读取媒体库失败：" + short(err.Error())}}
		}
		if len(libs) == 0 {
			return []Item{{Key: "libraries", Group: GroupLibraries, Title: "媒体库", Status: StatusSkip, Detail: "没有媒体库"}}
		}
		var saves []SavePath
		var loaded bool
		items := make([]Item, 0, len(libs))
		for _, lib := range libs {
			it := Item{Key: fmt.Sprintf("library:%d", lib.ID), Group: GroupLibraries, Title: "媒体库「" + lib.Name + "」", Detail: lib.Path}
			probes, err := check(ctx, lib.Path, lib.Mode)
			if err != nil {
				it.Status, it.Detail, it.Hint = StatusFail, lib.Path+"："+short(err.Error()), "在「媒体库」里改一下这个库的目录"
				items = append(items, it)
				continue
			}
			if p, ok := failedProbe(probes); ok {
				it.Status, it.Detail, it.Hint = StatusFail, lib.Path+"："+p.Name+"失败："+short(p.Message), libraryHint(p.Name)
				items = append(items, it)
				continue
			}
			it.Status = StatusOK
			if lib.Mode == models.MediaModeHardlink {
				if !loaded {
					saves, loaded = savePaths(ctx), true
				}
				if other := otherDevice(lib.Path, saves); len(other) > 0 {
					it.Status = StatusWarn
					it.Detail = lib.Path + "：和" + strings.Join(other, "、") + " 不在同一个文件系统"
					it.Hint = "存在那里的种子硬链接不进这个库：把下载目录和库放在同一块盘、同一个挂载里，或者把这个库的整理方式改成复制"
				}
			}
			items = append(items, it)
		}
		return items
	}}
}

func failedProbe(probes []Probe) (Probe, bool) {
	for _, p := range probes {
		if !p.OK {
			return p, true
		}
	}
	return Probe{}, false
}

// libraryHint 是检查库目录的一步失败时怎么修。
func libraryHint(step string) string {
	switch {
	case strings.HasPrefix(step, "硬链接"):
		return "硬链接要求下载目录和库在同一个文件系统（同一块盘、同一个挂载）：放到一起，或者把这个库的整理方式改成复制"
	case step == "库目录可写":
		return "给 pt-tools 写这个目录的权限（Docker 里看挂载与运行用户）"
	default:
		return "Docker 里要把媒体库挂进容器，挂载的位置和这里填的一样"
	}
}

// otherDevice 列出和库不在同一个文件系统的默认保存路径（写成「下载器「qb」的 /downloads」）；读不到的路径不算。
func otherDevice(lib string, saves []SavePath) []string {
	u, err := diskUsage(lib)
	if err != nil || u.Device == "" {
		return nil
	}
	var out []string
	for _, s := range saves {
		d, err := diskUsage(s.Path)
		if err != nil || d.Device == "" || d.Device == u.Device {
			continue
		}
		out = append(out, "下载器「"+s.Downloader+"」的 "+s.Path)
	}
	return out
}

// DiskPath 是要看剩余空间的一个目录（Label 是它是什么：数据目录、媒体库「电影」…）。
type DiskPath struct {
	Label string
	Path  string
}

// Disks 看这些目录所在的盘还剩多少：剩不到 5% 或 20 GB 时提醒。同一块盘（同一个设备）只列一次，读不到的目录跳过
// （目录不在由路径映射与媒体库两项报告）。
func Disks(paths func(ctx context.Context) []DiskPath) Check {
	return Check{Group: GroupDisks, Title: "磁盘空间", Run: func(ctx context.Context) []Item {
		var items []Item
		seen := map[string]int{}
		// labels 是每一行已经写了的名字：同一个名字的两个目录（下载器的保存路径与它的路径映射）只写一次
		labels := map[int]map[string]bool{}
		for _, p := range paths(ctx) {
			if strings.TrimSpace(p.Path) == "" {
				continue
			}
			u, err := diskUsage(p.Path)
			if err != nil {
				continue
			}
			if i, ok := seen[u.Device]; ok && u.Device != "" {
				if !labels[i][p.Label] {
					labels[i][p.Label] = true
					items[i].Title += "、" + p.Label
				}
				continue
			}
			it := Item{Key: "disk:" + p.Path, Group: GroupDisks, Title: p.Label, Detail: p.Path + "：" + usageText(u)}
			if fsutil.LowSpace(u.Free, u.Total) {
				it.Status, it.Hint = StatusWarn, "清理一下这块盘：数据目录满了 pt-tools 写不了数据库，媒体库的盘满了整理会失败，开着磁盘保护时下载盘满了不再推送"
			} else {
				it.Status = StatusOK
			}
			seen[u.Device] = len(items)
			labels[len(items)] = map[string]bool{p.Label: true}
			items = append(items, it)
		}
		if len(items) == 0 {
			return []Item{{Key: "disks", Group: GroupDisks, Title: "磁盘空间", Status: StatusSkip, Detail: "读不到磁盘的剩余空间"}}
		}
		return items
	}}
}

// usageText 是一块盘的用量说法：剩余多少，共多少。
func usageText(u fsutil.Usage) string {
	s := "剩余 " + sizeText(u.Free)
	if u.Total > 0 {
		s += "，共 " + sizeText(u.Total)
	}
	return s
}

// sizeText 同 utils.FormatBytes（大到 int64 放不下时按 int64 的最大值写）。
func sizeText(n uint64) string {
	if n > math.MaxInt64 {
		n = math.MaxInt64
	}
	return utils.FormatBytes(int64(n))
}

// BackupState 是备份的设置与最近一次的结果（core.ConfigStore.BackupSettings 给的）。
type BackupState struct {
	Enabled   bool
	LastAt    *time.Time
	LastName  string
	LastError string
	LastOKAt  *time.Time
}

// BackupLate 报告开着定时备份时备份是不是该提醒了：最近一次失败，或者最近一次成功距今超过两天。
// days 是最近一次成功距今几天（从没成功过时为 -1；旧版本没记成功时间时，最近一次没出错就当它是成功的那次）。
func BackupLate(st BackupState, now time.Time) (days int, late bool) {
	ok := st.LastOKAt
	if ok == nil && st.LastAt != nil && st.LastError == "" {
		ok = st.LastAt
	}
	days = -1
	if ok != nil {
		days = int(now.Sub(*ok) / (24 * time.Hour))
	}
	if !st.Enabled || st.LastAt == nil {
		return days, false
	}
	if st.LastError != "" {
		return days, true
	}
	return days, now.Sub(*ok) > backupStale
}

// Backup 看备份：开着定时备份时，最近一次失败或者两天没成功过要提醒。
func Backup(get func() (BackupState, error), now func() time.Time) Check {
	return Check{Group: GroupBackup, Title: "备份", Run: func(context.Context) []Item {
		it := Item{Key: "backup", Group: GroupBackup, Title: "备份"}
		st, err := get()
		switch {
		case err != nil:
			it.Status, it.Detail = StatusFail, "读取备份设置失败："+short(err.Error())
			return []Item{it}
		case !st.Enabled:
			it.Status, it.Detail, it.Hint = StatusSkip, "每天定时备份关着", "在「全局设置 → 备份」里打开每天定时备份"
			return []Item{it}
		case st.LastAt == nil:
			it.Status, it.Detail = StatusSkip, "没做过备份"
			return []Item{it}
		}
		days, late := BackupLate(st, now())
		switch {
		case st.LastError != "":
			it.Status = StatusWarn
			it.Detail = fmt.Sprintf("最近一次备份（%s）失败：%s", stamp(*st.LastAt), short(st.LastError))
			it.Hint = "在「全局设置 → 备份与恢复」里看原因，改好以后点「立即备份」"
		case late:
			it.Status = StatusWarn
			it.Detail = fmt.Sprintf("%d 天没备份成功了", days)
			it.Hint = "看 pt-tools 在定时备份的时刻是不是开着，或者在「全局设置 → 备份与恢复」里点「立即备份」"
		default:
			it.Status = StatusOK
			at := st.LastAt
			if st.LastOKAt != nil {
				at = st.LastOKAt
			}
			it.Detail = "最近一次 " + stamp(*at) + " 成功"
			if st.LastName != "" {
				it.Detail += "（" + st.LastName + "）"
			}
		}
		return []Item{it}
	}}
}

// Secret 看加密密钥：来自环境变量还是 secret.key，密钥文件读不读得出、还在不在。
// status 是 crypto.KeyStatus：path 为空表示用的是环境变量 PT_TOOLS_SECRET_KEY。
func Secret(status func() (path string, generated bool, err error)) Check {
	return Check{Group: GroupSecret, Title: "密钥", Run: func(context.Context) []Item {
		it := Item{Key: "secret", Group: GroupSecret, Title: "加密密钥"}
		path, generated, err := status()
		switch {
		case err != nil:
			it.Status, it.Detail = StatusFail, path+" 读不出来："+short(err.Error())
			it.Hint = "换回原来的 secret.key，或用 pt-tools secret import 导入备份的密钥；不然存着的 Cookie 与通知凭证都解不开"
		case path == "":
			it.Status, it.Detail = StatusOK, "用的是环境变量 PT_TOOLS_SECRET_KEY"
		default:
			if _, statErr := os.Stat(path); statErr != nil {
				it.Status, it.Detail = StatusFail, path+" 不见了：现在的密钥只在内存里，重启以后存着的 Cookie 与通知凭证都解不开"
				it.Hint = "在「全局设置」的「备份」里打开「带上密钥」并保存，再在「备份与恢复」里点「立即备份」，把备份里的 secret.key 放回数据目录"
				break
			}
			it.Status, it.Detail = StatusOK, "密钥在 "+path
			if generated {
				it.Detail += "（这次启动时新生成的）"
			}
		}
		return []Item{it}
	}}
}
