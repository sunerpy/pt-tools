package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/sunerpy/pt-tools/core"
	"github.com/sunerpy/pt-tools/global"
	"github.com/sunerpy/pt-tools/internal/cookiecloud"
	"github.com/sunerpy/pt-tools/internal/media/organize"
	"github.com/sunerpy/pt-tools/internal/media/recognize"
	"github.com/sunerpy/pt-tools/internal/media/subscribe"
	"github.com/sunerpy/pt-tools/internal/reseed"
	"github.com/sunerpy/pt-tools/models"
	v2 "github.com/sunerpy/pt-tools/site/v2"
)

// 配置网关登记的资源（见 config_gateway.go）。能改的字段尽量取自接口自己的请求结构（jsonFields），
// 接口改了字段这里跟着变；读出来的别的字段只给看。

// jsonFields 是结构体的 JSON 字段名（按声明顺序，跳过 "-"）。
func jsonFields[T any](skip ...string) []string {
	t := reflect.TypeFor[T]()
	var out []string
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		if !slices.Contains(skip, name) {
			out = append(out, name)
		}
	}
	return out
}

// globalFields 是 POST /api/global 收的字段（apiGlobal 里的请求结构）。签到、战报、失效种子扫描各有自己的资源。
var globalFields = []string{
	"default_interval_minutes", "default_concurrency", "default_enabled", "download_dir", "download_limit_enabled",
	"download_speed_limit", "torrent_size_gb", "torrent_min_size_gb", "min_free_minutes", "auto_start", "retain_hours",
	"max_retry", "cleanup_enabled", "cleanup_interval_min", "cleanup_scope", "cleanup_scope_tags", "cleanup_remove_data",
	"cleanup_condition_mode", "cleanup_max_seed_time_h", "cleanup_min_ratio", "cleanup_max_inactive_h",
	"cleanup_slow_seed_time_h", "cleanup_slow_max_ratio", "cleanup_del_free_expired", "cleanup_disk_protect",
	"cleanup_min_disk_space_gb", "cleanup_protect_dl", "cleanup_protect_hr", "cleanup_min_retain_h", "cleanup_protect_tags",
	"auto_delete_on_free_end", "free_end_advance_minutes", "default_filter_mode", "peer_ratio_enabled", "peer_ratio_max_sl",
	"peer_ratio_interval_min", "peer_ratio_remove_data",
}

// rssFields 是 RSS 订阅能改的字段（models.RSSConfig 里 UpsertSiteWithRSS 与 AppendRSSToSite 会存的那些）。
var rssFields = []string{
	"name", "category", "tag", "interval_minutes", "downloader_id", "download_path", "filter_rule_ids",
	"pause_on_free_end", "filter_mode", "notify_mode", "notify_conf_ids", "max_notifications_per_hour",
}

// notifySecretRe 判断通知通道配置里的键是不是密钥：令牌、密钥、密码、签名，以及可能带着令牌的地址、请求头与请求体模板。
var notifySecretRe = regexp.MustCompile(`(?i)(token|secret|password|passwd|key|sign|auth|cookie|webhook|endpoint|url|header|body_template|proxy)`)

const notifyPrefix = "config."

const (
	impactReload  = "保存以后 RSS 任务与相关的后台任务按新设置重新加载"
	impactNoRetro = "只影响以后：已经推送的种子、已经发出的通知不会跟着改"
)

