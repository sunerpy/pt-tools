/*
 * 画板验收用的假数据。
 *
 * 为什么必须有：画板的页脚带、多选条和一部分卡片只有**有数据时**才渲染
 * （`v-if="total > 0"` 之类）。拿一个空库去量，这些东西全都不出现，脚本就会把
 * 「没实现」和「没数据」混成同一件事 —— 那就又变成了一个永远绿灯的验收。
 *
 * 这里只造出「画板上那份示例数据的形状」：条数够撑出分页、状态够覆盖各种胶囊，
 * 数值量级跟画板上写的接近。它不校验后端，后端的正确性归 Go 测试。
 */

const TB = 1024 ** 4;
const SITES = [
  ["M-Team", "Crazy User", 38.4, 4.2, 9.14, 286, 12.4, 1284900, 2140],
  ["HDSky", "VeteranUser", 52.1, 6.8, 7.66, 412, 20.8, 2043110, 3880],
  ["OurBits", "Elite User", 21.6, 3.1, 6.97, 198, 8.6, 742300, 1190],
  ["PTerClub", "Power User", 14.2, 2.6, 5.46, 141, 6.2, 486020, 910],
  ["CHDBits", "Insane User", 33.8, 5.4, 6.26, 267, 11.9, 1102540, 1760],
  ["TTG", "Extreme User", 9.4, 1.8, 5.22, 86, 3.4, 271880, 540],
  ["LemonHD", "Elite User", 6.8, 2.4, 2.83, 64, 2.1, 158640, 330],
  ["HHanClub", "Power User", 4.2, 1.9, 2.21, 48, 1.6, 96470, 240],
];

const AGGREGATED = {
  totalUploaded: 412.6 * TB,
  totalDownloaded: 78.3 * TB,
  averageRatio: 8.42,
  totalSeeding: 1555,
  totalLeeching: 12,
  totalBonus: 6284000,
  siteCount: 14,
  lastUpdate: 1758000000,
  totalBonusPerHour: 11150,
  totalSeedingBonus: 982000,
  totalUnreadMessages: 3,
  totalSeederSize: 68.9 * TB,
  perSiteStats: SITES.map(([site, rank, up, dn, ratio, seed, seedTB, bonus, bph], i) => ({
    site,
    username: "qa-user",
    userId: String(i),
    uploaded: up * TB,
    downloaded: dn * TB,
    ratio,
    bonus,
    seeding: seed,
    leeching: 0,
    rank,
    levelName: rank,
    levelId: 4,
    lastUpdate: 1758000000,
    bonusPerHour: bph,
    seedingBonus: bonus,
    unreadMessageCount: i === 1 ? 128 : i === 5 ? 3 : 0,
    seederCount: seed,
    seederSize: seedTB * TB,
  })),
};

/** `/api/sites` 回的是「站点名 → 配置」的对象，不是数组 */
const SITE_CONFIGS = Object.fromEntries(
  SITES.map(([site], i) => [
    site,
    {
      enabled: i !== 6,
      auth_method: ["cookie", "api_key", "cookie_and_api_key", "passkey"][i % 4],
      cookie: "",
      has_cookie: i % 4 !== 1,
      api_key: "",
      api_url: "",
      passkey: "",
      upload_limit_kbs: 0,
      download_limit_kbs: 0,
      seeding_capacity_gb: i * 100,
      unavailable: i === 7,
      unavailable_reason: i === 7 ? "站点暂时关闭注册与访问" : "",
      urls: [`https://${site.toLowerCase()}.invalid`],
      rss:
        i % 3 === 2
          ? []
          : [
              {
                id: i * 10 + 1,
                name: `${site} 电影`,
                url: `https://${site.toLowerCase()}.invalid/rss?passkey=x`,
                category: "Mv",
                tag: site,
                interval_minutes: 10,
              },
            ],
    },
  ]),
);

