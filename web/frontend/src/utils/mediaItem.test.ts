/*
 * 钉子：条目详情页的文字（画板 04、08）。资料行、胶囊、主创；「你的 NAS 上」三行；集卡的播出日、说明与动作；
 * 收起行；种子行；默认打开哪一季；面包屑从哪里来。
 */
import { describe, expect, it } from "vitest";

import type { ItemDetail, ItemInfo, ItemTorrent, SeasonDetail, SeasonEpisode } from "@/api";

import {
  airDayText,
  collapsedCount,
  collapsedText,
  countryLabel,
  creditsLine,
  daysSinceAir,
  defaultSeason,
  deletableView,
  episodeAction,
  episodeNote,
  episodeStatePill,
  etaText,
  infoPills,
  itemFrom,
  itemLink,
  leftText,
  legendCounts,
  lockText,
  nasLines,
  originalLine,
  pickSubscription,
  runtimeText,
  searchLink,
  seasonTabText,
  seasonTorrents,
  seedingText,
  statusLabel,
  subscriptionLabel,
  todayOf,
  torrentData,
  torrentLine,
  torrentStatePill,
  votesText,
} from "./mediaItem";

const tv: ItemInfo = {
  media_type: "tv",
  tmdb_id: 100088,
  title: "长夜将尽",
  original_title: "The Long Night",
  year: 2025,
  genres: ["悬疑", "犯罪", "剧情", "动作"],
  countries: ["CN", "XX", "US"],
  runtime: 45,
  status: "Returning Series",
  studios: [],
  directors: ["陈默", "王二", "张三"],
  cast: [
    { name: "周屿", order: 0 },
    { name: "林晚晴", order: 1 },
    { name: "沈川", order: 2 },
    { name: "许曼", order: 3 },
    { name: "何青", order: 4 },
  ],
  seasons_count: 2,
  episodes_count: 24,
};

function detail(over: Partial<ItemDetail> = {}): ItemDetail {
  return {
    item: tv,
    seasons: [],
    nas: {
      library: { seasons: [], files: 0, size: 0, libraries: [] },
      seeding: { count: 0, hr_pending: 0 },
    },
    subscriptions: [],
    torrents: [],
    recommendations: [],
    errors: [],
    ...over,
  };
}

describe("资料", () => {
  it("原名一行、胶囊、主创、评分人数", () => {
    expect(originalLine(tv)).toBe("The Long Night · 2025 · 2 季 24 集");
    expect(originalLine({ ...tv, original_title: "长夜将尽", episodes_count: 0 })).toBe(
      "2025 · 2 季",
    );
    const movie: ItemInfo = { ...tv, media_type: "movie", original_title: "Dune", runtime: 166 };
    expect(originalLine(movie)).toBe("Dune · 2025 · 2 小时 46 分");
    expect(infoPills(tv)).toEqual([
      "剧集",
      "悬疑",
      "犯罪",
      "剧情",
      "中国大陆",
      "XX",
      "每集 45 分钟",
      "连载中",
    ]);
    expect(infoPills({ ...movie, status: "Released", genres: [], countries: [] })).toEqual([
      "电影",
      "已上映",
    ]);
    expect(creditsLine(tv)).toBe("导演 陈默、王二 · 主演 周屿、林晚晴、沈川、许曼");
    expect(creditsLine({ ...tv, directors: [], cast: [] })).toBe("");
    expect(votesText(1204)).toBe("TMDB 1,204 人评分");
    expect(votesText(0)).toBe("TMDB 评分");
  });

  it("状态、国家、片长", () => {
    expect(statusLabel("tv", "Ended")).toBe("已完结");
    expect(statusLabel("movie", "Rumored")).toBe("计划中");
    expect(statusLabel("tv", "Something")).toBe("Something");
    expect(statusLabel("tv", undefined)).toBe("");
    expect(countryLabel("JP")).toBe("日本");
    expect(runtimeText("movie", 59)).toBe("59 分钟");
    expect(runtimeText("movie", 120)).toBe("2 小时");
    expect(runtimeText("tv", 0)).toBe("");
  });
});

