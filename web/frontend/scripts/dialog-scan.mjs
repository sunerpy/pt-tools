#!/usr/bin/env node
/*
 * 画板 40「弹窗、抽屉与确认 · 总览」的可执行副本。
 *
 * 画板 40 是全站浮层的**清点**：15 个 el-dialog、3 个抽屉（其中 2 个标注「当前未实现」
 * 的是提案）、19 个 ElMessageBox 确认、以及右上角的消息堆叠。每块小图的说明里都写了
 * 它来自哪个 `.vue` 文件 —— 所以这份清点是可以逐条核对的。
 *
 * 为什么不用渲染验收逐个点开：39 块里 19 块是 ElMessageBox，点开要先造出「有选中行」
 * 之类的前置状态，而它们的规格（页头 42、双按钮页脚、文案）在 board-check 里已经抽查过
 * 两个代表。这份脚本要守的是另一件事：**清单本身不许漂移** ——
 *   · 画板列了的浮层，代码里必须还在（删掉一个就红）；
 *   · 代码里新加的 el-dialog / el-drawer，必须在画板清单里有一席（悄悄加一个也红）。
 * 少了哪一边，画板 40 就从「全站清点」退化成一张过期的图。
 *
 * 用法：node scripts/dialog-scan.mjs
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const SRC = join(ROOT, "src");

/**
 * 画板 40 的清单。`tile` 是画板上的小图名，`file` 是画板说明里标的来源文件，
 * `needle` 是这块浮层在代码里的锚点（对话框标题，或确认框里那句文案的关键片段）。
 *
 * 只有 `proposal: true` 的两块允许在代码里不存在 —— 画板自己标了「当前未实现」。
 */
const BOARD_40 = [
  // --- el-dialog（画板 00–15）
  { tile: "00 添加站点", file: "views/SiteList.vue", needle: 'title="添加站点"' },
  { tile: "01 保号配置", file: "views/SiteList.vue", needle: "保号配置 · ${configSiteName}" },
  { tile: "02 添加 RSS 订阅", file: "views/SiteDetail.vue", needle: 'title="添加 RSS 订阅"' },
  { tile: "03 编辑过滤规则", file: "views/FilterRules.vue", needle: "'编辑过滤规则'" },
  { tile: "04 匹配测试结果", file: "views/FilterRules.vue", needle: 'title="匹配测试结果"' },
  { tile: "05 推送到下载器", file: "views/TorrentSearch.vue", needle: 'title="推送到下载器"' },
  {
    tile: "06 批量推送到下载器",
    file: "views/TorrentSearch.vue",
    needle: 'title="批量推送到下载器"',
  },
  { tile: "07 编辑下载器", file: "views/DownloaderSettings.vue", needle: "'编辑下载器'" },
  {
    tile: "08 目录管理",
    file: "views/DownloaderSettings.vue",
    needle: "保存目录 · ${currentDownloader?.name",
  },
  {
    tile: "09 同步站点下载器",
    file: "views/DownloaderSettings.vue",
    needle: 'title="同步站点下载器"',
  },
  {
    tile: "10 添加通知通道",
    file: "views/chatops/Notifications.vue",
    needle: 'title="添加通知通道"',
  },
  { tile: "11 生成绑定码", file: "views/chatops/Bindings.vue", needle: 'title="生成绑定码"' },
  {
    tile: "12 批量修改存储路径",
    file: "views/DownloaderHub.vue",
    needle: 'title="批量修改存储路径"',
  },
  {
    tile: "13 添加种子到下载器",
    file: "views/DownloaderHub.vue",
    needle: 'title="添加种子到下载器"',
  },
  { tile: "14 添加目录 · 二层弹窗", file: "views/DownloaderSettings.vue", needle: "'添加目录'" },
  {
    tile: "15 删除确认 · 双动作页脚",
    file: "views/PausedTorrents.vue",
    needle: 'title="删除确认"',
  },

  // --- 抽屉（画板 16–18；17/18 画板自己标了「当前未实现」，是提案）
  { tile: "16 抽屉 · 任务详情", file: "views/DownloaderHub.vue", needle: "<el-drawer" },
  { tile: "17 抽屉 · 筛选", file: "views/SiteList.vue", needle: "<el-drawer", proposal: true },
  {
    tile: "18 抽屉 · 通道调试",
    file: "views/chatops/Notifications.vue",
    needle: "<el-drawer",
    proposal: true,
  },

  // --- ElMessageBox 确认（画板 19–37）
  { tile: "19 确认 · 停止所有任务", file: "stores/runtime.ts", needle: "停止所有任务" },
  { tile: "20 确认 · 启动所有任务", file: "stores/runtime.ts", needle: "启动所有任务" },
  { tile: "21 确认 · 批量删除记录", file: "views/TaskList.vue", needle: "ElMessageBox.confirm" },
  { tile: "22 确认 · 删除站点", file: "views/SiteList.vue", needle: "ElMessageBox.confirm" },
  { tile: "23 确认 · 删除 RSS", file: "views/SiteDetail.vue", needle: "ElMessageBox.confirm" },
  { tile: "24 确认 · 删除过滤规则", file: "views/FilterRules.vue", needle: "ElMessageBox.confirm" },
  {
    tile: "25 确认 · 删除下载器",
    file: "views/DownloaderSettings.vue",
    needle: "ElMessageBox.confirm",
  },
  { tile: "26 确认 · 删除保存目录", file: "views/DownloaderSettings.vue", needle: "删除目录" },
  { tile: "27 确认 · 恢复暂停任务", file: "views/PausedTorrents.vue", needle: "ElMessageBox" },
  { tile: "28 确认 · 批量删除暂停任务", file: "views/PausedTorrents.vue", needle: "批量" },
  { tile: "29 确认 · 清理工作目录", file: "views/AutoCleanup.vue", needle: "ElMessageBox" },
  { tile: "30 确认 · 批量下载", file: "views/TorrentSearch.vue", needle: "ElMessageBox" },
  { tile: "31 确认 · 批量推送", file: "views/TorrentSearch.vue", needle: "批量推送" },
  { tile: "32 确认 · 复制链接", file: "views/TorrentSearch.vue", needle: "复制" },
  {
    tile: "33 确认 · 删除通知通道",
    file: "views/chatops/Notifications.vue",
    needle: "ElMessageBox",
  },
  {
    tile: "34 确认 · 停用状态下测试",
    file: "views/chatops/NotificationDetail.vue",
    needle: "ElMessageBox",
  },
  {
    tile: "35 确认 · 取消 RSS 通知",
    file: "views/chatops/RSSNotifications.vue",
    needle: "ElMessageBox",
  },
  {
    tile: "36 确认 · 撤销 ChatOps 绑定",
    file: "views/chatops/Bindings.vue",
    needle: "ElMessageBox",
  },
  { tile: "37 确认 · 升级到预发版", file: "components/VersionChecker.vue", needle: "ElMessageBox" },

  // --- 消息堆叠（画板 38）
  { tile: "38 消息堆叠 · 右上角", file: "views/SiteList.vue", needle: "ElMessage" },
];

