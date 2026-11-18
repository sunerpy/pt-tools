/*
 * 钉子：系统体检页（媒体中心 M8）的分组顺序、状态的图标与颜色、页头的一句话，以及「有问题的」筛选只留提醒与失败。
 */
import type { HealthItem, HealthReport } from "@/api";
import { describe, expect, it } from "vitest";
import {
  changedKeys,
  checkGroups,
  checkHeadline,
  checkMeta,
  filterGroups,
  fixRoute,
  HEALTH_GROUPS,
  healthStatus,
  problemItems,
  ringSegments,
} from "./systemCheck";

const now = new Date("2026-11-04T20:00:00");

function item(group: HealthItem["group"], key: string, status: HealthItem["status"]): HealthItem {
  return { key, group, title: key, status };
}

function report(items: HealthItem[], extra: Partial<HealthReport> = {}): HealthReport {
  const n = (s: string) => items.filter((it) => it.status === s).length;
  return {
    at: "2026-11-04T09:30:00",
    duration_ms: 3200,
    items,
    ok: n("ok"),
    warn: n("warn"),
    fail: n("fail"),
    skip: n("skip"),
    ...extra,
  };
}

describe("状态", () => {
  it("四种状态的图标与颜色；不认识的按跳过", () => {
    expect(healthStatus("ok")).toEqual({ label: "正常", icon: "circle-check", tone: "ok" });
    expect(healthStatus("warn")).toEqual({ label: "提醒", icon: "triangle-alert", tone: "warn" });
    expect(healthStatus("fail")).toEqual({ label: "失败", icon: "circle-x", tone: "dang" });
    expect(healthStatus("skip")).toEqual({ label: "跳过", icon: "minus", tone: "neutral" });
    expect(healthStatus("别的")).toEqual(healthStatus("skip"));
  });

  it("分组顺序与后端一致", () => {
    expect(HEALTH_GROUPS.map((g) => g.key)).toEqual([
      "tmdb",
      "sites",
      "downloaders",
      "servers",
      "paths",
      "libraries",
      "disks",
      "nas",
      "time",
      "backup",
      "secret",
    ]);
  });
});

describe("页头的一句话", () => {
  it("没体检过写一次查什么", () => {
    expect(checkHeadline(null)).toContain("一次检查 TMDB");
    expect(checkHeadline(null)).toContain("磁盘、NAS、时间");
  });

  it("时间、用时与结果", () => {
    const bad = report([
      item("tmdb", "tmdb", "fail"),
      item("sites", "s1", "warn"),
      item("time", "time", "ok"),
    ]);
    expect(checkHeadline(bad, now)).toBe("11-04 09:30 体检 · 用时 3.2 秒 · 1 项失败、1 项提醒");
    const good = report([item("time", "time", "ok")], { duration_ms: 12_400 });
    expect(checkHeadline(good, now)).toBe("11-04 09:30 体检 · 用时 12 秒 · 都正常");
    expect(checkHeadline(report([item("sites", "s1", "warn")]), now)).toBe(
      "11-04 09:30 体检 · 用时 3.2 秒 · 1 项提醒",
    );
  });
});

describe("按组排好的各项", () => {
  const rep = report([
    item("secret", "secret", "ok"),
    item("sites", "s1", "ok"),
    item("tmdb", "tmdb", "warn"),
    item("sites", "s2", "fail"),
    item("sites", "s3", "skip"),
    item("time", "time", "skip"),
  ]);

  it("按分组顺序，组里按后端给的顺序；最坏的状态是失败 > 提醒 > 正常 > 跳过", () => {
    const groups = checkGroups(rep);
    expect(groups.map((g) => g.key)).toEqual(["tmdb", "sites", "time", "secret"]);
    expect(groups.map((g) => g.worst)).toEqual(["warn", "fail", "skip", "ok"]);
    expect(groups[1].items.map((it) => it.key)).toEqual(["s1", "s2", "s3"]);
    expect(groups[1]).toMatchObject({ label: "站点", icon: "globe" });
  });

  it("「有问题的」只留提醒与失败；没有这种项的组不列", () => {
    const groups = checkGroups(rep, true);
    expect(groups.map((g) => g.key)).toEqual(["tmdb", "sites"]);
    expect(groups[1].items.map((it) => it.key)).toEqual(["s2"]);
    expect(checkGroups(report([item("time", "time", "ok")]), true)).toEqual([]);
    expect(checkGroups(null)).toEqual([]);
  });
});

