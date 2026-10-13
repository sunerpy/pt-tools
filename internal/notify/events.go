package notify

import (
	"time"
)

// 事件类型。生产者用这些常量经 NewEvent 建事件；消息模板按它们一种一套。
const (
	EventRSSNew              = "rss.new"
	EventRSSMatched          = "rss.matched"
	EventRSSDigest           = "rss.digest"
	EventSubscribeDownloaded = "subscribe.downloaded"
	EventDoubanFailed        = "subscribe.douban_failed"
	EventMediaOrganized      = "media.organized"
	EventLoginExpiring       = "site.login_expiring"
	EventProbeFailed         = "site.probe_failed"
	EventProbeRecovered      = "site.probe_recovered"
	EventLoginTest           = "site.login_test"
	EventAttendanceDaily     = "attendance.daily"
	EventReportDaily         = "report.daily"
	EventDeadTorrents        = "downloader.dead_torrents"
	EventRemotePaired        = "remote.paired"
	EventTest                = "test"
)

// VarType 是变量的种类。数字分整数与浮点数：规范化按声明转成 int64 或 float64，不看字面上有没有小数点。
type VarType string

const (
	VarString VarType = "string"
	VarInt    VarType = "int"
	VarFloat  VarType = "float"
	VarBool   VarType = "bool"
	VarList   VarType = "list"
)

// Var 是一个模板变量。列表的每一项是对象，字段在 Item 里。
type Var struct {
	Key    string  `json:"key"`
	Type   VarType `json:"type"`
	Desc   string  `json:"desc"`
	Sample any     `json:"sample,omitempty"`
	Item   []Var   `json:"item,omitempty"`
}

// EventSpec 是一种事件：名称、说明、专有变量，以及界面上展示的内置标题与正文的示例（按 Sample 生成的样子）。
type EventSpec struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Desc        string `json:"desc"`
	Vars        []Var  `json:"vars"`
	SampleTitle string `json:"sample_title"`
	SampleText  string `json:"sample_text"`
}

// CommonVars 是每种事件都有的变量。
var CommonVars = []Var{
	{Key: "event", Type: VarString, Desc: "事件类型", Sample: EventRSSMatched},
	{Key: "event_name", Type: VarString, Desc: "事件名称", Sample: "RSS 命中过滤规则"},
	{Key: "time", Type: VarString, Desc: "发生时间（本地时间，2006-01-02 15:04:05）", Sample: "2026-10-12 08:00:00"},
	{Key: "timestamp", Type: VarInt, Desc: "发生时间（秒级时间戳）", Sample: int64(1791763200)},
	{Key: "title", Type: VarString, Desc: "内置的标题"},
	{Key: "text", Type: VarString, Desc: "内置的正文"},
}

// WebhookVars 是只有 Webhook 请求体与地址里才有的变量。
var WebhookVars = []Var{
	{Key: "channel", Type: VarString, Desc: "通知通道的名称", Sample: "我的 Webhook"},
}

func more(desc string) Var {
	return Var{Key: "items_more", Type: VarInt, Desc: desc + "（列表最多 100 项）", Sample: int64(0)}
}

