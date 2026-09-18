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
    titles: ["上传构成", "等级分布", "需要关注"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
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
    titles: ["站点命中", "没返回的站点", "搜索历史"],
    needsSeg: true, // 画板 bar-88 的分类分段
    controls: ["搜索", "保存搜索", "仅免费"], // search-head 的 go / ha-1 与 bar-88 的 chip // 画板这一页的卡（标题身份，防同宽卡互相顶替）
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
    titles: ["基本信息", "凭证与连接", "连通性测试", "操作提示文案"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
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
    titles: ["Manager 连接", "探测结果", "页面状态"], // 画板这一页的卡（标题身份，防同宽卡互相顶替）
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

async function goto(url) {
  await cdp.send("Page.navigate", { url }, sessionId);
  for (let i = 0; i < 80; i++) {
    await sleep(250);
    const ready = await ev(
      `document.readyState === 'complete' && !!document.querySelector('#app > *, form')`,
    ).catch(() => false);
    if (ready === true) return;
  }
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
    /* 关键控件：按钮/分段这类「画板上画了、少了就不算落地」的东西 */
    controlText: [...document.querySelectorAll('button, .el-segmented__item, .pt-band__tab')]
      .map((el) => (el.textContent ?? '').replace(/\\s+/g, ' ').trim())
      .filter(Boolean),
    hasSeg: Boolean(document.querySelector('.pt-seg, .el-segmented')),
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
        out.push({
          w: Math.round(r.width),
          title: (el.querySelector('.pt-panel__title')?.textContent ?? '').trim(),
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

// ---------------------------------------------------------------- 报告

const stale = Object.keys(ALLOWED_GAPS).filter(
  (k) => !gapsUsed.has(k) && (wanted.length === 0 || wanted.some((r) => k.startsWith(`${r} `))),
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

const checked = routes.length;
if (failures.length === 0 && stale.length === 0) {
  console.log(`\n${checked} 条路由与画板一致（${gapsUsed.size} 条已记偏离）`);
} else {
  console.log(
    `\n${checked} 条路由：${failures.length} 处与画板不一致，${stale.length} 条偏离登记多余`,
  );
}

chrome.kill("SIGKILL");
process.exit(failures.length === 0 && stale.length === 0 ? 0 : 1);
