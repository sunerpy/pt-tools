#!/usr/bin/env node
/*
 * 文档站截图 —— 首页 hero 与「站点」分屏用的那几张 docs/public/screens/*.webp。
 *
 * 画面来自真实的 Web UI，数据来自 board-fixtures.mjs 的验收假数据：截图里不会出现
 * 真实站点账号、Cookie 或下载器地址，而且每次重拍得到的是同一份数据。
 *
 *   home-{light,dark}.webp         /userinfo   1440 × 900
 *   sites-{light,dark}.webp        /sites      1440 × 900
 *   home-mobile-{light,dark}.webp  /userinfo   375 × 812（移动端布局）
 *
 * 首页（2026-11 版式）的两步演示与 showcase 只截页面的一块、按 2 倍或 1.5 倍像素截：图在首页上只有
 * 500–640 px 宽，整屏 1440 宽的截图缩到那么小，字就看不清了。
 *   media-follow-{light,dark}.webp   /media/item/tv/100088  640 宽的窄屏布局，条目头部到「你的 NAS 上」的追剧那一行
 *                                                           （版本锁），下面的媒体服务器一行与播放按钮不截
 *   media-history-{light,dark}.webp  /media/history         640 宽的窄屏布局，整理历史的面板，和上一张一样高
 *   brush-{light,dark}.webp          /brush                 1440 宽，去掉导航列与状态条，到页面内容的底
 *                                                           （窄于 1181 时 KPI 带退成 3 列、任务表挤在一起）
 *   assistant-{light,dark}.webp      /assistant             1100 × 640，只截对话那一栏
 * 实际像素宽高每次运行都会打印出来，改了截取区域要同步改首页 frontmatter 里这几张图的 width、height。
 *
 * 另有 ChatOps 指南的配图（docs/guide/images/chatops/*.webp，明亮主题 1440 × 900）：通道列表、
 * 添加通道弹窗、Telegram 与 QQ 通道的凭证、绑定列表与生成绑定码弹窗、操作审计、RSS 通知日志。
 * 凭证里的 Token、用户 ID 都是示例值（见下方 GUIDE_STUBS），不是真实账号。
 *
 * 截图里的版本号取 .release-please-manifest.json 的当前版本，并显示「已是最新」：验收假数据故意给的是
 * 「有新版本」（画板 42 的验收状态），那个状态不该出现在文档截图里。
 *
 * 用法（需要一个已经跑起来的 pt-tools 服务，前端 dist 必须是当前源码构建出来的；
 * 用一个临时 HOME 起服务，首次启动的初始账号就能登录）：
 *   HOME=$(mktemp -d) ./pt-tools web --host 127.0.0.1 --port 18310 &
 *   node scripts/docs-screens.mjs http://127.0.0.1:18310 [输出目录，默认为仓库的 docs/public/screens]
 * 指南配图的输出目录默认是 docs/guide/images/chatops，可用 PT_GUIDE_DIR 覆盖。
 * 只重拍其中几张：PT_SHOTS=media-follow,media-history（按文件名前缀，逗号分隔；设了就不拍指南配图）。
 *
 * 换了截图尺寸要同步改 docs/index.md 与 docs/en/index.md 里 home.visual / home.shots 的
 * width、height。提交前逐张看一遍图。
 *
 * 依赖：只用 Chrome 的 CDP，不装任何浏览器自动化包。Chrome 路径可用 PT_CHROME 覆盖；
 * 登录账号可用 PT_USER / PT_PASS 覆盖。
 */
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { FIXTURES, stubScript } from "./board-fixtures.mjs";

const CHROME =
  process.env.PT_CHROME ?? "/config/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome";
const USER = process.env.PT_USER ?? "admin";
const PASS = process.env.PT_PASS ?? "adminadmin";
const PORT = Number(process.env.CDP_PORT || 19593);
const ONLY = process.env.PT_SHOTS ? process.env.PT_SHOTS.split(",").map((s) => s.trim()) : null;

const here = dirname(fileURLToPath(import.meta.url));
const [base, outArg] = process.argv.slice(2);
if (!base) {
  console.error("用法：node scripts/docs-screens.mjs <pt-tools 地址> [输出目录]");
  process.exit(2);
}
if (!existsSync(CHROME)) {
  console.error(`找不到 Chrome：${CHROME}\n用 PT_CHROME 指定路径。`);
  process.exit(2);
}
const outDir = resolve(outArg ?? join(here, "../../../docs/public/screens"));
mkdirSync(outDir, { recursive: true });
const guideDir = resolve(
  process.env.PT_GUIDE_DIR ?? join(here, "../../../docs/guide/images/chatops"),
);
mkdirSync(guideDir, { recursive: true });

