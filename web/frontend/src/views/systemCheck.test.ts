// @vitest-environment happy-dom
/*
 * 钉子：系统体检页（媒体中心 M8，首页 NAS 概览 H1 改成卡片）。打开时显示上一次的结果（没体检过时写一句），
 * 「立即体检」现查一遍并提示结果；总览卡写计数与上次的时间，点计数块只看那个状态的项；
 * 「需要处理」失败在前、提醒在后，带「去处理」链接，这次变坏的加「新出现」；「全部检查项」按组列，
 * 「只看有问题的」只留提醒与失败；读不到、体检失败时写原因；手机上「立即体检」在总览卡里。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import type { HealthReport } from "@/api";
import { type MountedView, mountView } from "@/test-mount";

const check = vi.hoisted(() => ({ last: vi.fn(), run: vi.fn() }));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, systemCheckApi: check };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return { ...real, ElMessage: Object.assign(vi.fn(), ui) };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.useRealTimers();
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

const flush = async () => {
  for (let i = 0; i < 4; i++) await new Promise((r) => setTimeout(r, 0));
};
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);
const text = (id: string) => (q(id)?.textContent ?? "").replace(/\s+/g, " ").trim();
const ids = (prefix: string) =>
  [...document.querySelectorAll<HTMLElement>(`[data-testid^="${prefix}"]`)].map((el) =>
    el.dataset.testid!.slice(prefix.length),
  );

/** 手机断点与「减少动态效果」：测试里默认减少动态效果，数字直接到位 */
function viewport(mobile: boolean, reduced = true) {
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches:
          (mobile && query.includes("max-width: 768px")) ||
          (reduced && query.includes("prefers-reduced-motion: reduce")),
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      }) as unknown as MediaQueryList,
  );
}

const report: HealthReport = {
  at: "2026-11-04T09:30:00",
  duration_ms: 3200,
  items: [
    { key: "tmdb", group: "tmdb", title: "TMDB 接口", status: "ok", detail: "用时 320 毫秒" },
    {
      key: "site:hdsky",
      group: "sites",
      title: "站点「HDSky」",
      status: "warn",
      detail: "登录过期了",
    },
    {
      key: "downloader:2",
      group: "downloaders",
      title: "下载器「tr」",
      status: "fail",
      detail: "连不上：connection refused",
      hint: "检查下载器的地址、端口与账号",
    },
    {
      key: "library:7",
      group: "libraries",
      title: "媒体库「电影」",
      status: "fail",
      detail: "目录不存在：/media/movies",
    },
    {
      key: "backup",
      group: "backup",
      title: "备份",
      status: "warn",
      detail: "上次备份失败：permission denied",
    },
    { key: "secret", group: "secret", title: "加密密钥", status: "skip", detail: "用的是环境变量" },
  ],
  ok: 1,
  warn: 2,
  fail: 2,
  skip: 1,
};

const allOk: HealthReport = {
  ...report,
  items: [report.items[0]],
  ok: 1,
  warn: 0,
  fail: 0,
  skip: 0,
};

async function mount(mobile = false, reduced = true) {
  viewport(mobile, reduced);
  const Page = (await import("./SystemCheck.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/system/check", component: Page },
      { path: "/:any(.*)*", component: { template: "<div />" } },
    ],
  });
  await router.push("/system/check");
  view = mountView(Page, { router });
  await flush();
}

const headRun = () =>
  document.querySelector<HTMLButtonElement>('#pt-head-acts [data-testid="sc-run"]')!;

