/*
 * 钉子：首页（媒体中心 M8，画板 02、07）的文字。今天播出卡按台账状态写状态牌、副行与动作；继续观看、最近入库、
 * 这周还要播的副行；速度与磁盘的说法和颜色（剩不到 5% 或 20 GB 是 dang，同体检）；体检、备份、媒体服务器三条状态行。
 */
import type { HomeDisk, HomeTodayItem, MediaResumeItem } from "@/api";
import { describe, expect, it } from "vitest";
import {
  backupLine,
  diskBar,
  diskText,
  downloaderLine,
  headline,
  healthLine,
  homeItemLink,
  isIssueTodo,
  mobileDisks,
  recentSub,
  remainingText,
  resumeIdentity,
  resumeImage,
  resumeItems,
  resumeSub,
  serverLine,
  sizeText,
  speedHint,
  speedLabels,
  speedParts,
  speedText,
  todayCard,
  todayTap,
  todoIcon,
  todoTotal,
  weekLine,
  whenText,
} from "./home";

const now = new Date("2026-11-04T20:00:00");
const at = (hhmm: string) => new Date(`2026-11-04T${hhmm}:00`).toISOString();

function ep(state: string, extra: Partial<HomeTodayItem> = {}): HomeTodayItem {
  return {
    subscription_id: 3,
    media_type: "tv",
    tmdb_id: 100088,
    title: "长夜将尽",
    season: 2,
    episode: 9,
    air_date: "2026-11-04",
    state: state as HomeTodayItem["state"],
    ...extra,
  };
}

describe("页头的一句话", () => {
  it("今天几集、其中几集入库、几件事要处理", () => {
    const today = [ep("library"), ep("missing"), ep("upcoming")];
    expect(headline(today, 6)).toBe("今天 3 集播出 · 其中 1 集已入库 · 6 件事要处理");
    expect(headline([ep("upcoming"), { ...ep("library"), media_type: "movie" }], 0)).toBe(
      "今天 1 集播出、1 部上映 · 其中 1 部已入库 · 没有要处理的事",
    );
    expect(headline([ep("library"), { ...ep("library"), media_type: "movie" }], null)).toBe(
      "今天 1 集播出、1 部上映 · 其中 1 集、1 部已入库",
    );
    expect(headline([{ ...ep("upcoming"), media_type: "movie" }], 1)).toBe(
      "今天 1 部上映 · 1 件事要处理",
    );
    expect(headline([], null)).toBe("今天没有要播的");
    expect(headline(today, 6, true), "手机顶栏不写入库的那段").toBe("今天 3 集播出 · 6 件事要处理");
    expect(headline(null, 2)).toBe("2 件事要处理");
  });
});