const fixture = (prefix) => FIXTURES.find(([p]) => p === prefix)?.[1];
const release = `v${JSON.parse(readFileSync(join(here, "../../../.release-please-manifest.json"), "utf8"))["."]}`;
const VERSION_STUBS = {
  "/api/version": { ...fixture("/api/version"), version: release },
  "/api/version/check": {
    ...fixture("/api/version/check"),
    current_version: release,
    has_update: false,
    new_releases: [],
  },
};
/* 通道详情的凭证：验收假数据的 config_json 是空的，截图要看得出每一栏填什么，所以给示例值 */
const channels = fixture("/api/chatops/notifications");
const GUIDE_STUBS = {
  "/api/chatops/notifications/1": {
    ...channels[0],
    config_json: {
      bot_token: "123456789:AAEexampleTokenForDocsOnly",
      admin_users: [123456789],
      allowed_users: [123456789, 987654321],
      default_chat_id: 123456789,
      proxy_url: "",
    },
  },
  "/api/chatops/notifications/2": {
    ...channels[1],
    config_json: {
      listen_addr: "0.0.0.0:6701",
      access_token: "docs-example-token",
      admin_qq_users: [12345678],
      allowed_qq_users: [12345678, 87654321],
    },
  },
  /*
   * 绑定列表按真实接口的形状给（web/api_chatops.go listBindings：{ bindings, pending }）。
   * 验收假数据这里是一个平铺数组，页面读不到两个字段，绑定页在验收里一直是空态。
   * 待绑定码的到期时间按截图当时往后算，倒计时才不会显示「已过期」。
   */
  "/api/chatops/bindings": {
    bindings: [
      {
        id: 1,
        conf_id: 1,
        channel_type: "telegram",
        channel_user_id: "123456789",
        label: "我的 Telegram",
        reply_lang: "zh",
        admin: true,
        allowed: true,
        created_at: "2026-09-10T08:00:00Z",
        last_active: "2026-09-18T21:30:00Z",
      },
      {
        id: 2,
        conf_id: 2,
        channel_type: "qq_onebot",
        channel_user_id: "12345678",
        label: "我的 QQ",
        reply_lang: "zh",
        admin: true,
        allowed: true,
        created_at: "2026-09-12T09:00:00Z",
        last_active: "2026-09-18T20:10:00Z",
      },
    ],
    pending: [
      {
        code: "K3F9Q2XA",
        conf_id: 2,
        label: "家里的 QQ",
        expires_at: new Date(Date.now() + 4.5 * 60_000).toISOString(),
        created_at: new Date(Date.now() - 30_000).toISOString(),
      },
    ],
  },
  /*
   * 审计统计同理：接口是 AuditStatsDTO 的蛇形字段，success_rate 是 0–100 的百分比
   * （internal/app/audit_service.go）；验收假数据用的驼峰字段页面读不到，页头一直是 0。
   * 数字与审计列表的 10 条假记录对得上：今天 5 条、6 条成功、最慢 39 ms。
   */
  "/api/chatops/audit/stats": {
    today_count: 5,
    total_count: 10,
    success_rate: 60,
    max_latency_ms: 39,
    avg_latency_ms: 25.5,
  },
};

/*
 * 首页（2026-11 版式）另外几张图的假数据，只在这里用、不进验收假数据：
 *   - 两步演示：条目详情（追剧订阅，用验收假数据里已有的「长夜将尽」）→ 整理记录（这部剧新的一集 S02E07 整理进剧集库）；
 *   - showcase：刷流任务、AI 助手的一段对话。
 * 时间按截图当时往前推，页面上的「几分钟前」「今天」才对得上；数字都是编的，站点名用验收假数据里有的。
 */
const GiB = 1024 ** 3;
const ago = (min) => new Date(Date.now() - min * 60_000).toISOString();
/*
 * 整理历史按「最近一个 21:52」往前推：第 7 集是晚上播出、当晚整理进库的，什么时候重拍都是这个样子，
 * 和 App 截图里「站点数据更新于」的时间也对得上（那边写死在截图测试里）。
 */
