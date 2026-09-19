/*
 * 画板 29 工具栏那枚「最近 1 小时」。
 *
 * 这一档原先记在偏离表里，理由写的是「行首时间戳格式随编码器变（JSON 与 console 两种）」——
 * 这个理由是错的。`config/zap.go` 里四个日志**文件**（all/debug/info/error）用的都是
 * 同一个 JSON 编码器，`TimeKey: "time"` + ISO8601；console 那套编码器只写 stdout，
 * 而日志页 tail 的正是文件。所以每一行都带 `"time":"2026-09-19T10:00:00.000+0800"`，
 * 按时间切是可做的。
 *
 * 抽成纯函数的原因和状态栏版本号那次一样：可测。而且这里有一个必须测到的边界 ——
 * 一批日志里一行时间戳都解析不出来时，这枚 chip 不能把整页筛空，只能停用。
 */

/** 只认 JSON 编码器写出来的 `"time":"…"`；取不到返回 null（而不是 0 或 NaN） */
export function lineTime(line: string): number | null {
  const m = /"time"\s*:\s*"([^"]+)"/.exec(line);
  if (!m) return null;
  const t = Date.parse(m[1]!);
  return Number.isNaN(t) ? null : t;
}

/** 这批行里有没有可解析的时间戳 —— 没有就不该让用户点那枚 chip */
export function hasParsableTime(lines: readonly string[]): boolean {
  for (const line of lines) {
    if (lineTime(line) !== null) return true;
  }
  return false;
}

/**
 * 按时间窗筛行。
 *
 * - `sinceMs` 为 null：不筛（chip 没开）。
 * - 解析不出时间戳的行：**保留**。它可能是上一条日志的堆栈续行，
 *   按「时间未知」丢掉会把报错的上下文一起藏起来。
 */
export function withinWindow(lines: readonly string[], sinceMs: number | null): readonly string[] {
  if (sinceMs === null) return lines;
  return lines.filter((line) => {
    const t = lineTime(line);
    return t === null || t >= sinceMs;
  });
}
