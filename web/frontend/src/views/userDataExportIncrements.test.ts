// @vitest-environment happy-dom
/*
 * 钉子：分享图可以附上周期增量（预览就是导出的那张图）。有可比快照时海报上多出「本周增量」那一块；
 * 关掉开关、或还没有可比快照时不放；切换周期重新取该周期的增量。
 * 还有样式选择：点缩略图换样式、点配色换配色，选择记在浏览器里；下载时把预览的那个节点画成 PNG。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ getSummary: vi.fn() }));
const shot = vi.hoisted(() => ({ domToBlob: vi.fn() }));

const GiB = 1024 ** 3;

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    userInfoApi: {
      ...real.userInfoApi,
      getAggregated: vi.fn(() =>
        Promise.resolve({
          totalUploaded: 100 * GiB,
          totalDownloaded: 10 * GiB,
          averageRatio: 10,
          totalSeeding: 5,
          totalLeeching: 0,
          totalBonus: 1000,
          siteCount: 0,
          lastUpdate: 1758000000,
          perSiteStats: [],
        }),
      ),
      getSummary: api.getSummary,
    },
  };
});

/* 图标要从 /api/favicon 取：测试里不联网，直接给空表；字体也不加载 */
vi.mock("@/components/exportPoster/posterImage", async (orig) => {
  const real = await orig<typeof import("@/components/exportPoster/posterImage")>();
  return {
    ...real,
    ensurePosterFonts: () => Promise.resolve(),
    loadPosterLogos: () => Promise.resolve(new Map()),
  };
});

vi.mock("modern-screenshot", () => ({ domToBlob: shot.domToBlob }));

function summaryOf(range: string, hasBaseline: boolean) {
  return {
    range,
    from: "2026-09-30",
    to: "2026-10-06",
    sites: [
      {
        site: "hdsky",
        from: "2026-09-29",
        to: "2026-10-06",
        uploaded: hasBaseline ? 4 * GiB : 0,
        downloaded: hasBaseline ? 1 * GiB : 0,
        bonus: hasBaseline ? 250 : 0,
        hasBaseline,
        negative: false,
      },
    ],
    totalUploaded: hasBaseline ? 4 * GiB : 0,
    totalDownloaded: hasBaseline ? 1 * GiB : 0,
    totalBonus: hasBaseline ? 250 : 0,
  };
}

let view: MountedView | null = null;
beforeEach(() => localStorage.clear());
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

async function mountExport() {
  const Page = (await import("./UserDataExport.vue")).default;
  view = mountView(Page);
}

/** 海报上的文字（空白压成一个空格） */
function posterText(): string {
  return (document.querySelector(".poster")?.textContent ?? "").replace(/\s+/g, " ");
}

describe("分享图：周期增量", () => {
  it("有可比快照：海报上多出本周增量（上传、下载、魔力）", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", true));
    await mountExport();

    await vi.waitFor(() => expect(posterText()).toContain("本周增量"));
    expect(api.getSummary).toHaveBeenCalledWith("7d");
    expect(posterText()).toContain("+4 GB");
    expect(posterText()).toContain("+1 GB");
    expect(posterText()).toContain("+250");
    expect(posterText(), "账号的下载量写明是计费口径").toContain("下载（计费）");
  });

  it("关掉开关：增量撤下", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", true));
    await mountExport();
    await vi.waitFor(() => expect(posterText()).toContain("本周增量"));

    document.querySelector<HTMLElement>("[data-testid=export-increments]")!.click();
    await vi.waitFor(() => expect(posterText()).not.toContain("本周增量"));
  });

  it("还没有可比快照：不放增量，不在图上印一排 +0", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", false));
    await mountExport();
    await vi.waitFor(() => expect(api.getSummary).toHaveBeenCalled());
    await vi.waitFor(() => expect(document.querySelector(".poster")).not.toBeNull());
    await new Promise((r) => setTimeout(r, 0));
    expect(posterText()).not.toContain("增量");
  });

  it("切到本月：重新取本月的增量，标签跟着换", async () => {
    api.getSummary.mockImplementation((range: string) => Promise.resolve(summaryOf(range, true)));
    await mountExport();
    await vi.waitFor(() => expect(posterText()).toContain("本周增量"));

    const chip = document.querySelector("[data-testid=export-increment-range]")!;
    (chip.querySelector<HTMLElement>(".el-select__wrapper") ?? (chip as HTMLElement)).click();
    await vi.waitFor(() => {
      const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
        (o) => (o.textContent ?? "").trim() === "周期: 本月",
      );
      expect(opt).toBeTruthy();
      opt!.click();
    });
    await vi.waitFor(() => expect(api.getSummary).toHaveBeenCalledWith("30d"));
    await vi.waitFor(() => expect(posterText()).toContain("本月增量"));
    expect(posterText()).not.toContain("本周增量");
  });
});

