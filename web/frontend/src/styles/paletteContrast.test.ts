/// <reference types="node" />
/**
 * 钉子：八套配色里承载信息的文字都达到 WCAG AA（4.5:1）。
 *
 * 画板 G 的原值有一批不到 AA：t3（表头、时间、标签）2.9–4.4，cockpit / halo 浅色的主色作文字
 * 3.3–3.4、白字主按钮 3.8–3.9，hover / 按下态低到 2.9，几种语义色的胶囊 3.2–4.45。
 * 发布前 owner 让「你来决定实施」，于是只动不达标的令牌、只动 OKLCH 明度（色相与彩度不变），
 * 挪到刚好过线 —— 新旧值对照见 docs/design/webui-board-spec.md 的「发布前对比度调整」。
 *
 * 胶囊按它常驻的两种底校：表格与卡片里是 surface，手机行卡与浮层里是 raised。行 hover / 按下是
 * 瞬时的整行高亮，不作为约束（把它也算进去要再动 27 枚令牌，离画板更远）。
 *
 * t4 不在这里：它只留给装饰（分隔点）与禁用态，承载信息的地方都已改用 t3。
 * 算法与 shellTokens.test.ts 相同（sRGB 相对亮度；淡染按 sRGB 分量线性合成，和浏览器的 color-mix 一致）。
 */
import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const theme = readFileSync(fileURLToPath(new URL("./theme.scss", import.meta.url)), "utf8");
const AA = 4.5;

function schemes(): Array<{ name: string; vars: Record<string, string> }> {
  const out: Array<{ name: string; vars: Record<string, string> }> = [];
  for (const m of theme.matchAll(/^(html\.[^\n{]+)\{([\s\S]*?)^\}/gm)) {
    const vars: Record<string, string> = {};
    for (const v of m[2]!.matchAll(/--([a-z0-9-]+):\s*([^;]+);/g)) vars[v[1]!] = v[2]!.trim();
    if (vars["pt-chrome"]) out.push({ name: m[1]!.trim().split(",").pop()!.trim(), vars });
  }
  return out;
}

const channels = (hex: string) => [1, 3, 5].map((i) => Number.parseInt(hex.slice(i, i + 2), 16));

function luminance(hex: string): number {
  const lin = (c: number) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
  const [r, g, b] = channels(hex).map((c) => lin(c / 255));
  return 0.2126 * r! + 0.7152 * g! + 0.0722 * b!;
}

function contrast(a: string, b: string): number {
  const [l1, l2] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (l1! + 0.05) / (l2! + 0.05);
}

function blend(fg: string, bg: string, alpha: number): string {
  const [f, b] = [channels(fg), channels(bg)];
  return (
    "#" +
    f
      .map((v, i) => Math.round(v * alpha + b[i]! * (1 - alpha)))
      .map((v) => v.toString(16).padStart(2, "0"))
      .join("")
  );
}

describe("八套配色的文字对比度 ≥ 4.5", () => {
  const all = schemes();

  it("八套都在", () => {
    expect(all.length).toBe(8);
  });

  for (const { name, vars: v } of all) {
    const ref = (x: string) => (x.startsWith("var(--") ? v[x.slice(6, -1)]! : x);
    const surfaces = ["pt-canvas", "pt-surface", "pt-raised", "pt-hover"].map((k) => [k, v[k]!]);
    /** 最差的那一处：返回「底色名 比值」，全部达标时为 null */
    const worst = (fg: string, pairs: Array<[string, string]>) => {
      const low = pairs.map(([k, bg]) => [k, contrast(fg, bg)] as const).filter(([, r]) => r < AA);
      return low.length ? low.map(([k, r]) => `${k} ${r.toFixed(2)}`).join("、") : null;
    };

    /** 胶囊：字色压在自己 14% 的淡染上，淡染叠在 surface / raised 上 */
    const pillOk = (c: string) =>
      ["pt-surface", "pt-raised"].every((bg) => contrast(c, blend(c, v[bg]!, 0.14)) >= AA);

    it(`${name}：t2 / t3 在四种底色上，t2 作中性胶囊`, () => {
      expect(worst(v["pt-t2"]!, surfaces as Array<[string, string]>)).toBeNull();
      expect(worst(v["pt-t3"]!, surfaces as Array<[string, string]>)).toBeNull();
      expect(pillOk(v["pt-t2"]!), "中性胶囊").toBe(true);
    });

    /* 状态栏 / 手机底栏的陈旧态把字色降到 t3，坐在 shell-bg 上（浅色 = 白，深色 = chrome） */
    it(`${name}：t3 在 shell-bg 上（状态栏陈旧态）`, () => {
      expect(contrast(v["pt-t3"]!, ref(v["pt-shell-bg"]!))).toBeGreaterThanOrEqual(AA);
    });

    it(`${name}：主色作文字、作胶囊，主按钮三态上的字`, () => {
      const p = v["pt-p"]!;
      const onp = ref(v["pt-on-p"]!);
      expect(worst(p, surfaces as Array<[string, string]>)).toBeNull();
      expect(pillOk(p), "主色胶囊").toBe(true);
      for (const k of ["pt-p", "pt-p-hover", "pt-p-active"]) {
        expect(contrast(onp, v[k]!), `on-p 压在 ${k} 上`).toBeGreaterThanOrEqual(AA);
      }
    });

    it(`${name}：语义色作文字与胶囊`, () => {
      for (const t of ["ok", "warn", "dang", "info"]) {
        const c = v[`pt-${t}`]!;
        expect(worst(c, surfaces as Array<[string, string]>), `${t} 作文字`).toBeNull();
        expect(pillOk(c), `${t} 胶囊`).toBe(true);
      }
    });
  }
});

/*
 * t4 只给装饰与禁用：分隔符、排序箭头、禁用态。承载信息的文字一律 t3 ——
 * 发布前的审阅里 35 处提示 / 脚注 / 标签 / 空值「-」用的是 t4（1.8–2.3:1），全部改成了 t3。
 */
describe("t4 不用来写承载信息的文字", () => {
  const SFC = import.meta.glob("/src/**/*.vue", {
    query: "?raw",
    import: "default",
    eager: true,
  }) as Record<string, string>;
  const stylesDir = fileURLToPath(new URL(".", import.meta.url));
  const sources: Array<[string, string]> = [
    ...readdirSync(stylesDir)
      .filter((f) => /\.(css|scss)$/.test(f))
      .map((f) => [`styles/${f}`, readFileSync(`${stylesDir}/${f}`, "utf8")] as [string, string]),
    ...Object.entries(SFC).map(([f, src]) => [f.replace("/src/", ""), src] as [string, string]),
  ];
  const ALLOWED = /sep\b|:disabled|caret/;

  it("每一处 color: var(--pt-t4) 都落在分隔符 / 禁用态 / 排序箭头上", () => {
    const bad: string[] = [];
    for (const [file, src] of sources) {
      const lines = src.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, " ")).split("\n");
      lines.forEach((line, i) => {
        if (!/(^|[\s;{])color:\s*var\(--pt-t4\)/.test(line)) return;
        let selector = "";
        for (let j = i; j >= 0; j--) {
          const t = lines[j]!.trim();
          if (t.endsWith("{")) {
            selector = t.slice(0, -1).trim();
            break;
          }
        }
        if (!ALLOWED.test(selector)) bad.push(`${file}:${i + 1} ${selector}`);
      });
    }
    expect(bad).toEqual([]);
  });
});

