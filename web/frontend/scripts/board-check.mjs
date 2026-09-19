#!/usr/bin/env node
/*
 * 画板一致性验收 —— 拿真实渲染去比 Penpot 画板的构成，不一致就退非零。
 *
 * 为什么需要它：之前的验收脚本只在「横向溢出」或「出现未解析组件」时失败，几何数字
 * 只是打印出来给人看。于是少一整组卡片、分栏宽度错、坐标错都能拿到绿灯 ——
 * 一次 review 直接把这一点判成「验收不能支持已经按稿完成的结论」，判得对。
 *
 * 这个文件就是**画板事实的可执行副本**：
 *   EXPECT      每条路由该有哪些带、卡片层有哪些**栏宽**、至少几张卡（数字全部来自
 *               Penpot 读回，记在 docs/design/webui-board-spec.md）
 *   ALLOWED_GAPS 明确记下来的偏离，每条都得写原因。不在这张表里的缺失一律失败。
 *
 * 它校验的是带与栏的几何，不是逐张卡的存在：同一栏里少一张卡这种事量不出来，
 * 那一层靠 docs/design/webui-board-spec.md 的偏离表记录，别把它当成「全都对上了」。
 *
 * 用法（需要一个已经跑起来的 pt-tools 服务，前端 dist 必须是当前源码构建出来的）：
 *   node scripts/board-check.mjs http://127.0.0.1:8080 [--port 19400] [--route /tasks]
 *
 * 依赖：只用 Chrome 的 CDP，不装任何浏览器自动化包。Chrome 路径可用 PT_CHROME 覆盖。
 */
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { stubScript } from "./board-fixtures.mjs";

// ---------------------------------------------------------------- 画板事实

/**
 * 带式表格页的构成：head 64 → toolbar 40 → grid → （selband 44）→ foot 34 → 卡片。
 * 卡片宽度按画板的栏宽写，单位 px（1440 画布下主区 1112 宽、卡片区 1080 宽）。
 */
const BAND = "band";
/** 卡片页：head 之后直接进卡片层，没有表格带 */
const CARD = "card";

