import { describe, expect, it } from "vitest";

import type { MediaSearchGroup, MediaSearchMember, MediaSearchResult } from "@/api";

import {
  blockedLine,
  blockedMembers,
  buildCards,
  coverPill,
  discountText,
  episodeNote,
  epList,
  hrText,
  lockMismatch,
  memberLine,
  parsedTags,
  scoreCard,
  shortSize,
  spanText,
  summaryText,
} from "./entrySearch";

const member = (
  site: string,
  id: string,
  over: Partial<MediaSearchMember> = {},
): MediaSearchMember => ({
  site,
  torrent_id: id,
  title: `Chang.Ye.S02E0${id}.2160p.WEB-DL-HHWEB`,
  size_bytes: 4.71 * 1024 ** 3,
  seeders: 10,
  leechers: 1,
  discount: "NONE",
  free: false,
  hr: false,
  meta: { resolution: "2160p", source: "WEB-DL", group: "HHWEB" },
  verdict: { score: 90 },
  ...over,
});

const group = (
  id: string,
  members: MediaSearchMember[],
  over: Partial<MediaSearchGroup> = {},
): MediaSearchGroup => ({
  group_id: id,
  members,
  meta: members[0]!.meta,
  verdict: members[0]!.verdict,
  span: { start: 8, end: 8 },
  covers: [],
  downloading: [],
  in_library: false,
  ...over,
});

const result = (
  groups: MediaSearchGroup[],
  over: Partial<MediaSearchResult> = {},
): MediaSearchResult => ({
  item: { media_type: "tv", tmdb_id: 1, season: 2, title: "长夜将尽", total_episodes: 12 },
  subscription: null,
  profile: { id: 1, name: "默认" },
  keywords: ["长夜将尽"],
  episodes: [
    { episode: 7, state: "downloading", site: "HDSky" },
    { episode: 8, state: "waiting_version" },
  ],
  groups,
  total: groups.reduce((n, g) => n + g.members.length, 0),
  blocked_count: 0,
  unmatched_count: 0,
  errors: [],
  ...over,
});

describe("集与说明", () => {
  it("集号连着三集以上写成区间", () => {
    expect(epList([8])).toBe("E08");
    expect(epList([7, 8])).toBe("E07、E08");
    expect(epList([1, 2, 3, 4, 5, 6, 8])).toBe("E01–E06、E08");
    expect(epList([])).toBe("");
  });

  it("每集格子下面那一句：缺的、等版本的、在下载的", () => {
    expect(episodeNote(result([]).episodes)).toBe("缺 E08（等版本）· E07 下载中");
    expect(
      episodeNote([
        { episode: 1, state: "missing" },
        { episode: 2, state: "waiting_version" },
        { episode: 3, state: "library" },
      ]),
    ).toBe("缺 E01 · E02 等版本");
    expect(
      episodeNote([
        { episode: 1, state: "library" },
        { episode: 2, state: "upcoming" },
      ]),
    ).toBe("播出了的都已入库");
  });
});

