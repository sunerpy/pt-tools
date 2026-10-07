// @vitest-environment happy-dom
/*
 * 钉子：整理入库弹窗。打开时预览（条目、媒体库、每个文件放到哪里）；识别不对时填 TMDB 编号、换库后重新预览，
 * 请求里只带填了的字段；整个种子整理不了时不能点整理；整理后写明结果，后台还在跑时说明稍后看历史。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  preview: vi.fn(),
  organize: vi.fn(),
  libraries: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, organizeApi: api };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);

const library = {
  id: 1,
  name: "电影",
  kind: "movie",
  anime: false,
  path: "/media/movies",
  template: "",
  mode: "hardlink",
  scrape: true,
  scrape_overwrite: false,
  enabled: true,
};

const plan = {
  downloader_id: 1,
  downloader_name: "qb",
  hash: "abc",
  task_id: "abc",
  name: "Oppenheimer.2023.2160p.BluRay-FRDS",
  save_path: "/downloads",
  local_path: "/data/downloads",
  mapped: true,
  match: {
    id: 872585,
    media_type: "movie",
    title: "奥本海默",
    original_title: "Oppenheimer",
    year: 2023,
    poster_path: "/p.jpg",
  },
  source: "search",
  library,
  mode: "hardlink",
  items: [
    {
      rel: "Oppenheimer/Oppenheimer.2023.2160p.mkv",
      source: "/data/downloads/Oppenheimer/Oppenheimer.2023.2160p.mkv",
      size: 1,
      target: "/media/movies/奥本海默 (2023)/奥本海默 (2023) - 2160p.mkv",
      subtitles: [{ source: "a.ass", target: "b.zh-CN.ass" }],
      status: "pending",
    },
  ],
  skipped: [
    { file: { rel: "Oppenheimer/Sample/s.mkv", path: "", size: 1 }, reason: "在「Sample」目录里" },
  ],
};

async function mountDialog() {
  api.preview.mockResolvedValue(plan);
  api.libraries.mockResolvedValue([
    { ...library, effective_template: "", preview: "", created_at: "", updated_at: "" },
    {
      ...library,
      id: 2,
      name: "剧集",
      kind: "tv",
      effective_template: "",
      preview: "",
      created_at: "",
      updated_at: "",
    },
  ]);
  const OrganizeDialog = (await import("./OrganizeDialog.vue")).default;
  const done = vi.fn();
  const Host = defineComponent({
    setup() {
      const open = ref(true);
      return () =>
        h(OrganizeDialog, {
          modelValue: open.value,
          "onUpdate:modelValue": (v: boolean) => (open.value = v),
          target: { downloader_id: 1, hash: "abc", name: "Oppenheimer" },
          onDone: done,
        });
    },
  });
  view = mountView(Host);
  await vi.waitFor(() => expect(q("organize-match")).not.toBeNull());
  return done;
}

describe("整理入库弹窗", () => {
  it("预览：条目、媒体库、目标路径（库内相对路径）、字幕与没选上的文件", async () => {
    await mountDialog();
    expect(api.preview).toHaveBeenCalledWith({ downloader_id: 1, hash: "abc" });
    expect(q("organize-match")!.textContent).toContain("奥本海默 (2023)");
    expect(q("organize-match")!.textContent).toContain("按名字搜索");
    expect(q("organize-library")!.textContent).toContain("媒体库：电影");
    expect(q("organize-library")!.textContent).toContain("硬链接");
    const items = q("organize-items")!.textContent ?? "";
    expect(items).toContain("待整理");
    expect(items).toContain("奥本海默 (2023)/奥本海默 (2023) - 2160p.mkv");
    expect(items, "目标路径去掉库目录").not.toContain("/media/movies/奥本海默");
    expect(items).toContain("1 个字幕");
    expect(q("organize-skipped")!.textContent).toContain("Sample");
    expect(document.body.textContent).toContain("pt-tools 里是 /data/downloads");
    expect((q("organize-run") as HTMLButtonElement).disabled).toBe(false);
    expect(q("organize-run")!.textContent).toContain("整理 1 个文件");
  });

  it("整理：写明结果并通知外面；后台还在跑时说明稍后看历史", async () => {
    const done = await mountDialog();
    api.organize.mockResolvedValue({
      plan: { ...plan, items: [{ ...plan.items[0], status: "done" }] },
      created: 1,
      done: 0,
      skipped: 0,
      failed: 0,
      messages: ["刮削：fanart.jpg: boom"],
    });
    q("organize-run")!.click();
    await vi.waitFor(() => expect(q("organize-result")).not.toBeNull());
    expect(api.organize).toHaveBeenCalledWith({ downloader_id: 1, hash: "abc" });
    expect(q("organize-result")!.textContent).toContain("新入库 1 个");
    expect(q("organize-result")!.textContent).toContain("刮削：fanart.jpg: boom");
    expect(done).toHaveBeenCalledTimes(1);
    expect((q("organize-run") as HTMLButtonElement).disabled, "都整理完了").toBe(true);

    view?.unmount();
    await mountDialog();
    api.organize.mockResolvedValue({ created: 0, done: 0, skipped: 0, failed: 0, queued: true });
    q("organize-run")!.click();
    await vi.waitFor(() => expect(q("organize-result")).not.toBeNull());
    expect(q("organize-result")!.textContent).toContain("稍后在「整理历史」里看结果");
  });

  it("识别不对：填 TMDB 编号、换库后重新预览；整理不了时不能点整理", async () => {
    await mountDialog();
    api.preview.mockResolvedValue({
      ...plan,
      problem: "没有启用的剧集媒体库：先在「媒体库」里添加",
      items: [],
    });
    const typeSel = q("organize-type")!;
    expect(typeSel).not.toBeNull();
    const id = q("organize-tmdb-id")!.querySelector("input")!;
    id.value = "100088";
    id.dispatchEvent(new Event("input"));
    id.dispatchEvent(new Event("change"));
    await flush();
    q("organize-repreview")!.click();
    await vi.waitFor(() => expect(api.preview).toHaveBeenCalledTimes(2));
    expect(api.preview.mock.calls[1]![0]).toEqual({
      downloader_id: 1,
      hash: "abc",
      media_type: "movie",
      tmdb_id: 100088,
    });
    await vi.waitFor(() => expect(q("organize-problem")).not.toBeNull());
    expect(q("organize-problem")!.textContent).toContain("没有启用的剧集媒体库");
    expect((q("organize-run") as HTMLButtonElement).disabled).toBe(true);
  });

  it("预览失败时写明原因", async () => {
    api.preview.mockRejectedValue(new Error("下载器「qb」里没有这个种子"));
    api.libraries.mockResolvedValue([]);
    const OrganizeDialog = (await import("./OrganizeDialog.vue")).default;
    view = mountView(
      defineComponent({
        setup: () => () =>
          h(OrganizeDialog, { modelValue: true, target: { downloader_id: 1, hash: "x" } }),
      }),
    );
    await vi.waitFor(() => expect(q("organize-error")).not.toBeNull());
    expect(q("organize-error")!.textContent).toContain("没有这个种子");
    expect((q("organize-run") as HTMLButtonElement).disabled).toBe(true);
  });
});