describe("你的 NAS 上", () => {
  it("剧集：库里每季几集、做种与 H&R、追剧与版本锁", () => {
    const d = detail({
      nas: {
        library: {
          seasons: [
            { n: 1, have: 12, total: 12 },
            { n: 2, have: 6, total: 12 },
          ],
          files: 18,
          size: 82.6 * 1024 ** 3,
          libraries: ["剧集库"],
        },
        seeding: { count: 4, hr_pending: 1, hr_left_seconds: 2 * 86400, lowest_ratio: 0.42 },
      },
      subscriptions: [
        {
          id: 1,
          status: "active",
          season: 2,
          follow: true,
          lock: { group: "HHWEB", resolution: "2160p" },
        },
      ],
    });
    expect(nasLines(d, d.subscriptions[0]!)).toEqual([
      {
        key: "library",
        icon: "folder-open",
        title: "库里 S01 12/12 · S02 6/12",
        desc: "剧集库 · 18 个文件 · 82.6 GB",
      },
      { key: "seeding", icon: "upload", title: "做种 4 个种子", desc: "1 个 H&R 还差 2 天" },
      {
        key: "subscription",
        icon: "bookmark",
        title: "追剧中 · 第 2 季",
        desc: "版本锁 HHWEB 2160p",
      },
    ]);
  });

  it("没有的时候照样列：库里没有、没有做种、没有追剧；电影写版本", () => {
    expect(nasLines(detail(), null).map((l) => [l.title, l.desc])).toEqual([
      ["库里没有", ""],
      ["没有做种", ""],
      ["没有追剧", ""],
    ]);
    const movie = detail({
      item: { ...tv, media_type: "movie" },
      nas: {
        library: { seasons: [], files: 1, size: 0, libraries: [], version: "2160p FRDS" },
        seeding: { count: 1, hr_pending: 0, lowest_ratio: 2 },
      },
    });
    const lines = nasLines(movie, { id: 3, status: "paused", season: 0, follow: false });
    expect(lines[0]!.title).toBe("库里有 · 2160p FRDS");
    expect(lines[0]!.desc).toBe("1 个文件");
    expect(lines[1]!.desc).toBe("分享率最低 2.00");
    expect(lines[2]!.title).toBe("已暂停");
    expect(lines[2]!.desc).toBe("没开版本锁");
    expect(
      nasLines(
        { ...movie, nas: { ...movie.nas, library: { ...movie.nas.library, version: "" } } },
        null,
      )[0]!.title,
    ).toBe("库里有");
    expect(nasLines({ ...movie, item: { ...movie.item } }, null)[2]!.title).toBe("没有订阅");
  });

  it("订阅的说法与选哪一个", () => {
    expect(subscriptionLabel("tv", { id: 1, status: "active", season: 2, follow: false })).toBe(
      "订阅中 · 第 2 季",
    );
    expect(subscriptionLabel("tv", { id: 1, status: "done", season: 1, follow: true })).toBe(
      "已完成 · 第 1 季",
    );
    expect(subscriptionLabel("movie", { id: 1, status: "pending", season: 0, follow: false })).toBe(
      "待确认",
    );
    expect(subscriptionLabel("movie", { id: 1, status: "active", season: 0, follow: false })).toBe(
      "订阅中",
    );
    expect(lockText(undefined)).toBe("");
    expect(lockText({ group: "", resolution: "" })).toBe("");
    const subs = [
      { id: 2, status: "active" as const, season: 3, follow: true },
      { id: 1, status: "done" as const, season: 1, follow: true },
    ];
    expect(pickSubscription(subs, 1)?.id).toBe(1);
    expect(pickSubscription(subs, 2)?.id, "这一季没订阅时用最新的一季").toBe(2);
    expect(pickSubscription([], 1)).toBeNull();
  });
});