const TASKS = Array.from({ length: 12 }, (_, i) => ({
  id: i + 1,
  siteName: SITES[i % SITES.length][0],
  title: `Some.Release.Name.S0${(i % 4) + 1}E0${(i % 9) + 1}.2160p.WEB-DL.DDP5.1.HDR.H265-GROUP`,
  torrentHash: `hash${String(i).padStart(36, "0")}`,
  sizeBytes: (2 + i) * 1024 ** 3,
  isDownloaded: i % 3 !== 2,
  isPushed: i % 3 === 0,
  isExpired: i % 5 === 4,
  freeEndTime: i % 2 === 0 ? "2026-09-30T12:00:00Z" : "",
  createdAt: "2026-09-18T10:00:00Z",
  updatedAt: "2026-09-19T01:00:00Z",
}));

const PAUSED = Array.from({ length: 6 }, (_, i) => ({
  id: i + 1,
  site_name: SITES[i % SITES.length][0],
  title: `Paused.Title.${i + 1}.1080p.BluRay.REMUX`,
  torrent_hash: `paused${String(i).padStart(34, "0")}`,
  size_bytes: (10 + i) * 1024 ** 3,
  progress: 0.2 + i * 0.1,
  downloader_name: "qb-main",
  pause_reason: "免费期结束",
  paused_at: "2026-09-18T22:10:00Z",
  state: "paused",
}));

const ARCHIVE = Array.from({ length: 4 }, (_, i) => ({
  id: i + 100,
  site_name: SITES[i % SITES.length][0],
  title: `Archived.Title.${i + 1}.2160p`,
  torrent_hash: `arch${String(i).padStart(36, "0")}`,
  size_bytes: (8 + i) * 1024 ** 3,
  progress: 1,
  downloader_name: "qb-main",
  pause_reason: "已处理",
  status: "deleted",
  archived_at: "2026-09-17T08:00:00Z",
}));

const AUDIT = Array.from({ length: 10 }, (_, i) => ({
  id: i + 1,
  created_at: "2026-09-18T20:00:00Z",
  channel_type: ["telegram", "qq_onebot", "webhook"][i % 3],
  channel_user_id: `u${i}`,
  command: ["/status", "/sites", "/push", "/pause"][i % 4],
  result: ["success", "success", "denied", "error"][i % 4],
  latency_ms: 12 + i * 3,
  args_json: i % 2 === 0 ? '{"site":"M-Team"}' : "",
}));

const RSS_LOGS = Array.from({ length: 10 }, (_, i) => ({
  id: i + 1,
  rss_id: (i % 3) + 1,
  site_name: SITES[i % SITES.length][0],
  torrent_id: `t${1000 + i}`,
  notify_kind: i % 2 === 0 ? "filtered" : "all",
  notification_conf_id: (i % 2) + 1,
  result: ["sent", "failed", "pending", "suppressed"][i % 4],
  attempts: (i % 3) + 1,
  last_error: i % 4 === 1 ? "HTTP 502 Bad Gateway" : "",
  next_retry_at: i % 4 === 2 ? "2026-09-19T02:00:00Z" : "",
  delivered_at: i % 4 === 0 ? "2026-09-18T23:00:00Z" : "",
  payload_json: '{"title":"示例通知"}',
  created_at: "2026-09-18T22:00:00Z",
}));

const FILTER_RULES = Array.from({ length: 6 }, (_, i) => ({
  id: i + 1,
  name: ["4K 资源", "1080p 资源", "REMUX", "HDR", "HEVC", "国语配音"][i],
  pattern: ["4K|2160p", "1080p", "*REMUX*", "HDR", "HEVC|x265", "国语|国配"][i],
  pattern_type: ["regex", "keyword", "wildcard", "keyword", "regex", "regex"][i],
  match_field: "both",
  require_free: i % 2 === 0,
  min_size_gb: 0,
  max_size_gb: i === 0 ? 120 : 0,
  enabled: i !== 4,
  priority: 100 + i * 10,
  purpose: "download",
}));

