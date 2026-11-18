/*
 * 刷流收益的统计周期：按日、周、月、季度、年分段（接口 /api/brush/stats?granularity=&periods=，
 * 见 web/api_brush.go 的 buildBrushStats：主机时区，周从周一开始，最后一段是当前这一段、只算到今天）。
 *
 * 2026-10-28 用户：「刷流任务这个页面中，应该支持比如按天、周、月、季度、年等范围查看统计情况，菜单设计要美观一些，
 * 手机端也要同步；那么 30 天收益也应支持进行不同时间范围的切换」。
 */

export type BrushPeriod = "day" | "week" | "month" | "quarter" | "year";

export interface BrushPeriodMeta {
  value: BrushPeriod;
  /** 分段按钮上的字 */
  label: string;
  /** 请求几段 */
  periods: number;
  /** 当前这一段与上一段的叫法：今日 / 昨日、本周 / 上周… */
  current: string;
  previous: string;
  /** 一段的单位：「最近 7 天」「最近 7 个月」 */
  unit: string;
  /** 「每天」「每周」「每月」「每季度」「每年」 */
  each: string;
  /** 整个窗口：「最近 30 天」 */
  window: string;
}

export const BRUSH_PERIODS: BrushPeriodMeta[] = [
  {
    value: "day",
    label: "日",
    periods: 30,
    current: "今日",
    previous: "昨日",
    unit: "天",
    each: "每天",
    window: "最近 30 天",
  },
  {
    value: "week",
    label: "周",
    periods: 12,
    current: "本周",
    previous: "上周",
    unit: "周",
    each: "每周",
    window: "最近 12 周",
  },
  {
    value: "month",
    label: "月",
    periods: 12,
    current: "本月",
    previous: "上月",
    unit: "个月",
    each: "每月",
    window: "最近 12 个月",
  },
  {
    value: "quarter",
    label: "季",
    periods: 8,
    current: "本季度",
    previous: "上季度",
    unit: "个季度",
    each: "每季度",
    window: "最近 8 个季度",
  },
  {
    value: "year",
    label: "年",
    periods: 5,
    current: "今年",
    previous: "去年",
    unit: "年",
    each: "每年",
    window: "最近 5 年",
  },
];

export function brushPeriodMeta(p: string | null | undefined): BrushPeriodMeta {
  return BRUSH_PERIODS.find((m) => m.value === p) ?? BRUSH_PERIODS[0]!;
}

/** 「2026-10-29」→ 本地的那一天（不按 UTC 解析，否则东八区以西会差一天） */
function parseDay(s: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s);
  if (!m) return null;
  return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
}

/**
 * 柱子下面的短标签：日「10/29」，周「10/27」（周一），月「10月」（一月写「2026年1月」），
 * 季度「Q4」（第一季度写「2026 Q1」），年「2026」。
 */
export function brushPeriodShort(p: BrushPeriod, start: string): string {
  const d = parseDay(start);
  if (!d) return start;
  const y = d.getFullYear();
  const m = d.getMonth() + 1;
  switch (p) {
    case "week":
    case "day":
      return `${m}/${d.getDate()}`;
    case "month":
      return m === 1 ? `${y}年1月` : `${m}月`;
    case "quarter": {
      const q = Math.floor((m - 1) / 3) + 1;
      return q === 1 ? `${y} Q1` : `Q${q}`;
    }
    case "year":
      return String(y);
  }
}

/** 读数里的完整写法：「10月29日」「10月27日–11月2日」「2026年10月」「2026年第4季度」「2026年」 */
export function brushPeriodLong(p: BrushPeriod, start: string, end: string): string {
  const d = parseDay(start);
  if (!d) return start;
  const y = d.getFullYear();
  const m = d.getMonth() + 1;
  switch (p) {
    case "day":
      return `${m}月${d.getDate()}日`;
    case "week": {
      const e = parseDay(end);
      return e
        ? `${m}月${d.getDate()}日–${e.getMonth() + 1}月${e.getDate()}日`
        : `${m}月${d.getDate()}日起`;
    }
    case "month":
      return `${y}年${m}月`;
    case "quarter":
      return `${y}年第${Math.floor((m - 1) / 3) + 1}季度`;
    case "year":
      return `${y}年`;
  }
}

/** 记住上次选的周期（每台浏览器各记各的）；读不到时按日 */
const STORE_KEY = "pt-brush-period";

export function loadBrushPeriod(): BrushPeriod {
  try {
    return brushPeriodMeta(localStorage.getItem(STORE_KEY)).value;
  } catch {
    return "day";
  }
}

export function saveBrushPeriod(p: BrushPeriod) {
  try {
    localStorage.setItem(STORE_KEY, p);
  } catch {
    /* 存不了就算了：下次按日 */
  }
}
