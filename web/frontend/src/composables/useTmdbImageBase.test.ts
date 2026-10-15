/*
 * 钉子：海报用的图片地址。打开网页以后只读一次设置，读到以前不给海报地址；读不到时用原来的地址，下次再试。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

const media = vi.hoisted(() => ({ settings: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, mediaApi: media };
});

// 每个用例用新的模块：「已经读过」记在模块里
async function fresh() {
  vi.resetModules();
  const { loadTmdbImageBase } = await import("./useTmdbImageBase");
  const { posterURL } = await import("@/utils/media");
  return { loadTmdbImageBase, posterURL };
}

afterEach(() => {
  vi.clearAllMocks();
});

describe("海报的图片地址", () => {
  it("读设置里的图片地址，只读一次", async () => {
    media.settings.mockResolvedValue({ image_base_url: "https://static.example.com/t/p/" });
    const { loadTmdbImageBase, posterURL } = await fresh();
    expect(posterURL("/p.jpg"), "没读过时用官方地址").toBe("https://image.tmdb.org/t/p/w185/p.jpg");
    const first = loadTmdbImageBase();
    expect(posterURL("/p.jpg"), "读到以前先不加载海报").toBe("");
    await Promise.all([first, loadTmdbImageBase()]);
    await loadTmdbImageBase();
    expect(media.settings).toHaveBeenCalledTimes(1);
    expect(posterURL("/p.jpg")).toBe("https://static.example.com/t/p/w185/p.jpg");
  });

  it("设置里没换时用官方地址", async () => {
    media.settings.mockResolvedValue({ image_base_url: "" });
    const { loadTmdbImageBase, posterURL } = await fresh();
    await loadTmdbImageBase();
    expect(posterURL("/p.jpg", "w92")).toBe("https://image.tmdb.org/t/p/w92/p.jpg");
  });

  it("读不到时保持官方地址，下次调用再读", async () => {
    media.settings.mockRejectedValueOnce(new Error("未登录"));
    media.settings.mockResolvedValueOnce({ image_base_url: "http://10.0.0.2:9000/img/" });
    const { loadTmdbImageBase, posterURL } = await fresh();
    await loadTmdbImageBase();
    expect(posterURL("/p.jpg")).toBe("https://image.tmdb.org/t/p/w185/p.jpg");
    await loadTmdbImageBase();
    expect(media.settings).toHaveBeenCalledTimes(2);
    expect(posterURL("/p.jpg")).toBe("http://10.0.0.2:9000/img/w185/p.jpg");
  });
});
