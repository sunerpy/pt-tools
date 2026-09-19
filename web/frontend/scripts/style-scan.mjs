#!/usr/bin/env node
/*
 * 扫「模板里写了、样式里没有」的类名。
 *
 * 真实缺陷：运行日志页的 `.lf / .lf__row / .lf__k / .lf__v` 只写在模板里，样式块里一条
 * 都没有 —— `<ul>` 于是用浏览器默认的圆点列表渲染，键和值之间没有间距，读出来是
 * 「当前文件all.log」「大小2.3 MB」。`pnpm build` 绿、`vue-tsc -b` 绿、单测绿、
 * 画板验收也绿（它量带与卡的几何，量不到卡内部），只有看截图才发现。
 *
 * 为什么是独立脚本而不是一条 vitest：vitest 默认不处理 CSS，`import.meta.glob` 拿
 * `.css` 的 `?raw` 会得到**空串** —— 全局样式表因此看不见，`.progress-text` 这种
 * 定义在 src/styles/shared-components.css 里的类会被全部误判成「没实现」。
 * 而 tsconfig.app.json 的 types 里没有 @types/node，测试里又不能用 node:fs。
 *
 * 判据刻意收得很窄，宁可漏报不误报：只有当一个类名
 *   ① 全仓的 .css / .scss 与所有 SFC 的 <style> 块里都找不到 `.<类名>`，并且
 *   ② 除了这一个 SFC 的模板之外，任何文件里都没再出现过
 * 才算「写了没实现」。设计系统前缀（pt-)、Element（el-）、状态修饰符（is-/has-）
 * 一律跳过：它们的样式本来就在别处。
 *
 * 用法：node scripts/style-scan.mjs
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const SRC = join(ROOT, "src");

/** 设计系统 / 框架 / 状态修饰符的前缀，样式不在页面自己身上 */
const OWNED = [/^pt-/, /^el-/, /^is-/, /^has-/, /^v-/];

/**
 * 故意没有样式的类名 —— 键是 `<相对路径> .<类名>`，值是原因。
 *
 * 这些不是漏写，是**语义锚点**：布局由同一个节点上的 `pt-cards*` 原子类、
 * 由父组件、或由子元素自己给。加一条就等于承认「这个名字只为可读性/查找存在」，
 * 所以必须写清布局到底由谁给。写错了的话下面的「登记多余」会提醒删掉。
 */
const INTENTIONAL = {
  /* 页面根节点：布局全部来自同节点上的 pt-cards* 原子类，这个名字只用于定位与阅读 */
  "src/views/AutoCleanup.vue .cleanup-page": "布局由同节点的 pt-cards--wide 给",
  "src/views/CloakBrowserConfig.vue .cloak-page": "布局由同节点的 pt-cards--main 给",
  "src/views/DownloaderSettings.vue .downloader-page": "布局由同节点的 pt-cards--wide 给",
  "src/views/DownloaderSettings.vue .dl-cards": "布局由同节点的 pt-cards--2 给",
  "src/views/SupportedSites.vue .site-grid": "布局由同节点的 pt-cards--2 给",
  "src/views/chatops/Bindings.vue .bindings-page": "布局由同节点的 pt-cards--lead 给",
  "src/views/chatops/NotificationDetail.vue .notify-detail-page":
    "布局由同节点的 pt-cards--wide 给",
  "src/views/UserDataExport.vue .export-page":
    "只用来挂 SVG 滤镜与 Teleport（文件内注释已说明），栏宽由里层 .export-cols pt-cards--main 给",

  /* BEM 的外层壳：间距与排布都在子元素上 */
  "src/components/ui/PtBreakdown.vue .bd": "排布在 .bd__list 上（flex/grid 都在那里）",
  "src/views/SupportedSites.vue .cap__col": "三栏由父级 .cap 的栅格给，列自身不需要样式",
  "src/views/chatops/AuditLog.vue .args": "内部的 .args__head / .args__json 自带样式",
  "src/components/VersionChecker.vue .update-header": "行内元素自带样式，壳不需要",

  /* 组件实例上的钩子：卡片外观由 PtPanel 给，类名只用于定位 */
  "src/views/SiteDetail.vue .rss-card": "PtPanel 自带卡片外观，这个名字只用于定位",
  "src/views/SiteDetail.vue .cred-card": "PtPanel 自带卡片外观，这个名字只用于定位",
  "src/components/downloader/DownloaderTorrentVirtualTable.vue .vt-action-header":
    "列宽与对齐由同节点的 .vt-cell / .align-center 给",
  "src/components/downloader/DownloaderTorrentVirtualTable.vue .vt-action-cell":
    "列宽与对齐由同节点的 .vt-cell / .align-center 给",

  /* 其余 */
  "src/components/VersionChecker.vue .initial": "默认态，样式由 .status-state 给，不需要单独一条",
  "src/views/LogViewer.vue .virtual-spacer": "高度是虚拟滚动算出来的行内样式，没有静态样式",
  "src/components/V2DeprecationBanner.vue .v2-deprecation-alert":
    "外观由同节点的 pt-note 给，这个名字只用于定位",
};

