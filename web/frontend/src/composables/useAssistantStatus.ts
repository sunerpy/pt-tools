import { readonly, ref } from "vue";
import { assistantApi, type AssistantStatus } from "../api";

/**
 * AI 助手的启用状态（/api/assistant/status），整个页面共用一份：每一页右下角的悬浮按钮看它决定出不出现。
 *
 * 外壳挂载时读一次；AI 助手设置保存以后再读一次，开关一改按钮就跟着出现或消失，不用刷新页面。
 * 读失败时保持原样（第一次就读不到的话按钮不出现），网络抖一下不会让按钮闪没。
 *
 * 写成模块级的 ref 而不是 Pinia store：设置页的测试没有装 Pinia，保存后顺手刷新一次不该要求每个挂它的测试都补 Pinia。
 */
const status = ref<AssistantStatus | null>(null);
let seq = 0;

export const assistantStatus = readonly(status);

export async function refreshAssistantStatus(): Promise<void> {
  const my = ++seq;
  try {
    const st = await assistantApi.status();
    if (my === seq) status.value = st;
  } catch {
    /* 读不到：保持原样 */
  }
}

/** 只给测试用：回到没读过的状态 */
export function resetAssistantStatusForTest(): void {
  seq++;
  status.value = null;
}
