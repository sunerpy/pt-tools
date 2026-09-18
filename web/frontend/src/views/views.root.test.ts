/*
 * 钉子：路由页只能有一个根元素。
 *
 * App.vue 的 `<router-view>` 外面套着 `<transition name="fade" mode="out-in">`。
 * 路由组件有多个根节点时，Transition 认不出要过渡的那个元素，`out-in` 的 leave
 * 回调不会触发，于是**下一个页面永远不挂载** —— 用户看到的是主区一片空白，
 * 控制台一句报错都没有，路由和页头标题却已经换成新页面了，极难往这个方向想。
 *
 * 这个缺陷在本轮真实发生过：修改密码页按画板 44 补了「分隔线 + 三张说明卡」，
 * 顺手让它们和账号卡并列成了三个根节点，从该页跳站点详情就白屏。
 *
 * 判定用 `@vue/compiler-sfc` 解析模板 AST，而不是数尖括号：注释、文本空白、
 * `v-if/v-else` 链都要正确处理。`v-if` 链只算一个根（同一时刻只渲染一个）。
 */
import { parse } from "vue/compiler-sfc";
import { describe, expect, it } from "vitest";

const SOURCES = import.meta.glob("/src/views/**/*.vue", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

/** AST 节点类型：1 = 元素，2 = 文本，3 = 注释，5 = 插值 */
const NODE_ELEMENT = 1;
const NODE_TEXT = 2;
const NODE_INTERPOLATION = 5;

interface TemplateNode {
  type: number;
  tag?: string;
  content?: string | { content?: string };
  props?: { name: string }[];
}

/**
 * 模板的根节点数。`v-else` / `v-else-if` 不另计，它们和前面的 `v-if` 同属一条链。
 * 顶层的纯文本与插值也算根节点 —— 它们同样会让 Transition 失效。
 */
export function rootNodeCount(source: string): number {
  const ast = parse(source).descriptor.template?.ast;
  if (!ast) return 0;
  let count = 0;
  for (const raw of ast.children as TemplateNode[]) {
    if (raw.type === NODE_TEXT) {
      const text = typeof raw.content === "string" ? raw.content : "";
      if (text.trim() !== "") count += 1;
      continue;
    }
    if (raw.type === NODE_INTERPOLATION) {
      count += 1;
      continue;
    }
    if (raw.type !== NODE_ELEMENT) continue; // 注释不算
    const directives = (raw.props ?? []).map((p) => p.name);
    if (directives.includes("else") || directives.includes("else-if")) continue;
    count += 1;
  }
  return count;
}

describe("路由页只有一个根元素", () => {
  const entries = Object.entries(SOURCES).sort(([a], [b]) => a.localeCompare(b));

  it("扫到了页面文件（防止 glob 写错导致空跑）", () => {
    expect(entries.length).toBeGreaterThan(15);
  });

  it.each(entries)("%s", (_path, source) => {
    expect(rootNodeCount(source)).toBe(1);
  });
});

describe("rootNodeCount 本身", () => {
  it("注释不算根节点", () => {
    expect(rootNodeCount(`<template>\n  <!-- 说明 -->\n  <div />\n</template>\n`)).toBe(1);
  });

  it("v-if / v-else 链算一个根节点", () => {
    expect(rootNodeCount(`<template>\n  <p v-if="a" />\n  <p v-else />\n</template>\n`)).toBe(1);
  });

  it("并列的两个元素算两个根节点", () => {
    expect(rootNodeCount(`<template>\n  <div />\n  <span />\n</template>\n`)).toBe(2);
  });
});
