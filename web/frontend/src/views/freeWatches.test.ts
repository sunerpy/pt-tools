// @vitest-environment happy-dom
/*
 * 钉子：等免费页（媒体中心 M2b）。列出每条在等的种子推到哪里、现在怎样、下次什么时候看；按状态筛选；
 * 取消先确认；改到期时间只能改到现在以后、30 天以内；手机是行卡；读不到时写明原因。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  cancel: vi.fn(),
  setExpiry: vi.fn(),
  sites: vi.fn(),
  create: vi.fn(),
}));
const dl = vi.hoisted(() => ({ list: vi.fn() }));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
  confirm: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, freeWatchApi: api, downloadersApi: dl };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: ui.error,
      warning: ui.warning,
      info: ui.info,
    }),
    ElMessageBox: { ...real.ElMessageBox, confirm: ui.confirm },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);

const inTwoDays = new Date(Date.now() + 2 * 86400_000).toISOString();
const watch = (over: Record<string, unknown>) => ({
  id: 1,
  site_name: "hdtime",
  torrent_id: "300101",
  title: "Fake.Movie.2026.2160p.WEB-DL-FAKE",
  subtitle: "",
  size_bytes: 15 * 1024 ** 3,
  has_hr: true,
  hr_seed_time_h: 72,
  downloader_id: 1,
  save_path: "/data/movies",
  category: "movie",
  tags: "",
  media_type: "",
  tmdb_id: 0,
  season: 0,
  status: "watching",
  next_check_at: "2026-11-03T12:30:00Z",
  check_count: 2,
  failures: 0,
  expires_at: inTwoDays,
  info_hash: "",
  message: "还没免费（现在是 50%）",
  created_at: "2026-11-03T08:00:00Z",
  updated_at: "2026-11-03T12:00:00Z",
  ...over,
});

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
      }) as MediaQueryList,
  );
}

async function mountPage(opts: { fail?: boolean; mobile?: boolean } = {}) {
  viewport(Boolean(opts.mobile));
  if (opts.fail) api.list.mockRejectedValue(new Error("读取等免费失败"));
  else
    api.list.mockResolvedValue({
      items: [
        watch({}),
        watch({
          id: 2,
          title: "Other.Show.S01",
          status: "pushed",
          has_hr: false,
          save_path: "",
          downloader_id: 9,
          message: "免费了，已推送",
          check_count: 5,
        }),
        watch({ id: 3, status: "failed", message: "读不到种子现在的优惠（连续 5 次，不再等）" }),
      ],
      counts: { watching: 1, pushed: 1, failed: 1 },
    });
  dl.list.mockResolvedValue([{ id: 1, name: "qb", type: "qbittorrent", enabled: true }]);
  const Page = (await import("./FreeWatches.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.list).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("等免费", () => {
  it("列表：推到哪里、现在怎样、下次什么时候看；只有在等的能改到期、取消", async () => {
    await mountPage();
    expect(api.list).toHaveBeenCalledWith(undefined);
    const text = q("fw-table")!.textContent ?? "";
    expect(text).toContain("Fake.Movie.2026.2160p.WEB-DL-FAKE");
    expect(text).toContain("hdtime · 15 GB · H&R 72 时");
    expect(text).toContain("qb");
    expect(text).toContain("/data/movies · movie");
    expect(text).toContain("默认路径");
    expect(text).toContain("下载器 9");
    expect(text).toContain("等免费中");
    expect(text).toContain("还没免费（现在是 50%）");
    expect(text).toContain("下次");
    expect(text).toContain("看过 2 次");
    expect(text).toContain("结束于");
    expect(text).toContain("不再等");
    expect(q("fw-cancel-1")).not.toBeNull();
    expect(q("fw-expiry-1")).not.toBeNull();
    expect(q("fw-cancel-2"), "推送了的不能取消").toBeNull();
    const labels = [...q("fw-status")!.querySelectorAll("label")].map((l) => l.textContent?.trim());
    expect(labels).toEqual(["全部 3", "等免费中 1", "已推送 1", "到期了", "不再等 1", "已取消"]);
  });

  it("筛选：按状态重新读", async () => {
    await mountPage();
    const pushed = [...q("fw-status")!.querySelectorAll("label")].find((l) =>
      l.textContent?.includes("已推送"),
    )!;
    pushed.querySelector("input")!.click();
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    expect(api.list.mock.calls[1]![0]).toBe("pushed");
  });

  it("取消：先确认，取消对话框就不发", async () => {
    await mountPage();
    ui.confirm.mockRejectedValueOnce("cancel");
    q("fw-cancel-1")!.click();
    await flush();
    expect(api.cancel).not.toHaveBeenCalled();
    ui.confirm.mockResolvedValue("confirm");
    api.cancel.mockResolvedValue({});
    q("fw-cancel-1")!.click();
    await vi.waitFor(() => expect(api.cancel).toHaveBeenCalledWith(1));
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("已取消"));
    expect(api.list).toHaveBeenCalledTimes(2);
  });

  it("改到期时间：默认是现在的到期时间，保存发 ISO 时间；出错时写明原因", async () => {
    await mountPage();
    q("fw-expiry-1")!.click();
    await vi.waitFor(() => expect(q("fw-expiry-save")).not.toBeNull());
    expect(document.body.textContent).toContain("最多从现在起再等 30 天");
    api.setExpiry.mockResolvedValue({});
    q("fw-expiry-save")!.click();
    await vi.waitFor(() => expect(api.setExpiry).toHaveBeenCalledWith(1, inTwoDays));
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    // 刷新完、按钮退出加载态以后才能再点
    await vi.waitFor(() =>
      expect(q("fw-expiry-save")!.classList.contains("is-loading")).toBe(false),
    );

    api.setExpiry.mockRejectedValue(new Error("参数不对: 到期时间要晚于现在"));
    q("fw-expiry-1")!.click();
    await vi.waitFor(() => expect(q("fw-expiry-save")).not.toBeNull());
    q("fw-expiry-save")!.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("参数不对: 到期时间要晚于现在"));
  });

  it("到期时间已经过了（后台还没来得及处理）：保存点不了，写明原因", async () => {
    viewport(false);
    api.list.mockResolvedValue({
      items: [watch({ expires_at: "2026-01-01T00:00:00Z" })],
      counts: { watching: 1 },
    });
    dl.list.mockResolvedValue([]);
    const Page = (await import("./FreeWatches.vue")).default;
    view = mountView(Page);
    await vi.waitFor(() => expect(q("fw-expiry-1")).not.toBeNull());
    q("fw-expiry-1")!.click();
    await vi.waitFor(() => expect(q("fw-expiry-save")).not.toBeNull());
    expect(q("fw-expiry-save")!.hasAttribute("disabled")).toBe(true);
    expect(document.body.textContent).toContain("到期时间要晚于现在");
  });

  it("手机是行卡：在等的带改到期、取消", async () => {
    await mountPage({ mobile: true });
    expect(q("fw-table")).toBeNull();
    const cards = [...document.querySelectorAll(".pt-rowcard")];
    expect(cards).toHaveLength(3);
    expect(cards[0]!.textContent).toContain("推到 qb");
    expect(cards[0]!.textContent).toContain("改到期");
    expect(cards[1]!.textContent).not.toContain("取消");
  });

  it("读不到时写明原因", async () => {
    await mountPage({ fail: true });
    expect(q("fw-state")!.textContent).toContain("读取等免费失败");
  });
});
