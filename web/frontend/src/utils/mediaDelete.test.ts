/*
 * 钉子（媒体中心 M7）：删除一部片的文字与勾选。标题；「处理」的范围；默认勾哪些（大图、种子行、处理三种打开方式）；
 * 库文件那一行的说明；种子行的标题与说明；共用数据只删种子；请求体（受保护的要确认、按下载器编号与 hash）；底部的摘要。
 */
import { describe, expect, it } from "vitest";

import type { DeleteFile, DeletePreview, DeleteTorrent, MediaIssue } from "@/api";

import {
  canDelete,
  defaultChoice,
  deleteRequest,
  deleteSummary,
  deleteTitle,
  deleteTorrentSub,
  deleteTorrentTitle,
  existingLine,
  fileGroups,
  fileMeta,
  filesView,
  issueLine,
  issueSelector,
  pausable,
  protectedText,
  resultText,
  scopeText,
  sharedBy,
  stepKindLabel,
  subscriptionOptions,
  torrentKey,
} from "./mediaDelete";

const GB = 1024 ** 3;
const sel = { media_type: "tv" as const, tmdb_id: 100088, season: 2 };

const file = (id: number, over: Partial<DeleteFile> = {}): DeleteFile => ({
  id,
  path: `/media/tv/长夜将尽 (2025)/Season 02/E0${id}.mkv`,
  library: "剧集库",
  mode: "hardlink",
  existing: false,
  size: 6.8 * GB,
  season: 2,
  episode: id,
  episode_end: 0,
  state: "ours",
  deletable: true,
  ...over,
});

const torrent = (over: Partial<DeleteTorrent>): DeleteTorrent => ({
  site: "OurBits",
  title: "The.Long.Night.S02E01-E04.2160p-HHWEB",
  season: 2,
  episodes: "E01–E04",
  version: "2160p HHWEB",
  size: 27.3 * GB,
  downloader_id: 1,
  downloader: "qb",
  hash: "AAA",
  state: "seeding",
  percent: 100,
  ratio: 1.31,
  seeding_seconds: 18 * 86400,
  hr: true,
  hr_hours: 72,
  deletable: true,
  reason: "H&R 已满足",
  covers_more: false,
  shares: [],
  ...over,
});

const ok = torrent({});
const hr = torrent({
  site: "HDSky",
  episodes: "E05–E06",
  downloader_id: 2,
  downloader: "tr",
  hash: "bbb",
  deletable: false,
  reason: "H&R 还差 2 天",
  rule: "hr",
  why: "H&R 要求做种 7 天，还差 2 天",
  protection: "H&R：要求做种 168 小时，已做 120.0 小时",
});
const dl = torrent({
  site: "HDSky",
  episodes: "E07",
  hash: "ccc",
  state: "downloading",
  percent: 63,
  deletable: false,
  reason: "受保护：下载中",
  rule: "downloading",
  why: "还在下载（63%）",
});

const preview = (over: Partial<DeletePreview> = {}): DeletePreview => ({
  files: [1, 2, 3, 4, 5, 6].map((n) => file(n, { size: (41.2 / 6) * GB })),
  torrents: [ok, hr, dl],
  subscriptions: [{ id: 5, status: "active", season: 2, follow: true }],
  issues: [],
  errors: [],
  ...over,
});