/*
 * 不用 opacity 压暗承载信息的文字：opacity 合成后的对比度不在任何令牌上，令牌测试查不到它。
 * 发布前的门禁抓到状态栏陈旧态整格压 0.5（2.2–2.9:1）—— 数据正常时的扫描从来触发不到。
 * 允许的只有禁用态、箭头、头像、图标，以及导出海报的 DOM 复刻（它必须与 PNG 画布逐像素一致，属于图片内容）。
 */
describe("opacity 不用来压暗承载信息的文字", () => {
  const SFC = import.meta.glob("/src/**/*.vue", {
    query: "?raw",
    import: "default",
    eager: true,
  }) as Record<string, string>;
  const stylesDir = fileURLToPath(new URL(".", import.meta.url));
  const sources: Array<[string, string]> = [
    ...readdirSync(stylesDir)
      .filter((f) => /\.(css|scss)$/.test(f))
      .map((f) => [`styles/${f}`, readFileSync(`${stylesDir}/${f}`, "utf8")] as [string, string]),
    ...Object.entries(SFC).map(([f, src]) => [f.replace("/src/", ""), src] as [string, string]),
  ];
  const ALLOWED =
    /:disabled|caret|avatar|__av\b|__icon\b|\.poster|\.pstat|\.pcard|^\d+%$|^from$|^to$/;

  it("每一处半透明 opacity 都落在禁用态 / 箭头 / 头像 / 图标 / 导出海报上", () => {
    const bad: string[] = [];
    for (const [file, src] of sources) {
      const lines = src.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, " ")).split("\n");
      lines.forEach((line, i) => {
        if (!/(^|[\s;{])opacity:\s*0?\.[0-9]/.test(line)) return;
        let selector = "";
        for (let j = i; j >= 0; j--) {
          const t = lines[j]!.trim();
          if (t.endsWith("{")) {
            selector = t.slice(0, -1).trim();
            break;
          }
        }
        if (!ALLOWED.test(selector)) bad.push(`${file}:${i + 1} ${selector}`);
      });
    }
    expect(bad).toEqual([]);
  });
});