describe("分组卡片右上角", () => {
  const eps = result([]).episodes;
  it("不符合的写原因；库里都有的写库里都有了；能补上的写集号；在下载的写站点", () => {
    expect(
      coverPill(
        group(
          "a",
          [member("x", "1", { verdict: { score: 0, reject: "分辨率 1080p 不在档案里" } })],
          { verdict: { score: 0, reject: "分辨率 1080p 不在档案里" } },
        ),
        eps,
      ),
    ).toEqual({
      text: "不符合：分辨率 1080p 不在档案里",
      tone: "warn",
      icon: "triangle-alert",
    });
    expect(coverPill(group("a", [member("x", "1")], { in_library: true }), eps)).toEqual({
      text: "库里都有了",
      tone: "neutral",
      dot: true,
    });
    expect(coverPill(group("a", [member("x", "1")], { covers: [8] }), eps)).toEqual({
      text: "能补上 E08",
      tone: "ok",
      dot: true,
    });
    expect(coverPill(group("a", [member("x", "1")], { downloading: [7] }), eps)).toEqual({
      text: "E07 正在下载（HDSky）",
      tone: "info",
      dot: true,
    });
    expect(coverPill(group("a", [member("x", "1")]), eps)).toBeNull();
    expect(
      coverPill(group("a", [member("x", "1")], { span: undefined }), [], {
        state: "downloading",
        site: "OurBits",
      }),
    ).toEqual({
      text: "正在下载（OurBits）",
      tone: "info",
      dot: true,
    });
  });

  it("版本锁：只写对不上的那部分", () => {
    expect(
      lockMismatch(
        { group: "HHWEB", resolution: "2160p" },
        { group: "CHDWEB", resolution: "2160p" },
      ),
    ).toBe("HHWEB");
    expect(
      lockMismatch(
        { group: "HHWEB", resolution: "2160p" },
        { group: "hhweb", resolution: "1080p" },
      ),
    ).toBe("2160p");
    expect(
      lockMismatch(
        { group: "HHWEB", resolution: "2160p" },
        { group: "HHWEB", resolution: "2160p" },
      ),
    ).toBe("");
    expect(lockMismatch(null, { group: "X" })).toBe("");
  });

  it("档案分：通过、不符合、不按档案、库里都有", () => {
    expect(scoreCard({ score: 88 }, false)).toEqual({ value: "88", unit: "档案分", tone: "ok" });
    expect(scoreCard({ score: 0, reject: "x" }, false)).toEqual({
      value: "—",
      unit: "不符合",
      tone: "none",
    });
    expect(scoreCard(null, false)).toEqual({ value: "—", unit: "不按档案", tone: "none" });
    expect(scoreCard({ score: 92 }, true)).toEqual({ value: "92", unit: "档案分", tone: "dim" });
  });

  it("优惠与 H&R", () => {
    expect(
      discountText(
        member("x", "1", { discount: "FREE", free: true, free_left_seconds: 18 * 3600 + 120 }),
      ),
    ).toEqual({ text: "免费 18 时", tone: "ok" });
    expect(
      discountText(member("x", "1", { discount: "FREE", free: true, free_left_seconds: 40 * 60 })),
    ).toEqual({ text: "免费 40 分", tone: "ok" });
    expect(discountText(member("x", "1", { discount: "2XFREE", free: true }))).toEqual({
      text: "2x免费",
      tone: "ok",
    });
    expect(discountText(member("x", "1", { discount: "PERCENT_50" }))).toEqual({
      text: "50%",
      tone: "info",
    });
    expect(discountText(member("x", "1"))).toEqual({ text: "无优惠", tone: "neutral" });
    expect(hrText(member("x", "1", { hr: true, hr_hours: 120 }))).toEqual({
      text: "H&R 120 时",
      warn: true,
    });
    expect(hrText(member("x", "1", { hr: true }))).toEqual({ text: "有 H&R", warn: true });
    expect(hrText(member("x", "1"))).toEqual({ text: "无 H&R", warn: false });
    expect(shortSize(4.71 * 1024 ** 3)).toBe("4.71 GB");
    expect(shortSize(27.6 * 1024 ** 3)).toBe("27.6 GB");
    expect(shortSize(0)).toBe("—");
  });
});

describe("解析标签", () => {
  it("按画板的顺序，分辨率、HDR、中字带颜色", () => {
    expect(
      parsedTags({
        resolution: "2160p",
        source: "WEB-DL",
        platform: "Tencent Video",
        video_codec: "H.265",
        hdr: ["HDR10", "DV"],
        group: "CHDWEB",
        chinese_subs: true,
      }),
    ).toEqual([
      { label: "2160p", tone: "info" },
      { label: "WEB-DL", tone: "neutral" },
      { label: "腾讯视频", tone: "neutral" },
      { label: "H.265", tone: "neutral" },
      { label: "HDR10", tone: "kpi3" },
      { label: "杜比视界", tone: "kpi3" },
      { label: "CHDWEB", tone: "neutral" },
      { label: "中字", tone: "ok" },
    ]);
    expect(parsedTags(undefined)).toEqual([]);
    expect(
      memberLine(
        member("x", "1", {
          seeders: 41,
          discount: "FREE",
          free: true,
          free_left_seconds: 18 * 3600,
        }),
      ),
    ).toBe("做种 41 · 免费 18 时 · 无 H&R");
  });
});