describe("环形图的各段", () => {
  it("失败、提醒、正常、跳过依次排，按项数占圆周（总长 100），多段时每段扣 1.2 的缝", () => {
    const segs = ringSegments({ fail: 1, warn: 2, ok: 15, skip: 1 });
    expect(segs.map((s) => s.status)).toEqual(["fail", "warn", "ok", "skip"]);
    const r = (n: number) => Math.round(n * 1000) / 1000;
    expect(segs.map((s) => r(s.start))).toEqual([0, 5.263, 15.789, 94.737]);
    expect(segs.map((s) => r(s.length))).toEqual([4.063, 9.326, 77.747, 4.063]);
  });

  it("只有一种状态时整圈一段、不留缝；没有项时没有段（整圈灰底）", () => {
    expect(ringSegments({ fail: 0, warn: 0, ok: 3, skip: 0 })).toEqual([
      { status: "ok", start: 0, length: 100 },
    ]);
    expect(ringSegments({ fail: 0, warn: 0, ok: 0, skip: 0 })).toEqual([]);
    expect(ringSegments(null)).toEqual([]);
  });

  it("很小的一段不会被缝吃掉，至少留它一半的长度", () => {
    const segs = ringSegments({ fail: 1, warn: 0, ok: 199, skip: 0 });
    expect(segs[0].status).toBe("fail");
    expect(segs[0].length).toBeCloseTo(0.25, 5);
  });
});

describe("需要处理", () => {
  it("失败在前、提醒在后，同状态按报告里的顺序；正常与跳过不列", () => {
    const rep = report([
      item("sites", "site:a", "warn"),
      item("tmdb", "tmdb", "ok"),
      item("downloaders", "downloader:1", "fail"),
      item("backup", "backup", "warn"),
      item("libraries", "library:2", "fail"),
      item("time", "time", "skip"),
    ]);
    expect(problemItems(rep).map((it) => it.key)).toEqual([
      "downloader:1",
      "library:2",
      "site:a",
      "backup",
    ]);
    expect(problemItems(null)).toEqual([]);
  });
});

describe("去处理的链接", () => {
  it("按组给路由；单个站点到它的详情页", () => {
    const at = (group: HealthItem["group"], key: string) => fixRoute({ group, key });
    expect(at("tmdb", "tmdb")).toBe("/media/recognize");
    expect(at("tmdb", "tmdb:image")).toBe("/media/recognize");
    expect(at("time", "time")).toBe("/media/recognize");
    expect(at("sites", "site:hdsky")).toBe("/sites/hdsky");
    expect(at("sites", "site:a b")).toBe("/sites/a%20b");
    expect(at("downloaders", "downloader:2")).toBe("/downloaders");
    expect(at("servers", "server:1")).toBe("/media/library");
    expect(at("paths", "path:1")).toBe("/media/library");
    expect(at("libraries", "library:3")).toBe("/media/library");
    expect(at("backup", "backup")).toBe("/global");
    expect(at("nas", "nas:3")).toBe("/system/nas");
    expect(at("nas", "nas")).toBe("/system/nas");
  });

  it("一组的汇总项（没有配、读取失败）到那一组的列表页；磁盘、密钥没有能去改的页面", () => {
    const at = (group: HealthItem["group"], key: string) => fixRoute({ group, key });
    expect(at("sites", "sites")).toBe("/sites");
    expect(at("downloaders", "downloaders")).toBe("/downloaders");
    expect(at("libraries", "libraries")).toBe("/media/library");
    expect(at("disks", "disk:/data")).toBeNull();
    expect(at("disks", "disks")).toBeNull();
    expect(at("secret", "secret")).toBeNull();
    expect(fixRoute({ group: "别的" as HealthItem["group"], key: "x" })).toBeNull();
  });
});

