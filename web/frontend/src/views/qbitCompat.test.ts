// @vitest-environment happy-dom
/*
 * 钉子：qB 兼容入口页（路线图 M13）。没开监听时写明怎么开；开了时给客户端填的地址（监听在 0.0.0.0 上时用浏览器里的主机名）；
 * 保存只发下载器与完全控制两项；读不到时写明原因。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn(), list: vi.fn() }));
const ui = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    qbitCompatApi: { get: api.get, save: api.save },
    downloadersApi: { ...real.downloadersApi, list: api.list },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: ui.error,
      warning: ui.warning,
      info: vi.fn(),
    }),
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);

const downloaders = [
  {
    id: 1,
    name: "qb-home",
    type: "qbittorrent",
    url: "http://x",
    username: "",
    is_default: true,
    enabled: true,
  },
  {
    id: 2,
    name: "tr-nas",
    type: "transmission",
    url: "http://y",
    username: "",
    is_default: false,
    enabled: true,
  },
  {
    id: 3,
    name: "off",
    type: "qbittorrent",
    url: "http://z",
    username: "",
    is_default: false,
    enabled: false,
  },
];

function status(over: Record<string, unknown> = {}) {
  return {
    listen_addr: "",
    listening: false,
    downloader_id: 0,
    full_control: false,
    downloader: "qb-home",
    compat_torrents: 3,
    ...over,
  };
}

async function mountPage() {
  api.list.mockResolvedValue(downloaders);
  const Page = (await import("./QbitCompat.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.get).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("qB 兼容入口", () => {
  it("没开监听：写明怎么开，地址先不给", async () => {
    api.get.mockResolvedValue(status());
    await mountPage();
    expect(q("qc-off")!.textContent).toContain("--qbit-compat-addr");
    expect(q("qc-off")!.textContent).toContain("PT_QBIT_COMPAT_ADDR");
    expect(q("qc-status")!.textContent).toContain("没有开启");
    expect(q("qc-status")!.textContent).toContain("3 个");
    expect(q("qc-url")).toBeNull();
  });

  it("开了监听：监听在 0.0.0.0 上时客户端地址用浏览器里的主机名", async () => {
    api.get.mockResolvedValue(status({ listen_addr: "0.0.0.0:8081", listening: true }));
    await mountPage();
    expect(q("qc-off")).toBeNull();
    expect(q("qc-status")!.textContent).toContain("在监听");
    expect(q("qc-url")!.textContent).toBe(`http://${window.location.hostname}:8081`);
  });

  it("开了监听：监听在具体地址上时用它", async () => {
    api.get.mockResolvedValue(status({ listen_addr: "192.168.1.5:8081", listening: true }));
    await mountPage();
    expect(q("qc-url")!.textContent).toBe("http://192.168.1.5:8081");
  });

  it("保存只发下载器与完全控制；回来的状态写回表单", async () => {
    api.get.mockResolvedValue(status());
    api.save.mockResolvedValue(
      status({ full_control: true, downloader_id: 2, downloader: "tr-nas" }),
    );
    await mountPage();
    const sw = q("qc-full")!;
    (sw.querySelector("input") ?? sw).click();
    await flush();
    q("qc-save")!.click();
    await vi.waitFor(() => expect(api.save).toHaveBeenCalledTimes(1));
    expect(api.save.mock.calls[0]![0]).toEqual({ downloader_id: 0, full_control: true });
    await vi.waitFor(() => expect(q("qc-status")!.textContent).toContain("tr-nas"));
    expect(ui.success).toHaveBeenCalledWith("已保存");
  });

  it("下载器下拉只列启用的，默认那项写明现在是哪台", async () => {
    api.get.mockResolvedValue(status());
    await mountPage();
    const sel = q("qc-downloader")!;
    (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
    await vi.waitFor(() =>
      expect(document.querySelectorAll(".el-select-dropdown__item").length).toBe(3),
    );
    const labels = [...document.querySelectorAll(".el-select-dropdown__item")].map((o) =>
      o.textContent?.trim(),
    );
    expect(labels).toEqual([
      "默认下载器（现在是 qb-home）",
      "qb-home（qBittorrent）",
      "tr-nas（Transmission）",
    ]);
  });

  it("读不到时写明原因", async () => {
    api.get.mockRejectedValue(new Error("读取 qB 兼容入口状态失败"));
    await mountPage();
    expect(q("qc-state")!.textContent).toContain("读取 qB 兼容入口状态失败");
  });
});