const SEARCH_RESULTS = Array.from({ length: 10 }, (_, i) => ({
  id: `s${i}`,
  title: `Search.Hit.${i + 1}.2160p.WEB-DL.H265`,
  subtitle: "示例副标题",
  sourceSite: SITES[i % SITES.length][0],
  url: "https://example.invalid/t/1",
  downloadUrl: "https://example.invalid/d/1",
  sizeBytes: (4 + i) * 1024 ** 3,
  seeders: 20 + i,
  leechers: i,
  snatched: 50 + i * 3,
  uploadedAt: 1758000000,
  /*
   * 免费行与「没有分类」的行**必须有交集**：没有分类的是 i % 3 === 1，
   * 原来免费只给 i % 3 === 0，两组互不相交 —— 于是「分类 + 仅免费 + 一并显示」这个组合
   * 在假数据里永远走不到，一条报错的计数也测不出来（评审正是从这里查出计数会说谎）。
   * i % 6 === 1 让每两条没有分类的行里有一条是免费的。
   */
  isFree: i % 3 === 0 || i % 6 === 1,
  discountLevel: i % 3 === 0 || i % 6 === 1 ? "FREE" : "",
  discountEndTime: 0,
  hasHR: i % 5 === 0,
  /*
   * 用驱动**真实**会回的分类名，不是造出来刚好能命中的「电影 / 剧集 / 动漫」：
   * 前者来自 mteamCategoryMap 与 HDDolby 的 getCategoryName，
   * 里面既有漏收过的（影剧/综艺、Animation）也有误收过的（TV游戏）。
   */
  category:
    i % 3 === 1
      ? ""
      : [
          "电影/HD",
          "影剧/综艺/HD",
          "动画",
          "Movies/UHD",
          "TV/HD",
          "Animation",
          "音乐(无损)",
          "TV游戏",
          "纪录片",
        ][i % 9],
  /*
   * 每三条里有一条把分类留空、只给标签 —— Gazelle 的搜索响应就是这个形状
   * （没有分类字段，内容类型在 group 的 tags 上）。
   *
   * 标签用 MooKo 样本里的真实值（site/v2/definitions/mooko_fixture_test.go 的
   * mookoSearchFixture）：Gazelle 的 tags 是**题材词**，一个都归不进画板那五个桶。
   * 这里原来写的是 ["movie","1990s"]，那是造出来刚好能命中的形状，
   * 正好掩盖了「选具体档位时这些行会被整片筛掉」这个缺陷 —— 本文件开头反对的就是这种假数据。
   */
  tags: i % 3 === 1 ? ["剧情", "悬疑", "传记"] : [],
}));

const SUPPORTED = SITES.map(([site], i) => ({
  id: site.toLowerCase(),
  name: site,
  aka: i % 3 === 0 ? [`${site} 别名`] : [],
  schema: ["NexusPHP", "M-Team", "Unit3D", "Gazelle"][i % 4],
  description: "示例站点定义，用于画板验收。",
  urls: [`https://${site.toLowerCase()}.invalid`],
  /*
   * 字段名与真实响应对齐：后端是单数 `authMethod` 且 omitempty ——
   * 实测 /api/sites/definitions 里多数条目根本没有这个字段。
   * 这里也照这个形状造：三条有、一条没有，好让「没声明认证方式」这条路径进验收。
   */
  authMethod: i % 4 === 3 ? undefined : ["cookie", "api_key", "cookie_and_api_key"][i % 3],
  features: ["rss", "search"],
  unavailable: i === 7,
  unavailableReason: i === 7 ? "站点暂时关闭" : "",
}));

const CHANNELS = [
  {
    id: 1,
    name: "主 Telegram",
    channel_type: "telegram",
    enabled: true,
    config_json: "",
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-18T00:00:00Z",
  },
  {
    id: 2,
    name: "运维群",
    channel_type: "qq_onebot",
    enabled: true,
    config_json: "",
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-18T00:00:00Z",
  },
  {
    id: 3,
    name: "告警 Webhook",
    channel_type: "webhook",
    enabled: false,
    config_json: "",
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-18T00:00:00Z",
  },
  {
    id: 4,
    name: "企业微信",
    channel_type: "wecom_webhook",
    enabled: true,
    config_json: "",
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-18T00:00:00Z",
  },
];

const BINDINGS = [
  {
    id: 1,
    channel_user_id: "123456",
    channel_type: "telegram",
    label: "我自己",
    admin: true,
    allowed: true,
    bind_code: "",
    bound_at: "2026-09-10T00:00:00Z",
  },
  {
    id: 2,
    channel_user_id: "",
    channel_type: "qq_onebot",
    label: "",
    admin: false,
    allowed: false,
    bind_code: "K3F9Q2",
    expires_at: "2026-09-19T12:00:00Z",
  },
];

