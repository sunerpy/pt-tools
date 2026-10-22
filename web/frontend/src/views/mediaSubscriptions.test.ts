// @vitest-environment happy-dom
/*
 * 钉子：追剧与订阅页。列表写明进度与最近一次的结果，总开关关着时写明；追剧画每一集的格子、写明定时与版本锁；
 * 立即搜索、暂停、确认、删除；详情列出分集与种子，点一集可以跳过、换版本，集号规则与运行记录；修改追剧只发接口认的字段；
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
  episodes: vi.fn(),
  skipEpisode: vi.fn(),
  unlockEpisode: vi.fn(),
  runs: vi.fn(async (): Promise<{ items: unknown[]; total: number }> => ({ items: [], total: 0 })),
  episodeRules: vi.fn(async (): Promise<unknown[]> => []),
  saveEpisodeRule: vi.fn(),
  deleteEpisodeRule: vi.fn(),
}));
const organize = vi.hoisted(() => ({
  libraries: vi.fn(async () => [{ id: 4, name: "剧集库", kind: "tv" }]),
}));
const dl = vi.hoisted(() => ({ list: vi.fn() }));
const sites = vi.hoisted(() => ({ list: vi.fn() }));
const notifications = vi.hoisted(() => ({ list: vi.fn() }));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
  warning: vi.fn(),
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
    mediaApi: { ...real.mediaApi, settings: vi.fn(async () => ({ image_base_url: "" })) },
    organizeApi: organize,
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
      warning: ui.warning,
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

// 追剧：按播出时间，锁住了 HHWEB 1080p，第 5 集在等这个版本
const follow = {
  ...base,
  id: 3,
  media_type: "tv",
  tmdb_id: 231620,
  season: 4,
  title: "现在就出发",
  year: 2023,
  status: "active",
  source: "manual",
  message: "推送了第 3 集",
  next_search_at: "2026-10-22T14:00:00Z",
  total_episodes: 6,
  schedule: { kind: "airtime", at: "20:00" },
  lock_version: true,
  lock_group: "HHWEB",
  lock_resolution: "1080p",
  start_episode: 0,
  follow_next_season: true,
  library_id: 4,
  progress: {
    total: 6,
    required: 6,
    aired: 5,
    in_library: 2,
    downloading: 1,
    waiting: 1,
    skipped: 0,
    missing: [4, 5],
    states: ["library", "library", "downloading", "missing", "waiting_version", "upcoming"],
  },
};

async function mountPage(
  opts: { listFail?: boolean; settingsFail?: boolean; subs?: unknown[] } = {},
) {
  if (opts.settingsFail) api.settings.mockRejectedValue(new Error("读取设置失败"));
  else api.settings.mockResolvedValue(settings);
  if (opts.listFail) api.list.mockRejectedValue(new Error("读取订阅失败"));
  else api.list.mockResolvedValue(opts.subs ?? [movie, tv]);
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
          subtitle: "",
          score: 5300,
          episode: 2,
          episode_end: 2,
          complete: false,
          size_bytes: 1,
          downloader_id: 1,
          has_hr: false,
          adopted: false,
          status: "downloading",
          message: "",
          created_at: "2026-10-08T10:00:00Z",
        },
      ],
    });
    q("ms-detail-2")!.click();
    await vi.waitFor(() => expect(q("sub-detail-episodes")).not.toBeNull());
    const eps = q("sub-detail-episodes")!.textContent ?? "";
    expect(eps, "图例写明各种状态的集数").toContain("已入库 1");
    expect(eps).toContain("还没播 1");
    expect(q("sub-ep-1")!.getAttribute("aria-label")).toBe("E01 · 未来 · 2025-04-13 · 已入库");
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

  it("设置没读到时不能保存，避免用默认值盖掉真正的设置", async () => {
    await mountPage({ settingsFail: true });
    expect(document.body.textContent).toContain("设置没读到");
    const save = q("ms-save-settings") as HTMLButtonElement;
    expect(save.disabled).toBe(true);
    save.click();
    await flush();
    expect(api.saveSettings).not.toHaveBeenCalled();
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
        id: 3,
        source_id: 5,
        douban_id: "3",
        title: "奥本海默",
        year: 2023,
        media_type: "movie",
        tmdb_id: 872585,
        subscription_id: 0,
        status: "removed",
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
    expect(q("douban-items")!.textContent).toContain("订阅已删除");
  });

  it("追剧：列表画每一集的格子，写明定时与版本锁", async () => {
    await mountPage({ subs: [follow, movie] });
    const strip = q("ms-strip-3")!;
    expect(strip.querySelectorAll(".ms-strip__cell")).toHaveLength(6);
    expect(strip.querySelector(".ms-strip__cell--waiting_version")).not.toBeNull();
    expect(q("ms-strip-1"), "电影没有格子").toBeNull();
    const table = q("ms-table")!.textContent ?? "";
    expect(table).toContain("已入库 2/6 · 下载中 1 · 缺 2（等版本 1） · 1 集还没播");
    expect(q("ms-follow-3")!.textContent).toContain(
      "按播出时间（20:00 播出，2 小时后开始搜） · 版本锁 HHWEB 1080p",
    );
    expect(q("ms-follow-1"), "电影没有追剧的设置").toBeNull();
  });

  it("追剧详情：点一集跳过、换版本；集号规则；运行记录", async () => {
    await mountPage({ subs: [follow] });
    api.get.mockResolvedValue({
      ...follow,
      progress: {
        ...follow.progress,
        episodes: [
          {
            number: 1,
            name: "第1期上",
            air_date: "2026-10-01",
            state: "library",
            source: "subscription",
            version: "HHWEB 1080p",
          },
          { number: 4, name: "第2期下", air_date: "2026-10-08", state: "missing" },
          { number: 5, name: "第3期上", air_date: "2026-10-15", state: "waiting_version" },
        ],
      },
      torrent_list: [],
    });
    api.episodeRules.mockResolvedValue([
      {
        id: 1,
        subscription_id: 3,
        group: "PTerWEB",
        mode: "period_part",
        offset: 0,
        source: "manual",
        note: "",
        created_at: "",
        updated_at: "",
      },
    ]);
    const run = (id: number, summary: string) => ({
      id,
      subscription_id: 3,
      trigger: "schedule",
      started_at: "2026-10-21T12:05:00Z",
      status: "ok",
      summary,
      steps: [
        { kind: "search", text: "在 2 个站点搜到 5 个" },
        { kind: "reject", text: "Natural.High.S04E05.2160p：不是锁住的版本" },
      ],
      steps_truncated: false,
    });
    api.runs
      .mockResolvedValueOnce({ items: [run(11, "推送了第 3 集")], total: 21 })
      .mockResolvedValueOnce({ items: [run(10, "没有新的")], total: 21 });
    q("ms-detail-3")!.click();
    await vi.waitFor(() => expect(q("sub-ep-5")).not.toBeNull());
    expect(q("sub-detail-follow")!.textContent).toContain("版本锁 HHWEB 1080p");
    expect(q("sub-detail-follow")!.textContent).toContain("追完以后追下一季");

    // 在等锁住的版本的那一集：可以跳过，也可以换版本
    q("sub-ep-5")!.click();
    await flush();
    expect(q("sub-ep-detail")!.textContent).toContain("等版本");
    api.unlockEpisode.mockResolvedValue([]);
    q("sub-ep-unlock")!.click();
    await vi.waitFor(() => expect(api.unlockEpisode).toHaveBeenCalledWith(3, 5, true));
    await vi.waitFor(() =>
      expect(ui.success).toHaveBeenCalledWith("第 5 集不再受版本锁限制，会尽快搜一次"),
    );
    // 列表重新读进度
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    q("sub-ep-4")!.click();
    await flush();
    api.skipEpisode.mockResolvedValue([]);
    q("sub-ep-skip")!.click();
    await vi.waitFor(() => expect(api.skipEpisode).toHaveBeenCalledWith(3, 4, true));
    // 已入库的那一集不能跳过
    q("sub-ep-1")!.click();
    await flush();
    expect(q("sub-ep-skip")).toBeNull();
    expect(q("sub-ep-detail")!.textContent).toContain("版本 HHWEB 1080p");

    // 集号规则：删一条、加一条
    await vi.waitFor(() => expect(q("sub-detail-rules")!.textContent).toContain("PTerWEB"));
    expect(q("sub-detail-rules")!.textContent).toContain("按第几期与上下");
    api.deleteEpisodeRule.mockResolvedValue([]);
    q("sub-rule-del-1")!.click();
    await vi.waitFor(() => expect(api.deleteEpisodeRule).toHaveBeenCalledWith(3, "PTerWEB"));
    fill("sub-rule-group", " XX ");
    fill("sub-rule-note", "第几期分上下");
    await flush();
    api.saveEpisodeRule.mockResolvedValue([]);
    q("sub-rule-save")!.click();
    await vi.waitFor(() =>
      expect(api.saveEpisodeRule).toHaveBeenCalledWith(3, {
        group: "XX",
        mode: "period_part",
        offset: 0,
        note: "第几期分上下",
      }),
    );

    // 运行记录：点开看经过，再看后面的
    expect(q("sub-detail-runs")!.textContent).toContain("推送了第 3 集");
    expect(q("sub-detail-runs")!.textContent).not.toContain("不是锁住的版本");
    q("sub-run-11")!.click();
    await flush();
    expect(q("sub-detail-runs")!.textContent).toContain("不是锁住的版本");
    expect(q("sub-runs-more")!.textContent).toContain("再看 20 条");
    q("sub-runs-more")!.click();
    await vi.waitFor(() => expect(api.runs).toHaveBeenLastCalledWith(3, { limit: 20, offset: 1 }));
    await vi.waitFor(() => expect(q("sub-detail-runs")!.textContent).toContain("没有新的"));
  });

  it("修改追剧：只发接口认的字段；清掉锁住的版本", async () => {
    await mountPage({ subs: [follow] });
    api.update.mockResolvedValue(follow);
    q("ms-edit-3")!.click();
    await vi.waitFor(() => expect(q("subscribe-dialog")).not.toBeNull());
    await vi.waitFor(() => expect(organize.libraries).toHaveBeenCalled());
    expect(q("sd-locked")!.textContent).toContain("HHWEB 1080p");
    q("sd-clear-lock")!.click();
    await flush();
    expect(q("sd-locked")).toBeNull();
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    expect(api.update.mock.calls[0]).toEqual([
      3,
      {
        profile_id: 0,
        sites: [],
        downloader_id: 0,
        category: "",
        tags: "",
        save_path: "",
        upgrade: false,
        schedule: {
          kind: "airtime",
          at: "20:00",
          air_delay_hours: 0,
          air_retry_hours: 0,
          air_window_days: 0,
        },
        lock_version: true,
        follow_next_season: true,
        library_id: 4,
        start_episode: 0,
        lock_group: "",
        lock_resolution: "",
      },
    ]);
    expect(ui.success).toHaveBeenCalledWith("已保存追剧");
  });

  it("修改追剧：每周没选星期几时不保存", async () => {
    await mountPage({ subs: [{ ...follow, schedule: { kind: "weekly", at: "21:00", days: [] } }] });
    q("ms-edit-3")!.click();
    await vi.waitFor(() => expect(q("sd-weekdays")).not.toBeNull());
    q("sd-save")!.click();
    await flush();
    expect(ui.warning).toHaveBeenCalledWith("选一下星期几");
    expect(api.update).not.toHaveBeenCalled();
    (q("sd-weekdays")!.querySelectorAll("input")[6] as HTMLInputElement).click();
    await flush();
    api.update.mockResolvedValue(follow);
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    expect(api.update.mock.calls[0]![1].schedule).toEqual({
      kind: "weekly",
      at: "21:00",
      days: [6],
    });
  });

  it("读不到时写明原因", async () => {
    await mountPage({ listFail: true });
    expect(q("ms-state")!.textContent).toContain("读取订阅失败");
  });

  it("页面开着时每分钟静默刷新列表；切到别的标签页、开着弹窗时不刷", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    try {
      await mountPage();
      const n = api.list.mock.calls.length;
      await vi.advanceTimersByTimeAsync(60_000);
      expect(api.list.mock.calls.length, "一分钟刷新一次").toBe(n + 1);

      Object.defineProperty(document, "hidden", { configurable: true, get: () => true });
      try {
        await vi.advanceTimersByTimeAsync(60_000);
        expect(api.list.mock.calls.length, "切走了不刷").toBe(n + 1);
      } finally {
        delete (document as unknown as { hidden?: boolean }).hidden;
      }

      api.get.mockResolvedValue({
        ...tv,
        progress: { ...tv.progress, episodes: [] },
        torrent_list: [],
      });
      q("ms-detail-2")!.click();
      await vi.advanceTimersByTimeAsync(60_000);
      expect(api.list.mock.calls.length, "开着详情不刷").toBe(n + 1);
    } finally {
      vi.useRealTimers();
    }
  });
});