const EXPECT = {
  "/userinfo": {
    board: "10 用户统计",
    kind: BAND,
    ownHead: "kpi", // 画板用 KPI 带替代 head
    bands: ["toolbar", "grid", "foot"],
    cards: [548, 516, 1080], // p-up / p-dist / p-watch
    minCards: 3, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["上传构成", "等级分布", "需要关注"],
    /* 画板 10 的行高与十列（docs/design/webui-board-spec.md §6）；差异见 ALLOWED_GAPS */
    gridRowHeight: 34,
    gridColumns: ["站点", "等级", "数据量", "分享率", "做种", "做种体积", "积分", "时魔", "更新"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/userinfo/export": {
    board: "11 导出分享图",
    kind: CARD,
    cards: [700, 364], // p-prev / p-set
    minCards: 2, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["预览", "导出设置"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/sites": {
    board: "12 站点列表",
    kind: BAND,
    bands: ["toolbar", "grid", "foot"],
    cards: [364, 340, 344, 1080], // p-health / p-rss / p-ev / p-auth
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["探测健康", "RSS 订阅", "保号提醒", "认证方式"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/supported-sites": {
    board: "14 已支持站点",
    kind: CARD,
    bands: ["toolbar"], // bar-64，之后直接是卡
    cards: [548, 516, 1080], // g0..g3 两栏（数量随站点数变）+ p-cap 通栏
    minCards: 5, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["内置能力"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/search": {
    board: "15 种子搜索",
    kind: BAND,
    ownHead: "head88", // search-head 88：查询框 + 筛选 chip
    bands: ["toolbar", "grid", "foot"],
    cards: [548, 516, 1080], // p-sites / p-alt / p-hist
    minCards: 3, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["站点命中", "没返回的站点", "搜索历史", "已保存的搜索"],
    /*
     * 铺一条 v1 格式的「保存搜索」：它的 category 存的是站点原始分类名，
     * 读取时要被迁移成桶 ID。这条分支只有老用户才会走到，必须显式铺出来。
     */
    seed: `localStorage.setItem(
      'pt-tools-search-saved-v1',
      JSON.stringify([{ name: 'Dune', keyword: 'Dune', sites: [], sortBy: 'seeders',
        orderDesc: true, category: '电影/HD', freeOnly: false }]),
    )`,
    needsSeg: true, // 画板 bar-88 的分类分段
    /* 画板 bar-88 的五个固定档位 + search-head 的 go/ha-0/ha-1 + chip-1 */
    controls: ["搜索", "保存搜索", "最近搜索", "仅免费", "电影", "剧集", "动漫", "音乐"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
    /*
     * 搜索页的结果、页脚带与分析卡都要先搜一次才有。这里直接写进输入框再点按钮，
     * 不动内部状态 —— 走的是用户真实路径，页面自己的 loading / 六态照常参与。
     */
    prepare: `(async () => {
      const input = document.querySelector('.pt-band--head input');
      if (!input) return 'no-input';
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
      setter.call(input, 'Dune');
      input.dispatchEvent(new Event('input', { bubbles: true }));
      await new Promise((r) => setTimeout(r, 200));
      const go = [...document.querySelectorAll('.pt-band--head button')]
        .find((b) => b.textContent.includes('搜索'));
      if (!go) return 'no-button';
      go.click();
      await new Promise((r) => setTimeout(r, 1500));
      return 'searched';
    })()`,
  },
  "/tasks": {
    board: "16 任务列表",
    kind: BAND,
    bands: ["toolbar", "grid", "foot"],
    cards: [548, 516, 1080], // p-thr / p-site / p-warn
    minCards: 3, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["最近 7 天吞吐", "按站点分布", "需要关注的任务"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/paused": {
    board: "17 暂停任务",
    kind: CARD,
    cards: [1080, 1080], // c-paused / c-archive
    minCards: 2, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["暂停中", "历史归档"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/downloader-hub": {
    board: "18 下载器 Web UI",
    kind: CARD,
    bands: ["toolbar"],
    cards: [276, 276, 276, 788, 386, 386], // 左列三卡 / 右上 p-grid / 右下 p-rate + p-note
    minCards: 6, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["传输", "状态", "筛选", "任务列表", "速率", "这一页的口径"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/downloaders": {
    board: "19 下载器设置",
    kind: CARD,
    cards: [1080, 548, 516, 1080, 1080], // p-dl / p-dir / p-safe / p-bind / p-log
    minCards: 5, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["下载器", "下载目录", "磁盘保护", "站点绑定", "连通性检查"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/filter-rules": {
    board: "20 过滤规则",
    kind: BAND,
    bands: ["toolbar", "grid", "foot"], // bar-64 落地为空带，见 ALLOWED_GAPS
    cards: [548, 516, 1080, 1080], // p-order / p-test / p-hit / p-hint
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["匹配顺序", "试跑", "命中统计", "规则怎么生效"],
    needsSeg: true, // 画板 bar-64 的「全部 / 启用 / 禁用」分段
    controls: ["全部", "启用", "禁用", "试跑这条"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/cleanup": {
    board: "21 自动删种",
    kind: CARD,
    cards: [1080], // p-main
    minCards: 1, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["自动删种"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/chatops/notifications": {
    board: "22 消息通知",
    kind: CARD,
    cards: [548, 516, 548, 516, 1080], // nt0..nt3 两栏 + p-policy/p-stat + p-recent
    minCards: 7, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["投递策略", "最近投递统计", "最近的通知"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/chatops/notifications/1": {
    board: "23 通道详情",
    kind: CARD,
    detail: true, // head 88 带面包屑
    cards: [1080, 1080, 1080, 700], // hero / c-basic / c-test / p-msg
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["hero", "基本信息", "凭证与连接", "连通性测试", "操作提示文案"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/chatops/bindings": {
    board: "24 ChatOps 绑定",
    kind: CARD,
    cards: [612, 612, 612], // p-pending / p-active / p-note
    minCards: 3, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["待绑定", "已绑定", "绑定是怎么走的"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/chatops/audit": {
    board: "25 操作审计",
    kind: BAND,
    bands: ["toolbar", "grid", "foot"],
    cards: [548, 516, 1080, 1080], // p-cmd / p-ch / p-fail / p-keep
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["命令分布", "渠道分布", "失败与被拒", "保留与清理"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/chatops/rss-notifications": {
    board: "26 RSS 通知日志",
    kind: BAND,
    bands: ["toolbar", "grid", "foot"],
    cards: [548, 516, 548, 516, 1080], // p-res / p-idem / p-site / p-quiet / p-retry
    minCards: 5, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["推送结果分布", "幂等与去重", "待重试与失败", "按站点分布", "安静时段"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/global": {
    board: "27 系统设置",
    kind: CARD,
    cards: [1080], // p-cfg（画板另有一条 warn 提示，那是状态不是常驻卡）
    minCards: 1, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["全局配置"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/cloak-config": {
    board: "28 CloakBrowser",
    kind: CARD,
    cards: [1080, 700, 364, 1080], // intro / p-cfg / p-res / p-life
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["intro", "Manager 连接", "探测结果", "页面状态"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/logs": {
    board: "29 运行日志",
    kind: CARD,
    bands: ["toolbar"],
    cards: [300, 300, 300, 764], // 左列 p-files/p-lv/p-arc / 右 p-tail
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["日志文件", "级别筛选", "轮转归档", "运行日志"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/password": {
    board: "44 修改密码",
    kind: CARD,
    cards: [520, 344], // p-acct 居中 + p-rules/p-msg/p-note 三栏
    minCards: 4, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["账号信息", "口令规则", "改完会发生什么", "忘记密码怎么办"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
  },
  "/sites/M-Team": {
    board: "13 站点详情",
    kind: CARD,
    detail: true,
    tabs: true, // tabs 带 40 高
    tabLabels: ["概览", "RSS 订阅", "任务", "推送记录", "过滤规则", "凭据"], // 画板 tabs 的六项
    cards: [700, 700, 700, 364, 364, 364, 364], // 左列 p-rss/p-push/p-tasks · 右列 p-cred/p-keep/p-stat/p-danger
    minCards: 7, // 画板这一页的卡片张数（数据驱动的卡按下限算）
    titles: ["RSS 订阅", "最近推送", "任务", "站点凭据", "保号规则", "站点统计", "危险操作"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
    /*
     * 画板 13 的激活分区是「RSS 订阅」，左栏那三张卡就是这个分区下的内容。
     * 页面默认停在「概览」，所以先点一下分区带 —— 不点就只能量到右栏。
     */
    prepare: `(async () => {
      const tab = [...document.querySelectorAll('.pt-band__tab')]
        .find((b) => b.textContent.includes('RSS'));
      if (!tab) return 'no-tab';
      tab.click();
      await new Promise((r) => setTimeout(r, 400));
      return 'switched';
    })()`,
  },
};

/**
 * 明确记下来的偏离 —— 键是 `<路由> <检查项>`，值是原因。
 * 加一条就等于向 owner 承认「这里没按画板做」，所以原因必须写清是**为什么做不了**，
 * 不是「暂时不想做」。想让某处通过检查，先实现它，不是往这张表里加一行。
 */
const ALLOWED_GAPS = {
  /*
   * 用户统计表与画板 10 的四处差异。**都不是漏做，是与用户验收冲突后按用户决定保留的**：
   * 提交 266625b 记着「站点有消息角标时展示不全」是真实浏览器验收退回的缺陷，
   * 当时的修法就是把行高放到 48（.pt-grid--roomy）并让角标贴头像右上角向左生长。
   * 按画板压回 34 + 纯文本站点列，等于把那次修复回滚掉。
   * 那四个多出来的列（真实数据 / 入站 / 剩余天数 / 操作）是改版前就有的功能列，
   * 砍列属于产品取舍。两件都要 owner 拍板，不由实现单方面决定。
   */
  "/userinfo grid.rowHeight":
    "画板 34，落地 48（.pt-grid--roomy）：站点列有 32px 头像与未读角标，34 的行盒会把角标切掉 —— " +
    "角标显示不全正是用户验收退回过的缺陷（266625b）。改回 34 需先由 owner 决定是否放弃角标与头像。",
  "/userinfo grid.columns":
    "画板十列且站点列为纯文本；落地多出「真实数据 / 入站 / 剩余天数 / 操作」四列，站点列带头像+角标+双行。" +
    "那四列是改版前就有的功能列（计量差异、封禁提醒、打开/同步入口），砍掉属于产品取舍，需 owner 拍板。",

  /*
   * 移动画板 30 / 31 的筛选区与行卡走势图。两条都不是「忘了做」：
   */
  "/sites@375 filterRow":
    "画板 30 的筛选只有一排 26 高的 chip（全部/正常/异常/已禁用 各带计数），落地是把桌面工具栏整条搬了过来：" +
    "「已启用/全部」分段 + 搜索框 + 探测已启用 + 打开已启用 + 新增站点，换行堆到 240 高。" +
    "画板的移动稿里没有画这三个入口，砍掉它们等于手机上少三个功能入口 —— 要收进菜单还是要留在页面上属于产品取舍，需 owner 拍板。",
  "/tasks@375 filterRow":
    "画板 31 的筛选是一条 294×30 的分段器；落地是三个可叠加的筛选 chip + 搜索框 + 站点下拉。" +
    "那三个 chip 是可组合的（apiTasks 里逐个 AND），收成画板那条互斥分段器会删掉组合筛选能力 —— " +
    "这正是第九轮修掉的缺陷，不能为了对齐画板再改回去。",
  "/sites@375 rowCards.spark":
    "画板 30 的行卡里有一条 120×18 的 8 根柱走势图，落地没有：后端没有按站点的历史序列接口。" +
    "perSiteStats 只给当前快照（uploaded / ratio / seeding …），造一条假的走势比不画更糟。",

  /*
   * 这张表现在是空的 —— 上一轮登记的偏离都实现掉了。
   * 再往里加一条就等于向 owner 承认「这里没按画板做」，所以原因必须写清是**为什么做不了**
   * （需要后端新接口、会造成重复写入口一类），不是「暂时不想做」。
   */
};

// ---------------------------------------------------------------- CDP 夹具

const BASE = process.argv[2] ?? "http://127.0.0.1:8080";
const argOf = (name, fallback) => {
  const i = process.argv.indexOf(name);
  return i > 0 && process.argv[i + 1] ? process.argv[i + 1] : fallback;
};
const PORT = Number(argOf("--port", "19400"));
const ONLY = argOf("--route", "");
const CHROME =
  process.env.PT_CHROME ?? "/config/.cache/ms-playwright/chromium-1243/chrome-linux64/chrome";
const USER = process.env.PT_USER ?? "admin";
const PASS = process.env.PT_PASS ?? "adminadmin";

if (!existsSync(CHROME)) {
  console.error(`找不到 Chrome：${CHROME}\n用 PT_CHROME 指定路径。`);
  process.exit(2);
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const profile = mkdtempSync(join(tmpdir(), "ptboard-"));
const chrome = spawn(
  CHROME,
  [
    "--headless=new",
    "--no-sandbox",
    "--disable-dev-shm-usage",
    "--disable-gpu",
    `--remote-debugging-port=${PORT}`,
    `--user-data-dir=${profile}`,
    "--window-size=1440,1024",
    "about:blank",
  ],
  { stdio: ["ignore", "ignore", "pipe"] },
);
chrome.stderr.on("data", () => {});

async function wsUrl() {
  for (let i = 0; i < 60; i++) {
    try {
      const r = await fetch(`http://127.0.0.1:${PORT}/json/version`);
      const j = await r.json();
      if (j.webSocketDebuggerUrl) return j.webSocketDebuggerUrl;
    } catch {
      /* Chrome 还没起来 */
    }
    await sleep(300);
  }
  throw new Error("Chrome 未就绪");
}

class CDP {
  constructor(ws) {
    this.ws = ws;
    this.id = 0;
    this.pending = new Map();
    this.onEvent = () => {};
    ws.addEventListener("message", (ev) => {
      const m = JSON.parse(ev.data);
      const slot = m.id && this.pending.get(m.id);
      if (!slot) {
        if (m.method) this.onEvent(m);
        return;
      }
      this.pending.delete(m.id);
      if (m.error) slot.reject(new Error(JSON.stringify(m.error)));
      else slot.resolve(m.result);
    });
  }
  send(method, params = {}, sessionId) {
    const id = ++this.id;
    return new Promise((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      this.ws.send(JSON.stringify({ id, method, params, sessionId }));
    });
  }
}

const ws = new WebSocket(await wsUrl());
await new Promise((r) => ws.addEventListener("open", r, { once: true }));
const cdp = new CDP(ws);
const { targetInfos } = await cdp.send("Target.getTargets");
const target = targetInfos.find((t) => t.type === "page");
const { sessionId } = await cdp.send("Target.attachToTarget", {
  targetId: target.targetId,
  flatten: true,
});
await cdp.send("Page.enable", {}, sessionId);
await cdp.send("Runtime.enable", {}, sessionId);
/** 假数据脚本装上了没有 —— 装上之后每条路由都要求当前文档真的铺到了（见 goto） */
let stubInstalled = false;

/* 控制台报错也算不一致：渲染崩了页面照样可能量不出东西，得说清是崩了还是没做 */
let consoleErrors = [];
cdp.onEvent = (m) => {
  if (m.method === "Runtime.consoleAPICalled" && m.params.type === "error") {
    consoleErrors.push(
      (m.params.args ?? [])
        .map((a) => a.value ?? a.description ?? "")
        .join(" ")
        .slice(0, 220),
    );
  }
  if (m.method === "Runtime.exceptionThrown") {
    consoleErrors.push(
      (m.params.exceptionDetails?.exception?.description ?? "未捕获异常").slice(0, 220),
    );
  }
};

async function ev(expression) {
  const r = await cdp.send(
    "Runtime.evaluate",
    { expression, awaitPromise: true, returnByValue: true },
    sessionId,
  );
  if (r.exceptionDetails) throw new Error(JSON.stringify(r.exceptionDetails).slice(0, 300));
  return r.result.value;
}

async function waitReady() {
  for (let i = 0; i < 80; i++) {
    await sleep(250);
    const ready = await ev(
      `document.readyState === 'complete' && !!document.querySelector('#app > *, form')`,
    ).catch(() => false);
    if (ready === true) return;
  }
}

/**
 * 跳到一条路由。
 *
 * 这里有一个会把整轮验收变成假绿的坑：路由是 hash 模式，`Page.navigate` 到
 * `…/#/sites` 时如果当前文档已经是同一个路径，Chrome 只当成片段跳转，**不产生新文档**，
 * 于是 `Page.addScriptToEvaluateOnNewDocument` 注册的假数据脚本一次都不执行 ——
 * 页面拿到的是真后端的空库，量出来的「没有页脚带 / 卡片区是空的」看着像没实现。
 * 所以导航后要确认这一篇文档确实铺上了假数据（stubScript 会挂 window.__ptStub），
 * 没有就强制 reload 一次，reload 一定是新文档。
 */
async function goto(url) {
  await cdp.send("Page.navigate", { url }, sessionId);
  await waitReady();
  if (!stubInstalled) return;
  const stubbed = await ev(`window.__ptStub === true`).catch(() => false);
  if (stubbed === true) return;
  await cdp.send("Page.reload", {}, sessionId);
  await waitReady();
}

await goto(`${BASE}/`);
await ev(
  `fetch('/login', { method: 'POST', body: new URLSearchParams({ username: ${JSON.stringify(USER)}, password: ${JSON.stringify(PASS)} }) })`,
);
// v2 升级横幅不在任何画板的构成里，关掉它才能量到画板的坐标
await ev(`localStorage.setItem('pt_tools_v2_banner_dismissed_v1', '1')`);
/*
 * 喂假数据 —— 画板的页脚带、多选条和一部分卡片只有有数据时才渲染，
 * 空库量出来的「没有页脚带」是没数据而不是没实现，不喂就分不开这两件事。
 */
await cdp.send("Page.addScriptToEvaluateOnNewDocument", { source: stubScript() }, sessionId);
stubInstalled = true;
await cdp.send(
  "Emulation.setDeviceMetricsOverride",
  { width: 1440, height: 1024, deviceScaleFactor: 1, mobile: false },
  sessionId,
);

// ---------------------------------------------------------------- 测量

const MEASURE = `(() => {
  const box = (el) => {
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
  };
  const one = (sel) => { const el = document.querySelector(sel); return el ? box(el) : null; };
  const inner = document.querySelector('.pt-shell__inner');
  return {
    head: one('.pt-head'),
    kpi: one('.pt-kpi'),
    /* KPI 每格：画板要求每格都有变化 pill 与 48×22 柱图，所以逐格量，不只量整条高度 */
    kpiCells: [...document.querySelectorAll('.pt-kpi__cell')].map((el) => ({
      label: (el.querySelector('.pt-kpi__label')?.textContent ?? '').trim(),
      pill: Boolean(el.querySelector('.pt-pill, .pt-status-pill, [class*="pill"]')),
      bars: Boolean(el.querySelector('.pt-kpi__bars')),
    })),
    tabLabels: [...document.querySelectorAll('.pt-band__tab')].map((el) =>
      (el.textContent ?? '').replace(/\\s+/g, ' ').trim(),
    ),
    /*
     * 关键控件：按钮/分段这类「画板上画了、少了就不算落地」的东西。
     * 纯图标钮没有文字，身份在 aria-label / title 上，所以三者都收 ——
     * 只看 textContent 会把画板上那些 32×32 的图标钮全都漏掉。
     */
    controlText: [...document.querySelectorAll('button, .el-segmented__item, .pt-band__tab')]
      .flatMap((el) => [
        (el.textContent ?? '').replace(/\\s+/g, ' ').trim(),
        el.getAttribute('aria-label') ?? '',
        el.getAttribute('title') ?? '',
      ])
      .filter(Boolean),
    hasSeg: Boolean(document.querySelector('.pt-seg, .el-segmented')),
    /* 表格带的列名与行高 —— 画板对表格页规定了列集合与 34 的行节奏 */
    gridColumns: [...document.querySelectorAll('.pt-band--grid th .cell')]
      .map((el) => (el.textContent ?? '').replace(/\\s+/g, ' ').trim())
      .filter(Boolean),
    gridRowHeight: (() => {
      const row = document.querySelector('.pt-band--grid tbody tr td');
      return row ? Math.round(row.getBoundingClientRect().height) : null;
    })(),
    ownHead: one('.pt-band--head'),
    tabs: one('.pt-band--tabs'),
    toolbar: one('.pt-band--toolbar'),
    grid: one('.pt-band--grid'),
    foot: one('.pt-band--foot'),
    cardsBox: one('.pt-cards'),
    /* 卡片层里每一块的宽度：PtPanel、手搓卡和 hero 都算，空态块不算 */
    /*
     * 卡片层里每一块的宽度与标题。
     *
     * 三个坑，都踩过：
     * ① 页面可能有多个 .pt-cards（修改密码页是「居中窄卡」+「三栏说明卡」两段），
     *    只查第一个会漏掉后面整段；
     * ② 栏容器里可能还套着真正的卡（下载器控制台的左栏 aside 里有三张 PtPanel），
     *    只量栏容器就会把「三张卡」和「一整块面」判成一样；
     * ③ 宽度要按**多重集合**比，画板要两张 1080 就必须真有两张。
     * 所以遇到直接子节点里有 PtPanel 的栏容器就下钻一层，其余按自身算。
     */
    cards: (() => {
      const out = [];
      const push = (el) => {
        if (getComputedStyle(el).display === 'none') return;
        const r = el.getBoundingClientRect();
        if (r.width <= 4) return; /* 拖拽把手一类 */
        /*
         * 身份优先取 PtPanel 的标题；没有标题的块（hero、提示条一类）取 data-card。
         * 少了这一层，无标题的卡就没有可测身份 —— 删掉通道详情的 hero，
         * 剩下的卡照样能把宽度多重集合与张数下限配满，检查不会红。
         */
        out.push({
          w: Math.round(r.width),
          title:
            (el.querySelector('.pt-panel__title')?.textContent ?? '').trim() ||
            el.dataset.card ||
            '',
          /* 卡头高度：画板「卡片 p-*」规定发丝线在 29；没有卡头的块记 null */
          head: (() => {
            const h = el.querySelector('.pt-panel__head');
            return h ? Math.round(h.getBoundingClientRect().height) : null;
          })(),
        });
      };
      /* 只从最外层的卡片容器出发：里层容器的卡已经被外层的后代查询收进来了，
         两层都处理会把同一张卡数两遍（下载器设置页实测 9 张变 4 张真卡）。 */
      const boxes = [...document.querySelectorAll('.pt-cards')]
        .filter((el) => !el.parentElement?.closest('.pt-cards'));
      for (const box of boxes) {
        /* 卡可以嵌在任意深的栏容器里（控制台右栏是「表格卡 + 两张小卡」两层），
           所以直接把这一段里所有 PtPanel 都算上，不数层数。 */
        for (const panel of box.querySelectorAll('.pt-panel')) push(panel);
        /* 没有 PtPanel 的直接子节点也算一块卡：手搓卡、hero、提示条都在这一类 */
        for (const col of box.children) {
          if (col.classList.contains('pt-panel')) continue;
          if (col.querySelector('.pt-panel')) continue;
          push(col);
        }
        /*
         * 显式标了 data-card 的块，无论嵌在第几层都算一块。
         * 站点详情的 bn 横幅就嵌在含 PtPanel 的栏容器里，上面两条都收不到它 ——
         * 加了 data-card 却仍然不可测，等于没加。
         */
        for (const marked of box.querySelectorAll('[data-card]')) {
          if (marked.classList.contains('pt-panel')) continue;
          if (out.some((c) => c.title === marked.dataset.card)) continue;
          push(marked);
        }
      }
      return out;
    })(),
    overflow: inner ? inner.scrollWidth - Math.round(inner.getBoundingClientRect().width) : 0,
    unknown: [...new Set([...document.querySelectorAll('*')]
      .filter((el) => el instanceof HTMLUnknownElement)
      .map((el) => el.tagName.toLowerCase()))],
  };
})()`;

/** 主区左边界与宽度：画板里所有带都是 x=328 w=1112，卡片层内缩 16 */
const MAIN_X = 328;
const MAIN_W = 1112;
/** 容差：滚动条会让宽度少 10 左右，1px 的发丝线也会让高度差 1 */
const TOL = 12;

const near = (a, b, tol = TOL) => a !== null && a !== undefined && Math.abs(a - b) <= tol;

// ---------------------------------------------------------------- 逐路由检查

/** --route 可以给逗号分隔的子集，调试时只跑几条 */
const wanted = ONLY ? ONLY.split(",").map((r) => r.trim()) : [];
const routes = Object.keys(EXPECT).filter((r) => wanted.length === 0 || wanted.includes(r));
const failures = [];
const gapsUsed = new Set();

for (const route of routes) {
  const want = EXPECT[route];
  consoleErrors = [];
  /*
   * 导航之前先铺状态。localStorage 里的历史数据会走到只有老用户才碰得到的分支
   * （例如「保存搜索」的 v1→v2 迁移），而每条路由都用全新的 Chrome profile，
   * 不铺就永远测不到那条路 —— 一次 review 正是从这里找出一个整页白屏的 TDZ。
   */
  if (want.seed) await ev(want.seed).catch(() => {});
  await goto(`${BASE}/#${route}`);
  await sleep(2400);
  if (want.prepare) {
    const outcome = await ev(want.prepare).catch((e) => `prepare 失败：${e.message}`);
    if (typeof outcome === "string" && outcome.startsWith("no-"))
      failures.push({
        route,
        board: want.board,
        key: "prepare",
        msg: `准备步骤没跑起来：${outcome}`,
      });
    await sleep(600);
  }
  const got = await ev(MEASURE);
  const fail = (key, msg) => {
    const gapKey = `${route} ${key}`;
    if (ALLOWED_GAPS[gapKey]) {
      gapsUsed.add(gapKey);
      return;
    }
    failures.push({ route, board: want.board, key, msg });
  };

  for (const err of [...new Set(consoleErrors)].slice(0, 3)) fail("console", err);
  if (got.overflow > 0) fail("overflow", `主区横向溢出 ${got.overflow}px`);
  if (got.unknown.length > 0) fail("unknown", `未解析组件 ${got.unknown.join(", ")}`);

  // 页头：画板里除 KPI 页与搜索页之外，每页都是 64（详情页 88）
  if (want.ownHead === "kpi") {
    if (!got.kpi) fail("kpi", "画板用 KPI 带替代 head，页面上找不到 .pt-kpi");
    else if (!near(got.kpi.h, 64)) fail("kpi", `KPI 带高 ${got.kpi.h}，画板 64`);
    /* 画板 kpibar：6 格，每格都有变化 pill 与 48×22 柱图 */
    const cells = got.kpiCells;
    if (cells.length !== (want.kpiCells ?? 6)) {
      fail("kpi.cells", `KPI 有 ${cells.length} 格，画板 ${want.kpiCells ?? 6} 格`);
    }
    const noBars = cells.filter((c) => !c.bars).map((c) => c.label);
    if (noBars.length > 0) fail("kpi.bars", `这些格没有柱图：${noBars.join("、")}`);
    const noPill = cells.filter((c) => !c.pill).map((c) => c.label);
    if (noPill.length > 0) fail("kpi.pill", `这些格没有变化 pill：${noPill.join("、")}`);
  } else if (want.ownHead === "head88") {
    if (!got.ownHead) fail("ownHead", "画板 search-head 88，页面上找不到 .pt-band--head");
    else if (!near(got.ownHead.h, 88, 2)) fail("ownHead", `自带页头高 ${got.ownHead.h}，画板 88`);
  } else {
    const wantH = want.detail ? 88 : 64;
    if (!got.head) fail("head", "找不到 .pt-head");
    else if (!near(got.head.h, wantH, 2)) fail("head", `页头高 ${got.head.h}，画板 ${wantH}`);
  }

  /* 表格页的行节奏与列集合（画板对表格页规定了这两样） */
  if (want.gridRowHeight && got.gridRowHeight !== null) {
    if (!near(got.gridRowHeight, want.gridRowHeight, 3)) {
      fail("grid.rowHeight", `行高 ${got.gridRowHeight}，画板 ${want.gridRowHeight}`);
    }
  }
  if (want.gridColumns) {
    /* 画板的列必须都在 */
    for (const col of want.gridColumns) {
      if (!got.gridColumns.some((c) => c.includes(col))) {
        fail("grid.columns", `表头里没有「${col}」；实测 ${got.gridColumns.join(" / ")}`);
      }
    }
    /*
     * 画板之外的列也要报。只查「缺了没」挡不住「多了几列」——
     * 而多列一样是与画板不一致，且会改变整表的列宽分配。
     */
    const extra = got.gridColumns.filter((c) => !want.gridColumns.some((w) => c.includes(w)));
    if (extra.length > 0) {
      fail("grid.columns", `多出画板之外的列：${extra.join(" / ")}`);
    }
  }

  /*
   * 卡头高度。画板「卡片 p-*」规定卡头发丝线在 29（docs/design/webui-board-spec.md）。
   * 这是 85 个 PtPanel 实例共用的一个数，错了会系统性改变所有卡的内部比例，
   * 而它在验收里一直没人看守 —— 实现曾经是 42，跑了很多轮都没红。
   */
  for (const card of got.cards) {
    if (card.head === null) continue;
    if (!near(card.head, 29, 3)) {
      fail(`cards.head(${card.title || card.w})`, `卡头高 ${card.head}，画板 29`);
    }
  }

  /*
   * 关键控件。画板上画了一枚按钮或一条分段器，少了就不算按稿落地 ——
   * 光量带高与栏宽看不出「工具栏里少了分类分段」这种事。
   */
  if (want.needsSeg && !got.hasSeg) {
    fail("controls.seg", "画板这一页有分段器，页面上找不到");
  }
  for (const label of want.controls ?? []) {
    if (!got.controlText.some((t) => t.includes(label))) {
      fail(`controls(${label})`, `找不到写着「${label}」的控件`);
    }
  }

  /*
   * 张数下限。光比栏宽挡不住「用别的卡冒充」：画板 22 是四张通道卡（548/516 两栏）
   * 外加 p-policy 548 / p-stat 516 / p-recent 1080，通道卡本身就把 548 与 516 占了，
   * 宽度多重集合会给出假绿。张数是不含糊的第二道。
   * 用下限而不是精确值：通道卡、站点卡这类的数量随数据变。
   */
  if (want.minCards && got.cards.length < want.minCards) {
    fail(
      "cards.count",
      `卡片层只有 ${got.cards.length} 块，画板至少 ${want.minCards} 张：` +
        got.cards.map((c) => `${c.w}${c.title ? `(${c.title})` : ""}`).join(" / "),
    );
  }

  if (want.tabs) {
    if (!got.tabs) fail("tabs", "画板有 40 高的分区带，页面上找不到 .pt-band--tabs");
    else if (!near(got.tabs.h, 40, 2)) fail("tabs", `分区带高 ${got.tabs.h}，画板 40`);
    /* 分区带的项要逐个对上画板的标签，不能只对上带高 */
    for (const label of want.tabLabels ?? []) {
      if (!got.tabLabels.some((t) => t.includes(label))) {
        fail(`tabs.label(${label})`, `分区带里没有「${label}」；实测 ${got.tabLabels.join(" / ")}`);
      }
    }
  }

  // 带：存在、贴住主区两侧、高度对得上、彼此相邻
  for (const band of want.bands ?? []) {
    const b = got[band];
    if (!b) {
      fail(`bands.${band}`, `画板有 ${band} 带，页面上没有`);
      continue;
    }
    if (!near(b.x, MAIN_X) || !near(b.w, MAIN_W))
      fail(`bands.${band}`, `${band} 带在 x=${b.x} 宽 ${b.w}，画板 x=${MAIN_X} 宽 ${MAIN_W}`);
    const wantH = { toolbar: 40, foot: 34 }[band];
    if (wantH && !near(b.h, wantH, 3)) fail(`bands.${band}`, `${band} 带高 ${b.h}，画板 ${wantH}`);
  }
  if (got.toolbar && got.grid && !near(got.grid.y, got.toolbar.y + got.toolbar.h, 2))
    fail(
      "bands.adjacent",
      `表格带 y=${got.grid.y} 没有紧贴工具栏带底沿 ${got.toolbar.y + got.toolbar.h}`,
    );
  if (got.grid && got.foot && got.foot.y < got.grid.y)
    fail("bands.adjacent", "页脚带排在表格带之前");

  // 卡片层：内缩 16，栏宽按画板
  const wantCards = want.cards ?? [];
  if (wantCards.length > 0) {
    if (!got.cardsBox) {
      fail(`cards[${wantCards.join(",")}]`, "画板有卡片区，页面上找不到 .pt-cards");
    } else {
      if (!near(got.cardsBox.x, MAIN_X))
        fail("cards.x", `卡片区 x=${got.cardsBox.x}，画板 ${MAIN_X}`);
      const found = got.cards;
      if (found.length === 0) {
        fail(`cards[${wantCards.join(",")}]`, "卡片区是空的");
      } else {
        /*
         * 按多重集合配：每期望一张就从实测里消掉一张，不去重 ——
         * 画板要两张 1080，实际只有一张就得红。
         */
        const pool = [...found];
        const missing = [];
        for (const w of wantCards) {
          const hit = pool.findIndex((c) => near(c.w, w));
          if (hit < 0) missing.push(w);
          else pool.splice(hit, 1);
        }
        const shown = found.map((c) => `${c.w}${c.title ? `(${c.title})` : ""}`).join(" / ");
        /*
         * 身份断言。只比栏宽与张数挡不住「用另一张同宽的卡补位」——
         * 删掉通道详情里任意一张 1080 的卡，剩下的照样能把三张 1080 配满。
         * 所以每条路由列出画板那几张卡的标题，逐个要求它在页面上真的存在。
         */
        for (const title of want.titles ?? []) {
          if (!found.some((c) => c.title.includes(title))) {
            fail(`cards.title(${title})`, `找不到标题含「${title}」的卡；实测 ${shown}`);
          }
        }
        /* 逐张上报：一页里某一张卡做不到，不该把整页的卡片检查一起豁免 */
        const seen = new Map();
        for (const w of missing) {
          const n = (seen.get(w) ?? 0) + 1;
          seen.set(w, n);
          /* 同宽多张时键带序号：第 2 张缺了要单独登记，不能被第 1 张的偏离顺带豁免 */
          fail(`cards.${w}${n > 1 ? `#${n}` : ""}`, `缺画板 ${w} 宽的卡；实测 ${shown}`);
        }
      }
    }
  }
}

// ------------------------------------------------- 移动端（画板 30–35 与 §9）

/*
 * 桌面那 21 条路由之外，画板还有六块移动稿，此前完全没进验收 —— 脚本固定
 * 1440×1024，于是「零偏离」只在桌面口径内成立，而文档没把这个口径写出来。
 *
 * 六块移动画板读回来的骨架完全一样（docs/design/webui-board-spec.md §10）：
 *   topbar 0,0 375×88 fill chrome —— 但**前 36 是手机自己的状态栏**
 *          （14:32 / 信号 / Wi-Fi / 电量），浏览器里不存在也不该画。属于 app 的是
 *          40…88：左 h1 19/700 + sub 11/400，右两个 30×30 r=4 图标钮（search、bell）
 *   内容   x=16 宽 343（左右各内缩 16）
 *   tabbar 0,739 375×72 —— **状态行在这 72 之内**（sel 指示条在 765）
 *   五个 tab：概览 / 站点 / 任务 / 搜索 / 我的
 * §9 另外规定：触控目标 ≥ 44×44、桌面表格一律降级成行卡（不做横向滚动表格）、
 * KPI 条降级成 2×2。
 */
const M_VIEWPORT = { width: 375, height: 812 };
/** app 顶栏：画板属于 app 的那段是 48，落地 52（理由见 theme.scss 的注释） */
const M_TOPBAR_H = 52;
/** 底栏 = 状态行 24 + tab 行 48 = 72，与画板 tabbar 的 72 对上 */
const M_NAV_H = 72;
const M_TABS = ["概览", "站点", "任务", "搜索", "我的"];
/** 内容列：画板一律 x=16 宽 343 */
const M_INNER_X = 16;

const MOBILE_EXPECT = {
  "/sites": {
    board: 30,
    title: "站点",
    sub: true,
    rowCards: 1,
    activeTab: "站点",
    /* 画板 30 的筛选是一排 26 高的 chip（全部 14 / 正常 12 / 异常 2 / 已禁用 1） */
    filterRowH: 26,
    /* 画板 30 的行卡里有 120×18 的 8 根柱 */
    rowSpark: true,
  },
  "/tasks": {
    board: 31,
    title: "任务",
    sub: true,
    rowCards: 1,
    activeTab: "任务",
    /* 画板 31 是一条 294×30 的分段器 */
    filterRowH: 30,
  },
  "/sites/M-Team": { board: 32, title: "M-Team", sub: true },
  "/chatops/notifications": { board: 33, title: "消息通知", sub: true },
  "/logs": { board: 35, title: "运行日志", sub: true },
  /* 画板 34「我的」在产品里没有对应路由（折成了上拉面板），换成 §9 的 KPI 2×2 规则 */
  "/userinfo": { board: "§9", title: "用户统计", kpiCols: 2, activeTab: "概览" },
};

const MEASURE_M = `(() => {
  const box = (el) => {
    const r = el.getBoundingClientRect();
    return { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) };
  };
  const one = (sel) => { const el = document.querySelector(sel); return el ? box(el) : null; };
  const txt = (sel) => (document.querySelector(sel)?.textContent ?? '').replace(/\\s+/g, ' ').trim();
  const inner = document.querySelector('.pt-shell__inner');
  return {
    topbar: one('.pt-mchrome'),
    title: txt('.pt-mchrome__title'),
    sub: txt('#pt-mhead-sub'),
    /*
     * 顶栏图标钮量的是**命中区**，不是可见方块。画板给 30×30，而 §9 要 ≥44×44，
     * 两者只能用「方块 30 + ::before 外扩到 44」同时满足 —— 伪元素量不到尺寸，
     * 所以从中心朝四个方向各打一个 21px 的点，看命中的还是不是这个钮自己。
     */
    icons: [...document.querySelectorAll('.pt-mchrome__icon')].map((el) => {
      const r = el.getBoundingClientRect();
      const cx = r.left + r.width / 2;
      const cy = r.top + r.height / 2;
      const owns = (x, y) => {
        const hit = document.elementFromPoint(x, y);
        return Boolean(hit && (hit === el || el.contains(hit)));
      };
      return {
        label: el.getAttribute('aria-label') ?? '',
        w: Math.round(r.width),
        h: Math.round(r.height),
        touch: owns(cx, cy - 21) && owns(cx, cy + 21) && owns(cx - 21, cy) && owns(cx + 21, cy),
      };
    }),
    /* 工具栏带：画板 30 的筛选是一排 26 高的 chip，31 是一条 30 高的分段器 */
    toolbar: one('.pt-band--toolbar'),
    /* 行卡里有没有柱图（画板 30 的行卡带 120×18 的 8 根柱） */
    rowCardsWithSpark: [...document.querySelectorAll('.pt-rowcard')]
      .filter((el) => el.querySelector('.pt-bars, .pt-kpi__bars')).length,
    nav: one('.pt-mnav'),
    tabs: [...document.querySelectorAll('.pt-mnav__tab')].map((el) => {
      const r = el.getBoundingClientRect();
      return {
        label: (el.textContent ?? '').replace(/\\s+/g, ' ').trim(),
        w: Math.round(r.width),
        h: Math.round(r.height),
        active: el.classList.contains('is-active'),
      };
    }),
    /* 桌面外壳必须整套让位：375 下 rail 与桌面页头会把内容挤没 */
    deskVisible: ['.pt-rail', '.pt-status', '.pt-head'].filter((sel) => {
      const el = document.querySelector(sel);
      return el && getComputedStyle(el).display !== 'none';
    }),
    /*
     * 内容列的**内容边界**，不是元素边界。.pt-shell__inner 本身是整宽的，
     * 16 的内缩在它的 padding 里 —— 量 getBoundingClientRect().left 永远得到 0，
     * 那样的断言只会证明「元素贴着屏幕左边」，跟画板的内缩没关系。
     * （这段注释身处一个模板字符串内部，所以不能用反引号引类名。）
     */
    inner: inner
      ? (() => {
          const r = inner.getBoundingClientRect();
          const cs = getComputedStyle(inner);
          const padL = parseFloat(cs.paddingLeft) || 0;
          const padR = parseFloat(cs.paddingRight) || 0;
          return {
            x: Math.round(r.left + padL),
            w: Math.round(r.width - padL - padR),
          };
        })()
      : null,
    /* 行卡：§9 要求桌面表格在移动端降级成行卡 */
    rowCards: document.querySelectorAll('.pt-rowcard').length,
    /*
     * 还在横向滚的表格。§9 明确「不做横向滚动表格」，而一张 el-table 在 375 下默认
     * 就是横向滚的 —— 它自己滚，不会让 documentElement 溢出，所以只看页面溢出量不出来。
     */
    scrollers: [...document.querySelectorAll('table, .el-table__body-wrapper, .pt-band--grid')]
      .filter((el) => getComputedStyle(el).display !== 'none' && el.scrollWidth - el.clientWidth > 8)
      .map((el) => String(el.className).split(' ')[0] || el.tagName.toLowerCase()),
    /* 列数量在 .pt-kpi__grid 上，不在 .pt-kpi 上（外层是带，内层才是网格） */
    kpiCols: (() => {
      const grid = document.querySelector('.pt-kpi__grid');
      if (!grid) return null;
      return getComputedStyle(grid).gridTemplateColumns.split(' ').filter(Boolean).length;
    })(),
    kpiNoBars: [...document.querySelectorAll('.pt-kpi__cell')]
      .filter((el) => !el.querySelector('.pt-kpi__bars'))
      .map((el) => (el.querySelector('.pt-kpi__label')?.textContent ?? '').trim()),
    /* 整页横向溢出：手机上出现横向滚动条等于这块画板完全不成立 */
    docOverflow: document.documentElement.scrollWidth - window.innerWidth,
    unknown: [...new Set([...document.querySelectorAll('*')]
      .filter((el) => el instanceof HTMLUnknownElement)
      .map((el) => el.tagName.toLowerCase()))],
  };
})()`;

const mobileRoutes = Object.keys(MOBILE_EXPECT).filter(
  (r) => wanted.length === 0 || wanted.includes(r),
);

if (mobileRoutes.length > 0) {
  await cdp.send(
    "Emulation.setDeviceMetricsOverride",
    { ...M_VIEWPORT, deviceScaleFactor: 1, mobile: true },
    sessionId,
  );

  for (const route of mobileRoutes) {
    const want = MOBILE_EXPECT[route];
    consoleErrors = [];
    await goto(`${BASE}/#${route}`);
    await sleep(2400);
    const got = await ev(MEASURE_M);
    /* 移动端的偏离键带 @375，桌面那份登记不会顺带豁免移动端 */
    const fail = (key, msg) => {
      const gapKey = `${route}@375 ${key}`;
      if (ALLOWED_GAPS[gapKey]) {
        gapsUsed.add(gapKey);
        return;
      }
      failures.push({ route: `${route}@375`, board: want.board, key, msg });
    };

    for (const err of [...new Set(consoleErrors)].slice(0, 3)) fail("console", err);
    if (got.docOverflow > 0) fail("overflow", `整页横向溢出 ${got.docOverflow}px`);
    if (got.unknown.length > 0) fail("unknown", `未解析组件 ${got.unknown.join(", ")}`);
    if (got.deskVisible.length > 0)
      fail("shell", `375 下桌面外壳没让位：${got.deskVisible.join(" / ")}`);

    if (!got.topbar) fail("topbar", "找不到 .pt-mchrome");
    else {
      if (!near(got.topbar.h, M_TOPBAR_H, 2))
        fail("topbar", `顶栏高 ${got.topbar.h}，应为 ${M_TOPBAR_H}`);
      if (!near(got.topbar.w, M_VIEWPORT.width, 2))
        fail("topbar", `顶栏宽 ${got.topbar.w}，应为 ${M_VIEWPORT.width}`);
    }
    if (!got.title.includes(want.title))
      fail("topbar.title", `标题是「${got.title}」，画板 h1 是「${want.title}」`);
    /*
     * 摘要行。画板 30–35 每一块的 topbar 都有 sub，而它是页面 Teleport 进来的 ——
     * 桌面页头在 ≤768 整条 display:none，这一行曾经在手机上完全消失。
     */
    if (want.sub && !got.sub) fail("topbar.sub", "画板 topbar 有 sub 摘要行，手机上是空的");

    for (const icon of got.icons) {
      if (!near(icon.w, 30, 2) || !near(icon.h, 30, 2))
        fail(`topbar.icon(${icon.label})`, `钮 ${icon.w}×${icon.h}，画板 30×30`);
      if (!icon.touch) fail(`topbar.touch(${icon.label})`, "命中区不足 44×44（§9 的触控目标下限）");
    }
    for (const label of ["种子搜索", "消息通知"]) {
      if (!got.icons.some((i) => i.label.includes(label)))
        fail(`topbar.icon(${label})`, `画板 topbar 右侧有这个钮，页面上找不到`);
    }

    if (!got.nav) fail("nav", "找不到 .pt-mnav");
    else if (!near(got.nav.h, M_NAV_H, 2)) fail("nav", `底栏高 ${got.nav.h}，画板 ${M_NAV_H}`);
    if (got.tabs.length !== M_TABS.length)
      fail("nav.tabs", `底栏有 ${got.tabs.length} 个 tab，画板 ${M_TABS.length} 个`);
    for (const label of M_TABS) {
      if (!got.tabs.some((t) => t.label.includes(label)))
        fail(
          `nav.tab(${label})`,
          `底栏里没有「${label}」；实测 ${got.tabs.map((t) => t.label).join(" / ")}`,
        );
    }
    /* §9 的触控下限对 tab 同样成立：375/5 = 75 宽，高按 tab 行 48 */
    for (const tab of got.tabs) {
      if (tab.h < 44 || tab.w < 44)
        fail(`nav.touch(${tab.label})`, `tab ${tab.w}×${tab.h}，§9 要求 ≥44×44`);
    }
    if (want.activeTab && !got.tabs.some((t) => t.active && t.label.includes(want.activeTab)))
      fail(
        "nav.active",
        `这条路由应点亮「${want.activeTab}」tab；实测 ${
          got.tabs
            .filter((t) => t.active)
            .map((t) => t.label)
            .join(" / ") || "没有点亮的"
        }`,
      );

    if (got.inner) {
      if (!near(got.inner.x, M_INNER_X, 2))
        fail("inner.x", `内容列 x=${got.inner.x}，画板 ${M_INNER_X}`);
      /*
       * 画板的内容宽度是 375 - 16*2 = 343。容差走默认的 12：模拟视口里的竖向滚动条
       * 占掉 10 左右，实测 333 是滚动条占的，不是内缩写错了。
       */
      const wantW = M_VIEWPORT.width - M_INNER_X * 2;
      if (!near(got.inner.w, wantW)) fail("inner.w", `内容列宽 ${got.inner.w}，画板 ${wantW}`);
    }

    if (want.rowCards && got.rowCards < want.rowCards)
      fail("rowCards", `只有 ${got.rowCards} 张行卡，§9 要求表格降级成行卡`);
    /*
     * 筛选区的高度。画板在手机上只给一排 chip（30）或一条分段器（31），
     * 落地是把桌面工具栏整条搬了过来，控件换行堆高 —— 812 的屏上第一张卡被推得很低。
     */
    if (want.filterRowH && got.toolbar && !near(got.toolbar.h, want.filterRowH, 14))
      fail("filterRow", `筛选区高 ${got.toolbar.h}，画板 ${want.filterRowH}`);
    if (want.rowSpark && got.rowCardsWithSpark === 0)
      fail("rowCards.spark", "画板 30 的行卡里有 8 根柱的走势图，行卡里一个都没有");
    if (got.scrollers.length > 0)
      fail("scroller", `还有横向滚动的表格：${got.scrollers.join(" / ")}（§9 不允许）`);

    if (want.kpiCols) {
      if (got.kpiCols === null) fail("kpi", "找不到 .pt-kpi");
      else if (got.kpiCols !== want.kpiCols)
        fail("kpi.cols", `KPI 是 ${got.kpiCols} 列，§9 要求 ${want.kpiCols} 列（2×2）`);
      if (got.kpiNoBars.length > 0)
        fail("kpi.bars", `降级成 2×2 之后这些格丢了柱图：${got.kpiNoBars.join("、")}`);
    }
  }
}

// ---------------------------------------------------------------- 报告

const stale = Object.keys(ALLOWED_GAPS).filter(
  (k) =>
    !gapsUsed.has(k) &&
    /* --route 只跑子集时，别把没跑到的路由的偏离判成多余。移动端的键是 `<路由>@375 <项>` */
    (wanted.length === 0 || wanted.some((r) => k.startsWith(`${r} `) || k.startsWith(`${r}@375 `))),
);

for (const f of failures) {
  console.log(`✗ ${f.route.padEnd(28)} [画板 ${f.board}] ${f.key}: ${f.msg}`);
}
for (const k of gapsUsed) {
  console.log(`· 已记偏离 ${k} —— ${ALLOWED_GAPS[k]}`);
}
for (const k of stale) {
  console.log(`? 偏离登记多余（这一项其实已经对上了，删掉它）：${k}`);
}

/* 桌面与移动分开报数：把两者合成一个数字会让「覆盖了多少画板」重新变得含糊 */
const scope = `桌面 ${routes.length} 条 + 移动 ${mobileRoutes.length} 条`;
if (failures.length === 0 && stale.length === 0) {
  console.log(`\n${scope} 与画板一致（${gapsUsed.size} 条已记偏离）`);
} else {
  console.log(`\n${scope}：${failures.length} 处与画板不一致，${stale.length} 条偏离登记多余`);
}

chrome.kill("SIGKILL");
process.exit(failures.length === 0 && stale.length === 0 ? 0 : 1);