describe("季与集", () => {
  const now = new Date(2026, 9, 26, 12, 0, 0);
  const season: SeasonDetail = {
    tmdb_id: 1,
    season: 2,
    episodes: [],
    errors: [],
    subscription: {
      id: 1,
      status: "active",
      follow: true,
      airtime: true,
      search_delay_hours: 3,
      lock: { group: "HHWEB", resolution: "2160p" },
    },
  };

  it("播出日：日期加星期、今天写时刻、最后一集写季终", () => {
    expect(airDayText({ episode: 7, air_date: "2026-10-26", state: "missing" }, now, false)).toBe(
      "今天",
    );
    const at = new Date(2026, 9, 26, 20, 0, 0).toISOString();
    expect(
      airDayText({ episode: 7, air_date: "2026-10-26", air_at: at, state: "upcoming" }, now, false),
    ).toBe("今天 20:00");
    expect(airDayText({ episode: 8, air_date: "2026-10-27", state: "upcoming" }, now, false)).toBe(
      "10-27 周二",
    );
    expect(airDayText({ episode: 12, air_date: "2026-11-10", state: "upcoming" }, now, true)).toBe(
      "11-10 周二 · 季终",
    );
    const later = new Date(2026, 10, 3, 21, 30).toISOString();
    expect(
      airDayText(
        { episode: 9, air_date: "2026-11-03", air_at: later, state: "upcoming" },
        now,
        false,
      ),
    ).toBe("11-03 周二 21:30");
    expect(airDayText({ episode: 13, state: "upcoming" }, now, false)).toBe("播出日期未定");
    expect(todayOf(now)).toBe("2026-10-26");
  });

  it("说明与动作", () => {
    const ep = (e: Partial<SeasonEpisode>): SeasonEpisode => ({
      episode: 1,
      state: "library",
      ...e,
    });
    expect(
      episodeNote(ep({ version: "2160p HHWEB", size: 6.8 * 1024 ** 3 }), season, now, false),
    ).toBe("2160p HHWEB · 6.8 GB");
    expect(
      episodeNote(
        ep({ state: "downloading", percent: 63.4, eta_seconds: 720 }),
        season,
        now,
        false,
      ),
    ).toBe("63% · 还要 12 分钟");
    expect(episodeNote(ep({ state: "downloading", percent: 10 }), season, now, false)).toBe("10%");
    expect(episodeNote(ep({ state: "downloading", site: "hdsky" }), season, now, false)).toBe(
      "在 hdsky 下载",
    );
    expect(episodeNote(ep({ state: "downloading" }), season, now, false)).toBe("");
    expect(episodeNote(ep({ state: "waiting_version" }), season, now, false)).toBe(
      "锁定 HHWEB 2160p",
    );
    expect(
      episodeNote(
        ep({ state: "waiting_version" }),
        { ...season, subscription: undefined },
        now,
        false,
      ),
    ).toBe("");
    expect(episodeNote(ep({ state: "missing", air_date: "2026-10-23" }), season, now, false)).toBe(
      "播出 3 天，还没找到",
    );
    expect(episodeNote(ep({ state: "missing", air_date: "2026-10-26" }), season, now, false)).toBe(
      "今天播出，还没找到",
    );
    expect(episodeNote(ep({ state: "upcoming" }), season, now, true)).toBe("播出后 3 小时开始搜");
    expect(episodeNote(ep({ state: "upcoming" }), season, now, false), "只写在下一集上").toBe("");
    expect(
      episodeNote(
        ep({ state: "upcoming" }),
        { ...season, subscription: { ...season.subscription!, search_delay_hours: 0 } },
        now,
        true,
      ),
    ).toBe("播出后 2 小时开始搜");
    expect(
      episodeNote(
        ep({ state: "upcoming" }),
        { ...season, subscription: { ...season.subscription!, airtime: false } },
        now,
        true,
      ),
    ).toBe("");
    expect(episodeNote(ep({ state: "skipped" }), season, now, false)).toBe("");
    expect(episodeAction("waiting_version")).toBe("versions");
    expect(episodeAction("missing")).toBe("search");
    expect(episodeAction("library")).toBe("");
    expect(daysSinceAir(undefined, now)).toBe(0);
    expect(daysSinceAir("2026-11-01", now)).toBe(0);
  });

  it("状态胶囊与图例", () => {
    expect(episodeStatePill("library")).toEqual({ label: "已入库", tone: "ok" });
    expect(episodeStatePill("skipped").label).toBe("不追");
    expect(episodeStatePill("odd")).toEqual({ label: "odd", tone: "neutral" });
    const eps: SeasonEpisode[] = [
      { episode: 1, state: "library" },
      { episode: 2, state: "library" },
      { episode: 3, state: "downloading" },
      { episode: 4, state: "upcoming" },
    ];
    expect(legendCounts(eps)).toEqual([
      { state: "library", label: "已入库", count: 2 },
      { state: "downloading", label: "下载中", count: 1 },
      { state: "upcoming", label: "还没播", count: 1 },
    ]);
  });

  it("收起开头连续入库的集：手机超过 3 集就收，桌面一季超过 24 集才收", () => {
    const lib = (n: number): SeasonEpisode[] =>
      Array.from({ length: n }, (_, i) => ({ episode: i + 1, state: "library" as const }));
    const four = [...lib(4), { episode: 5, state: "upcoming" as const }];
    expect(collapsedCount(four, true)).toBe(4);
    expect(collapsedCount(lib(3), true)).toBe(0);
    expect(collapsedCount(four, false)).toBe(0);
    const long = [
      ...lib(20),
      ...Array.from({ length: 6 }, (_, i) => ({ episode: 21 + i, state: "upcoming" as const })),
    ];
    expect(collapsedCount(long, false)).toBe(20);
    expect(collapsedCount(lib(24), false), "一季 24 集不收").toBe(0);
    expect(collapsedCount(lib(25), false), "25 集才收").toBe(25);
    expect(collapsedCount([{ episode: 1, state: "missing" }, ...lib(30)], false)).toBe(0);
    expect(collapsedText(long, 20)).toBe("E01–E20 · 20 集都已入库");
    expect(collapsedText(long, 1)).toBe("E01 · 1 集都已入库");
    expect(collapsedText(long, 0)).toBe("");
  });

  it("季 tab 与默认打开哪一季", () => {
    expect(seasonTabText({ n: 2, episode_count: 12, have: 6 })).toEqual({
      name: "第 2 季",
      count: "6/12",
    });
    expect(seasonTabText({ n: 0, episode_count: 0, have: 0 })).toEqual({
      name: "特别篇",
      count: "",
    });
    const seasons = [
      { n: 1, episode_count: 12, have: 12 },
      { n: 2, episode_count: 12, have: 6 },
      { n: 3, episode_count: 0, have: 0 },
      { n: 0, episode_count: 2, have: 0 },
    ];
    expect(defaultSeason(detail({ seasons }), 1)).toBe(1);
    expect(defaultSeason(detail({ seasons }), 9), "给的季不存在").toBe(2);
    expect(
      defaultSeason(
        detail({ seasons, subscriptions: [{ id: 1, status: "active", season: 3, follow: true }] }),
        0,
      ),
    ).toBe(3);
    expect(defaultSeason(detail({ seasons: [{ n: 0, episode_count: 2, have: 0 }] }), 0)).toBe(0);
    expect(defaultSeason(detail({ seasons: [{ n: 1, episode_count: 0, have: 0 }] }), 0)).toBe(1);
    expect(defaultSeason(detail({ seasons: [] }), 0)).toBe(1);
    expect(defaultSeason(detail({ item: { ...tv, media_type: "movie" } }), 2)).toBe(0);
  });
});

