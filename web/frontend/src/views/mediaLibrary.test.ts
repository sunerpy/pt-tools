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
  watch_interval_min: 10,
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
  api.classify.mockResolvedValue({ rules: { movie: [], tv: [] }, presets: classifyPresets });
  api.watchDirs.mockResolvedValue([
    { id: 4, path: "/media/incoming", enabled: true, created_at: "", updated_at: "" },
  ]);
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
      watch_interval_min: 10,
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
        scrape_source: "",
        classify_dir: false,
        overwrite_mode: "",
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
    const src = q("ml-lib-scrape-source")!;
    (src.querySelector<HTMLElement>(".el-select__wrapper") ?? src).click();
    await vi.waitFor(() => {
      const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
        (o) => (o.textContent ?? "").trim().startsWith("豆瓣"),
      );
      expect(opt).toBeTruthy();
      opt!.click();
    });
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("按中文名与年份在豆瓣上找同一部"),
    );
    api.updateLibrary.mockResolvedValue(library);
    q("ml-lib-save")!.click();
    await vi.waitFor(() => expect(api.updateLibrary).toHaveBeenCalledTimes(1));
    expect(api.updateLibrary.mock.calls[0]![1]).toMatchObject({
      scrape_items: ["nfo", "fanart", "season_poster", "thumb", "logo"],
      scrape_image_lang: "en",
      scrape_source: "douban",
    });

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
    q("ml-watch-enabled-4")!.click();
    await vi.waitFor(() =>
      expect(api.updateWatchDir).toHaveBeenCalledWith(4, {
        path: "/media/incoming",
        enabled: false,
      }),
    );

    ui.confirm.mockResolvedValue("confirm");
    api.deleteWatchDir.mockResolvedValue({ ok: true });
    q("ml-watch-del-4")!.click();
    await vi.waitFor(() => expect(api.deleteWatchDir).toHaveBeenCalledWith(4));
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