describe("系统体检", () => {
  it("显示上一次的结果：页头的一句话、总览卡的计数与上次时间、按组列的各项", async () => {
    check.last.mockResolvedValue({ report });
    await mount();
    expect(check.run).not.toHaveBeenCalled();
    expect(document.querySelector("#pt-head-sub")?.textContent).toContain(
      "体检 · 用时 3.2 秒 · 2 项失败、2 项提醒",
    );
    expect(text("sc-meta")).toContain("体检 · 用时 3.2 秒 · 共 6 项");
    expect(text("sc-ring-num")).toBe("4");
    expect(document.querySelectorAll(".sc-ring__seg")).toHaveLength(4);
    expect(
      ["ok", "warn", "fail", "skip"].map((s) => q(`sc-tile-${s}`)!.getAttribute("aria-label")),
    ).toEqual(["正常 1", "提醒 2", "失败 2", "跳过 1"]);
    /* 分组卡按 HEALTH_GROUPS 的顺序，不按报告顺序 */
    expect(ids("sc-group-")).toEqual([
      "tmdb",
      "sites",
      "downloaders",
      "libraries",
      "backup",
      "secret",
    ]);
    const dl = q("sc-item-downloader:2")!;
    expect(dl.classList.contains("st-fail")).toBe(true);
    expect(dl.textContent).toContain("连不上：connection refused");
    expect(q("sc-group-downloaders")!.classList.contains("is-dang")).toBe(true);
    expect(text("sc-group-sites")).toContain("1 项提醒");
  });

  it("需要处理：失败在前、提醒在后，同状态按报告顺序；「去处理」按组去对应的页面", async () => {
    check.last.mockResolvedValue({ report });
    await mount();
    expect(ids("sc-prob-").filter((k) => k !== "new" && k !== "go")).toEqual([
      "downloader:2",
      "library:7",
      "site:hdsky",
      "backup",
    ]);
    const href = (k: string) =>
      q(`sc-prob-${k}`)!.querySelector('[data-testid="sc-prob-go"]')?.getAttribute("href");
    expect(href("downloader:2")).toBe("/downloaders");
    expect(href("library:7")).toBe("/media/library");
    expect(href("site:hdsky")).toBe("/sites/hdsky");
    expect(href("backup")).toBe("/global");
    expect(q("sc-prob-downloader:2")!.textContent).toContain("检查下载器的地址、端口与账号");
    expect(q("sc-prob-downloader:2")!.textContent).toContain("下载器");
    /* 刚打开页面时没有可比的上一次，不标「新出现」 */
    expect(document.querySelectorAll('[data-testid="sc-prob-new"]')).toHaveLength(0);
  });

  it("都正常时「需要处理」写一行，没有问题卡", async () => {
    check.last.mockResolvedValue({ report: allOk });
    await mount();
    expect(text("sc-allok")).toBe("没有要处理的项");
    expect(q("sc-problems")).toBeNull();
    expect(document.querySelector("#pt-head-sub")?.textContent).toContain("都正常");
  });

  it("立即体检以后，比上一次变坏的问题加「新出现」，没变的不加", async () => {
    check.last.mockResolvedValue({ report });
    const worse: HealthReport = {
      ...report,
      items: report.items.map((it) =>
        it.key === "tmdb"
          ? { ...it, status: "fail" as const, detail: "连不上 api.themoviedb.org" }
          : it.key === "backup"
            ? { ...it, status: "fail" as const }
            : it,
      ),
      ok: 0,
      warn: 1,
      fail: 4,
    };
    check.run.mockResolvedValue({ report: worse });
    await mount();
    headRun().click();
    await flush();
    const isNew = (k: string) =>
      Boolean(q(`sc-prob-${k}`)?.querySelector('[data-testid="sc-prob-new"]'));
    expect(isNew("tmdb")).toBe(true);
    expect(isNew("backup")).toBe(true);
    expect(isNew("downloader:2")).toBe(false);
    expect(isNew("site:hdsky")).toBe(false);
    expect(q("sc-prob-tmdb")!.classList.contains("sc-new")).toBe(true);
    expect(ui.warning).toHaveBeenCalledWith("体检完了：4 项失败");
  });

  it("都正常时立即体检查出问题：「没有要处理的项」换成问题卡的网格，不留在旧的那一行里", async () => {
    check.last.mockResolvedValue({ report: allOk });
    check.run.mockResolvedValue({ report });
    await mount();
    expect(text("sc-allok")).toBe("没有要处理的项");
    headRun().click();
    await flush();
    expect(q("sc-allok")).toBeNull();
    const grid = q("sc-problems")!;
    expect(grid.classList.contains("sc-probs")).toBe(true);
    expect(
      [...grid.querySelectorAll<HTMLElement>(".sc-prob")].map((el) => el.dataset.testid),
    ).toEqual([
      "sc-prob-downloader:2",
      "sc-prob-library:7",
      "sc-prob-site:hdsky",
      "sc-prob-backup",
    ]);
    /* 再体检一次都正常了：问题卡的网格换回那一行 */
    check.run.mockResolvedValue({ report: allOk });
    headRun().click();
    await flush();
    expect(q("sc-problems")).toBeNull();
    expect(q("sc-allok")?.classList.contains("sc-allok")).toBe(true);
    expect(text("sc-allok")).toBe("没有要处理的项");
  });

  it("点计数块只看那个状态的项，再点一次取消；「只看有问题的」只留提醒与失败", async () => {
    check.last.mockResolvedValue({ report });
    await mount();
    q("sc-tile-fail")!.click();
    await flush();
    expect(q("sc-tile-fail")!.getAttribute("aria-pressed")).toBe("true");
    expect(text("sc-status-chip")).toBe("只看失败");
    expect(ids("sc-item-")).toEqual(["downloader:2", "library:7"]);
    q("sc-tile-fail")!.click();
    await flush();
    expect(q("sc-status-chip")).toBeNull();
    expect(ids("sc-item-")).toHaveLength(6);

    q("sc-filter")!.click();
    await flush();
    expect(ids("sc-item-")).toEqual(["site:hdsky", "downloader:2", "library:7", "backup"]);

    /* 两个筛选同时开：「只看有问题的」+ 跳过，一项都留不下 */
    q("sc-tile-skip")!.click();
    await flush();
    expect(text("sc-none")).toBe("没有跳过的项");
  });

  it("没有有问题的项时写一句", async () => {
    check.last.mockResolvedValue({ report: allOk });
    await mount();
    q("sc-filter")!.click();
    await flush();
    expect(text("sc-none")).toBe("没有有问题的项");
  });

  it("点总览卡上一组的状态条：被筛选藏起来时先清掉筛选，再滚到那一组", async () => {
    check.last.mockResolvedValue({ report });
    await mount();
    const scrolled: string[] = [];
    vi.spyOn(Element.prototype, "scrollIntoView").mockImplementation(function (this: Element) {
      scrolled.push(this.id);
    });
    q("sc-filter")!.click();
    await flush();
    expect(q("sc-group-tmdb")).toBeNull();
    q("sc-chip-tmdb")!.click();
    await flush();
    expect(q("sc-group-tmdb")).not.toBeNull();
    expect(scrolled).toEqual(["sc-g-tmdb"]);
  });

  it("体检进行中：总览卡写已用几秒（每秒更新），旧结果留着，页面标进行中", async () => {
    check.last.mockResolvedValue({ report });
    let done!: (v: { report: HealthReport }) => void;
    check.run.mockReturnValue(new Promise((r) => (done = r)));
    await mount();
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval", "Date"] });
    headRun().click();
    await flush();
    expect(q("system-check")!.classList.contains("is-running")).toBe(true);
    expect(text("sc-stale")).toBe("上一次的结果");
    expect(text("sc-meta")).toBe("正在体检… 已用 0 秒");
    vi.advanceTimersByTime(2100);
    await flush();
    expect(text("sc-meta")).toBe("正在体检… 已用 2 秒");
    expect(document.querySelector("#pt-head-sub")?.textContent).toContain("正在体检… 已用 2 秒");
    expect(q("sc-prob-downloader:2")).not.toBeNull();
    done({ report: allOk });
    await flush();
    expect(q("system-check")!.classList.contains("is-running")).toBe(false);
    expect(q("sc-stale")).toBeNull();
    expect(text("sc-allok")).toBe("没有要处理的项");
    expect(ui.success).toHaveBeenCalledWith("体检完了：都正常");
  });

  it("没体检过时写一句；「立即体检」现查一遍，按结果提示", async () => {
    check.last.mockResolvedValue({ report: null });
    check.run.mockResolvedValue({ report });
    await mount();
    expect(text("sc-empty")).toContain("还没体检过");
    expect(document.querySelector("#pt-head-sub")?.textContent).toContain("一次检查 TMDB");
    expect(headRun().textContent).toContain("立即体检");
    q("sc-empty-run")!.click();
    await flush();
    expect(check.run).toHaveBeenCalledTimes(1);
    expect(ui.warning).toHaveBeenCalledWith("体检完了：2 项失败");
    expect(q("sc-item-downloader:2")).not.toBeNull();
    /* 第一次体检没有可比的上一次 */
    expect(document.querySelectorAll('[data-testid="sc-prob-new"]')).toHaveLength(0);
  });

  it("体检结果只有提醒或都正常时的提示", async () => {
    check.last.mockResolvedValue({ report: null });
    check.run
      .mockResolvedValueOnce({ report: { ...report, fail: 0 } })
      .mockResolvedValueOnce({ report: allOk });
    await mount();
    headRun().click();
    await flush();
    expect(ui.info).toHaveBeenCalledWith("体检完了：2 项提醒");
    headRun().click();
    await flush();
    expect(ui.success).toHaveBeenCalledWith("体检完了：都正常");
  });

  it("读不到上一次的结果时写原因，可以重新读；体检失败时提示原因", async () => {
    check.last.mockRejectedValueOnce(new Error("没有登录")).mockResolvedValueOnce({ report });
    check.run.mockRejectedValue(new Error("体检已经在跑"));
    await mount();
    expect(text("sc-load-error")).toContain("没有登录");
    headRun().click();
    await flush();
    expect(ui.error).toHaveBeenCalledWith("体检已经在跑");
    const retry = [...q("sc-load-error")!.querySelectorAll("button")].find((b) =>
      b.textContent?.includes("重新读取"),
    )!;
    retry.click();
    await flush();
    expect(check.last).toHaveBeenCalledTimes(2);
    expect(q("sc-hero")).not.toBeNull();
  });

  it("加载中显示骨架卡", async () => {
    check.last.mockReturnValue(new Promise(() => {}));
    await mount();
    expect(q("sc-loading")).not.toBeNull();
    expect(document.querySelector("#pt-head-sub")?.textContent).toContain("加载中…");
  });

  it("手机：「立即体检」在总览卡里，页头不放", async () => {
    check.last.mockResolvedValue({ report });
    check.run.mockResolvedValue({ report });
    await mount(true);
    expect(document.querySelector('#pt-head-acts [data-testid="sc-run"]')).toBeNull();
    const run = document.querySelector<HTMLButtonElement>('.sc-mbar [data-testid="sc-run"]')!;
    run.click();
    await flush();
    expect(check.run).toHaveBeenCalledTimes(1);
  });

  it("不减少动态效果时，数字从 0 过渡到终值", async () => {
    check.last.mockResolvedValue({ report });
    await mount(false, false);
    const num = () => Number(text("sc-ring-num"));
    expect(num()).toBeLessThan(4);
    await new Promise((r) => setTimeout(r, 600));
    expect(num()).toBe(4);
  });
});