const DOWNLOADERS = [
  {
    id: 1,
    name: "qb-main",
    type: "qbittorrent",
    url: "http://127.0.0.1:8080",
    username: "admin",
    enabled: true,
    is_default: true,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-18T00:00:00Z",
  },
  {
    id: 2,
    name: "tr-backup",
    type: "transmission",
    url: "http://127.0.0.1:9091",
    username: "admin",
    enabled: true,
    is_default: false,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-18T00:00:00Z",
  },
];

const HUB_TORRENTS = Array.from({ length: 12 }, (_, i) => ({
  hash: `hub${String(i).padStart(37, "0")}`,
  downloader_id: (i % 2) + 1,
  downloader_name: i % 2 === 0 ? "qb-main" : "tr-backup",
  title: `Hub.Task.${i + 1}.2160p.WEB-DL`,
  size: (5 + i) * 1024 ** 3,
  progress: i % 4 === 0 ? 1 : 0.1 * (i % 9),
  state: ["downloading", "seeding", "paused", "error"][i % 4],
  upload_speed: i * 120_000,
  download_speed: i % 4 === 0 ? 0 : i * 350_000,
  seeds: 3 + i,
  peers: i,
  ratio: 1 + i * 0.3,
  eta: 600 + i * 30,
  category: ["电影", "剧集", ""][i % 3],
  tags: i % 2 === 0 ? "MT" : "",
  save_path: "/downloads",
  added_at: 1758000000,
  completed_at: i % 4 === 0 ? 1758003600 : 0,
}));

