/**
 * 模板里写死的图标名必须真的在 LUCIDE_ICONS 里。
 *
 * PtIcon 的 name 类型是 `LucideIconName | string`，写错名字类型检查拦不住，生产里就是一个空白 svg
 * （只有开发模式会在控制台告警）。发布前的一次审阅在五处找到了这种空图标：
 * files / flask-conical / copy-check / shield-x 都不在生成清单里。
 * lucide.ts 是设计工具包 icons.js 的生成物，不能手加 —— 要么换成清单里有的，要么回工具包里加。
 *
 * 只查静态写死的名字（`icon="x"`、`name="x"`、`action-icon="x"`）；`:name="expr"` 这类动态绑定
 * 在运行时才知道值，这里不管。
 */
import { describe, expect, it } from "vitest";

import { LUCIDE_ICONS } from "./lucide";

const SOURCES = import.meta.glob("/src/**/*.vue", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

/*
 * 会把字符串当成图标名的组件与属性。标签体用「非引号非 > 的字符 | 双引号串 | 单引号串」来走，
 * 否则 `v-if="rules.length > 0"` 里的 > 会把标签截断，后面的 icon 就漏扫了（第一版就漏了一处）。
 */
const TAG_BODY = `(?:[^>"']|"[^"]*"|'[^']*')*?`;
const attr = (tag: string, name: string) =>
  new RegExp(`<${tag}\\b${TAG_BODY}\\s${name}="([^"]+)"`, "g");
const ICON_ATTRS: Array<[RegExp, string]> = [
  [attr("PtIcon", "name"), "PtIcon name"],
  [attr("PtPanel", "icon"), "PtPanel icon"],
  [attr("PtPanel", "action-icon"), "PtPanel action-icon"],
  [attr("PtTag", "icon"), "PtTag icon"],
  [attr("PtDataState", "icon"), "PtDataState icon"],
];

function staticIconNames(source: string): Array<{ name: string; where: string }> {
  const match = /<template>([\s\S]*)\n<\/template>/.exec(source);
  if (!match) return [];
  const template = match[1].replace(/<!--[\s\S]*?-->/g, "");
  const out: Array<{ name: string; where: string }> = [];
  for (const [re, where] of ICON_ATTRS) {
    for (const m of template.matchAll(re)) out.push({ name: m[1]!, where });
  }
  return out;
}

describe("模板里写死的图标名都在生成清单里", () => {
  const known = new Set(Object.keys(LUCIDE_ICONS));
  const files = Object.entries(SOURCES);

  it("扫到了 SFC 与图标", () => {
    expect(files.length).toBeGreaterThan(30);
    const total = files.reduce((n, [, src]) => n + staticIconNames(src).length, 0);
    expect(total).toBeGreaterThan(100);
  });

  for (const [path, src] of files) {
    const names = staticIconNames(src);
    if (names.length === 0) continue;
    it(path.replace("/src/", ""), () => {
      const missing = names.filter((n) => !known.has(n.name)).map((n) => `${n.where}="${n.name}"`);
      expect(missing, "不在 icons/lucide.ts 里（生产里会是空白图标）").toEqual([]);
    });
  }
});