// configResources 是能经网关读改的全部资源。
func (s *Server) configResources() []*configResource {
	list := []*configResource{
		{
			Name: "global", Title: "全局设置", Kind: configObject, Path: "/api/global", SetMethod: http.MethodPost,
			Desc:   "RSS 的默认间隔与下载目录、推送限速、种子大小限制、最短免费时间、启动时自动开始，自动删种、免费结束时的处理与做种人数比监控。",
			Fields: globalFields, Ops: []string{configOpSet},
			Hidden: []string{
				"id", "created_at", "updated_at", "attendance_window_start", "attendance_window_end", "attendance_ai_enabled",
				"attendance_vision_model", "attendance_notify_enabled", "attendance_notify_channel_ids", "daily_report_enabled",
				"daily_report_time", "daily_report_channel_ids", "daily_report_scope", "dead_torrent_scan_enabled", "dead_torrent_scan_interval_h",
				"dead_torrent_scan_channel_ids",
			},
			Hints: map[string]string{
				"default_filter_mode":      "auto_free（免费的自动下、命中规则的也下）、filter_only（只下命中规则的）、free_only（只下来得及下完的免费种子）",
				"cleanup_scope":            "database（只管 pt-tools 推送的种子）、tag（按 cleanup_scope_tags 里的标签）、all（下载器里的全部种子）",
				"cleanup_condition_mode":   "or（满足一个条件就删）、and（全部满足才删）",
				"download_dir":             "不能为空",
				"torrent_size_gb":          "最大种子大小（GB），0 不限；要大于 torrent_min_size_gb",
				"free_end_advance_minutes": "免费结束前提前处理的分钟数（0–60）",
			},
			Labels: map[string]string{
				// 和网页「全局设置」上的叫法一样
				"default_interval_minutes": "默认间隔（分钟）", "default_concurrency": "默认并发数", "default_enabled": "新 RSS 默认启用",
				"download_dir": "种子下载目录", "download_limit_enabled": "启用下载限速判断", "download_speed_limit": "预估下载速度（MB/s）",
				"torrent_size_gb": "最大种子大小（GB）", "torrent_min_size_gb": "最小种子大小（GB）", "min_free_minutes": "最短免费时间（分钟）",
				"auto_start": "自动启动 RSS 任务", "retain_hours": "暂存种子保留时长（小时）", "max_retry": "最大重试次数",
				"cleanup_enabled": "自动删种", "cleanup_remove_data": "删种时删数据", "auto_delete_on_free_end": "免费结束自动删除",
				"free_end_advance_minutes": "免费期结束前提前处理（分钟）", "default_filter_mode": "默认下载模式", "peer_ratio_enabled": "做种人数比监控",
			},
			Impact: []string{impactReload, "自动删种与做种人数比监控按新条件处理下载器里的种子（cleanup_remove_data、peer_ratio_remove_data 为真时连数据一起删）"},
		},
		{
			Name: "attendance", Title: "自动签到", Kind: configObject, Path: "/api/sites/attendance/settings", SetMethod: http.MethodPut,
			Desc:   "每天签到的时间窗口、要不要让 AI 识别验证码与答题、签到结果发到哪些通知通道。各站点开不开签到在 sites 的 attendance_enabled。",
			Fields: []string{"window_start", "window_end", "ai_enabled", "vision_model", "notify_enabled", "notify_channel_ids"},
			Ops:    []string{configOpSet},
			Hints: map[string]string{
				"window_start": "HH:MM，各站每天在窗口里随机挑一个时刻签到", "window_end": "HH:MM",
				"vision_model": "识别验证码用的看图模型，空着用 AI 助手设置里的模型", "notify_channel_ids": "通知通道的编号（notify_channels 的 id）",
			},
			Impact: []string{"从下一次签到起按新设置"},
		},
		{
			Name: "daily_report", Title: "每日战报", Kind: configObject, Path: "/api/v2/userinfo/daily-report", SetMethod: http.MethodPut,
			Desc:   "每天定时把各站点的上传、下载、魔力汇总发到通知通道。",
			Fields: jsonFields[core.DailyReportSettings](), Ops: []string{configOpSet},
			Hints: map[string]string{
				"time": "HH:MM", "channel_ids": "开启时至少一个通知通道（notify_channels 的 id）",
				"scope": "previous_day（前一天整天，默认）或 today（当天截至发送时刻）",
			},
		},
		{
			Name: "dead_scan", Title: "失效种子扫描", Kind: configObject, Path: "/api/downloader-assistant/dead-scan", SetMethod: http.MethodPut,
			Desc:   "定时扫描下载器里 tracker 报错（被删、未注册）的种子，发到通知通道（只通知，不删）。",
			Fields: jsonFields[core.DeadTorrentScanSettings](), Ops: []string{configOpSet},
			Hints: map[string]string{"interval_hours": "6–168", "channel_ids": "开启时至少一个通知通道"},
		},
		{
			Name: "log_level", Title: "日志级别", Kind: configObject, Path: "/api/log-level", SetMethod: http.MethodPut,
			Desc: "pt-tools 写日志的详细程度。", Fields: []string{"level"}, Ops: []string{configOpSet},
			Hints: map[string]string{"level": "debug、info、warn、error"},
		},
		{
			Name: "reseed", Title: "辅种", Kind: configObject, Path: "/api/reseed/settings", SetMethod: http.MethodPut,
			Desc:   "按 IYUU 的辅种接口，给下载器里已有的种子在别的站点找同一个资源并添加。",
			Fields: jsonFields[reseed.SettingsUpdate]("token"), Secrets: map[string]string{"token": "has_token"},
			Ops:    []string{configOpSet},
			Hints:  map[string]string{"token": "IYUU 的令牌", "site_names": "要辅种的站点（sites 的 name）", "downloader_ids": "要辅种的下载器（downloaders 的 id）"},
			Impact: []string{"开着、有令牌时按间隔自动辅种：会往下载器里加种子"},
		},
		{
			Name: "cookiecloud", Title: "CookieCloud 同步", Kind: configObject, Path: "/api/cookiecloud/settings", SetMethod: http.MethodPut,
			Desc:    "从 CookieCloud 服务器取浏览器里的 Cookie，更新站点的 Cookie。",
			Fields:  jsonFields[cookiecloud.SettingsUpdate]("password", "uuid"),
			Secrets: map[string]string{"password": "has_password", "uuid": ""}, Resend: []string{"uuid"},
			Ops:    []string{configOpSet},
			Impact: []string{"打开定时同步以后会按间隔覆盖站点的 Cookie"},
		},
		{
			Name: "cloak", Title: "CloakBrowser", Kind: configObject, Path: "/api/cloak/config", SetMethod: http.MethodPut,
			Desc:   "CloakBrowser Manager 的地址与 Profile：Cookie 方式检查不了登录状态的站点用它打开网页检查。",
			Fields: []string{"endpoint", "profile_id"}, Secrets: map[string]string{"token": "has_token"},
			Ops: []string{configOpSet}, Hidden: []string{"manager_version"},
		},
		{
			Name: "media", Title: "媒体识别（TMDB）", Kind: configObject, Path: "/api/media/settings", SetMethod: http.MethodPut,
			Desc: "识别种子标题对应的电影与剧集用的 TMDB：API Key（没填时用发布版本内置的，key_source 写着现在用哪个）、语言、代理与换掉的接口地址；" +
				"以及刮削用的 Fanart.tv 与 OMDb 的 API Key（填了才用这两个来源）。",
			Fields: jsonFields[recognize.SettingsInput]("tmdb_api_key", "fanart_api_key", "fanart_client_key", "omdb_api_key"),
			Secrets: map[string]string{
				"tmdb_api_key": "has_tmdb_key", "fanart_api_key": "has_fanart_key", "fanart_client_key": "has_fanart_client_key",
				"omdb_api_key": "has_omdb_key",
			},
			Ops:   []string{configOpSet},
			Hints: map[string]string{"language": "例如 zh-CN", "api_base_url": "空着用官方地址", "image_base_url": "空着用官方地址"},
		},
		{
			Name: "organize", Title: "整理入库", Kind: configObject, Path: "/api/media/organize/settings", SetMethod: http.MethodPut,
			Desc:   "下载完的电影与剧集要不要自动整理进媒体库、定期扫描、只管哪些下载器、分类、标签与保存路径。",
			Fields: jsonFields[organize.SettingsInput](), Ops: []string{configOpSet},
			Impact: []string{"打开自动整理以后，之后下完的种子按媒体库的设置硬链接或复制进媒体库"},
		},
		{
			Name: "classify", Title: "二级分类", Kind: configObject, Path: "/api/media/organize/classify", SetMethod: http.MethodPut,
			Wrap:   "rules",
			Desc:   "整理入库时按条件把电影、剧集放进分类目录的规则（movie、tv 各是一组规则，整组替换）。每条规则：name，以及 genres（TMDB 类型编号）、languages（原始语言，例如 ja）、countries（例如 CN）里的任意几项。",
			Fields: jsonFields[organize.ClassifyRules](), Ops: []string{configOpSet},
		},
		{
			Name: "subscribe", Title: "订阅设置", Kind: configObject, Path: "/api/media/subscribe/settings", SetMethod: http.MethodPut,
			Desc:   "电影、剧集订阅的开关、搜索间隔、跳过的站点、默认的质量规则与下载器、通知通道、洗版时旧文件怎么办。",
			Fields: jsonFields[subscribe.Settings](), Ops: []string{configOpSet},
		},
		{
			Name: "downloaders", Title: "下载器", Kind: configList, Path: "/api/downloaders", ItemPath: "/api/downloaders/{id}",
			Desc:   "qBittorrent 与 Transmission 下载器：地址、账号、默认下载器、推送后自动开始。",
			Fields: jsonFields[DownloaderRequest]("password"), Secrets: map[string]string{"password": ""},
			Required: []string{"name", "type", "url"},
			Ops:      []string{configOpAdd, configOpUpdate, configOpDelete},
			Hints: map[string]string{
				"type": "qbittorrent 或 transmission", "is_default": "只能有一个默认下载器；默认下载器不能停用，要先把别的设为默认",
				"url": "例如 http://192.168.1.2:8080",
			},
			Impact: []string{
				"RSS 与订阅按「RSS 指定 → 站点指定 → 默认下载器」选下载器：改默认下载器会改变没指定下载器的 RSS 推到哪里",
				"改了地址或账号以后 pt-tools 重新连接这个下载器",
			},
		},
		s.configSitesResource(),
		s.configRSSResource(),
		{
			Name: "filter_rules", Title: "过滤规则", Kind: configList, Path: "/api/filter-rules", ItemPath: "/api/filter-rules/{id}",
			Desc:   "RSS 里按标题或标签匹配要下载（或者只通知）的种子的规则；在 rss 的 filter_rule_ids 里关联到 RSS。",
			Fields: jsonFields[FilterRuleRequest](), Required: []string{"name", "pattern"},
			Ops: []string{configOpAdd, configOpUpdate, configOpDelete},
			Hints: map[string]string{
				"pattern_type": "keyword、wildcard、regex", "match_field": "title、tag、both",
				"purpose": "download（下载）、notify（只用于命中规则的通知）、both", "min_size_gb": "0 不限", "max_size_gb": "0 不限",
			},
			Impact: []string{"停用一条规则会同时解除它与 RSS 的关联（重新启用以后要重新关联）", impactNoRetro},
		},
		s.configNotifyChannelsResource(),
		{
			Name: "notify_templates", Title: "消息模板", Kind: configList, Path: "/api/chatops/templates", Wrap: "events",
			ItemPath: "/api/chatops/templates/{id}", IDField: "type",
			Desc:   "各种通知（以及 ChatOps 命令回复）的标题与正文模板，Go 模板语法，例如 {{.site}}；vars 是能用的变量。title、text 空着用内置文字。",
			Fields: []string{"title", "text"}, Ops: []string{configOpUpdate, configOpDelete},
			Show:       []string{"type", "group", "name", "desc", "title", "text", "customized", "vars", "sample_title", "sample_text"},
			DeleteVerb: "恢复默认", DeleteImpact: []string{"恢复成内置的文字"},
			view: func(_, out map[string]any) {
				if vars, ok := out["vars"].([]any); ok {
					keys := make([]any, 0, len(vars))
					for _, v := range vars {
						if m, ok := v.(map[string]any); ok {
							keys = append(keys, m["key"])
						}
					}
					out["vars"] = keys
				}
			},
			Impact: []string{"所有通知通道发这种消息时都用新模板；模板出错时照旧发内置文字"},
		},
		{
			Name: "login_reminders", Title: "登录提醒", Kind: configList, Path: "/api/sites/login-state", IDField: "site_name",
			NameField: "display_name", ItemPath: "/api/sites/{id}/login-state/config", Partial: true,
			Desc:   "各站点的保号提醒：多少天不登录会被封、提前几天提醒、什么时候检查、发到哪些通知通道、怎么探测登录状态。",
			Fields: []string{"ban_threshold_days", "remind_before_days", "reminder_cron", "notification_channel_ids", "probe_mode"},
			Show:   []string{"site_name", "display_name", "enabled", "tier", "days_remaining", "last_probe_status"},
			Ops:    []string{configOpUpdate},
			Hints: map[string]string{
				"reminder_cron": "cron 表达式，例如 0 10,22 * * *", "probe_mode": "auto、manual、disabled",
				"ban_threshold_days": "1–365", "remind_before_days": "1–365",
			},
		},
		{
			Name: "brush_tasks", Title: "刷流任务", Kind: configList, Path: "/api/brush/tasks", ItemPath: "/api/brush/tasks/{id}",
			Desc:   "按条件从站点的种子列表挑种子推送到下载器刷上传量，并按做种时间、分享率、速度删种。",
			Fields: jsonFields[BrushTaskRequest](), Required: []string{"name", "site_name", "downloader_id"},
			Ops: []string{configOpAdd, configOpUpdate, configOpDelete},
			Hints: map[string]string{
				"discounts": "逗号分隔的优惠类型，例如 FREE,2XFREE", "remove_with_data": "删种时连数据一起删",
				"interval_min": "多久挑一轮（分钟）",
				"section":      "只对 M-Team：normal 综合区、adult 成人区、all 两个都读；空着等于 normal",
			},
			Impact: []string{"开着的任务会按间隔往下载器里加种子、按条件删种", "任务正在运行一轮时不能保存，稍后再试"},
		},
		{
			Name: "transfer_rules", Title: "转移做种规则", Kind: configList, Path: "/api/transfer/rules", Wrap: "items",
			ItemPath: "/api/transfer/rules/{id}",
			Desc:     "把一个下载器里做种够久的种子按条件转到另一个下载器接着做种。",
			Fields:   jsonFields[TransferRuleRequest](), Required: []string{"name", "source_downloader_id", "target_downloader_id"},
			Ops:    []string{configOpAdd, configOpUpdate, configOpDelete},
			Impact: []string{"开着的规则会按间隔把种子从来源下载器转到目标下载器"},
		},
		{
			Name: "libraries", Title: "媒体库", Kind: configList, Path: "/api/media/libraries", ItemPath: "/api/media/libraries/{id}",
			Desc:   "整理入库的目标：电影库、剧集库的路径、命名模板、硬链接还是复制、刮削。",
			Fields: jsonFields[organize.LibraryInput](), Required: []string{"name", "kind", "path"},
			Ops:    []string{configOpAdd, configOpUpdate, configOpDelete},
			Hints:  map[string]string{"kind": "movie 或 tv", "mode": "link（硬链接）、copy、move 之类，见网页上的选项"},
			Impact: []string{"只影响以后整理的文件：已经放进库的文件不会挪动"},
		},
		{
			Name: "media_path_maps", Title: "下载器路径映射", Kind: configList, Path: "/api/media/path-maps", ItemPath: "/api/media/path-maps/{id}",
			NameField: "downloader_prefix",
			Desc:      "下载器里的路径对应到 pt-tools 看到的路径（两边挂载不一样时整理入库要它）。",
			Fields:    jsonFields[organize.PathMapInput](), Required: []string{"downloader_id", "downloader_prefix", "local_prefix"},
			Ops: []string{configOpAdd, configOpUpdate, configOpDelete},
		},
		{
			Name: "watch_dirs", Title: "监控目录", Kind: configList, Path: "/api/media/watch-dirs", ItemPath: "/api/media/watch-dirs/{id}",
			NameField: "path", Desc: "不经下载器、直接放进来的文件也整理进媒体库的目录。",
			Fields: jsonFields[organize.WatchDirInput](), Required: []string{"path"},
			Ops: []string{configOpAdd, configOpUpdate, configOpDelete},
		},
		{
			Name: "media_servers", Title: "媒体服务器", Kind: configList, Path: "/api/media/servers", ItemPath: "/api/media/servers/{id}",
			Desc:   "整理入库以后通知刷新的 Emby、Jellyfin、Plex。",
			Fields: jsonFields[organize.ServerInput]("token"), Secrets: map[string]string{"token": "has_token"},
			Required: []string{"name", "kind", "url", "token"},
			Ops:      []string{configOpAdd, configOpUpdate, configOpDelete},
		},
		{
			Name: "quality_profiles", Title: "质量规则", Kind: configList, Path: "/api/media/quality-profiles", Wrap: "items",
			ItemPath: "/api/media/quality-profiles/{id}",
			Desc:     "订阅挑种子的质量要求：分辨率、片源、编码、HDR、中字、免费、制作组、大小与做种人数。",
			Fields:   jsonFields[subscribe.ProfileInput](), Required: []string{"name"},
			Ops: []string{configOpAdd, configOpUpdate, configOpDelete},
		},
		{
			Name: "douban_sources", Title: "豆瓣想看", Kind: configList, Path: "/api/media/douban-sources", ItemPath: "/api/media/douban-sources/{id}",
			Desc:   "定期读豆瓣用户的想看，把新加的电影、剧集加成订阅。",
			Fields: jsonFields[subscribe.DoubanSourceInput](), Required: []string{"user_id"},
			Ops:    []string{configOpAdd, configOpUpdate, configOpDelete},
			Impact: []string{"开着时会按想看列表自动添加订阅"},
		},
		{
			Name: "media_words", Title: "识别词", Kind: configList, Path: "/api/media/words", ItemPath: "/api/media/words/{id}",
			NameField: "pattern",
			Desc:      "识别种子标题以前先做的替换、屏蔽与集数偏移。",
			Fields:    jsonFields[mediaWordBody](), Required: []string{"kind", "pattern"},
			Ops: []string{configOpAdd, configOpUpdate, configOpDelete},
		},
		{
			Name: "media_overrides", Title: "识别纠正", Kind: configList, Path: "/api/media/overrides", ItemPath: "/api/media/overrides/{id}",
			NameField: "title",
			Desc:      "指定某个标题就是 TMDB 上的哪一部（自动识别认错时用）。",
			Fields:    jsonFields[recognize.OverrideInput](), Required: []string{"title", "tmdb_id", "media_type"},
			Ops:   []string{configOpAdd, configOpDelete},
			Hints: map[string]string{"media_type": "movie 或 tv"},
		},
	}
	return list
}