const EVENING = (() => {
  const d = new Date();
  d.setHours(21, 52, 0, 0);
  if (d.getTime() > Date.now()) d.setDate(d.getDate() - 1);
  return d.getTime();
})();
const evening = (min) => new Date(EVENING - min * 60_000).toISOString();
const ymd = (d) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
const historyRow = (over) => ({
  id: 1,
  downloader_id: 1,
  downloader_name: "qb-main",
  info_hash: "d4e5f60718293a4b5c6d7e8f9012345678901a2b",
  task_id: "d4e5f60718293a4b5c6d7e8f9012345678901a2b",
  torrent_name: "The.Long.Night.S02E07.2160p.WEB-DL.H265-HHWEB",
  library_id: 2,
  library_name: "剧集库",
  source_path: "/downloads/tv/The.Long.Night.S02E07.2160p.WEB-DL.H265-HHWEB.mkv",
  target_path: "/media/剧集/长夜将尽 (2025)/Season 02/长夜将尽 - S02E07 - 失踪者.mkv",
  mode: "hardlink",
  chosen_mode: "",
  media_type: "tv",
  tmdb_id: 100088,
  title: "长夜将尽",
  year: 2025,
  season: 2,
  episode: 7,
  episode_end: 0,
  size: 7.2 * GiB,
  status: "done",
  message: "",
  fail_kind: "",
  attempts: 0,
  trigger: "auto",
  match_source: "subscription",
  subtitles: 1,
  has_files: true,
  scrape_status: "ok",
  scrape_note: "",
  scraped_at: evening(6),
  created_at: evening(6),
  updated_at: evening(6),
  ...over,
});
const HISTORY_ROWS = [
  historyRow({ id: 46 }),
  historyRow({
    id: 45,
    downloader_id: 2,
    downloader_name: "tr-backup",
    torrent_name: "The.Long.Night.S02E05-E06.2160p.WEB-DL.H265-HHWEB",
    source_path:
      "/downloads/tv/The.Long.Night.S02E05-E06.2160p.WEB-DL.H265-HHWEB/The.Long.Night.S02E06.2160p.WEB-DL.H265-HHWEB.mkv",
    target_path: "/media/剧集/长夜将尽 (2025)/Season 02/长夜将尽 - S02E06 - 暗涌.mkv",
    episode: 6,
    size: 6.8 * GiB,
    created_at: evening(5 * 1440 + 40),
    updated_at: evening(5 * 1440 + 40),
    scraped_at: evening(5 * 1440 + 40),
  }),
  historyRow({
    id: 44,
    downloader_id: 2,
    downloader_name: "tr-backup",
    torrent_name: "The.Long.Night.S02E05-E06.2160p.WEB-DL.H265-HHWEB",
    source_path:
      "/downloads/tv/The.Long.Night.S02E05-E06.2160p.WEB-DL.H265-HHWEB/The.Long.Night.S02E05.2160p.WEB-DL.H265-HHWEB.mkv",
    target_path: "/media/剧集/长夜将尽 (2025)/Season 02/长夜将尽 - S02E05 - 回声.mkv",
    episode: 5,
    size: 6.8 * GiB,
    created_at: evening(5 * 1440 + 41),
    updated_at: evening(5 * 1440 + 41),
    scraped_at: evening(5 * 1440 + 41),
  }),
  historyRow({
    id: 43,
    info_hash: "e5f60718293a4b5c6d7e8f9012345678901a2b3c",
    task_id: "e5f60718293a4b5c6d7e8f9012345678901a2b3c",
    torrent_name: "Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX.DV.HDR.HEVC.TrueHD.7.1-FRDS",
    library_id: 1,
    library_name: "电影库",
    source_path:
      "/downloads/movie/Dune.Part.Two.2024.2160p.UHD.BluRay.REMUX-FRDS/Dune.Part.Two.mkv",
    target_path: "/media/电影/沙丘2 (2024)/沙丘2 (2024) - 2160p.mkv",
    media_type: "movie",
    tmdb_id: 693134,
    title: "沙丘2",
    year: 2024,
    season: 0,
    episode: 0,
    size: 76.4 * GiB,
    subtitles: 2,
    match_source: "",
    created_at: evening(8 * 1440 + 200),
    updated_at: evening(8 * 1440 + 200),
    scraped_at: evening(8 * 1440 + 200),
  }),
  ...[4, 3, 2, 1].map((ep, i) =>
    historyRow({
      id: 42 - i,
      downloader_name: "qb-main",
      info_hash: "b2c3d4e5f60718293a4b5c6d7e8f901234567890",
      task_id: "b2c3d4e5f60718293a4b5c6d7e8f901234567890",
      torrent_name: "The.Long.Night.S02E01-E04.2160p.WEB-DL.H265-HHWEB",
      source_path: `/downloads/tv/The.Long.Night.S02E01-E04.2160p.WEB-DL.H265-HHWEB/E0${ep}.mkv`,
      target_path: `/media/剧集/长夜将尽 (2025)/Season 02/长夜将尽 - S02E0${ep} - ${["归来", "旧案", "雨夜", "证人"][ep - 1]}.mkv`,
      episode: ep,
      size: 6.8 * GiB,
      created_at: evening(18 * 1440 + i),
      updated_at: evening(18 * 1440 + i),
      scraped_at: evening(18 * 1440 + i),
    }),
  ),
];

