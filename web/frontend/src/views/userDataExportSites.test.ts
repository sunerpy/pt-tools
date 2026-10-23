// @vitest-environment happy-dom
/*
 * 钉子：分享图的隐私默认值与入图站点。
 * 用户名默认也打码（以前只遮站点名与图标，用户名明文，看上去像脱敏了其实没有）；
 * 「清空」就是图上不列逐站明细（以前一个都不勾时悄悄换成前 10 个，计数写 0 / N，图上照样十个站）。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const GiB = 1024 ** 3;

function site(name: string, username: string, up: number) {
  return {
    site: name,
    username,
    userId: "1",
    uploaded: up * GiB,
    downloaded: GiB,
    ratio: up,
    bonus: 100,
    seeding: 1,
    leeching: 0,
    rank: "",
    lastUpdate: 1758000000,
  };
}

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    userInfoApi: {
      ...real.userInfoApi,
      getAggregated: vi.fn(() =>
        Promise.resolve({
          totalUploaded: 30 * GiB,
          totalDownloaded: 2 * GiB,
          averageRatio: 15,
          totalSeeding: 2,
          totalLeeching: 0,
          totalBonus: 200,
          siteCount: 2,
          lastUpdate: 1758000000,
          perSiteStats: [site("hdsky", "lumen_pt", 20), site("ourbits", "lumen_ob", 10)],
        }),
      ),
      getSummary: vi.fn(() => Promise.resolve(null)),
    },
  };
});

vi.mock("@/components/exportPoster/posterImage", async (orig) => {
  const real = await orig<typeof import("@/components/exportPoster/posterImage")>();
  return {
    ...real,
    ensurePosterFonts: () => Promise.resolve(),
    loadPosterLogos: () => Promise.resolve(new Map()),
  };
});

vi.mock("modern-screenshot", () => ({ domToBlob: vi.fn() }));

let view: MountedView | null = null;
beforeEach(() => localStorage.clear());
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const rows = () => document.querySelectorAll(".poster .pb-site").length;
const posterText = () => document.querySelector(".poster")?.textContent ?? "";

async function mountExport() {
  const Page = (await import("./UserDataExport.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(rows()).toBe(2));
}

describe("分享图：隐私默认值与入图站点", () => {
  it("用户名默认打码：图上没有用户名原文，摘要写明三项都打码了", async () => {
    await mountExport();
    expect(posterText()).not.toContain("lumen_pt");
    expect(posterText()).not.toContain("lumen_ob");
    expect(document.body.textContent).toContain("已打码：用户名 / 站点名 / 图标");
  });

  it("「清空」以后图上不列逐站明细，「前 10 个」恢复默认", async () => {
    await mountExport();
    document.querySelector<HTMLElement>("[data-testid=ex-sites-clear]")!.click();
    await vi.waitFor(() => expect(rows()).toBe(0));
    expect(document.body.textContent).toContain("0 / 2");

    document.querySelector<HTMLElement>("[data-testid=ex-sites-top]")!.click();
    await vi.waitFor(() => expect(rows()).toBe(2));
    expect(document.body.textContent).toContain("2 / 2");
  });
});
