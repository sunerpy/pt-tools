// @vitest-environment happy-dom
/*
 * 钉子：远程访问页（路线图 M15）。设置只发接口认的三项（relay 按行拆开）；状态写明 hostId 与 relay 的连接；
 * 添加设备按选的权限生成二维码，等到配对完成后刷新设备列表，关掉弹窗时还在等的二维码作废；
 * 设备可以改权限、撤销（都要确认），撤销了的可以删记录；读不到时写明原因。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  get: vi.fn(),
  save: vi.fn(),
  rotateKeys: vi.fn(),
  startPairing: vi.fn(),
  pairingStatus: vi.fn(),
  cancelPairing: vi.fn(),
  devices: vi.fn(),
  updateDevice: vi.fn(),
  revokeDevice: vi.fn(),
  deleteDevice: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  confirm: vi.fn(),
  prompt: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, remoteApi: api };
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
    ElMessageBox: { ...real.ElMessageBox, confirm: ui.confirm, prompt: ui.prompt },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
  vi.useRealTimers();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);
const inputOf = (id: string) => {
  const el = q(id)!;
  return (el.matches("input, textarea") ? el : el.querySelector("input, textarea")) as
    | HTMLInputElement
    | HTMLTextAreaElement;
};
const button = (text: string) =>
  [...document.querySelectorAll<HTMLButtonElement>("button")].find((b) =>
    (b.textContent ?? "").includes(text),
  )!;

function overview(over: Record<string, unknown> = {}) {
  return {
    enabled: true,
    relays: ["wss://relay.example.com"],
    direct_url: "http://192.168.1.10:8080",
    host_id: "abcdefghijklmnopqrstuvwxyz",
    host_key: "k",
    relay_status: [
      {
        url: "wss://relay.example.com",
        state: "offline",
        error: "连接 relay 失败",
        since: "",
        streams: 0,
      },
    ],
    sessions: 1,
    stream_path: "/remote/v1/stream",
    ...over,
  };
}

const phone = {
  id: 1,
  name: "我的手机",
  scopes: ["app:read", "app:write"],
  created_at: "2026-10-01T10:00:00Z",
  last_seen_at: "2026-10-08T10:00:00Z",
  last_seen_via: "relay",
  online: true,
  online_via: "direct",
};
const pad = {
  id: 2,
  name: "旧平板",
  scopes: ["app:read"],
  created_at: "2026-09-01T10:00:00Z",
  revoked_at: "2026-10-02T10:00:00Z",
  online: false,
};

async function mountPage(opts: { fail?: boolean; ov?: Record<string, unknown> } = {}) {
  if (opts.fail) api.get.mockRejectedValue(new Error("远程访问没有初始化"));
  else api.get.mockResolvedValue(overview(opts.ov));
  api.devices.mockResolvedValue([phone, pad]);
  const Page = (await import("./RemoteAccess.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.get).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("远程访问", () => {
  it("状态：hostId、在线的连接与 relay 的连接情况", async () => {
    await mountPage();
    const st = q("ra-status")!.textContent ?? "";
    expect(st).toContain("已开启");
    expect(st).toContain("abcdefghijklmnopqrstuvwxyz");
    expect(st).toContain("1 个");
    expect(st).toContain("wss://relay.example.com");
    expect(st).toContain("断开了");
    expect(st).toContain("连接 relay 失败");
  });

  it("设备列表：权限、在线与撤销", async () => {
    await mountPage();
    const table = q("ra-table")!.textContent ?? "";
    expect(table).toContain("我的手机");
    expect(table).toContain("完全控制");
    expect(table).toContain("在线 · 直连");
    expect(table).toContain("旧平板");
    expect(table).toContain("只读");
    expect(table).toContain("已撤销");
    expect(q("ra-delete-2")).not.toBeNull();
    expect(q("ra-revoke-2")).toBeNull();
  });

  it("保存：只发三项，relay 按行拆开、去掉空行", async () => {
    await mountPage();
    api.save.mockResolvedValue(
      overview({ relays: ["wss://a.example.com", "wss://b.example.com"] }),
    );
    const relays = inputOf("ra-relays");
    relays.value = " wss://a.example.com \n\n wss://b.example.com";
    relays.dispatchEvent(new Event("input"));
    await flush();
    q("ra-save")!.click();
    await vi.waitFor(() => expect(api.save).toHaveBeenCalledTimes(1));
    expect(api.save.mock.calls[0]![0]).toEqual({
      enabled: true,
      relays: ["wss://a.example.com", "wss://b.example.com"],
      direct_url: "http://192.168.1.10:8080",
    });
  });

  it("有托管 relay 时可以一键填入", async () => {
    await mountPage({
      ov: { relays: [], relay_status: [], default_relay: "wss://relay.firlab.app" },
    });
    q("ra-default-relay")!.click();
    await flush();
    expect(inputOf("ra-relays").value).toBe("wss://relay.firlab.app");
  });

  it("没打开时不能添加设备，并写明先打开", async () => {
    await mountPage({ ov: { enabled: false } });
    expect(q("ra-devices")!.textContent ?? "").not.toContain("添加设备");
  });

  it("添加设备：按选的权限生成二维码，配对完成后刷新设备列表", async () => {
    await mountPage();
    api.startPairing.mockResolvedValue({
      link: "pttools://pair?v=1&h=x",
      expires_at: new Date(Date.now() + 600_000).toISOString(),
      scopes: ["app:read"],
      host_id: "abcdefghijklmnopqrstuvwxyz",
      qr_svg: '<svg xmlns="http://www.w3.org/2000/svg"></svg>',
    });
    button("添加设备").click();
    await vi.waitFor(() => expect(q("ra-pair-dialog")).not.toBeNull());
    expect(inputOf("ra-pair-direct").value).toBe("http://192.168.1.10:8080");
    const radios = q("ra-perm")!.querySelectorAll<HTMLInputElement>("input[type=radio]");
    radios[1]!.click();
    await flush();
    q("ra-pair-start")!.click();
    await vi.waitFor(() => expect(api.startPairing).toHaveBeenCalledTimes(1));
    expect(api.startPairing.mock.calls[0]![0]).toEqual({
      scopes: ["app:read"],
      direct_url: "http://192.168.1.10:8080",
    });
    await vi.waitFor(() => expect(q("ra-pair-qr")).not.toBeNull());
    expect(q("ra-pair-qr")!.querySelector("img")!.getAttribute("src")).toContain(
      "data:image/svg+xml",
    );
    expect(inputOf("ra-link").value).toBe("pttools://pair?v=1&h=x");
    expect(q("ra-pair-state")!.textContent).toContain("等待扫码");
    expect(q("ra-countdown")!.textContent).toMatch(/^\s*(10:00|9:\d\d) 后过期/);

    api.pairingStatus.mockResolvedValue({
      state: "paired",
      failures: 0,
      device: { id: 3, name: "新手机", scopes: ["app:read"], created_at: "2026-10-09T00:00:00Z" },
    });
    api.devices.mockClear();
    await vi.waitFor(() => expect(q("ra-pair-state")!.textContent).toContain("已配对：新手机"), {
      timeout: 4000,
    });
    await vi.waitFor(() => expect(api.devices).toHaveBeenCalled());
    expect(ui.success).toHaveBeenCalledWith("「新手机」已配对");
    // 用过的二维码淡下去，链接与复制按钮收起来
    expect(q("ra-pair-qr")!.querySelector("img")!.classList.contains("ra-qr--used")).toBe(true);
    expect(q("ra-link")).toBeNull();
    expect(q("ra-copy-link")).toBeNull();
    q("ra-pair-done")!.click();
    await flush();
    expect(api.cancelPairing).not.toHaveBeenCalled();
  });

  it("关掉弹窗时还在等扫码的二维码作废", async () => {
    await mountPage();
    api.startPairing.mockResolvedValue({
      link: "pttools://pair?v=1&h=x",
      expires_at: new Date(Date.now() + 600_000).toISOString(),
      scopes: ["app:read", "app:write"],
      host_id: "h",
      qr_svg: "<svg></svg>",
    });
    api.pairingStatus.mockResolvedValue({ state: "waiting", failures: 0 });
    api.cancelPairing.mockResolvedValue({ state: "closed", failures: 0 });
    button("添加设备").click();
    await vi.waitFor(() => expect(q("ra-pair-start")).not.toBeNull());
    q("ra-pair-start")!.click();
    await vi.waitFor(() => expect(q("ra-pair-qr")).not.toBeNull());
    q("ra-pair-done")!.click();
    await vi.waitFor(() => expect(api.cancelPairing).toHaveBeenCalledTimes(1));
  });

  it("改权限与撤销都要确认；撤销了的可以删记录", async () => {
    await mountPage();
    ui.confirm.mockResolvedValue("confirm");
    api.updateDevice.mockResolvedValue({ ...phone, scopes: ["app:read"] });
    q("ra-scope-1")!.click();
    await vi.waitFor(() =>
      expect(api.updateDevice).toHaveBeenCalledWith(1, { scopes: ["app:read"] }),
    );
    expect(ui.confirm).toHaveBeenCalledTimes(1);

    api.revokeDevice.mockResolvedValue({ ...phone, revoked_at: "2026-10-09T00:00:00Z" });
    q("ra-revoke-1")!.click();
    await vi.waitFor(() => expect(api.revokeDevice).toHaveBeenCalledWith(1));
    expect(ui.confirm).toHaveBeenCalledTimes(2);

    ui.confirm.mockRejectedValue("cancel");
    q("ra-revoke-1")!.click();
    await flush();
    expect(api.revokeDevice).toHaveBeenCalledTimes(1);

    api.deleteDevice.mockResolvedValue({ ok: true });
    q("ra-delete-2")!.click();
    await vi.waitFor(() => expect(api.deleteDevice).toHaveBeenCalledWith(2));
  });

  it("改名", async () => {
    await mountPage();
    ui.prompt.mockResolvedValue({ value: " 客厅平板 " });
    api.updateDevice.mockResolvedValue({ ...phone, name: "客厅平板" });
    q("ra-rename-1")!.click();
    await vi.waitFor(() => expect(api.updateDevice).toHaveBeenCalledWith(1, { name: "客厅平板" }));
  });

  it("轮换主机密钥要确认", async () => {
    await mountPage();
    ui.confirm.mockResolvedValue("confirm");
    api.rotateKeys.mockResolvedValue(overview({ host_id: "zyxwvutsrqponmlkjihgfedcba" }));
    // 轮换以后刷新设备与状态：接口读到的也是新的 hostId
    api.get.mockResolvedValue(overview({ host_id: "zyxwvutsrqponmlkjihgfedcba" }));
    q("ra-rotate")!.click();
    await vi.waitFor(() => expect(api.rotateKeys).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(q("ra-hostid")!.textContent).toBe("zyxwvutsrqponmlkjihgfedcba"));
  });

  it("读不到时写明原因", async () => {
    await mountPage({ fail: true });
    await vi.waitFor(() => expect(q("ra-state")!.textContent).toContain("远程访问没有初始化"));
  });
});