describe("种子", () => {
  const t = (over: Partial<ItemTorrent>): ItemTorrent => ({
    site: "hdsky",
    title: "x",
    season: 2,
    episodes: "E01–E04",
    version: "2160p HHWEB",
    size: 27.3 * 1024 ** 3,
    downloader_id: 1,
    downloader: "qb",
    hash: "h",
    state: "seeding",
    percent: 100,
    ratio: 1.31,
    seeding_seconds: 18 * 86400,
    hr: false,
    deletable: true,
    reason: "可以删",
    ...over,
  });

  it("种子那一列与数据那一列", () => {
    expect(torrentLine(t({}))).toBe("S02E01–E04 · 2160p HHWEB · 27.3 GB");
    expect(torrentLine(t({ episodes: "整季" }))).toBe("S02 整季 · 2160p HHWEB · 27.3 GB");
    expect(torrentLine(t({}), { version: false })).toBe("S02E01–E04 · 27.3 GB");
    expect(
      torrentLine(t({ season: 0, episodes: "", version: "", size: 0, title: "Dune.2024" })),
    ).toBe("Dune.2024");
    expect(torrentLine(t({ season: 0, episodes: "整季" }))).toBe("整季 · 2160p HHWEB · 27.3 GB");
    expect(torrentData(t({}))).toBe("分享率 1.31 · 做种 18 天");
    expect(torrentData(t({}), true)).toBe("分享率 1.31 · 18 天");
    expect(torrentData(t({ seeding_seconds: 0 }))).toBe("分享率 1.31");
    expect(torrentData(t({ state: "downloading", percent: 63.9, eta_seconds: 720 }))).toBe(
      "63% · 还要 12 分钟",
    );
    expect(torrentData(t({ state: "downloading", percent: 5 }))).toBe("5%");
    expect(torrentData(t({ state: "unknown" }))).toBe("下载器没读到");
  });

  it("状态与能不能删", () => {
    expect(torrentStatePill("seeding")).toEqual({ label: "做种中", tone: "ok" });
    expect(torrentStatePill("error").tone).toBe("dang");
    expect(torrentStatePill("odd")).toEqual({ label: "odd", tone: "neutral" });
    expect(deletableView(t({}))).toEqual({ text: "可以删", warn: false });
    expect(deletableView(t({ deletable: false, reason: "H&R 还差 2 天" }))).toEqual({
      text: "H&R 还差 2 天",
      warn: true,
    });
    expect(deletableView(t({ reason: "" }))).toEqual({ text: "可以删", warn: false });
  });

  it("这一季的种子：剧集按季筛（不知道季的也列），电影全部", () => {
    const list = [t({ season: 1 }), t({ season: 2 }), t({ season: 0 })];
    expect(seasonTorrents(list, "tv", 2)).toHaveLength(2);
    expect(seasonTorrents(list, "movie", 0)).toHaveLength(3);
  });
});