describe("今天播出卡", () => {
  it("还没播：时刻牌写「20:00 播出」，按开始搜的时刻写说明", () => {
    const c = todayCard(ep("upcoming", { air_at: at("20:00"), search_at: at("22:00") }), now);
    expect(c.state).toEqual({ label: "还没播", tone: "neutral" });
    expect(c.time).toBe("20:00 播出");
    expect(c.sub).toBe("S02E09 · 第 9 集");
    expect(c.action).toEqual({ kind: "note", text: "播出后 2 小时开始搜" });
    expect(
      todayCard(ep("upcoming", { air_at: at("20:00"), search_at: at("20:00") }), now).action.text,
    ).toBe("播出就开始搜");
    expect(todayCard(ep("upcoming", { search_at: at("21:15") }), now).action.text).toBe(
      "21:15 开始搜",
    );
    expect(todayCard(ep("upcoming", { search_at: "坏的" }), now).action.text).toBe("按追剧计划搜");
    expect(todayCard(ep("upcoming", { name: "回家" }), now).sub).toBe("S02E09 · 回家");
    expect(todayCard(ep("upcoming"), now).action.text).toBe("按追剧计划搜");
  });

  it("已入库：副行写入库的版本，有打开链接时「播放」", () => {
    const c = todayCard(
      ep("library", {
        air_at: at("19:30"),
        version: "2160p HHWEB",
        open_url: "http://jf/1",
        still_path: "/s.jpg",
      }),
      now,
    );
    expect(c.state.tone).toBe("ok");
    expect(c.time).toBe("19:30");
    expect(c.sub).toBe("S02E09 · 2160p HHWEB");
    expect(c.action).toEqual({ kind: "play", text: "播放", icon: "play", href: "http://jf/1" });
    expect(c.image).toBe("/s.jpg");
    const plain = todayCard(ep("library", { backdrop_path: "/bg.jpg" }), now);
    expect(plain.action).toEqual({ kind: "note", text: "已入库" });
    expect(plain.sub).toBe("S02E09 · 已入库");
    expect(plain.image, "没有剧照用背景图").toBe("/bg.jpg");
    expect(todayCard(ep("library"), now).image).toBeUndefined();
  });

  it("下载中：站点与进度、预计时间", () => {
    const c = todayCard(ep("downloading", { site: "HDSky", percent: 63.4, eta_seconds: 720 }), now);
    expect(c.state).toEqual({ label: "下载中", tone: "info" });
    expect(c.sub).toBe("S02E09 · HDSky 63%");
    expect(c.action.text).toBe("下载中，预计 12 分钟");
    const bare = todayCard(ep("downloading"), now);
    expect(bare.sub).toBe("S02E09 · 下载中");
    expect(bare.action.text).toBe("下载中");
  });

  it("缺：写播出了多久，「搜资源」带上这一季", () => {
    const c = todayCard(ep("missing", { air_at: at("13:30") }), now);
    expect(c.state.tone).toBe("warn");
    expect(c.sub).toBe("S02E09 · 播出 6 小时");
    expect(c.action).toMatchObject({ kind: "search", text: "搜资源" });
    expect(c.action.to).toEqual({
      path: "/search",
      query: { media_type: "tv", tmdb_id: "100088", season: "2" },
    });
    expect(todayCard(ep("missing", { air_at: at("19:40") }), now).sub).toBe("S02E09 · 刚播出");
    expect(todayCard(ep("missing", { air_at: at("21:00") }), now).sub, "还没到播出时刻").toBe(
      "S02E09 · 第 9 集",
    );
  });

  it("等版本：写锁住的版本，「看版本」打开详情滚到这一集", () => {
    const c = todayCard(ep("waiting_version", { lock: "HHWEB 2160p" }), now);
    expect(c.state).toEqual({ label: "等版本", tone: "primary" });
    expect(c.sub).toBe("S02E09 · 锁定 HHWEB 2160p");
    expect(c.action.to).toEqual({
      path: "/media/item/tv/100088",
      query: { from: "home", season: "2", episode: "9" },
    });
    expect(todayCard(ep("waiting_version"), now).sub).toBe("S02E09 · 等锁住的版本");
    expect(todayCard(ep("skipped"), now).action.text).toBe("这一集不追");
  });

  it("电影写「电影 · 今天上映」，入库后写版本", () => {
    const m = {
      ...ep("upcoming"),
      media_type: "movie" as const,
      season: undefined,
      episode: undefined,
    };
    expect(todayCard(m, now).sub).toBe("电影 · 今天上映");
    expect(todayCard({ ...m, state: "library", version: "2160p" }, now).sub).toBe("电影 · 2160p");
    expect(todayCard({ ...m, state: "missing" }, now).action.to).toEqual({
      path: "/search",
      query: { media_type: "movie", tmdb_id: "100088" },
    });
  });

  it("手机上整张卡点了去哪", () => {
    expect(todayTap(ep("library", { open_url: "http://jf/1" }))).toEqual({ href: "http://jf/1" });
    expect(todayTap(ep("library")).to).toEqual({
      path: "/media/item/tv/100088",
      query: { from: "home", season: "2", episode: "9" },
    });
    expect(todayTap(ep("missing")).to).toMatchObject({ path: "/search" });
    expect(todayTap(ep("downloading")).to).toMatchObject({ path: "/media/item/tv/100088" });
  });

  it("详情的地址：电影不带季与集", () => {
    expect(homeItemLink("movie", 603, 0, 3)).toEqual({
      path: "/media/item/movie/603",
      query: { from: "home" },
    });
    expect(homeItemLink("tv", 1, 2)).toEqual({
      path: "/media/item/tv/1",
      query: { from: "home", season: "2" },
    });
  });
});

