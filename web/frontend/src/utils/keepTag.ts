/**
 * 「保留」标签（后端 models.KeepTag）：带它的种子，自动删种（含紧急清理）、做种竞争度、免费结束时的删除、
 * 刷流删种都不删。下载器页的种子列表一键加、去；Transmission 里它是一个 label。
 */
export const KEEP_TAG = "pt-tools-keep";

/** 行上「保留」标记的悬停说明。 */
export const KEEP_TIP =
  "带 pt-tools-keep 标签：自动删种（含紧急清理）、做种竞争度、免费结束时的删除和刷流删种都不删它。" +
  "刷流任务的种子点了保留以后不再归刷流管，取消保留也不回到刷流，按自动删种的规则处理。";

/** 种子的标签里有没有保留标签（逗号分隔，大小写不敏感）。 */
export function hasKeepTag(tags?: string | null): boolean {
  return (tags ?? "").split(",").some((tag) => tag.trim().toLowerCase() === KEEP_TAG);
}
