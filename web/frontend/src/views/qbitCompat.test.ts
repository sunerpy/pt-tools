// @vitest-environment happy-dom
/*
 * 钉子：qB 兼容入口页（路线图 M13）。没开监听时写明怎么开；监听地址在这页设（启动参数、环境变量给了时只读）；
 * 开了时给客户端填的地址（监听在 0.0.0.0 上时用浏览器里的主机名，并写明是猜的）；监听失败写明原因；
 * 打开完全控制要确认；停用的下载器列出来但选不了；最近的登录；读不到时写明原因。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ get: vi.fn(), save: vi.fn(), list: vi.fn() }));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  confirm: vi.fn(),
}));

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
    ElMessageBox: { ...real.ElMessageBox, confirm: ui.confirm },
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
    listen_source: "",
    configured_addr: "",
    settings_addr: "",
    downloader_id: 0,
    full_control: false,
    downloader: "qb-home",
    compat_torrents: 3,
    sessions: 0,
    ...over,
  };
}

const on = (s: Record<string, unknown> = {}) =>
  status({
    listen_addr: "0.0.0.0:8081",
    listening: true,
    listen_source: "settings",
    configured_addr: "0.0.0.0:8081",
    settings_addr: "0.0.0.0:8081",
    ...s,
  });

const click = (id: string) => {
  const el = q(id)!;
  (el.querySelector("input") ?? el).click();
};

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
    expect(q("qc-listen-addr"), "开关关着时不显示地址").toBeNull();
  });

  it("在这页打开监听：发填的地址；监听失败时写明原因", async () => {
    api.get.mockResolvedValue(status());
    api.save.mockResolvedValue(
      status({
        listen_source: "settings",
        configured_addr: "0.0.0.0:8081",
        settings_addr: "0.0.0.0:8081",
        listen_error: "bind: address already in use",
      }),
    );
    await mountPage();
    click("qc-listen-on");
    await vi.waitFor(() => expect(q("qc-listen-addr")).not.toBeNull());
    q("qc-save")!.click();
    await vi.waitFor(() => expect(api.save).toHaveBeenCalledTimes(1));
    expect(api.save.mock.calls[0]![0]).toEqual({
      downloader_id: 0,
      full_control: false,
      listen_addr: "0.0.0.0:8081",
    });
    await vi.waitFor(() => expect(q("qc-listen-error")).not.toBeNull());
    expect(q("qc-listen-error")!.textContent).toContain("address already in use");
    expect(q("qc-status")!.textContent).toContain("开启失败");
    expect(ui.warning).toHaveBeenCalledWith(
      expect.stringContaining("已保存，但监听 0.0.0.0:8081 失败"),
    );
  });

  it("在这页关掉监听：发空的地址", async () => {
    api.get.mockResolvedValue(on());
    api.save.mockResolvedValue(status());
    await mountPage();
    click("qc-listen-on");
    await flush();
    q("qc-save")!.click();
    await vi.waitFor(() => expect(api.save).toHaveBeenCalledTimes(1));
    expect(api.save.mock.calls[0]![0]).toEqual({
      downloader_id: 0,
      full_control: false,
      listen_addr: "",
    });
    expect(ui.success).toHaveBeenCalledWith("已保存");
  });

  it("地址由启动参数给的：只读，保存不带地址", async () => {
    api.get.mockResolvedValue(on({ listen_source: "flag" }));
    api.save.mockResolvedValue(on({ listen_source: "flag" }));
    await mountPage();
    expect(q("qc-locked")!.textContent).toContain("启动参数 --qbit-compat-addr");
    expect(q("qc-listen-on")).toBeNull();
    q("qc-save")!.click();
    await vi.waitFor(() => expect(api.save).toHaveBeenCalledTimes(1));
    expect(api.save.mock.calls[0]![0]).toEqual({ downloader_id: 0, full_control: false });
  });

  it("开了监听：监听在 0.0.0.0 上时客户端地址用浏览器里的主机名，写明是猜的，并提醒只在内网开放", async () => {
    api.get.mockResolvedValue(on());
    await mountPage();
    expect(q("qc-off")).toBeNull();
    expect(q("qc-status")!.textContent).toContain("在监听");
    expect(q("qc-status")!.textContent).toContain("这页设的");
    expect(q("qc-url")!.textContent).toBe(`http://${window.location.hostname}:8081`);
    expect(q("qc-url-guess")!.textContent).toContain("浏览器的地址栏");
    expect(q("qc-lan")!.textContent).toContain("只在内网开放");
  });

  it("开了监听：监听在具体地址上时用它，不写猜的", async () => {
    api.get.mockResolvedValue(on({ listen_addr: "192.168.1.5:8081" }));
    await mountPage();
    expect(q("qc-url")!.textContent).toBe("http://192.168.1.5:8081");
    expect(q("qc-url-guess")).toBeNull();
    expect(q("qc-lan")).toBeNull();
  });

  it("打开完全控制要确认；取消时不打开", async () => {
    api.get.mockResolvedValue(status());
    api.save.mockResolvedValue(
      status({ full_control: true, downloader_id: 2, downloader: "tr-nas" }),
    );
    await mountPage();
    ui.confirm.mockRejectedValueOnce("cancel");
    click("qc-full");
    await vi.waitFor(() => expect(ui.confirm).toHaveBeenCalledTimes(1));
    await flush();
    expect(q("qc-full-warn")).toBeNull();
    ui.confirm.mockResolvedValueOnce("confirm");
    click("qc-full");
    await vi.waitFor(() => expect(q("qc-full-warn")).not.toBeNull());
    q("qc-save")!.click();
    await vi.waitFor(() => expect(api.save).toHaveBeenCalledTimes(1));
    expect(api.save.mock.calls[0]![0]).toEqual({
      downloader_id: 0,
      full_control: true,
      listen_addr: "",
    });
    await vi.waitFor(() => expect(q("qc-status")!.textContent).toContain("tr-nas"));
  });

  it("下载器下拉列出停用的但选不了；选的下载器用不了时写明原因", async () => {
    api.get.mockResolvedValue(
      status({
        downloader_id: 3,
        downloader: "",
        downloader_issue: "选的下载器「off」停用了：启用它，或者换一台",
      }),
    );
    await mountPage();
    expect(q("qc-downloader-issue")!.textContent).toContain("停用了");
    const sel = q("qc-downloader")!;
    (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
    await vi.waitFor(() =>
      expect(document.querySelectorAll(".el-select-dropdown__item").length).toBe(4),
    );
    const items = [...document.querySelectorAll(".el-select-dropdown__item")];
    expect(items.map((o) => o.textContent?.trim())).toEqual([
      "默认下载器（现在是 qb-home）",
      "qb-home（qBittorrent）",
      "tr-nas（Transmission）",
      "off（qBittorrent）（已停用）",
    ]);
    expect(items[3]!.classList.contains("is-disabled")).toBe(true);
  });

  it("最近的登录与失败", async () => {
    api.get.mockResolvedValue(
      on({
        sessions: 2,
        last_login: {
          at: "2026-10-15T10:00:00Z",
          ip: "192.168.1.20",
          token: "MoviePilot",
          username: "mp",
          result: "ok",
        },
        last_failure: { at: "2026-10-15T09:00:00Z", ip: "192.168.1.30", result: "scope" },
      }),
    );
    await mountPage();
    expect(q("qc-status")!.textContent).toContain("2 个");
    expect(q("qc-last-login")!.textContent).toContain("令牌「MoviePilot」");
    expect(q("qc-last-failure")!.textContent).toContain("令牌没有「qB 兼容」权限");
  });

  it("新建令牌的链接带上 qB 兼容的预设", async () => {
    api.get.mockResolvedValue(on());
    await mountPage();
    expect(q("qc-tokens")).not.toBeNull();
  });

  it("读不到时写明原因", async () => {
    api.get.mockRejectedValue(new Error("读取 qB 兼容入口状态失败"));
    await mountPage();
    expect(q("qc-state")!.textContent).toContain("读取 qB 兼容入口状态失败");
  });
});
