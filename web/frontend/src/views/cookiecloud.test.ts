// @vitest-environment happy-dom
/*
 * 钉子：CookieCloud 导入页。没填全时不能读取；保存时只在填了新密码时才带上它，清除要确认并关闭定时同步；
 * 预览只列 Cookie 的名字与站点情况；导入选中的站点前要确认，结果写明更新了哪些；读取失败时留着上次的列表并写明。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  settings: vi.fn(),
  saveSettings: vi.fn(),
  preview: vi.fn(),
  import: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  message: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  confirm: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, cookieCloudApi: api };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(ui.message, {
      success: ui.success,
      error: ui.error,
      warning: vi.fn(),
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
const testid = (id: string) => {
  const el = document.querySelector<HTMLElement>(`[data-testid=${id}]`);
  if (!el) throw new Error(`找不到 ${id}`);
  return el;
};

function settings(extra: Record<string, unknown> = {}) {
  return {
    server_url: "",
    uuid: "",
    has_password: false,
    auto_sync: false,
    interval_hours: 24,
    last_result: "",
    ...extra,
  };
}

const configured = settings({ server_url: "https://cc.example", uuid: "u1", has_password: true });

const previewData = {
  domains: 5,
  items: [
    {
      site: "hdsky",
      site_name: "HDSky",
      host: "hdsky.me",
      cookie_names: ["c_secure_pass", "c_secure_uid"],
      enabled: true,
      changed: true,
    },
    {
      site: "ourbits",
      site_name: "OurBits",
      host: "ourbits.club",
      cookie_names: ["ob"],
      enabled: false,
      changed: true,
    },
    {
      site: "audiences",
      site_name: "Audiences",
      host: "audiences.me",
      cookie_names: ["a"],
      enabled: true,
      changed: false,
    },
  ],
};

async function mountPage(s = settings()) {
  api.settings.mockResolvedValue(s);
  const Page = (await import("./CookieCloud.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.settings).toHaveBeenCalled());
  await flush();
  await flush();
}

function setInput(id: string, v: string) {
  const el = testid(id);
  const input = (
    el instanceof HTMLInputElement ? el : el.querySelector("input")
  ) as HTMLInputElement;
  input.value = v;
  input.dispatchEvent(new Event("input"));
}

describe("CookieCloud 导入", () => {
  it("没填全时不能读取；保存时带上新填的密码", async () => {
    await mountPage();
    expect((testid("cc-preview") as HTMLButtonElement).disabled).toBe(true);
    expect(document.body.textContent).toContain("密码不发给 CookieCloud");
    expect(document.body.textContent).toContain("先在上面填写服务地址");

    setInput("cc-server", " https://cc.example ");
    setInput("cc-uuid", "u1");
    setInput("cc-password", "pw");
    await flush();
    api.saveSettings.mockResolvedValue(configured);
    testid("cc-save").click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalled());
    expect(api.saveSettings.mock.calls[0][0]).toMatchObject({
      server_url: "https://cc.example",
      uuid: "u1",
      password: "pw",
      auto_sync: false,
    });
    await flush();
    expect((testid("cc-preview") as HTMLButtonElement).disabled).toBe(false);
  });

  it("已有密码：不填就不带密码；清除要确认并关闭定时同步", async () => {
    await mountPage(settings({ ...configured, auto_sync: true }));
    api.saveSettings.mockResolvedValue(configured);
    testid("cc-save").click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalled());
    expect(api.saveSettings.mock.calls[0][0]).not.toHaveProperty("password");

    ui.confirm.mockResolvedValue("confirm");
    api.saveSettings.mockResolvedValue(settings({ server_url: "https://cc.example", uuid: "u1" }));
    testid("cc-clear-password").click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(2));
    expect(api.saveSettings.mock.calls[1][0]).toMatchObject({ password: "", auto_sync: false });
  });

  it("预览列出站点与 Cookie 名；导入前确认，结果写明更新了哪些", async () => {
    await mountPage(configured);
    api.preview.mockResolvedValue(previewData);
    testid("cc-preview").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("hdsky.me"));
    const text = document.body.textContent ?? "";
    expect(text).toContain("c_secure_pass、c_secure_uid");
    expect(text).toContain("Cookie 有变化");
    expect(text).toContain("没有启用");
    expect(text).toContain("没有变化");
    expect(testid("cc-summary").textContent).toContain("5 个网站");
    expect((testid("cc-import") as HTMLButtonElement).disabled).toBe(true);

    testid("cc-select-changed").click();
    await vi.waitFor(() => expect(testid("cc-import").textContent).toContain("导入选中 1 个"));

    ui.confirm.mockResolvedValue("confirm");
    api.import.mockResolvedValue({ imported: ["hdsky"], unchanged: [], missing: [], failed: [] });
    testid("cc-import").click();
    await vi.waitFor(() => expect(api.import).toHaveBeenCalledWith(["hdsky"]));
    expect(ui.confirm.mock.calls[0][0]).toContain("选中的 1 个站点");
    await vi.waitFor(() =>
      expect(ui.message).toHaveBeenCalledWith(
        expect.objectContaining({ type: "success", message: "更新了 1 个站点的 Cookie（hdsky）" }),
      ),
    );
    expect(api.preview).toHaveBeenCalledTimes(2);
  });

  it("读取失败时留着上次的列表，并写明是旧的", async () => {
    await mountPage(configured);
    api.preview.mockResolvedValue(previewData);
    testid("cc-preview").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("hdsky.me"));
    api.preview.mockRejectedValue(new Error("解密失败：请检查 UUID 和密码"));
    testid("cc-preview").click();
    await vi.waitFor(() => expect(testid("cc-stale").textContent).toContain("解密失败"));
    expect(document.body.textContent).toContain("hdsky.me");
  });
});