describe("标题与范围", () => {
  it("标题写名字、季与集", () => {
    const t = (selector: typeof sel & { episodes?: number[] }) =>
      deleteTitle({ selector, title: "长夜将尽", preset: { kind: "item" } });
    expect(t(sel)).toBe("删除《长夜将尽》第 2 季");
    expect(t({ ...sel, episodes: [3] })).toBe("删除《长夜将尽》第 2 季 E03");
    expect(t({ ...sel, episodes: [1, 2, 3, 5] })).toBe("删除《长夜将尽》第 2 季 E01–E03、E05");
    expect(t({ ...sel, season: 0 })).toBe("删除《长夜将尽》特别篇");
    expect(
      deleteTitle({
        selector: { media_type: "movie", tmdb_id: 693134, season: 0 },
        title: "沙丘2",
        preset: { kind: "item" },
      }),
    ).toBe("删除《沙丘2》");
    expect(scopeText(sel)).toBe("这一季");
    expect(scopeText({ ...sel, episodes: [3] })).toBe("这几集");
    expect(scopeText({ media_type: "movie", tmdb_id: 1, season: 0 })).toBe("这部电影");
  });

  it("「处理」的范围：第一个问题那一季里有问题的集；有不知道是哪一集的就整季；电影是整部", () => {
    expect(
      issueSelector("tv", 9, [
        { id: 1, season: 2, episode: 5 },
        { id: 2, season: 2, episode: 3 },
        { id: 3, season: 2, episode: 5 },
        { id: 4, season: 3, episode: 1 },
      ]),
    ).toEqual({ media_type: "tv", tmdb_id: 9, season: 2, episodes: [3, 5] });
    expect(issueSelector("tv", 9, [{ id: 1, season: 2 }])).toEqual({
      media_type: "tv",
      tmdb_id: 9,
      season: 2,
      episodes: [],
    });
    expect(issueSelector("movie", 9, [{ id: 1 }])).toEqual({
      media_type: "movie",
      tmdb_id: 9,
      season: 0,
    });
  });

  it("要处理的问题怎么写", () => {
    const is = (over: Partial<MediaIssue>): MediaIssue => ({
      id: 1,
      kind: "server_removed",
      status: "open",
      server_id: 1,
      server_name: "Jellyfin",
      server_item_id: "x",
      history_id: 0,
      media_type: "tv",
      tmdb_id: 9,
      season: 2,
      episode: 3,
      title: "",
      detail: "",
      created_at: "",
      updated_at: "",
      ...over,
    });
    expect(issueLine(is({}))).toBe("S02E03 在 Jellyfin 里被删了");
    expect(issueLine(is({ kind: "library_missing", episode: 5 }))).toBe("S02E05 库里的文件不见了");
    expect(issueLine(is({ episode: 0, server_name: "" }))).toBe("S02 在 媒体服务器 里被删了");
    expect(issueLine(is({ media_type: "movie", kind: "library_missing" }))).toBe(
      "库里的文件不见了",
    );
  });
});

describe("默认勾选", () => {
  it("大图打开：库里的文件、能删的种子、暂停订阅；受保护的与种子里还有没选的集的不勾", () => {
    const p = preview({ torrents: [ok, hr, dl, torrent({ hash: "ddd", covers_more: true })] });
    const c = defaultChoice(p, { kind: "item" }, "tv");
    expect(c.files).toBe(true);
    expect(c.existing).toBe(false);
    expect(c.subscription).toBe("pause");
    expect(Object.fromEntries(Object.entries(c.torrents).map(([k, v]) => [k, v.on]))).toEqual({
      "1|aaa": true,
      "2|bbb": false,
      "1|ccc": false,
      "1|ddd": false,
    });
    expect(Object.values(c.torrents).every((t) => t.data && !t.force)).toBe(true);
    expect(defaultChoice(p, { kind: "issues" }, "tv").files).toBe(true);
  });

  it("「删除这个种子」：只勾那一个（按下载器编号与 hash），不勾库文件，订阅保留；受保护的也不勾", () => {
    const p = preview({ torrents: [ok, torrent({ downloader_id: 2, hash: "aaa" }), hr] });
    const c = defaultChoice(p, { kind: "torrent", key: { downloader_id: 2, hash: "AAA" } }, "tv");
    expect(c.files).toBe(false);
    expect(c.subscription).toBe("keep");
    expect(c.torrents["1|aaa"]!.on, "另一台下载器上同一个 hash 的不勾").toBe(false);
    expect(c.torrents["2|aaa"]!.on).toBe(true);
    const guarded = defaultChoice(
      p,
      { kind: "torrent", key: { downloader_id: 2, hash: "bbb" } },
      "tv",
    );
    expect(guarded.torrents["2|bbb"]!.on).toBe(false);
  });

  it("没有 pt-tools 放进去的文件时不勾库文件", () => {
    const p = preview({ files: [file(1, { existing: true, mode: "existing" })] });
    expect(defaultChoice(p, { kind: "item" }, "tv").files).toBe(false);
  });
});

