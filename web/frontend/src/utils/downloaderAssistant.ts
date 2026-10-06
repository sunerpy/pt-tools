import type { AssistantApplyResult, AssistantDeadReason } from "@/api";

export const DEAD_REASON_LABEL: Record<AssistantDeadReason, string> = {
  unregistered: "未注册",
  not_found: "不存在",
};

export function deadReasonLabel(reason: AssistantDeadReason | string): string {
  return DEAD_REASON_LABEL[reason as AssistantDeadReason] ?? reason;
}

/** 执行结果的一句话：成功几个、跳过几个、失败几个，再带上第一条失败原因 */
export function applySummary(verb: string, res: AssistantApplyResult): string {
  const parts = [`${verb} ${res.done} 个`];
  if (res.skipped.length) parts.push(`跳过 ${res.skipped.length} 个`);
  if (res.failed.length) {
    const first = res.failed[0];
    parts.push(`失败 ${res.failed.length} 个（${first.name || first.hash}：${first.error}）`);
  }
  return parts.join("，");
}

/** 结果的提示级别：全部成功是 success，有失败是 warning，什么都没做是 info */
export function applyTone(res: AssistantApplyResult): "success" | "warning" | "info" {
  if (res.failed.length) return "warning";
  if (res.done === 0) return "info";
  return "success";
}
