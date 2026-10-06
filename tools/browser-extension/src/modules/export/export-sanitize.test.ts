import JSZip from "jszip";
import { describe, expect, it } from "vitest";

import type { CollectionSession } from "../../core/types";
import { generateIssueBody } from "./github";
import { createExportZip } from "./zipper";

const SECRET = "0123456789abcdef0123";

/** 旧版本采集、存在本地的页面：地址与 HTML 都还带着 passkey */
function legacySession(): CollectionSession {
  return {
    id: "s1",
    site: {
      name: "tracker.example",
      url: "https://tracker.example",
      schema: "NexusPHP",
      authMethod: "cookie",
    },
    pages: [
      {
        pageType: "detail",
        url: `https://tracker.example/details.php?id=1&passkey=${SECRET}`,
        html: `<a href="download.php?id=1&passkey=${SECRET}">dl</a><input type="password" value="hunter2">`,
        capturedAt: "2026-10-06T00:00:00Z",
        detectedSchema: "NexusPHP",
      },
    ],
    createdAt: "2026-10-06T00:00:00Z",
    status: "complete",
  } as CollectionSession;
}

describe("导出前再脱敏一次", () => {
  it("Issue 正文里的页面地址不带 passkey", () => {
    expect(generateIssueBody(legacySession())).not.toContain(SECRET);
  });

  it("ZIP 里的 HTML 与 site-info.json 都不带凭证", async () => {
    const blob = await createExportZip(legacySession());
    const zip = await JSZip.loadAsync(await blob.arrayBuffer());
    const html = await zip.file("detail.html")!.async("string");
    const info = await zip.file("site-info.json")!.async("string");
    expect(html).not.toContain(SECRET);
    expect(html).not.toContain("hunter2");
    expect(info).not.toContain(SECRET);
  });
});