/**
 * 画板清单之外、但代码里确实存在的弹窗 —— 每条都要写清为什么画板上没有它。
 * 这不是豁免：它同样会被打印出来给 owner 看。
 */
const NOT_ON_BOARD = {
  "views/BrushTasks.vue":
    "刷流任务页（路线图 M3）晚于画板 40 定稿：任务编辑弹窗与种子抽屉都不在那张清点图里",
  "views/TorrentTransfer.vue":
    "转移做种页（路线图 M5）晚于画板 40 定稿：定时规则的编辑弹窗不在那张清点图里",
  "views/MediaRecognize.vue":
    "媒体识别页（路线图 M9）晚于画板 40 定稿：识别词的添加 / 编辑弹窗不在那张清点图里",
  "views/MediaLibrary.vue":
    "媒体库页（路线图 M10）晚于画板 40 定稿：媒体库、路径映射、媒体服务器的编辑弹窗不在那张清点图里",
  "views/MediaHistory.vue":
    "整理历史页（路线图 M10）晚于画板 40 定稿：删除记录（可连库里的文件一起删）的确认弹窗不在那张清点图里",
  "components/media/OrganizeDialog.vue":
    "「整理入库」（路线图 M10）晚于画板 40 定稿：任务列表与下载器 Web UI 共用这个弹窗",
  "views/MediaSubscriptions.vue":
    "订阅页（路线图 M11）晚于画板 40 定稿：豆瓣想看的条目弹窗不在那张清点图里",
  "components/media/SubscribeDialog.vue":
    "「订阅」（路线图 M11）晚于画板 40 定稿：探索页与订阅页共用这个弹窗",
  "components/media/SubscriptionDetailDialog.vue":
    "订阅详情（路线图 M11）晚于画板 40 定稿：分集与下载过的种子",
  "components/media/QualityProfileDialog.vue": "质量档案的添加与修改（路线图 M11）晚于画板 40 定稿",
  "components/media/DoubanSourceDialog.vue":
    "豆瓣想看来源的添加与修改（路线图 M11）晚于画板 40 定稿",
  "components/downloader/TransferDialog.vue":
    "「转移到其他下载器」（路线图 M5）晚于画板 40 定稿：下载器 Web UI 与任务列表的批量操作共用这个弹窗",
  "views/DynamicSiteSettings.vue":
    "动态站点两个弹窗：路由在 router/index.ts 里已注释掉，画板刻意不画（design-system.md 的说明）",
  "views/SiteList.vue#导入站点模板":
    "「导入站点模板」弹窗晚于画板 40 定稿，功能已上线但没进那张清点图",
  "views/SiteDetail.vue#编辑 RSS 订阅":
    "「编辑 RSS 订阅」与「添加 RSS 订阅」除 v-model 目标外逐字相同，画板只出一个规格",
  "views/chatops/Notifications.vue#通知详情":
    "列表页的只读详情弹窗，画板把通道详情画成了独立路由（板 23/37）",
  "components/shell/MobileChrome.vue":
    "底栏「我的」的上拉面板 —— 它的规格在画板 34（移动 · 我的），不在这张清点图里",
  "views/chatops/RSSNotifications.vue#通知详情":
    "移动端专用：行卡替代了桌面表格的展开行，详情只能放弹窗。画板 26 画的是桌面展开行，" +
    "画板 40 定稿时移动端行卡还没落地，所以两张图上都没有它。桌面走展开行，不会用到这个弹窗。",
};

