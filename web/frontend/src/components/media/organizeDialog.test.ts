// @vitest-environment happy-dom
/*
 * 钉子：整理入库弹窗。打开时预览（条目、媒体库、每个文件放到哪里）；识别不对时填 TMDB 编号、换库后重新预览，
 * 请求里只带填了的字段；整个种子整理不了时不能点整理；整理后写明结果，后台还在跑时说明稍后看历史。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  preview: vi.fn(),
  organize: vi.fn(),
  libraries: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    organizeApi: api,
    mediaApi: { ...real.mediaApi, settings: vi.fn(async () => ({ image_base_url: "" })) },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);

/** 打开下拉框 id，点文字是 label 的选项 */
async function pick(id: string, label: string) {
  q(id)!.click();
  await flush();
  const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
    (el) => el.textContent?.trim() === label,
  );
  if (!opt) throw new Error(`下拉框 ${id} 里没有「${label}」`);
  opt.click();
  await flush();
}

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
  category: "外语电影",
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
    expect(q("organize-category")!.textContent, "放进的二级分类").toContain("分类：外语电影");
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

  it("选整理方式：重新预览并带上 mode，整理也按它；没选时写明跟媒体库的哪种方式", async () => {
    await mountDialog();
    expect(q("organize-mode")).not.toBeNull();
    expect(q("organize-mode")!.textContent, "没选时写明库的方式").toContain("跟媒体库（硬链接）");
    expect(api.preview.mock.calls[0]![0], "没选时不带 mode").toEqual({
      downloader_id: 1,
      hash: "abc",
    });

    api.preview.mockResolvedValue({ ...plan, mode: "copy" });
    await pick("organize-mode", "复制");
    await vi.waitFor(() => expect(api.preview).toHaveBeenCalledTimes(2));
    expect(api.preview.mock.calls[1]![0]).toEqual({ downloader_id: 1, hash: "abc", mode: "copy" });
    await vi.waitFor(() => expect(q("organize-library")!.textContent).toContain("复制"));
    expect(q("organize-mode-hint")!.textContent).toContain("多占一份空间");

    api.organize.mockResolvedValue({ created: 1, done: 0, skipped: 0, failed: 0 });
    q("organize-run")!.click();
    await vi.waitFor(() => expect(api.organize).toHaveBeenCalledTimes(1));
    expect(api.organize.mock.calls[0]![0]).toEqual({ downloader_id: 1, hash: "abc", mode: "copy" });
  });

  it("沿用之前整理用的条目时写明来源", async () => {
    api.preview.mockResolvedValue({
      ...plan,
      source: "history",
      items: [{ ...plan.items[0], status: "done" }],
    });
    api.libraries.mockResolvedValue([]);
    const OrganizeDialog = (await import("./OrganizeDialog.vue")).default;
    view = mountView(
      defineComponent({
        setup: () => () =>
          h(OrganizeDialog, { modelValue: true, target: { downloader_id: 1, hash: "abc" } }),
      }),
    );
    await vi.waitFor(() => expect(q("organize-match")).not.toBeNull());
    expect(q("organize-match")!.textContent).toContain("沿用之前整理用的条目");
    expect(q("organize-items")!.textContent).toContain("已在库里");
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

/*
 * 钉子：手动整理（指定一个文件或目录）。打开时不预览，等填了路径点「预览」；从整理历史带着路径打开时立即预览；
 * 请求里只带填了的字段；改了路径要重新预览才能整理；写明链接放到哪个库、刮不刮削，移动整理时提醒源文件会搬走。
 */
const pathPlan = {
  ...plan,
  downloader_id: 0,
  downloader_name: "指定路径",
  hash: "path:0123456789abcdef",
  task_id: "",
  save_path: "/data/in",
  local_path: "/data/in",
  mapped: false,
  source_path: "/data/in/Oppenheimer.2023.2160p.BluRay-FRDS",
  items: [{ ...plan.items[0], source: "/data/in/Oppenheimer.2023.2160p.BluRay-FRDS/a.mkv" }],
};

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

const button = (id: string) => q(id) as HTMLButtonElement;

async function mountPathDialog(
  sourcePath: string,
  mode?: "hardlink" | "copy" | "symlink" | "move",
) {
  api.libraries.mockResolvedValue([
    { ...library, effective_template: "", preview: "", created_at: "", updated_at: "" },
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
          target: mode ? { source_path: sourcePath, mode } : { source_path: sourcePath },
          onDone: done,
        });
    },
  });
  view = mountView(Host);
  await vi.waitFor(() => expect(q("organize-source")).not.toBeNull());
  await flush();
  return done;
}