describe("库里的文件", () => {
  it("说明写个数、大小与方式；换了的、不在了的、移动整理的另起一行", () => {
    expect(filesView(preview().files, sel)).toEqual({
      line: "6 个文件 · 41.2 GB · 都是硬链接，种子里的数据不受影响",
      notes: [],
      enabled: true,
    });
    const mixed = [
      file(1),
      file(2, { mode: "copy", state: "changed" }),
      file(3, { mode: "copy", state: "missing" }),
      file(4, { mode: "move", deletable: false }),
      file(5, { mode: "existing", existing: true }),
    ];
    expect(filesView(mixed, sel)).toEqual({
      line: "3 个文件 · 13.6 GB · 硬链接 1 个、复制 2 个，种子里的数据不受影响",
      notes: [
        "1 个已经不在了，删的是记录",
        "1 个换成了别的文件，只删记录、留着文件",
        "1 个是移动整理的，是唯一的一份，不在这里删",
      ],
      enabled: true,
    });
    expect(filesView([file(1, { mode: "symlink" })], sel).line).toContain("都是软链接");
    expect(filesView([file(1, { mode: "copy" })], sel).line).toContain("都是复制出来的");
    expect(filesView([], sel)).toEqual({ line: "库里没有这一季的文件", notes: [], enabled: false });
    expect(filesView([], { media_type: "movie", tmdb_id: 1, season: 0 }).line).toBe(
      "库里没有这部电影的文件",
    );
    expect(filesView([file(4, { mode: "move", deletable: false })], sel)).toEqual({
      line: "没有 pt-tools 放进去的文件",
      notes: ["1 个是移动整理的，是唯一的一份，不在这里删"],
      enabled: false,
    });
    expect(fileGroups(mixed)).toEqual({
      placed: [mixed[0], mixed[1], mixed[2]],
      existing: [mixed[4]],
      locked: [mixed[3]],
    });
    expect(existingLine(mixed)).toBe("1 个文件 · 6.8 GB · 导入前就在库里，删了就没有了");
    expect(fileMeta(file(1))).toBe("S02E01 · 剧集库 · 硬链接 · 6.8 GB");
    expect(fileMeta(file(3, { episode_end: 4, library: undefined, size: 0 }))).toBe(
      "S02E03–E04 · 硬链接",
    );
    expect(fileMeta(file(1, { episode: 0, mode: "existing" }))).toBe("剧集库 · 已有文件 · 6.8 GB");
  });
});

describe("种子行", () => {
  it("标题写站点、集与下载器；说明写分享率、做种与 H&R；受保护的写原因", () => {
    expect(deleteTorrentTitle(ok)).toBe("OurBits · S02E01–E04 · qb");
    expect(deleteTorrentTitle(hr)).toBe("HDSky · S02E05–E06 · tr");
    expect(
      deleteTorrentTitle(torrent({ site: "", season: 0, episodes: "", version: "2160p FRDS" })),
    ).toBe("下载器里 · 2160p FRDS · qb");
    expect(deleteTorrentSub(ok)).toBe("分享率 1.31 · 做种 18 天 · H&R 已满足");
    expect(deleteTorrentSub(torrent({ reason: "可以删" }))).toBe("分享率 1.31 · 做种 18 天");
    expect(protectedText(hr)).toBe("受保护：H&R 要求做种 7 天，还差 2 天");
    expect(protectedText(dl)).toBe("受保护：还在下载（63%）");
    expect(
      protectedText(torrent({ why: undefined, protection: "", reason: "受保护：标签 keep" })),
    ).toBe("受保护：标签 keep");
  });

  it("共用数据：同一台下载器里还用着这份数据、这次又不删的种子；一起删的不算", () => {
    const a = torrent({
      hash: "aaa",
      shares: [
        { hash: "eee", name: "别的种子" },
        { hash: "ccc", name: "E07" },
      ],
    });
    const p = preview({ torrents: [a, dl] });
    const c = defaultChoice(p, { kind: "item" }, "tv");
    expect(sharedBy(a, c)).toEqual(["别的种子", "E07"]);
    c.torrents["1|ccc"]!.on = true;
    expect(sharedBy(a, c), "E07 这次一起删").toEqual(["别的种子"]);
    expect(sharedBy(torrent({ shares: [{ hash: "fff", name: "" }] }), c)).toEqual(["fff"]);
  });
});