// configNormalize 把 Go 的值换成 JSON 解析出来的样子（数字是 json.Number），和接口读出来的一致。
func configNormalize(v any) map[string]any {
	buf, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if configDecode(buf, &out) != nil {
		return map[string]any{}
	}
	return out
}

// configNumber 把 Go 的整数、浮点数换成 json.Number（JSON 解析出来的已经是 json.Number 或 float64）。
func configNumber(v any) any {
	switch v.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32:
		if buf, err := json.Marshal(v); err == nil {
			return json.Number(buf)
		}
	}
	return v
}

func configInt(field string, v any) (int, error) {
	switch t := configNumber(v).(type) {
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return int(n), nil
		}
		if f, err := t.Float64(); err == nil && f == float64(int(f)) {
			return int(f), nil
		}
	case float64:
		if t == float64(int(t)) {
			return int(t), nil
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return n, nil
		}
	}
	return 0, configBad("%s 要是整数", field)
}

func configFloat(field string, v any) (float64, error) {
	switch t := configNumber(v).(type) {
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return f, nil
		}
	case float64:
		return t, nil
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
			return f, nil
		}
	}
	return 0, configBad("%s 要是数字", field)
}

func configBool(field string, v any) (bool, error) {
	if b, ok := v.(bool); ok {
		return b, nil
	}
	return false, configBad("%s 要是 true 或 false", field)
}

