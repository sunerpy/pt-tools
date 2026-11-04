/*
 * 钉子：系统体检页（媒体中心 M8）的分组顺序、状态的图标与颜色、页头的一句话，以及「有问题的」筛选只留提醒与失败。
 */
import type { HealthItem, HealthReport } from "@/api";
import { describe, expect, it } from "vitest";
import { checkGroups, checkHeadline, HEALTH_GROUPS, healthStatus } from "./systemCheck";

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
      "time",
      "backup",
      "secret",
    ]);
  });
});

describe("页头的一句话", () => {
  it("没体检过写一次查什么", () => {
    expect(checkHeadline(null)).toContain("一次检查 TMDB");
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