describe("时间与链接", () => {
  it("还差、还要、做种多久", () => {
    expect(leftText(0)).toBe("");
    expect(leftText(60)).toBe("1 小时");
    expect(leftText(36 * 3600)).toBe("2 天");
    expect(etaText(undefined)).toBe("");
    expect(etaText(30)).toBe("不到 1 分钟");
    expect(etaText(3600)).toBe("1 小时");
    expect(etaText(3900)).toBe("1 小时 5 分钟");
    expect(etaText(3 * 86400)).toBe("3 天");
    expect(seedingText(0)).toBe("");
    expect(seedingText(600)).toBe("1 小时");
    expect(seedingText(5 * 3600)).toBe("5 小时");
  });

  it("从哪里来与地址", () => {
    expect(itemFrom("history")).toBe("history");
    expect(itemFrom("nope")).toBe("explore");
    expect(itemFrom(undefined)).toBe("explore");
    expect(itemLink("tv", 5, "subscriptions", 2)).toEqual({
      path: "/media/item/tv/5",
      query: { from: "subscriptions", season: "2" },
    });
    expect(itemLink("movie", 5, "explore", 2)).toEqual({
      path: "/media/item/movie/5",
      query: { from: "explore" },
    });
    expect(searchLink("tv", 5, 3)).toEqual({
      path: "/search",
      query: { media_type: "tv", tmdb_id: "5", season: "3" },
    });
    expect(searchLink("movie", 5, 3)).toEqual({
      path: "/search",
      query: { media_type: "movie", tmdb_id: "5" },
    });
  });
});