describe("请求体与摘要", () => {
  it("默认：库文件、能删的种子连数据、暂停追剧", () => {
    const p = preview();
    const c = defaultChoice(p, { kind: "item" }, "tv");
    const req = deleteRequest(sel, p, c);
    expect(req).toEqual({
      ...sel,
      history_ids: [1, 2, 3, 4, 5, 6],
      allow_existing: false,
      torrents: [{ downloader_id: 1, hash: "AAA", delete_data: true }],
      confirm_protected: [],
      subscription: "pause",
    });
    expect(deleteSummary(req, p)).toBe("将删除 6 个库文件、1 个种子（连数据），暂停追剧");
    expect(canDelete(req, p)).toBe(true);
  });

  it("受保护的勾了「仍然删除」才删，并按（下载器编号, hash）写进确认；只删种子、删除订阅", () => {
    const p = preview();
    const c = defaultChoice(p, { kind: "item" }, "tv");
    c.torrents["2|bbb"]!.on = true;
    expect(deleteRequest(sel, p, c).torrents, "没勾「仍然删除」不删").toHaveLength(1);
    c.torrents["2|bbb"]!.force = true;
    c.torrents["1|aaa"]!.data = false;
    c.subscription = "delete";
    const req = deleteRequest(sel, p, c);
    expect(req.torrents).toEqual([
      { downloader_id: 1, hash: "AAA", delete_data: false },
      { downloader_id: 2, hash: "bbb", delete_data: true },
    ]);
    expect(req.confirm_protected).toEqual([{ downloader_id: 2, hash: "bbb" }]);
    expect(deleteSummary(req, p)).toBe("将删除 6 个库文件、2 个种子（1 个连数据），删除订阅");
    c.torrents["2|bbb"]!.data = false;
    expect(deleteSummary(deleteRequest(sel, p, c), p)).toBe(
      "将删除 6 个库文件、2 个种子（只删种子），删除订阅",
    );
  });

  it("共用数据的只删种子；导入前就有的文件要另外勾", () => {
    const shared = torrent({ shares: [{ hash: "eee", name: "别的种子" }] });
    const p = preview({
      torrents: [shared],
      files: [file(1), file(2, { existing: true, mode: "existing" })],
    });
    const c = defaultChoice(p, { kind: "item" }, "tv");
    let req = deleteRequest(sel, p, c);
    expect(req.torrents[0]!.delete_data).toBe(false);
    expect(req.history_ids).toEqual([1]);
    expect(req.allow_existing).toBe(false);
    c.existing = true;
    req = deleteRequest(sel, p, c);
    expect(req.history_ids).toEqual([1, 2]);
    expect(req.allow_existing).toBe(true);
    c.files = false;
    expect(deleteRequest(sel, p, c).history_ids, "两个勾选框各管各的").toEqual([2]);
  });

  it("什么也没选：不能删；只处理订阅时写「只暂停追剧」；没有订阅时订阅发 keep", () => {
    const p = preview();
    const c = defaultChoice(p, { kind: "torrent", key: { downloader_id: 9, hash: "zzz" } }, "tv");
    let req = deleteRequest(sel, p, c);
    expect(deleteSummary(req, p)).toBe("没有选要删的");
    expect(canDelete(req, p)).toBe(false);
    c.subscription = "pause";
    req = deleteRequest(sel, p, c);
    expect(deleteSummary(req, p)).toBe("只暂停追剧");
    expect(canDelete(req, p)).toBe(true);
    const none = preview({ subscriptions: [] });
    const r2 = deleteRequest(sel, none, defaultChoice(none, { kind: "item" }, "tv"));
    expect(r2.subscription).toBe("keep");
    const pickedPause = {
      ...defaultChoice(none, { kind: "item" }, "tv"),
      subscription: "pause" as const,
    };
    expect(
      deleteRequest(sel, none, pickedPause).subscription,
      "没有订阅时不管选了什么都发 keep",
    ).toBe("keep");
    expect(deleteSummary(r2, none)).toBe("将删除 6 个库文件、1 个种子（连数据）");
    const movieSub = preview({
      subscriptions: [{ id: 9, status: "active", season: 0, follow: false }],
    });
    expect(
      deleteSummary(
        deleteRequest(sel, movieSub, defaultChoice(movieSub, { kind: "item" }, "tv")),
        movieSub,
      ),
    ).toBe("将删除 6 个库文件、1 个种子（连数据），暂停订阅");
  });

  it("订阅的三个选项：追剧与电影的叫法", () => {
    expect(subscriptionOptions(preview(), "tv")).toEqual([
      {
        value: "pause",
        label: "暂停追剧",
        desc: "推荐：不暂停的话，台账会把删掉的集当成缺的，再去下一遍",
      },
      { value: "delete", label: "删除订阅", desc: "以后不再找这一季" },
      { value: "keep", label: "保留，继续追", desc: "只删这次选的文件和种子" },
    ]);
    const movie = subscriptionOptions(
      preview({ subscriptions: [{ id: 9, status: "active", season: 0, follow: false }] }),
      "movie",
    );
    expect(movie.map((o) => o.label)).toEqual(["暂停订阅", "删除订阅", "保留订阅"]);
    expect(movie[0]!.desc).toBe("推荐：不暂停的话，还在找的订阅会再去下一遍");
    expect(movie[1]!.desc).toBe("以后不再找这部电影");
  });

  it("没有要暂停的订阅：暂停那一项写明为什么，默认保留，摘要不写暂停", () => {
    const sub = (status: "active" | "paused" | "pending" | "done") => ({
      id: 9,
      status,
      season: 2,
      follow: false,
    });
    const pauseDesc = (p: DeletePreview, kind: "tv" | "movie") =>
      subscriptionOptions(p, kind)[0]!.desc;
    const paused = preview({ subscriptions: [sub("paused")] });
    expect(pausable(paused, "tv")).toBe(0);
    expect(pauseDesc(paused, "tv")).toBe("不用暂停：已经暂停了");
    expect(pauseDesc(preview({ subscriptions: [sub("pending")] }), "tv")).toBe(
      "不用暂停：待确认的订阅不会去下",
    );
    const doneMovie = preview({ subscriptions: [sub("done")] });
    expect(pausable(doneMovie, "movie")).toBe(0);
    expect(pauseDesc(doneMovie, "movie")).toBe("不用暂停：已经完成了，不会再去下");
    expect(pausable(doneMovie, "tv"), "完成了的剧集也要暂停：台账会把删掉的集当成缺的").toBe(1);
    expect(pauseDesc(preview({ subscriptions: [sub("paused"), sub("pending")] }), "tv")).toBe(
      "不用暂停：现在不会去下",
    );

    const c = defaultChoice(paused, { kind: "item" }, "tv");
    expect(c.subscription).toBe("keep");
    let req = deleteRequest(sel, paused, c);
    expect(deleteSummary(req, paused)).toBe("将删除 6 个库文件、1 个种子（连数据）");
    // 只选了暂停一个已经暂停的订阅：没有要做的
    const nothing = defaultChoice(
      paused,
      { kind: "torrent", key: { downloader_id: 9, hash: "zzz" } },
      "tv",
    );
    nothing.subscription = "pause";
    req = deleteRequest(sel, paused, nothing);
    expect(deleteSummary(req, paused)).toBe("没有选要删的");
    expect(canDelete(req, paused)).toBe(false);
    nothing.subscription = "delete";
    req = deleteRequest(sel, paused, nothing);
    expect(deleteSummary(req, paused)).toBe("只删除订阅");
    expect(canDelete(req, paused)).toBe(true);
  });

  it("结果：每一步的类别与一句话", () => {
    expect(stepKindLabel("subscription")).toBe("订阅");
    expect(stepKindLabel("torrent")).toBe("种子");
    expect(stepKindLabel("file")).toBe("库文件");
    const step = { kind: "file" as const, target: "/x", ok: true, message: "删了" };
    expect(resultText({ steps: [step, step], failed: 0 })).toBe("删完了，2 步都做成了");
    expect(resultText({ steps: [step, { ...step, ok: false }], failed: 1 })).toBe(
      "1 步没做成，原因写在下面；做成了的不受影响",
    );
    expect(resultText({ steps: [], failed: 0 })).toBe("没有要做的");
    expect(torrentKey({ downloader_id: 3, hash: "ABC" })).toBe("3|abc");
  });
});
