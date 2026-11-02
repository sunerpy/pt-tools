import { describe, expect, it } from "vitest";

import {
  applyFacets,
  buildFacets,
  type FacetRecord,
  facetLabel,
  selectedCount,
  toggleFacet,
} from "./searchFacets";

const rec = (
  resolution: string,
  group: string,
  over: Partial<FacetRecord> & { subs?: boolean; hdr?: string[]; source?: string } = {},
): FacetRecord => ({
  meta: {
    resolution,
    group,
    source: over.source ?? "WEB-DL",
    chinese_subs: over.subs,
    hdr: over.hdr,
  },
  free: over.free ?? false,
  hr: over.hr ?? false,
  discount: over.discount ?? "NONE",
  site: over.site ?? "hdsky",
});

const records: FacetRecord[] = [
  rec("2160p", "HHWEB", { subs: true, free: true }),
  rec("2160p", "CHDWEB", { subs: true, hdr: ["HDR10"] }),
  rec("2160p", "HHWEB", { hdr: ["DV"], site: "ourbits" }),
  rec("1080p", "ADWeb", { subs: true, hr: true, site: "pttime" }),
  rec("1080p", "HHWEB", { source: "BluRay", free: true }),
];

const counts = (groups: ReturnType<typeof buildFacets>, key: string) =>
  Object.fromEntries(
    (groups.find((g) => g.key === key)?.options ?? []).map((o) => [o.value, o.count]),
  );

describe("筛选组与计数", () => {
  it("没选时数的是全部结果；分辨率按从好到差排，其余按条数", () => {
    const groups = buildFacets(records, {}, ["resolution", "group", "flags"]);
    expect(groups.map((g) => g.key)).toEqual(["resolution", "group", "flags"]);
    expect(groups[0]!.options.map((o) => o.value)).toEqual(["2160p", "1080p"]);
    expect(counts(groups, "resolution")).toEqual({ "2160p": 3, "1080p": 2 });
    expect(groups[1]!.options.map((o) => o.value)).toEqual(["HHWEB", "ADWeb", "CHDWEB"]);
    expect(counts(groups, "flags")).toEqual({ subs: 3, free: 2, nohr: 4 });
  });

  it("一个选项的数是其他组的筛选下它有几条：勾上一个，同组其他选项的数不变成 0", () => {
    const groups = buildFacets(records, { resolution: ["2160p"] }, ["resolution", "group"]);
    expect(counts(groups, "resolution"), "同组不受自己的筛选影响").toEqual({
      "2160p": 3,
      "1080p": 2,
    });
    expect(counts(groups, "group"), "别的组跟着变").toEqual({ HHWEB: 2, CHDWEB: 1, ADWeb: 0 });
    const adweb = groups.find((g) => g.key === "group")!.options.find((o) => o.value === "ADWeb")!;
    expect(adweb.count).toBe(0);
  });

  it("同组多选取并集，组与组取交集", () => {
    expect(applyFacets(records, { group: ["HHWEB", "CHDWEB"] })).toHaveLength(4);
    expect(applyFacets(records, { group: ["HHWEB", "CHDWEB"], flags: ["subs"] })).toHaveLength(2);
    expect(applyFacets(records, { resolution: ["1080p"], group: ["CHDWEB"] })).toHaveLength(0);
    expect(applyFacets(records, {})).toHaveLength(5);
  });

  it("来源里 Remux 单独成一项，HDR 一条可以有几个值", () => {
    const r = [
      rec("2160p", "FRDS", { source: "BluRay" }),
      rec("2160p", "X", { hdr: ["DV", "HDR10"] }),
    ];
    r[0]!.meta!.remux = true;
    const groups = buildFacets(r, {}, ["source", "hdr"]);
    expect(counts(groups, "source")).toEqual({ BluRay: 1, Remux: 1, "WEB-DL": 1 });
    expect(counts(groups, "hdr")).toEqual({ DV: 1, HDR10: 1 });
    expect(applyFacets(r, { hdr: ["HDR10"] })).toHaveLength(1);
  });

  it("没有选项的组不出现；选项名给人看的写法", () => {
    expect(buildFacets(records, {}, ["platform"])).toEqual([]);
    expect(facetLabel("hdr", "DV")).toBe("杜比视界");
    expect(facetLabel("platform", "Tencent Video")).toBe("腾讯视频");
    expect(facetLabel("flags", "nohr")).toBe("无 H&R");
    expect(facetLabel("group", "HHWEB")).toBe("HHWEB");
  });

  it("50% 只在要它的地方出现（手机筛选面板）", () => {
    const r = [rec("1080p", "X", { discount: "PERCENT_50" })];
    expect(counts(buildFacets(r, {}, ["flags"]), "flags")).toEqual({ subs: 0, free: 0, nohr: 1 });
    expect(counts(buildFacets(r, {}, ["flags"], { half: true }), "flags")).toEqual({
      subs: 0,
      free: 0,
      half: 1,
      nohr: 1,
    });
  });

  it("勾选、取消与已选个数", () => {
    let sel = toggleFacet({}, "group", "HHWEB");
    sel = toggleFacet(sel, "flags", "free");
    expect(selectedCount(sel)).toBe(2);
    sel = toggleFacet(sel, "group", "HHWEB");
    expect(sel.group).toBeUndefined();
    expect(selectedCount(sel)).toBe(1);
  });
});

describe("季集", () => {
  it("给了 episodes 时用它，不给时按标题解析出的季集写", () => {
    const parsed: FacetRecord = {
      meta: { season: 2, episode: 8 },
      free: false,
      hr: false,
      site: "hdsky",
    };
    const mapped: FacetRecord = { ...parsed, meta: { season: 4 }, episodes: "E04" };
    const groups = buildFacets([parsed, mapped], {}, ["episodes"]);
    expect(groups[0]!.options.map((o) => o.value).sort()).toEqual(["E04", "S02E08"]);
  });
});