func configString(field string, v any) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	return "", configBad("%s 要是字符串", field)
}

// ---- 站点 ----

func (s *Server) configSitesResource() *configResource {
	return &configResource{
		Name: "sites", Title: "站点", Kind: configList, IDField: "name", NameField: "display_name",
		Desc: "PT 站点：启用、Cookie / API Key / Passkey、上传下载限速、刷流容量上限、H&R 做种要求、每日自动签到。" +
			"站点本身来自内置的站点定义（不能新增、删除）；认证方式与地址在网页上改。",
		Fields:  []string{"enabled", "upload_limit_kbs", "download_limit_kbs", "seeding_capacity_gb", "hr_seed_time_h", "attendance_enabled"},
		Secrets: map[string]string{"cookie": "has_cookie", "api_key": "", "passkey": ""},
		Ops:     []string{configOpUpdate},
		Hints: map[string]string{
			"enabled":             "启用时要有认证方式要的凭据（auth_method：cookie、api_key、cookie_and_api_key、passkey）",
			"seeding_capacity_gb": "0 不限", "hr_seed_time_h": "站点定义里没有 H&R 做种时长时按它（小时），0 是没填",
			"attendance_enabled": "每天自动签到；不支持签到的站点开不了",
		},
		Impact: []string{"改了凭据或启用状态以后 pt-tools 刷新站点并检查一次登录状态", impactReload},
		load:   s.configSitesLoad,
		save:   s.configSitesSave,
	}
}

