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
 * 用法（需要一个已经跑起来的 pt-tools 服务，前端 dist 必须是当前源码构建出来的；
 * 用一个临时 HOME 起服务，首次启动的初始账号就能登录）：
 *   HOME=$(mktemp -d) ./pt-tools web --host 127.0.0.1 --port 18310 &
 *   node scripts/docs-screens.mjs http://127.0.0.1:18310 [输出目录，默认为仓库的 docs/public/screens]
 *
 * 换了截图尺寸要同步改 docs/index.md 与 docs/en/index.md 里 home.visual / home.shots 的
 * width、height。提交前逐张看一遍图。
 *
 * 依赖：只用 Chrome 的 CDP，不装任何浏览器自动化包。Chrome 路径可用 PT_CHROME 覆盖；
 * 登录账号可用 PT_USER / PT_PASS 覆盖。
 */
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { stubScript } from "./board-fixtures.mjs";

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

/* [路由, 宽, 高, 移动端, 文件名前缀] */
const SHOTS = [
  ["/userinfo", 1440, 900, false, "home"],
  ["/sites", 1440, 900, false, "sites"],
  ["/userinfo", 375, 812, true, "home-mobile"],
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
    stubScript() +
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
} catch (e) {
  console.error(e instanceof Error ? e.message : e);
  failed = true;
} finally {
  chrome.kill("SIGKILL");
}
process.exit(failed ? 1 : 0);
