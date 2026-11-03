// @vitest-environment happy-dom
/*
 * 钉子（M5）：导入已有媒体库的几个接口，以及整理历史的删除与批量带上的参数：allow_existing 只在页面上确认过、
 * 要连文件删除导入的已有文件时才带。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { organizeApi } from "@/api";

const fetchMock = vi.fn();
beforeEach(() => {
  fetchMock.mockImplementation(
    async () =>
      new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { "content-type": "application/json" },
      }),
  );
  vi.stubGlobal("fetch", fetchMock);
});
afterEach(() => {
  vi.unstubAllGlobals();
  fetchMock.mockReset();
});

/** 最后一次请求：地址、方法与请求体 */
function last() {
  const [url, init] = fetchMock.mock.calls.at(-1) as [string, RequestInit | undefined];
  return {
    url,
    method: init?.method ?? "GET",
    body: typeof init?.body === "string" ? JSON.parse(init.body) : undefined,
  };
}

describe("整理历史的删除与批量", () => {
  it("连文件删除才带 files；确认过导入的已有文件才带 allow_existing", async () => {
    await organizeApi.deleteHistory(3);
    expect(last()).toMatchObject({ url: "/api/media/history/3", method: "DELETE" });
    await organizeApi.deleteHistory(3, true);
    expect(last().url).toBe("/api/media/history/3?files=1");
    await organizeApi.deleteHistory(3, true, true);
    expect(last().url).toBe("/api/media/history/3?files=1&allow_existing=1");
    await organizeApi.deleteHistory(3, false, true);
    expect(last().url, "只删记录时不带").toBe("/api/media/history/3");
  });

  it("批量：确认过才带 allow_existing", async () => {
    await organizeApi.historyBatch([1, 2], "delete_files");
    expect(last()).toMatchObject({ url: "/api/media/history/batch", method: "POST" });
    expect(last().body).toEqual({ ids: [1, 2], action: "delete_files" });
    await organizeApi.historyBatch([1, 2], "delete_files", true);
    expect(last().body).toEqual({ ids: [1, 2], action: "delete_files", allow_existing: true });
  });
});

describe("导入已有媒体库", () => {
  it("扫描、看进度、取消、改一项、导入", async () => {
    await organizeApi.importScan(3, "/media/movies/华语电影");
    expect(last()).toEqual({
      url: "/api/media/libraries/3/import/scan",
      method: "POST",
      body: { path: "/media/movies/华语电影" },
    });
    await organizeApi.importScanStatus("0123456789abcdef", true);
    expect(last()).toMatchObject({
      url: "/api/media/import/scans/0123456789abcdef?brief=1",
      method: "GET",
    });
    await organizeApi.importScanStatus("0123456789abcdef");
    expect(last().url).toBe("/api/media/import/scans/0123456789abcdef");
    await organizeApi.cancelImportScan("0123456789abcdef");
    expect(last()).toMatchObject({
      url: "/api/media/import/scans/0123456789abcdef",
      method: "DELETE",
    });
    await organizeApi.setImportItem("0123456789abcdef", "/media/movies/a", 194);
    expect(last()).toEqual({
      url: "/api/media/import/scans/0123456789abcdef/item",
      method: "PUT",
      body: { path: "/media/movies/a", tmdb_id: 194 },
    });
    await organizeApi.importItems("0123456789abcdef", ["/media/movies/a"], true);
    expect(last()).toEqual({
      url: "/api/media/import/scans/0123456789abcdef/import",
      method: "POST",
      body: { paths: ["/media/movies/a"], scrape: true },
    });
  });
});
