/// <reference types="node" />
/*
 * 钉子：表格的行 / 表头 / 选中行 / 悬停行背景一律不透明。
 *
 * 固定列（fixed="left" / "right"）是 position: sticky 的单元格，背景 inherit 自行（Element 的规则）。
 * 行背景一旦半透明，横滚到固定列底下的内容就会透出来 —— 用户统计的站点表选中行用的是 p-soft
 * （主色 12% 混 transparent），操作列底下透出了「-」「1年前」（用户原话「右侧操作列背景透明，
 * 底层文字内容会透出来」）。这里把 Element 表格的几个背景变量逐一拿出来查。
 */
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const read = (f: string) => readFileSync(fileURLToPath(new URL(f, import.meta.url)), "utf8");
const sources = { "atoms.css": read("./atoms.css"), "theme.scss": read("./theme.scss") };

const VARS =
  /--el-table-(?:bg|tr-bg|header-bg|row-hover-bg|current-row-bg|expanded-cell-bg)-color:\s*([^;]+);/g;

/** 半透明的写法：混 transparent、rgba / hsla 带 alpha、或者本项目的 *-soft 令牌（它们都是混 transparent 的） */
function translucent(value: string): boolean {
  return (
    /transparent/.test(value) ||
    /rgba?\([^)]*,\s*0?\.\d+\s*\)|\/\s*0?\.\d+/.test(value) ||
    /var\(--pt-[a-z]+-soft(-strong)?\)/.test(value)
  );
}

describe("表格背景变量都不透明", () => {
  for (const [file, src] of Object.entries(sources)) {
    const decls = [...src.replace(/\/\*[\s\S]*?\*\//g, "").matchAll(VARS)].map((m) => m[0]);

    it(`${file} 里声明了表格背景变量`, () => {
      expect(decls.length).toBeGreaterThan(0);
    });

    it(`${file} 的表格背景变量没有半透明的`, () => {
      expect(decls.filter((d) => translucent(d.split(":").slice(1).join(":")))).toEqual([]);
    });
  }
});
