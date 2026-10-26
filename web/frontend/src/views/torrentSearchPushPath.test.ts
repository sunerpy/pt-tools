// @vitest-environment happy-dom
/*
 * 钉子：搜索页推送到下载器时的保存路径。以前只能从预设目录里选，现在点开列出所选下载器的预设目录，
 * 列表里没有的路径也能直接输入（2026-10-26 用户：「应支持下拉选择和自行输入」）。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ multiSite: vi.fn(), getSites: vi.fn(), push: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    searchApi: { getSites: api.getSites, multiSite: api.multiSite, clearCache: vi.fn() },
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
    downloaderDirectoriesApi: {
      listAll: vi.fn(() =>
        Promise.resolve({
          1: [
            { id: 3, downloader_id: 1, path: "/downloads", alias: "", is_default: true },
            {
              id: 4,
              downloader_id: 1,
              path: "/downloads/movies",
              alias: "电影",
              is_default: false,
            },
          ],
        }),
      ),
    },
    siteCategoriesApi: { getAll: vi.fn(() => Promise.resolve({})) },
    torrentPushApi: { push: api.push, batchPush: vi.fn() },
  };
});

let view: MountedView | null = null;

const result = {
  items: [
    {
      id: "98525",
      title: "Weathering.With.You.2019.2160p",
      sizeBytes: 20 * 1024 ** 3,
      seeders: 10,
      leechers: 0,
      sourceSite: "xingyunge",
      discountLevel: "FREE",
      downloadUrl: "/api/site/xingyunge/torrent/98525/download",
    },
  ],
  totalResults: 1,
  siteResults: { xingyunge: 1 },
  errors: [],
  durationMs: 5,
};

afterEach(() => {
  view?.unmount();
  view = null;
  sessionStorage.clear();
  vi.clearAllMocks();
});

async function openPush() {
  api.getSites.mockResolvedValue(["xingyunge"]);
  api.multiSite.mockResolvedValue(result);
  api.push.mockResolvedValue({ success: true, results: [], message: "" });
  const TorrentSearch = (await import("./TorrentSearch.vue")).default;
  view = mountView(TorrentSearch);
  const sel = "input[placeholder='输入关键词，回车即搜']";
  await vi.waitFor(() => expect(document.querySelector(sel)).not.toBeNull());
  const input = document.querySelector<HTMLInputElement>(sel)!;
  input.value = "天气之子";
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await Promise.resolve();
  input.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
  await vi.waitFor(() =>
    expect(
      document.querySelector("button[aria-label='推送到下载器']:not([disabled])"),
    ).not.toBeNull(),
  );
  document
    .querySelector<HTMLButtonElement>("button[aria-label='推送到下载器']:not([disabled])")!
    .click();
  await vi.waitFor(() =>
    expect(document.querySelector("[data-testid=push-save-path]")).not.toBeNull(),
  );
  const el = document.querySelector<HTMLElement>("[data-testid=push-save-path]")!;
  return (el instanceof HTMLInputElement ? el : el.querySelector("input")) as HTMLInputElement;
}

describe("种子搜索：推送时的保存路径", () => {
  it("点开列出默认下载器的预设目录，选一个推送", async () => {
    const path = await openPush();
    path.focus();
    path.dispatchEvent(new Event("focus"));
    let items: HTMLElement[] = [];
    await vi.waitFor(() => {
      items = [...document.querySelectorAll<HTMLElement>(".pt-path-input__popper li")];
      expect(items).toHaveLength(2);
    });
    items.find((li) => (li.textContent ?? "").includes("电影"))!.click();
    await vi.waitFor(() => expect(path.value).toBe("/downloads/movies"));
    buttonByText("推送").click();
    await vi.waitFor(() => expect(api.push).toHaveBeenCalledTimes(1));
    expect(api.push.mock.calls[0]![0].savePath).toBe("/downloads/movies");
  });

  it("列表里没有的路径直接输入", async () => {
    const path = await openPush();
    path.value = "/mnt/other";
    path.dispatchEvent(new Event("input", { bubbles: true }));
    await Promise.resolve();
    buttonByText("推送").click();
    await vi.waitFor(() => expect(api.push).toHaveBeenCalledTimes(1));
    expect(api.push.mock.calls[0]![0].savePath).toBe("/mnt/other");
  });
});