describe("继续观看与最近入库", () => {
  const item = (extra: Partial<MediaResumeItem> = {}): MediaResumeItem => ({
    server_id: 1,
    item_id: "x",
    media_type: "tv",
    title: "长夜将尽",
    season: 1,
    episode: 13,
    percent: 40,
    remaining_seconds: 18 * 60,
    ...extra,
  });

  it("还剩多久", () => {
    expect(remainingText(18 * 60)).toBe("18 分钟");
    expect(remainingText(72 * 60)).toBe("1 小时 12 分");
    expect(remainingText(3600)).toBe("1 小时");
    expect(remainingText(10)).toBe("1 分钟");
    expect(remainingText(0)).toBe("");
  });

  it("副行：剧集带集号，电影只写还剩多久；不知道还剩多久时写看了多少", () => {
    expect(resumeSub(item())).toBe("S01E13 · 还剩 18 分钟");
    expect(
      resumeSub(item({ media_type: "movie", season: undefined, remaining_seconds: 72 * 60 })),
    ).toBe("还剩 1 小时 12 分");
    expect(resumeSub(item({ remaining_seconds: undefined, percent: 41.6 }))).toBe(
      "S01E13 · 看了 42%",
    );
  });

  it("图：剧集用剧照，电影用背景图，都没有时用海报", () => {
    expect(resumeImage(item({ still_path: "/s.jpg", backdrop_path: "/b.jpg" }))).toEqual({
      path: "/s.jpg",
      poster: false,
    });
    expect(
      resumeImage(item({ media_type: "movie", still_path: "/s.jpg", backdrop_path: "/b.jpg" })),
    ).toEqual({ path: "/b.jpg", poster: false });
    expect(resumeImage(item({ poster_path: "/p.jpg" }))).toEqual({ path: "/p.jpg", poster: true });
    expect(resumeImage(item())).toBeNull();
  });

  it("摊平与身份 chip", () => {
    const groups = [
      { server_id: 1, name: "Jellyfin", kind: "jellyfin" as const, user: "admin", items: [item()] },
      {
        server_id: 2,
        name: "Emby",
        kind: "emby" as const,
        user: "",
        items: [item({ item_id: "y" })],
      },
    ];
    expect(resumeItems(groups).map((i) => i.item_id)).toEqual(["x", "y"]);
    expect(resumeItems(null)).toEqual([]);
    expect(resumeIdentity(groups)).toBe("Jellyfin · admin");
    expect(resumeIdentity([groups[1]])).toBe("Emby");
    expect(resumeIdentity([])).toBe("");
  });

  it("最近入库的副行", () => {
    expect(recentSub({ media_type: "tv", season: 1, episode: 14 })).toBe("S01E14");
    expect(recentSub({ media_type: "tv", season: 2 })).toBe("第 2 季");
    expect(recentSub({ media_type: "tv" })).toBe("剧集");
    expect(recentSub({ media_type: "movie" })).toBe("电影");
  });
});