func (s *Server) configSitesLoad(ctx context.Context) (any, error) {
	if global.GlobalDB == nil {
		return nil, &configError{Status: http.StatusServiceUnavailable, Message: "数据库还没有打开"}
	}
	db := global.GlobalDB.DB.WithContext(ctx)
	var rows []models.SiteSetting
	if err := db.Order("name").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("读站点失败: %w", err)
	}
	type count struct {
		SiteID uint
		N      int
	}
	var counts []count
	if err := db.Model(&models.RSSSubscription{}).Select("site_id, count(*) as n").Group("site_id").Scan(&counts).Error; err != nil {
		return nil, fmt.Errorf("读 RSS 失败: %w", err)
	}
	rssBy := map[uint]int{}
	for _, c := range counts {
		rssBy[c.SiteID] = c.N
	}
	defs := v2.GetDefinitionRegistry()
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		display := row.DisplayName
		unavailable := false
		if def, ok := defs.Get(row.Name); ok {
			display, unavailable = def.Name, def.Unavailable
		}
		items = append(items, configNormalize(map[string]any{
			"name": row.Name, "display_name": display, "enabled": row.Enabled, "auth_method": row.AuthMethod,
			"has_cookie": strings.TrimSpace(row.CookieEncrypted) != "" || strings.TrimSpace(row.Cookie) != "",
			"api_key":    row.APIKey, "passkey": row.Passkey, "upload_limit_kbs": row.UploadLimitKBs,
			"download_limit_kbs": row.DownloadLimitKBs, "seeding_capacity_gb": row.SeedingCapacityGB,
			"hr_seed_time_h": row.HRSeedTimeH, "attendance_enabled": row.AttendanceEnabled, "rss_count": rssBy[row.ID],
			"unavailable": unavailable, "is_builtin": row.IsBuiltin,
		}))
	}
	return items, nil
}

