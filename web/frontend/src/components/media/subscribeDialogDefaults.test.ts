// @vitest-environment happy-dom
/*
 * 钉子：订阅弹窗按媒体库带出下载设置（追剧方案 3.1.3、3.1.4）。新建时读草稿：「按类型自动选」写明会整理进哪个库，
 * 按生效的库带出下载器、保存目录、分类、标签并写明来源；自己改过的项换库也不动，可以「换成媒体库的」；库里的下载器停用了
 * 不带出；修改时不自动带出，换了库出「套用」；媒体库只列同类型、启用着的库，电影也能选。2026-11-07 用户：「选择了媒体库
 * 剧集 类型后，应该支持自动带出一些设置的比如保存目录、分类、标签等内容」。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  profiles: vi.fn(async () => ({ items: [] })),
  settings: vi.fn(async () => ({})),
  create: vi.fn(),
  update: vi.fn(),
  previewSchedule: vi.fn(async () => ({ next: [] })),
  episodeGroups: vi.fn(async () => ({ groups: [] })),
  draft: vi.fn(),
}));

const DLS = [
  {
    id: 1,
    name: "qB",
    type: "qbittorrent",
    url: "",
    username: "",
    is_default: true,
    enabled: true,
  },
  {
    id: 2,
    name: "qB-2",
    type: "qbittorrent",
    url: "",
    username: "",
    is_default: false,
    enabled: false,
  },
];

const lib = (id: number, name: string, extra: Record<string, unknown> = {}) => ({
  id,
  name,
  kind: "tv",
  anime: false,
  enabled: true,
  path: `/media/${id}`,
  download_downloader_id: 0,
  download_save_path: "",
  download_category: "",
  download_tags: "",
  ...extra,
});
const LIBS = [
  lib(1, "剧集", {
    download_downloader_id: 1,
    download_save_path: "/downloads/tv",
    download_category: "tv",
    download_tags: "pt-tools",
  }),
  lib(2, "动漫", {
    anime: true,
    download_save_path: "/downloads/anime",
    download_category: "anime",
  }),
  lib(3, "剧集 2", {
    download_downloader_id: 2,
    download_save_path: "/downloads/tv2",
    download_category: "tv2",
  }),
  lib(4, "停用的剧集", { enabled: false, download_category: "off" }),
  lib(5, "电影", {
    kind: "movie",
    download_save_path: "/downloads/movies",
    download_category: "movies",
  }),
];

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    subscribeApi: { ...real.subscribeApi, ...api },
    downloadersApi: { list: vi.fn(async () => DLS) },
    downloaderDirectoriesApi: { listAll: vi.fn(async () => ({})) },
    sitesApi: { list: vi.fn(async () => ({})) },
    organizeApi: { ...real.organizeApi, libraries: vi.fn(async () => LIBS) },
    mediaApi: { ...real.mediaApi, settings: vi.fn(async () => ({ image_base_url: "" })) },
  };
});

const draftOf = (extra: Record<string, unknown> = {}) => ({
  media_type: "tv",
  tmdb_id: 7,
  season: 2,
  seasons: 2,
  total_episodes: 3,
  title: "乘风",
  original_title: "Ride",
  year: 2026,
  poster_path: "",
  anime: false,
  auto_library_id: 1,
  subscription_id: 0,
  ...extra,
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);
const text = (id: string) => (q(id)?.textContent ?? "").replace(/\s+/g, " ").trim();
function inputOf(id: string): HTMLInputElement {
  const el = q(id);
  if (!el) throw new Error(`找不到 ${id}`);
  return (el instanceof HTMLInputElement ? el : el.querySelector("input")) as HTMLInputElement;
}
function fill(id: string, v: string) {
  const i = inputOf(id);
  i.value = v;
  i.dispatchEvent(new Event("input"));
}

function options(id: string): () => HTMLElement[] {
  const sel = q(id)!;
  (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
  const listId = sel.querySelector("input")?.getAttribute("aria-controls");
  return () => [
    ...(document
      .getElementById(listId ?? "")
      ?.querySelectorAll<HTMLElement>(".el-select-dropdown__item") ?? []),
  ];
}
const optionTexts = (list: () => HTMLElement[]) => list().map((o) => (o.textContent ?? "").trim());

async function pick(id: string, label: string) {
  const list = options(id);
  await vi.waitFor(() => {
    const opt = list().find((o) => (o.textContent ?? "").trim() === label);
    expect(opt, `下拉里有「${label}」`).toBeTruthy();
    opt!.click();
  });
  await flush();
}

/** 下拉框现在显示的文字 */
const shown = (id: string) =>
  (q(id)?.querySelector(".el-select__placeholder")?.textContent ?? "").trim();

