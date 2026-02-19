import { GITHUB_NEW_ISSUE_URL } from "../../core/constants";
import { getMessages, t } from "../../core/i18n";
import type { CollectionSession } from "../../core/types";

export function generateIssueBody(session: CollectionSession): string {
  const zh = getMessages("zh-CN");
  const en = getMessages("en-US");
  const bi = (key: keyof typeof zh): string => `${zh[key]} / ${en[key]}`;
  const pageTypes = session.pages.map((page) => `- ${page.pageType}: ${page.url}`).join("\n");
  const timestamp = new Date().toISOString();

  return [
    `## ${bi("issue.siteInfo")}`,
    `- ${bi("issue.siteName")}: ${session.site.name}`,
    `- ${bi("issue.siteUrl")}: ${session.site.url}`,
    `- ${bi("issue.schema")}: ${session.site.schema}`,
    `- ${bi("issue.authMethod")}: ${session.site.authMethod}`,
    "",
    `## ${bi("issue.pages")}`,
    pageTypes || `${zh["issue.none"]} / ${en["issue.none"]}`,
    "",
    `## ${bi("issue.timeInfo")}`,
    `- ${bi("issue.sessionCreated")}: ${session.createdAt}`,
    `- ${bi("issue.exportedAt")}: ${timestamp}`,
    "",
    `## ${bi("issue.attachments")}`,
    `> ${zh["issue.attachmentsHint"]}`,
    `> ${en["issue.attachmentsHint"]}`,
    "",
    `## ${bi("issue.checklist")}`,
    `- [x] ${zh["issue.checkDownloaded"]} / ${en["issue.checkDownloaded"]}`,
    `- [ ] ${zh["issue.checkUploaded"]} / ${en["issue.checkUploaded"]}`,
    `- [x] ${zh["issue.checkSanitized"]} / ${en["issue.checkSanitized"]}`,
    "",
    "---",
    `> ${zh["issue.groupHint"]}`,
    `> ${en["issue.groupHint"]}`,
    "> - Telegram: https://t.me/+7YK2kmWIX0s1Nzdl",
    "> - QQ: 274984594",
  ].join("\n");
}

export async function createGitHubIssue(
  session: CollectionSession,
  _zipBlob: Blob,
): Promise<string> {
  const title = t("issue.title", session.site.name);
  const body = generateIssueBody(session);
  const params = new URLSearchParams({ title, body });
  const issueUrl = `${GITHUB_NEW_ISSUE_URL}?${params.toString()}`;
  window.open(issueUrl, "_blank", "noopener,noreferrer");
  return issueUrl;
}
