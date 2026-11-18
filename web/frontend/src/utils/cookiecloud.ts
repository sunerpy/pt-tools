import type { CookieCloudImportResult, CookieCloudPreviewItem } from "@/api";

type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

/** 预览里一个站点的情况：没启用的导入后会启用；启用了的看 Cookie 有没有变化 */
export function cookieCloudItemStatus(it: Pick<CookieCloudPreviewItem, "enabled" | "changed">): {
  label: string;
  tone: Tone;
} {
  if (!it.enabled) return { label: "没有启用", tone: "info" };
  if (it.changed) return { label: "Cookie 有变化", tone: "warn" };
  return { label: "没有变化", tone: "neutral" };
}

/** 导入结果的一句话（与后端的 Summary 一致） */
export function cookieCloudImportSummary(r: CookieCloudImportResult): string {
  const parts: string[] = [];
  if (r.imported.length)
    parts.push(`更新了 ${r.imported.length} 个站点的 Cookie（${r.imported.join("、")}）`);
  if (r.unchanged.length) parts.push(`${r.unchanged.length} 个没有变化`);
  if (r.missing.length)
    parts.push(`${r.missing.length} 个在 CookieCloud 里没有找到（${r.missing.join("、")}）`);
  if (r.failed.length)
    parts.push(`${r.failed.length} 个写入失败（${r.failed.map((f) => f.site).join("、")}）`);
  return parts.length ? parts.join("，") : "没有可以更新的站点";
}

/** 提示级别：有写入失败或没找到的是 warning，什么都没写是 info */
export function cookieCloudImportTone(r: CookieCloudImportResult): "success" | "warning" | "info" {
  if (r.failed.length || r.missing.length) return "warning";
  if (!r.imported.length) return "info";
  return "success";
}
