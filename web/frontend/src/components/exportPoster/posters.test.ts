// @vitest-environment happy-dom
/*
 * 钉子：五个样式都按同一份数据画出来，打码时原文不进 DOM（导出的 PNG 就是这份 DOM 画的），
 * 没有的部分（逐站明细、周期增量、做种积分）对应的区块收起。
 */
import { afterEach, describe, expect, it } from "vitest";
import { createApp, h } from "vue";

import ExportPoster from "./ExportPoster.vue";
import { buildPosterModel, type PosterInput } from "./model";
import { POSTER_STYLES, type PosterStyleId } from "./styles";

const TiB = 1024 ** 4;

const SITES = [
  ["springsunday", 24.66, 3.08, "lumen_ssd", "Crazy User"],
  ["hdsky", 38.02, 4.11, "lumen_sky", "Veteran User"],
  ["mteam", 62.18, 5.42, "lumen_mt", "Elite User"],
  ["ourbits", 21.09, 2.95, "lumen_ob", "Insane User"],
] as const;

function input(over: Partial<PosterInput> = {}): PosterInput {
  const per = SITES.map(([site, up, down, username, levelName]) => ({
    site,
    username,
    userId: "1",
    uploaded: up * TiB,
    downloaded: down * TiB,
    ratio: up / down,
    bonus: 412_300,
    seeding: 300,
    leeching: 0,
    rank: "",
    levelName,
    joinDate: 1_552_521_600,
    lastUpdate: 1_760_000_000,
  }));
  return {
    stats: {
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
      totalSeedingBonus: 124_000,
    },
    sites: per,
    showSites: true,
    increments: {
      range: "7d",
      from: "2026-10-15",
      to: "2026-10-22",
      sites: [],
      totalUploaded: 2.31 * TiB,
      totalDownloaded: 0.18 * TiB,
      totalBonus: 21_000,
    },
    incrementLabel: "本周",
    title: "我的 PT 数据统计",
    mask: { names: true, users: true, logos: true },
    logos: new Map(),
    now: new Date(2026, 9, 22, 20, 30),
    ...over,
  };
}

let unmount: (() => void) | null = null;
afterEach(() => {
  unmount?.();
  unmount = null;
  document.body.innerHTML = "";
});

function render(styleId: PosterStyleId, colorway: string, over: Partial<PosterInput> = {}) {
  unmount?.();
  const el = document.createElement("div");
  document.body.appendChild(el);
  const model = buildPosterModel(input(over));
  const app = createApp({ render: () => h(ExportPoster, { model, styleId, colorway }) });
  app.mount(el);
  unmount = () => app.unmount();
  const poster = el.querySelector<HTMLElement>(".poster");
  if (!poster) throw new Error(`${styleId} 没有画出 .poster`);
  return poster;
}

const ALL = POSTER_STYLES.flatMap((s) => s.colorways.map((c) => [s.id, c.id] as const));

describe("五个样式", () => {
  it.each(ALL)(
    "%s · %s：画出标题、总上传、平均分享率与魔力，配色写在根节点上",
    (styleId, colorway) => {
      const p = render(styleId, colorway);
      const text = p.textContent ?? "";
      expect(p.classList.contains(`is-${colorway}`)).toBe(true);
      expect(text).toContain("我的 PT 数据统计");
      expect(text).toContain("145.95"); // 总上传 145.95 TB 的数字部分
      expect(text).toContain("8.04");
      expect(text).toContain("328.45");
      expect(text).toContain("+2.31 TB");
    },
  );

  it.each(POSTER_STYLES.map((s) => s.id))(
    "%s：打码时站点名、用户名不在 DOM 里，图标是 16 格色块",
    (styleId) => {
      const p = render(styleId, POSTER_STYLES.find((s) => s.id === styleId)!.colorways[0]!.id);
      const html = p.outerHTML;
      for (const [site, , , user] of SITES) {
        expect(html, `站点名 ${site}`).not.toContain(site);
        expect(html, `用户名 ${user}`).not.toContain(user);
      }
      const masks = p.querySelectorAll(".poster-logo--mask");
      expect(masks.length).toBeGreaterThanOrEqual(SITES.length);
      expect(masks[0]!.querySelectorAll("b")).toHaveLength(16);
      expect(p.querySelectorAll(".poster-mk").length).toBeGreaterThan(0);
    },
  );

  it.each(POSTER_STYLES.map((s) => s.id))(
    "%s：不打码时写出站点名与用户名，图标的 alt 为空",
    (styleId) => {
      const logo = { src: "data:image/png;base64,iVBORw0KGgo=", cells: Array(16).fill("#000") };
      const p = render(styleId, POSTER_STYLES.find((s) => s.id === styleId)!.colorways[0]!.id, {
        mask: { names: false, users: false, logos: false },
        logos: new Map([["mteam", logo]]),
      });
      const text = p.textContent ?? "";
      expect(text).toContain("mteam");
      expect(text).toContain("lumen_sky");
      const img = p.querySelector<HTMLImageElement>(".poster-logo--img img");
      expect(img?.getAttribute("src")).toBe(logo.src);
      expect(img?.getAttribute("alt")).toBe("");
    },
  );

  it.each(POSTER_STYLES.map((s) => s.id))(
    "%s：只放汇总时没有逐站区块；没有增量时不画增量",
    (styleId) => {
      const p = render(styleId, POSTER_STYLES.find((s) => s.id === styleId)!.colorways[0]!.id, {
        showSites: false,
        increments: null,
        mask: { names: false, users: false, logos: false },
      });
      const text = p.textContent ?? "";
      for (const [site] of SITES) expect(text).not.toContain(site);
      expect(text).not.toContain("+2.31 TB");
      expect(text).not.toContain("本周");
    },
  );
});

describe("各样式的收起与补位", () => {
  it("便当：没有增量时「账号」格占满一行", () => {
    const p = render("bento", "light", { increments: null });
    expect(p.querySelector(".pb-mini.is-wide")).not.toBeNull();
  });

  it("回顾：入图不到三个站时不画领奖台", () => {
    const two = input().sites.slice(0, 2);
    expect(render("wrapped", "sunset").querySelector(".pw-podium")).not.toBeNull();
    expect(render("wrapped", "sunset", { sites: two }).querySelector(".pw-podium")).toBeNull();
  });

  it("素笺：没有做种积分时汇总格补「平均每站上传」凑满两行", () => {
    const base = input();
    const p = render("paper", "warm", {
      stats: { ...base.stats, totalSeedingBonus: 0 },
    });
    expect(p.querySelectorAll(".pp-cell")).toHaveLength(8);
    expect(p.textContent).toContain("平均每站上传");
  });

  it("夜幕：六个小格，没有做种积分时往后补", () => {
    const base = input();
    const p = render("night", "teal", { stats: { ...base.stats, totalSeedingBonus: 0 } });
    expect(p.querySelectorAll(".pn-tile")).toHaveLength(6);
    expect(p.textContent).not.toContain("做种积分");
  });

  it("小票：周期写成中英两种，红章上是平均分享率", () => {
    const p = render("receipt", "white");
    expect(p.textContent).toContain("THIS WEEK");
    expect(p.querySelector(".pr-stamp")?.textContent).toContain("8.04");
  });
});
