/// <reference types="node" />
/**
 * 钉子：八套配色里承载信息的文字都达到 WCAG AA（4.5:1）。
 *
 * 画板 G 的原值有一批不到 AA：t3（表头、时间、标签）2.9–4.4，cockpit / halo 浅色的主色作文字
 * 3.3–3.4、白字主按钮 3.8–3.9，hover / 按下态低到 2.9，几种语义色的胶囊 3.2–4.45。
 * 发布前 owner 让「你来决定实施」，于是只动不达标的令牌、只动 OKLCH 明度（色相与彩度不变），
 * 挪到刚好过线 —— 新旧值对照见 docs/design/webui-board-spec.md 的「发布前对比度调整」。
 *
 * t4 不在这里：它只留给装饰（分隔点）与禁用态，承载信息的地方都已改用 t3。
 * 算法与 shellTokens.test.ts 相同（sRGB 相对亮度；淡染按 sRGB 分量线性合成，和浏览器的 color-mix 一致）。
 */
import { readFileSync } from "node:fs";
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

    it(`${name}：t2 / t3 在四种底色上`, () => {
      expect(worst(v["pt-t2"]!, surfaces as Array<[string, string]>)).toBeNull();
      expect(worst(v["pt-t3"]!, surfaces as Array<[string, string]>)).toBeNull();
    });

    it(`${name}：主色作文字、作胶囊，主按钮三态上的字`, () => {
      const p = v["pt-p"]!;
      const onp = ref(v["pt-on-p"]!);
      expect(worst(p, surfaces as Array<[string, string]>)).toBeNull();
      expect(contrast(p, blend(p, v["pt-surface"]!, 0.14))).toBeGreaterThanOrEqual(AA);
      for (const k of ["pt-p", "pt-p-hover", "pt-p-active"]) {
        expect(contrast(onp, v[k]!), `on-p 压在 ${k} 上`).toBeGreaterThanOrEqual(AA);
      }
    });

    it(`${name}：语义色作文字与胶囊`, () => {
      for (const t of ["ok", "warn", "dang", "info"]) {
        const c = v[`pt-${t}`]!;
        expect(worst(c, surfaces as Array<[string, string]>), `${t} 作文字`).toBeNull();
        expect(contrast(c, blend(c, v["pt-surface"]!, 0.14)), `${t} 胶囊`).toBeGreaterThanOrEqual(
          AA,
        );
      }
    });
  }
});