function walk(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) out.push(...walk(p));
    else out.push(p);
  }
  return out;
}

const files = walk(SRC).filter((f) => f.endsWith(".vue") || f.endsWith(".ts"));
const body = new Map(files.map((f) => [relative(SRC, f), readFileSync(f, "utf8")]));

const problems = [];
let proposals = 0;

/* ① 画板列了的，代码里必须还在 */
for (const row of BOARD_40) {
  const src = body.get(row.file);
  if (src === undefined) {
    problems.push(`画板「${row.tile}」标的来源文件不存在：${row.file}`);
    continue;
  }
  if (src.includes(row.needle)) continue;
  if (row.proposal) {
    proposals += 1;
    continue;
  }
  problems.push(`画板「${row.tile}」在 ${row.file} 里找不到锚点「${row.needle}」`);
}

/*
 * ② 代码里的每一个 el-dialog / el-drawer **实例**都要有归属。
 *
 * 这里原来是按**文件**判的：只要某个文件因为任意一条（哪怕是一条 ElMessageBox）
 * 进了 claimedFiles，整个文件的弹窗反扫就被跳过 —— 于是
 * `RSSNotifications.vue` 的移动端「通知详情」弹窗既不在清单里、也没有说明，脚本照样报
 * 「清点一致」。评审指着这一点说「这不能证明浮层数量按稿」，判得对。
 *
 * 改成按实例判：把每个弹窗开标签里的 title 抓出来当身份，
 * 逐个要求它被 BOARD_40 的某条认领，或在 NOT_ON_BOARD 里写明为什么不在画板上。
 */
function dialogInstances(src) {
  const out = [];
  const re = /<el-(dialog|drawer)\b/g;
  let m;
  while ((m = re.exec(src)) !== null) {
    /* 开标签可能跨多行，取到第一个 '>' 为止 */
    const close = src.indexOf(">", m.index);
    const tag = src.slice(m.index, close < 0 ? m.index + 400 : close);
    const title = /:?title="([^"]*)"/.exec(tag)?.[1] ?? "";
    out.push({ kind: m[1], title });
  }
  return out;
}

for (const [rel, src] of body) {
  if (!rel.endsWith(".vue")) continue;
  for (const inst of dialogInstances(src)) {
    const id = `${rel}#${inst.title || `(${inst.kind} 无 title)`}`;
    /* 画板清单认领：同文件的某条锚点出现在这个实例的 title 里，或它就是抽屉那条 */
    const claimed = BOARD_40.some((r) => {
      if (r.file !== rel) return false;
      if (r.needle === "<el-drawer") return inst.kind === "drawer";
      const needle = r.needle.replace(/^title="|"$/g, "").replace(/^'|'$/g, "");
      return inst.title !== "" && (inst.title.includes(needle) || needle.includes(inst.title));
    });
    if (claimed) continue;
    if (NOT_ON_BOARD[id] || NOT_ON_BOARD[rel]) continue;
    problems.push(`${id} 这个弹窗/抽屉画板 40 的清单里没有，也没写为什么`);
  }
}

for (const p of problems) console.log(`✗ ${p}`);
for (const [k, why] of Object.entries(NOT_ON_BOARD)) {
  console.log(`· 画板清单之外 ${k} —— ${why}`);
}
console.log(
  problems.length === 0
    ? `画板 40 清点一致：${BOARD_40.length - proposals} 处浮层在位，${proposals} 处是画板标注的未实现提案`
    : `画板 40 清点不一致：${problems.length} 处`,
);
process.exit(problems.length === 0 ? 0 : 1);
