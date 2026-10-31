// @vitest-environment happy-dom
/*
 * 钉子：系统设置里的「站点代理」。和每日战报一样走自己的接口：读回来的是打码地址与站点选项，
 * 改了才保存；全局配置先存，失败就停；之后每日战报与站点代理各存各的、两个都试，
 * 成功的更新自己的基线，失败的留着改动，一条提示说清哪些没保存。读不回来时整段禁用。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  saveGlobal: vi.fn(),
  getReport: vi.fn(),
  saveReport: vi.fn(),
  getProxy: vi.fn(),
  saveProxy: vi.fn(),
}));
const ui = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    globalApi: {
      get: vi.fn(() =>
        Promise.resolve({
          default_interval_minutes: 10,
          download_dir: "/downloads",
          download_limit_enabled: false,
          download_speed_limit: 20,
          torrent_size_gb: 200,
          torrent_min_size_gb: 0,
          min_free_minutes: 30,
          auto_start: false,
          retain_hours: 24,
          max_retry: 3,
          default_concurrency: 3,
          default_enabled: false,
          auto_delete_on_free_end: false,
          free_end_advance_minutes: 5,
          default_filter_mode: "auto_free",
        }),
      ),
      save: api.saveGlobal,
    },
    chatopsApi: {
      notifications: {
        list: vi.fn(() =>
          Promise.resolve([{ id: 7, name: "TG", channel_type: "telegram", enabled: true }]),
        ),
      },
    },
    userInfoApi: {
      ...real.userInfoApi,
      getDailyReport: api.getReport,
      saveDailyReport: api.saveReport,
    },
    siteProxyApi: { get: api.getProxy, save: api.saveProxy },
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

const OPTIONS = [
  { id: "hdsky", name: "HDSky", enabled: true },
  { id: "springsunday", name: "SSD", enabled: true },
  { id: "byrpt", name: "BYR", enabled: false },
];

function proxySettings(over: Record<string, unknown> = {}) {
  return {
    enabled: false,
    proxy_url: "",
    mode: "all",
    sites: [] as string[],
    updated_at: null,
    site_options: OPTIONS,
    ...over,
  };
}

/** 后端存好以后读回来的样子：密码打码、站点排好序 */
function echoSaved(v: { proxy_url: string; sites: string[] } & Record<string, unknown>) {
  return Promise.resolve(
    proxySettings({
      ...v,
      proxy_url: v.proxy_url.replace(/:([^:@/]+)@/, ":***@"),
      sites: [...v.sites].sort(),
      updated_at: "2026-10-31T12:00:00Z",
    }),
  );
}

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const q = <T extends HTMLElement = HTMLElement>(id: string) =>
  document.querySelector<T>(`[data-testid=${id}]`);

function saveButton(): HTMLButtonElement {
  const btn = [...document.querySelectorAll<HTMLButtonElement>("button")].find(
    (b) => (b.textContent ?? "").trim() === "保存设置" && !b.disabled,
  );
  if (!btn) throw new Error("找不到可点的「保存设置」");
  return btn;
}

const tick = () => new Promise((r) => setTimeout(r, 0));
const head = () => document.querySelector("#pt-head-sub")?.textContent ?? "";

function disabled(id: string): boolean {
  const el = q(id)!;
  return (
    el.classList.contains("is-disabled") ||
    el.querySelector(".is-disabled") !== null ||
    (el as HTMLInputElement).disabled === true
  );
}