describe("分享图：样式与导出", () => {
  it("默认是便当；点缩略图换样式、点配色换配色，选择记在浏览器里", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", false));
    await mountExport();
    await vi.waitFor(() => expect(document.querySelector(".poster-bento.is-light")).not.toBeNull());

    document.querySelector<HTMLElement>("[data-testid=poster-style-receipt]")!.click();
    await vi.waitFor(() =>
      expect(document.querySelector(".poster-receipt.is-white")).not.toBeNull(),
    );
    document.querySelector<HTMLElement>("[data-testid=poster-colorway-kraft]")!.click();
    await vi.waitFor(() =>
      expect(document.querySelector(".poster-receipt.is-kraft")).not.toBeNull(),
    );

    const saved = JSON.parse(localStorage.getItem("pt-tools:export-poster") ?? "{}");
    expect(saved).toMatchObject({ style: "receipt", colorways: { receipt: "kraft" } });

    view!.unmount();
    view = null;
    await mountExport();
    await vi.waitFor(() =>
      expect(document.querySelector(".poster-receipt.is-kraft")).not.toBeNull(),
    );
  });

  it("复制图片：在点击的同一轮把「画好的图」交给剪贴板", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", false));
    shot.domToBlob.mockResolvedValue(new Blob(["png"], { type: "image/png" }));
    class FakeItem {
      constructor(public data: Record<string, unknown>) {}
    }
    const write = vi.fn((_items: FakeItem[]) => Promise.resolve());
    vi.stubGlobal("ClipboardItem", FakeItem);
    const secure = Object.getOwnPropertyDescriptor(window, "isSecureContext");
    const clip = Object.getOwnPropertyDescriptor(navigator, "clipboard");
    Object.defineProperty(window, "isSecureContext", { value: true, configurable: true });
    Object.defineProperty(navigator, "clipboard", { value: { write }, configurable: true });
    try {
      await mountExport();
      await vi.waitFor(() => expect(document.querySelector(".poster")).not.toBeNull());
      buttonByText("复制图片").click();
      await vi.waitFor(() => expect(write).toHaveBeenCalledTimes(1));
      const [item] = write.mock.calls[0]![0];
      // 交进去的是 Promise（Safari 要求 write 在点击的同一轮里调用），兑现以后是 PNG
      await expect(item!.data["image/png"]).resolves.toBeInstanceOf(Blob);
      expect(shot.domToBlob).toHaveBeenCalledTimes(1);
    } finally {
      vi.unstubAllGlobals();
      if (secure) Object.defineProperty(window, "isSecureContext", secure);
      else delete (window as { isSecureContext?: boolean }).isSecureContext;
      if (clip) Object.defineProperty(navigator, "clipboard", clip);
      else delete (navigator as { clipboard?: unknown }).clipboard;
    }
  });

  it("下载图片：把预览的海报节点按 2 倍画成 PNG", async () => {
    api.getSummary.mockResolvedValue(summaryOf("7d", true));
    shot.domToBlob.mockResolvedValue(new Blob(["png"], { type: "image/png" }));
    const createUrl = vi.spyOn(URL, "createObjectURL").mockReturnValue("blob:poster");
    await mountExport();
    await vi.waitFor(() => expect(document.querySelector(".poster")).not.toBeNull());

    buttonByText("下载图片").click();
    await vi.waitFor(() => expect(shot.domToBlob).toHaveBeenCalledTimes(1));
    const [node, opts] = shot.domToBlob.mock.calls[0]!;
    expect(node).toBe(document.querySelector(".poster"));
    expect(opts).toMatchObject({ scale: 2, type: "image/png" });
    expect(typeof opts.font.cssText).toBe("string");
    await vi.waitFor(() => expect(createUrl).toHaveBeenCalled());
    createUrl.mockRestore();
  });
});
