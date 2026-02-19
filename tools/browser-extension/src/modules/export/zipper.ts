import JSZip from "jszip";

import type { CollectionSession, PageType } from "../../core/types";

const PAGE_FILE_NAMES: Partial<Record<PageType, string>> = {
  search: "search.html",
  detail: "detail.html",
  userinfo: "userinfo.html",
  index: "index.html",
  bonus: "bonus.html",
  api_response: "api-response.html",
  unknown: "unknown.html",
};

export async function createExportZip(session: CollectionSession): Promise<Blob> {
  const zip = new JSZip();
  const counters = new Map<string, number>();

  for (const page of session.pages) {
    const base = PAGE_FILE_NAMES[page.pageType] ?? "page.html";
    const count = counters.get(base) ?? 0;
    counters.set(base, count + 1);
    const fileName = count === 0 ? base : base.replace(".html", `-${count + 1}.html`);
    zip.file(fileName, page.html);
  }

  const metadata = {
    id: session.id,
    site: session.site,
    status: session.status,
    createdAt: session.createdAt,
    pages: session.pages.map((page) => ({
      pageType: page.pageType,
      url: page.url,
      capturedAt: page.capturedAt,
      detectedSchema: page.detectedSchema,
    })),
  };

  zip.file("site-info.json", JSON.stringify(metadata, null, 2));
  return zip.generateAsync({ type: "blob" });
}

export function downloadZip(blob: Blob, filename: string): void {
  const objectUrl = URL.createObjectURL(blob);
  const anchor = document.createElement("a");
  anchor.href = objectUrl;
  anchor.download = filename;
  anchor.click();
  URL.revokeObjectURL(objectUrl);
}
