// @vitest-environment happy-dom
/*
 * 钉子：连搜两次时，晚到的旧搜索不能顶掉新结果，也不能弹一条「搜索失败」。
 *
 * 搜索按钮在加载中是禁用的，但查询框里按回车照样能再搜一次。useDataState.run 原来没有请求序号：
 * 旧搜索 A 晚到时，它的结果覆盖新搜索 B；A 失败的话还把 error 写上、走失败分支清空 B 的结果、
 * 再弹「搜索失败」。现在 run 只让最新一次写状态，被顶掉的 A 返回 null，页面用 isStale 认出它直接 return。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ multiSite: vi.fn() }));
const ui = vi.hoisted(() => ({ error: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    searchApi: {
      getSites: vi.fn(() => Promise.resolve(["mteam"])),
      multiSite: api.multiSite,
      clearCache: vi.fn(),
    },
    downloadersApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderDirectoriesApi: { listAll: vi.fn(() => Promise.resolve({})) },
    siteCategoriesApi: { getAll: vi.fn(() => Promise.resolve({})) },
    torrentPushApi: { push: vi.fn(), batchPush: vi.fn() },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), {
      success: vi.fn(),
      error: ui.error,
      warning: vi.fn(),
      info: vi.fn(),
    }),
  };
});

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function response(title: string) {
  return {
    items: [
      {
        id: title,
        title,
        category: "电影/HD",
        isFree: false,
        sizeBytes: 1024 ** 3,
        seeders: 1,
        leechers: 0,
        snatched: 0,
        sourceSite: "mteam",
      },
    ],
    totalResults: 1,
    siteResults: { mteam: 1 },
    errors: [],
    durationMs: 10,
  };
}

let view: MountedView | null = null;

/** 在查询框里输入关键词并按回车（加载中按钮是禁用的，回车不是） */
async function searchFor(keyword: string) {
  const input = document.querySelector<HTMLInputElement>(
    "input[placeholder='输入关键词，回车即搜']",
  )!;
  input.value = keyword;
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await Promise.resolve();
  input.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true }));
}

const tableText = () => document.querySelector(".el-table__body")?.textContent ?? "";

async function mountPage() {
  sessionStorage.clear();
  const TorrentSearch = (await import("./TorrentSearch.vue")).default;
  view = mountView(TorrentSearch);
  await vi.waitFor(() =>
    expect(document.querySelector("input[placeholder='输入关键词，回车即搜']")).not.toBeNull(),
  );
}

afterEach(() => {
  view?.unmount();
  view = null;
  sessionStorage.clear();
  vi.clearAllMocks();
});

describe("种子搜索：旧搜索晚到", () => {
  it("旧搜索晚到且失败：新结果留着，不弹「搜索失败」", async () => {
    const a = deferred<ReturnType<typeof response>>();
    const b = deferred<ReturnType<typeof response>>();
    api.multiSite.mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);
    await mountPage();

    await searchFor("甲");
    await vi.waitFor(() => expect(api.multiSite).toHaveBeenCalledTimes(1));
    await searchFor("乙");
    await vi.waitFor(() => expect(api.multiSite).toHaveBeenCalledTimes(2));

    b.resolve(response("乙的结果"));
    await vi.waitFor(() => expect(tableText()).toContain("乙的结果"));

    a.reject(new Error("站点超时"));
    await new Promise((r) => setTimeout(r, 30));
    expect(tableText()).toContain("乙的结果");
    expect(ui.error).not.toHaveBeenCalled();
  });

  it("旧搜索晚到且成功：也不能把新结果换成旧关键词的结果", async () => {
    const a = deferred<ReturnType<typeof response>>();
    const b = deferred<ReturnType<typeof response>>();
    api.multiSite.mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);
    await mountPage();

    await searchFor("甲");
    await vi.waitFor(() => expect(api.multiSite).toHaveBeenCalledTimes(1));
    await searchFor("乙");
    await vi.waitFor(() => expect(api.multiSite).toHaveBeenCalledTimes(2));

    b.resolve(response("乙的结果"));
    await vi.waitFor(() => expect(tableText()).toContain("乙的结果"));
    a.resolve(response("甲的结果"));
    await new Promise((r) => setTimeout(r, 30));
    expect(tableText()).toContain("乙的结果");
    expect(tableText()).not.toContain("甲的结果");
  });
});
