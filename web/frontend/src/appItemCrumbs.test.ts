// @vitest-environment happy-dom
/*
 * 钉子：条目详情（媒体中心 M3）的页头是详情页的 88 高带面包屑：「媒体 / 从哪里进来的那一页 / 条目名」（画板 04）。
 * 中间那段按路由的 ?from= 写、点了回那一页；导航高亮也跟着它；标题是页面读到的条目名，读到以前是路由的标题。
 */
import ElementPlus from "element-plus";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type App as VueApp, createApp, defineComponent, h } from "vue";
import { createMemoryHistory, createRouter, type Router } from "vue-router";

import { detailTitle } from "./composables/useDetailTitle";

const shell = vi.hoisted(() => ({ activePath: "" }));

vi.mock("./components/shell/AppNav.vue", async () => {
  const { defineComponent, h } = await import("vue");
  return {
    default: defineComponent({
      props: { activePath: { type: String, default: "" } },
      setup(props, { expose }) {
        expose({ focusSearch: () => {}, hidePrefs: () => false, focusToggle: () => {} });
        return () => {
          shell.activePath = props.activePath;
          return h("nav", { class: "nav-stub" });
        };
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
vi.mock("./components/shell/RssTaskBanner.vue", () => ({ default: { render: () => null } }));
vi.mock("./stores/logLevel", () => ({ useLogLevelStore: () => ({ fetchLogLevel: vi.fn() }) }));
vi.mock("./stores/version", () => ({
  useVersionStore: () => ({ fetchVersionInfo: vi.fn(), checkForUpdates: vi.fn() }),
}));
vi.mock("./stores/runtime", () => ({
  useRuntimeStore: () => ({ startPolling: vi.fn(), stopPolling: vi.fn() }),
}));

let app: VueApp | null = null;
let router: Router;

const Item = defineComponent({ render: () => h("div", "条目") });

async function mountShellAt(path: string) {
  const App = (await import("./App.vue")).default;
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      {
        path: "/media/item/:type/:id",
        name: "media-item",
        component: Item,
        meta: { title: "条目详情" },
      },
      {
        path: "/media/subscriptions",
        name: "media-subscriptions",
        component: Item,
        meta: { title: "追剧与订阅" },
      },
      { path: "/:p(.*)*", component: { render: () => null } },
    ],
  });
  await router.push(path);
  await router.isReady();
  document.body.innerHTML = '<div id="app"></div>';
  app = createApp(App).use(router).use(ElementPlus);
  app.mount("#app");
}

afterEach(() => {
  app?.unmount();
  app = null;
  document.body.innerHTML = "";
  detailTitle.value = "";
});

const crumbs = () => document.querySelector(".pt-head__crumbs");
const title = () => document.querySelector(".pt-head__title")?.textContent;

describe("条目详情的页头", () => {
  it("面包屑写从哪里进来的那一页，导航跟着高亮；标题换成条目名", async () => {
    await mountShellAt("/media/item/tv/100088?from=subscriptions&season=2");
    await vi.waitFor(() => expect(crumbs()).not.toBeNull());
    expect(document.querySelector(".pt-head")!.classList.contains("pt-head--crumbs")).toBe(true);
    expect(title(), "读到条目以前是路由的标题").toBe("条目详情");
    expect(crumbs()!.textContent).toContain("媒体");
    const link = crumbs()!.querySelector("a")!;
    expect(link.textContent).toBe("追剧与订阅");
    expect(link.getAttribute("href")).toBe("/media/subscriptions");
    expect(shell.activePath).toBe("/media/subscriptions");

    detailTitle.value = "长夜将尽";
    await vi.waitFor(() => expect(title()).toBe("长夜将尽"));
    expect(crumbs()!.textContent).toContain("长夜将尽");
  });

  it("没有 from 时是探索", async () => {
    await mountShellAt("/media/item/movie/693134");
    await vi.waitFor(() => expect(crumbs()).not.toBeNull());
    expect(crumbs()!.querySelector("a")!.textContent).toBe("探索");
    expect(shell.activePath).toBe("/media/explore");
  });
});