func (s *Server) configSitesSave(_ context.Context, _, id string, _, _, changed map[string]any) (string, error) {
	var o core.SiteOptions
	opts, refresh := false, false
	for k, v := range changed {
		var err error
		switch k {
		case "enabled":
			var b bool
			b, err = configBool(k, v)
			o.Enabled, opts, refresh = &b, true, true
		case "upload_limit_kbs":
			var n int
			n, err = configInt(k, v)
			o.UploadLimitKBs, opts = &n, true
		case "download_limit_kbs":
			var n int
			n, err = configInt(k, v)
			o.DownloadLimitKBs, opts = &n, true
		case "seeding_capacity_gb":
			var f float64
			f, err = configFloat(k, v)
			o.SeedingCapacityGB, opts = &f, true
		case "hr_seed_time_h":
			var n int
			n, err = configInt(k, v)
			o.HRSeedTimeH, opts = &n, true
		case "cookie", "api_key", "passkey":
			var str string
			str, err = configString(k, v)
			switch k {
			case "cookie":
				o.Cookie = &str
			case "api_key":
				o.APIKey = &str
			default:
				o.Passkey = &str
			}
			opts, refresh = true, true
		}
		if err != nil {
			return "", err
		}
	}
	if opts {
		if err := s.store.UpdateSiteOptions(models.SiteGroup(id), o); err != nil {
			return "", configBad("%v", err)
		}
	}
	if v, ok := changed["attendance_enabled"]; ok {
		on, err := configBool("attendance_enabled", v)
		if err != nil {
			return "", err
		}
		if on {
			def, _ := v2.GetDefinitionRegistry().Get(id)
			if ok, reason := v2.AttendanceSupport(def); !ok {
				return "", configBad("站点 %s 不能开启自动签到：%s", id, reason)
			}
		}
		if err := s.store.SetSiteAttendanceEnabled(id, on); err != nil {
			return "", err
		}
	}
	if refresh {
		// 同网页保存站点：刷新站点注册，成功后检查一次登录状态
		s.goBackground(func() {
			if err := RefreshSiteRegistrations(s.store); err != nil {
				global.GetSlogger().Warnf("[Config] 刷新站点注册失败: %v", err)
				return
			}
			s.requestLoginProbe(id)
		})
	}
	return "", nil
}

// ---- RSS ----