/** 路由（去掉查询串）→ 响应体。第一个前缀命中即用。 */
export const FIXTURES = [
  ["/api/v2/userinfo/aggregated", AGGREGATED],
  /* 单站点详情要排在列表之前：前缀匹配第一个命中即用 */
  ...AGGREGATED.perSiteStats.map((row) => [`/api/v2/userinfo/sites/${row.site}`, row]),
  /* getSites 回的是 UserInfoResponse[]，不是站点名数组 */
  ["/api/v2/userinfo/sites", AGGREGATED.perSiteStats],
  /* getRegisteredSites 回的是 { sites: string[] } */
  ["/api/v2/userinfo/registered", { sites: SITES.map(([s]) => s) }],
  ["/api/sites/definitions", SUPPORTED],
  /* listLoginStates 回的是 SiteLoginState[] */
  [
    "/api/sites/login-state",
    SITES.map(([site], i) => ({
      site_name: site,
      state: ["ok", "ok", "expiring", "expired"][i % 4],
      probe_mode: "auto",
      /* last_probe_at 是 unix 秒，不是 ISO 串 —— 给成串会让页面印 Invalid Date */
      last_probe_at: 1758200000 - i * 3600,
      last_probe_status: i % 4 === 3 ? "fail" : "ok",
      last_probe_error: i % 4 === 3 ? "302 → /login" : "",
      consecutive_probe_failures: i % 4 === 3 ? 2 : 0,
      ban_threshold_days: 30,
      remind_before_days: 7,
      reminder_cron: "0 9 * * *",
      notification_channel_ids: [1],
      last_reminder_tier: "none",
      days_remaining: [30, 21, 5, 0][i % 4],
      tier: ["ok", "ok", "warn", "dang"][i % 4],
    })),
  ],
  [
    "/api/sites/downloader-summary",
    {
      sites: SITES.map(([site], i) => ({
        site_id: i + 1,
        site_name: site,
        display_name: site,
        downloader_id: i % 3 === 2 ? undefined : (i % 2) + 1,
        downloader_name: i % 3 === 2 ? "" : i % 2 === 0 ? "qb-main" : "tr-backup",
      })),
    },
  ],
  /* 站点详情取的是单个站点：必须排在 /api/sites 之前，前缀匹配是第一个命中即用 */
  ...SITES.map(([site]) => [`/api/sites/${site}`, SITE_CONFIGS[site]]),
  ["/api/sites", SITE_CONFIGS],
  /* 必须排在 /api/tasks 之前：前缀匹配是第一个命中即用 */
  [
    "/api/tasks/stats",
    {
      total: 37,
      active: 25,
      pushedToday: 6,
      free: 12,
      daily: Array.from({ length: 7 }, (_, i) => ({
        date: `2026-09-${String(13 + i).padStart(2, "0")}`,
        created: 4 + ((i * 3) % 7),
        pushed: 2 + (i % 5),
        free: 1 + (i % 4),
      })),
    },
  ],
  ["/api/tasks", { items: TASKS, total: 37, page: 1, page_size: 20 }],
  ["/api/torrents/paused", { items: PAUSED, total: 6, page: 1, page_size: 20 }],
  ["/api/torrents/archive", { items: ARCHIVE, total: 4, page: 1, page_size: 20 }],
  ["/api/chatops/audit/stats", { todayCount: 42, successRate: 0.93, maxLatencyMs: 860 }],
  ["/api/chatops/audit", { items: AUDIT, total: 128, page: 1, page_size: 20 }],
  ["/api/chatops/rss-notifications", { items: RSS_LOGS, total: 64, page: 1, page_size: 20 }],
  /*
   * 通道详情必须排在列表**之前**：匹配是前缀匹配、先命中先返回，
   * 否则 `/api/chatops/notifications/1` 会拿到列表那个数组，详情页把数组当对象读，
   * 画出来就是「未命名通道 / ID -」—— 画板 23 的内容等于没进验收。
   */
  ["/api/chatops/notifications/1", CHANNELS[0]],
  ["/api/chatops/notifications", CHANNELS],
  ["/api/chatops/bindings", BINDINGS],
  /* 必须排在 /api/filter-rules 之前：前缀匹配第一个命中即用 */
  ["/api/filter-rules/hits", { hits: { 1: 128, 2: 46, 3: 12, 4: 3, 6: 1 } }],
  ["/api/filter-rules", FILTER_RULES],
  [
    "/api/v2/search/multi",
    {
      items: SEARCH_RESULTS,
      siteResults: Object.fromEntries(SITES.slice(0, 6).map(([s], i) => [s, 2 + i])),
      errors: [{ site: "TTG", error: "请求超时" }],
      durationMs: 1240,
      totalResults: 128,
    },
  ],
  ["/api/v2/search/sites", SITES.map(([s]) => s)],
  ["/api/v2/sites/supported", SITES.map(([s2]) => s2)],
  ["/api/v2/sites/categories", {}],
  ["/api/v2/sites/levels", {}],
  ["/api/downloaders/all-directories", { 1: [{ path: "/downloads", alias: "默认" }], 2: [] }],
  ["/api/downloaders", DOWNLOADERS],
  [
    "/api/downloader-torrents/transfer-stats",
    {
      total_upload_speed: 28_300_000,
      total_download_speed: 98_400_000,
      total_uploaded: 412 * TB,
      total_downloaded: 78 * TB,
      total_session_uploaded: 9 * 1024 ** 3,
      total_session_downloaded: 22 * 1024 ** 3,
      total_free_space: 1.8 * TB,
      /*
       * 逐台明细。**必须有**：状态栏右端那格（画板 statusbar 的「名称 · 连接态」）
       * 就靠这里的 reachable 判连接态，数组缺了的话它永远显示「未知」，
       * 那一格等于没进验收。第二台故意给不可达，覆盖「取到实例但连不上」这条路径。
       */
      downloaders: DOWNLOADERS.map((d, i) => ({
        downloader_id: d.id,
        downloader_name: d.name,
        downloader_type: d.type,
        upload_speed: i === 0 ? 27_000_000 : 1_300_000,
        download_speed: i === 0 ? 93_800_000 : 4_600_000,
        uploaded: i === 0 ? 380 * TB : 32 * TB,
        downloaded: i === 0 ? 62 * TB : 16 * TB,
        session_uploaded: 8 * 1024 ** 3,
        session_downloaded: 20 * 1024 ** 3,
        free_space: i === 0 ? 1.6 * TB : 0.2 * TB,
        reachable: i === 0,
        error: i === 0 ? undefined : "dial tcp 10.0.0.9:9091: connect: connection refused",
      })),
    },
  ],
  /*
   * 版本检查 —— 画板 42 的六个 popover 状态。空库下这一串接口回的是「没检查过」，
   * 于是画板 42 里最主要的那个状态（hasUpdate · updates-list）根本走不到。
   * 这里给一个「有更新」的响应，让状态栏那颗更新点与更新列表进验收。
   */
  ["/api/version/runtime", { is_docker: false, platform: "linux", arch: "amd64" }],
  ["/api/version/upgrade", { status: "idle", percent: 0, message: "" }],
  [
    "/api/version/check",
    {
      current_version: "v0.47.2",
      has_update: true,
      changelog_url: "https://github.com/sunerpy/pt-tools/releases",
      has_more_releases: false,
      checked_at: 1758200000,
      new_releases: [
        {
          version: "v0.48.0",
          name: "v0.48.0",
          changelog: "- 画板验收补移动端\n- 修下载器连接态误报",
          url: "https://github.com/sunerpy/pt-tools/releases/tag/v0.48.0",
          published_at: 1758100000,
          assets: [
            {
              name: "pt-tools_linux_amd64",
              download_url: "https://example.invalid/a",
              size: 45_000_000,
            },
          ],
        },
      ],
    },
  ],
  [
    "/api/version",
    { version: "v0.47.2", build_time: "2026-09-18T00:00:00Z", commit_id: "b31f660" },
  ],
  ["/api/downloader-torrents/meta", { categories: ["电影", "剧集"], tags: ["MT", "HDS"] }],
  [
    "/api/downloader-torrents/capabilities",
    DOWNLOADERS.map((d) => ({
      downloader_id: d.id,
      downloader_name: d.name,
      type: d.type,
      set_location: true,
      set_category: d.type === "qbittorrent",
      set_tags: d.type === "qbittorrent",
      force_start: d.type === "qbittorrent",
    })),
  ],
  [
    "/api/downloader-torrents",
    { items: HUB_TORRENTS, total: 12, page: 1, page_size: 100, failures: [] },
  ],
  /* 必须排在 /api/logs 之前 */
  [
    "/api/logs/files",
    {
      dir: "/config/.pt-tools/logs",
      files: [
        { name: "all.log", size: 2400000, mod_time: 1758200000, rotated: false, is_active: true },
        {
          name: "all-2026-09-17T03-00-00.000.log",
          size: 10485760,
          mod_time: 1758100000,
          rotated: true,
          is_active: false,
        },
        {
          name: "all-2026-09-15T03-00-00.000.log",
          size: 10485760,
          mod_time: 1757900000,
          rotated: true,
          is_active: false,
        },
      ],
      max_age: 30,
      max_backups: 10,
    },
  ],
  [
    "/api/logs",
    {
      lines: Array.from(
        { length: 120 },
        (_, i) => `2026-09-19 01:0${i % 10}:00 INFO  示例日志第 ${i + 1} 行`,
      ),
      /* 与 /api/logs/files 里 is_active 的那个文件保持一致：两处不一样时页面上会出现
         「顶栏说 all.log、工具栏说 app.log」这种自相矛盾的画面 */
      path: "/config/.pt-tools/logs/all.log",
      truncated: false,
    },
  ],
];