describe("这次新出现的问题", () => {
  it("这次是提醒或失败、上次没有这一项或比这次好的 key", () => {
    const prev = report([
      item("sites", "site:a", "ok"),
      item("sites", "site:b", "warn"),
      item("downloaders", "downloader:1", "warn"),
      item("backup", "backup", "fail"),
      item("time", "time", "skip"),
    ]);
    const next = report([
      item("sites", "site:a", "warn"),
      item("sites", "site:b", "warn"),
      item("downloaders", "downloader:1", "fail"),
      item("backup", "backup", "warn"),
      item("time", "time", "ok"),
      item("libraries", "library:9", "fail"),
    ]);
    expect([...changedKeys(prev, next)].sort()).toEqual(["downloader:1", "library:9", "site:a"]);
  });

  it("没有上一次（第一次体检、页面刚打开）时不标", () => {
    expect(changedKeys(null, report([item("sites", "site:a", "fail")])).size).toBe(0);
    expect(changedKeys(report([]), null).size).toBe(0);
  });
});

describe("总览卡的一行", () => {
  it("上次的时间、用时与一共几项", () => {
    const rep = report([item("tmdb", "tmdb", "ok"), item("time", "time", "warn")]);
    expect(checkMeta(rep, now)).toBe("上次 11-04 09:30 体检 · 用时 3.2 秒 · 共 2 项");
    expect(checkMeta({ ...rep, duration_ms: 12_400 }, now)).toBe(
      "上次 11-04 09:30 体检 · 用时 12 秒 · 共 2 项",
    );
    /* 很快查完（几十毫秒）时不写「用时 0.0 秒」 */
    expect(checkMeta({ ...rep, duration_ms: 30 }, now)).toBe(
      "上次 11-04 09:30 体检 · 用时不到 0.1 秒 · 共 2 项",
    );
    expect(checkHeadline({ ...rep, duration_ms: 99 }, now)).toBe(
      "11-04 09:30 体检 · 用时不到 0.1 秒 · 1 项提醒",
    );
  });
});

describe("分组卡按筛选显示", () => {
  const rep = report([
    item("tmdb", "tmdb", "ok"),
    item("sites", "s1", "ok"),
    item("sites", "s2", "fail"),
    item("sites", "s3", "warn"),
    item("backup", "backup", "skip"),
  ]);

  it("不筛选时每组全列；组的最坏状态按整组算", () => {
    const got = filterGroups(rep, { onlyProblems: false, status: "" });
    expect(got.map((g) => [g.group.key, g.items.length])).toEqual([
      ["tmdb", 1],
      ["sites", 3],
      ["backup", 1],
    ]);
    expect(got[1].group.worst).toBe("fail");
  });

  it("按状态筛：只留这个状态的项，组的最坏状态仍按整组算；没有这种项的组不列", () => {
    const got = filterGroups(rep, { onlyProblems: false, status: "ok" });
    expect(got.map((g) => [g.group.key, g.items.map((it) => it.key)])).toEqual([
      ["tmdb", ["tmdb"]],
      ["sites", ["s1"]],
    ]);
    expect(got[1].group.worst).toBe("fail");
  });

  it("只看有问题的：只留提醒与失败；和状态筛选同时开时两个都要满足", () => {
    expect(
      filterGroups(rep, { onlyProblems: true, status: "" }).map((g) => g.items.map((i) => i.key)),
    ).toEqual([["s2", "s3"]]);
    expect(filterGroups(rep, { onlyProblems: true, status: "ok" })).toEqual([]);
    expect(filterGroups(null, { onlyProblems: false, status: "" })).toEqual([]);
  });
});
