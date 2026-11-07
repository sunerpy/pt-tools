// @vitest-environment happy-dom
/*
 * 钉子：媒体库页。整理设置只发接口认的字段（保存路径可从下载器的预设目录选、也可输入，不带 auto_since）；媒体库、路径映射、媒体服务器的
 * 增删改只发输入字段（严格解析）；编辑媒体服务器时不填 Token 就不带 Token；列表读不到时留在页面上。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  settings: vi.fn(),
  saveSettings: vi.fn(),
  classify: vi.fn(),
  saveClassify: vi.fn(),
  watchDirs: vi.fn(),
  createWatchDir: vi.fn(),
  updateWatchDir: vi.fn(),
  deleteWatchDir: vi.fn(),
  libraries: vi.fn(),
  createLibrary: vi.fn(),
  updateLibrary: vi.fn(),
  deleteLibrary: vi.fn(),
  checkLibrary: vi.fn(),
  testCSF: vi.fn(),
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
  syncServer: vi.fn(),
  serverUsers: vi.fn(),
  createWebhook: vi.fn(),
  deleteWebhook: vi.fn(),
  resume: vi.fn(),
  preview: vi.fn(),
  organize: vi.fn(),
}));
const dl = vi.hoisted(() => ({ list: vi.fn() }));
const dirs = vi.hoisted(() => ({ listAll: vi.fn() }));
const media = vi.hoisted(() => ({ settings: vi.fn() }));
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
    downloaderDirectoriesApi: { ...real.downloaderDirectoriesApi, listAll: dirs.listAll },
    mediaApi: { ...real.mediaApi, settings: media.settings },
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

/** 点开下拉选一项（按选项的文字） */
async function pick(id: string, text: string) {
  const sel = q(id)!;
  (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
  const listId = sel.querySelector("input")?.getAttribute("aria-controls");
  await vi.waitFor(() => {
    const opt = [
      ...(document
        .getElementById(listId ?? "")
        ?.querySelectorAll<HTMLElement>(".el-select-dropdown__item") ?? []),
    ].find((o) => (o.textContent ?? "").trim() === text);
    expect(opt, `下拉里有「${text}」`).toBeTruthy();
    opt!.click();
  });
  await flush();
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
  watch_interval_min: 10,
  watch_realtime: true,
  runs: { watch_last: "2026-10-15T10:00:00Z", watch_next: "2026-10-15T10:10:00Z" },
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
  scrape_items: ["nfo", "poster", "fanart", "season_poster", "thumb"],
  scrape_image_lang: "",
  scrape_source: "",
  scrape_sources: { plot: "", rating: "", episode: "", artwork: "" },
  classify_dir: false,
  overwrite_mode: "",
  enabled: true,
  created_at: "",
  updated_at: "",
  effective_template: "{{.Title}}",
  preview: "奥本海默 (2023)/奥本海默 (2023) - 2160p BluRay HDR10 H.265",
};

const classifyPresets = {
  movie: [{ name: "华语电影", languages: ["zh"] }, { name: "外语电影" }],
  tv: [{ name: "欧美剧", countries: ["US"] }],
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
  play_url: "",
  index_hours: 6,
  indexed_at: "2026-11-04T06:00:00Z",
  index_error: "",
  index_movies: 120,
  index_series: 30,
  index_episodes: 900,
  count_library: false,
  resume_user_id: "",
  resume_user_name: "",
  webhook: false,
  webhook_event: "",
  playback_channels: [],
  created_at: "",
  updated_at: "",
};

async function mountPage(
  opts: {
    libsFail?: boolean;
    settingsFail?: boolean;
    library?: Record<string, unknown>;
    downloaders?: Record<string, unknown>[];
    downloadersFail?: boolean;
    settings?: Record<string, unknown>;
  } = {},
) {
  if (opts.settingsFail) api.settings.mockRejectedValue(new Error("读取整理设置失败"));
  else api.settings.mockResolvedValue(opts.settings ?? settings);
  media.settings.mockResolvedValue({
    key_source: "user",
    has_fanart_key: false,
    has_omdb_key: false,
  });
  if (opts.libsFail) api.libraries.mockRejectedValue(new Error("读取媒体库失败"));
  else api.libraries.mockResolvedValue([opts.library ?? library]);
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
  api.classify.mockResolvedValue({ rules: { movie: [], tv: [] }, presets: classifyPresets });
  api.watchDirs.mockResolvedValue([
    {
      id: 4,
      path: "/media/incoming",
      enabled: true,
      created_at: "",
      updated_at: "",
      realtime: "on",
    },
    {
      id: 6,
      path: "/mnt/nas/in",
      enabled: true,
      created_at: "",
      updated_at: "",
      realtime: "unavailable",
      realtime_note: "在网络文件系统（NFS）上，收不到别的机器写进来的通知",
    },
    // 关着的目录：即使状态还写着 on 也不写
    { id: 8, path: "/media/off", enabled: false, created_at: "", updated_at: "", realtime: "on" },
  ]);
  api.templatePreview.mockResolvedValue({ template: "", preview: "示例路径" });
  if (opts.downloadersFail) dl.list.mockRejectedValue(new Error("读取下载器失败"));
  else
    dl.list.mockResolvedValue(
      opts.downloaders ?? [
        {
          id: 1,
          name: "qb",
          type: "qbittorrent",
          url: "",
          username: "",
          is_default: true,
          enabled: true,
        },
      ],
    );
  notifications.list.mockResolvedValue([
    { id: 2, channel_type: "telegram", name: "TG", enabled: true },
  ]);
  dirs.listAll.mockResolvedValue({
    1: [{ id: 5, downloader_id: 1, path: "/data/tv", alias: "剧集", is_default: false }],
  });
  const Page = (await import("./MediaLibrary.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.settings).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("媒体库", () => {
  it("整理设置：只发输入字段，保存路径可选下载器的预设目录、也可输入", async () => {
    await mountPage();
    expect(document.body.textContent).toContain("以后下载完成的种子");
    api.saveSettings.mockResolvedValue(settings);
    // 从下拉里选下载器预设的目录（显示别名与路径）
    const sel = q("ml-scope-path")!;
    (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
    const listId = sel.querySelector("input")?.getAttribute("aria-controls");
    await vi.waitFor(() => {
      const opt = [
        ...(document
          .getElementById(listId ?? "")
          ?.querySelectorAll<HTMLElement>(".el-select-dropdown__item") ?? []),
      ].find((o) => (o.textContent ?? "").trim() === "剧集（/data/tv）");
      expect(opt).toBeTruthy();
      opt!.click();
    });
    // 列表里没有的路径输入后回车
    const input = inputOf("ml-scope-path") as HTMLInputElement;
    input.value = " /custom/x ";
    input.dispatchEvent(new Event("input"));
    await flush();
    input.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Enter", code: "Enter", bubbles: true }),
    );
    await flush();
    q("ml-save-settings")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(1));
    expect(api.saveSettings.mock.calls[0]![0]).toEqual({
      auto_enabled: true,
      scan_enabled: false,
      scan_interval_min: 60,
      downloaders: [],
      categories: ["movies"],
      tags: [],
      save_paths: ["/downloads", "/data/tv", "/custom/x"],
      min_video_mb: 50,
      notify_channels: [],
      delete_links_on_remove: false,
      watch_interval_min: 10,
      watch_realtime: true,
    });
    expect(ui.success).toHaveBeenCalledWith("已保存整理设置");
    expect(q("ml-watch-runs")!.textContent, "目录监控写明上次与下次扫描").toContain("下次大约");
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
        scrape_items: ["nfo", "poster", "fanart", "season_poster", "thumb"],
        scrape_image_lang: "",
        scrape_sources: { plot: "", rating: "", episode: "", artwork: "" },
        classify_dir: false,
        overwrite_mode: "",
        scrape_actor_zh: false,
        scrape_fill: false,
        site_subtitles: false,
        csf_url: "",
        csf_path: "",
        download_downloader_id: 0,
        download_save_path: "",
        download_category: "",
        download_tags: "",
        enabled: true,
      },
    ]);
    expect(Object.keys(api.updateLibrary.mock.calls[0]![1]), "没填 Key 时不带 Key").not.toContain(
      "csf_key",
    );

    q("ml-lib-enabled-3")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(2));
    expect(api.updateLibrary.mock.calls[1]![1]).toMatchObject({ enabled: false });
    expect(Object.keys(api.updateLibrary.mock.calls[1]![1])).not.toContain("preview");

    ui.confirm.mockResolvedValue("confirm");
    api.deleteLibrary.mockResolvedValue({ ok: true });
    q("ml-lib-del-3")!.click();
    await vi.waitFor(() => expect(api.deleteLibrary).toHaveBeenCalledWith(3));
  });

  it("媒体库：站点字幕与 ChineseSubFinder 跟着保存，Key 只写不读；测试没填 Key 时用保存的", async () => {
    await mountPage({
      library: {
        ...library,
        site_subtitles: false,
        csf_url: "http://csf.lan/",
        csf_path: "",
        has_csf_key: true,
      },
    });
    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(q("ml-lib-site-subs")).not.toBeNull());
    expect(inputOf("ml-lib-csf-url").value).toBe("http://csf.lan/");
    expect(inputOf("ml-lib-csf-key").value, "Key 不回显").toBe("");
    expect(inputOf("ml-lib-csf-key").getAttribute("placeholder")).toContain("已设置");

    // 测试：没填 Key 时带上媒体库编号，用保存的
    api.testCSF.mockResolvedValue({ ok: true });
    q("ml-lib-csf-test")!.click();
    await vi.waitFor(() => expect(api.testCSF).toHaveBeenCalledTimes(1));
    expect(api.testCSF).toHaveBeenCalledWith({ id: 3, url: "http://csf.lan/", key: undefined });
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("ChineseSubFinder 连接正常"));
    api.testCSF.mockRejectedValue(new Error("ChineseSubFinder 的 API Key 不对"));
    fill("ml-lib-csf-key", " new-key ");
    q("ml-lib-csf-test")!.click();
    await vi.waitFor(() => expect(api.testCSF).toHaveBeenCalledTimes(2));
    expect(api.testCSF.mock.calls[1]![0]).toEqual({
      id: 3,
      url: "http://csf.lan/",
      key: "new-key",
    });
    await vi.waitFor(() =>
      expect(ui.error).toHaveBeenCalledWith("ChineseSubFinder 的 API Key 不对"),
    );

    q("ml-lib-site-subs")!.querySelector("input")!.click();
    fill("ml-lib-csf-url", " http://csf2.lan:19035 ");
    fill("ml-lib-csf-path", " /csf/movies ");
    await flush();
    api.updateLibrary.mockResolvedValue(library);
    q("ml-lib-save")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(1));
    expect(api.updateLibrary.mock.calls[0]![1]).toMatchObject({
      site_subtitles: true,
      csf_url: "http://csf2.lan:19035",
      csf_path: "/csf/movies",
      csf_key: "new-key",
    });
  });

  it("媒体库：新建订阅时带出的下载设置跟着保存；停用的下载器标出来；下载器删了显示不指定、保存时清成 0；不在整理范围里时提醒", async () => {
    await mountPage({
      library: {
        ...library,
        kind: "tv",
        download_downloader_id: 9,
        download_save_path: "/downloads/tv",
        download_category: "tv",
        download_tags: "pt-tools",
      },
      downloaders: [
        {
          id: 1,
          name: "qB",
          type: "qbittorrent",
          url: "",
          username: "",
          is_default: true,
          enabled: true,
        },
        {
          id: 2,
          name: "qB-2",
          type: "qbittorrent",
          url: "",
          username: "",
          is_default: false,
          enabled: false,
        },
      ],
    });
    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(q("ml-lib-dl-downloader")).not.toBeNull());
    await flush();
    expect(q("ml-lib-dl-gone")!.textContent).toContain("原来设的下载器已删除，保存时改成不指定");
    expect(q("ml-lib-dl-downloader")!.textContent, "删了的下载器显示不指定").toContain("不指定");
    expect(inputOf("ml-lib-dl-path").value).toBe("/downloads/tv");
    expect(inputOf("ml-lib-dl-category").value).toBe("tv");
    expect(q("ml-lib-dl-qb")!.textContent).toContain("默认种子管理模式是「自动」");
    // 整理范围只收分类 movies、保存目录在 /downloads 下
    expect(q("ml-lib-dl-scope")!.textContent).toContain(
      "这样推送的种子下完不会自动整理：分类「tv」不在范围里（movies）",
    );

    await pick("ml-lib-dl-downloader", "qB-2（已停用）");
    expect(q("ml-lib-dl-disabled")!.textContent).toContain(
      "这个下载器停用期间，新建订阅不会带出它",
    );
    expect(q("ml-lib-dl-gone"), "选了别的下载器以后不再写删了").toBeNull();
    await pick("ml-lib-dl-downloader", "qB");
    expect(q("ml-lib-dl-disabled")).toBeNull();

    fill("ml-lib-dl-category", " movies ");
    fill("ml-lib-dl-tags", " pt-tools,追剧 ");
    fill("ml-lib-dl-path", "/data/tv");
    await flush();
    expect(q("ml-lib-dl-scope")!.textContent).toContain("保存目录不在范围里（/downloads）");
    fill("ml-lib-dl-path", " /downloads/tv2 ");
    await flush();
    expect(q("ml-lib-dl-scope"), "都在范围里时不提醒").toBeNull();

    api.updateLibrary.mockResolvedValue(library);
    q("ml-lib-save")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(1));
    expect(api.updateLibrary.mock.calls[0]![1]).toMatchObject({
      download_downloader_id: 1,
      download_save_path: "/downloads/tv2",
      download_category: "movies",
      download_tags: "pt-tools,追剧",
    });

    // 列表里的启用开关照原样带着四项（删了的下载器编号照存，服务器不挡）
    q("ml-lib-enabled-3")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(2));
    expect(api.updateLibrary.mock.calls[1]![1]).toMatchObject({
      enabled: false,
      download_downloader_id: 9,
      download_save_path: "/downloads/tv",
      download_category: "tv",
      download_tags: "pt-tools",
    });
  });

  it("媒体库：库里的下载器删了、保存时发出 0；自动整理与定期扫描都关着时不提醒范围", async () => {
    await mountPage({
      settings: { ...settings, auto_enabled: false, scan_enabled: false },
      library: { ...library, download_downloader_id: 9, download_category: "tv" },
    });
    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(q("ml-lib-dl-gone")).not.toBeNull());
    expect(q("ml-lib-dl-scope"), "不自动整理时范围不起作用").toBeNull();
    api.updateLibrary.mockResolvedValue(library);
    q("ml-lib-save")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(1));
    expect(api.updateLibrary.mock.calls[0]![1]).toMatchObject({
      download_downloader_id: 0,
      download_category: "tv",
    });
  });

  it("媒体库：读不到下载器列表时不改库里存的下载器", async () => {
    await mountPage({ library: { ...library, download_downloader_id: 9 }, downloadersFail: true });
    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(q("ml-lib-dl-downloader")).not.toBeNull());
    await flush();
    expect(q("ml-lib-dl-gone")).toBeNull();
    expect(q("ml-lib-dl-downloader")!.textContent).toContain("下载器 9");
    api.updateLibrary.mockResolvedValue(library);
    q("ml-lib-save")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(1));
    expect(api.updateLibrary.mock.calls[0]![1]).toMatchObject({ download_downloader_id: 9 });
  });

  it("媒体库：新建时填了 ChineseSubFinder 的地址却没填 Key 不保存；地址没填时测试不发请求", async () => {
    await mountPage();
    // 媒体库那张卡右上角的「添加」
    [...document.querySelectorAll<HTMLElement>(".pt-panel__action")]
      .find((b) => b.textContent?.includes("添加"))!
      .click();
    await vi.waitFor(() => expect(q("ml-lib-csf-url")).not.toBeNull());
    q("ml-lib-csf-test")!.click();
    await flush();
    expect(api.testCSF).not.toHaveBeenCalled();
    expect(ui.warning).toHaveBeenCalledWith("先填 ChineseSubFinder 的地址");
    fill("ml-lib-name", "剧集");
    fill("ml-lib-path", "/media/tv");
    fill("ml-lib-csf-url", "http://csf.lan/");
    await flush();
    q("ml-lib-save")!.click();
    await flush();
    expect(api.createLibrary).not.toHaveBeenCalled();
    expect(ui.warning).toHaveBeenCalledWith("填了 ChineseSubFinder 的地址就要填它的 API Key");
    fill("ml-lib-csf-key", "k");
    api.createLibrary.mockResolvedValue(library);
    q("ml-lib-save")!.click();
    await vi.waitFor(() => expect(api.createLibrary).toHaveBeenCalledTimes(1));
    expect(api.createLibrary.mock.calls[0]![0]).toMatchObject({
      csf_url: "http://csf.lan/",
      csf_key: "k",
    });
  });

  it("媒体库：演员名用中文、每周补刮跟着保存，关着刮削时不能选；导入已有文件打开导入弹窗", async () => {
    await mountPage();
    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(q("ml-lib-actor-zh")).not.toBeNull());
    const box = (id: string) => q(id)!.querySelector("input")!;
    expect(box("ml-lib-actor-zh").checked).toBe(false);
    expect(box("ml-lib-fill").checked).toBe(false);
    box("ml-lib-actor-zh").click();
    await flush();
    box("ml-lib-fill").click();
    await flush();
    api.updateLibrary.mockResolvedValue(library);
    q("ml-lib-save")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(1));
    expect(api.updateLibrary.mock.calls[0]![1]).toMatchObject({
      scrape_actor_zh: true,
      scrape_fill: true,
    });

    // 关掉刮削：这两项跟着不能选
    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(q("ml-lib-actor-zh")).not.toBeNull());
    expect(box("ml-lib-actor-zh").disabled).toBe(false);
    q("ml-lib-scrape")!.click();
    await flush();
    expect(box("ml-lib-actor-zh").disabled, "关着刮削").toBe(true);
    expect(box("ml-lib-fill").disabled).toBe(true);

    // 列表里每个媒体库都能导入已有文件：打开弹窗，路径默认是库目录
    document.querySelector<HTMLElement>(".el-dialog__headerbtn")?.click();
    await flush();
    q("ml-lib-import-3")!.click();
    await vi.waitFor(() => expect(q("import-path")).not.toBeNull());
    expect((q("import-path") as HTMLInputElement).value).toBe("/media/movies");
    expect(document.body.textContent).toContain("导入已有文件：电影");
  });

  it("媒体库：刮削内容按类型列出，选了才写；开着刮削时至少选一项", async () => {
    await mountPage();
    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(q("ml-lib-items")).not.toBeNull());
    expect(q("ml-lib-item-logo")).not.toBeNull();
    expect(q("ml-lib-item-season_poster"), "电影没有季海报").toBeNull();
    expect(q("ml-lib-item-thumb")).toBeNull();
    const box = (id: string) => q(id)!.querySelector("input")!;
    expect(box("ml-lib-item-poster").checked).toBe(true);
    expect(box("ml-lib-item-logo").checked, "没选过时不写 Logo").toBe(false);
    // 值是空串的选项（TMDB、跟识别语言、不替换）照样显示出来，不是「请选择」
    const shown = (id: string) =>
      q(id)!.querySelector(".el-select__placeholder:not(.is-transparent)")?.textContent?.trim();
    expect(shown("ml-lib-image-lang")).toBe("跟识别语言（TMDB 默认的那张）");
    expect(shown("ml-lib-source-plot")).toBe("TMDB");
    expect(shown("ml-lib-overwrite-mode")).toBe("不替换");
    // 下拉框旁边只是一段文字：读屏软件靠 aria-label 读出它是什么
    const named = (id: string) =>
      q(id)!.querySelector("[role=combobox]")?.getAttribute("aria-label");
    expect(named("ml-lib-image-lang")).toBe("图片语言");
    expect(named("ml-lib-source-plot")).toBe("简介的数据来源");

    // 每点一下等一轮更新：选项组按上一次的值算下一次
    box("ml-lib-item-poster").click();
    await flush();
    box("ml-lib-item-logo").click();
    await flush();
    const lang = q("ml-lib-image-lang")!;
    (lang.querySelector<HTMLElement>(".el-select__wrapper") ?? lang).click();
    await vi.waitFor(() => {
      const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
        (o) => (o.textContent ?? "").trim() === "英文",
      );
      expect(opt).toBeTruthy();
      opt!.click();
    });
    // 数据来源按项选：电影没有「每一集」；评分选了 IMDb、没填 OMDb 的 Key 时提示
    expect(q("ml-lib-source-episode"), "电影没有每一集").toBeNull();
    const pick = async (id: string, text: string) => {
      const sel = q(id)!;
      (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
      await vi.waitFor(() => {
        const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
          (o) => (o.textContent ?? "").trim().startsWith(text),
        );
        expect(opt).toBeTruthy();
        opt!.click();
      });
      await flush();
    };
    await pick("ml-lib-source-plot", "豆瓣");
    await pick("ml-lib-source-rating", "IMDb");
    await vi.waitFor(() =>
      expect(q("ml-lib-source-warn")?.textContent).toContain("还没在「媒体识别」里填 OMDb 的 Key"),
    );
    box("ml-lib-item-clearart").click();
    await flush();
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain(
        "Clearart 只有 Fanart.tv 有，还没在「媒体识别」里填 Fanart.tv 的 Key",
      ),
    );
    api.updateLibrary.mockResolvedValue(library);
    q("ml-lib-save")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(1));
    expect(api.updateLibrary.mock.calls[0]![1]).toMatchObject({
      scrape_items: ["nfo", "fanart", "season_poster", "thumb", "logo", "clearart"],
      scrape_image_lang: "en",
      scrape_sources: { plot: "douban", rating: "omdb", episode: "", artwork: "" },
    });
    expect(Object.keys(api.updateLibrary.mock.calls[0]![1])).not.toContain("scrape_source");

    // 电影能选的几项都去掉：不保存，提示关掉刮削
    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(q("ml-lib-items")).not.toBeNull());
    for (const id of ["nfo", "poster", "fanart", "logo"]) {
      if (box(`ml-lib-item-${id}`).checked) box(`ml-lib-item-${id}`).click();
      await flush();
    }
    q("ml-lib-save")!.click();
    await vi.waitFor(() =>
      expect(ui.warning).toHaveBeenCalledWith("刮削内容至少选一项；不想刮削就关掉刮削"),
    );
    expect(api.updateLibrary).toHaveBeenCalledTimes(1);
  });

  it("目录监控：实时监控的开关跟着保存；每个目录写明在不在实时监控、不可用的原因", async () => {
    await mountPage();
    await vi.waitFor(() => expect(q("ml-watch-4")).not.toBeNull());
    expect(q("ml-watch-rt-4")!.textContent).toBe("实时监控中");
    expect(q("ml-watch-rt-6")!.textContent).toBe(
      "实时监控不可用：在网络文件系统（NFS）上，收不到别的机器写进来的通知，按间隔扫描",
    );
    expect(q("ml-watch-rt-6")!.classList.contains("ml-watch__rt--warn")).toBe(true);
    expect(q("ml-watch-rt-8"), "关着的目录不写").toBeNull();
    const field = q("ml-watch-realtime")!.closest(".el-form-item")!;
    expect(field.textContent).toContain("实时监控漏掉的照样整理");

    api.saveSettings.mockResolvedValue({ ...settings, watch_realtime: false });
    q("ml-watch-realtime")!.click();
    await flush();
    expect(field.textContent, "关掉以后只按间隔扫描").toContain(
      "分钟扫一次下面「目录监控」里开着的目录",
    );
    q("ml-save-settings")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(1));
    expect(api.saveSettings.mock.calls[0]![0]).toMatchObject({ watch_realtime: false });
  });

  it("设置没读到时保存的是默认值：实时监控同后端一样默认开", async () => {
    await mountPage({ settingsFail: true });
    await vi.waitFor(() => expect(document.body.textContent).toContain("设置没读到"));
    api.saveSettings.mockResolvedValue(settings);
    q("ml-save-settings")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(1));
    expect(api.saveSettings.mock.calls[0]![0]).toMatchObject({
      watch_realtime: true,
      watch_interval_min: 10,
    });
  });

  it("目录监控：列表、添加只发输入字段、启停与删除", async () => {
    await mountPage();
    await vi.waitFor(() => expect(q("ml-watch-4")).not.toBeNull());
    expect(q("ml-watch-4")!.textContent).toContain("/media/incoming");

    const add = [...document.querySelectorAll<HTMLButtonElement>("button")].filter((b) =>
      (b.textContent ?? "").includes("添加"),
    );
    // 「目录监控」面板的添加：按面板顺序找到它
    const watchPanel = q("ml-watch")!.closest(".pt-panel") ?? document.body;
    const btn = add.find((b) => watchPanel.contains(b)) ?? add[0]!;
    btn.click();
    await vi.waitFor(() => expect(q("ml-watch-path")).not.toBeNull());
    q("ml-watch-save")!.click();
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalledWith("填写要监控的目录"));
    api.createWatchDir.mockResolvedValue({ id: 5, path: "/data/in", enabled: true });
    fill("ml-watch-path", " /data/in ");
    q("ml-watch-save")!.click();
    await vi.waitFor(() => expect(api.createWatchDir).toHaveBeenCalledTimes(1));
    expect(api.createWatchDir.mock.calls[0]![0]).toEqual({ path: "/data/in", enabled: true });

    api.updateWatchDir.mockResolvedValue({});
    const before = api.settings.mock.calls.length;
    q("ml-watch-enabled-4")!.click();
    await vi.waitFor(() =>
      expect(api.updateWatchDir).toHaveBeenCalledWith(4, {
        path: "/media/incoming",
        enabled: false,
      }),
    );

    await vi.waitFor(() =>
      expect(api.settings.mock.calls.length, "开关了目录：重新读扫描的上次与下次").toBeGreaterThan(
        before,
      ),
    );

    ui.confirm.mockResolvedValue("confirm");
    api.deleteWatchDir.mockResolvedValue({ ok: true });
    q("ml-watch-del-4")!.click();
    await vi.waitFor(() => expect(api.deleteWatchDir).toHaveBeenCalledWith(4));
  });

  it("页头的「手动整理」打开指定路径的弹窗，不发预览请求；目录监控的说明指向它", async () => {
    await mountPage();
    const watchTip = q("ml-watch")!.closest(".pt-panel")?.textContent ?? "";
    expect(watchTip).toContain(
      "要马上整理某个文件或文件夹（不用加监控目录），点页面上方的「手动整理」。",
    );
    q("ml-manual")!.click();
    await vi.waitFor(() => expect(q("organize-source")).not.toBeNull());
    await flush();
    expect(document.querySelector(".el-dialog__title")?.textContent).toBe("手动整理");
    expect(api.preview).not.toHaveBeenCalled();
  });

  it("二级分类：填入常用分类、调整顺序、添加与删除，每次整份保存", async () => {
    await mountPage();
    await vi.waitFor(() => expect(api.classify).toHaveBeenCalled());
    expect(q("ml-classify-movie")!.textContent).toContain("没有规则：电影不分类");

    api.saveClassify.mockImplementation(async (rules) => ({ rules, presets: classifyPresets }));
    const fillBtn = [...document.querySelectorAll<HTMLButtonElement>("button")].find((b) =>
      (b.textContent ?? "").includes("填入常用分类"),
    );
    fillBtn!.click();
    await vi.waitFor(() => expect(api.saveClassify).toHaveBeenCalledTimes(1));
    expect(ui.confirm, "还没有规则时不用确认").not.toHaveBeenCalled();
    expect(api.saveClassify.mock.calls[0]![0]).toEqual(classifyPresets);
    await flush();
    expect(q("ml-rule-movie-0")!.textContent).toContain("华语电影");
    expect(q("ml-rule-movie-0")!.textContent).toContain("语言：中文");
    expect(q("ml-rule-movie-1")!.textContent).toContain("前面都对不上的放进这里");
    expect(q("ml-rule-tv-0")!.textContent).toContain("国家或地区：美国");

    q("ml-rule-down-movie-0")!.click();
    await vi.waitFor(() => expect(api.saveClassify).toHaveBeenCalledTimes(2));
    expect(api.saveClassify.mock.calls[1]![0].movie.map((r: { name: string }) => r.name)).toEqual([
      "外语电影",
      "华语电影",
    ]);
    await flush();

    q("ml-classify-add-tv")!.click();
    await vi.waitFor(() => expect(q("ml-rule-name")).not.toBeNull());
    q("ml-rule-save")!.click();
    await vi.waitFor(() => expect(ui.warning).toHaveBeenCalledWith("填写分类名称"));
    fill("ml-rule-name", " 日韩剧 ");
    q("ml-rule-save")!.click();
    await vi.waitFor(() => expect(api.saveClassify).toHaveBeenCalledTimes(3));
    expect(api.saveClassify.mock.calls[2]![0].tv).toEqual([
      { name: "欧美剧", countries: ["US"] },
      { name: "日韩剧", genres: [], languages: [], countries: [] },
    ]);
    await flush();

    ui.confirm.mockResolvedValue("confirm");
    q("ml-rule-del-tv-0")!.click();
    await vi.waitFor(() => expect(api.saveClassify).toHaveBeenCalledTimes(4));
    expect(api.saveClassify.mock.calls[3]![0].tv.map((r: { name: string }) => r.name)).toEqual([
      "日韩剧",
    ]);

    // 已经有规则时填入常用分类要先确认；取消就不保存
    ui.confirm.mockRejectedValueOnce("cancel");
    fillBtn!.click();
    await flush();
    expect(api.saveClassify).toHaveBeenCalledTimes(4);
  });

  it("二级分类：片长上限与标题关键字（短剧）一起保存，摘要写成「或」；编辑时带出原来的值", async () => {
    // mountPage 把读分类设成空的：排一次带短剧规则的回答（排着的先用）
    api.classify.mockResolvedValueOnce({
      rules: { movie: [], tv: [{ name: "短剧", max_runtime: 15, keywords: ["短剧"] }] },
      presets: classifyPresets,
    });
    await mountPage();
    await vi.waitFor(() => expect(q("ml-rule-tv-0")).not.toBeNull());
    expect(q("ml-rule-tv-0")!.textContent).toContain("片长不超过 15 分钟或标题含「短剧」");

    // 编辑：带出原来的片长与关键字
    q("ml-rule-edit-tv-0")!.click();
    await vi.waitFor(() => expect(q("ml-rule-runtime")).not.toBeNull());
    expect(q("ml-rule-runtime")!.querySelector("input")!.value).toBe("15");
    expect(q("ml-rule-keywords")!.textContent).toContain("短剧");
    [...document.querySelectorAll<HTMLButtonElement>("button")]
      .find((b) => b.textContent?.trim() === "取消" && b.closest(".el-dialog"))!
      .click();
    await flush();

    api.saveClassify.mockImplementation(async (rules) => ({ rules, presets: classifyPresets }));
    q("ml-classify-add-tv")!.click();
    await vi.waitFor(() => expect(q("ml-rule-name")).not.toBeNull());
    fill("ml-rule-name", "微短剧");
    const runtime = q("ml-rule-runtime")!.querySelector("input")!;
    runtime.value = "10";
    runtime.dispatchEvent(new Event("input"));
    runtime.dispatchEvent(new Event("change"));
    const kw = q("ml-rule-keywords")!.querySelector("input")!;
    kw.value = " 竖屏 ";
    kw.dispatchEvent(new Event("input"));
    await flush();
    // 能自己填的下拉框：输入的字作为一个选项出现在最前面，点它
    const created = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
      (el) => el.textContent?.trim() === "竖屏",
    );
    created!.click();
    await flush();
    q("ml-rule-save")!.click();
    await vi.waitFor(() => expect(api.saveClassify).toHaveBeenCalledTimes(1));
    expect(api.saveClassify.mock.calls[0]![0].tv[1], "关键字去掉前后的空白").toEqual({
      name: "微短剧",
      genres: [],
      languages: [],
      countries: [],
      max_runtime: 10,
      keywords: ["竖屏"],
    });
  });

  it("媒体库：同名文件选「新文件更大时替换」时带上 overwrite_mode", async () => {
    await mountPage();
    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(q("ml-lib-overwrite-mode")).not.toBeNull());
    const sel = q("ml-lib-overwrite-mode")!;
    (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
    await vi.waitFor(() => {
      const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
        (o) => (o.textContent ?? "").trim() === "新文件更大时替换",
      );
      expect(opt).toBeTruthy();
      opt!.click();
    });
    api.updateLibrary.mockResolvedValue(library);
    q("ml-lib-save")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(1));
    expect(api.updateLibrary.mock.calls[0]![1]).toMatchObject({ overwrite_mode: "larger" });
  });

  it("媒体库：勾选按二级分类放进子目录时带上 classify_dir", async () => {
    await mountPage();
    q("ml-lib-edit-3")!.click();
    await vi.waitFor(() => expect(q("ml-lib-classify")).not.toBeNull());
    q("ml-lib-classify")!.querySelector("input")!.click();
    await flush();
    api.updateLibrary.mockResolvedValue(library);
    q("ml-lib-save")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(1));
    expect(api.updateLibrary.mock.calls[0]![1]).toMatchObject({ classify_dir: true });
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
      play_url: "",
      index_hours: 6,
      count_library: false,
      resume_user_id: "",
      resume_user_name: "",
      playback_channels: [],
    });

    // 新建时不填 Token 不发请求
    q("ml-server-edit-5")!.click();
    await flush();
    fill("ml-server-token", " newtoken ");
    q("ml-server-save")!.click();
    await vi.waitFor(() => expect(api.updateServer).toHaveBeenCalledTimes(2));
    expect(api.updateServer.mock.calls[1]![1]).toMatchObject({ token: "newtoken" });
  });

  it("媒体服务器的索引：列表写条目数与继续观看，能马上同步；关了索引不能同步", async () => {
    api.resume.mockResolvedValue({
      servers: [{ server_id: 5, name: "Emby", kind: "emby", user: "admin", items: [{}, {}, {}] }],
    });
    await mountPage();
    api.servers.mockResolvedValue([{ ...server, resume_user_id: "u1", resume_user_name: "admin" }]);
    // 选了继续观看的用户才去取继续观看
    expect(api.resume).not.toHaveBeenCalled();
    expect(q("ml-server-index-5")!.textContent).toContain("正常");
    expect(q("ml-server-index-5")!.textContent).toContain("电影 120 · 剧 30 · 集 900");

    api.syncServer.mockResolvedValue({
      movies: 121,
      series: 30,
      episodes: 901,
      added: 2,
      updated: 0,
      removed: 1,
      restored: 0,
      purged: 0,
      issues: 1,
    });
    q("ml-server-sync-5")!.click();
    await vi.waitFor(() => expect(api.syncServer).toHaveBeenCalledWith(5));
    await vi.waitFor(() =>
      expect(ui.success).toHaveBeenCalledWith(
        "同步完成：电影 121、剧 30、集 901（新加 2、没了 1），1 项 pt-tools 整理过的在服务器里没了",
      ),
    );
    await vi.waitFor(() => expect(api.resume).toHaveBeenCalled());
    await vi.waitFor(() => expect(q("ml-server-index-5")!.textContent).toContain("继续观看 3"));

    api.syncServer.mockRejectedValue(new Error("媒体服务器连不上"));
    q("ml-server-sync-5")!.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("媒体服务器连不上"));

    api.servers.mockResolvedValue([{ ...server, index_hours: 0 }]);
    q("ml-refresh")!.click();
    await vi.waitFor(() => expect(q("ml-server-index-5")!.textContent).toContain("关闭"));
    expect(
      q("ml-server-sync-5")!.hasAttribute("disabled") ||
        q("ml-server-sync-5")!.classList.contains("is-disabled"),
    ).toBe(true);

    api.servers.mockResolvedValue([{ ...server, index_error: "HTTP 500", indexed_at: undefined }]);
    q("ml-refresh")!.click();
    await vi.waitFor(() => expect(q("ml-server-index-5")!.textContent).toContain("HTTP 500"));
    expect(q("ml-server-index-5")!.textContent).toContain("失败");
  });

  it("媒体服务器弹窗：索引间隔、继续观看的用户、打开链接的地址、Webhook 地址只显示一次", async () => {
    await mountPage();
    api.updateServer.mockResolvedValue(server);
    q("ml-server-edit-5")!.click();
    await flush();
    fill("ml-server-play-url", " https://emby.example.com ");
    api.serverUsers.mockResolvedValue([
      { id: "u1", name: "admin" },
      { id: "u2", name: "家人" },
    ]);
    q("ml-server-users")!.click();
    await vi.waitFor(() =>
      expect(api.serverUsers).toHaveBeenCalledWith({
        id: 5,
        kind: "emby",
        url: "http://192.168.1.10:8096",
        token: undefined,
      }),
    );
    const sel = q("ml-server-resume-user")!;
    (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
    await vi.waitFor(() => {
      const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
        (o) => (o.textContent ?? "").trim() === "家人",
      );
      expect(opt).toBeTruthy();
      opt!.click();
    });
    await flush();

    // Webhook：生成以后地址显示在弹窗里；再生成要先确认
    api.createWebhook.mockResolvedValue({ token: "tok", path: "/api/media/servers/5/webhook/tok" });
    api.servers.mockResolvedValue([{ ...server, webhook: true }]);
    q("ml-server-webhook-create")!.click();
    await vi.waitFor(() => expect(api.createWebhook).toHaveBeenCalledWith(5));
    await vi.waitFor(() =>
      expect((inputOf("ml-server-webhook-url") as HTMLInputElement).value).toBe(
        `${window.location.origin}/api/media/servers/5/webhook/tok`,
      ),
    );
    await vi.waitFor(() =>
      expect(q("ml-server-webhook-create")!.textContent).toContain("重新生成地址"),
    );
    ui.confirm.mockRejectedValueOnce(new Error("cancel"));
    q("ml-server-webhook-create")!.click();
    await flush();
    expect(api.createWebhook, "取消了重新生成").toHaveBeenCalledTimes(1);

    q("ml-server-save")!.click();
    await vi.waitFor(() => expect(api.updateServer).toHaveBeenCalledTimes(1));
    expect(api.updateServer.mock.calls[0]![1]).toMatchObject({
      play_url: "https://emby.example.com",
      resume_user_id: "u2",
      resume_user_name: "家人",
      index_hours: 6,
    });

    // 关掉 Webhook
    api.servers.mockResolvedValue([{ ...server, webhook: true }]);
    q("ml-refresh")!.click();
    await flush();
    q("ml-server-edit-5")!.click();
    await flush();
    expect(q("ml-server-webhook-url"), "重新打开弹窗以后不再显示地址").toBeNull();
    ui.confirm.mockResolvedValueOnce("confirm");
    api.deleteWebhook.mockResolvedValue({ ok: true });
    api.servers.mockResolvedValue([server]);
    q("ml-server-webhook-delete")!.click();
    await vi.waitFor(() => expect(api.deleteWebhook).toHaveBeenCalledWith(5));
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("已关掉 Webhook"));
  });

  it("Plex 不选继续观看的用户；新建的服务器保存以后才能生成 Webhook", async () => {
    await mountPage();
    q("ml-server-edit-5")!.click();
    await flush();
    expect(q("ml-server-resume-user")).not.toBeNull();
    const kinds = [...q("ml-server-kind")!.querySelectorAll<HTMLElement>("label")];
    kinds.find((k) => (k.textContent ?? "").includes("Plex"))!.click();
    await flush();
    expect(q("ml-server-resume-user"), "Plex 没有继续观看").toBeNull();
    api.updateServer.mockResolvedValue(server);
    q("ml-server-save")!.click();
    await vi.waitFor(() => expect(api.updateServer).toHaveBeenCalledTimes(1));
    expect(api.updateServer.mock.calls[0]![1]).toMatchObject({
      kind: "plex",
      resume_user_id: "",
      resume_user_name: "",
    });

    // 添加：还没有编号，不能生成 Webhook
    const panel = q("ml-servers")!.closest(".pt-panel")!;
    panel.querySelector<HTMLElement>(".pt-panel__action")!.click();
    await flush();
    expect(q("ml-server-webhook-create")).toBeNull();
    expect(document.body.textContent).toContain("保存以后，编辑这台服务器时生成地址");
  });

  it("媒体库读不到时写明原因，不画成「还没有」", async () => {
    await mountPage({ libsFail: true });
    const state = q("ml-libs-state")!.textContent ?? "";
    expect(state).toContain("读取媒体库失败");
    expect(state).not.toContain("还没有媒体库");
  });
});
