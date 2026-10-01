// @vitest-environment happy-dom
/*
 * 钉子：种子搜索页的 ⌘K / Ctrl+K 聚焦查询框，不能被外壳的「跳转导航」抢走。
 *
 * 外壳（App.vue）和搜索页原来都在 window 的**冒泡**阶段听这个键：谁后执行谁赢，而外壳那次
 * 在 nextTick 之后才把焦点送进导航跳转框 —— 不管谁先注册，最后都是导航框拿到焦点，
 * 画板 15 查询框里那枚 ⌘K 键帽成了一个按了没反应的快捷键。
 *
 * 修法：搜索页在**捕获**阶段注册并 preventDefault（window 的捕获监听先于任何冒泡监听），
 * 外壳见到 defaultPrevented 就让路。这里挂的是真实的 App.vue 与真实的 TorrentSearch.vue，
 * 只把外壳的几个子组件与 store 换成桩 —— 导航框的 focusSearch 换成探针，看它有没有被调用。
 * 两种注册顺序都要过：冷启动直接打开 /search（页面先挂载），以及从别的页面切过来（外壳先注册）。
 */
import ElementPlus from "element-plus";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type App as VueApp, createApp } from "vue";
import { createMemoryHistory, createRouter, type Router } from "vue-router";

const shell = vi.hoisted(() => ({ navFocus: vi.fn() }));

vi.mock("./components/shell/AppNav.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    default: defineComponent({
      setup(_, { expose }) {
        expose({ focusSearch: shell.navFocus, hidePrefs: () => false, focusToggle: () => {} });
        return () => h("nav", { class: "nav-stub" });
      },
    }),
  };
});
vi.mock("./components/shell/AppRail.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    default: defineComponent({
      setup(_, { expose }) {
        expose({ hidePrefs: () => false, focusToggle: () => {} });
        return () => h("aside");
      },
    }),
  };
});
vi.mock("./components/shell/AppStatusBar.vue", () => ({ default: { render: () => null } }));
vi.mock("./components/shell/MobileChrome.vue", () => ({ default: { render: () => null } }));
vi.mock("./components/V2DeprecationBanner.vue", () => ({ default: { render: () => null } }));
vi.mock("./stores/logLevel", () => ({ useLogLevelStore: () => ({ fetchLogLevel: vi.fn() }) }));
vi.mock("./stores/version", () => ({
  useVersionStore: () => ({ fetchVersionInfo: vi.fn(), checkForUpdates: vi.fn() }),
}));
vi.mock("./stores/runtime", () => ({
  useRuntimeStore: () => ({ startPolling: vi.fn(), stopPolling: vi.fn() }),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    searchApi: {
      getSites: vi.fn(() => Promise.resolve([])),
      multiSite: vi.fn(),
      clearCache: vi.fn(),
    },
    downloadersApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderDirectoriesApi: { listAll: vi.fn(() => Promise.resolve({})) },
    siteCategoriesApi: { getAll: vi.fn(() => Promise.resolve({})) },
    torrentPushApi: { push: vi.fn(), batchPush: vi.fn() },
  };
});

let app: VueApp | null = null;
let router: Router;

async function mountShellAt(path: string) {
  const App = (await import("./App.vue")).default;
  const TorrentSearch = (await import("./views/TorrentSearch.vue")).default;
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/search", name: "search", component: TorrentSearch },
      { path: "/tasks", name: "tasks", component: { render: () => null } },
    ],
  });
  await router.push(path);
  await router.isReady();
  document.body.innerHTML = '<div id="app"></div>';
  app = createApp(App).use(router).use(ElementPlus);
  app.mount("#app");
}

const queryInput = () =>
  document.querySelector<HTMLInputElement>("input[placeholder='输入关键词，回车即搜']");

/** 焦点在页面正文上时按 Ctrl+K；返回这个事件，看它有没有被 preventDefault */
function pressJump(): KeyboardEvent {
  const e = new KeyboardEvent("keydown", {
    key: "k",
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
  });
  document.body.dispatchEvent(e);
  return e;
}

afterEach(() => {
  app?.unmount();
  app = null;
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

describe("⌘K：页面接管时外壳让路", () => {
  it("冷启动直接打开搜索页：Ctrl+K 聚焦查询框，不去导航跳转框", async () => {
    await mountShellAt("/search");
    await vi.waitFor(() => expect(queryInput()).not.toBeNull());

    const e = pressJump();
    expect(e.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(queryInput());
    expect(shell.navFocus).not.toHaveBeenCalled();
  });

  it("从别的页面切到搜索页（外壳的监听先注册）：照样是查询框拿到焦点", async () => {
    await mountShellAt("/tasks");
    await router.push("/search");
    await vi.waitFor(() => expect(queryInput()).not.toBeNull());

    pressJump();
    expect(document.activeElement).toBe(queryInput());
    expect(shell.navFocus).not.toHaveBeenCalled();
  });

  it("没有自己 ⌘K 的页面：外壳照旧把焦点送进导航跳转框", async () => {
    await mountShellAt("/tasks");
    const e = pressJump();
    expect(e.defaultPrevented).toBe(true);
    expect(shell.navFocus).toHaveBeenCalledTimes(1);
  });

  /*
   * 浏览器里 removeEventListener 必须带上与注册时相同的 capture 才摘得掉，摘不掉的话离开搜索页
   * 之后每次 ⌘K 都会被这个已经卸载的页面 preventDefault，外壳永远让路。happy-dom 的
   * removeEventListener 不看 capture（先删冒泡、再删捕获），行为上测不出来，所以直接核对参数。
   */
  it("离开搜索页之后，页面的监听带着同样的 capture 摘掉，⌘K 回到外壳", async () => {
    const add = vi.spyOn(window, "addEventListener");
    const remove = vi.spyOn(window, "removeEventListener");
    const captureOf = (opts: unknown) =>
      typeof opts === "boolean"
        ? opts
        : Boolean((opts as AddEventListenerOptions | undefined)?.capture);

    await mountShellAt("/search");
    await vi.waitFor(() => expect(queryInput()).not.toBeNull());
    const pageHandlers = add.mock.calls
      .filter(([type, , opts]) => type === "keydown" && captureOf(opts))
      .map(([, fn]) => fn);
    expect(pageHandlers).toHaveLength(1);

    await router.push("/tasks");
    await vi.waitFor(() => expect(queryInput()).toBeNull());
    const removed = remove.mock.calls.filter(
      ([type, fn]) => type === "keydown" && fn === pageHandlers[0],
    );
    expect(removed).toHaveLength(1);
    expect(captureOf(removed[0]![2])).toBe(true);

    pressJump();
    expect(shell.navFocus).toHaveBeenCalledTimes(1);
    add.mockRestore();
    remove.mockRestore();
  });
});
