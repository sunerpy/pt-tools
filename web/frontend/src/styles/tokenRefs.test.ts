/// <reference types="node" />
/**
 * 每一处 `var(--pt-xxx)` 都必须能找到 `--pt-xxx` 的定义。
 *
 * 引用一个没定义的自定义属性不会报任何错：整条声明在计算值阶段作废，属性回落成继承值。
 * 发布前的审阅在三处找到过这种声明：两处分段写的 --pt-fz-xs、通道详情标题写的 --pt-fz-h3，
 * 都从来没定义过 —— 标题因此一直是 13 号而不是画板的 19 号，没有任何检查能看出来。
 *
 * 定义来源有三种：样式文件里的 `--pt-xxx:` 声明、SFC 的 <style> 里的声明、
 * 以及模板 / 脚本里通过 :style 绑定的 `'--pt-xxx'` 字符串键（例如 --pt-kpi-cols）。
 * 带兜底值的引用 `var(--pt-xxx, 8px)` 不算缺失：写兜底就是承认它可能没有定义。
 */
import { readdirSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const SFC = import.meta.glob("/src/**/*.vue", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

const stylesDir = fileURLToPath(new URL(".", import.meta.url));
const STYLE_FILES = Object.fromEntries(
  readdirSync(stylesDir)
    .filter((f) => /\.(css|scss)$/.test(f))
    .map((f) => [f, readFileSync(`${stylesDir}/${f}`, "utf8")]),
);

const stripComments = (s: string) =>
  s.replace(/\/\*[\s\S]*?\*\//g, "").replace(/<!--[\s\S]*?-->/g, "");

function definitions(): Set<string> {
  const out = new Set<string>();
  const sources = [...Object.values(STYLE_FILES), ...Object.values(SFC)].map(stripComments);
  for (const src of sources) {
    for (const m of src.matchAll(/(--pt-[a-z0-9-]+)\s*:/g)) out.add(m[1]!);
    for (const m of src.matchAll(/["'](--pt-[a-z0-9-]+)["']/g)) out.add(m[1]!);
  }
  return out;
}

function references(src: string): string[] {
  const out: string[] = [];
  for (const m of stripComments(src).matchAll(/var\(\s*(--pt-[a-z0-9-]+)\s*(,)?/g)) {
    /* 以 - 结尾的是模板字符串拼出来的名字前缀（`var(--pt-kpi-${n})`），运行时才完整，这里不管 */
    if (!m[2] && !m[1]!.endsWith("-")) out.push(m[1]!);
  }
  return out;
}

describe("--pt-* 令牌的引用都有定义", () => {
  const defined = definitions();

  it("扫到了定义", () => {
    expect(defined.size).toBeGreaterThan(100);
  });

  const all: Array<[string, string]> = [
    ...Object.entries(STYLE_FILES).map(([f, s]) => [`styles/${f}`, s] as [string, string]),
    ...Object.entries(SFC).map(([f, s]) => [f.replace("/src/", ""), s] as [string, string]),
  ];
  for (const [file, src] of all) {
    const refs = references(src);
    if (refs.length === 0) continue;
    it(file, () => {
      const missing = [...new Set(refs.filter((r) => !defined.has(r)))];
      expect(missing, "引用了从没定义过的令牌（整条声明会作废）").toEqual([]);
    });
  }
});
