// @vitest-environment happy-dom
/*
 * 钉子：MCP 接入页（路线图 M14）。地址取浏览器里的 origin 加设置里的路径（默认 /mcp）；三种客户端的填法都带上这个地址；
 * 工具清单，写工具标「MCP 操作」；复制按钮把地址交给剪贴板；开关与路径只发接口认的字段，改了以后地址跟着变。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const ui = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn() }));
const api = vi.hoisted(() => ({ settings: vi.fn(), save: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, mcpApi: api };
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

const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);

const flush = () => new Promise((r) => setTimeout(r, 0));

async function mountPage(settings: Record<string, unknown> | Error = {}) {
  if (settings instanceof Error) api.settings.mockRejectedValue(settings);
  else
    api.settings.mockResolvedValue({
      enabled: true,
      path: "/mcp",
      default_path: "/mcp",
      tokens: 2,
      ...settings,
    });
  const Page = (await import("./McpAccess.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: Page },
      { path: "/api-tokens", component: { template: "<div />" } },
    ],
  });
  await router.push("/");
  view = mountView(Page, { router });
  await vi.waitFor(() => expect(api.settings).toHaveBeenCalled());
  await flush();
  await flush();
}

function fill(id: string, v: string) {
  const el = q(id);
  if (!el) throw new Error(`找不到 ${id}`);
  const i = (el instanceof HTMLInputElement ? el : el.querySelector("input"))!;
  i.value = v;
  i.dispatchEvent(new Event("input"));
}

describe("MCP 接入", () => {
  it("地址是这台 pt-tools 的 /mcp，三种客户端的填法都用它", async () => {
    await mountPage();
    const endpoint = `${window.location.origin}/mcp`;
    expect(q("mcp-url")!.textContent).toBe(endpoint);
    expect(q("mcp-snippet-http")!.textContent).toContain(endpoint);
    expect(q("mcp-snippet-http")!.textContent).toContain("Authorization");
    expect(q("mcp-snippet-claude-code")!.textContent).toContain(
      `--transport http pt-tools ${endpoint}`,
    );
    expect(q("mcp-snippet-stdio")!.textContent).toContain("PT_TOOLS_MCP_TOKEN");
    expect(q("mcp-tokens")!.getAttribute("href")).toContain("api-tokens");
  });

  it("工具清单：37 个（含订阅与追剧、媒体库与配置），写工具标 MCP 操作", async () => {
    await mountPage();
    const rows = document.querySelectorAll("[data-testid^=mcp-tool-]");
    expect(rows).toHaveLength(37);
    expect(q("mcp-tool-update_subscription")!.textContent).toContain("MCP 操作");
    expect(q("mcp-tool-preview_schedule")!.textContent).toContain("MCP 读取");
    expect(q("mcp-tool-reorganize_subscription_torrent")!.textContent).toContain("MCP 操作");
    expect(q("mcp-tool-change_config")!.textContent).toContain("MCP 操作");
    expect(q("mcp-tool-get_config")!.textContent).toContain("MCP 读取");
    expect(q("mcp-tool-delete_torrent")!.textContent).toContain("MCP 操作");
    expect(q("mcp-tool-search_torrents")!.textContent).toContain("MCP 读取");
    expect(q("mcp-tool-organize_media")!.textContent).toContain("MCP 操作");
    expect(q("mcp-tool-recognize_media")!.textContent).toContain("MCP 读取");
  });

  it("复制地址；浏览器不让复制时提示手动复制", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    await mountPage();
    q("mcp-copy-url")!.click();
    await vi.waitFor(() => expect(writeText).toHaveBeenCalledWith(`${window.location.origin}/mcp`));
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalled());

    writeText.mockRejectedValue(new Error("denied"));
    q("mcp-copy-url")!.click();
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalled());
  });

  it("开关与路径：只发接口认的字段，保存后地址与填法跟着变", async () => {
    await mountPage();
    expect(q("mcp-state")!.textContent).toContain("开着");
    expect(q("mcp-token-count")!.textContent).toContain("2 个");
    const save = q("mcp-save") as HTMLButtonElement;
    expect(save.disabled, "没改动时不能保存").toBe(true);

    fill("mcp-path", " /agent/mcp ");
    await flush();
    expect(save.disabled).toBe(false);
    api.save.mockResolvedValue({
      enabled: true,
      path: "/agent/mcp",
      default_path: "/mcp",
      tokens: 2,
    });
    save.click();
    await vi.waitFor(() =>
      expect(api.save).toHaveBeenCalledWith({ enabled: true, path: "/agent/mcp" }),
    );
    const endpoint = `${window.location.origin}/agent/mcp`;
    await vi.waitFor(() => expect(q("mcp-url")!.textContent).toBe(endpoint));
    expect(ui.success).toHaveBeenCalledWith(`已打开 MCP 接入，地址 ${endpoint}`);
    expect(q("mcp-snippet-claude-code")!.textContent).toContain(endpoint);
    expect(q("mcp-snippet-stdio")!.textContent, "换了路径时转发给完整地址").toContain(endpoint);

    // 关掉
    (q("mcp-enabled")!.querySelector("input") ?? q("mcp-enabled"))!.click();
    await flush();
    api.save.mockResolvedValue({
      enabled: false,
      path: "/agent/mcp",
      default_path: "/mcp",
      tokens: 2,
    });
    (q("mcp-save") as HTMLButtonElement).click();
    await vi.waitFor(() =>
      expect(api.save).toHaveBeenLastCalledWith({ enabled: false, path: "/agent/mcp" }),
    );
    await vi.waitFor(() => expect(q("mcp-state")!.textContent).toContain("关着"));
    expect(ui.success).toHaveBeenLastCalledWith("已关闭 MCP 接入");

    // 保存失败写明原因
    fill("mcp-path", "/api/x");
    await flush();
    api.save.mockRejectedValue(new Error("/api 下面是 pt-tools 自己用的地址，换一个路径"));
    (q("mcp-save") as HTMLButtonElement).click();
    await vi.waitFor(() =>
      expect(ui.error).toHaveBeenCalledWith("/api 下面是 pt-tools 自己用的地址，换一个路径"),
    );
  });

  it("默认路径时 stdio 转发只给主机与端口；没有能用的令牌时给出新建的入口；读不到设置时照默认地址写", async () => {
    await mountPage({ tokens: 0 });
    expect(q("mcp-snippet-stdio")!.textContent).toContain(`"${window.location.origin}"`);
    expect(q("mcp-new-token")!.getAttribute("href")).toContain("api-tokens?new=mcp");
    view?.unmount();
    view = null;
    document.body.innerHTML = "";
    await mountPage(new Error("读取失败"));
    expect(q("mcp-settings-state")).not.toBeNull();
    expect(q("mcp-url")!.textContent).toBe(`${window.location.origin}/mcp`);
    expect(q("mcp-save")).toBeNull();
  });
});
