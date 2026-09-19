/*
 * 状态栏那格第三段（下载器版本）的归属判定。
 *
 * 抽成纯函数只为一件事：让「换下载器之后会不会串台」可测。
 * 一次评审查出的缺陷是 —— 版本只记一个字符串，而后端在问不到版本时**合法地**省掉该字段，
 * 于是「A 报过版本，切到问不到版本的 B」会显示成「B · 状态 · A 的版本」。
 * 判定规则因此必须带上「这条版本属于哪一台」。
 */

/** 记下来的版本：连着它属于哪一台 */
export interface HeldVersion {
  id: number;
  version: string;
}

/**
 * 这一格该显示的版本文本。对不上、或还没问到，都返回空串（前端就不画第三段）。
 */
export function versionFor(downloaderId: number | undefined, held: HeldVersion | null): string {
  if (downloaderId === undefined || held === null) return "";
  if (held.id !== downloaderId) return "";
  return held.version;
}

/**
 * 收到一轮 transfer-stats 之后，该记什么。
 *
 * - 这一轮带了版本：记下「这一台 + 这个版本」；
 * - 这一轮没带（后端问不到）：保留**同一台**的旧值，换台则丢弃 —— 旧值属于别人。
 */
export function nextHeldVersion(
  downloaderId: number | undefined,
  clientVersion: string | undefined,
  held: HeldVersion | null,
): HeldVersion | null {
  if (downloaderId === undefined) return null;
  if (clientVersion) return { id: downloaderId, version: clientVersion };
  if (held && held.id === downloaderId) return held;
  return null;
}