describe("结果卡片：合并、筛选、排序", () => {
  const g1 = group("h:1", [
    member("OurBits", "1", { seeders: 41 }),
    member("CHDBits", "2", { seeders: 23, free: true, discount: "FREE" }),
  ]);
  const g2 = group("h:2", [
    member("HDSky", "3", { seeders: 128, uploaded_at: 300 }),
    member("OurBits", "4", { seeders: 9, uploaded_at: 100 }),
  ]);
  const g3 = group(
    "s:x",
    [
      member("PTTime", "5", {
        seeders: 212,
        size_bytes: 1.38 * 1024 ** 3,
        verdict: { score: 0, reject: "分辨率 1080p 不在档案里" },
        meta: { resolution: "1080p", group: "ADWeb" },
      }),
    ],
    { verdict: { score: 0, reject: "分辨率 1080p 不在档案里" } },
  );
  const res = result([g1, g2, g3]);

  it("默认按档案分：服务端排好的顺序，首选在前", () => {
    const cards = buildCards(res, { merge: true, sort: "score", sel: {}, text: "" });
    expect(cards.map((c) => c.key)).toEqual(["h:1", "h:2", "s:x"]);
    expect(cards[0]!.members.map((m) => m.site)).toEqual(["OurBits", "CHDBits"]);
    expect(cards[0]!.pass).toBe(true);
    expect(cards[2]!.pass).toBe(false);
    expect(summaryText(res, cards, true)).toBe("5 条结果 · 合并成 3 组 · 符合档案 2");
  });

  it("按做种、发布时间排；按发布时间用成员的 uploaded_at", () => {
    expect(
      buildCards(res, { merge: true, sort: "seeders", sel: {}, text: "" }).map((c) => c.key),
    ).toEqual(["s:x", "h:2", "h:1"]);
    expect(buildCards(res, { merge: true, sort: "uploaded", sel: {}, text: "" })[0]!.key).toBe(
      "h:2",
    );
  });

  it("筛选到成员：组里只剩对得上的成员，首选跟着换；一个都不剩的组不显示", () => {
    const cards = buildCards(res, {
      merge: true,
      sort: "score",
      sel: { flags: ["free"] },
      text: "",
    });
    expect(cards.map((c) => c.key)).toEqual(["h:1"]);
    expect(cards[0]!.members.map((m) => m.site)).toEqual(["CHDBits"]);
    expect(summaryText(res, cards, true)).toBe("1 条结果 · 合并成 1 组 · 符合档案 1");
  });

  it("关掉合并：一个成员一张卡，概况不写组数", () => {
    const cards = buildCards(res, { merge: false, sort: "score", sel: {}, text: "" });
    expect(cards.map((c) => c.key)).toEqual([
      "h:1|OurBits|1",
      "h:1|CHDBits|2",
      "h:2|HDSky|3",
      "h:2|OurBits|4",
      "s:x|PTTime|5",
    ]);
    expect(summaryText(res, cards, false)).toBe("5 条结果 · 符合档案 4");
    // 手机上的短写法（画板 09）
    expect(summaryText(res, cards, false, true)).toBe("5 条 · 符合档案 4");
  });

  it("输入框里再写的字按标题筛；点一集只看包含这一集的", () => {
    expect(
      buildCards(res, { merge: true, sort: "score", sel: {}, text: "s02e05" }).map((c) => c.key),
    ).toEqual(["s:x"]);
    const ep = buildCards(res, { merge: true, sort: "score", sel: {}, text: "", episode: 9 });
    expect(ep).toEqual([]);
    expect(
      buildCards(res, { merge: true, sort: "score", sel: {}, text: "", episode: 8 }),
    ).toHaveLength(3);
  });

  it("不按档案时概况不写符合档案", () => {
    const r = result([group("a", [member("x", "1", { verdict: null })], { verdict: null })], {
      profile: null,
    });
    const cards = buildCards(r, { merge: true, sort: "score", sel: {}, text: "" });
    expect(cards[0]!.pass).toBe(true);
    expect(summaryText(r, cards, true)).toBe("1 条结果 · 合并成 1 组");
  });
});

