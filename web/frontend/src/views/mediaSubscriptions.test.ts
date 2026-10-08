// @vitest-environment happy-dom
/*
 * 钉子：订阅页。列表写明进度与最近一次的结果，总开关关着时写明；立即搜索、暂停、确认、删除；详情列出分集与种子；
 * 设置、质量档案、豆瓣来源只发接口认的字段（严格解析）；豆瓣立即拉取写明建了几个订阅。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  settings: vi.fn(),
  saveSettings: vi.fn(),
  profiles: vi.fn(),
  createProfile: vi.fn(),
  updateProfile: vi.fn(),
  deleteProfile: vi.fn(),
  list: vi.fn(),
  get: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  setStatus: vi.fn(),
  search: vi.fn(),
  doubanSources: vi.fn(),
  createDouban: vi.fn(),
  updateDouban: vi.fn(),
  deleteDouban: vi.fn(),
  fetchDouban: vi.fn(),
  doubanItems: vi.fn(),
}));
const dl = vi.hoisted(() => ({ list: vi.fn() }));
const sites = vi.hoisted(() => ({ list: vi.fn() }));
const notifications = vi.hoisted(() => ({ list: vi.fn() }));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  confirm: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    subscribeApi: api,
    downloadersApi: dl,
    sitesApi: sites,
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
      info: ui.info,
      warning: vi.fn(),
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
function fill(id: string, v: string) {
  const el = q(id);
  if (!el) throw new Error(`找不到 ${id}`);
  const i = (el instanceof HTMLInputElement ? el : el.querySelector("input"))!;
  i.value = v;
  i.dispatchEvent(new Event("input"));
}

const settings = {
  enabled: false,
  search_interval_hours: 12,
  search_skip_sites: ["mteam"],
  default_profile_id: 0,
  default_downloader_id: 0,
  notify_channels: [],
  upgrade_old: "keep",
};

const base = {
  tmdb_id: 1,
  original_title: "",
  imdb_id: "",
  poster_path: "",
  profile_id: 0,
  sites: [],
  downloader_id: 0,
  category: "",
  tags: "",
  save_path: "",
  upgrade: false,
  best_score: 0,
  best_title: "",
  douban_id: "",
  total_episodes: 0,
  created_at: "",
  updated_at: "",
  torrents: 0,
};
const movie = {
  ...base,
  id: 1,
  media_type: "movie",
  season: 0,
  title: "沙丘2",
  year: 2024,
  status: "active",
  source: "explore",
  message: "没有找到对得上的资源",
  next_search_at: "2026-10-08T20:00:00Z",
  progress: { total: 1, aired: 1, in_library: 0, downloading: 0, missing: [1] },
};
const tv = {
  ...base,
  id: 2,
  media_type: "tv",
  season: 2,
  title: "最后生还者",
  year: 2025,
  status: "pending",
  source: "douban",
  upgrade: true,
  message: "",
  progress: { total: 3, aired: 2, in_library: 1, downloading: 1, missing: [] },
};
const profile = {
  id: 3,
  name: "4K",
  resolutions: ["2160p"],
  sources: [],
  codecs: [],
  remux: "",
  hdr: "prefer",
  chinese_subs: "",
  free: "",
  groups: [],
  min_size_gb: 0,
  max_size_gb: 0,
  min_seeders: 0,
  exclude_hr: false,
};
const source = {
  id: 5,
  user_id: "qa",
  name: "QA",
  enabled: true,
  confirm: true,
  profile_id: 0,
  failures: 3,
  last_error: "豆瓣返回 HTTP 503",
  abnormal: true,
  subscribed: 2,
  unmatched: 1,
};

async function mountPage(opts: { listFail?: boolean } = {}) {
  api.settings.mockResolvedValue(settings);
  if (opts.listFail) api.list.mockRejectedValue(new Error("读取订阅失败"));
  else api.list.mockResolvedValue([movie, tv]);
  api.profiles.mockResolvedValue({
    items: [profile],
    resolutions: ["2160p", "1080p"],
    sources: ["BluRay"],
    codecs: ["H.265"],
  });
  api.doubanSources.mockResolvedValue([source]);
  dl.list.mockResolvedValue([{ id: 1, name: "qb", enabled: true }]);
  sites.list.mockResolvedValue({ hdsky: { enabled: true }, mteam: { enabled: true } });
  notifications.list.mockResolvedValue([
    { id: 2, channel_type: "telegram", name: "TG", enabled: true },
  ]);
  const Page = (await import("./MediaSubscriptions.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:p(.*)*", component: { render: () => null } }],
  });
  view = mountView(Page, { router });
  await vi.waitFor(() => expect(api.list).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("订阅", () => {
  it("列表：进度、来源与最近一次的结果；总开关关着时写明", async () => {
    await mountPage();
    expect(q("ms-disabled")).not.toBeNull();
    const table = q("ms-table")!.textContent ?? "";
    expect(table).toContain("沙丘2 (2024)");
    expect(table).toContain("最后生还者 (2025) 第 2 季");
    expect(table).toContain("还没下载");
    expect(table).toContain("已入库 1/3 · 下载中 1 · 1 集还没播");
    expect(table).toContain("豆瓣想看");
    expect(table).toContain("洗版");
    expect(table).toContain("待确认");
    expect(table).toContain("没有找到对得上的资源");
    expect(table).toContain("下次搜索");
    expect(api.list).toHaveBeenCalledWith({ status: "", q: "" });
  });

  it("立即搜索、暂停、确认、删除", async () => {
    await mountPage();
    api.search.mockResolvedValue({ message: "下载了 Dune.Part.Two.2024.2160p" });
    q("ms-search-1")!.click();
    await vi.waitFor(() =>
      expect(ui.success).toHaveBeenCalledWith("下载了 Dune.Part.Two.2024.2160p"),
    );
    api.search.mockResolvedValue({ message: "没有找到对得上的资源" });
    q("ms-search-1")!.click();
    await vi.waitFor(() => expect(ui.info).toHaveBeenCalledWith("没有找到对得上的资源"));

    api.setStatus.mockResolvedValue({ ...movie, status: "paused" });
    q("ms-pause-1")!.click();
    await vi.waitFor(() => expect(api.setStatus).toHaveBeenCalledWith(1, "paused"));
    q("ms-confirm-2")!.click();
    await vi.waitFor(() => expect(api.setStatus).toHaveBeenCalledWith(2, "active"));
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("已确认，开始找资源"));

    ui.confirm.mockResolvedValue("confirm");
    api.remove.mockResolvedValue({ ok: true });
    q("ms-del-1")!.click();
    await vi.waitFor(() => expect(api.remove).toHaveBeenCalledWith(1));
    expect(ui.confirm.mock.calls[0]![0]).toContain("库里的文件不动");
  });

  it("详情：分集与下载过的种子", async () => {
    await mountPage();
    api.get.mockResolvedValue({
      ...tv,
      last_search_at: "2026-10-08T10:00:00Z",
      progress: {
        ...tv.progress,
        episodes: [
          { number: 1, name: "未来", air_date: "2025-04-13", state: "library" },
          { number: 2, name: "穿越", air_date: "2025-04-20", state: "downloading" },
          { number: 3, name: "路", air_date: "2026-12-01", state: "upcoming" },
        ],
      },
      torrent_list: [
        {
          id: 7,
          subscription_id: 2,
          site_name: "hdsky",
          torrent_id: "9",
          info_hash: "h",
          title: "The.Last.of.Us.S02E02.2160p",
          score: 5300,
          episode: 2,
          episode_end: 2,
          complete: false,
          size_bytes: 1,
          downloader_id: 1,
          status: "downloading",
          message: "",
          created_at: "2026-10-08T10:00:00Z",
        },
      ],
    });
    q("ms-detail-2")!.click();
    await vi.waitFor(() => expect(q("sub-detail-episodes")).not.toBeNull());
    const eps = q("sub-detail-episodes")!.textContent ?? "";
    expect(eps).toContain("E01");
    expect(eps).toContain("已入库");
    expect(eps).toContain("还没播");
    const ts = q("sub-detail-torrents")!.textContent ?? "";
    expect(ts).toContain("The.Last.of.Us.S02E02.2160p");
    expect(ts).toContain("第 2 集");
    expect(ts).toContain("5300 分");
  });

  it("设置：只发接口认的字段", async () => {
    await mountPage();
    api.saveSettings.mockResolvedValue({ ...settings, enabled: true });
    (q("ms-enabled")!.querySelector("input") ?? q("ms-enabled"))!.click();
    await flush();
    q("ms-save-settings")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(1));
    expect(api.saveSettings.mock.calls[0]![0]).toEqual({ ...settings, enabled: true });
    expect(ui.success).toHaveBeenCalledWith("已保存订阅设置");
  });

  it("质量档案：添加只发输入字段", async () => {
    await mountPage();
    expect(q("ms-profiles")!.textContent).toContain("2160p · HDR优先");
    api.createProfile.mockResolvedValue({ ...profile, id: 4, name: "原盘" });
    const add = [...document.querySelectorAll<HTMLButtonElement>("button")].filter((b) =>
      (b.textContent ?? "").includes("添加"),
    );
    add[0]!.click();
    await vi.waitFor(() => expect(q("profile-dialog")).not.toBeNull());
    fill("qp-name", "原盘");
    await flush();
    q("qp-save")!.click();
    await vi.waitFor(() => expect(api.createProfile).toHaveBeenCalledTimes(1));
    expect(api.createProfile.mock.calls[0]![0]).toEqual({
      name: "原盘",
      resolutions: [],
      sources: [],
      codecs: [],
      remux: "",
      hdr: "",
      chinese_subs: "",
      free: "",
      groups: [],
      min_size_gb: 0,
      max_size_gb: 0,
      min_seeders: 0,
      exclude_hr: false,
    });
  });

  it("豆瓣想看：异常写明原因，立即拉取写明建了几个订阅，条目列表", async () => {
    await mountPage();
    const table = q("ms-douban")!.textContent ?? "";
    expect(table).toContain("异常");
    expect(table).toContain("豆瓣返回 HTTP 503");
    expect(table).toContain("2 个 · 1 个没找到条目");
    api.fetchDouban.mockResolvedValue({ created: 2 });
    q("ms-douban-fetch-5")!.click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("建了 2 个订阅"));
    api.doubanItems.mockResolvedValue([
      {
        id: 1,
        source_id: 5,
        douban_id: "1",
        title: "沙丘2",
        year: 2024,
        media_type: "movie",
        tmdb_id: 1,
        subscription_id: 1,
        status: "subscribed",
        created_at: "",
      },
      {
        id: 2,
        source_id: 5,
        douban_id: "2",
        title: "不存在的电影",
        year: 0,
        media_type: "",
        tmdb_id: 0,
        subscription_id: 0,
        status: "unmatched",
        created_at: "",
      },
    ]);
    q("ms-douban-items-5")!.click();
    await vi.waitFor(() => expect(q("douban-items")!.textContent).toContain("没找到条目"));
    expect(q("douban-items")!.textContent).toContain("沙丘2 (2024)");
  });

  it("读不到时写明原因", async () => {
    await mountPage({ listFail: true });
    expect(q("ms-state")!.textContent).toContain("读取订阅失败");
  });
});
