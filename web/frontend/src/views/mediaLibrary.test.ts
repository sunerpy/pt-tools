// @vitest-environment happy-dom
/*
 * 钉子：媒体库页。整理设置只发接口认的字段（保存路径按行拆开，不带 auto_since）；媒体库、路径映射、媒体服务器的
 * 增删改只发输入字段（严格解析）；编辑媒体服务器时不填 Token 就不带 Token；列表读不到时留在页面上。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  settings: vi.fn(),
  saveSettings: vi.fn(),
  libraries: vi.fn(),
  createLibrary: vi.fn(),
  updateLibrary: vi.fn(),
  deleteLibrary: vi.fn(),
  checkLibrary: vi.fn(),
  templatePreview: vi.fn(),
  pathMaps: vi.fn(),
  createPathMap: vi.fn(),
  updatePathMap: vi.fn(),
  deletePathMap: vi.fn(),
  servers: vi.fn(),
  createServer: vi.fn(),
  updateServer: vi.fn(),
  deleteServer: vi.fn(),
  testServer: vi.fn(),
}));
const dl = vi.hoisted(() => ({ list: vi.fn() }));
const notifications = vi.hoisted(() => ({ list: vi.fn() }));
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
    organizeApi: api,
    downloadersApi: dl,
    chatopsApi: { ...real.chatopsApi, notifications },
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
function inputOf(id: string): HTMLInputElement | HTMLTextAreaElement {
  const el = q(id);
  if (!el) throw new Error(`找不到 ${id}`);
  if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) return el;
  return (el.querySelector("input") ?? el.querySelector("textarea")) as HTMLInputElement;
}
function fill(id: string, v: string) {
  const i = inputOf(id);
  i.value = v;
  i.dispatchEvent(new Event("input"));
}

const settings = {
  auto_enabled: true,
  auto_since: "2026-10-07T12:00:00Z",
  scan_enabled: false,
  scan_interval_min: 60,
  downloaders: [],
  categories: ["movies"],
  tags: [],
  save_paths: ["/downloads"],
  min_video_mb: 50,
  notify_channels: [],
  delete_links_on_remove: false,
};

const library = {
  id: 3,
  name: "电影",
  kind: "movie",
  anime: false,
  path: "/media/movies",
  template: "",
  mode: "hardlink",
  scrape: true,
  scrape_overwrite: false,
  enabled: true,
  created_at: "",
  updated_at: "",
  effective_template: "{{.Title}}",
  preview: "奥本海默 (2023)/奥本海默 (2023) - 2160p BluRay HDR10 H.265",
};

const server = {
  id: 5,
  name: "Emby",
  kind: "emby",
  url: "http://192.168.1.10:8096",
  enabled: true,
  refresh_mode: "path",
  local_prefix: "/media",
  server_prefix: "/data",
  has_token: true,
  last_error: "",
  last_refresh_at: "2026-10-07T12:00:00Z",
  created_at: "",
  updated_at: "",
};

async function mountPage(opts: { libsFail?: boolean } = {}) {
  api.settings.mockResolvedValue(settings);
  if (opts.libsFail) api.libraries.mockRejectedValue(new Error("读取媒体库失败"));
  else api.libraries.mockResolvedValue([library]);
  api.pathMaps.mockResolvedValue([
    {
      id: 7,
      downloader_id: 1,
      downloader_prefix: "/downloads",
      local_prefix: "/data/downloads",
      created_at: "",
      updated_at: "",
    },
  ]);
  api.servers.mockResolvedValue([server]);
  api.templatePreview.mockResolvedValue({ template: "", preview: "示例路径" });
  dl.list.mockResolvedValue([
    {
      id: 1,
      name: "qb",
      type: "qbittorrent",
      url: "",
      username: "",
      is_default: true,
      enabled: true,
    },
  ]);
  notifications.list.mockResolvedValue([
    { id: 2, channel_type: "telegram", name: "TG", enabled: true },
  ]);
  const Page = (await import("./MediaLibrary.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.settings).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("媒体库", () => {
  it("整理设置：只发输入字段，保存路径按行拆开", async () => {
    await mountPage();
    expect(document.body.textContent).toContain("以后下载完成的种子");
    api.saveSettings.mockResolvedValue(settings);
    fill("ml-scope-path", " /downloads \n\n/data/tv ");
    q("ml-save-settings")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(1));
    expect(api.saveSettings.mock.calls[0]![0]).toEqual({
      auto_enabled: true,
      scan_enabled: false,
      scan_interval_min: 60,
      downloaders: [],
      categories: ["movies"],
      tags: [],
      save_paths: ["/downloads", "/data/tv"],
      min_video_mb: 50,
      notify_channels: [],
      delete_links_on_remove: false,
    });
    expect(ui.success).toHaveBeenCalledWith("已保存整理设置");
  });

  it("媒体库：列表、编辑只发输入字段、预览模板、检查目录、启停与删除", async () => {
    await mountPage();
    const table = q("ml-libs")!.textContent ?? "";
    expect(table).toContain("/media/movies");
    expect(table).toContain("奥本海默 (2023)/奥本海默 (2023) - 2160p");
    expect(table).toContain("硬链接");

    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(api.templatePreview).toHaveBeenCalled());
    await flush();
    expect(q("ml-lib-preview")!.textContent).toContain("示例：示例路径");
    api.checkLibrary.mockResolvedValue([
      { name: "库目录存在", ok: true },
      { name: "硬链接：/data/downloads", ok: false, message: "不在同一个文件系统" },
    ]);
    q("ml-lib-check")!.click();
    await vi.waitFor(() => expect(q("ml-lib-checks")).not.toBeNull());
    expect(api.checkLibrary).toHaveBeenCalledWith({ path: "/media/movies", mode: "hardlink" });
    expect(q("ml-lib-checks")!.textContent).toContain("不在同一个文件系统");

    api.updateLibrary.mockResolvedValue(library);
    fill("ml-lib-name", " 电影库 ");
    q("ml-lib-save")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(1));
    expect(api.updateLibrary.mock.calls[0]).toEqual([
      3,
      {
        name: "电影库",
        kind: "movie",
        anime: false,
        path: "/media/movies",
        template: "",
        mode: "hardlink",
        scrape: true,
        scrape_overwrite: false,
        enabled: true,
      },
    ]);

    q("ml-lib-enabled-3")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(2));
    expect(api.updateLibrary.mock.calls[1]![1]).toMatchObject({ enabled: false });
    expect(Object.keys(api.updateLibrary.mock.calls[1]![1])).not.toContain("preview");

    ui.confirm.mockResolvedValue("confirm");
    api.deleteLibrary.mockResolvedValue({ ok: true });
    q("ml-lib-del-3")!.click();
    await vi.waitFor(() => expect(api.deleteLibrary).toHaveBeenCalledWith(3));
  });

  it("模板写错时预览写明原因", async () => {
    await mountPage();
    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(api.templatePreview).toHaveBeenCalledTimes(1));
    api.templatePreview.mockResolvedValue({
      template: "{{.Nope}}",
      preview: "",
      error: "命名模板不能用: Nope",
    });
    fill("ml-lib-template", "{{.Nope}}");
    await vi.waitFor(() => expect(api.templatePreview).toHaveBeenCalledTimes(2), { timeout: 2000 });
    await flush();
    expect(q("ml-lib-preview")!.textContent).toContain("命名模板不能用");
  });

  it("路径映射与媒体服务器：编辑时不填 Token 就不带；测试用保存的 Token", async () => {
    await mountPage();
    expect(q("ml-maps")!.textContent).toContain("qb");
    api.updatePathMap.mockResolvedValue({});
    q("ml-map-edit-7")!.click();
    await flush();
    fill("ml-map-to", " /mnt/downloads ");
    q("ml-map-save")!.click();
    await vi.waitFor(() => expect(api.updatePathMap).toHaveBeenCalledTimes(1));
    expect(api.updatePathMap.mock.calls[0]).toEqual([
      7,
      { downloader_id: 1, downloader_prefix: "/downloads", local_prefix: "/mnt/downloads" },
    ]);

    expect(q("ml-servers")!.textContent).toContain("只扫整理到的目录");
    api.testServer.mockResolvedValue({ name: "Emby", version: "4.8" });
    q("ml-server-test-5")!.click();
    await vi.waitFor(() =>
      expect(api.testServer).toHaveBeenCalledWith({
        id: 5,
        kind: "emby",
        url: "http://192.168.1.10:8096",
      }),
    );
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("连接正常：Emby 4.8"));

    api.updateServer.mockResolvedValue(server);
    q("ml-server-edit-5")!.click();
    await flush();
    q("ml-server-save")!.click();
    await vi.waitFor(() => expect(api.updateServer).toHaveBeenCalledTimes(1));
    const body = api.updateServer.mock.calls[0]![1];
    expect(body).not.toHaveProperty("token");
    expect(body).toEqual({
      name: "Emby",
      kind: "emby",
      url: "http://192.168.1.10:8096",
      enabled: true,
      refresh_mode: "path",
      local_prefix: "/media",
      server_prefix: "/data",
    });

    // 新建时不填 Token 不发请求
    q("ml-server-edit-5")!.click();
    await flush();
    fill("ml-server-token", " newtoken ");
    q("ml-server-save")!.click();
    await vi.waitFor(() => expect(api.updateServer).toHaveBeenCalledTimes(2));
    expect(api.updateServer.mock.calls[1]![1]).toMatchObject({ token: "newtoken" });
  });

  it("媒体库读不到时写明原因，不画成「还没有」", async () => {
    await mountPage({ libsFail: true });
    const state = q("ml-libs-state")!.textContent ?? "";
    expect(state).toContain("读取媒体库失败");
    expect(state).not.toContain("还没有媒体库");
  });
});
