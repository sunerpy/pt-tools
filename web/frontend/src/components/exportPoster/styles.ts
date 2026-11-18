/**
 * 海报的五个样式与各自的配色。样式决定版式，配色只换颜色；每个样式的第一种配色是默认。
 * 名字与说明出现在导出页的「样式」选择里。
 */
export type PosterStyleId = "paper" | "night" | "bento" | "wrapped" | "receipt";

export interface PosterColorway {
  id: string;
  name: string;
  /** 配色按钮上的两个色点：底色与主色 */
  swatch: [string, string];
}

export interface PosterStyleDef {
  id: PosterStyleId;
  name: string;
  hint: string;
  colorways: PosterColorway[];
}

export const POSTER_STYLES: readonly PosterStyleDef[] = [
  {
    id: "bento",
    name: "便当",
    hint: "大小格子拼成一张图，信息最全，和网页的卡片风格一致",
    colorways: [
      { id: "light", name: "浅色", swatch: ["#f2f4f7", "#2e90fa"] },
      { id: "dark", name: "深色", swatch: ["#111318", "#53b1fd"] },
    ],
  },
  {
    id: "paper",
    name: "素笺",
    hint: "纸底细线、衬线大数字，像一份年度报告",
    colorways: [
      { id: "warm", name: "暖白", swatch: ["#f7f4ee", "#1f1b16"] },
      { id: "cool", name: "冷白", swatch: ["#f4f6f9", "#16202c"] },
      { id: "green", name: "墨绿", swatch: ["#1e2b25", "#efe7d6"] },
    ],
  },
  {
    id: "night",
    name: "夜幕",
    hint: "深色玻璃卡片，环形图画出上传与下载的比例",
    colorways: [
      { id: "teal", name: "青紫", swatch: ["#2dd4bf", "#8b5cf6"] },
      { id: "amber", name: "蓝橙", swatch: ["#3b82f6", "#f97316"] },
      { id: "black", name: "纯黑", swatch: ["#050505", "#a1a1aa"] },
    ],
  },
  {
    id: "wrapped",
    name: "回顾",
    hint: "一句话一个大数字，上传最多的站上领奖台",
    colorways: [
      { id: "sunset", name: "紫红橙", swatch: ["#6d28d9", "#ea580c"] },
      { id: "ocean", name: "蓝绿", swatch: ["#1e40af", "#059669"] },
      { id: "berry", name: "粉紫", swatch: ["#db2777", "#7c3aed"] },
    ],
  },
  {
    id: "receipt",
    name: "小票",
    hint: "热敏小票、红章与条形码，站多时也整齐",
    colorways: [
      { id: "white", name: "白纸", swatch: ["#fbfaf6", "#d33a2c"] },
      { id: "kraft", name: "牛皮纸", swatch: ["#e3cfa9", "#9b2c1f"] },
    ],
  },
];

export const DEFAULT_POSTER_STYLE: PosterStyleId = "bento";

export function posterStyle(id: string): PosterStyleDef {
  return POSTER_STYLES.find((s) => s.id === id) ?? POSTER_STYLES[0]!;
}

/** 样式里有这个配色就用它，没有就用这个样式的第一种 */
export function posterColorway(style: PosterStyleDef, id: string | undefined): PosterColorway {
  return style.colorways.find((c) => c.id === id) ?? style.colorways[0]!;
}
