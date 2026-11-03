/*
 * 导入已有媒体库（媒体中心 M5）用到的纯函数：一项能不能导入、季集的一句话、导入结果的一句话。
 */
import type { MediaImportFile, MediaImportItem, MediaImportRun } from "@/api";

/** 一项的情况：能导入（认出了条目、有没有记录的视频）、已经有记录（视频都有整理记录）、导入不了 */
export type ImportItemState = "ready" | "recorded" | "problem";

export function importItemState(it: MediaImportItem): ImportItemState {
  if (it.problem || !it.match) return "problem";
  return it.ready > 0 ? "ready" : "recorded";
}

/** 连续的集数写成区间：1、2、3、5 → 1–3、5 */
function ranges(nums: number[]): string {
  const sorted = [...new Set(nums)].sort((a, b) => a - b);
  const out: string[] = [];
  for (let i = 0; i < sorted.length;) {
    let j = i;
    while (j + 1 < sorted.length && sorted[j + 1] === sorted[j] + 1) j++;
    out.push(i === j ? `${sorted[i]}` : `${sorted[i]}–${sorted[j]}`);
    i = j + 1;
  }
  return out.join("、");
}

/** 剧集的季集：「第 1 季 1–10 集、第 2 季 1–3 集、特别篇 1 集」；没认出集数的不算。列出的视频不全时说明 */
export function importEpisodes(files: MediaImportFile[], total = files.length): string {
  const bySeason = new Map<number, number[]>();
  for (const f of files) {
    if (!f.episode) continue;
    const eps = bySeason.get(f.season ?? 0) ?? [];
    for (let n = f.episode; n <= Math.max(f.episode_end ?? 0, f.episode); n++) eps.push(n);
    bySeason.set(f.season ?? 0, eps);
  }
  const parts = [...bySeason.entries()]
    .sort(([a], [b]) => (a === 0 ? 1 : b === 0 ? -1 : a - b))
    .map(([s, eps]) => `${s === 0 ? "特别篇" : `第 ${s} 季`} ${ranges(eps)} 集`);
  if (!parts.length) return "";
  return parts.join("、") + (total > files.length ? "（只列了前面的视频）" : "");
}

/** 导入的结果 */
export function importRunText(run: MediaImportRun): string {
  if (run.status === "queued") return `排进了整理队列，等前面的整理做完就导入（${run.items} 项）`;
  if (run.status === "running") return `正在导入：${run.done} / ${run.items} 项`;
  const parts = [`导入了 ${run.created} 个视频`];
  if (run.skipped) parts.push(`${run.skipped} 个已经有整理记录，跳过了`);
  if (run.failed) parts.push(`${run.failed} 处导入不了（原因见下面）`);
  if (run.scraped) parts.push(`补刮了 ${run.scraped} 部`);
  const head = run.status === "failed" ? "没导入完：" : "";
  return head + parts.join("，");
}
