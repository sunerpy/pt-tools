/*
 * 钉子：海报的数据整理（buildPosterModel）。五个样式只管排版，数从这里来：
 * 数与单位拆开、站点按上传排、条形的比例、增量的符号、没有的指标留空串让样式收起对应的格子。
 */
import { describe, expect, it } from "vitest";

import type { AggregatedStatsResponse, DeltaSummary, UserInfoResponse } from "@/api";

import {
  buildPosterModel,
  bytesNumber,
  countNumber,
  fallbackCells,
  hueOf,
  maskParts,
  type PosterInput,
  posterDate,
  posterDateTime,
} from "./model";

const TiB = 1024 ** 4;
const GiB = 1024 ** 3;

function site(
  name: string,
  uploaded: number,
  downloaded: number,
  extra: Partial<UserInfoResponse> = {},
) {
  return {
    site: name,
    username: "lumen",
    userId: "1",
    uploaded,
    downloaded,
    ratio: downloaded > 0 ? uploaded / downloaded : 0,
    bonus: 12_345,
    seeding: 10,
    leeching: 0,
    rank: "User",
    lastUpdate: 1_760_000_000,
    ...extra,
  } satisfies UserInfoResponse;
}

function stats(per: UserInfoResponse[], extra: Partial<AggregatedStatsResponse> = {}) {
  return {
    totalUploaded: per.reduce((a, s) => a + s.uploaded, 0),
    totalDownloaded: per.reduce((a, s) => a + s.downloaded, 0),
    averageRatio: 8.04,
    totalSeeding: 2416,
    totalLeeching: 0,
    totalBonus: 3_284_500,
    siteCount: per.length,
    lastUpdate: 1_760_000_000,
    perSiteStats: per,
    totalBonusPerHour: 1290,
    totalSeederSize: 98.6 * TiB,
    ...extra,
  } satisfies AggregatedStatsResponse;
}

function input(over: Partial<PosterInput> = {}): PosterInput {
  const per = [
    site("hdsky", 38 * TiB, 4 * TiB, { joinDate: 1_565_000_000, levelName: "Veteran User" }),
    site("mteam", 62 * TiB, 5 * TiB, { joinDate: 1_552_521_600 }),
    site("pterclub", 4 * TiB, 1 * TiB),
  ];
  return {
    stats: stats(per),
    sites: per,
    showSites: true,
    increments: null,
    incrementLabel: "本周",
    title: "我的 PT 数据统计",
    mask: { names: true, users: false, logos: true },
    logos: new Map(),
    now: new Date(2026, 9, 22, 20, 5),
    ...over,
  };
}

describe("数与单位", () => {
  it("formatBytes 的写法拆成数与单位", () => {
    expect(bytesNumber(186.42 * TiB)).toEqual({ text: "186.42 TB", value: "186.42", unit: "TB" });
    expect(bytesNumber(0)).toEqual({ text: "0 B", value: "0", unit: "B" });
  });

  it("formatNumber 的 K、万、亿 是单位，没有单位时为空串", () => {
    expect(countNumber(3_284_500)).toEqual({ text: "328.45万", value: "328.45", unit: "万" });
    expect(countNumber(1290)).toEqual({ text: "1.29K", value: "1.29", unit: "K" });
    expect(countNumber(412)).toEqual({ text: "412", value: "412", unit: "" });
    expect(countNumber(-25_000).text).toBe("-2.50万");
  });

  it("日期不随浏览器语言变：2026/10/22 与 2026/10/22 20:05", () => {
    const d = new Date(2026, 9, 22, 20, 5);
    expect(posterDate(d)).toBe("2026/10/22");
    expect(posterDateTime(d)).toBe("2026/10/22 20:05");
  });
});