describe("这周还要播", () => {
  it("日期、标题与说明", () => {
    const line = weekLine(ep("upcoming", { air_date: "2026-11-06" }));
    expect(line).toEqual({ date: "周五 11-06", title: "长夜将尽 S02E09", note: "已追剧" });
    const movie = weekLine({ ...ep("upcoming", { air_date: "2026-11-07" }), media_type: "movie" });
    expect(movie.title).toBe("长夜将尽 上映");
    expect(movie.note).toBe("已订阅");
    const timed = weekLine(
      ep("upcoming", {
        episode: 10,
        air_date: "2026-11-05",
        air_at: new Date("2026-11-05T21:30:00").toISOString(),
      }),
    );
    expect(timed).toEqual({
      date: "周四 11-05",
      title: "长夜将尽 S02E10",
      note: "21:30 播出 · 已追剧",
    });
    const premiere = weekLine({
      ...ep("upcoming", {
        air_date: "2026-11-07",
        air_at: new Date("2026-11-07T10:00:00").toISOString(),
      }),
      media_type: "movie",
    });
    expect(premiere.note).toBe("10:00 上映 · 已订阅");
  });
});

describe("NAS 状态", () => {
  it("速度与大小", () => {
    expect(speedText(0)).toBe("0 B/s");
    expect(speedText(512)).toBe("512 B/s");
    expect(speedText(3 * 1024 * 1024)).toBe("3.0 MB/s");
    expect(speedText(150 * 1024)).toBe("150 KB/s");
    expect(speedText(Number.NaN)).toBe("0 B/s");
    // 首页网络卡大字只放数字、单位用小字：拆开以后拼回去和 speedText 一样
    expect(speedParts(29.8 * 1024 * 1024)).toEqual({ num: "29.8", unit: "MB/s" });
    expect(speedParts(150 * 1024)).toEqual({ num: "150", unit: "KB/s" });
    expect(speedParts(512)).toEqual({ num: "512", unit: "B/s" });
    expect(speedParts(-1)).toEqual({ num: "0", unit: "B/s" });
    expect(speedParts(2 * 1024 ** 4)).toEqual({ num: "2048", unit: "GB/s" });
    expect(sizeText(1.2 * 1024 ** 4)).toBe("1.2 TB");
    expect(sizeText(850 * 1024 ** 3)).toBe("850 GB");
    expect(sizeText(512)).toBe("512 B");
    expect(sizeText(-1)).toBe("0 B");
  });

  it("速度柱的悬停提示写出没取到的那几拍", () => {
    const s = (answered: number) => ({ at: "", down: 1, up: 1, answered, total: 2 });
    expect(speedHint([])).toBe("");
    expect(speedHint([s(2), s(2)])).toBe("最近 2 拍的总速度（30 秒一拍）");
    expect(speedHint([s(2), s(1)])).toBe("有一拍有 1 台下载器没取到，那一拍只算了连上的");
    expect(speedHint([s(1), s(0)])).toBe("有 2 拍有下载器没取到，那几拍只算了连上的");
  });

  const disk = (extra: Partial<HomeDisk>): HomeDisk => ({
    kind: "download",
    label: "下载盘 · qb",
    path: "/downloads",
    free: 0,
    total: 0,
    low: false,
    ...extra,
  });

  it("磁盘条：已用 85% 以上 warn，剩得太少 dang，不知道总量时不画", () => {
    const tb = 1024 ** 4;
    expect(diskBar(disk({ free: 4 * tb, total: 8 * tb }))).toEqual({
      percent: 50,
      tone: "primary",
    });
    expect(diskBar(disk({ free: 1 * tb, total: 8 * tb }))).toEqual({ percent: 87.5, tone: "warn" });
    expect(diskBar(disk({ free: 0.2 * tb, total: 8 * tb, low: true }))).toEqual({
      percent: 97.5,
      tone: "dang",
    });
    expect(diskBar(disk({ free: 1, low: true }))).toEqual({ percent: null, tone: "dang" });
    expect(diskBar(disk({ free: tb }))).toEqual({ percent: null, tone: "primary" });
    expect(diskText(disk({ free: 1.2 * tb, total: 8 * tb }))).toBe("剩余 1.2 TB / 8.0 TB");
    expect(diskText(disk({ free: 1.2 * tb, total: 8 * tb }), true)).toBe("剩余 1.2 TB");
    expect(diskText(disk({ free: 1.2 * tb }))).toBe("剩余 1.2 TB");
    expect(diskText(disk({ error: "连不上" }))).toBe("取不到：连不上");
  });

  it("手机只列下载盘与媒体库盘各一块（剩余最少的）", () => {
    const list = [
      disk({ label: "a", free: 5 }),
      disk({ label: "b", free: 2 }),
      disk({ label: "c", error: "x" }),
      disk({ kind: "library", label: "lib", free: 9 }),
    ];
    expect(mobileDisks(list).map((d) => d.label)).toEqual(["b", "lib"]);
    expect(
      mobileDisks([disk({ label: "c", error: "x", free: 0 }), disk({ label: "d", free: 7 })]).map(
        (d) => d.label,
      ),
    ).toEqual(["d"]);
    expect(mobileDisks([])).toEqual([]);
  });

  it("速度标签与下载器一行", () => {
    expect(speedLabels({ downloading: 4, seeding: 1284 }, false)).toEqual({
      down: "下载 · 下载中 4",
      up: "上传 · 做种 1,284",
    });
    expect(speedLabels({ downloading: 4, seeding: 1 }, true)).toEqual({ down: "下载", up: "上传" });
    expect(speedLabels(null, false)).toEqual({ down: "下载", up: "上传" });
    const d = {
      id: 1,
      name: "qb",
      type: "qbittorrent",
      online: true,
      down: 3 * 1024 * 1024,
      up: 15.2 * 1024 * 1024,
      downloading: 0,
      seeding: 0,
    };
    expect(downloaderLine(d)).toBe("↓ 3.0 MB/s   ↑ 15.2 MB/s");
    expect(downloaderLine({ ...d, online: false, error: "连不上" })).toBe("离线：连不上");
    expect(downloaderLine({ ...d, online: false })).toBe("离线");
  });

  it("状态行的时刻：今天的只写时刻，别的天带日期", () => {
    expect(whenText("2026-11-04T09:30:00", now)).toBe("09:30");
    expect(whenText("2026-11-03T09:30:00", now)).toBe("11-03 09:30");
    expect(whenText("2025-12-31T09:30:00", now)).toBe("2025-12-31 09:30");
    expect(whenText("坏的", now)).toBe("");
  });

  it("体检那一行：标题写结论，说明写第一个问题", () => {
    expect(healthLine(null, now)).toMatchObject({
      icon: "activity",
      title: "体检：没体检过",
      right: "体检",
      rightKind: "action",
      to: "/system/check",
    });
    const ok = healthLine({ at: "2026-11-04T09:30:00", ok: 12, warn: 0, fail: 0, skip: 2 }, now);
    expect(ok).toMatchObject({
      icon: "circle-check",
      tone: "ok",
      title: "体检：12 项通过",
      desc: "都正常",
      right: "09:30",
      rightKind: "time",
    });
    const bad = healthLine(
      { at: "2026-11-03T09:30:00", ok: 8, warn: 1, fail: 2, skip: 0, problem: "tr：连不上" },
      now,
    );
    expect(bad).toMatchObject({
      icon: "triangle-alert",
      tone: "dang",
      title: "体检：8 项通过，2 项失败、1 项提醒",
      desc: "tr：连不上",
      right: "11-03 09:30",
    });
    const warn = healthLine({ at: "2026-11-04T09:30:00", ok: 13, warn: 1, fail: 0, skip: 0 }, now);
    expect(warn).toMatchObject({
      tone: "warn",
      title: "体检：13 项通过，1 项提醒",
      desc: "看系统体检里的说明",
    });
  });

  it("备份那一行", () => {
    expect(backupLine(null, now)).toMatchObject({ title: "备份", desc: "读不到备份设置" });
    expect(backupLine({ enabled: false, late: false, days: -1 }, now)).toMatchObject({
      title: "备份：每天定时备份关着",
      to: "/global?section=backup",
    });
    expect(
      backupLine({ enabled: true, late: true, days: 3, error: "/backup 没有写权限" }, now),
    ).toMatchObject({
      icon: "triangle-alert",
      tone: "warn",
      title: "备份：3 天没成功",
      desc: "/backup 没有写权限",
    });
    expect(
      backupLine({ enabled: true, late: true, days: 3, last_ok_at: "2026-11-01T04:30:00" }, now)
        .desc,
    ).toBe("最近一次成功在 11-01 04:30");
    expect(
      backupLine({ enabled: true, late: true, days: 0, error: "磁盘满了" }, now),
    ).toMatchObject({
      title: "备份：最近一次失败",
      desc: "磁盘满了",
    });
    expect(backupLine({ enabled: true, late: true, days: -1 }, now).desc).toBe(
      "看备份与恢复里的原因",
    );
    expect(
      backupLine({ enabled: true, late: false, days: 0, last_ok_at: "2026-11-04T04:30:00" }, now),
    ).toMatchObject({ icon: "circle-check", tone: "ok", title: "备份：最近一次 04:30 成功" });
    expect(
      backupLine({ enabled: true, late: false, days: 0, last_at: "2026-11-04T04:30:00" }, now)
        .title,
      "旧版本没记成功时间：没出错的那次当成功",
    ).toBe("备份：最近一次 04:30 成功");
    expect(backupLine({ enabled: true, late: false, days: -1 }, now).title).toBe("备份：没做过");
  });

  it("媒体服务器那一行", () => {
    expect(serverLine([], null, now)).toMatchObject({
      desc: "没有连媒体服务器",
      right: "去设置",
      to: "/media/library",
    });
    const srv = {
      id: 1,
      name: "家里",
      kind: "jellyfin" as const,
      version: "12.2",
      online: true,
      indexed_at: "2026-11-04T14:00:00",
      movies: 1204,
      series: 86,
      episodes: 3000,
      open_url: "http://jf/web/",
    };
    expect(serverLine([srv], 3, now)).toMatchObject({
      title: "Jellyfin 12.2 · 14:00 同步",
      desc: "1,204 部电影 · 86 部剧 · 继续观看 3",
      right: "打开",
      href: "http://jf/web/",
    });
    expect(serverLine([{ ...srv, online: false, error: "超时" }], null, now)).toMatchObject({
      title: "Jellyfin 12.2：连不上",
      desc: "超时",
      tone: "warn",
    });
    expect(serverLine([{ ...srv, index_error: "HTTP 500" }], null, now)).toMatchObject({
      title: "Jellyfin 12.2：上次同步索引失败",
      desc: "HTTP 500",
    });
    expect(
      serverLine(
        [{ ...srv, version: undefined, indexed_at: undefined, open_url: undefined }],
        null,
        now,
      ),
    ).toMatchObject({ title: "Jellyfin", desc: "1,204 部电影 · 86 部剧", right: "" });
    expect(serverLine([{ ...srv, kind: "" as never, version: undefined }], null, now).title).toBe(
      "家里 · 14:00 同步",
    );
  });
});

describe("待处理", () => {
  it("图标、总数与在抽屉里处理的那几类", () => {
    expect(todoIcon("organize_failed")).toBe("circle-x");
    expect(todoIcon("recognize")).toBe("scan");
    expect(todoIcon("waiting_version")).toBe("layers");
    expect(todoIcon("attendance_failed")).toBe("calendar-check");
    expect(todoIcon("login_failed")).toBe("log-in");
    expect(todoIcon("server_removed")).toBe("monitor");
    expect(todoIcon("library_missing")).toBe("folder-open");
    expect(todoIcon("backup")).toBe("archive");
    expect(todoIcon("health")).toBe("activity");
    expect(todoTotal(null)).toBeNull();
    expect(
      todoTotal([
        { kind: "recognize", count: 2, label: "", target: "", tone: "warn" },
        { kind: "backup", count: 1, label: "", target: "", tone: "warn" },
      ]),
    ).toBe(3);
    expect(isIssueTodo({ kind: "server_removed" })).toBe(true);
    expect(isIssueTodo({ kind: "library_missing" })).toBe(true);
    expect(isIssueTodo({ kind: "backup" })).toBe(false);
  });
});
