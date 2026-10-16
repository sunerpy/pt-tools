package notify

import "slices"

// 事件的组：通知（发到所有通知通道，Webhook 收到的也是这些）、ChatOps 命令回复、AI 助手在通知通道里的回复。
// 回复与助手的事件只有正文（NoTitle），只在回复命令或助手说话的那个通道里发。
const (
	GroupNotify    = "notify"
	GroupReply     = "reply"
	GroupAssistant = "assistant"
)

// EventGroup 是消息模板页上的一组事件。
type EventGroup struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Desc string `json:"desc"`
}

// Groups 是事件的组，按页面上的顺序。
var Groups = []EventGroup{
	{Key: GroupNotify, Name: "通知", Desc: "发到所有通知通道的通知，Webhook 收到的也是这些"},
	{Key: GroupReply, Name: "ChatOps 命令回复", Desc: "在 Telegram、QQ 等通道里发命令时 pt-tools 的回复，只有正文"},
	{Key: GroupAssistant, Name: "AI 助手回复", Desc: "AI 助手在通知通道里发的回答、提示与确认，只有正文"},
}

// ChatOps 命令回复与 AI 助手回复的事件类型。
const (
	EventReplyStatus    = "reply.status"
	EventReplyTorrents  = "reply.torrents"
	EventReplySites     = "reply.sites"
	EventReplySiteInfo  = "reply.site_info"
	EventReplyTasks     = "reply.tasks"
	EventReplySignin    = "reply.signin"
	EventReplyVersion   = "reply.version"
	EventReplyHelp      = "reply.help"
	EventReplySubs      = "reply.subs"
	EventReplyOther     = "reply.other"
	EventAssistantReply = "assistant.reply"
)

// Events 是事件目录：通知、ChatOps 命令回复、AI 助手回复三组。改这里要同时改对应生产者（events_test 核对目录自身一致，
// 生产者的测试核对各变量的值）。
var Events = slices.Concat(
	inGroup(GroupNotify, false, notifyEvents),
	inGroup(GroupReply, true, replyEvents),
	inGroup(GroupAssistant, true, assistantEvents),
)

func inGroup(group string, noTitle bool, specs []EventSpec) []EventSpec {
	out := slices.Clone(specs)
	for i := range out {
		out[i].Group, out[i].NoTitle = group, noTitle
	}
	return out
}