function walk(dir) {
  const out = [];
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) out.push(...walk(path));
    else out.push(path);
  }
  return out;
}

const files = walk(SRC);
const vueFiles = files.filter((f) => f.endsWith(".vue"));
const styleFiles = files.filter((f) => f.endsWith(".css") || f.endsWith(".scss"));

const read = (f) => readFileSync(f, "utf8");

/** 所有样式文本：全局样式表 + 每个 SFC 的 style 块 */
const styleText = [
  ...styleFiles.map(read),
  ...vueFiles.map((f) => {
    const src = read(f);
    const i = src.indexOf("<style");
    return i < 0 ? "" : src.slice(i);
  }),
].join("\n");

/** 模板里出现的静态类名：`class="a b"` 与 `:class="{ 'a': x }"` 两种写法 */
function templateClasses(source) {
  const match = /<template>([\s\S]*)\n<\/template>/.exec(source);
  if (!match) return [];
  const template = match[1].replace(/<!--[\s\S]*?-->/g, "");
  const out = new Set();
  for (const m of template.matchAll(/\sclass="([^"]*)"/g)) {
    for (const cls of m[1].split(/\s+/)) if (cls && !cls.includes("{")) out.add(cls);
  }
  for (const m of template.matchAll(/:class="\{([^}]*)\}"/g)) {
    for (const k of m[1].matchAll(/['"]?([A-Za-z][\w-]*)['"]?\s*:/g)) out.add(k[1]);
  }
  return [...out];
}

const cache = new Map();
const bodyOf = (f) => {
  if (!cache.has(f)) cache.set(f, read(f));
  return cache.get(f);
};

const selector = (cls) => new RegExp(`\\.${cls}(?![\\w-])`);
const anywhere = (cls) => new RegExp(`(^|[\\s"'.])${cls}(?![\\w-])`);

const findings = [];
const used = new Set();
for (const file of vueFiles) {
  const source = bodyOf(file);
  const rel = relative(ROOT, file);
  for (const cls of templateClasses(source)) {
    if (OWNED.some((re) => re.test(cls))) continue;
    if (selector(cls).test(styleText)) continue;
    const elsewhere =
      vueFiles.some((f) => f !== file && anywhere(cls).test(bodyOf(f))) ||
      styleFiles.some((f) => anywhere(cls).test(bodyOf(f)));
    if (elsewhere) continue;
    const key = `${rel} .${cls}`;
    if (INTENTIONAL[key]) {
      used.add(key);
      continue;
    }
    findings.push({ file: rel, cls });
  }
}

/* 登记多余：这一条现在有样式了（或类名删了），登记留着只会掩盖下一次真缺陷 */
const stale = Object.keys(INTENTIONAL).filter((k) => !used.has(k));

for (const f of findings) {
  console.log(`✗ ${f.file} 的 .${f.cls} 只写在模板里，样式里没有这个类`);
}
for (const k of stale) {
  console.log(`? 登记多余（这一条已经有样式或类名不在了，删掉它）：${k}`);
}
const bad = findings.length + stale.length;
console.log(
  bad === 0
    ? `${vueFiles.length} 个 SFC：模板里的类名都有样式（${used.size} 个语义锚点已登记）`
    : `${vueFiles.length} 个 SFC：${findings.length} 个类名写了没实现，${stale.length} 条登记多余`,
);
process.exit(bad === 0 ? 0 : 1);
