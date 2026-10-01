/**
 * 外壳令牌的两枚钉子。都来自 d698e90 那一轮评审里被证实的缺陷：
 *
 * 1. 强调色 --pt-p 直接压在 chrome（深色侧栏）上：deck 浅色里 p 与 chrome 是同一个色值
 *    （#18181b），选中项的字、徽标、3px 条全部消失；atlas 浅色的紫色在深底上只有 2.6:1。
 *    每套配色都必须给一个 --pt-chrome-p，且与该套的 --pt-chrome 对比 ≥ 4.5:1（12.5px 正文）。
 *
 * 2. `visibility 0s linear var(--pt-transition-normal)`：令牌展开是「240ms cubic-bezier(…)」，
 *    一条 <single-transition> 里就有了两个缓动函数，整条 transition 简写在计算值阶段作废 ——
 *    实测 rail 的 transitionDuration 是 0s，交叉淡入淡出只在四个方向里的一个成立。
 *    所以时长要单独成令牌（--pt-dur-*），shell.css 里不允许再把 --pt-transition-* 当时长用。
 */
/// <reference types="node" />
/*
 * 这里必须直接读文件：vitest 把所有样式 import（连 `?raw`）都换成空串，而 views.*.test.ts
 * 那招 `import.meta.glob(..., { query: "?raw" })` 只对 .vue 有效。tsconfig.app.json 的 types
 * 只有 vite/client，所以用三斜线指令把 @types/node（tsconfig.node.json 已依赖）拉进来。
 */
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const read = (rel: string) => readFileSync(fileURLToPath(new URL(rel, import.meta.url)), "utf8");
const theme = read("./theme.scss");
const shell = read("./shell.css");