async function mountDialog(
  props: Record<string, unknown> = { target: { kind: "tv", tmdb_id: 7, title: "乘风" } },
) {
  const Dialog = (await import("./SubscribeDialog.vue")).default;
  const Host = defineComponent({
    setup() {
      const open = ref(true);
      return () =>
        h(Dialog, {
          modelValue: open.value,
          "onUpdate:modelValue": (v: boolean) => (open.value = v),
          ...props,
        });
    },
  });
  view = mountView(Host);
  await vi.waitFor(() => expect(q("sd-library")).not.toBeNull());
  await flush();
}

describe("订阅弹窗：按媒体库带出下载设置", () => {
  it("新建剧集：按类型自动选写明整理进哪个库，四项带出并写明来源；只列同类型、启用着的库", async () => {
    api.draft.mockResolvedValue(draftOf());
    await mountDialog();
    expect(api.draft).toHaveBeenCalledWith({ media_type: "tv", tmdb_id: 7, season: 0 });
    await vi.waitFor(() => expect(inputOf("sd-category").value).toBe("tv"));
    expect(shown("sd-library")).toBe("按类型自动选（会整理进：剧集）");
    expect(text("sd-sub")).toBe("剧集 · 一共 2 季 · 第 2 季 3 集");
    expect(inputOf("sd-season").getAttribute("placeholder")).toBe("最新一季（第 2 季）");
    expect(shown("sd-downloader")).toBe("qB");
    expect(inputOf("sd-save-path").value).toBe("/downloads/tv");
    expect(inputOf("sd-tags").value).toBe("pt-tools");
    expect(text("sd-src-category")).toBe("来自媒体库「剧集」");
    expect(text("sd-src-downloader")).toBe("来自媒体库「剧集」");
    expect(q("sd-already")).toBeNull();

    const list = options("sd-library");
    await vi.waitFor(() =>
      expect(optionTexts(list)).toEqual([
        "按类型自动选（会整理进：剧集）",
        "剧集",
        "动漫",
        "剧集 2",
      ]),
    );
    expect(optionTexts(options("sd-downloader")), "停用的下载器标出来").toContain("qB-2（已停用）");

    // 带出的四项紧跟在媒体库下面，选了库马上看得到；怎么找（档案、站点、包含排除）放在后面
    const order = [
      "sd-library",
      "sd-downloader",
      "sd-category",
      "sd-tags",
      "sd-save-path",
      "sd-profile",
      "sd-include",
    ].map((id) => q(id)!);
    for (let i = 1; i < order.length; i++) {
      expect(
        order[i - 1]!.compareDocumentPosition(order[i]!) & Node.DOCUMENT_POSITION_FOLLOWING,
        `${order[i - 1]!.dataset.testid} 在 ${order[i]!.dataset.testid} 前面`,
      ).toBeTruthy();
    }
  });

  it("自己改过的项换库不动，可以换成媒体库的；库里的下载器停用了不带出；保存照表单发出", async () => {
    api.draft.mockResolvedValue(draftOf());
    api.create.mockResolvedValue({ id: 9, title: "乘风" });
    await mountDialog();
    await vi.waitFor(() => expect(inputOf("sd-category").value).toBe("tv"));

    fill("sd-category", "mine");
    await flush();
    expect(q("sd-src-category")?.textContent ?? "", "自己填的不写来源").not.toContain("来自");
    await pick("sd-library", "剧集 2");
    expect(inputOf("sd-category").value, "自己填的不换").toBe("mine");
    expect(inputOf("sd-save-path").value).toBe("/downloads/tv2");
    expect(inputOf("sd-tags").value, "新库没设的项清空").toBe("");
    expect(text("sd-src-tags")).toBe("媒体库「剧集 2」没设，不加标签");
    expect(shown("sd-downloader")).toBe("用设置里的默认下载器");
    expect(text("sd-src-downloader")).toBe(
      "媒体库「剧集 2」设的下载器「qB-2」已停用，用设置里的默认下载器",
    );
    expect(text("sd-reset-category")).toBe("换成媒体库「剧集 2」的");

    // 改回自动选：来自库的项跟着换回去，自己填的仍不动
    await pick("sd-library", "按类型自动选（会整理进：剧集）");
    expect(inputOf("sd-save-path").value).toBe("/downloads/tv");
    expect(inputOf("sd-category").value).toBe("mine");
    await pick("sd-library", "剧集 2");

    q("sd-reset-category")!.click();
    await flush();
    expect(inputOf("sd-category").value).toBe("tv2");
    expect(text("sd-src-category")).toBe("来自媒体库「剧集 2」");
    expect(q("sd-reset-category")).toBeNull();

    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toMatchObject({
      media_type: "tv",
      tmdb_id: 7,
      library_id: 3,
      downloader_id: 0,
      save_path: "/downloads/tv2",
      category: "tv2",
      tags: "",
    });
  });

  it("清空也算自己填的：换库以后保持空", async () => {
    api.draft.mockResolvedValue(draftOf());
    await mountDialog();
    await vi.waitFor(() => expect(inputOf("sd-tags").value).toBe("pt-tools"));
    fill("sd-tags", "");
    await flush();
    await pick("sd-library", "动漫");
    expect(inputOf("sd-tags").value).toBe("");
    expect(inputOf("sd-category").value).toBe("anime");
    expect(text("sd-reset-tags"), "库里的标签是空的，和现在一样时不用换").toBe("");
  });

  it("动画按类型进动漫库，带出动漫库的设置", async () => {
    api.draft.mockResolvedValue(draftOf({ anime: true, auto_library_id: 2 }));
    await mountDialog();
    await vi.waitFor(() => expect(inputOf("sd-category").value).toBe("anime"));
    expect(shown("sd-library")).toBe("按类型自动选（会整理进：动漫）");
    expect(inputOf("sd-save-path").value).toBe("/downloads/anime");
    expect(shown("sd-downloader")).toBe("用设置里的默认下载器");
    expect(text("sd-src-downloader")).toBe("媒体库「动漫」没设下载器，用设置里的默认下载器");
  });

  it("没有能用的库时写明整理会失败，不带出；读草稿前自己填的不被覆盖", async () => {
    let resolve: (v: unknown) => void = () => {};
    api.draft.mockReturnValue(new Promise((r) => (resolve = r)));
    await mountDialog();
    fill("sd-category", "早填的");
    await flush();
    resolve(draftOf({ auto_library_id: 0 }));
    await vi.waitFor(() =>
      expect(shown("sd-library")).toBe("按类型自动选（没有启用的剧集媒体库，整理会失败）"),
    );
    expect(inputOf("sd-category").value).toBe("早填的");
    expect(inputOf("sd-save-path").value).toBe("");
  });

  it("改了季重新读草稿；已经在追的这一季写明；TMDB 上没有这一季时写明原因", async () => {
    api.draft.mockResolvedValue(draftOf({ subscription_id: 12 }));
    await mountDialog();
    await vi.waitFor(() => expect(q("sd-already")).not.toBeNull());
    expect(text("sd-already")).toContain("这一季已经在追了");

    api.draft.mockRejectedValue(new Error("参数不对: TMDB 上「乘风」没有第 5 季"));
    fill("sd-season", "5");
    await vi.waitFor(() =>
      expect(api.draft).toHaveBeenLastCalledWith({ media_type: "tv", tmdb_id: 7, season: 5 }),
    );
    await vi.waitFor(() => expect(text("sd-draft-error")).toBe("TMDB 上「乘风」没有第 5 季"));
    expect(q("sd-already")).toBeNull();
  });

  it("电影也能选媒体库：只列电影库，新建时带上 library_id", async () => {
    api.draft.mockResolvedValue(
      draftOf({
        media_type: "movie",
        tmdb_id: 278,
        season: 0,
        seasons: 0,
        total_episodes: 0,
        auto_library_id: 5,
      }),
    );
    api.create.mockResolvedValue({ id: 9, title: "肖申克的救赎" });
    await mountDialog({ target: { kind: "movie", tmdb_id: 278, title: "肖申克的救赎" } });
    expect(api.draft).toHaveBeenCalledWith({ media_type: "movie", tmdb_id: 278, season: 0 });
    await vi.waitFor(() => expect(inputOf("sd-category").value).toBe("movies"));
    expect(text("sd-sub")).toBe("电影");
    const list = options("sd-library");
    await vi.waitFor(() =>
      expect(optionTexts(list)).toEqual(["按类型自动选（会整理进：电影）", "电影"]),
    );
    await pick("sd-library", "电影");
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toMatchObject({
      media_type: "movie",
      library_id: 5,
      save_path: "/downloads/movies",
      category: "movies",
    });
  });

  it("修改：不自动带出、没改库不带 library_id；换了库出「套用」，点了才换，停用的下载器不换", async () => {
    const sub = {
      id: 4,
      media_type: "tv",
      tmdb_id: 7,
      season: 2,
      title: "乘风",
      year: 2026,
      status: "active",
      profile_id: 0,
      sites: [],
      downloader_id: 1,
      category: "old",
      tags: "x",
      save_path: "/data/old",
      upgrade: false,
      include_keywords: "",
      exclude_keywords: "",
      lock_version: true,
      schedule: { kind: "airtime" },
      library_id: 1,
    };
    api.update.mockResolvedValue(sub);
    await mountDialog({ sub });
    expect(api.draft, "修改时不读草稿").not.toHaveBeenCalled();
    expect(inputOf("sd-category").value).toBe("old");
    expect(inputOf("sd-save-path").value, "库的设置不一样也不自动换").toBe("/data/old");
    expect(q("sd-apply-bar")).toBeNull();
    expect(q("sd-src-category")).toBeNull();
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    expect(api.update.mock.calls[0]![1]).not.toHaveProperty("library_id");

    await pick("sd-library", "剧集 2");
    expect(inputOf("sd-category").value, "换了库也不自动换").toBe("old");
    expect(text("sd-apply-bar")).toContain("套用「剧集 2」的下载设置");
    q("sd-apply")!.click();
    await flush();
    expect(inputOf("sd-category").value).toBe("tv2");
    expect(inputOf("sd-save-path").value).toBe("/downloads/tv2");
    expect(inputOf("sd-tags").value).toBe("");
    expect(shown("sd-downloader"), "库里的下载器停用了：保留订阅原来的").toBe("qB");
    expect(q("sd-apply-bar")).toBeNull();
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(2));
    expect(api.update.mock.calls[1]![1]).toMatchObject({
      library_id: 3,
      downloader_id: 1,
      save_path: "/downloads/tv2",
      category: "tv2",
      tags: "",
    });
  });

  it("修改：订阅选的库停用了以后照样列出来，标「已停用」", async () => {
    const sub = {
      id: 5,
      media_type: "tv",
      tmdb_id: 7,
      season: 1,
      title: "乘风",
      status: "active",
      profile_id: 0,
      sites: [],
      downloader_id: 0,
      category: "",
      tags: "",
      save_path: "",
      upgrade: false,
      lock_version: true,
      schedule: { kind: "airtime" },
      library_id: 4,
    };
    await mountDialog({ sub });
    await vi.waitFor(() => expect(shown("sd-library")).toBe("停用的剧集（已停用）"));
  });
});
