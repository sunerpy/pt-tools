// @vitest-environment happy-dom
/*
 * 钉子（媒体中心 M7）：删除弹窗（画板 03 的片段「删除这一季」）。打开时读预览；库里的文件、种子（受保护的要勾「仍然删除」才能勾）、
 * 订阅三块与跟着勾选变的摘要；共用数据的只删种子、不能改；导入前就有的文件另外勾；删完以后每一步写结果，页面重新读；
 * 预览读不到时能重试；「处理」打开时写明要处理的问题。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import type { DeleteFile, DeletePreview, DeleteTorrent } from "@/api";
import { type MountedView, mountView } from "@/test-mount";
import type { DeleteTarget } from "@/utils/mediaDelete";

const api = vi.hoisted(() => ({ deletePreview: vi.fn(), deleteItem: vi.fn() }));
const ui = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, subscribeApi: api };
});
vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return { ...real, ElMessage: Object.assign(vi.fn(), ui) };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.resetAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);
const qa = (id: string) => [...document.querySelectorAll<HTMLElement>(`[data-testid=${id}]`)];
const text = (id: string) => (q(id)?.textContent ?? "").replace(/\s+/g, " ").trim();
const box = (el: HTMLElement) => el.querySelector<HTMLInputElement>("input")!;

const GB = 1024 ** 3;
const file = (id: number, over: Partial<DeleteFile> = {}): DeleteFile => ({
  id,
  path: `/media/tv/长夜将尽 (2025)/Season 02/S02E0${id}.mkv`,
  library: "剧集库",
  mode: "hardlink",
  existing: false,
  size: (41.2 / 6) * GB,
  season: 2,
  episode: id,
  episode_end: 0,
  state: "ours",
  deletable: true,
  ...over,
});
const torrent = (over: Partial<DeleteTorrent>): DeleteTorrent => ({
  site: "OurBits",
  title: "The.Long.Night.S02E01-E04.2160p-HHWEB",
  season: 2,
  episodes: "E01–E04",
  version: "2160p HHWEB",
  size: 27.3 * GB,
  downloader_id: 1,
  downloader: "qb",
  hash: "aaa",
  state: "seeding",
  percent: 100,
  ratio: 1.31,
  seeding_seconds: 18 * 86400,
  hr: true,
  hr_hours: 72,
  deletable: true,
  reason: "H&R 已满足",
  covers_more: false,
  shares: [],
  ...over,
});
const board: DeletePreview = {
  files: [1, 2, 3, 4, 5, 6].map((n) => file(n)),
  torrents: [
    torrent({}),
    torrent({
      site: "HDSky",
      episodes: "E05–E06",
      downloader_id: 2,
      downloader: "tr",
      hash: "bbb",
      deletable: false,
      reason: "H&R 还差 2 天",
      rule: "hr",
      why: "H&R 要求做种 7 天，还差 2 天",
      protection: "H&R：要求做种 168 小时，已做 120.0 小时",
    }),
    torrent({
      site: "HDSky",
      episodes: "E07",
      hash: "ccc",
      state: "downloading",
      percent: 63,
      deletable: false,
      reason: "受保护：下载中",
      rule: "downloading",
      why: "还在下载（63%）",
    }),
  ],
  subscriptions: [{ id: 5, status: "active", season: 2, follow: true }],
  issues: [],
  errors: [],
};

const itemTarget = (over: Partial<DeleteTarget> = {}): DeleteTarget => ({
  selector: { media_type: "tv", tmdb_id: 100088, season: 2 },
  title: "长夜将尽",
  preset: { kind: "item" },
  ...over,
});

async function mountDialog(target: DeleteTarget, preview: DeletePreview = board) {
  api.deletePreview.mockResolvedValue(preview);
  const Dialog = (await import("./MediaDeleteDialog.vue")).default;
  const done = vi.fn();
  const open = ref(true);
  const Host = defineComponent({
    setup() {
      return () =>
        h(Dialog, {
          modelValue: open.value,
          "onUpdate:modelValue": (v: boolean) => (open.value = v),
          target,
          onDone: done,
        });
    },
  });
  view = mountView(Host);
  await vi.waitFor(() => expect(q("delete-torrents") ?? q("delete-error")).not.toBeNull());
  await flush();
  return { done, open };
}

describe("删除弹窗", () => {
  it("大图打开：画板 03 的三块，默认勾库文件与能删的种子、暂停追剧；受保护的勾「仍然删除」才删；删完写每一步", async () => {
    const { done, open } = await mountDialog(itemTarget());
    expect(api.deletePreview).toHaveBeenCalledWith({
      media_type: "tv",
      tmdb_id: 100088,
      season: 2,
    });
    expect(document.querySelector(".md-dialog .el-dialog__title")!.textContent).toBe(
      "删除《长夜将尽》第 2 季",
    );
    expect(text("delete-files")).toContain("删除库里的文件");
    expect(text("delete-files")).toContain("6 个文件 · 41.2 GB · 都是硬链接，种子里的数据不受影响");
    expect(box(q("delete-files-check")!).checked).toBe(true);
    expect(q("delete-existing"), "没有导入前就有的文件").toBeNull();

    // 看文件：展开清单
    expect(q("delete-file-list")).toBeNull();
    q("delete-files-more")!.click();
    await flush();
    expect(q("delete-file-list")!.textContent).toContain("S02E01 · 剧集库 · 硬链接");
    expect(q("delete-files-more")!.textContent!.trim()).toBe("收起");

    expect(text("delete-torrents")).toContain("种子（每台下载器分开判断）");
    const rows = qa("delete-torrent");
    expect(rows.map((r) => r.querySelector(".md-row__title")!.textContent)).toEqual([
      "OurBits · S02E01–E04 · qb",
      "HDSky · S02E05–E06 · tr",
      "HDSky · S02E07 · qb",
    ]);
    expect(rows[0]!.textContent).toContain("分享率 1.31 · 做种 18 天 · H&R 已满足");
    expect(rows[1]!.textContent).toContain("受保护：H&R 要求做种 7 天，还差 2 天");
    expect(rows[2]!.textContent).toContain("受保护：还在下载（63%）");
    const checks = qa("delete-torrent-check").map(box);
    expect(checks.map((c) => c.checked)).toEqual([true, false, false]);
    expect(
      checks.map((c) => c.disabled),
      "受保护的不勾「仍然删除」不能勾",
    ).toEqual([false, true, true]);
    expect(qa("delete-torrent-how"), "能删的才有「连数据一起删」").toHaveLength(1);
    expect(rows[0]!.querySelector(".el-select")!.textContent).toContain("连数据一起删");

    expect(text("delete-subscription")).toContain("暂停追剧");
    expect(text("delete-subscription")).toContain(
      "推荐：不暂停的话，台账会把删掉的集当成缺的，再去下一遍",
    );
    expect(box(q("delete-sub-pause")!).checked).toBe(true);
    expect(text("delete-summary")).toBe("将删除 6 个库文件、1 个种子（连数据），暂停追剧");

    // 受保护的：勾「仍然删除」以后一并勾上，多出「连数据一起删」，摘要跟着变
    box(qa("delete-torrent-force")[0]!).click();
    await flush();
    expect(qa("delete-torrent-check").map(box)[1]!.checked).toBe(true);
    expect(qa("delete-torrent-check").map(box)[1]!.disabled).toBe(false);
    expect(qa("delete-torrent-how")).toHaveLength(2);
    expect(text("delete-summary")).toBe("将删除 6 个库文件、2 个种子（连数据），暂停追剧");

    // 订阅改成保留：摘要里不写订阅；点标题也能选
    q("delete-subscription")!.querySelectorAll<HTMLElement>(".md-sub__text")[2]!.click();
    await flush();
    expect(text("delete-summary")).toBe("将删除 6 个库文件、2 个种子（连数据）");

    api.deleteItem.mockResolvedValue({
      steps: [
        {
          kind: "torrent",
          target: "The.Long.Night.S02E01-E04",
          downloader_id: 1,
          hash: "aaa",
          ok: true,
          message: "连数据删了",
        },
        {
          kind: "torrent",
          target: "The.Long.Night.S02E05-E06",
          downloader_id: 2,
          hash: "bbb",
          ok: false,
          message: "删除失败：连不上",
        },
        {
          kind: "file",
          target: "/media/tv/E01.mkv",
          id: 1,
          ok: true,
          message: "删了库里的文件与整理记录",
        },
      ],
      failed: 1,
    });
    q("delete-run")!.click();
    await vi.waitFor(() => expect(q("delete-steps")).not.toBeNull());
    expect(api.deleteItem).toHaveBeenCalledWith({
      media_type: "tv",
      tmdb_id: 100088,
      season: 2,
      history_ids: [1, 2, 3, 4, 5, 6],
      allow_existing: false,
      torrents: [
        { downloader_id: 1, hash: "aaa", delete_data: true },
        { downloader_id: 2, hash: "bbb", delete_data: true },
      ],
      confirm_protected: [{ downloader_id: 2, hash: "bbb" }],
      subscription: "keep",
    });
    expect(done).toHaveBeenCalledTimes(1);
    expect(text("delete-result")).toBe("1 步没做成，原因写在下面；做成了的不受影响");
    const steps = qa("delete-step");
    expect(steps).toHaveLength(3);
    expect(steps[0]!.textContent).toContain("种子");
    expect(steps[1]!.classList.contains("is-fail")).toBe(true);
    expect(steps[1]!.textContent).toContain("删除失败：连不上");
    expect(steps[2]!.textContent).toContain("库文件");
    expect(q("delete-run"), "删完只剩「关闭」").toBeNull();
    q("delete-close")!.click();
    await flush();
    expect(open.value).toBe(false);
  });

  it("「删除这个种子」：只勾那一个，库文件不勾、订阅保留；什么都不勾时不能删", async () => {
    await mountDialog(
      itemTarget({ preset: { kind: "torrent", key: { downloader_id: 1, hash: "AAA" } } }),
    );
    expect(box(q("delete-files-check")!).checked).toBe(false);
    expect(qa("delete-torrent-check").map((c) => box(c).checked)).toEqual([true, false, false]);
    expect(box(q("delete-sub-keep")!).checked).toBe(true);
    expect(text("delete-summary")).toBe("将删除 1 个种子（连数据）");

    // 改成只删种子
    q("delete-torrent-how")!.querySelector<HTMLElement>(".el-select__wrapper")!.click();
    await flush();
    const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
      (el) => el.textContent?.trim() === "只删种子",
    )!;
    opt.click();
    await flush();
    expect(text("delete-summary")).toBe("将删除 1 个种子（只删种子）");

    box(qa("delete-torrent-check")[0]!).click();
    await flush();
    expect(text("delete-summary")).toBe("没有选要删的");
    expect((q("delete-run") as HTMLButtonElement).disabled).toBe(true);
  });

  it("共用数据的只删种子、不能改；种子里还有没选的集的默认不勾；导入前就有的另外勾；移动整理的写明不能删", async () => {
    const preview: DeletePreview = {
      ...board,
      files: [
        file(1),
        file(2, { mode: "move", deletable: false, note: "移动整理的文件是唯一的一份，不在这里删" }),
        file(3, { mode: "existing", existing: true }),
      ],
      torrents: [
        torrent({ shares: [{ hash: "ddd", name: "The.Long.Night.S02E01.Other" }] }),
        torrent({
          hash: "eee",
          episodes: "整季",
          covers_more: true,
          more: "整季包，还有这次没选的集",
        }),
      ],
      errors: ["读不到下载器「tr」里的种子：连不上"],
    };
    await mountDialog(
      itemTarget({ selector: { media_type: "tv", tmdb_id: 100088, season: 2, episodes: [1] } }),
      preview,
    );
    expect(document.querySelector(".md-dialog .el-dialog__title")!.textContent).toBe(
      "删除《长夜将尽》第 2 季 E01",
    );
    expect(text("delete-warn")).toContain("有几部分没读到：读不到下载器「tr」里的种子：连不上");
    expect(text("delete-files")).toContain("1 个是移动整理的，是唯一的一份，不在这里删");
    q("delete-files-more")!.click();
    await flush();
    expect(q("delete-file-list")!.querySelector(".is-warn")!.textContent).toContain(
      "移动整理的文件",
    );

    const rows = qa("delete-torrent");
    expect(q("delete-torrent-shared")!.textContent).toContain(
      "数据还有别的种子在用（The.Long.Night.S02E01.Other），只删种子",
    );
    const how = rows[0]!.querySelector(".el-select")!;
    expect(how.classList.contains("is-disabled") || how.querySelector(".is-disabled")).toBeTruthy();
    expect(how.textContent).toContain("只删种子");
    expect(text("delete-torrent-more")).toBe("整季包，还有这次没选的集，默认不勾");
    expect(qa("delete-torrent-check").map((c) => box(c).checked)).toEqual([true, false]);

    expect(text("delete-existing")).toContain("也删导入前就有的文件");
    expect(text("delete-existing")).toContain("1 个文件 · 6.87 GB · 导入前就在库里，删了就没有了");
    expect(box(q("delete-existing-check")!).checked).toBe(false);
    expect(text("delete-summary")).toBe("将删除 1 个库文件、1 个种子（只删种子），暂停追剧");
    box(q("delete-existing-check")!).click();
    await flush();
    expect(text("delete-summary")).toBe("将删除 2 个库文件、1 个种子（只删种子），暂停追剧");

    api.deleteItem.mockResolvedValue({ steps: [], failed: 0 });
    q("delete-run")!.click();
    await vi.waitFor(() => expect(api.deleteItem).toHaveBeenCalled());
    expect(api.deleteItem.mock.calls[0]![0]).toMatchObject({
      episodes: [1],
      history_ids: [1, 3],
      allow_existing: true,
      torrents: [{ downloader_id: 1, hash: "aaa", delete_data: false }],
    });
  });

  it("「处理」打开：最上面写要处理的问题；没有订阅时不显示订阅那一块；库里没有文件时不能勾", async () => {
    await mountDialog(
      itemTarget({
        selector: { media_type: "tv", tmdb_id: 100088, season: 2, episodes: [3] },
        preset: { kind: "issues" },
      }),
      {
        files: [],
        torrents: [],
        subscriptions: [],
        issues: [
          {
            id: 31,
            kind: "server_removed",
            status: "open",
            server_id: 1,
            server_name: "Jellyfin",
            server_item_id: "x",
            history_id: 0,
            media_type: "tv",
            tmdb_id: 100088,
            season: 2,
            episode: 3,
            title: "长夜将尽",
            detail: "",
            created_at: "",
            updated_at: "",
          },
        ],
        errors: [],
      },
    );
    expect(text("delete-issues")).toBe(
      "要处理的：S02E03 在 Jellyfin 里被删了。删掉库里的文件与整理记录以后，这些问题改成已处理。",
    );
    expect(q("delete-subscription")).toBeNull();
    expect(box(q("delete-files-check")!).disabled).toBe(true);
    expect(text("delete-files")).toContain("库里没有这几集的文件");
    expect(text("delete-torrents")).toContain("下载器里没有关联的种子");
    expect((q("delete-run") as HTMLButtonElement).disabled).toBe(true);
  });

  it("预览读不到：写明原因，能重试；删除失败时提示", async () => {
    api.deletePreview.mockRejectedValueOnce(new Error("整理入库服务没有启动"));
    const Dialog = (await import("./MediaDeleteDialog.vue")).default;
    view = mountView(
      defineComponent({ setup: () => () => h(Dialog, { modelValue: true, target: itemTarget() }) }),
    );
    await vi.waitFor(() => expect(q("delete-error")).not.toBeNull());
    expect(text("delete-error")).toContain("整理入库服务没有启动");
    api.deletePreview.mockResolvedValue(board);
    [...q("delete-error")!.querySelectorAll("button")]
      .find((b) => b.textContent === "重试")!
      .click();
    await vi.waitFor(() => expect(q("delete-torrents")).not.toBeNull());

    api.deleteItem.mockRejectedValueOnce(new Error("参数无效: 没有选要删的"));
    q("delete-run")!.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("参数无效: 没有选要删的"));
    expect(q("delete-steps")).toBeNull();
  });
});