const brushTask = (over) => ({
  name: "",
  enabled: true,
  site_name: "",
  downloader_id: 1,
  save_path: "/downloads/brush",
  category: "brush",
  tags: "pt-tools-brush",
  interval_min: 10,
  section: "",
  discounts: "FREE,2XFREE",
  min_free_remain_min: 60,
  min_size_gb: 1,
  max_size_gb: 80,
  max_seeders: 20,
  min_leechers: 5,
  max_publish_age_min: 120,
  exclude_hr: true,
  include_keywords: "",
  exclude_keywords: "",
  max_downloading: 3,
  max_total_size_gb: 500,
  max_daily_download_gb: 300,
  remove_seed_time_h: 48,
  remove_ratio: 3,
  remove_low_speed_kbs: 50,
  remove_low_speed_window_min: 30,
  remove_inactive_h: 12,
  remove_free_expired_incomplete: true,
  remove_with_data: true,
  id: 1,
  last_error: "",
  last_result: "",
  last_run_at: ago(4),
  created_at: "2026-10-02T09:00:00Z",
  updated_at: "2026-10-28T09:00:00Z",
  downloader_name: "qb-main",
  site_enabled: true,
  active_count: 0,
  downloading_count: 0,
  active_size_bytes: 0,
  today: { uploaded: 0, downloaded: 0, added: 0, removed: 0 },
  total: { uploaded: 0, downloaded: 0, added: 0, removed: 0 },
  ...over,
});
const BRUSH_TASKS = [
  brushTask({
    id: 1,
    name: "HDSky 免费刷流",
    site_name: "hdsky",
    last_result: "列表 50 个、符合条件 4 个、加入 2 个",
    active_count: 7,
    downloading_count: 2,
    active_size_bytes: 186.4 * GiB,
    today: { uploaded: 64.2 * GiB, downloaded: 21.7 * GiB, added: 5, removed: 3 },
    total: { uploaded: 1.82 * 1024 * GiB, downloaded: 612 * GiB, added: 214, removed: 201 },
  }),
  brushTask({
    id: 2,
    name: "OurBits 刷流",
    site_name: "ourbits",
    downloader_id: 2,
    downloader_name: "tr-backup",
    min_leechers: 3,
    remove_seed_time_h: 72,
    last_result: "列表 50 个、符合条件 1 个、加入 1 个",
    last_run_at: ago(7),
    active_count: 4,
    downloading_count: 1,
    active_size_bytes: 92.8 * GiB,
    today: { uploaded: 23.5 * GiB, downloaded: 9.1 * GiB, added: 2, removed: 1 },
    total: { uploaded: 846 * GiB, downloaded: 301 * GiB, added: 97, removed: 92 },
  }),
];
/* 收益统计：按日 30 段，最后一段是今天；两个任务每天的上传有起有落 */
const BRUSH_STATS = (() => {
  const days = Array.from({ length: 30 }, (_, i) => {
    const d = new Date();
    d.setDate(d.getDate() - (29 - i));
    return ymd(d);
  });
  const wave = (i, base, amp, phase) =>
    Math.max(0, base + amp * Math.sin(i / 3.1 + phase) + ((i * 7) % 5));
  const series = (k, base, amp, phase) =>
    days.map((date, i) => {
      const up = (i === 29 ? [64.2, 23.5][k] : wave(i, base, amp, phase)) * GiB;
      return {
        date,
        end: date,
        uploaded: up,
        downloaded: up * 0.34,
        added: 2 + ((i + k) % 4),
        removed: 1 + ((i + k) % 3),
      };
    });
  const tasks = [
    { task_id: 1, name: "HDSky 免费刷流", series: series(0, 52, 18, 0) },
    { task_id: 2, name: "OurBits 刷流", series: series(1, 21, 9, 1.4) },
  ];
  return {
    granularity: "day",
    periods: 30,
    days: 30,
    from: days[0],
    to: days[29],
    dates: days,
    tasks,
    totals: days.map((date, i) => ({
      date,
      end: date,
      uploaded: tasks[0].series[i].uploaded + tasks[1].series[i].uploaded,
      downloaded: tasks[0].series[i].downloaded + tasks[1].series[i].downloaded,
      added: tasks[0].series[i].added + tasks[1].series[i].added,
      removed: tasks[0].series[i].removed + tasks[1].series[i].removed,
    })),
  };
})();