/** 注入页面的脚本：拦 fetch，命中前缀就回假数据，其余照常走后端 */
export function stubScript() {
  return `(() => {
  /* 无头 Chrome 里 document.hidden 恒为 true，靠可见性判断的定时刷新不会跑 */
  Object.defineProperty(document, 'hidden', { get: () => false, configurable: true });
  Object.defineProperty(document, 'visibilityState', { get: () => 'visible', configurable: true });
  const table = ${JSON.stringify(FIXTURES.map(([prefix, body]) => [prefix, JSON.stringify(body)]))};
  /* 给验收脚本一个可探测的标记：它靠这个判断这一篇文档到底有没有铺上假数据 */
  window.__ptStub = true;
  const real = window.fetch;
  window.fetch = (input, init) => {
    const raw = typeof input === 'string' ? input : input.url;
    const path = raw.split('?')[0].replace(location.origin, '');
    const method = (init && init.method ? init.method : 'GET').toUpperCase();
    /* 搜索是 POST，所以不能只拦 GET；写操作（PUT/DELETE）照常打到后端 */
    if (method === 'GET' || method === 'POST') {
      for (const [prefix, body] of table) {
        if (path === prefix || path.startsWith(prefix + '/')) {
          return Promise.resolve(new Response(body, {
            status: 200, headers: { 'Content-Type': 'application/json' },
          }));
        }
      }
    }
    return real(input, init);
  };
  return 'stubbed';
})()`;
}