describe("手动整理（指定路径）", () => {
  // 每个用例自己定预览与整理回什么（排着的 mockResolvedValueOnce 不留给下一个用例）
  beforeEach(() => {
    api.preview.mockReset();
    api.organize.mockReset();
  });

  it("打开时不预览；路径为空时不能预览；请求只带源路径，填了 TMDB 编号再带条目", async () => {
    await mountPathDialog("");
    expect(document.querySelector(".el-dialog__title")?.textContent).toBe("手动整理");
    expect(api.preview).not.toHaveBeenCalled();
    expect(button("organize-source-preview").disabled, "路径为空").toBe(true);
    expect(button("organize-run").disabled).toBe(true);
    expect(document.body.textContent).toContain("Docker 里填容器里的路径");
    expect(document.body.textContent).toContain("要放到别的目录，先在「媒体库」里加一个库");

    api.preview.mockResolvedValue(pathPlan);
    fill("organize-source", " /data/in/Oppenheimer.2023.2160p.BluRay-FRDS ");
    await flush();
    expect(button("organize-source-preview").disabled).toBe(false);
    button("organize-source-preview").click();
    await vi.waitFor(() => expect(api.preview).toHaveBeenCalledTimes(1));
    expect(api.preview.mock.calls[0]![0]).toEqual({
      source_path: "/data/in/Oppenheimer.2023.2160p.BluRay-FRDS",
    });
    await vi.waitFor(() => expect(q("organize-match")).not.toBeNull());
    expect(document.body.textContent).toContain("指定路径 · /data/in");

    fill("organize-tmdb-id", "872585");
    inputOf("organize-tmdb-id").dispatchEvent(new Event("change"));
    await flush();
    q("organize-repreview")!.click();
    await vi.waitFor(() => expect(api.preview).toHaveBeenCalledTimes(2));
    expect(api.preview.mock.calls[1]![0]).toEqual({
      source_path: "/data/in/Oppenheimer.2023.2160p.BluRay-FRDS",
      media_type: "movie",
      tmdb_id: 872585,
    });
  });

  it("带着路径打开时立即预览；改了路径要重新预览才能整理，整理的请求是源路径", async () => {
    api.preview.mockResolvedValue(pathPlan);
    const done = await mountPathDialog("/data/in/Oppenheimer.2023.2160p.BluRay-FRDS");
    await vi.waitFor(() => expect(q("organize-match")).not.toBeNull());
    expect(api.preview).toHaveBeenCalledWith({
      source_path: "/data/in/Oppenheimer.2023.2160p.BluRay-FRDS",
    });
    expect(inputOf("organize-source").value).toBe("/data/in/Oppenheimer.2023.2160p.BluRay-FRDS");
    expect(button("organize-run").disabled).toBe(false);

    fill("organize-source", "/data/in/Other");
    await flush();
    expect(button("organize-run").disabled, "路径改了，计划还是旧的").toBe(true);
    expect(q("organize-stale")).not.toBeNull();

    button("organize-source-preview").click();
    await vi.waitFor(() => expect(api.preview).toHaveBeenCalledTimes(2));
    await vi.waitFor(() => expect(button("organize-run").disabled).toBe(false));
    expect(q("organize-stale")).toBeNull();

    api.organize.mockResolvedValue({
      plan: { ...pathPlan, items: [{ ...pathPlan.items[0], status: "done" }] },
      created: 1,
      done: 0,
      skipped: 0,
      failed: 0,
    });
    button("organize-run").click();
    await vi.waitFor(() => expect(q("organize-result")).not.toBeNull());
    expect(api.organize).toHaveBeenCalledWith({ source_path: "/data/in/Other" });
    expect(q("organize-result")!.textContent).toContain("新入库 1 个");
    expect(done).toHaveBeenCalledTimes(1);
  });

  it("先发的预览晚回来时不覆盖后发的结果", async () => {
    let first!: (v: unknown) => void;
    api.preview
      .mockImplementationOnce(() => new Promise((r) => (first = r)))
      .mockResolvedValueOnce({ ...pathPlan, name: "后发的" });
    await mountPathDialog("/data/in/a");
    // 第一次预览还没回来时「预览」按钮在转圈，改了路径直接按回车再预览一次
    fill("organize-source", "/data/in/b");
    await flush();
    inputOf("organize-source").dispatchEvent(new KeyboardEvent("keyup", { key: "Enter" }));
    await vi.waitFor(() => expect(api.preview).toHaveBeenCalledTimes(2));
    expect(api.preview.mock.calls[1]![0]).toEqual({ source_path: "/data/in/b" });
    await vi.waitFor(() => expect(q("organize-name")?.textContent).toContain("后发的"));
    first({ ...pathPlan, name: "先发的" });
    await flush();
    await flush();
    expect(q("organize-name")!.textContent).toContain("后发的");
    expect(button("organize-run").disabled).toBe(false);
  });

  it("下载器里的种子按种子整理：写明说明，不提醒移动", async () => {
    api.preview.mockResolvedValue({
      ...plan,
      mode: "move",
      notes: ["「x」是下载器「qb」里的种子「x」：按种子整理（整理记录记在这个种子下）"],
    });
    await mountPathDialog("/data/downloads/x");
    await vi.waitFor(() => expect(q("organize-notes")).not.toBeNull());
    expect(q("organize-notes")!.textContent).toContain("按种子整理");
    expect(q("organize-move-warn")).toBeNull();
    expect(document.body.textContent).toContain("qb · /downloads");
  });

  it("指定路径用移动整理时提醒源文件会搬进媒体库", async () => {
    api.preview.mockResolvedValue({
      ...pathPlan,
      mode: "move",
      library: { ...library, mode: "move" },
    });
    await mountPathDialog("/data/in/x");
    await vi.waitFor(() => expect(q("organize-move-warn")).not.toBeNull());
    expect(q("organize-move-warn")!.textContent).toContain("源文件会搬进媒体库");
  });

  it("带着选过的整理方式打开（整理历史里重新整理）：预览带上它；选了移动时提醒「这次用移动整理」", async () => {
    api.preview.mockResolvedValue({ ...pathPlan, mode: "move" });
    await mountPathDialog("/data/in/x", "move");
    await vi.waitFor(() => expect(q("organize-move-warn")).not.toBeNull());
    expect(api.preview).toHaveBeenCalledWith({ source_path: "/data/in/x", mode: "move" });
    expect(q("organize-move-warn")!.textContent).toContain("这次用移动整理");
    expect(q("organize-mode")!.textContent).toContain("移动");
    expect(q("organize-mode-hint"), "提醒已经写明，不再写讲种子的那句").toBeNull();
  });

  it("路径不对（400）时写明原因，不能整理", async () => {
    api.preview.mockRejectedValue(
      new Error("pt-tools 里找不到 /data/in/x：Docker 里要把它所在的目录挂进容器"),
    );
    await mountPathDialog("/data/in/x");
    await vi.waitFor(() => expect(q("organize-error")).not.toBeNull());
    expect(q("organize-error")!.textContent).toContain("找不到");
    expect(button("organize-run").disabled).toBe(true);
  });

  it("媒体库写明整理方式、目录与刮不刮削；整理失败时说明会记进整理历史", async () => {
    api.preview.mockResolvedValue(pathPlan);
    await mountPathDialog("/data/in/x");
    await vi.waitFor(() => expect(q("organize-library")).not.toBeNull());
    expect(q("organize-library")!.textContent).toContain("写 NFO 与图片");
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("电影（硬链接 · /media/movies）"),
    );

    api.organize.mockResolvedValue({ created: 0, done: 0, skipped: 0, failed: 1 });
    button("organize-run").click();
    await vi.waitFor(() => expect(q("organize-result")).not.toBeNull());
    expect(q("organize-result")!.textContent).toContain("失败的文件已记进「整理历史」");

    view?.unmount();
    api.preview.mockResolvedValue({ ...pathPlan, library: { ...library, scrape: false } });
    await mountPathDialog("/data/in/x");
    await vi.waitFor(() => expect(q("organize-library")).not.toBeNull());
    expect(q("organize-library")!.textContent).toContain("不刮削（媒体库关着刮削）");
  });
});
