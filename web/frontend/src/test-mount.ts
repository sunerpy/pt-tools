/**
 * 只在测试里用：把一个页面组件挂进 happy-dom，带上外壳给的那几个 Teleport 靶子。
 *
 * 页面把页头摘要与页头动作 Teleport 进外壳（#pt-head-sub / #pt-head-acts，手机上是
 * #pt-mhead-sub），靶子不在的话 Vue 只报一条警告、什么都不渲染 —— 「保存配置」这种
 * 恰好长在页头里的按钮就测不到了。所以每次挂载前把靶子铺好。
 *
 * 用法：测试文件顶上写 `// @vitest-environment happy-dom`，再按需 vi.mock("@/api")。
 */
import ElementPlus from "element-plus";
import { type Component, createApp } from "vue";
import type { Router } from "vue-router";

export interface MountedView {
  /** 挂载点；页头里的东西不在它下面，用 document 去查 */
  root: HTMLElement;
  unmount: () => void;
}

export function mountView(component: Component, opts: { router?: Router } = {}): MountedView {
  document.body.innerHTML =
    '<div id="pt-head-sub"></div><div id="pt-head-acts"></div>' +
    '<div id="pt-mhead-sub"></div><div id="app"></div>';
  const root = document.querySelector<HTMLElement>("#app")!;
  const app = createApp(component);
  if (opts.router) app.use(opts.router);
  app.use(ElementPlus);
  app.mount(root);
  return {
    root,
    unmount: () => {
      app.unmount();
      document.body.innerHTML = "";
    },
  };
}

/** 按按钮上的文字找 el-button（文字常常包在内层 span 里，按 textContent 比） */
export function buttonByText(text: string, scope: ParentNode = document): HTMLButtonElement {
  const hit = [...scope.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => (b.textContent ?? "").trim() === text,
  );
  if (!hit) throw new Error(`找不到文字为「${text}」的按钮`);
  return hit;
}
