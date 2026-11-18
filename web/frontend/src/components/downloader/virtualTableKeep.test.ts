// @vitest-environment happy-dom
// 虚拟滚动表格（「全部」视图）同样标「保留」，右键菜单按行给「保留」或「取消保留」。
import { afterEach, describe, expect, it } from "vitest";
import { createApp, h } from "vue";

import type { DownloaderTorrentItem } from "@/api";
import DownloaderTorrentVirtualTable from "./DownloaderTorrentVirtualTable.vue";

function row(id: string, tags: string): DownloaderTorrentItem {
  return {
    downloader_id: 1,
    downloader_name: "qb",
    downloader_type: "qbittorrent",
    task_id: id,
    info_hash: id,
    title: `任务${id}`,
    progress: 100,
    seeds: 1,
    connections: 1,
    size: 1,
    added_at: 0,
    completed_at: 0,
    ratio: 1,
    state: "seeding",
    save_path: "/d",
    category: "",
    tags,
    upload_speed: 0,
    download_speed: 0,
    eta: 0,
  };
}

let unmount: (() => void) | null = null;

afterEach(() => {
  unmount?.();
  unmount = null;
  document.body.innerHTML = "";
});

describe("虚拟滚动表格：保留", () => {
  it("标题前标「保留」，右键保留着的行给「取消保留」", async () => {
    const actions: { action: string; id: string }[] = [];
    const data = [row("a", ""), row("b", "x,pt-tools-keep")];
    document.body.innerHTML = '<div id="app"></div>';
    const app = createApp({
      render: () =>
        h(DownloaderTorrentVirtualTable, {
          data,
          allData: data,
          selectedRowKeys: [],
          visibleColumns: ["title"],
          columnOrder: ["title"],
          density: "compact",
          sortBy: "",
          sortOrder: "desc",
          onContextAction: (p: { action: string; row: DownloaderTorrentItem }) =>
            actions.push({ action: p.action, id: p.row.task_id }),
        }),
    });
    app.mount("#app");
    unmount = () => app.unmount();

    const marks = document.querySelectorAll('[data-testid="row-keep-mark"]');
    expect(marks).toHaveLength(1);
    expect(marks[0]!.closest(".vt-row")!.textContent).toContain("任务b");

    const rows = document.querySelectorAll(".vt-row");
    rows[1]!.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true }));
    await new Promise((r) => setTimeout(r, 0));
    const menu = document.querySelector(".table-context-menu")!;
    const unkeep = [...menu.querySelectorAll("button")].find(
      (b) => b.textContent?.trim() === "取消保留",
    )!;
    unkeep.click();
    rows[0]!.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true }));
    await new Promise((r) => setTimeout(r, 0));
    const keep = [
      ...document.querySelector(".table-context-menu")!.querySelectorAll("button"),
    ].find((b) => b.textContent?.trim() === "保留")!;
    keep.click();
    expect(actions).toEqual([
      { action: "unkeep", id: "b" },
      { action: "keep", id: "a" },
    ]);
  });
});