describe("buildPosterModel", () => {
  it("站点按上传从多到少，条形按这张图里上传最多的站算", () => {
    const m = buildPosterModel(input());
    expect(m.sites.map((s) => s.id)).toEqual(["mteam", "hdsky", "pterclub"]);
    expect(m.sites[0]!.upShare).toBe(1);
    expect(m.sites[1]!.upShare).toBeCloseTo(38 / 62);
    expect(m.sites[0]!.upFrac).toBeCloseTo(62 / 67);
    expect(m.sites[1]!.level).toBe("Veteran User");
    expect(m.sites[2]!.level, "没有等级名时用 rank").toBe("User");
  });

  it("关掉逐站明细：sites 是空的，汇总照旧", () => {
    const m = buildPosterModel(input({ showSites: false }));
    expect(m.sites).toEqual([]);
    expect(m.up.text).toBe("104 TB");
    expect(m.siteCount).toBe(3);
  });

  it("分享率是各站的平均（与统计页同一个数），环与占比条用上传占上传加下载的比例", () => {
    const m = buildPosterModel(input());
    expect(m.ratio).toBe("8.04");
    expect(m.upFrac).toBeCloseTo(104 / 114);
  });

  it("最早入站取所有站点里最早的那个（不只入图的）", () => {
    const m = buildPosterModel(input({ sites: [] }));
    expect(m.join).toBe(new Date(1_552_521_600 * 1000).toLocaleDateString("zh-CN"));
    expect(m.joinDur).not.toBe("");
  });

  it("没有站点报入站日期、做种积分：对应的字段是空串，样式收起那一格", () => {
    const per = [site("a", 1 * TiB, 0)];
    const m = buildPosterModel(input({ stats: stats(per), sites: per }));
    expect(m.join).toBe("");
    expect(m.joinDur).toBe("");
    expect(m.seedBonus).toBeNull();
    expect(m.upFrac, "没有下载时全是上传").toBe(1);
  });

  it("有做种积分时给出合计", () => {
    const per = [site("a", 1 * TiB, 0)];
    const m = buildPosterModel(
      input({ stats: stats(per, { totalSeedingBonus: 124_000 }), sites: per }),
    );
    expect(m.seedBonus?.text).toBe("12.40万");
  });

  it("增量带上「+」，标签跟着周期走；不放增量时是 null", () => {
    const inc: DeltaSummary = {
      range: "7d",
      from: "2026-10-15",
      to: "2026-10-22",
      sites: [],
      totalUploaded: 2.31 * TiB,
      totalDownloaded: 186.2 * GiB,
      totalBonus: 21_000.4,
    };
    const m = buildPosterModel(input({ increments: inc }));
    expect(m.inc).toMatchObject({
      label: "本周",
      up: "+2.31 TB",
      down: "+186.2 GB",
      bonus: "+2.10万",
    });
    expect(buildPosterModel(input()).inc).toBeNull();
  });

  it("站点自己的魔力名：SSD 叫茉莉", () => {
    const per = [site("springsunday", 1 * TiB, 0)];
    const m = buildPosterModel(input({ stats: stats(per), sites: per }));
    expect(m.sites[0]!.bonusName).toBe("茉莉");
  });

  it("小票编号是日期加站点数", () => {
    expect(buildPosterModel(input()).serial).toBe("20261022-0003");
  });

  it("有取到的图标就用，没有时用站点颜色的 16 格色块", () => {
    const logo = { src: "data:image/png;base64,AAAA", cells: Array(16).fill("rgb(1 2 3)") };
    const m = buildPosterModel(input({ logos: new Map([["mteam", logo]]) }));
    expect(m.sites[0]!.logo).toBe(logo);
    expect(m.sites[1]!.logo.src).toBeNull();
    expect(m.sites[1]!.logo.cells).toHaveLength(16);
  });

  it("打码开关原样带进去（打码在渲染时做，数据里保留原文）", () => {
    const m = buildPosterModel(input({ mask: { names: false, users: true, logos: false } }));
    expect(m.mask).toEqual({ names: false, users: true, logos: false });
    expect(m.sites[0]!.name).toBe("mteam");
  });
});

describe("打码", () => {
  it("只留首字，条宽随字数，一个字时也有一点宽", () => {
    expect(maskParts("springsunday")).toEqual({ first: "s", barEm: 11 * 0.56 });
    expect(maskParts("a").barEm).toBe(1.4);
    expect(maskParts("馒头").first).toBe("馒");
  });

  it("没有图标时的色块：同一站点每次一样，取站点颜色的色相", () => {
    const a = fallbackCells("hdsky", "#1890ff");
    expect(a).toHaveLength(16);
    expect(fallbackCells("hdsky", "#1890ff")).toEqual(a);
    expect(hueOf("#1890ff")).toBe(209);
    expect(a[0]).toMatch(/^hsl\(209 46% [\d.]+%\)$/);
    expect(hueOf("hsl(152, 70%, 50%)")).toBe(152);
    expect(hueOf("not-a-color")).toBe(210);
  });
});

describe("英文海报数字与逐站指标", () => {
  it("使用 K/M/B 单位，同时保留默认中文格式", () => {
    expect(countNumber(12_345, "en")).toEqual({ text: "12.35K", value: "12.35", unit: "K" });
    expect(countNumber(3_284_500, "en").text).toBe("3.28M");
    expect(countNumber(-2_500_000_000, "en").text).toBe("-2.50B");
    expect(countNumber(12_345).text).toBe("1.23万");
  });
  it("英文 model 同时覆盖逐站魔力、时魔、增量与入站时长", () => {
    const per = [site("springsunday", 100, 10, { bonusPerHour: 12_345, joinDate: 1_552_521_600 })];
    const m = buildPosterModel(
      input({
        locale: "en",
        stats: stats(per),
        sites: per,
        increments: {
          range: "7d",
          from: "2026-10-15",
          to: "2026-10-22",
          sites: [],
          totalUploaded: 1,
          totalDownloaded: 0,
          totalBonus: 12345,
        },
      }),
    );
    expect(m.sites[0]!.bonusName).toBe("Bonus");
    expect(m.sites[0]!.bonus.text).toBe("12.35K");
    expect(m.sites[0]!.bph).toBe("12.35K");
    expect(m.inc!.bonus).toBe("+12.35K");
    expect(m.joinDur).not.toMatch(/[年月天]/);
    expect(m.sites[0]!.joinDur).not.toMatch(/[年月天]/);
  });
});