/** 把 theme.scss 按 `html.xxx[...] {` 块切开，取每块里的令牌 */
function schemes(): Array<{ name: string; vars: Record<string, string> }> {
  const out: Array<{ name: string; vars: Record<string, string> }> = [];
  const re = /^(html\.[^\n{]+)\{([\s\S]*?)^\}/gm;
  for (const m of theme.matchAll(re)) {
    const vars: Record<string, string> = {};
    for (const v of m[2]!.matchAll(/--([a-z0-9-]+):\s*([^;]+);/g)) vars[v[1]!] = v[2]!.trim();
    if (vars["pt-chrome"]) out.push({ name: m[1]!.trim(), vars });
  }
  return out;
}

function luminance(hex: string): number {
  const h = hex.replace("#", "");
  const full =
    h.length === 3
      ? h
          .split("")
          .map((c) => c + c)
          .join("")
      : h;
  const [r, g, b] = [0, 2, 4].map((i) => Number.parseInt(full.slice(i, i + 2), 16) / 255);
  const lin = (c: number) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  return 0.2126 * lin(r!) + 0.7152 * lin(g!) + 0.0722 * lin(b!);
}

function contrast(a: string, b: string): number {
  const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (l1! + 0.05) / (l2! + 0.05);
}

/** 把 fg 以 alpha 叠在 bg 上得到的实际颜色（sRGB 分量线性插值，和浏览器合成 opacity / color-mix 一致） */
function blend(fg: string, bg: string, alpha: number): string {
  const ch = (hex: string) => [1, 3, 5].map((i) => Number.parseInt(hex.slice(i, i + 2), 16));
  const [f, b] = [ch(fg), ch(bg)];
  return (
    "#" +
    f
      .map((v, i) => Math.round(v * alpha + b[i]! * (1 - alpha)))
      .map((v) => v.toString(16).padStart(2, "0"))
      .join("")
  );
}

describe("chrome 上的强调色", () => {
  const all = schemes();

  it("八套配色都在", () => {
    expect(all.length).toBe(8);
  });

  for (const s of all) {
    it(`${s.name} 有 --pt-chrome-p 且与 chrome 对比 ≥ 4.5`, () => {
      const p = s.vars["pt-chrome-p"];
      expect(p, "缺 --pt-chrome-p").toMatch(/^#[0-9a-f]{6}$/i);
      expect(contrast(p!, s.vars["pt-chrome"]!)).toBeGreaterThanOrEqual(4.5);
    });
  }

  /*
   * 状态栏链接与移动 tab 选中态原先坐在 chrome 上、用 --pt-chrome-p。发布前 owner 定了状态栏与移动端顶栏 / 底栏
   * 「改为跟随主题的浅色」，它们改走 --pt-shell-p（深色配色里就是 chrome-p 的值）—— 仍然不许直接用 --pt-p。
   */
  it("状态栏链接与移动 tab 选中态用 --pt-shell-p，不用 --pt-p", () => {
    const tab = shell.match(/\.pt-mnav__tab\.is-active \{[^}]*\}/)?.[0] ?? "";
    const copy = shell.match(/\.pt-status__copy a \{[^}]*\}/)?.[0] ?? "";
    for (const block of [tab, copy]) {
      expect(block).not.toBe("");
      expect(block).not.toMatch(/var\(--pt-p\)/);
      expect(block).toMatch(/var\(--pt-shell-p\)/);
    }
  });

  it("状态栏与移动端顶栏 / 底栏只吃 shell-*，不直接吃 chrome-*（明亮主题下跟着变浅）", () => {
    const code = shell.replace(/\/\*[\s\S]*?\*\//g, "");
    const rules = [...code.matchAll(/(\.pt-(?:status|mchrome|mnav)[^{}]*)\{([^}]*)\}/g)];
    expect(rules.length).toBeGreaterThan(10);
    const leaks = rules.filter((m) => /var\(--pt-chrome/.test(m[2]!)).map((m) => m[1]!.trim());
    expect(leaks).toEqual([]);
  });
});

/*
 * 侧栏跟主题走：明亮主题下是浅色面，深色主题下是 chrome。用户原话「明亮主题下 侧栏导航也应该是浅色吧」。
 * 所以侧栏不再直接吃 chrome-*，而是吃一组 shell-*：浅色配色里映射到 surface / hover / border / t1 / t2，
 * 深色配色里映射到 chrome 那一族。每套配色都必须给全，且字与强调色对底 ≥ 4.5:1。
 */
describe("侧栏面（shell-*）跟主题走", () => {
  const all = schemes();
  const KEYS = [
    "pt-shell-bg",
    "pt-shell-raised",
    "pt-shell-border",
    "pt-shell-t1",
    "pt-shell-t2",
    "pt-shell-p",
  ];

  for (const s of all) {
    const light = s.name.startsWith("html.light");
    it(`${s.name} 六枚 shell-* 都在，且 ${light ? "是浅色面" : "是深色面"}`, () => {
      for (const k of KEYS) expect(s.vars[k], `缺 --${k}`).toMatch(/^#[0-9a-f]{6}$/i);
      const bgLum = luminance(s.vars["pt-shell-bg"]!);
      if (light) expect(bgLum).toBeGreaterThan(0.5);
      else expect(bgLum).toBeLessThan(0.2);
    });
    it(`${s.name} 侧栏文字与强调色对**实际渲染面** ≥ 4.5`, () => {
      const bg = s.vars["pt-shell-bg"]!;
      const p = s.vars["pt-shell-p"]!;
      expect(contrast(s.vars["pt-shell-t1"]!, bg)).toBeGreaterThanOrEqual(7);
      expect(contrast(s.vars["pt-shell-t2"]!, bg)).toBeGreaterThanOrEqual(4.5);
      /*
       * 选中项的字与徽标的字不是坐在裸底上，而是坐在 shell-p 自己 14% / 16% 的淡染上 ——
       * 上一版只对裸底量，cockpit / halo / atlas 浅色实际是 4.24–4.48，测试却全绿。
       * 选中项里的徽标翻成实底（p 底 + bg 字），所以不存在 16% 叠 14% 的情形，另测一条 bg 对 p。
       */
      expect(contrast(p, blend(p, bg, 0.14)), "选中项字对 14% 淡染").toBeGreaterThanOrEqual(4.5);
      expect(contrast(p, blend(p, bg, 0.16)), "徽标字对 16% 淡染").toBeGreaterThanOrEqual(4.5);
      expect(contrast(bg, p), "选中项里翻成实底的徽标：bg 字对 p 底").toBeGreaterThanOrEqual(4.5);
      /* 组标题是 shell-t2 叠 --pt-shell-dim 的 opacity；10px/600 是小字，按混合后的实际颜色量 */
      const dim = Number.parseFloat(shell.match(/--pt-shell-dim:\s*(0?\.\d+|1)/)?.[1] ?? "NaN");
      expect(dim, "shell.css 里要有 --pt-shell-dim").not.toBeNaN();
      expect(
        contrast(blend(s.vars["pt-shell-t2"]!, bg, dim), bg),
        "组标题（t2 × dim）对底",
      ).toBeGreaterThanOrEqual(4.5);
    });
  }

  it("shell.css 的侧栏（.pt-side / rail / nav）只吃 shell-*，不直接吃 chrome-* 或 --pt-p", () => {
    const code = shell.replace(/\/\*[\s\S]*?\*\//g, "");
    // 从「网格骨架」里的 .pt-side 起，到 nav 账号页脚 .pt-nav__empty 结束，是侧栏的全部规则
    const start = code.indexOf(".pt-side {");
    const end = code.indexOf(".pt-nav__empty {");
    expect(start).toBeGreaterThan(0);
    expect(end).toBeGreaterThan(start);
    const side = code.slice(start, end);
    expect(side).not.toMatch(/var\(--pt-chrome/);
    expect(side).not.toMatch(/var\(--pt-p\)/);
    for (const sel of [
      ".pt-side {",
      ".pt-rail__item.is-active {",
      ".pt-nav__item.is-active {",
      ".pt-nav__badge {",
    ]) {
      expect(side).toContain(sel);
    }
    const railActive = side.match(/\.pt-rail__item\.is-active \{[^}]*\}/)?.[0] ?? "";
    const navActive = side.match(/\.pt-nav__item\.is-active \{[^}]*\}/)?.[0] ?? "";
    const badge = side.match(/\.pt-nav__badge \{[^}]*\}/)?.[0] ?? "";
    for (const block of [railActive, navActive, badge])
      expect(block).toMatch(/var\(--pt-shell-p\)/);
    // 选中项里的徽标翻成实底，避免 16% 淡染叠在 14% 淡染上把对比再压掉一档
    expect(side).toMatch(
      /\.pt-nav__item\.is-active \.pt-nav__badge \{[^}]*color: var\(--pt-shell-bg\)[^}]*background: var\(--pt-shell-p\)/s,
    );
  });
});

