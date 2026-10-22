// @vitest-environment happy-dom
/*
 * RSS 任务（原「调度器」）在外壳里的三处：页面顶上的提示条、状态栏浮层 / 手机抽屉里的面板、runtime store。
 *
 * 用户反馈：重启容器后 RSS 任务不会自己运行，而入口只是左下角一行小字。
 * 现在状态来自 GET /api/control/status；没开自动启动、RSS 任务没在跑时，每页顶上给一条提示，
 * 「启动 RSS 任务」直接启动（不再弹确认框），「打开自动启动」只改这一项。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import type { RssTaskStatus } from "@/api";
import { useRuntimeStore } from "@/stores/runtime";
import { buttonByText, type MountedView, mountView } from "@/test-mount";

const control = vi.hoisted(() => ({
  status: vi.fn(),
  start: vi.fn(),
  stop: vi.fn(),
  setAutoStart: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  confirm: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  const fail = () => Promise.reject(new Error("not in this test"));
  return {
    ...real,
    controlApi: control,
    sitesApi: { ...real.sitesApi, list: fail },
    tasksApi: { ...real.tasksApi, list: fail },
    pausedTorrentsApi: { ...real.pausedTorrentsApi, list: fail },
    downloaderTorrentsApi: { ...real.downloaderTorrentsApi, transferStats: fail },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessageBox: { confirm: ui.confirm },
    ElMessage: { success: ui.success, error: ui.error },
  };
});

function st(over: Partial<RssTaskStatus> = {}): RssTaskStatus {
  return { state: "manual", jobs: 0, subscriptions: 2, auto_start: false, ...over };
}

const flush = async () => {
  for (let i = 0; i < 4; i++) await Promise.resolve();
  await nextTick();
};

let view: MountedView | undefined;

async function mount(file: "RssTaskBanner" | "RssTaskPanel", status: RssTaskStatus | null) {
  const comp =
    file === "RssTaskBanner"
      ? (await import("./RssTaskBanner.vue")).default
      : (await import("./RssTaskPanel.vue")).default;
  useRuntimeStore().rss = status;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:p(.*)*", component: { template: "<div />" } }],
  });
  await router.push("/");
  view = mountView(comp, { router });
  await nextTick();
  return router;
}

const banner = () => document.querySelector('[data-testid="rss-task-banner"]');

beforeEach(() => {
  setActivePinia(createPinia());
  window.sessionStorage.clear();
  control.start.mockResolvedValue(undefined);
  control.stop.mockResolvedValue(undefined);
  ui.confirm.mockResolvedValue("confirm");
});

afterEach(() => {
  useRuntimeStore().stopPolling(); // 取消操作以后的跟查
  view?.unmount();
  view = undefined;
  vi.clearAllMocks();
  vi.useRealTimers();
});

describe("提示条", () => {
  it("没开自动启动、有订阅、RSS 任务没在跑：提示；「启动 RSS 任务」不弹确认框，启动后消失", async () => {
    await mount("RssTaskBanner", st());
    expect(banner()?.textContent).toContain("RSS 任务没有运行");
    expect(banner()?.textContent).toContain("重启");

    control.status.mockResolvedValue(st({ state: "running", jobs: 2 }));
    buttonByText("启动 RSS 任务").click();
    await flush();
    expect(ui.confirm).not.toHaveBeenCalled();
    expect(control.start).toHaveBeenCalledTimes(1);
    expect(control.status).toHaveBeenCalled();
    await nextTick();
    expect(banner()).toBeNull();
  });

  it("「打开自动启动」只改这一项；打开以后提示消失", async () => {
    await mount("RssTaskBanner", st());
    control.setAutoStart.mockResolvedValue(st({ state: "starting", auto_start: true }));
    control.status.mockResolvedValue(st({ state: "starting", auto_start: true }));
    buttonByText("打开自动启动").click();
    await flush();
    expect(control.setAutoStart).toHaveBeenCalledWith(true);
    expect(control.start).not.toHaveBeenCalled();
    expect(banner()).toBeNull();
  });

  it("没有默认下载器时引去设置下载器", async () => {
    const router = await mount("RssTaskBanner", st({ state: "no_downloader" }));
    buttonByText("设置下载器").click();
    await vi.waitFor(() => expect(router.currentRoute.value.path).toBe("/downloaders"));
    expect(control.start).not.toHaveBeenCalled();
  });

  it("关掉以后同一种状态不再提示，状态变了会再出现", async () => {
    await mount("RssTaskBanner", st());
    (
      document.querySelector('[data-testid="rss-task-banner-dismiss"]') as HTMLButtonElement
    ).click();
    await nextTick();
    expect(banner()).toBeNull();
    expect(window.sessionStorage.getItem("pt_tools_rss_banner_dismissed")).toBe("manual");

    useRuntimeStore().rss = st({ state: "downloader_unhealthy" });
    await nextTick();
    expect(banner()?.textContent).toContain("默认下载器连不上");
  });

  it("开着自动启动、手动停止、没有订阅：不提示", async () => {
    for (const s of [st({ auto_start: true }), st({ state: "paused" }), st({ subscriptions: 0 })]) {
      view?.unmount();
      setActivePinia(createPinia());
      await mount("RssTaskBanner", s);
      expect(banner()).toBeNull();
    }
  });
});

describe("状态栏浮层 / 手机抽屉里的面板", () => {
  it("写明状态与原因；没有任务在跑时「停止」不可点", async () => {
    await mount("RssTaskPanel", st());
    expect(document.querySelector('[data-testid="rss-task-detail"]')?.textContent).toContain(
      "没有打开自动启动",
    );
    expect(document.body.textContent).toContain("刷流、辅种、自动删种、签到等功能不受它影响");
    expect(buttonByText("停止").disabled).toBe(true);
    expect(buttonByText("启动").disabled).toBe(false);
  });

  it("「启动」先确认再启动；拨开关只切换自动启动", async () => {
    await mount("RssTaskPanel", st());
    control.status.mockResolvedValue(st({ state: "running", jobs: 1 }));
    buttonByText("启动").click();
    await flush();
    expect(ui.confirm).toHaveBeenCalledTimes(1);
    expect(control.start).toHaveBeenCalledTimes(1);

    control.setAutoStart.mockResolvedValue(st({ state: "running", jobs: 1, auto_start: true }));
    (document.querySelector('[data-testid="rss-task-autostart"]') as HTMLElement).click();
    await flush();
    expect(control.setAutoStart).toHaveBeenCalledWith(true);
    expect(ui.success).toHaveBeenLastCalledWith(
      "已打开自动启动：以后程序启动时会自动运行 RSS 任务",
    );
  });

  it("取消确认框时什么也不做", async () => {
    await mount("RssTaskPanel", st({ state: "running", jobs: 1 }));
    ui.confirm.mockRejectedValue("cancel");
    buttonByText("停止").click();
    await flush();
    expect(control.stop).not.toHaveBeenCalled();
  });
});

describe("runtime store", () => {
  it("定时刷新带上 RSS 任务的状态；查不到时保留上一次的结果", async () => {
    const store = useRuntimeStore();
    control.status.mockResolvedValueOnce(st({ state: "running", jobs: 3 }));
    await store.refresh();
    expect(store.rss?.state).toBe("running");
    expect(store.rssView.label).toBe("RSS 任务运行中");

    control.status.mockRejectedValueOnce(new Error("boom"));
    await store.refresh();
    expect(store.rss?.state).toBe("running");
  });

  it("操作以后跟查：「启动中」时每 2 秒再查一次，到终态就停", async () => {
    vi.useFakeTimers();
    const store = useRuntimeStore();
    control.setAutoStart.mockResolvedValue(st({ state: "starting", auto_start: true }));
    control.status
      .mockResolvedValueOnce(st({ state: "starting", auto_start: true }))
      .mockResolvedValueOnce(st({ state: "starting", auto_start: true }))
      .mockResolvedValue(st({ state: "running", jobs: 1, auto_start: true }));

    await store.setAutoStart(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(control.status).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(control.status).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(2_000);
    expect(control.status).toHaveBeenCalledTimes(3);
    expect(store.rss?.state).toBe("running");
    await vi.advanceTimersByTimeAsync(10_000);
    expect(control.status).toHaveBeenCalledTimes(3);
  });

  it("手动停止的状态下打开自动启动：提示现在仍是停止的", async () => {
    const store = useRuntimeStore();
    control.setAutoStart.mockResolvedValue(st({ state: "paused", auto_start: true }));
    control.status.mockResolvedValue(st({ state: "paused", auto_start: true }));
    await store.setAutoStart(true);
    expect(ui.success).toHaveBeenCalledWith(
      "已打开自动启动。RSS 任务现在是手动停止的，要运行请点「启动」",
    );
  });
});
