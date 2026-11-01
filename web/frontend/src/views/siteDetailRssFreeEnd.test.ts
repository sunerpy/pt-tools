// @vitest-environment happy-dom
/*
 * 钉子：RSS 的「免费结束时没下完」三选一（2026-11-01 用户决定每条 RSS 自己选）：不处理、暂停、删除，
 * 存成 pause_on_free_end 与 free_end_delete 两项；列表上的标记写「免费结束暂停」或「免费结束删除」。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  getSite: vi.fn(),
  saveSite: vi.fn(),
  testRssLink: vi.fn(),
}));

const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    sitesApi: {
      get: api.getSite,
      save: api.saveSite,
      testRssLink: api.testRssLink,
      listLoginStates: vi.fn(() => Promise.resolve([])),
      deleteRss: vi.fn(),
      delete: vi.fn(),
    },
    attendanceApi: { list: vi.fn(() => Promise.resolve([])), signNow: vi.fn() },
    downloadersApi: {
      list: vi.fn(() =>
        Promise.resolve([
          {
            id: 1,
            name: "qb",
            type: "qbittorrent",
            url: "",
            username: "",
            is_default: true,
            enabled: true,
          },
        ]),
      ),
    },
    filterRulesApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderDirectoriesApi: {
      listAll: vi.fn(() =>
        Promise.resolve({
          1: [
            { id: 3, downloader_id: 1, path: "/downloads", alias: "", is_default: true },
            { id: 4, downloader_id: 1, path: "/downloads/tv", alias: "剧集", is_default: false },
          ],
        }),
      ),
    },
    chatopsApi: { notifications: { list: vi.fn(() => Promise.resolve([])) } },
    userInfoApi: {
      getSite: vi.fn(() => Promise.resolve(null)),
      getHistory: vi.fn(() => Promise.resolve({ points: [] })),
    },
    tasksApi: { list: vi.fn(() => Promise.resolve({ items: [], total: 0 })) },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: ui.error,
      warning: ui.warning,
      info: vi.fn(),
    }),
  };
});

function site(rss: Record<string, unknown>[]) {
  return {
    enabled: true,
    auth_method: "cookie",
    cookie: "",
    api_key: "",
    api_url: "https://hdsky.example",
    urls: ["https://hdsky.example"],
    passkey: "",
    upload_limit_kbs: 0,
    download_limit_kbs: 0,
    seeding_capacity_gb: 0,
    rss,
  };
}

const deleting = {
  id: 7,
  name: "删除的",
  url: "https://hdsky.example/rss?a=1",
  interval_minutes: 10,
  pause_on_free_end: true,
  free_end_delete: true,
};
const pausing = {
  id: 8,
  name: "暂停的",
  url: "https://hdsky.example/rss?a=2",
  interval_minutes: 10,
  pause_on_free_end: true,
  free_end_delete: false,
};

let view: MountedView | null = null;

async function mountDetail(rss: Record<string, unknown>[]): Promise<Router> {
  api.getSite.mockImplementation(() => Promise.resolve(site(rss)));
  api.saveSite.mockResolvedValue(undefined);
  const SiteDetail = (await import("./SiteDetail.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/sites/:name", name: "site-detail", component: SiteDetail },
      { path: "/sites", component: { render: () => null } },
    ],
  });
  await router.push("/sites/hdsky");
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
  await vi.waitFor(() => expect(buttonByText("保存配置").disabled).toBe(false));
  return router;
}

function fill(selector: string, value: string) {
  const el = document.querySelector<HTMLElement>(selector);
  const input = (
    el?.tagName === "INPUT" ? el : el?.querySelector("input")
  ) as HTMLInputElement | null;
  if (!input) throw new Error(`找不到输入框 ${selector}`);
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
}

/** 看得见的对话框里「免费结束时没下完」的三个选项 */
function choices(): HTMLElement[] {
  const dlg = [...document.querySelectorAll<HTMLElement>(".el-dialog")].find(
    (d) =>
      getComputedStyle(d).display !== "none" && d.querySelector('[data-testid="rss-free-end"]'),
  );
  return [...(dlg?.querySelectorAll<HTMLElement>('[data-testid="rss-free-end"] label') ?? [])];
}

function choose(text: string) {
  const label = choices().find((l) => (l.textContent ?? "").trim() === text);
  if (!label) throw new Error(`没有选项 ${text}`);
  label.click();
}

function checked(): string {
  return (
    choices()
      .find((l) => l.classList.contains("is-active") || l.querySelector("input")?.checked)
      ?.textContent?.trim() ?? ""
  );
}

function savedRss(name: string) {
  const body = api.saveSite.mock.calls.at(-1)!.at(-1) as {
    rss: { name: string; pause_on_free_end?: boolean; free_end_delete?: boolean }[];
  };
  return body.rss.find((r) => r.name === name)!;
}

afterEach(() => {
  view?.unmount();
  view = null;
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

describe("站点详情：RSS 免费结束时没下完", () => {
  it("新增：默认暂停；选删除存成两项都开，选不处理两项都关", async () => {
    for (const [pick, pause, del] of [
      ["删除", true, true],
      ["不处理", false, false],
    ] as const) {
      await mountDetail([]);
      buttonByText("添加 RSS").click();
      await vi.waitFor(() => expect(choices()).toHaveLength(3));
      expect(checked()).toBe("暂停");
      fill('input[placeholder="如：CMCT 电视剧"]', "HDSky 剧集");
      fill('[data-testid="rss-new-url"]', "https://hdsky.example/torrentrss.php?passkey=k");
      choose(pick);
      buttonByText("添加").click();
      await vi.waitFor(() => expect(api.saveSite).toHaveBeenCalled());
      const r = savedRss("HDSky 剧集");
      expect(r.pause_on_free_end, pick).toBe(pause);
      expect(r.free_end_delete, pick).toBe(del);
      view?.unmount();
      view = null;
      document.body.innerHTML = "";
      vi.clearAllMocks();
    }
  });

  it("编辑：读回删除，改成暂停以后删除那一项关掉", async () => {
    await mountDetail([deleting]);
    buttonByText("编辑").click();
    await vi.waitFor(() => expect(choices()).toHaveLength(3));
    expect(checked()).toBe("删除");
    choose("暂停");
    buttonByText("保存").click();
    await vi.waitFor(() => expect(api.saveSite).toHaveBeenCalledTimes(1));
    const r = savedRss("删除的");
    expect(r.pause_on_free_end).toBe(true);
    expect(r.free_end_delete).toBe(false);
  });

  it("列表标记写免费结束删除或免费结束暂停", async () => {
    await mountDetail([deleting, pausing]);
    await vi.waitFor(() => expect(document.body.textContent).toContain("免费结束删除"));
    expect(document.body.textContent).toContain("免费结束暂停");
  });
});