// replyEvents 是 ChatOps 命令回复：几个查询命令各一种（带结构化的变量），其余的回复都是 reply.other。
var replyEvents = []EventSpec{
	{
		Type: EventReplyStatus, Name: "/status 状态概览", Desc: "发送 /status 时的回复",
		Vars: []Var{
			{Key: "tasks", Type: VarInt, Desc: "RSS 任务总数（任务服务不可用时是 0）", Sample: int64(3)},
			{Key: "running", Type: VarInt, Desc: "运行中的 RSS 任务数", Sample: int64(2)},
			{Key: "items", Type: VarList, Desc: "下载器", Item: []Var{
				{Key: "name", Type: VarString, Desc: "下载器名称"},
				{Key: "type", Type: VarString, Desc: "类型（qbittorrent、transmission）"},
				{Key: "healthy", Type: VarBool, Desc: "连得上"},
			}, Sample: []map[string]any{
				{"name": "家里的 qB", "type": "qbittorrent", "healthy": true},
				{"name": "NAS 的 TR", "type": "transmission", "healthy": false},
			}},
			more("省略的下载器数"),
		},
		SampleText: "状态概览\n任务: 3 总计 / 2 运行中\n下载器: 2\n  ✓ 家里的 qB (qbittorrent)\n  ✗ NAS 的 TR (transmission)",
	},
	{
		Type: EventReplyTorrents, Name: "/torrents 种子列表", Desc: "发送 /torrents <下载器> 时的回复（最多列 20 个）",
		Vars: []Var{
			{Key: "downloader", Type: VarString, Desc: "下载器", Sample: "家里的 qB"},
			{Key: "total", Type: VarInt, Desc: "下载器里的种子总数", Sample: int64(42)},
			{Key: "items", Type: VarList, Desc: "种子（最多 20 个）", Item: []Var{
				{Key: "name", Type: VarString, Desc: "种子名"},
				{Key: "state", Type: VarString, Desc: "状态（seeding、downloading、paused 等）"},
				{Key: "progress", Type: VarFloat, Desc: "进度（0–100）"},
				{Key: "size", Type: VarString, Desc: "大小"},
				{Key: "size_bytes", Type: VarInt, Desc: "大小（字节）"},
			}, Sample: []map[string]any{
				{"name": "Fake.Movie.2026.1080p.WEB-DL", "state": "seeding", "progress": float64(100), "size": "15.00 GiB", "size_bytes": int64(16106127360)},
				{"name": "Fake.Show.S01.2160p", "state": "downloading", "progress": float64(45.5), "size": "30.00 GiB", "size_bytes": int64(32212254720)},
			}},
			{Key: "items_more", Type: VarInt, Desc: "没列出的种子数", Sample: int64(40)},
		},
		SampleText: "```\n家里的 qB 种子（42 总计，显示前 2）\n[seeding] Fake.Movie.2026.1080p.WEB-DL 100.0% 15.00 GiB\n" +
			"[downloading] Fake.Show.S01.2160p 45.5% 30.00 GiB\n```",
	},
	{
		Type: EventReplySites, Name: "/sites 站点列表", Desc: "发送 /sites 时的回复",
		Vars: []Var{
			{Key: "count", Type: VarInt, Desc: "站点数", Sample: int64(2)},
			{Key: "items", Type: VarList, Desc: "站点", Item: []Var{
				{Key: "name", Type: VarString, Desc: "站点"},
				{Key: "status", Type: VarString, Desc: "enabled 或 disabled"},
				{Key: "enabled", Type: VarBool, Desc: "启用了"},
			}, Sample: []map[string]any{
				{"name": "hdtime", "status": "enabled", "enabled": true},
				{"name": "hdsky", "status": "disabled", "enabled": false},
			}},
			more("省略的站点数"),
		},
		SampleText: "站点（2）\n[enabled] hdtime\n[disabled] hdsky",
	},
	{
		Type: EventReplySiteInfo, Name: "/sites <站点> 用户信息", Desc: "发送 /sites <站点> 时的回复",
		Vars: []Var{
			{Key: "site", Type: VarString, Desc: "站点", Sample: "hdtime"},
			{Key: "user", Type: VarString, Desc: "用户名", Sample: "fakeuser"},
			{Key: "uploaded", Type: VarString, Desc: "上传量", Sample: "12.50 TiB"},
			{Key: "downloaded", Type: VarString, Desc: "下载量（计费）", Sample: "3.20 TiB"},
			{Key: "uploaded_bytes", Type: VarInt, Desc: "上传量（字节）", Sample: int64(13743895347200)},
			{Key: "downloaded_bytes", Type: VarInt, Desc: "下载量（计费，字节）", Sample: int64(3518437208883)},
			{Key: "ratio", Type: VarFloat, Desc: "分享率", Sample: float64(3.906)},
			{Key: "bonus", Type: VarFloat, Desc: "魔力", Sample: float64(123456)},
			{Key: "class", Type: VarString, Desc: "等级", Sample: "Elite User"},
		},
		SampleText: "```\nSite: hdtime\nUser: fakeuser\nUploaded: 12.50 TiB\nDownloaded (billed): 3.20 TiB\nRatio: 3.906\nBonus: 123456\nClass: Elite User\n```",
	},
	{
		Type: EventReplyTasks, Name: "/tasks RSS 任务", Desc: "发送 /tasks 时的回复",
		Vars: []Var{
			{Key: "count", Type: VarInt, Desc: "RSS 任务数", Sample: int64(2)},
			{Key: "running", Type: VarInt, Desc: "运行中的 RSS 任务数", Sample: int64(1)},
			{Key: "items", Type: VarList, Desc: "RSS 任务", Item: []Var{
				{Key: "site", Type: VarString, Desc: "站点"},
				{Key: "rss", Type: VarString, Desc: "RSS 名称"},
				{Key: "state", Type: VarString, Desc: "running 或 stopped"},
			}, Sample: []map[string]any{
				{"site": "hdtime", "rss": "HDTime 电影", "state": "running"},
				{"site": "hdsky", "rss": "HDSky 剧集", "state": "stopped"},
			}},
			more("省略的任务数"),
		},
		SampleText: "RSS 任务（2）\n[running] hdtime/HDTime 电影\n[stopped] hdsky/HDSky 剧集",
	},
	{
		Type: EventReplySignin, Name: "/signin 签到", Desc: "发送 /signin 时的回复（签到一个站点，或开启了自动签到的全部站点）",
		Vars: []Var{
			{Key: "signed", Type: VarInt, Desc: "签到成功或今天已签到的站点数", Sample: int64(1)},
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
		SampleText: "hdtime: 签到成功 (获得 10 魔力)\nhdsky: 签到失败 (站点超时)",
	},
	{
		Type: EventReplyVersion, Name: "/version 版本", Desc: "发送 /version 时的回复",
		Vars: []Var{
			{Key: "version", Type: VarString, Desc: "当前版本", Sample: "v1.0.0-rc.27"},
			{Key: "build", Type: VarString, Desc: "构建时间", Sample: "2026-10-12T08:00:00Z"},
			{Key: "commit", Type: VarString, Desc: "提交", Sample: "5a5734d"},
			{Key: "update_checked", Type: VarBool, Desc: "检查过更新（没检查过时下面两项没有意义）", Sample: true},
			{Key: "has_update", Type: VarBool, Desc: "有新版本", Sample: true},
			{Key: "latest", Type: VarString, Desc: "最新的版本（有新版本时才有）", Sample: "v1.0.0"},
		},
		SampleText: "```\nVersion: v1.0.0-rc.27\nBuild: 2026-10-12T08:00:00Z\nCommit: 5a5734d\n有新版本可用: v1.0.0\n```",
	},
	{
		Type: EventReplyHelp, Name: "/help 命令列表", Desc: "发送 /help 时的回复（不是管理员的绑定看不到管理员命令）",
		Vars: []Var{
			{Key: "items", Type: VarList, Desc: "命令", Item: []Var{
				{Key: "name", Type: VarString, Desc: "命令（不带 /）"},
				{Key: "description", Type: VarString, Desc: "说明"},
				{Key: "admin", Type: VarBool, Desc: "只有管理员能用"},
			}, Sample: []map[string]any{
				{"name": "help", "description": "列出可用命令 (List available commands)", "admin": false},
				{"name": "signin", "description": "立即签到 (Sign in now)", "admin": true},
			}},
			more("省略的命令数"),
		},
		SampleText: "可用命令：\n/help — 列出可用命令 (List available commands)\n/signin [admin] — 立即签到 (Sign in now)",
	},
	{
		Type: EventReplySubs, Name: "/subs 订阅列表", Desc: "发送 /subs 时的回复（新的在前，最多列 20 个）",
		Vars: []Var{
			{Key: "count", Type: VarInt, Desc: "订阅总数", Sample: int64(2)},
			{Key: "items", Type: VarList, Desc: "订阅（最多 20 个）", Item: []Var{
				{Key: "title", Type: VarString, Desc: "订阅（带年份与季）"},
				{Key: "status", Type: VarString, Desc: "状态（在找、暂停、待确认、完成）"},
				{Key: "progress", Type: VarString, Desc: "进度（可能是空）"},
			}, Sample: []map[string]any{
				{"title": "沙丘 (2021)", "status": "完成", "progress": "已入库"},
				{"title": "最后生还者 (2023) 第 2 季", "status": "在找", "progress": "已入库 3/7"},
			}},
			{Key: "items_more", Type: VarInt, Desc: "没列出的订阅数", Sample: int64(0)},
		},
		SampleText: "1. 沙丘 (2021) · 完成 · 已入库\n2. 最后生还者 (2023) 第 2 季 · 在找 · 已入库 3/7",
	},
	{
		Type: EventReplyOther, Name: "其他回复",
		Desc: "上面没单列的回复：其他命令（/pause、/addrss 等）的结果、命令出错，以及没绑定、权限不够、命令不认识时的提示",
		Vars: []Var{
			{Key: "command", Type: VarString, Desc: "命令（不带 /；不是命令时是空，命令的下一步回复是 session:<步骤>）", Sample: "pause"},
			{Key: "ok", Type: VarBool, Desc: "命令做成了（参数不对、服务不可用、出错、被拒时是 false）", Sample: true},
		},
		SampleText: "已暂停 abc123",
	},
}

// assistantEvents 是 AI 助手在通知通道里发的消息。
var assistantEvents = []EventSpec{
	{
		Type: EventAssistantReply, Name: "AI 助手的回复",
		Desc: "AI 助手在通知通道里发的所有消息：回答、pt-tools 的提示、要你确认的操作、确认超时、收到消息时的即时答复。" +
			"回答太长时先套模板再分成几条发",
		Vars: []Var{
			{
				Key: "kind", Type: VarString,
				Desc:   "哪一种：answer（回答）、notice（pt-tools 的提示）、confirm（要确认的操作）、timeout（确认超时）、status（收到消息时的即时答复）",
				Sample: "answer",
			},
			{Key: "summary", Type: VarString, Desc: "要确认的操作（confirm、timeout 才有）", Sample: ""},
			{Key: "expires", Type: VarString, Desc: "确认的截止时间（HH:MM，confirm 才有）", Sample: ""},
		},
		SampleText: "HDTime 今天上传了 100.00 GiB，下载了 20.00 GiB。",
	},
}
