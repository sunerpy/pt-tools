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
  cleanup_emergency: true,
  peer_ratio_enabled: true,
  peer_ratio_max_sl: 5,
  peer_ratio_remove_data: false,
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
          {
            downloader: "nas",
            managed: 4,
            protected: 0,
            candidates: 4,
            items: [],
            deleted: 1,
            error: "删除（保留数据）失败: timeout",
          },
          {
            downloader: "box",
            managed: 3,
            protected: 0,
            candidates: 3,
            items: [],
            deleted: 1,
            paused: 2,
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
    expect(text, "一批成了一批失败：两样都写").toContain(
      "nas 删了 1 个，另有失败：删除（保留数据）失败: timeout",
    );
    expect(text, "做种竞争度暂停的也写").toContain("box 删了 1 个、暂停了 2 个");
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
            {
              name: "Crowded.Seed",
              info_hash: "h3",
              size: 1024 ** 3,
              seeding_h: 30,
              ratio: 0.4,
              upload_kbs: 0,
              reasons: ["做种竞争度过高：60 人做种、2 人下载（S/L 30.0），超过 5"],
              action: "pause",
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
      peer_ratio_enabled: true,
      peer_ratio_max_sl: 5,
      peer_ratio_remove_data: false,
      cleanup_emergency: true,
    });
    await vi.waitFor(() => expect(q("ac-pv-qb")).not.toBeNull());
    const box = q("ac-pv-qb")!.textContent ?? "";
    expect(box).toContain("会删 2 个，会暂停 1 个");
    expect(box).toContain("做种超过 72 小时；分享率达到 2");
    expect(box).toContain("只删种子");
    expect(box).toContain("低于下限");
    expect(box).toContain("只暂停：做种竞争度过高");
    expect(q("ac-preview-sum")!.textContent).toContain("共会删 2 个");
    expect(q("ac-preview-sum")!.textContent).toContain("会暂停 1 个");
    expect(q("ac-preview-sum")!.textContent, "预览也没有暂停").toContain("没有删也没有暂停");
    expect(document.querySelector(".el-dialog__title")?.textContent).toBe(
      "按现在的规则会删、会暂停这些",
    );
    expect(api.saveGlobal).not.toHaveBeenCalled();
  });
  it("预览写明下载器里共几个、刷流任务的几个，并列出受保护的种子与原因", async () => {
    api.status.mockResolvedValue({ enabled: true, interval_min: 30 });
    api.preview.mockResolvedValue({
      results: [
        {
          downloader: "qb",
          total: 9,
          managed: 8,
          protected: 2,
          candidates: 6,
          brush: 1,
          deleted: 0,
          items: [],
          protected_items: [
            {
              name: "Young.Movie",
              info_hash: "hy",
              size: 1024 ** 3,
              seeding_h: 2,
              ratio: 0,
              upload_kbs: 0,
              reasons: ["加进下载器不到 24 小时（最短保种时间），还差 20.0 小时"],
            },
            {
              name: "HR.Show",
              info_hash: "hh",
              size: 1024 ** 3,
              seeding_h: 10,
              ratio: 0.2,
              upload_kbs: 0,
              reasons: ["H&R：要求做种 72 小时，已做 10.0 小时"],
            },
          ],
        },
        // 旧版本的结果里连不上的下载器没有列表（null）：页面照样算得出来、不报错
        {
          downloader: "tr",
          managed: 0,
          protected: 0,
          candidates: 0,
          items: null,
          deleted: 0,
          error: "下载器连不上",
        },
      ],
    });
    await mountPage();
    await vi.waitFor(() => expect((q("ac-preview") as HTMLButtonElement).disabled).toBe(false));
    q("ac-preview")!.click();
    await vi.waitFor(() => expect(q("ac-pv-qb")).not.toBeNull());
    const box = q("ac-pv-qb")!.textContent ?? "";
    expect(box).toContain("下载器里共 9 个，管理范围里 8 个，受保护 2 个，会删 0 个");
    expect(box).toContain("1 个刷流任务的种子按刷流任务的规则删");
    const prot = q("ac-pv-protected-qb")!;
    expect(prot.textContent).toContain("受保护、这一轮不删的 2 个");
    expect(prot.textContent).toContain("Young.Movie");
    expect(prot.textContent).toContain("还差 20.0 小时");
    expect(prot.textContent).toContain("H&R：要求做种 72 小时，已做 10.0 小时");
    expect(q("ac-pv-tr")!.textContent).toContain("下载器连不上");
    expect(q("ac-preview-sum")!.textContent).toContain("共会删 0 个");
    expect(q("ac-preview-sum")!.textContent).not.toContain("暂停");
    expect(document.querySelector(".el-dialog__title")?.textContent).toBe("按现在的规则会删这些");
  });

  it("「不活跃」写明按最后活动时间判，低效做种的说明写明最短保种时间也会挡住删除", async () => {
    api.status.mockResolvedValue({ enabled: true, interval_min: 30 });
    api.getGlobal.mockResolvedValue({
      ...SETTINGS,
      cleanup_slow_seed_time_h: 3,
      cleanup_slow_max_ratio: 0.5,
      cleanup_min_retain_h: 24,
    });
    const Page = (await import("./AutoCleanup.vue")).default;
    view = mountView(Page);
    await vi.waitFor(() =>
      expect(document.querySelector(".slow-note")?.textContent).toContain("3 小时"),
    );
    const slow = document.querySelector(".slow-note")!.textContent ?? "";
    expect(slow).toContain("做种超过 3 小时、分享率仍低于 0.5 的已完成种子会被删除");
    expect(slow).toContain("加进下载器不满 24 小时的受「最短保种时间」保护，不删");
    expect(document.body.textContent).toContain("做种中的种子连续这么久没有上传或下载就删");
  });
});