func (s *Server) configRSSResource() *configResource {
	return &configResource{
		Name: "rss", Title: "RSS 订阅", Kind: configList,
		Desc:   "各站点的 RSS 订阅：多久抓一次、推到哪个下载器与路径、关联哪些过滤规则、下载模式、通知。新增要给 site（sites 的 name）。",
		Fields: rssFields, AddOnly: []string{"site"}, Secrets: map[string]string{"url": ""},
		Required: []string{"site", "name", "url"},
		Ops:      []string{configOpAdd, configOpUpdate, configOpDelete},
		Hints: map[string]string{
			"url": "站点 RSS 页面上复制的地址（带 passkey，只写不读）", "interval_minutes": "最少 " + strconv.Itoa(int(models.MinIntervalMinutes)) + " 分钟",
			"downloader_id": "null 用站点或默认下载器", "filter_mode": "空着用全局的 default_filter_mode；auto_free、filter_only、free_only",
			"notify_mode": "空着不通知；all（每条新种子）、filtered（命中规则的）、both", "notify_conf_ids": "通知通道编号的 JSON 数组，例如 [1,2]",
			"filter_rule_ids": "过滤规则的编号（filter_rules 的 id）",
		},
		Impact: []string{impactReload, impactNoRetro},
		load:   s.configRSSLoad,
		save:   s.configRSSSave,
		before: func(_ string, _, values map[string]any) error {
			// notify_conf_ids 存的是 JSON 字符串：给了数组时写成字符串
			if v, ok := values["notify_conf_ids"]; ok {
				if _, isStr := v.(string); !isStr && v != nil {
					buf, err := json.Marshal(v)
					if err != nil {
						return configBad("notify_conf_ids 要是编号的数组")
					}
					values["notify_conf_ids"] = string(buf)
				}
			}
			return nil
		},
	}
}

func (s *Server) configRSSLoad(ctx context.Context) (any, error) {
	if global.GlobalDB == nil {
		return nil, &configError{Status: http.StatusServiceUnavailable, Message: "数据库还没有打开"}
	}
	db := global.GlobalDB.DB.WithContext(ctx)
	var sites []models.SiteSetting
	if err := db.Select("id", "name").Find(&sites).Error; err != nil {
		return nil, fmt.Errorf("读站点失败: %w", err)
	}
	siteName := map[uint]string{}
	for _, st := range sites {
		siteName[st.ID] = st.Name
	}
	var subs []models.RSSSubscription
	if err := db.Order("id").Find(&subs).Error; err != nil {
		return nil, fmt.Errorf("读 RSS 失败: %w", err)
	}
	assoc := models.NewRSSFilterAssociationDB(db)
	items := make([]map[string]any, 0, len(subs))
	for _, r := range subs {
		name, ok := siteName[r.SiteID]
		if !ok {
			continue
		}
		ruleIDs, err := assoc.GetFilterRuleIDsForRSS(r.ID)
		if err != nil {
			return nil, fmt.Errorf("读过滤规则关联失败: %w", err)
		}
		if ruleIDs == nil {
			ruleIDs = []uint{}
		}
		items = append(items, configNormalize(map[string]any{
			"id": r.ID, "site": name, "name": r.Name, "url": r.URL, "category": r.Category, "tag": r.Tag,
			"interval_minutes": r.IntervalMinutes, "downloader_id": r.DownloaderID, "download_path": r.DownloadPath,
			"filter_rule_ids": ruleIDs, "pause_on_free_end": r.PauseOnFreeEnd, "filter_mode": r.FilterMode,
			"notify_mode": r.NotifyMode, "notify_conf_ids": r.NotifyConfIDs, "max_notifications_per_hour": r.MaxNotificationsPerHour,
			"is_example": r.IsExample,
		}))
	}
	return items, nil
}

func (s *Server) configRSSSave(ctx context.Context, op, id string, cur, next, _ map[string]any) (string, error) {
	switch op {
	case configOpDelete:
		rid, err := strconv.ParseUint(id, 10, 64)
		if err != nil {
			return "", configBad("RSS 的 id 要是数字")
		}
		site, _ := cur["site"].(string)
		if _, err := s.store.DeleteRSSFromSite(site, uint(rid)); err != nil {
			return "", configBad("%v", err)
		}
		return "", nil
	}
	buf, err := json.Marshal(next)
	if err != nil {
		return "", err
	}
	var entry models.RSSConfig
	if err := json.Unmarshal(buf, &entry); err != nil {
		return "", configBad("RSS 的字段格式不对：%v", err)
	}
	site, _ := next["site"].(string)
	if op == configOpAdd {
		if err := configCheckFilterRules(ctx, entry.FilterRuleIDs); err != nil {
			return "", err
		}
		if _, ok := v2.GetGlobalSiteRegistry().Get(site); !ok {
			if _, ok := v2.GetDefinitionRegistry().Get(site); !ok {
				return "", configBad("没有站点 %s（sites 的 name）", site)
			}
		}
		created, err := s.store.AppendRSSToSite(site, entry)
		if err != nil {
			return "", configBad("%v", err)
		}
		return strconv.FormatUint(uint64(created.ID), 10), nil
	}
	if _, err := s.store.UpdateRSSInSite(site, entry); err != nil {
		return "", configBad("%v", err)
	}
	return id, nil
}

