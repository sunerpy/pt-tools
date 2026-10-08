// @vitest-environment happy-dom
/*
 * 钉子：MCP 接入页（路线图 M14）。地址取浏览器里的 origin 加 /mcp；三种客户端的填法都带上这个地址；
 * 工具清单 13 个，写工具标「MCP 操作」；复制按钮把地址交给剪贴板。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const ui = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn() }));

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

async function mountPage() {
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
  await vi.waitFor(() => expect(q("mcp-url")).not.toBeNull());
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

  it("工具清单：13 个，写工具标 MCP 操作", async () => {
    await mountPage();
    const rows = document.querySelectorAll("[data-testid^=mcp-tool-]");
    expect(rows).toHaveLength(13);
    expect(q("mcp-tool-delete_torrent")!.textContent).toContain("MCP 操作");
    expect(q("mcp-tool-search_torrents")!.textContent).toContain("MCP 读取");
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
});