/* AI 助手：一段问追剧进度的对话，两次工具调用，回答里是工具查到的数（和上面两张图一致） */
const ASSISTANT_CONV = {
  id: 7,
  title: "长夜将尽第 2 季的进度",
  entry: "web",
  running: false,
  created_at: ago(3),
  updated_at: ago(2),
};
const ASSISTANT_MESSAGES = [
  {
    id: 1,
    run_id: "r1",
    role: "user",
    content: "长夜将尽第 2 季追到哪一集了？今晚更新的第 7 集下好了吗？",
    created_at: ago(3),
  },
  {
    id: 2,
    run_id: "r1",
    role: "assistant",
    content: "",
    model: "deepseek-v4-flash",
    tool_calls: [
      { id: "c1", name: "get_subscription", arguments: '{"id":5}' },
      { id: "c2", name: "list_media_history", arguments: '{"q":"长夜将尽","limit":3}' },
    ],
    created_at: ago(3),
  },
  {
    id: 3,
    run_id: "r1",
    role: "tool",
    tool_name: "get_subscription",
    tool_call_id: "c1",
    content:
      '{"id":5,"title":"长夜将尽","season":2,"status":"active","follow":true,"lock":{"group":"HHWEB","resolution":"2160p"},"have":[1,2,3,4,5,6,7],"next":{"episode":8,"air_date":"2099-11-12"}}',
    created_at: ago(3),
  },
  {
    id: 4,
    run_id: "r1",
    role: "tool",
    tool_name: "list_media_history",
    tool_call_id: "c2",
    content:
      '{"items":[{"title":"长夜将尽","season":2,"episode":7,"status":"done","library":"剧集库","mode":"hardlink"}],"total":7}',
    created_at: ago(3),
  },
  {
    id: 5,
    run_id: "r1",
    role: "assistant",
    model: "deepseek-v4-flash",
    content:
      "**长夜将尽** 第 2 季在追，版本锁在 HHWEB 2160p。\n\n- **第 7 集已经下好**：今晚在 HDSky 找到同一版本，下载完成后已硬链接进剧集库的 `长夜将尽 (2025)/Season 02`，海报和简介也刮削好了。\n- 第 1 到 6 集早已入库，现在这一季有 7 集。\n- 第 8 集还没播，播出后会按同一个版本自动搜索。\n\n要不要第 8 集入库后在 Telegram 提醒你？",
    created_at: ago(2),
  },
];

/*
 * 「长夜将尽」的海报与背景图：这部剧是编的，两张图也是生成的（Z-Image Turbo，Apache-2.0，2026-11-09），
 * 放在 docs-media/。页面按「媒体识别」里设置的图片地址加载海报，这里把那个地址指到一个不存在的主机
 * （.invalid 不会被解析），浏览器发出的图片请求由 CDP 的 Fetch 拦下、用本地文件回答，不会去连 TMDB。
 */
const DOCS_IMAGE_HOST = "http://ptt-docs.invalid";
const DOCS_MEDIA_DIR = join(here, "docs-media");
const itemTv = fixture("/api/media/items/tv/100088");

const DOCS_STUBS = {
  "/api/media/settings": {
    has_tmdb_key: false,
    key_source: "builtin",
    builtin_key: true,
    language: "zh-CN",
    proxy_url: "",
    api_base_url: "",
    image_base_url: `${DOCS_IMAGE_HOST}/t/p/`,
    has_fanart_key: false,
    has_fanart_client_key: false,
    has_omdb_key: false,
  },
  "/api/media/items/tv/100088": {
    ...itemTv,
    item: {
      ...itemTv.item,
      poster_path: "/long-night-poster.webp",
      backdrop_path: "/long-night-backdrop.webp",
    },
  },
  "/api/media/history": { items: HISTORY_ROWS, total: HISTORY_ROWS.length },
  "/api/brush/tasks": BRUSH_TASKS,
  "/api/brush/stats": BRUSH_STATS,
  "/api/assistant/status": {
    enabled: true,
    model: "deepseek-v4-flash",
    can_confirm: true,
    chatops: true,
  },
  "/api/assistant/models": {
    default: "deepseek-v4-flash",
    models: ["deepseek-v4-flash", "deepseek-v4-pro"],
  },
  /* 详情排在列表前面：前缀匹配是第一个命中即用 */
  "/api/assistant/conversations/7": {
    ...ASSISTANT_CONV,
    messages: ASSISTANT_MESSAGES,
    confirmations: [],
    last_seq: 40,
  },
  "/api/assistant/conversations": [ASSISTANT_CONV],
};