// configCheckFilterRules 检查过滤规则都在（新增 RSS 时；修改时 UpdateRSSInSite 自己查）。
func configCheckFilterRules(ctx context.Context, ids []uint) error {
	if len(ids) == 0 || global.GlobalDB == nil {
		return nil
	}
	seen := map[uint]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	var n int64
	if err := global.GlobalDB.DB.WithContext(ctx).Model(&models.FilterRule{}).Where("id IN ?", ids).Count(&n).Error; err != nil {
		return err
	}
	if int(n) != len(seen) {
		return configBad("有的过滤规则不存在（filter_rules 的 id）")
	}
	return nil
}

// ---- 通知通道 ----

func (s *Server) configNotifyChannelsResource() *configResource {
	return &configResource{
		Name: "notify_channels", Title: "通知通道", Kind: configList,
		Path: "/api/chatops/notifications", ItemPath: "/api/chatops/notifications/{id}",
		Desc: "Telegram、QQ、企业微信、钉钉、飞书、Bark、ntfy、Server 酱、Webhook 等通知通道。各通道的配置写成 config.<键>" +
			"（例如 config.default_chat_id）；令牌、密钥、地址这类键只写不读。新增要给 channel_type。",
		Fields:  []string{"name", "enabled", "quiet_hours_start", "quiet_hours_end"},
		AddOnly: []string{"channel_type"}, Prefix: notifyPrefix,
		secretKey: func(field string) bool {
			return strings.HasPrefix(field, notifyPrefix) && notifySecretRe.MatchString(strings.TrimPrefix(field, notifyPrefix))
		},
		Required: []string{"channel_type", "name"},
		Ops:      []string{configOpAdd, configOpUpdate, configOpDelete},
		Hints: map[string]string{
			"channel_type":      "telegram、qq、wecom、dingtalk、feishu、bark、ntfy、serverchan、webhook",
			"quiet_hours_start": "HH:MM，免打扰开始；空着不免打扰", "quiet_hours_end": "HH:MM",
		},
		Impact: []string{"保存以后通道按新配置重新连接；配置不对时这个通道发不出消息"},
		load:   s.configNotifyLoad,
		save:   s.configNotifySave,
		before: func(_ string, _, values map[string]any) error {
			// 也收 {"config": {...}}：摊成 config.<键>
			if v, ok := values["config"]; ok {
				m, ok := v.(map[string]any)
				if !ok {
					return configBad("config 要是对象，或者写成 config.<键>")
				}
				for k, x := range m {
					values[notifyPrefix+k] = x
				}
				delete(values, "config")
			}
			return nil
		},
	}
}

func (s *Server) configNotifyLoad(ctx context.Context) (any, error) {
	body, err := s.configDo(ctx, http.MethodGet, "/api/chatops/notifications", nil)
	if err != nil {
		return nil, err
	}
	var list []map[string]any
	if err := configDecode(body, &list); err != nil {
		return nil, fmt.Errorf("读通知通道失败: %w", err)
	}
	items := make([]map[string]any, 0, len(list))
	for _, it := range list {
		id := configIDText(it["id"])
		detail, err := s.configDo(ctx, http.MethodGet, "/api/chatops/notifications/"+id, nil)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if err := configDecode(detail, &m); err != nil {
			return nil, fmt.Errorf("读通知通道 %s 失败: %w", id, err)
		}
		item := map[string]any{}
		for k, v := range m {
			if k == "config_json" {
				continue
			}
			item[k] = v
		}
		if cfg, ok := m["config_json"].(map[string]any); ok {
			for k, v := range cfg {
				item[notifyPrefix+k] = v
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Server) configNotifySave(ctx context.Context, op, id string, _, next, changed map[string]any) (string, error) {
	cfg := map[string]any{}
	for k, v := range changed {
		if strings.HasPrefix(k, notifyPrefix) {
			cfg[strings.TrimPrefix(k, notifyPrefix)] = v
		}
	}
	switch op {
	case configOpAdd:
		body := map[string]any{"channel_type": next["channel_type"], "name": next["name"], "config_json": cfg, "enabled": next["enabled"] == true}
		out, err := s.configDo(ctx, http.MethodPost, "/api/chatops/notifications", body)
		if err != nil {
			return "", err
		}
		var m map[string]any
		if configDecode(out, &m) == nil {
			return configIDText(m["id"]), nil
		}
		return "", nil
	case configOpUpdate:
		body := map[string]any{}
		for _, k := range []string{"name", "enabled", "quiet_hours_start", "quiet_hours_end"} {
			if v, ok := changed[k]; ok {
				body[k] = v
			}
		}
		if len(cfg) > 0 {
			body["config_json"] = cfg
		}
		_, err := s.configDo(ctx, http.MethodPut, "/api/chatops/notifications/"+id, body)
		return id, err
	case configOpDelete:
		_, err := s.configDo(ctx, http.MethodDelete, "/api/chatops/notifications/"+id, nil)
		return "", err
	}
	return "", configBad("不认识的操作 %s", op)
}
