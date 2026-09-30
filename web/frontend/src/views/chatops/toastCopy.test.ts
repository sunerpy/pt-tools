/// <reference types="node" />
/*
 * 钉子：通道详情「操作提示文案」卡与页面真实的 ElMessage 一条不多、一条不少。
 *
 * 这张卡自称列出「这一页会弹出的每一种提示」「与 ElMessage 的实参一字不差」。
 * 发布前的门禁抓到它漏了「测试消息发送失败：…」—— 改卡时删掉旧的失败行，没按真实调用补回来。
 * 规则：ElMessage 的字面量（含 `|| "兜底"`）都要在卡里；模板字符串里的 ${…} 在卡里写成「…」。
 */
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

const src = readFileSync(
  fileURLToPath(new URL("./NotificationDetail.vue", import.meta.url)),
  "utf8",
);
const script = src.slice(src.indexOf("<script"), src.indexOf("</script>"));

function cardTexts(): string[] {
  const start = script.indexOf("const toastCopy");
  const block = script.slice(start, script.indexOf("];", start));
  return [...block.matchAll(/text:\s*"([^"]+)"/g)].map((m) => m[1]!);
}

function toastTexts(): string[] {
  const out: string[] = [];
  /* ElMessage.success("…") / ElMessage.error((e as Error).message || "…") */
  for (const m of script.matchAll(/ElMessage\.(?:success|warning|error|info)\(([^;]*?)\);/g)) {
    const lit = /"([^"]+)"\s*$/.exec(m[1]!.trim());
    if (lit) out.push(lit[1]!);
  }
  /* ElMessage({ … message: `…${x}…` }) */
  for (const m of script.matchAll(/ElMessage\(\{[\s\S]*?message:\s*`([^`]+)`/g)) {
    out.push(m[1]!.replace(/\$\{[^}]+\}/g, "…"));
  }
  return out;
}

describe("通道详情的提示文案卡", () => {
  const card = cardTexts();
  const toasts = [...new Set(toastTexts())];

  it("扫到了卡与真实调用", () => {
    expect(card.length).toBeGreaterThan(3);
    expect(toasts.length).toBeGreaterThan(3);
  });

  it("页面会弹的每一条都列在卡里", () => {
    expect(toasts.filter((t) => !card.includes(t))).toEqual([]);
  });

  it("卡里没有页面根本不弹的条目", () => {
    expect(card.filter((t) => !toasts.includes(t))).toEqual([]);
  });
});
