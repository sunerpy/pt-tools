// @vitest-environment happy-dom
/*
 * 钉子：下载器连通性检查超时单列一档「超时」，说明可能离线或网络不通，不再配一句安抚。
 *
 * 前端每台只等 5 秒，后端的 qB 客户端要等 30 秒才放弃 —— 下载器离线时最常见的样子正是前端先超时。
 * 原来超时和「请求没发出去」一起归成黄色「未知」，还写着「不代表下载器有问题」「不等于下载器坏了」，
 * 把最该提醒的那一种情形说成了没事。与 origin/main 上「检查超时」（红色、算作检查失败）的语义对齐。
 * 请求本身失败（pt-tools 自己的接口报错、网络断了）仍是「未知」：那是真的没拿到结论。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    downloadersApi: {
      list: vi.fn(() =>
        Promise.resolve([
          {
            id: 1,
            name: "qb-家里",
            type: "qbittorrent",
            url: "http://192.168.1.2:8080",
            username: "admin",
            is_default: true,
            enabled: true,
          },
        ]),
      ),
    },
    downloaderDirectoriesApi: { listAll: vi.fn(() => Promise.resolve({})) },
    dynamicSitesApi: { getDownloaderSummary: vi.fn(() => Promise.resolve({ sites: [] })) },
    globalApi: { get: vi.fn(() => Promise.resolve(null)) },
  };
});

const REASSURANCE = ["不代表下载器有问题", "不等于下载器坏了"];

let view: MountedView | null = null;

/** /api/downloaders/1/health 的行为：超时（AbortError）、请求失败、或者正常回结论 */
function stubHealth(kind: "timeout" | "network" | "healthy") {
  vi.stubGlobal(
    "fetch",
    vi.fn(() => {
      if (kind === "timeout") {
        return Promise.reject(new DOMException("The operation was aborted.", "AbortError"));
      }
      if (kind === "network") return Promise.reject(new TypeError("Failed to fetch"));
      return Promise.resolve(
        new Response(JSON.stringify({ name: "qb-家里", is_healthy: true, message: "" }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    }),
  );
}

function mobileViewport() {
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches: query.includes("max-width: 768px"),
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

async function mountPage() {
  const DownloaderSettings = (await import("./DownloaderSettings.vue")).default;
  view = mountView(DownloaderSettings);
}

/** 桌面表格「连通性」那一列的胶囊（这张表里只有它用 PtStatusPill） */
function tablePill(): HTMLElement | null {
  return document.querySelector<HTMLElement>(".pt-grid .el-table__body .pt-pill");
}

/** 「连通性检查」卡里这台下载器那一行：值、柱子色调、说明 */
function healthCardRow() {
  const panel = [...document.querySelectorAll<HTMLElement>(".pt-panel")].find(
    (p) => p.querySelector(".pt-panel__title")?.textContent?.trim() === "连通性检查",
  );
  const row = panel?.querySelector<HTMLElement>(".bd__row");
  return {
    value: row?.querySelector(".bd__v")?.textContent?.trim() ?? "",
    bar: row?.querySelector(".bd__bar")?.className ?? "",
    hint: row?.querySelector(".bd__hint")?.textContent?.trim() ?? "",
    foot: panel?.querySelector(".bd__foot")?.textContent ?? "",
  };
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("连通性检查：超时单列一档", () => {
  it("超时：表格胶囊写「超时」（红色），卡里说明可能离线或网络不通，整页没有那句安抚", async () => {
    stubHealth("timeout");
    await mountPage();

    await vi.waitFor(() => expect(tablePill()?.textContent?.trim()).toBe("超时"));
    expect(tablePill()!.className).toContain("pt-pill--dang");
    const row = healthCardRow();
    expect(row.value).toBe("超时");
    expect(row.bar).toContain("is-dang");
    expect(row.hint).toContain("可能离线");
    for (const s of REASSURANCE) expect(document.body.textContent).not.toContain(s);
  });

  it("超时不算「没探到」：不挂部分失败的提示条", async () => {
    stubHealth("timeout");
    await mountPage();
    await vi.waitFor(() => expect(tablePill()?.textContent?.trim()).toBe("超时"));
    expect(document.querySelector(".partial-note")).toBeNull();
  });

  it("手机行卡：超时的原因直接排在卡上（没有 tooltip 可用），同样不安抚", async () => {
    mobileViewport();
    stubHealth("timeout");
    await mountPage();

    await vi.waitFor(() =>
      expect(document.querySelector(".card-reason")?.textContent ?? "").toContain("可能离线"),
    );
    expect(document.querySelector(".card-reason")!.className).toContain("is-dang");
    for (const s of REASSURANCE) expect(document.body.textContent).not.toContain(s);
  });

  it("请求本身失败（不是超时）：仍是「未知」，原因照实写，不安抚", async () => {
    stubHealth("network");
    await mountPage();

    await vi.waitFor(() => expect(tablePill()?.textContent?.trim()).toBe("未知"));
    const row = healthCardRow();
    expect(row.value).toBe("未知");
    expect(row.hint).toContain("Failed to fetch");
    for (const s of REASSURANCE) expect(document.body.textContent).not.toContain(s);
    // 这一种才是「没探到」：partial 提示条照旧在
    expect(document.querySelector(".partial-note")).not.toBeNull();
  });

  it("检查成功：正常", async () => {
    stubHealth("healthy");
    await mountPage();
    await vi.waitFor(() => expect(tablePill()?.textContent?.trim()).toBe("正常"));
    expect(healthCardRow().value).toBe("正常");
  });
});
