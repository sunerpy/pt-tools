// @vitest-environment happy-dom
/*
 * 钉子：NAS 监控页（首页 NAS 概览 H2，方案 3.2.5）。本机与各来源一行：类型、名字、地址、「实验性」「明文连接」标签、
 * 读到的主机、状态与原因（连不上写原因与下次重试、登录失败与证书的问题写怎么处理、停用与用不了写为什么）；
 * 有能接入的类型时才有「添加来源」；本机来源的开关与首页显示改了马上保存，回应换进页面；「重新连接」只给能用的来源，
 * 回应的状态换进那一行；删除要确认；读不到时写原因，刷新再取；手机上是行卡。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import type { NasKind, NasSnapshot, NasSource, NasView } from "@/api";
import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  get: vi.fn(),
  getSource: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  test: vi.fn(),
  retry: vi.fn(),
  settings: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  confirm: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, nasApi: api };
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
  vi.unstubAllGlobals();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);
const text = (id: string) => q(id)?.textContent?.replace(/\s+/g, " ").trim() ?? "";

const KINDS: NasKind[] = [
  {
    name: "truenas",
    label: "TrueNAS",
    experimental: false,
    username: "none",
    secret: "required",
    plain: "never",
  },
  {
    name: "glances",
    label: "Glances",
    experimental: false,
    username: "optional",
    secret: "with_username",
    plain: "without_credentials",
  },
  {
    name: "fnos",
    label: "飞牛",
    experimental: true,
    username: "required",
    secret: "required",
    plain: "with_ack",
  },
];

function snap(over: Partial<NasSnapshot["system"]> = {}): NasSnapshot {
  return {
    at: new Date(Date.now() - 5_000).toISOString(),
    system: {
      hostname: "truenas",
      platform: "TrueNAS SCALE",
      version: "25.04.1",
      model: "i7-12700",
      cores: 8,
      uptime_seconds: 3600,
      ...over,
    },
    cpu: { percent: 3, load1: 1, load5: 1, load15: 1 },
    memory: { total: 64 * 1024 ** 3, used: 40 * 1024 ** 3, available: 24 * 1024 ** 3 },
    network: { rx_bps: 0, tx_bps: 0, interfaces: [] },
    pools: [],
    disks: [],
    alerts: [],
    apps: { running: 0, total: 0, upgradable: 0 },
    facets: null,
  };
}

function source(id: string, over: Partial<NasSource> = {}): NasSource {
  return {
    id,
    name: id === "local" ? "本机" : `NAS ${id}`,
    kind: id === "local" ? "local" : "truenas",
    enabled: true,
    experimental: false,
    url: id === "local" ? undefined : `https://192.168.1.${id}`,
    has_secret: id !== "local",
    insecure_ack: false,
    state: "ok",
    snapshot: snap(),
    history: [],
    ...over,
  };
}

function nasView(over: Partial<NasView> = {}): NasView {
  return {
    home_source: "",
    home: "2",
    local_enabled: true,
    local_supported: true,
    kinds: KINDS,
    sources: [
      source("local", {
        snapshot: snap({ hostname: "pt-host", platform: "Linux", version: "6.12" }),
      }),
      source("2"),
      source("3", {
        name: "客厅的飞牛",
        kind: "fnos",
        experimental: true,
        url: "http://192.168.1.3:5666",
        insecure_ack: true,
        state: "error",
        message: "dial tcp 192.168.1.3:5666: connection refused",
        last_ok_at: "2026-11-06T08:00:00Z",
        next_try_at: new Date(Date.now() + 25_000).toISOString(),
        snapshot: null,
      }),
      source("4", { state: "auth_failed", message: "API Key 不对", snapshot: null }),
      source("5", { state: "cert_untrusted", snapshot: null }),
      source("6", { enabled: false, state: "disabled", snapshot: null }),
    ],
    ...over,
  };
}

async function mountPage(got: NasView | Error = nasView()) {
  if (got instanceof Error) api.get.mockRejectedValue(got);
  else api.get.mockResolvedValue(got);
  const Page = (await import("./NasMonitor.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.get).toHaveBeenCalled());
  await flush();
  await flush();
}

/** 打开下拉框 id，点文字是 label 的选项 */
async function pick(id: string, label: string) {
  q(id)!.click();
  await flush();
  const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
    (el) => el.textContent?.trim() === label,
  );
  if (!opt) throw new Error(`下拉框 ${id} 里没有「${label}」`);
  opt.click();
  await flush();
}

