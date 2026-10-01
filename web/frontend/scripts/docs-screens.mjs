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

/* [路由, 宽, 高, 移动端, 文件名前缀] */
const SHOTS = [
  ["/userinfo", 1440, 900, false, "home"],
  ["/sites", 1440, 900, false, "sites"],
  ["/userinfo", 375, 812, true, "home-mobile"],
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

/* 假数据，外加一个记录「最后一次 DOM 变化」的观察器：等页面安定靠它，不靠固定时长 */
await cmd("Page.addScriptToEvaluateOnNewDocument", {
  source:
    stubScript({ ...VERSION_STUBS, ...GUIDE_STUBS }) +
    `
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

async function viewport(width, height, mobile) {
  await cmd("Emulation.setDeviceMetricsOverride", { width, height, deviceScaleFactor: 1, mobile });
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
    for (const [route, width, height, mobile, name] of SHOTS) {
      await viewport(width, height, mobile);
      await goto(route);
      await cmd("Page.reload");
      await settled();
      await closeNotifications();
      const shot = await cmd("Page.captureScreenshot", { format: "webp", quality: 85 });
      const file = join(outDir, `${name}-${mode}.webp`);
      writeFileSync(file, Buffer.from(shot.data, "base64"));
      console.log(file);
    }
  }

  await prepare("light");
  await viewport(1440, 900, false);
  for (const [route, name, action] of GUIDE_SHOTS) {
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
