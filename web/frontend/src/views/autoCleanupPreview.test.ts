// @vitest-environment happy-dom
/*
 * 钉子：自动删种页的「预览会删哪些」与「上次运行」。预览按页面上的规则（还没保存的也算）发给后端、只算不删，
 * 弹窗里写明每台下载器会删哪些与为什么；上次运行写明每台删了几个、出了什么错，以及下一轮大约什么时候。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  getGlobal: vi.fn(),
  saveGlobal: vi.fn(),
  status: vi.fn(),
  preview: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    globalApi: { get: api.getGlobal, save: api.saveGlobal },
    cleanupApi: { status: api.status, preview: api.preview },
    maintenanceApi: { preview: vi.fn().mockResolvedValue({ categories: [] }), clean: vi.fn() },
  };
});

const SETTINGS = {
  cleanup_enabled: true,
  cleanup_interval_min: 30,
  cleanup_scope: "all",
  cleanup_scope_tags: "",
  cleanup_protect_tags: "keep,hr",
  cleanup_condition_mode: "or",
  cleanup_max_seed_time_h: 72,
  cleanup_min_ratio: 2,
  cleanup_max_inactive_h: 0,
};

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);

async function mountPage() {
  api.getGlobal.mockResolvedValue({ ...SETTINGS });
  const Page = (await import("./AutoCleanup.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.getGlobal).toHaveBeenCalled());
}

describe("自动删种：预览与上次运行", () => {
  it("上次运行：每台删了几个、出错的写原因，下一轮的时间", async () => {
    api.status.mockResolvedValue({
      enabled: true,
      interval_min: 30,
      last_run: {
        at: "2026-10-15T10:00:00Z",
        results: [
          { downloader: "qb", managed: 5, protected: 1, candidates: 4, items: [], deleted: 2 },
          {
            downloader: "tr",
            managed: 0,
            protected: 0,
            candidates: 0,
            items: [],
            deleted: 0,
            error: "下载器连不上，这一轮没检查",
          },
        ],
      },
      next_run_at: "2026-10-15T10:30:00Z",
    });
    await mountPage();
    await vi.waitFor(() => expect(q("ac-lastrun")).not.toBeNull());
    const text = q("ac-lastrun")!.textContent ?? "";
    expect(text).toContain("qb 删了 2 个");
    expect(text).toContain("tr：下载器连不上");
    expect(text).toContain("下次大约");
  });

  it("预览按页面上的规则发、只算不删，弹窗写明会删哪些与为什么", async () => {
    api.status.mockResolvedValue({ enabled: true, interval_min: 30 });
    api.preview.mockResolvedValue({
      results: [
        {
          downloader: "qb",
          managed: 3,
          protected: 1,
          candidates: 2,
          emergency: true,
          free_gb: 8.2,
          deleted: 0,
          items: [
            {
              name: "Old.Movie.2160p",
              info_hash: "h1",
              size: 40 * 1024 ** 3,
              seeding_h: 100,
              ratio: 2.5,
              upload_kbs: 0,
              reasons: ["做种超过 72 小时", "分享率达到 2"],
            },
            {
              name: "Shared.Seed",
              info_hash: "h2",
              size: 1024 ** 3,
              seeding_h: 80,
              ratio: 1,
              upload_kbs: 0,
              reasons: ["做种超过 72 小时"],
              keep_data: true,
            },
          ],
        },
      ],
    });
    await mountPage();
    await vi.waitFor(() => expect((q("ac-preview") as HTMLButtonElement).disabled).toBe(false));
    q("ac-preview")!.click();
    await vi.waitFor(() => expect(api.preview).toHaveBeenCalledTimes(1));
    expect(api.preview.mock.calls[0]![0]).toMatchObject({
      cleanup_scope: "all",
      cleanup_max_seed_time_h: 72,
      cleanup_min_ratio: 2,
      cleanup_protect_tags: "keep,hr",
    });
    await vi.waitFor(() => expect(q("ac-pv-qb")).not.toBeNull());
    const box = q("ac-pv-qb")!.textContent ?? "";
    expect(box).toContain("会删 2 个");
    expect(box).toContain("做种超过 72 小时；分享率达到 2");
    expect(box).toContain("只删种子");
    expect(box).toContain("低于下限");
    expect(q("ac-preview-sum")!.textContent).toContain("共会删 2 个");
    expect(api.saveGlobal).not.toHaveBeenCalled();
  });
});