/*
 * 只截一块时的截取区域：在页面里求值的表达式，返回 { x, y, width, height }（CSS 像素）；
 * 要截的东西没找到、或者放不进规定的高度，就返回 null，按失败处理，不悄悄截一张不对的图。
 * 两步演示的两张图一样高（STAGE_H），在首页上并排时才齐。530 截到「你的 NAS 上」追剧那一行的下面：
 * 截到整个面板（660）时，1280 × 800 的首屏只露出演示带的 35%。
 */
const STAGE_H = 530;
const rectOf = (sel) => `document.querySelector(${JSON.stringify(sel)})?.getBoundingClientRect()`;
const STAGE_FOLLOW = `(() => { const h = ${rectOf(".mi-hero")}, n = ${rectOf('[data-testid="mi-nas-subscription"] .mi-nas__desc')};
  if (!h || !n || n.bottom + 4 > h.top + ${STAGE_H}) return null;
  return { x: 0, y: h.top, width: innerWidth, height: ${STAGE_H} }; })()`;
const STAGE_HISTORY = `(() => { const p = ${rectOf(".pt-panel")}, c = ${rectOf(".mh-cards > *")};
  if (!p || !c || c.bottom > p.top - 8 + ${STAGE_H}) return null;
  return { x: 0, y: p.top - 8, width: innerWidth, height: ${STAGE_H} }; })()`;
/* 去掉左边的侧栏（导航列或 rail）与底下的状态条，截到页面内容的底 */
const CONTENT_OF = (sel) => `(() => { const head = ${rectOf(".pt-head")}, page = ${rectOf(sel)};
  const status = ${rectOf(".pt-status")};
  if (!head || !page || (status && page.bottom + 16 > status.top)) return null;
  return { x: head.left, y: 0, width: innerWidth - head.left, height: Math.ceil(page.bottom) + 16 }; })()`;
const CHAT = `(() => { const c = ${rectOf(".as-chat")};
  return c ? { x: c.left, y: c.top, width: c.width, height: c.height } : null; })()`;

/* [路由, 宽, 高, 移动端, 文件名前缀, { dpr, clip }] */
const SHOTS = [
  ["/userinfo", 1440, 900, false, "home"],
  ["/sites", 1440, 900, false, "sites"],
  ["/userinfo", 375, 812, true, "home-mobile"],
  ["/media/item/tv/100088", 640, 1100, false, "media-follow", { dpr: 2, clip: STAGE_FOLLOW }],
  ["/media/history", 640, 1100, false, "media-history", { dpr: 2, clip: STAGE_HISTORY }],
  ["/brush", 1440, 900, false, "brush", { dpr: 1.5, clip: CONTENT_OF(".brush-page") }],
  ["/assistant", 1100, 640, false, "assistant", { dpr: 1.5, clip: CHAT }],
];

/* ChatOps 指南配图：[路由, 文件名, 截图前的动作]。动作返回 false 表示没找到要点的东西，按失败处理 */
const clickButton = (label) => `(() => { const b = [...document.querySelectorAll('button')]
  .find((x) => x.textContent.trim() === ${JSON.stringify(label)} && x.offsetParent !== null);
  if (!b) return false; b.click(); return true; })()`;
const scrollToPanel = (title) => `(() => { const el = [...document.querySelectorAll('.pt-panel')]
  .find((x) => x.querySelector('.pt-panel__title')?.textContent.trim() === ${JSON.stringify(title)});
  if (!el) return false; el.scrollIntoView({ block: 'start' }); return true; })()`;
const GUIDE_SHOTS = [
  ["/chatops/notifications", "chatops-notifications-list", null],
  ["/chatops/notifications", "chatops-add-channel-dialog", clickButton("添加通道")],
  ["/chatops/notifications/1", "chatops-telegram-detail", scrollToPanel("凭证与连接")],
  ["/chatops/notifications/2", "chatops-qq-detail", scrollToPanel("凭证与连接")],
  ["/chatops/bindings", "chatops-bindings-list", null],
  ["/chatops/bindings", "chatops-bindings-dialog", clickButton("生成绑定码")],
  ["/chatops/audit", "chatops-audit-stats", null],
  ["/chatops/rss-notifications", "chatops-rss-notifications", null],
];

const busy = await fetch(`http://127.0.0.1:${PORT}/json/version`, {
  signal: AbortSignal.timeout(1200),
}).catch(() => null);
if (busy?.ok) {
  console.error(`CDP 端口 ${PORT} 已被占用，换一个：CDP_PORT=<端口>`);
  process.exit(2);
}
const chrome = spawn(
  CHROME,
  [
    "--headless=new",
    "--no-sandbox",
    "--disable-dev-shm-usage",
    `--remote-debugging-port=${PORT}`,
    "about:blank",
  ],
  { stdio: ["ignore", "ignore", "pipe"] },
);
const pause = (ms) => new Promise((r) => setTimeout(r, ms));

