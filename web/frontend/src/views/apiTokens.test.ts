// @vitest-environment happy-dom
/*
 * 钉子：API 令牌页。列表写明权限、最近一次使用与过期；新建只发接口认的字段，明文只在新建后的弹窗里出现一次、
 * 关掉以后清空；撤销要确认；读不到时写明原因。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), revoke: vi.fn() }));
const mcp = vi.hoisted(() => ({
  settings: vi.fn(async () => ({ enabled: true, path: "/mcp", default_path: "/mcp", tokens: 0 })),
  save: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  confirm: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, tokensApi: api, mcpApi: mcp };
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

const phone = {
  id: 1,
  name: "我的手机",
  scopes: ["app:read", "app:write"],
  expires_at: "2099-01-01T00:00:00Z",
  last_used_at: "2026-10-08T10:00:00Z",
  created_by: "admin",
  created_at: "2026-10-01T10:00:00Z",
};
const old = {
  id: 2,
  name: "旧平板",
  scopes: ["app:read"],
  expires_at: "2020-01-01T00:00:00Z",
  created_by: "admin",
  created_at: "2019-10-01T10:00:00Z",
};

async function mountPage(fail = false) {
  if (fail) api.list.mockRejectedValue(new Error("读取令牌失败"));
  else api.list.mockResolvedValue([phone, old]);
  const Page = (await import("./ApiTokens.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.list).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("API 令牌", () => {
  it("列表：权限、最近一次使用与过期", async () => {
    await mountPage();
    const table = q("tk-table")!.textContent ?? "";
    expect(table).toContain("我的手机");
    expect(table).toContain("读取");
    expect(table).toContain("操作");
    expect(table).toContain("还没用过");
    expect(table).toContain("已过期");
    expect(table).not.toContain("ptt_");
  });

  it("新建：只发接口认的字段；明文只在弹窗里出现一次，关掉以后清空", async () => {
    await mountPage();
    api.create.mockResolvedValue({ token: { ...phone, id: 3 }, plaintext: "ptt_3_secretvalue" });
    const add = [...document.querySelectorAll<HTMLButtonElement>("button")].find((b) =>
      (b.textContent ?? "").includes("新建"),
    )!;
    add.click();
    await vi.waitFor(() => expect(q("token-dialog")).not.toBeNull());
    const name = q("tk-name")!;
    const input = (name instanceof HTMLInputElement ? name : name.querySelector("input"))!;
    input.value = " 我的手机 ";
    input.dispatchEvent(new Event("input"));
    await flush();
    q("tk-save")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toEqual({
      name: "我的手机",
      scopes: ["app:read"],
      expires_in_days: 90,
    });
    await vi.waitFor(() => expect(q("token-plain")).not.toBeNull());
    const plain = q("tk-plain")!;
    const plainInput = (plain instanceof HTMLInputElement ? plain : plain.querySelector("input"))!;
    expect(plainInput.value).toBe("ptt_3_secretvalue");

    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    q("tk-copy")!.click();
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith("ptt_3_secretvalue"));
    q("tk-done")!.click();
    await flush();
    await vi.waitFor(() =>
      expect(document.body.textContent ?? "").not.toContain("ptt_3_secretvalue"),
    );
  });

  it("从 qB 兼容入口页点过来：直接打开新建，权限只选 qB 兼容", async () => {
    api.list.mockResolvedValue([]);
    const { createMemoryHistory, createRouter } = await import("vue-router");
    const Page = (await import("./ApiTokens.vue")).default;
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/api-tokens", component: Page }],
    });
    await router.push("/api-tokens?new=qbit");
    view = mountView(Page, { router });
    await vi.waitFor(() => expect(q("token-dialog")).not.toBeNull());
    api.create.mockResolvedValue({ token: { ...phone, id: 4 }, plaintext: "ptt_4_x" });
    q("tk-save")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toEqual({
      name: "qB 兼容入口",
      scopes: ["qbit:compat"],
      expires_in_days: 90,
    });
    await vi.waitFor(() => expect(router.currentRoute.value.query.new).toBeUndefined());
  });

  it("从 MCP 接入页点过来：选好 MCP 读取与操作；MCP 接入关着时提醒去打开", async () => {
    api.list.mockResolvedValue([]);
    mcp.settings.mockResolvedValueOnce({
      enabled: false,
      path: "/mcp",
      default_path: "/mcp",
      tokens: 0,
    });
    const { createMemoryHistory, createRouter } = await import("vue-router");
    const Page = (await import("./ApiTokens.vue")).default;
    const router = createRouter({
      history: createMemoryHistory(),
      routes: [
        { path: "/api-tokens", component: Page },
        { path: "/mcp", component: { template: "<div />" } },
      ],
    });
    await router.push("/api-tokens?new=mcp");
    view = mountView(Page, { router });
    await vi.waitFor(() => expect(q("token-dialog")).not.toBeNull());
    await vi.waitFor(() => expect(q("tk-mcp-off")).not.toBeNull());
    expect(q("tk-mcp-off")!.textContent).toContain("MCP 接入现在关着");
    api.create.mockResolvedValue({ token: { ...phone, id: 5 }, plaintext: "ptt_5_x" });
    q("tk-save")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toEqual({
      name: "AI 客户端（MCP）",
      scopes: ["mcp:read", "mcp:write"],
      expires_in_days: 90,
    });
  });

  it("MCP 接入开着时不提醒；没选 MCP 权限时也不提醒", async () => {
    await mountPage();
    const add = [...document.querySelectorAll<HTMLButtonElement>("button")].find((b) =>
      (b.textContent ?? "").includes("新建"),
    )!;
    add.click();
    await vi.waitFor(() => expect(q("token-dialog")).not.toBeNull());
    expect(q("tk-mcp-off")).toBeNull();
  });

  it("新建：名字没填时不发", async () => {
    await mountPage();
    const add = [...document.querySelectorAll<HTMLButtonElement>("button")].find((b) =>
      (b.textContent ?? "").includes("新建"),
    )!;
    add.click();
    await vi.waitFor(() => expect(q("tk-save")).not.toBeNull());
    q("tk-save")!.click();
    await flush();
    expect(api.create).not.toHaveBeenCalled();
    expect(ui.warning).toHaveBeenCalledWith("名字要填");
  });

  it("撤销要确认", async () => {
    await mountPage();
    ui.confirm.mockResolvedValue("confirm");
    api.revoke.mockResolvedValue({ ok: true });
    q("tk-revoke-1")!.click();
    await vi.waitFor(() => expect(api.revoke).toHaveBeenCalledWith(1));
    expect(ui.confirm.mock.calls[0]![0]).toContain("马上就连不上了");
    ui.confirm.mockRejectedValue("cancel");
    q("tk-revoke-2")!.click();
    await flush();
    expect(api.revoke).toHaveBeenCalledTimes(1);
  });

  it("读不到时写明原因", async () => {
    await mountPage(true);
    expect(q("tk-state")!.textContent).toContain("读取令牌失败");
  });
});