/*
 * brand.md 硬性规则落在侧栏上的两条：彩色版「自带底板，不要再套一层背景」；
 * 「四周留白 ≥ 标志高度的 25%，从底板边缘量起，彩色版自身的 12.5% 内缩不算」。
 * 32 的盒装 28 的 logo，外部留白 = 2 + 间距，要 ≥ 7 就得间距 ≥ 5；品牌行的 gap 用 8。
 */
describe("侧栏里的彩色 logo 守 brand.md 的硬性规则", () => {
  const code = shell.replace(/\/\*[\s\S]*?\*\//g, "");
  it("品牌盒没有 hover 底", () => {
    expect(code).not.toMatch(/\.pt-rail__brand:hover/);
    expect(code).not.toMatch(/\.pt-nav__logo:hover/);
  });
  it("品牌行的间距 ≥ 5，使底板到字标的外部留白 ≥ 7", () => {
    const brand = code.match(/\.pt-nav__brand \{[^}]*\}/)?.[0] ?? "";
    const gap = Number.parseInt(brand.match(/gap:\s*(\d+)px/)?.[1] ?? "0", 10);
    expect(gap).toBeGreaterThanOrEqual(5);
  });
});

describe("外壳过渡简写", () => {
  it("时长令牌存在", () => {
    expect(theme).toMatch(/--pt-dur-normal:\s*\d+ms;/);
  });

  it("shell.css 不把 --pt-transition-* 当时长/延迟塞进 visibility 简写", () => {
    // `visibility 0s … var(--pt-transition-normal)` 展开后有两个缓动函数 → 整条作废。
    // 注释里会原样引用这个错误写法来解释它，所以先把注释剥掉再查。
    const code = shell.replace(/\/\*[\s\S]*?\*\//g, "");
    expect(code).not.toMatch(/visibility\s+0s[^;,]*var\(--pt-transition-/);
  });

  it("rail 的露出态也覆盖了 visibility 的延迟（否则先透明 240ms 再弹出）", () => {
    expect(shell).toMatch(
      /\.pt-shell:not\(\.is-nav-docked\):not\(\.is-nav-open\)\s+\.pt-rail\s*\{[^}]*visibility 0s/,
    );
  });
});
