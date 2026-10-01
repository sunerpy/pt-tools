// @vitest-environment happy-dom
/*
 * 钉子：暂停任务 / 归档记录的时间跨年要带年份。
 *
 * 表格里原来的 formatTimeShort 永远只印「月-日 时:分」：去年 12-31 暂停的和今年 12-31 暂停的
 * 两行一模一样，跨年的记录没法区分。改用 utils/format 的 formatShortDateTime ——
 * 当年省掉年份（画板 25 的「09-15 14:32」照旧），跨年才带上。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const local = (y: number, mo: number, d: number, h: number, mi: number) =>
  new Date(y, mo - 1, d, h, mi).toISOString();

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  const paused = (id: number, title: string, at: string) => ({
    id,
    site_name: "mteam",
    title,
    progress: 42,
    torrent_size: 1024 ** 3,
    downloader_name: "qb",
    downloader_task_id: `h${id}`,
    paused_at: at,
    pause_reason: "免费期结束",
    created_at: at,
  });
  const archived = (id: number, title: string, at: string) => ({
    id,
    original_id: id,
    site_name: "mteam",
    title,
    is_free: false,
    is_completed: false,
    progress: 10,
    is_paused_by_system: true,
    pause_reason: "免费期结束",
    downloader_name: "qb",
    original_created_at: at,
    archived_at: at,
  });
  return {
    ...real,
    pausedTorrentsApi: {
      list: vi.fn(() =>
        Promise.resolve({
          items: [
            paused(1, "去年暂停", local(2025, 12, 31, 23, 30)),
            paused(2, "今年暂停", local(2026, 9, 15, 14, 32)),
          ],
          total: 2,
          page: 1,
          page_size: 20,
        }),
      ),
      listArchive: vi.fn(() =>
        Promise.resolve({
          items: [archived(9, "去年归档", local(2025, 6, 1, 8, 5))],
          total: 1,
          page: 1,
          page_size: 20,
        }),
      ),
      delete: vi.fn(),
      resume: vi.fn(),
    },
    globalApi: { get: vi.fn(() => Promise.resolve({ auto_delete_on_free_end: false })) },
  };
});

let view: MountedView | null = null;

/** 某一行（按标题找）里那格时间的文字 */
function timeCell(title: string): string {
  const tr = [...document.querySelectorAll<HTMLElement>(".el-table__body tr")].find((r) =>
    (r.textContent ?? "").includes(title),
  );
  const cell = tr?.querySelector<HTMLElement>("td.pt-cell-1line span[title]");
  return cell?.textContent?.trim() ?? "";
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.useRealTimers();
});

describe("暂停任务：跨年的时间带年份", () => {
  it("暂停时间与归档时间：今年的省年份，去年的带上年份", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date(2026, 9, 1, 12, 0));
    const PausedTorrents = (await import("./PausedTorrents.vue")).default;
    view = mountView(PausedTorrents);

    await vi.waitFor(() => expect(timeCell("去年暂停")).not.toBe(""));
    expect(timeCell("去年暂停")).toBe("2025-12-31 23:30");
    expect(timeCell("今年暂停")).toBe("09-15 14:32");
    await vi.waitFor(() => expect(timeCell("去年归档")).not.toBe(""));
    expect(timeCell("去年归档")).toBe("2025-06-01 08:05");
  });
});
