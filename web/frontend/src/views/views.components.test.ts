/*
 * 钉子：模板里用到的组件必须在脚本作用域里。
 *
 * 为什么需要这条测试：`vue-tsc -b`（本项目真正的类型闸门）**不会**报未导入的组件。
 * `<PtPanel>` 忘了 import，Volar 只当它是个未知元素，编译照过，运行时 Vue 把它渲染成
 * 字面的 `<ptpanel>` 自定义元素 —— 卡片的页头、页脚、边框全没了，只剩插槽内容裸在那里，
 * 页面看上去「少了一层」但控制台之外没有任何硬错误。本次重构里 PausedTorrents 和
 * TorrentSearch 各中过一次，两次都是 build 绿灯、渲染缺件。
 *
 * 做法是纯文本扫描而不是挂载每个页面：挂载要造 router、pinia 和一堆后端桩，
 * 而这个缺陷是静态可判定的 —— 模板里出现的大驼峰标签，要么是 Vue 内置，
 * 要么必须能在 `<script setup>` 里找到同名标识符。
 */
import { describe, expect, it } from "vitest";

/** Vue 内置的大驼峰标签，不需要导入 */
const BUILTIN = new Set([
  "Teleport",
  "Transition",
  "TransitionGroup",
  "Suspense",
  "KeepAlive",
  "Component",
  "Fragment",
  "RouterView",
  "RouterLink",
]);

/*
 * 用 Vite 的 glob 而不是 node:fs 取源码：tsconfig.app.json 的 types 只有 vite/client，
 * 没有 @types/node，`import { readFileSync } from "node:fs"` 过不了 `vue-tsc -b`。
 * `?raw` 拿到的是 SFC 的原始文本，正是这条扫描需要的东西。
 */
const SOURCES = import.meta.glob("/src/**/*.vue", {
  query: "?raw",
  import: "default",
  eager: true,
}) as Record<string, string>;

/** 模板里出现、却在 SFC 其余部分找不到同名标识符的大驼峰标签 */
function undefinedComponents(source: string): string[] {
  const match = /<template>([\s\S]*)\n<\/template>/.exec(source);
  if (!match) return [];
  const template = match[1].replace(/<!--[\s\S]*?-->/g, "");
  const script = source.slice(0, match.index) + source.slice(match.index + match[0].length);
  const tags = new Set(
    [...template.matchAll(/<([A-Z][A-Za-z0-9]*)/g)].map((m) => m[1]).filter((t) => !BUILTIN.has(t)),
  );
  return [...tags].filter((tag) => !new RegExp(`\\b${tag}\\b`).test(script)).sort();
}

describe("SFC 模板里的组件都已导入", () => {
  const entries = Object.entries(SOURCES).sort(([a], [b]) => a.localeCompare(b));

  it("扫到了页面文件（防止 glob 写错导致空跑）", () => {
    expect(entries.length).toBeGreaterThan(20);
  });

  it.each(entries)("%s", (_path, source) => {
    expect(undefinedComponents(source)).toEqual([]);
  });
});

describe("undefinedComponents 本身", () => {
  it("认出未导入的组件", () => {
    const sfc = `<script setup lang="ts">
import PtIcon from "@/components/PtIcon";
</script>

<template>
  <PtPanel><PtIcon name="x" /></PtPanel>
</template>
`;
    expect(undefinedComponents(sfc)).toEqual(["PtPanel"]);
  });

  it("不把注释里的标签和内置组件算进来", () => {
    const sfc = `<script setup lang="ts"></script>

<template>
  <!-- <PtPanel> 只是注释 -->
  <Teleport to="#x"><span /></Teleport>
</template>
`;
    expect(undefinedComponents(sfc)).toEqual([]);
  });
});
