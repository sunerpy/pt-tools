/**
 * 运行日志一行的级别 —— 画板 29 左栏的 p-lv（级别筛选）与计数都靠它。
 *
 * 后端只给一整段文本（单文件 tail），级别只能从行里认：
 * 结构化日志是 `"level":"error"`，Zap 的 console 编码器是行里一个裸的 `ERROR`。
 *
 * **有 JSON level 字段就只认它**。之前先对整行跑 `\berror\b`，于是一条
 * `{"level":"warn",…,"msg":"retry after error"}` 被归成 ERROR —— 筛 WARN 看不到它、计数也偏。
 * 没有 level 字段的纯文本行才按级别词猜；认不出来的归「其他」，不静默丢掉。
 */
export const LEVELS = ["error", "warn", "info", "debug", "other"] as const;
export type LogLevel = (typeof LEVELS)[number];

const JSON_LEVEL: Record<string, LogLevel> = {
  error: "error",
  dpanic: "error",
  panic: "error",
  fatal: "error",
  warn: "warn",
  warning: "warn",
  info: "info",
  debug: "debug",
};

const JSON_LEVEL_RE = /"level"\s*:\s*"([a-zA-Z]+)"/;

const cache = new Map<string, LogLevel>();
const MAX_CACHE = 12000;

function detect(line: string): LogLevel {
  const m = JSON_LEVEL_RE.exec(line);
  if (m) return JSON_LEVEL[m[1]!.toLowerCase()] ?? "other";
  if (/\berror\b/i.test(line)) return "error";
  if (/\bwarn(ing)?\b/i.test(line)) return "warn";
  if (/\binfo\b/i.test(line)) return "info";
  if (/\bdebug\b/i.test(line)) return "debug";
  return "other";
}

/** 同一行会在计数与筛选里各算一次、滚动时又反复算，所以带一层按行文本的缓存 */
export function levelOf(line: string): LogLevel {
  const hit = cache.get(line);
  if (hit !== undefined) return hit;
  const level = detect(line);
  if (cache.size > MAX_CACHE) cache.clear();
  cache.set(line, level);
  return level;
}

/**
 * 工具栏级别分段（单选）此刻该亮哪一格。左栏那张卡是多选，两处共用同一个集合：
 * 一个都没勾 → ""（「全部」）；只勾一个 → 那一档；勾了两个以上 → 一个不对应任何分段选项的值，一格都不亮。
 *
 * 多选时不能回 ""：分段会把「全部」画成选中，而 el-segmented 的切换挂在原生 radio 的 change 上，
 * 点一个已经选中的 radio 不触发 change —— 列表明明按两个级别筛着，「全部」却亮着、点了也没反应。
 */
export const LEVEL_SEG_MIXED = "mixed";

export function levelSegValue(active: ReadonlySet<LogLevel>): string {
  if (active.size === 0) return "";
  if (active.size === 1) return [...active][0]!;
  return LEVEL_SEG_MIXED;
}