describe("集号范围", () => {
  it("用服务端对过的 span：单集、区间、整季；电影没有 span", () => {
    expect(spanText({ start: 8, end: 8 })).toBe("E08");
    expect(spanText({ start: 1, end: 6 })).toBe("E01–E06");
    expect(spanText({ start: 4 })).toBe("E04");
    expect(spanText({ whole: true })).toBe("整季");
    expect(spanText(undefined)).toBe("");
    expect(spanText({})).toBe("");
  });

  it("「季集」筛选按 span 写：只写期号、解析不出集号的种子也归到它对上的那一集", () => {
    const qi = member("PTTime", "9", {
      title: "现在就出发 第四季 第2期下 1080p-YYWEB",
      meta: { season: 4, resolution: "1080p" },
    });
    const res = result([group("s:pttime:9", [qi], { span: { start: 4, end: 4 } })]);
    expect(
      buildCards(res, { merge: true, sort: "score", sel: { episodes: ["E04"] }, text: "" }),
    ).toHaveLength(1);
    expect(
      buildCards(res, { merge: true, sort: "score", sel: { episodes: ["S04（整季）"] }, text: "" }),
    ).toHaveLength(0);
  });
});

describe("拉黑的结果", () => {
  const base = (): MediaSearchResult =>
    result([
      group("h:a", [member("ourbits", "1"), member("chdbits", "2", { block: { id: 9 } })]),
      group("h:b", [member("pttime", "3", { block: { id: 7, reason: "假种" } })], {
        blocked: true,
      }),
      group("h:c", [member("hdsky", "4")]),
    ]);
  const opts = { merge: true, sort: "score" as const, sel: {}, text: "" };

  it("默认不显示拉黑的成员；全拉黑的组整张卡不显示", () => {
    const cards = buildCards(base(), opts);
    expect(cards.map((c) => c.key)).toEqual(["h:a", "h:c"]);
    expect(cards[0]!.members.map((m) => m.site)).toEqual(["ourbits"]);
    expect(cards.every((c) => !c.blocked)).toBe(true);
  });

  it("显示拉黑的：全拉黑的卡排在最后、标成 blocked，部分拉黑的卡不算", () => {
    const cards = buildCards(base(), { ...opts, showBlocked: true });
    expect(cards.map((c) => [c.key, c.blocked])).toEqual([
      ["h:a", false],
      ["h:c", false],
      ["h:b", true],
    ]);
    expect(cards[0]!.members.map((m) => m.site)).toEqual(["ourbits", "chdbits"]);
    // 关掉合并：一个成员一张卡，拉黑的排最后
    const flat = buildCards(base(), { ...opts, merge: false, showBlocked: true });
    expect(flat.map((c) => c.blocked)).toEqual([false, false, true, true]);
  });

  it("最后一行：一条写标题与原因，没写原因只写「不要这个种子」，多条只写条数，没有时为空", () => {
    const res = base();
    expect(blockedMembers(res).map((m) => m.torrent_id)).toEqual(["2", "3"]);
    expect(blockedLine(res)).toEqual({ count: "已拉黑 2 条", detail: "" });
    res.groups[0]!.members[1]!.block = undefined;
    expect(blockedLine(res)).toEqual({
      count: "已拉黑 1 条",
      detail: "：Chang.Ye.S02E03.2160p.WEB-DL-HHWEB（不要这个种子：假种）",
    });
    res.groups[1]!.members[0]!.block = { id: 7 };
    expect(blockedLine(res)?.detail).toBe("：Chang.Ye.S02E03.2160p.WEB-DL-HHWEB（不要这个种子）");
    res.groups[1]!.members[0]!.block = undefined;
    expect(blockedLine(res)).toBeNull();
  });
});