async function mountPage(
  proxy: unknown = proxySettings(),
  report: unknown = { enabled: true, time: "22:00", channel_ids: [7] },
) {
  api.getReport.mockResolvedValue(report);
  if (proxy instanceof Error) api.getProxy.mockRejectedValue(proxy);
  else api.getProxy.mockResolvedValue(proxy);
  const Page = (await import("./GlobalSettings.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.getProxy).toHaveBeenCalled());
  await vi.waitFor(() => saveButton());
  await tick();
}

async function toggle(id: string) {
  q(id)!.click();
  await tick();
}

async function fillProxyUrl(value: string) {
  const input = q<HTMLInputElement>("site-proxy-url")!;
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
  await tick();
}

async function pickMode(mode: string) {
  const radio = [...q("site-proxy-mode")!.querySelectorAll<HTMLInputElement>("input")].find(
    (r) => r.value === mode,
  )!;
  radio.click();
  await vi.waitFor(() => expect(radio.checked).toBe(true));
}

/** 打开站点多选，按显示的名字点一项 */
async function pickSite(label: string) {
  const field = q("site-proxy-sites")!;
  (field.querySelector<HTMLElement>(".el-select__wrapper") ?? field).click();
  await vi.waitFor(() => {
    const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
      (o) => (o.textContent ?? "").trim() === label,
    );
    expect(opt).toBeTruthy();
    opt!.click();
  });
  await tick();
}

/** 改一下每日战报（关掉），让战报也成了要保存的一块 */
const changeReport = () => toggle("daily-report-enabled");

/** 开启站点代理、填上地址 */
async function enableProxy(url = "http://user:secret@192.168.3.150:1081") {
  await toggle("site-proxy-enabled");
  await fillProxyUrl(url);
}

describe("系统设置：站点代理的读取", () => {
  it("没有设置时：开关关着、地址空、全部站点，下面几项不可改，页头不提站点代理", async () => {
    await mountPage();
    expect(q("site-proxy-enabled")!.classList.contains("is-checked")).toBe(false);
    expect(q<HTMLInputElement>("site-proxy-url")!.value).toBe("");
    expect(q("site-proxy-mode")!.querySelector<HTMLInputElement>("input[value=all]")!.checked).toBe(
      true,
    );
    expect(q("site-proxy-sites")).toBeNull();
    expect(disabled("site-proxy-url")).toBe(true);
    expect(disabled("site-proxy-mode")).toBe(true);
    await vi.waitFor(() => expect(head()).toContain("智能模式"));
    expect(head()).not.toContain("站点代理");
    expect(q("site-proxy-load-failed")).toBeNull();
  });

  it("读回打码地址、模式与站点；选项来自 site_options，没启用的标出来；页头写选了几个站点", async () => {
    await mountPage(
      proxySettings({
        enabled: true,
        proxy_url: "http://user:***@192.168.3.150:1081",
        mode: "include",
        sites: ["hdsky"],
      }),
    );
    expect(q("site-proxy-enabled")!.classList.contains("is-checked")).toBe(true);
    expect(q<HTMLInputElement>("site-proxy-url")!.value).toBe("http://user:***@192.168.3.150:1081");
    expect(
      q("site-proxy-mode")!.querySelector<HTMLInputElement>("input[value=include]")!.checked,
    ).toBe(true);
    expect(q("site-proxy-sites")!.textContent).toContain("HDSky");
    await vi.waitFor(() => expect(head()).toContain("站点代理：1 个站点"));

    // 选项只来自接口：三个站点都在，没启用的带「未启用」
    await pickSite("SSD");
    const labels = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].map(
      (o) => (o.textContent ?? "").trim(),
    );
    expect(labels).toEqual(expect.arrayContaining(["HDSky", "SSD", "BYR（未启用）"]));
  });

  it("「除了选中的站点」不选站点时页头写全部站点；选了写「除 N 个站点」", async () => {
    await mountPage(
      proxySettings({ enabled: true, proxy_url: "http://p.test:1", mode: "exclude", sites: [] }),
    );
    await vi.waitFor(() => expect(head()).toContain("站点代理：全部站点"));
    view!.unmount();
    await mountPage(
      proxySettings({
        enabled: true,
        proxy_url: "http://p.test:1",
        mode: "exclude",
        sites: ["byrpt", "hdsky"],
      }),
    );
    await vi.waitFor(() => expect(head()).toContain("站点代理：除 2 个站点"));
  });

  it("存着的地址解不开：提示重新填", async () => {
    await mountPage(proxySettings({ enabled: true, proxy_url: "", proxy_unreadable: true }));
    expect(q("site-proxy-unreadable")?.textContent).toContain("重新填写代理地址");
  });

  it("读不回来：整段禁用并说明，保存时不碰站点代理", async () => {
    await mountPage(new Error("502"));
    await vi.waitFor(() => expect(q("site-proxy-load-failed")).toBeTruthy());
    expect(disabled("site-proxy-enabled")).toBe(true);
    await toggle("site-proxy-enabled");
    expect(q("site-proxy-enabled")!.classList.contains("is-checked")).toBe(false);

    api.saveGlobal.mockResolvedValue(undefined);
    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("保存成功"));
    expect(api.saveProxy).not.toHaveBeenCalled();
  });
});

