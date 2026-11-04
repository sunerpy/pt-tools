// @vitest-environment happy-dom
/*
 * 钉子：系统体检页（媒体中心 M8）。打开时显示上一次的结果（没体检过时写一句），「立即体检」现查一遍并提示结果；
 * 各项按组列、写现在的样子与怎么修；「有问题的」只留提醒与失败；读不到、体检失败时写原因；手机上「立即体检」在面板里。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

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
  vi.clearAllMocks();
  vi.restoreAllMocks();
});

const flush = async () => {
  for (let i = 0; i < 4; i++) await new Promise((r) => setTimeout(r, 0));
};
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);
const text = (id: string) => (q(id)?.textContent ?? "").replace(/\s+/g, " ").trim();

/** 点 el-radio-button（它是 label 包着 input，不是 button） */
function radio(label: string) {
  const hit = [...document.querySelectorAll<HTMLElement>(".el-radio-button")].find(
    (l) => (l.textContent ?? "").replace(/\s+/g, " ").trim() === label,
  );
  if (!hit) throw new Error(`找不到「${label}」`);
  hit.querySelector("input")!.click();
}

function viewport(mobile: boolean) {
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches: mobile && query.includes("max-width: 768px"),
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
      key: "downloader:2",
      group: "downloaders",
      title: "下载器「tr」",
      status: "fail",
      detail: "连不上：connection refused",
      hint: "检查下载器的地址、端口与账号",
    },
    {
      key: "site:hdsky",
      group: "sites",
      title: "站点「HDSky」",
      status: "warn",
      detail: "登录过期了",
    },
    { key: "backup", group: "backup", title: "备份", status: "skip", detail: "每天定时备份关着" },
  ],
  ok: 1,
  warn: 1,
  fail: 1,
  skip: 1,
};

async function mount(mobile = false) {
  viewport(mobile);
  const Page = (await import("./SystemCheck.vue")).default;
  view = mountView(Page);
  await flush();
}

describe("系统体检", () => {
  it("显示上一次的结果：页头的一句话、各状态的计数、按组列的各项", async () => {
    check.last.mockResolvedValue({ report });
    await mount();
    expect(check.run).not.toHaveBeenCalled();
    expect(document.querySelector("#pt-head-sub")?.textContent).toContain(
      "体检 · 用时 3.2 秒 · 1 项失败、1 项提醒",
    );
    expect(
      [...q("sc-summary")!.querySelectorAll(".sc-sum")].map((c) => c.textContent?.trim()),
    ).toEqual(["正常 1", "提醒 1", "失败 1", "跳过 1"]);
    const groups = [...document.querySelectorAll<HTMLElement>('[data-testid^="sc-group-"]')].map(
      (g) => g.dataset.testid?.replace("sc-group-", ""),
    );
    expect(groups).toEqual(["tmdb", "sites", "downloaders", "backup"]);
    const dl = q("sc-item-downloader:2")!;
    expect(dl.classList.contains("st-fail")).toBe(true);
    expect(dl.textContent).toContain("连不上：connection refused");
    expect(dl.textContent).toContain("检查下载器的地址、端口与账号");
    expect(
      q("sc-group-downloaders")!.querySelector(".sc-group__state")?.classList.contains("tone-dang"),
    ).toBe(true);
  });

  it("「有问题的」只留提醒与失败", async () => {
    check.last.mockResolvedValue({ report });
    await mount();
    expect(text("sc-filter")).toContain("有问题的 2");
    radio("有问题的 2");
    await flush();
    expect(q("sc-item-tmdb")).toBeNull();
    expect(q("sc-item-backup")).toBeNull();
    expect(q("sc-item-downloader:2")).not.toBeNull();
    expect(q("sc-item-site:hdsky")).not.toBeNull();
  });

  it("没有有问题的项时写一句", async () => {
    check.last.mockResolvedValue({
      report: { ...report, items: [report.items[0]], ok: 1, warn: 0, fail: 0, skip: 0 },
    });
    await mount();
    radio("有问题的 0");
    await flush();
    expect(text("sc-none")).toBe("没有有问题的项");
  });

  it("没体检过时写一句；「立即体检」现查一遍，按结果提示", async () => {
    check.last.mockResolvedValue({ report: null });
    check.run.mockResolvedValue({ report });
    await mount();
    expect(text("sc-empty")).toContain("没体检过");
    expect(document.querySelector("#pt-head-sub")?.textContent).toContain("一次检查 TMDB");
    const run = document.querySelector<HTMLButtonElement>('#pt-head-acts [data-testid="sc-run"]')!;
    expect(run.textContent).toContain("立即体检");
    run.click();
    await flush();
    expect(check.run).toHaveBeenCalledTimes(1);
    expect(ui.warning).toHaveBeenCalledWith("体检完了：1 项失败");
    expect(q("sc-item-downloader:2")).not.toBeNull();
  });

  it("体检结果只有提醒或都正常时的提示", async () => {
    check.last.mockResolvedValue({ report: null });
    check.run
      .mockResolvedValueOnce({ report: { ...report, fail: 0 } })
      .mockResolvedValueOnce({ report: { ...report, fail: 0, warn: 0 } });
    await mount();
    const run = () =>
      document.querySelector<HTMLButtonElement>('#pt-head-acts [data-testid="sc-run"]')!.click();
    run();
    await flush();
    expect(ui.info).toHaveBeenCalledWith("体检完了：1 项提醒");
    run();
    await flush();
    expect(ui.success).toHaveBeenCalledWith("体检完了：都正常");
  });

  it("读不到上一次的结果、体检失败时写原因", async () => {
    check.last.mockRejectedValue(new Error("没有登录"));
    check.run.mockRejectedValue(new Error("体检已经在跑"));
    await mount();
    expect(text("sc-load-error")).toContain("没有登录");
    document.querySelector<HTMLButtonElement>('#pt-head-acts [data-testid="sc-run"]')!.click();
    await flush();
    expect(ui.error).toHaveBeenCalledWith("体检已经在跑");
  });

  it("手机：「立即体检」在面板里，页头不放", async () => {
    check.last.mockResolvedValue({ report });
    check.run.mockResolvedValue({ report });
    await mount(true);
    expect(document.querySelector('#pt-head-acts [data-testid="sc-run"]')).toBeNull();
    const run = document.querySelector<HTMLButtonElement>('.sc-mbar [data-testid="sc-run"]')!;
    run.click();
    await flush();
    expect(check.run).toHaveBeenCalledTimes(1);
  });
});