// Events 是事件目录。改这里要同时改对应生产者（events_test 核对两边的变量一致）。
var Events = []EventSpec{
	{
		Type: EventRSSNew, Name: "RSS 新种子", Desc: "RSS 拉到新种子（通知方式选了「全部」）",
		Vars: []Var{
			{Key: "site", Type: VarString, Desc: "站点", Sample: "hdtime"},
			{Key: "rss", Type: VarString, Desc: "RSS 名称", Sample: "HDTime 电影"},
			{Key: "torrent_title", Type: VarString, Desc: "种子标题", Sample: "Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE"},
			{Key: "torrent_id", Type: VarString, Desc: "种子编号", Sample: "300101"},
			{Key: "link", Type: VarString, Desc: "RSS 里这一条的链接", Sample: "https://hdtime.org/details.php?id=300101"},
			{Key: "published", Type: VarString, Desc: "发布时间", Sample: "2026-10-12 08:00"},
		},
		SampleTitle: "Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE",
		SampleText:  "🆕 [hdtime] Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE\n\n📅 2026-10-12 08:00\n🔗 https://hdtime.org/details.php?id=300101",
	},
	{
		Type: EventRSSMatched, Name: "RSS 命中过滤规则", Desc: "RSS 的种子命中了过滤规则（通知方式选了「命中规则」）",
		Vars: []Var{
			{Key: "site", Type: VarString, Desc: "站点", Sample: "hdtime"},
			{Key: "rss", Type: VarString, Desc: "RSS 名称", Sample: "HDTime 电影"},
			{Key: "torrent_title", Type: VarString, Desc: "种子标题", Sample: "Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE"},
			{Key: "torrent_id", Type: VarString, Desc: "种子编号", Sample: "300101"},
			{Key: "subtitle", Type: VarString, Desc: "副标题", Sample: "假电影 | 中字"},
			{Key: "size", Type: VarString, Desc: "大小", Sample: "15.00 GB"},
			{Key: "size_bytes", Type: VarInt, Desc: "大小（字节）", Sample: int64(16106127360)},
			{Key: "free", Type: VarBool, Desc: "是不是免费", Sample: true},
			{Key: "free_end", Type: VarString, Desc: "免费到什么时候（本地时间；没有就是空）", Sample: "2026-10-13 08:00:00"},
			{Key: "free_remaining", Type: VarString, Desc: "免费还剩多久", Sample: "23h59min"},
			{Key: "rule", Type: VarString, Desc: "命中的过滤规则", Sample: "只要免费"},
			{Key: "link", Type: VarString, Desc: "种子详情页", Sample: "https://hdtime.org/details.php?id=300101"},
			{Key: "seeders", Type: VarInt, Desc: "做种数", Sample: int64(12)},
			{Key: "leechers", Type: VarInt, Desc: "下载数", Sample: int64(3)},
			{Key: "has_hr", Type: VarBool, Desc: "有没有 H&R", Sample: false},
		},
		SampleTitle: "Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE",
		SampleText:  "🎯 [hdtime] Fake.Movie.2026.1080p.WEB-DL.H.264-FAKE\n\n📦 15.00 GB\n🆓 免费 (剩余 23h59min)\n📌 匹配规则：只要免费\n🔗 https://hdtime.org/details.php?id=300101",
	},
	{
		Type: EventRSSDigest, Name: "RSS 合并通知", Desc: "短时间里来了多条 RSS 通知，合成一条发",
		Vars: []Var{
			{Key: "count", Type: VarInt, Desc: "一共几条", Sample: int64(2)},
			{Key: "items", Type: VarList, Desc: "各条通知", Item: []Var{
				{Key: "title", Type: VarString, Desc: "这一条的内置标题"},
				{Key: "site", Type: VarString, Desc: "站点"},
				{Key: "torrent_title", Type: VarString, Desc: "种子标题"},
				{Key: "torrent_id", Type: VarString, Desc: "种子编号"},
			}, Sample: []map[string]any{
				{"title": "Fake.Movie.A.2026.1080p", "site": "hdtime", "torrent_title": "Fake.Movie.A.2026.1080p", "torrent_id": "300101"},
				{"title": "Fake.Movie.B.2026.2160p", "site": "hdtime", "torrent_title": "Fake.Movie.B.2026.2160p", "torrent_id": "300102"},
			}},
			more("省略的条数"),
		},
		SampleTitle: "RSS 摘要 · 2 条",
		SampleText:  "📦 2 条新通知\n\n1. Fake.Movie.A.2026.1080p\n2. Fake.Movie.B.2026.2160p\n",
	},
	{
		Type: EventSubscribeDownloaded, Name: "订阅下载", Desc: "订阅找到了资源并推送到下载器",
		Vars: []Var{
			{Key: "subscription", Type: VarString, Desc: "订阅（带年份与季）", Sample: "沙丘 (2021)"},
			{Key: "media_type", Type: VarString, Desc: "movie 或 tv", Sample: "movie"},
			{Key: "year", Type: VarInt, Desc: "年份", Sample: int64(2021)},
			{Key: "season", Type: VarInt, Desc: "季（电影是 0）", Sample: int64(0)},
			{Key: "episodes", Type: VarString, Desc: "这次下的集数（整季、第 3–5 集；电影是空）", Sample: ""},
			{Key: "torrent_title", Type: VarString, Desc: "种子标题", Sample: "Dune.2021.2160p.BluRay.x265-FAKE"},
			{Key: "site", Type: VarString, Desc: "站点", Sample: "hdtime"},
			{Key: "quality", Type: VarString, Desc: "质量（分辨率、来源、编码）", Sample: "2160p BluRay x265"},
			{Key: "poster", Type: VarString, Desc: "海报地址", Sample: "https://image.tmdb.org/t/p/w500/d5NXSklXo0qyIYkgV94XAgMIckC.jpg"},
		},
		SampleTitle: "订阅下载：沙丘 (2021)",
		SampleText:  "种子：Dune.2021.2160p.BluRay.x265-FAKE\n站点：hdtime\n质量：2160p BluRay x265\nhttps://image.tmdb.org/t/p/w500/d5NXSklXo0qyIYkgV94XAgMIckC.jpg",
	},
	{
		Type: EventDoubanFailed, Name: "豆瓣想看拉取失败", Desc: "豆瓣想看连续几次没拉到",
		Vars: []Var{
			{Key: "source", Type: VarString, Desc: "豆瓣用户", Sample: "我的豆瓣"},
			{Key: "fails", Type: VarInt, Desc: "连续失败次数", Sample: int64(3)},
			{Key: "error", Type: VarString, Desc: "最近一次的原因", Sample: "豆瓣返回 403"},
		},
		SampleTitle: "豆瓣想看拉取失败",
		SampleText:  "我的豆瓣：连续 3 次没拉到（豆瓣返回 403），之后按退避继续重试",
	},
	{
		Type: EventMediaOrganized, Name: "入库", Desc: "下载完的电影或剧集整理进了媒体库",
		Vars: []Var{
			{Key: "name", Type: VarString, Desc: "片名（带年份）", Sample: "沙丘 (2021)"},
			{Key: "media_type", Type: VarString, Desc: "movie 或 tv", Sample: "movie"},
			{Key: "episodes", Type: VarString, Desc: "入库的集（剧集才有）", Sample: ""},
			{Key: "files", Type: VarInt, Desc: "入库的文件数", Sample: int64(1)},
			{Key: "quality", Type: VarString, Desc: "画质", Sample: "2160p BluRay"},
			{Key: "library", Type: VarString, Desc: "媒体库", Sample: "电影"},
			{Key: "torrent", Type: VarString, Desc: "种子名", Sample: "Dune.2021.2160p.BluRay.x265-FAKE"},
			{Key: "poster", Type: VarString, Desc: "海报地址", Sample: "https://image.tmdb.org/t/p/w500/d5NXSklXo0qyIYkgV94XAgMIckC.jpg"},
		},
		SampleTitle: "入库：沙丘 (2021)",
		SampleText:  "电影\n画质：2160p BluRay\n媒体库：电影\n种子：Dune.2021.2160p.BluRay.x265-FAKE\n海报：https://image.tmdb.org/t/p/w500/d5NXSklXo0qyIYkgV94XAgMIckC.jpg",
	},
	{
		Type: EventLoginExpiring, Name: "站点即将被封号", Desc: "站点很久没有活跃，快到封号期限",
		Vars: []Var{
			{Key: "site", Type: VarString, Desc: "站点", Sample: "hdtime"},
			{Key: "tier", Type: VarString, Desc: "提醒档位", Sample: "d7"},
			{Key: "days_remaining", Type: VarInt, Desc: "还剩几天", Sample: int64(7)},
			{Key: "last_active", Type: VarString, Desc: "上次活跃（本地时间）", Sample: "2026-08-20 10:00:00"},
			{Key: "days_since_active", Type: VarInt, Desc: "距上次活跃几天", Sample: int64(53)},
			{Key: "access_stale", Type: VarBool, Desc: "自动访问刷新不了活跃时间、要手动登录", Sample: false},
		},
		SampleTitle: "[pt-tools] 站点 hdtime 即将被封号",
		SampleText:  "上次活跃 2026-08-20T02:00:00Z（53 天前，依据：站点用户信息）；建议访问站点并浏览页面以刷新活跃时间（多数站点仅靠登录不更新 last_access）。剩余 7 天 (tier=d7)。",
	},
	{
		Type: EventProbeFailed, Name: "站点探测失败", Desc: "站点的登录状态探测持续失败",
		Vars: []Var{
			{Key: "site", Type: VarString, Desc: "站点", Sample: "hdtime"},
			{Key: "status", Type: VarString, Desc: "状态", Sample: "cookie_expired"},
			{Key: "status_label", Type: VarString, Desc: "状态说明", Sample: "Cookie 失效"},
			{Key: "since", Type: VarString, Desc: "从什么时候开始失败（本地时间）", Sample: "2026-10-12 06:00:00"},
			{Key: "advice", Type: VarString, Desc: "建议", Sample: "请重新同步 Cookie，或检查 API Key / Passkey 是否仍然有效。"},
			{Key: "diagnosis", Type: VarString, Desc: "诊断信息（可能是空）", Sample: "跳转到了登录页"},
		},
		SampleTitle: "[pt-tools] 站点 hdtime 探测失败",
		SampleText:  "状态：Cookie 失效（cookie_expired），自 2026-10-11T22:00:00Z 起持续失败。请重新同步 Cookie，或检查 API Key / Passkey 是否仍然有效。\n诊断：跳转到了登录页",
	},
	{
		Type: EventProbeRecovered, Name: "站点探测恢复", Desc: "探测失败以后又成功了",
		Vars: []Var{
			{Key: "site", Type: VarString, Desc: "站点", Sample: "hdtime"},
			{Key: "probed_at", Type: VarString, Desc: "最近一次成功的探测（本地时间）", Sample: "2026-10-12 09:00:00"},
		},
		SampleTitle: "[pt-tools] 站点 hdtime 探测已恢复",
		SampleText:  "最近一次探测成功（2026-10-12T01:00:00Z），登录状态已恢复正常。",
	},
	{
		Type: EventLoginTest, Name: "登录提醒测试", Desc: "在站点的登录状态里点了「测试提醒」",
		Vars: []Var{
			{Key: "site", Type: VarString, Desc: "站点", Sample: "hdtime"},
			{Key: "tier", Type: VarString, Desc: "当前提醒档位", Sample: "ok"},
			{Key: "days_remaining", Type: VarInt, Desc: "还剩几天", Sample: int64(60)},
			{Key: "last_active", Type: VarString, Desc: "当前判定的活跃时间（本地时间；不知道就是空）", Sample: "2026-10-12 08:00:00"},
		},
		SampleTitle: "[pt-tools][测试] 站点 hdtime 登录提醒",
		SampleText:  "这是一条测试提醒。当前判定活跃 2026-10-12T00:00:00Z，剩余 60 天 (tier=ok)。若你收到此消息，说明该站点的通知通道配置正常。",
	},
	{
		Type: EventAttendanceDaily, Name: "签到结果", Desc: "当天开启签到的站点都有结果以后的汇总",
		Vars: []Var{
			{Key: "day", Type: VarString, Desc: "日期", Sample: "2026-10-12"},
			{Key: "signed", Type: VarInt, Desc: "签到成功或已签到的站点数", Sample: int64(1)},
			{Key: "failed", Type: VarInt, Desc: "签到失败的站点数", Sample: int64(1)},
			{Key: "items", Type: VarList, Desc: "各站点", Item: []Var{
				{Key: "site", Type: VarString, Desc: "站点"},
				{Key: "status", Type: VarString, Desc: "状态（signed、already、failed、unsupported、pending）"},
				{Key: "status_label", Type: VarString, Desc: "状态说明"},
				{Key: "message", Type: VarString, Desc: "站点返回的说明或失败原因"},
			}, Sample: []map[string]any{
				{"site": "hdtime", "status": "signed", "status_label": "签到成功", "message": "获得 10 魔力"},
				{"site": "hdsky", "status": "failed", "status_label": "签到失败", "message": "站点超时"},
			}},
			more("省略的站点数"),
		},
		SampleTitle: "[pt-tools] 2026-10-12 签到结果",
		SampleText:  "· hdtime：签到成功（获得 10 魔力）\n· hdsky：签到失败（站点超时）",
	},
	{
		Type: EventReportDaily, Name: "每日战报", Desc: "每天定时发的上传、下载与魔力统计（登录异常、签到、刷流几段只在 text 里）",
		Vars: []Var{
			{Key: "day", Type: VarString, Desc: "日期", Sample: "2026-10-12"},
			{Key: "uploaded", Type: VarString, Desc: "今天合计上传", Sample: "120.00 GB"},
			{Key: "downloaded", Type: VarString, Desc: "今天合计下载", Sample: "30.00 GB"},
			{Key: "uploaded_bytes", Type: VarInt, Desc: "今天合计上传（字节）", Sample: int64(128849018880)},
			{Key: "downloaded_bytes", Type: VarInt, Desc: "今天合计下载（字节）", Sample: int64(32212254720)},
			{Key: "bonus", Type: VarFloat, Desc: "今天合计魔力", Sample: float64(1234.5)},
			{Key: "items", Type: VarList, Desc: "各站点（按上传从多到少）", Item: []Var{
				{Key: "site", Type: VarString, Desc: "站点"},
				{Key: "uploaded", Type: VarString, Desc: "上传"},
				{Key: "downloaded", Type: VarString, Desc: "下载"},
				{Key: "uploaded_bytes", Type: VarInt, Desc: "上传（字节）"},
				{Key: "downloaded_bytes", Type: VarInt, Desc: "下载（字节）"},
				{Key: "bonus", Type: VarFloat, Desc: "魔力"},
			}, Sample: []map[string]any{
				{"site": "HDTime", "uploaded": "100.00 GB", "downloaded": "20.00 GB", "uploaded_bytes": int64(107374182400), "downloaded_bytes": int64(21474836480), "bonus": float64(1000)},
				{"site": "HDSky", "uploaded": "20.00 GB", "downloaded": "10.00 GB", "uploaded_bytes": int64(21474836480), "downloaded_bytes": int64(10737418240), "bonus": float64(234.5)},
			}},
			more("省略的站点数"),
		},
		SampleTitle: "📊 每日战报 2026-10-12",
		SampleText:  "今日合计：上传 120.00 GB · 下载 30.00 GB · 魔力 1234.5\n· HDTime：上传 100.00 GB · 下载 20.00 GB · 魔力 1000\n· HDSky：上传 20.00 GB · 下载 10.00 GB · 魔力 234.5",
	},
	{
		Type: EventDeadTorrents, Name: "失效种子", Desc: "下载器助手扫到 tracker 报告未注册或不存在的种子",
		Vars: []Var{
			{Key: "downloader", Type: VarString, Desc: "下载器", Sample: "家里的 qB"},
			{Key: "count", Type: VarInt, Desc: "失效种子数", Sample: int64(2)},
			{Key: "size", Type: VarString, Desc: "合计大小", Sample: "30.0 GiB"},
			{Key: "size_bytes", Type: VarInt, Desc: "合计大小（字节）", Sample: int64(32212254720)},
			{Key: "items", Type: VarList, Desc: "各种子", Item: []Var{
				{Key: "name", Type: VarString, Desc: "种子名"},
				{Key: "size", Type: VarString, Desc: "大小"},
				{Key: "size_bytes", Type: VarInt, Desc: "大小（字节）"},
			}, Sample: []map[string]any{
				{"name": "Old.Movie.2019.1080p", "size": "20.0 GiB", "size_bytes": int64(21474836480)},
				{"name": "Old.Show.S01.720p", "size": "10.0 GiB", "size_bytes": int64(10737418240)},
			}},
			more("省略的种子数"),
		},
		SampleTitle: "下载器助手：发现失效种子",
		SampleText:  "家里的 qB 里有 2 个种子（共 30.0 GiB）的 tracker 报告未注册或不存在。到「下载器助手 → 失效种子」确认后再删除，这里不会自动删。\n· Old.Movie.2019.1080p\n· Old.Show.S01.720p",
	},
	{
		Type: EventRemotePaired, Name: "新设备配对", Desc: "手机 App 扫码配对成功",
		Vars: []Var{
			{Key: "device", Type: VarString, Desc: "设备名", Sample: "我的手机"},
			{Key: "permission", Type: VarString, Desc: "权限（完全控制或只读）", Sample: "完全控制"},
		},
		SampleTitle: "新设备已配对",
		SampleText:  "设备「我的手机」已经配对，权限：完全控制。如果不是你本人操作，请在「系统 → 远程访问」撤销这台设备。",
	},
	{
		Type: EventTest, Name: "通道测试", Desc: "在通知通道里点了「发送测试」",
		Vars: []Var{
			{Key: "channel", Type: VarString, Desc: "通知通道的名称", Sample: "我的 Telegram"},
		},
		SampleTitle: "pt-tools 测试通知",
		SampleText:  "如果你看到此消息，说明通道配置正常。",
	},
}