describe("NAS 监控页", () => {
  it("每个来源一行：类型、名字、地址、标签、读到的主机", async () => {
    await mountPage();
    const local = text("nm-row-local");
    expect(local).toContain("本机");
    expect(local).toContain("pt-tools 所在的主机");
    const nas2 = text("nm-row-2");
    expect(nas2).toContain("NAS 2");
    expect(nas2).toContain("TrueNAS · https://192.168.1.2");
    expect(nas2, "https 没有明文标签").not.toContain("明文连接");
    const fn = text("nm-row-3");
    expect(fn).toContain("飞牛 · http://192.168.1.3:5666");
    expect(fn).toContain("实验性");
    expect(fn).toContain("明文连接");
    const table = text("nm-table");
    expect(table).toContain("truenas · i7-12700 · 8 核 · 64.0 GB · TrueNAS SCALE 25.04.1");
    expect(table).toContain("pt-host");
    expect(table).toContain("还没有读到数据");
    expect(text("nm-sources")).toContain("6");
  });

  it("状态下面写原因：连不上写原文与下次重试；登录失败、证书、停用各写怎么处理", async () => {
    await mountPage();
    expect(text("nm-detail-2")).toBe("刚刚更新");
    const err = text("nm-detail-3");
    expect(err).toContain("dial tcp 192.168.1.3:5666: connection refused");
    expect(err).toMatch(/\d+ 秒后再试/);
    expect(q("nm-detail-3")!.classList.contains("is-dang")).toBe(true);
    expect(text("nm-table")).toContain("上次读到数据：");
    expect(text("nm-detail-4")).toBe("API Key 不对。改了设置或点「重新连接」以后才会再试");
    expect(text("nm-detail-5")).toContain("编辑这个来源，测试连接并确认证书");
    expect(text("nm-detail-6")).toBe("停用了：编辑里打开「启用」以后开始读");
  });

  it("「重新连接」只给能用的来源；回应的状态换进那一行", async () => {
    await mountPage();
    expect(q("nm-retry-6"), "停用的没有").toBeNull();
    expect(q("nm-retry-local")).not.toBeNull();
    expect(q("nm-edit-local"), "本机不能编辑、删除").toBeNull();
    expect(q("nm-delete-local")).toBeNull();
    api.retry.mockResolvedValueOnce(source("4"));
    q("nm-retry-4")!.click();
    await vi.waitFor(() => expect(api.retry).toHaveBeenCalledWith("4"));
    await flush();
    expect(text("nm-detail-4")).toBe("刚刚更新");
    expect(ui.success).toHaveBeenCalledWith("NAS 4：连上了");

    api.retry.mockResolvedValueOnce(source("3", { state: "error", message: "还是连不上" }));
    q("nm-retry-3")!.click();
    await vi.waitFor(() =>
      expect(ui.warning).toHaveBeenCalledWith("客厅的飞牛：连不上：还是连不上"),
    );

    api.retry.mockRejectedValueOnce(new Error("30 秒内没有连完，稍后看列表里的状态\n"));
    q("nm-retry-2")!.click();
    await vi.waitFor(() =>
      expect(ui.error).toHaveBeenCalledWith("30 秒内没有连完，稍后看列表里的状态"),
    );
  });

  it("本机来源的开关：改了马上保存，回应换进页面；这个平台用不了时开关是灰的", async () => {
    await mountPage();
    api.settings.mockResolvedValueOnce(
      nasView({
        local_enabled: false,
        sources: [source("local", { enabled: false, state: "disabled", snapshot: null })],
        home: "",
      }),
    );
    q("nm-local-switch")!.click();
    await vi.waitFor(() => expect(api.settings).toHaveBeenCalledWith({ local_enabled: false }));
    await flush();
    expect(ui.success).toHaveBeenCalledWith("本机来源关掉了");
    expect(text("nm-detail-local")).toContain("关着：打开以后读 pt-tools 所在主机");
    expect(text("nm-home-now")).toBe("现在没有能显示的来源，首页的 NAS 概览是空的。");
    view?.unmount();

    await mountPage(
      nasView({
        local_supported: false,
        local_enabled: true,
        sources: [source("local", { state: "unsupported", snapshot: null })],
      }),
    );
    expect(q("nm-local-switch")!.classList.contains("is-disabled")).toBe(true);
    expect(text("nm-detail-local")).toBe("本机来源只支持 Linux");
    expect(q("nm-retry-local"), "用不了的没有重新连接").toBeNull();
  });

  it("首页显示：选了马上保存；写明现在显示的是哪个，选的停用了时写退回了哪个", async () => {
    await mountPage();
    expect(text("nm-home-now")).toBe("自动：显示第一个能用的来源，没有时用本机；现在显示 NAS 2。");
    // 「自动」的值是空串：下拉框要显示这一项，不能当成没选、只显示「请选择」
    expect(text("nm-home-select")).toContain("自动");
    expect(text("nm-home-select")).not.toContain("Select");
    api.settings.mockResolvedValueOnce(nasView({ home_source: "local", home: "local" }));
    await pick("nm-home-select", "本机");
    expect(api.settings).toHaveBeenCalledWith({ home_source: "local" });
    expect(text("nm-home-now")).toBe("首页现在显示 本机。");
    view?.unmount();

    await mountPage(nasView({ home_source: "6", home: "2" }));
    expect(text("nm-home-now")).toBe("选的来源停用了或这个版本用不了，首页现在显示 NAS 2。");
    api.settings.mockRejectedValueOnce(new Error("没有这个来源\n"));
    await pick("nm-home-select", "NAS 2（TrueNAS）");
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("没有这个来源"));
  });

  it("有能接入的类型时才有「添加来源」，点开是添加来源的弹窗；编辑打开那一个", async () => {
    await mountPage(nasView({ kinds: [] }));
    expect(text("nm-sources")).not.toContain("添加来源");
    expect(text("nm-kinds")).toBe("这个版本还不能接入 NAS，只有本机来源。");
    view?.unmount();

    await mountPage(nasView({ kinds: [KINDS[0]!] }));
    expect(text("nm-kinds"), "没有实验性的类型时不写实验性的说明").toBe("能接入：TrueNAS。");
    view?.unmount();

    await mountPage();
    expect(text("nm-kinds")).toBe(
      "能接入：TrueNAS、Glances、飞牛（实验性）。实验性的还没在真机上验证过，读到的数据可能不全。",
    );
    const add = [...q("nm-sources")!.querySelectorAll("button")].find((b) =>
      b.textContent?.includes("添加来源"),
    )!;
    add.click();
    await flush();
    expect(document.querySelector(".el-dialog__title")?.textContent).toBe("添加来源");

    api.getSource.mockResolvedValueOnce({
      id: 2,
      name: "NAS 2",
      kind: "truenas",
      enabled: true,
      url: "https://192.168.1.2",
      username: "",
      has_secret: true,
      has_extra: false,
      tls_pin_sha256: "",
      insecure_ack: false,
    });
    q("nm-edit-2")!.click();
    await vi.waitFor(() => expect(api.getSource).toHaveBeenCalledWith(2));
  });

  it("删除要确认：确认了才删，删完重取；取消了不删", async () => {
    await mountPage();
    ui.confirm.mockRejectedValueOnce("cancel");
    q("nm-delete-2")!.click();
    await flush();
    expect(api.remove).not.toHaveBeenCalled();

    ui.confirm.mockResolvedValueOnce("confirm");
    api.remove.mockResolvedValueOnce({ ok: true });
    q("nm-delete-2")!.click();
    await vi.waitFor(() => expect(api.remove).toHaveBeenCalledWith(2));
    expect(ui.confirm.mock.calls[1]![0]).toContain("保存的密钥与登录凭据一起删掉");
    await vi.waitFor(() => expect(api.get).toHaveBeenCalledTimes(2));
    expect(ui.success).toHaveBeenCalledWith("已删除");

    ui.confirm.mockResolvedValueOnce("confirm");
    api.remove.mockRejectedValueOnce(new Error("没有这个来源\n"));
    q("nm-delete-3")!.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("没有这个来源"));
  });

  it("读不到时写原因；刷新再取，再失败时提示", async () => {
    await mountPage(new Error("NAS 监控没有启动\n"));
    expect(text("nm-state")).toContain("NAS 监控没有启动");
    api.get.mockResolvedValueOnce(nasView());
    q("nm-refresh")!.click();
    await vi.waitFor(() => expect(q("nm-table")).not.toBeNull());

    api.get.mockRejectedValueOnce(new Error("读取 NAS 监控设置失败\n"));
    q("nm-refresh")!.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("读取 NAS 监控设置失败"));
    expect(q("nm-table"), "留着上一次的").not.toBeNull();
  });

  it("连上了但读不到主机信息（账号没有权限）：「读到的主机」写原因，手机的行卡也写", async () => {
    const denied = "这个账号没有读主机信息的权限：DSM 只让管理员组（administrators）的账号读";
    const empty: NasSnapshot = {
      ...snap({ hostname: "", platform: "", version: "", model: "", cores: 0, uptime_seconds: 0 }),
      memory: { total: 0, used: 0, available: 0 },
      facets: {
        system: { status: "denied", reason: denied },
        memory: { status: "denied", reason: denied },
      },
    };
    const got = nasView({
      sources: [
        source("7", { name: "群晖", snapshot: empty }),
        source("8", { state: "connecting", snapshot: null }),
      ],
    });
    await mountPage(got);
    expect(text("nm-host-7")).toBe(denied);
    expect(text("nm-host-8"), "还没读到时照旧").toBe("还没有读到数据");
    expect(text("nm-detail-7")).toBe("刚刚更新");

    view?.unmount();
    view = null;
    vi.stubGlobal(
      "matchMedia",
      vi.fn((query: string) => ({
        matches: query.includes("max-width: 768px"),
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    );
    await mountPage(got);
    expect(q("nm-table")).toBeNull();
    expect(text("nm-host-7")).toBe(denied);
    expect(q("nm-host-8"), "行卡上还没读到时不多写一行（状态那行写着正在连）").toBeNull();
  });

  it("手机：一个来源一张行卡", async () => {
    vi.stubGlobal(
      "matchMedia",
      vi.fn((query: string) => ({
        matches: query.includes("max-width: 768px"),
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    );
    await mountPage();
    expect(q("nm-table")).toBeNull();
    expect(document.querySelectorAll("[data-testid^=nm-card-]").length).toBe(6);
    const card = text("nm-card-3");
    expect(card).toContain("客厅的飞牛");
    expect(card).toContain("连不上");
    expect(card).toContain("明文连接");
    expect(text("nm-card-local")).toContain("pt-tools 所在的主机");
  });
});