describe("系统设置：站点代理的保存", () => {
  it("没改：不调站点代理的接口", async () => {
    await mountPage();
    api.saveGlobal.mockResolvedValue(undefined);
    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("保存成功"));
    expect(api.saveProxy).not.toHaveBeenCalled();
    expect(api.saveReport).not.toHaveBeenCalled();
  });

  it("开启却没填地址、「只有选中的站点」却没选：前端拦下，什么都不保存", async () => {
    await mountPage();
    await toggle("site-proxy-enabled");
    saveButton().click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("开启站点代理要填代理地址"));

    await fillProxyUrl("http://p.test:1");
    await pickMode("include");
    saveButton().click();
    await vi.waitFor(() =>
      expect(ui.error).toHaveBeenCalledWith("选了「只有选中的站点」，至少选一个站点"),
    );
    expect(api.saveGlobal).not.toHaveBeenCalled();
    expect(api.saveProxy).not.toHaveBeenCalled();
  });

  it("只改站点代理、成功：交上去的是表单里的值，页面换成打码地址，页头更新，再点一次不重复写", async () => {
    await mountPage();
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveProxy.mockImplementation(echoSaved);
    await enableProxy(" http://user:secret@192.168.3.150:1081 ");
    await pickMode("include");
    await pickSite("SSD");
    await pickSite("BYR（未启用）");

    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("保存成功"));
    expect(api.saveProxy).toHaveBeenCalledWith({
      enabled: true,
      proxy_url: "http://user:secret@192.168.3.150:1081",
      mode: "include",
      sites: ["springsunday", "byrpt"],
    });
    expect(api.saveReport).not.toHaveBeenCalled();
    await vi.waitFor(() =>
      expect(q<HTMLInputElement>("site-proxy-url")!.value).toBe(
        "http://user:***@192.168.3.150:1081",
      ),
    );
    await vi.waitFor(() => expect(head()).toContain("站点代理：2 个站点"));

    await vi.waitFor(() => saveButton());
    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledTimes(2));
    expect(api.saveProxy).toHaveBeenCalledTimes(1);
  });

  it("战报与站点代理都改、都成功：两个都存，一条「保存成功」", async () => {
    await mountPage();
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveReport.mockImplementation((v: Record<string, unknown>) => Promise.resolve({ ...v }));
    api.saveProxy.mockImplementation(echoSaved);
    await changeReport();
    await enableProxy();

    saveButton().click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledTimes(1));
    expect(api.saveReport).toHaveBeenCalledTimes(1);
    expect(api.saveProxy).toHaveBeenCalledTimes(1);
    expect(ui.warning).not.toHaveBeenCalled();
  });

  it("战报失败、站点代理成功：照样存站点代理，提示只说战报没保存；再点只重试战报", async () => {
    await mountPage();
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveReport.mockRejectedValue(new Error("磁盘只读"));
    api.saveProxy.mockImplementation(echoSaved);
    await changeReport();
    await enableProxy();

    saveButton().click();
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalledTimes(1));
    const msg = String(ui.warning.mock.calls[0]![0]);
    expect(msg).toContain("全局配置已保存");
    expect(msg).toContain("每日战报设置没有保存：磁盘只读");
    expect(msg).not.toContain("站点代理没有保存");
    expect(api.saveProxy).toHaveBeenCalledTimes(1);
    expect(ui.success).not.toHaveBeenCalled();
    await vi.waitFor(() => expect(head()).toContain("站点代理：全部站点"));

    await vi.waitFor(() => saveButton());
    saveButton().click();
    await vi.waitFor(() => expect(api.saveReport).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalledTimes(2));
    expect(api.saveProxy).toHaveBeenCalledTimes(1);
  });

  it("站点代理失败、战报成功：提示说站点代理没保存和原因；表单留着改动，再点只重试站点代理", async () => {
    await mountPage();
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveReport.mockImplementation((v: Record<string, unknown>) => Promise.resolve({ ...v }));
    api.saveProxy.mockRejectedValue(
      new Error("代理地址要以 http://、https://、socks5:// 或 socks5h:// 开头"),
    );
    await changeReport();
    await enableProxy("ftp://p.test:1");

    saveButton().click();
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalledTimes(1));
    const msg = String(ui.warning.mock.calls[0]![0]);
    expect(msg).toContain("全局配置已保存");
    expect(msg).toContain("站点代理没有保存：代理地址要以");
    expect(msg).not.toContain("每日战报设置没有保存");
    expect(q<HTMLInputElement>("site-proxy-url")!.value).toBe("ftp://p.test:1");
    expect(head()).not.toContain("站点代理");

    await vi.waitFor(() => saveButton());
    saveButton().click();
    await vi.waitFor(() => expect(api.saveProxy).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalledTimes(2));
    expect(api.saveReport).toHaveBeenCalledTimes(1);
  });

  it("两个都失败：一条提示里逐个列出", async () => {
    await mountPage();
    api.saveGlobal.mockResolvedValue(undefined);
    api.saveReport.mockRejectedValue(new Error("磁盘只读"));
    api.saveProxy.mockRejectedValue(new Error(""));
    await changeReport();
    await enableProxy();

    saveButton().click();
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalledTimes(1));
    expect(String(ui.warning.mock.calls[0]![0])).toBe(
      "全局配置已保存，每日战报设置没有保存：磁盘只读；站点代理没有保存：请求失败",
    );
    expect(ui.success).not.toHaveBeenCalled();
  });

  it("全局配置保存失败：报错，战报和站点代理都不试", async () => {
    await mountPage();
    api.saveGlobal.mockRejectedValue(new Error("磁盘只读"));
    await changeReport();
    await enableProxy();

    saveButton().click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("磁盘只读"));
    expect(api.saveReport).not.toHaveBeenCalled();
    expect(api.saveProxy).not.toHaveBeenCalled();
  });
});