let wsUrl;
for (let i = 0; i < 80 && !wsUrl; i++) {
  try {
    wsUrl = (await (await fetch(`http://127.0.0.1:${PORT}/json/version`)).json())
      .webSocketDebuggerUrl;
  } catch {
    /* Chrome 还在启动 */
  }
  if (!wsUrl) await pause(150);
}
if (!wsUrl) {
  console.error("Chrome 的 CDP 没有起来");
  chrome.kill("SIGKILL");
  process.exit(2);
}

const ws = new WebSocket(wsUrl);
await new Promise((r) => (ws.onopen = r));
let msgId = 0;
const pending = new Map();
ws.onmessage = (e) => {
  const m = JSON.parse(e.data);
  if (m.method === "Fetch.requestPaused") {
    serveDocsImage(m.params);
    return;
  }
  if (m.id && pending.has(m.id)) {
    const { resolve: ok, reject } = pending.get(m.id);
    pending.delete(m.id);
    if (m.error) reject(new Error(JSON.stringify(m.error)));
    else ok(m.result);
  }
};
const send = (method, params = {}, sessionId) =>
  new Promise((ok, reject) => {
    const id = ++msgId;
    pending.set(id, { resolve: ok, reject });
    ws.send(JSON.stringify({ id, method, params, sessionId }));
  });
const { targetId } = await send("Target.createTarget", { url: "about:blank" });
const { sessionId } = await send("Target.attachToTarget", { targetId, flatten: true });
const cmd = (m, p) => send(m, p, sessionId);
await cmd("Page.enable");
await cmd("Runtime.enable");
/* 截图里不要滚动条；海报请求（见 DOCS_IMAGE_HOST）在发出之前拦下 */
await cmd("Emulation.setScrollbarsHidden", { hidden: true });
await cmd("Fetch.enable", { patterns: [{ urlPattern: `${DOCS_IMAGE_HOST}/*` }] });

/* 只回答 docs-media 里有的文件，别的一律 404 */
function serveDocsImage({ requestId, request }) {
  const name = new URL(request.url).pathname.split("/").pop() ?? "";
  const file = join(DOCS_MEDIA_DIR, name);
  const found = /^[a-z0-9-]+\.webp$/.test(name) && existsSync(file);
  cmd(
    "Fetch.fulfillRequest",
    found
      ? {
          requestId,
          responseCode: 200,
          responseHeaders: [{ name: "Content-Type", value: "image/webp" }],
          body: readFileSync(file).toString("base64"),
        }
      : { requestId, responseCode: 404, responseHeaders: [], body: "" },
  ).catch(() => {});
}

/* 假数据，外加一个记录「最后一次 DOM 变化」的观察器：等页面安定靠它，不靠固定时长 */
await cmd("Page.addScriptToEvaluateOnNewDocument", {
  source:
    stubScript({ ...VERSION_STUBS, ...GUIDE_STUBS, ...DOCS_STUBS }) +
    `
;(() => {
  /* AI 助手打开会话后订阅事件流：这里没有真的会话，换成一个一直开着、什么也不发的事件流，页面才不会显示「连接断开」 */
  class QuietEventSource extends EventTarget {
    constructor(url) { super(); this.url = url; this.readyState = 1; this.onmessage = null; this.onerror = null; this.onopen = null; }
    close() { this.readyState = 2; }
  }
  QuietEventSource.CONNECTING = 0; QuietEventSource.OPEN = 1; QuietEventSource.CLOSED = 2;
  window.EventSource = QuietEventSource;
})();
;(() => { window.__lastMut = performance.now();
  new MutationObserver(() => { window.__lastMut = performance.now(); })
    .observe(document, { subtree: true, childList: true, attributes: true, characterData: true }); })();`,
});

const ev = async (expr) => {
  const r = await cmd("Runtime.evaluate", {
    expression: expr,
    awaitPromise: true,
    returnByValue: true,
  });
  if (r.exceptionDetails)
    throw new Error(r.exceptionDetails.exception?.description ?? r.exceptionDetails.text);
  return r.result?.value;
};

/* 条件等待：文档就绪、有渲染内容、没有可见的 loading 遮罩、DOM 连续 400ms 没有变化；上限 10s */
async function settled() {
  const t0 = Date.now();
  while (Date.now() - t0 < 10000) {
    const ok = await ev(`document.readyState === 'complete' && !!document.querySelector('#app > *')
      && ![...document.querySelectorAll('.el-loading-mask')].some((m) => m.offsetParent !== null)
      && performance.now() - (window.__lastMut ?? 0) > 400`).catch(() => false);
    if (ok) return true;
    await pause(100);
  }
  return false;
}