var (
	specByType = map[string]*EventSpec{}
	// listVars 是可以 range 的变量名（目录里声明为列表的）
	listVars = map[string]bool{}
	// zeroData 是所有事件的全部变量的零值
	zeroData = map[string]any{}
)

func init() {
	for i := range Events {
		spec := &Events[i]
		specByType[spec.Type] = spec
		for _, v := range spec.Vars {
			if v.Type == VarList {
				listVars[v.Key] = true
			}
			zeroData[v.Key] = zeroOf(v.Type)
		}
	}
	for _, v := range append(append([]Var{}, CommonVars...), WebhookVars...) {
		zeroData[v.Key] = zeroOf(v.Type)
	}
}

func zeroOf(t VarType) any {
	switch t {
	case VarInt:
		return int64(0)
	case VarFloat:
		return float64(0)
	case VarBool:
		return false
	case VarList:
		return []any{}
	}
	return ""
}

// Spec 是事件类型的说明，不认识的类型返回 nil。
func Spec(typ string) *EventSpec { return specByType[typ] }

// ListVars 是可以 range 的变量名。
func ListVars() map[string]bool { return listVars }

// SampleEvent 是这种事件的示例（按目录里各变量的 Sample），与内置标题、正文的示例：预览与保存前的校验用。
func SampleEvent(typ string) (ev *Event, title, text string) {
	spec := specByType[typ]
	if spec == nil {
		return nil, "", ""
	}
	fields := make(map[string]any, len(spec.Vars))
	for _, v := range spec.Vars {
		if v.Sample != nil {
			fields[v.Key] = v.Sample
		}
	}
	return NewEvent(typ, time.Date(2026, 10, 12, 8, 0, 0, 0, time.Local), fields), spec.SampleTitle, spec.SampleText
}
