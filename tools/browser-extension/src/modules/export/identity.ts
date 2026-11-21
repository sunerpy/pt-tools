import { getMessages, t } from "../../core/i18n";
import type { CollectionSession, UserIdentity } from "../../core/types";
import { extractOwnUserId } from "../collector/auto-collector";
import { extractOwnUsername } from "../collector/page-scan";
import { type IdentityPlan, planIdentity } from "../collector/sanitizer";

/**
 * 导出时要换掉的登录用户身份：一键采集存下的；没有时（手动逐页采集、旧版本的会话）从页面的 info_block 认。
 * 只用在本地脱敏，本身不导出。
 */
export function resolveIdentity(session: CollectionSession): UserIdentity | null {
  if (session.identity?.userId || session.identity?.username) {
    return session.identity;
  }
  const ordered = [
    ...session.pages.filter((page) => page.pageType === "index"),
    ...session.pages.filter((page) => page.pageType !== "index"),
  ];
  for (const page of ordered) {
    if ((page.format ?? "html") !== "html") continue;
    const userId = extractOwnUserId(page.html);
    if (userId) {
      return { userId, username: extractOwnUsername(page.html, userId) };
    }
  }
  return null;
}

/**
 * 导出时怎么换身份：按会话里的全部页面决定（见 planIdentity）。用户名是常见词、或在某一页里出现得太多时
 * 只换用户 ID，usernameKept 为 true，导出要提示用户提交前自己检查。
 */
export function planExportIdentity(session: CollectionSession): IdentityPlan {
  return planIdentity(
    resolveIdentity(session),
    session.pages.map((page) => page.html),
  );
}

/** 「用户名没有自动替换」那条提示的两种语言（采集时按当时的语言存进了 facts.warnings） */
function usernameNotes(): Set<string> {
  return new Set([
    getMessages("zh-CN")["collector.warnUsernameCommon"],
    getMessages("en-US")["collector.warnUsernameCommon"],
  ]);
}

/**
 * 导出时写进 site-info.json 的 warnings：采集时的提示，加上这次导出用户名没有自动替换的提示。
 * 采集时存下的那条（不管哪种语言）先去掉，再按这次导出的决定加一条现在语言的，不会重复。
 */
export function exportWarnings(session: CollectionSession, plan: IdentityPlan): string[] {
  const notes = usernameNotes();
  const kept = (session.facts?.warnings ?? []).filter((warning) => !notes.has(warning));
  return plan.usernameKept ? [...kept, t("collector.warnUsernameCommon")] : kept;
}

/** 导出时要让用户自己看一眼的提示（弹窗里显示）：用户名没有自动替换 */
export function exportNotes(session: CollectionSession): string[] {
  return planExportIdentity(session).usernameKept ? [t("collector.warnUsernameCommon")] : [];
}