async function viewport(width, height, mobile, dpr = 1) {
  await cmd("Emulation.setDeviceMetricsOverride", {
    width,
    height,
    deviceScaleFactor: dpr,
    mobile,
  });
}

async function goto(route) {
  await cmd("Page.navigate", { url: `${base}/#${route}` });
  await settled();
  /* 假数据没装上（导航太早）就重载一次，绝不能拿真实接口的空数据去截图 */
  if ((await ev(`window.__ptStub === true`).catch(() => false)) !== true) {
    await cmd("Page.reload");
    await settled();
  }
}

/* 主题与本地偏好写进 localStorage 后重载生效；扩展推荐条按「7 天不再提醒」关掉，免得占掉首屏 */
async function prepare(mode) {
  await ev(`localStorage.setItem('theme', '${mode}'); localStorage.setItem('theme-style', 'cockpit');
    localStorage.setItem('pt-v2-banner-dismissed', '1'); localStorage.setItem('pt_tools_v2_banner_dismissed_v1', '1');
    localStorage.setItem('nav-collapsed', '0');
    localStorage.setItem('pt-userinfo-ext-hint-dismissed-at', String(Date.now()));
    localStorage.removeItem('pt-userinfo-sites-height-v1'); 1`);
  await cmd("Page.reload");
  await settled();
}

async function closeNotifications() {
  await ev(`document.querySelectorAll('.el-notification__closeBtn').forEach((b) => b.click()); 1`);
  await pause(350);
}

let failed = false;
try {
  await viewport(1440, 900, false);
  await cmd("Page.navigate", { url: `${base}/#/login` });
  await settled();
  const status = await ev(
    `fetch('/login', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username: ${JSON.stringify(USER)}, password: ${JSON.stringify(PASS)} }) }).then((r) => r.status)`,
  );
  if (status !== 200 && status !== 302)
    throw new Error(`登录失败（HTTP ${status}），用 PT_USER / PT_PASS 指定账号`);

  for (const mode of ["light", "dark"]) {
    await prepare(mode);
    for (const [route, width, height, mobile, name, opts = {}] of SHOTS) {
      if (ONLY && !ONLY.includes(name)) continue;
      const dpr = opts.dpr ?? 1;
      await viewport(width, height, mobile, dpr);
      await goto(route);
      await cmd("Page.reload");
      await settled();
      await closeNotifications();
      /* 海报是图片，DOM 安定了它不一定画出来了：等屏幕里的图片解码完，最多等 5 秒 */
      await ev(`Promise.race([
        Promise.all([...document.images]
          .filter((i) => { const b = i.getBoundingClientRect(); return b.bottom > 0 && b.top < innerHeight; })
          .map((i) => i.decode().catch(() => 0))),
        new Promise((r) => setTimeout(r, 5000)),
      ]).then(() => 1)`);
      const clip = opts.clip ? await ev(opts.clip) : null;
      if (opts.clip && !clip)
        throw new Error(`${name}：没找到要截的区域，或者它放不进规定的高度（${route}）`);
      const shot = await cmd("Page.captureScreenshot", {
        format: "webp",
        quality: 85,
        ...(clip && { clip: { ...clip, scale: 1 } }),
      });
      const file = join(outDir, `${name}-${mode}.webp`);
      writeFileSync(file, Buffer.from(shot.data, "base64"));
      const [w, h] = clip ? [clip.width, clip.height] : [width, height];
      console.log(`${file}  ${Math.round(w * dpr)} × ${Math.round(h * dpr)}`);
    }
  }

  await prepare("light");
  await viewport(1440, 900, false);
  for (const [route, name, action] of ONLY ? [] : GUIDE_SHOTS) {
    await goto(route);
    await cmd("Page.reload");
    await settled();
    await closeNotifications();
    if (action) {
      if ((await ev(action)) !== true) throw new Error(`${name}：没找到要操作的元素（${route}）`);
      await settled();
    }
    const shot = await cmd("Page.captureScreenshot", { format: "webp", quality: 85 });
    const file = join(guideDir, `${name}.webp`);
    writeFileSync(file, Buffer.from(shot.data, "base64"));
    console.log(file);
  }
} catch (e) {
  console.error(e instanceof Error ? e.message : e);
  failed = true;
} finally {
  chrome.kill("SIGKILL");
}
process.exit(failed ? 1 : 0);
