// @vitest-environment happy-dom
/*
 * 钉子（M4b）：整理弹窗里的「让 AI 认」、二级分类说明、浏览文件与监控目录的一项。
 * 没识别出来时才有「让 AI 认」；认出以后填进「识别不对」那一行、预览按它算，整理前按「记成纠正」先存纠正；
 * 都不是时列出候选；助手没配好（409）时写明原因、不再让点。浏览先列根目录，进文件夹、选文件或文件夹后填进源路径并预览。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  preview: vi.fn(),
  organize: vi.fn(),
  libraries: vi.fn(),
  aiRecognize: vi.fn(),
  aiStatus: vi.fn(),
  fsRoots: vi.fn(),
  fsList: vi.fn(),
}));
const media = vi.hoisted(() => ({ setOverride: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    organizeApi: api,
    mediaApi: {
      ...real.mediaApi,
      settings: vi.fn(async () => ({ image_base_url: "" })),
      setOverride: media.setOverride,
    },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.resetAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);
const button = (id: string) => q(id) as HTMLButtonElement;
const text = (id: string) => q(id)?.textContent?.replace(/\s+/g, " ").trim() ?? "";

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

const item = {
  rel: "Shawshank.1971/Shawshank.1971.mkv",
  source: "/data/downloads/Shawshank.1971/Shawshank.1971.mkv",
  size: 1,
  status: "failed",
};

/** 没识别出来的计划 */
const unknownPlan = {
  downloader_id: 1,
  downloader_name: "qb",
  hash: "abc",
  task_id: "abc",
  name: "The.Shawshank.Redemption.1971.1080p",
  save_path: "/downloads",
  local_path: "/data/downloads",
  mapped: true,
  items: [item],
  problem: "没有识别出来：TMDB 上没有可靠的匹配",
  fail_kind: "recognize",
};

const shawshank = {
  id: 278,
  media_type: "movie",
  title: "肖申克的救赎",
  original_title: "The Shawshank Redemption",
  year: 1994,
  score: 0.9,
};

/** 按 AI 认出的条目算出的预览 */
const chosenPlan = {
  ...unknownPlan,
  problem: undefined,
  fail_kind: undefined,
  match: shawshank,
  source: "manual",
  library,
  mode: "hardlink",
  items: [{ ...item, status: "pending", target: "/media/movies/肖申克的救赎 (1994)/a.mkv" }],
};

const aiChosen = {
  input: { title: "The.Shawshank.Redemption.1971.1080p", subtitle: "肖申克的救赎 1994" },
  candidates: [
    shawshank,
    { ...shawshank, id: 5000, title: "Shawshank: The Redeeming Feature", year: 2001 },
  ],
  chosen: 0,
  reason: "名字对得上，种子名里的年份写错了",
  model: "deepseek-chat",
  prompt_tokens: 320,
  completion_tokens: 12,
  request: { downloader_id: 1, hash: "abc", media_type: "movie", tmdb_id: 278 },
  preview: chosenPlan,
};

type Target = Record<string, unknown>;

async function mountDialog(target: Target, firstPlan: object = unknownPlan) {
  api.preview.mockResolvedValue(firstPlan);
  // 用例没另外说时 AI 能用（排着的 mockResolvedValueOnce 与用例里设的照旧先用）
  if (!api.aiStatus.getMockImplementation()) api.aiStatus.mockResolvedValue({ ready: true });
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
          target: target as never,
          onDone: done,
        });
    },
  });
  view = mountView(Host);
  await flush();
  await flush();
  return done;
}

const inputOf = (id: string) => q(id)!.querySelector("input") as HTMLInputElement;

describe("让 AI 认", () => {
  it("没识别出来时才有按钮；认出以后填进「识别不对」、预览按它算，写明理由与用量", async () => {
    await mountDialog({ downloader_id: 1, hash: "abc", name: "x" });
    await vi.waitFor(() => expect(q("organize-ai-ask")).not.toBeNull());
    expect(api.aiRecognize).not.toHaveBeenCalled();

    api.aiRecognize.mockResolvedValue(aiChosen);
    button("organize-ai-ask").click();
    await vi.waitFor(() => expect(q("organize-ai-result")).not.toBeNull());
    expect(api.aiRecognize).toHaveBeenCalledWith({ downloader_id: 1, hash: "abc" });
    const note = text("organize-ai-result");
    expect(note).toContain("AI 认成「肖申克的救赎（1994）」（电影 · TMDB 278）");
    expect(note).toContain("理由：名字对得上，种子名里的年份写错了");
    expect(note).toContain("模型 deepseek-chat，输入 320 个 token、输出 12 个");
    expect(inputOf("organize-tmdb-id").value).toBe("278");
    expect(q("organize-match")!.textContent).toContain("肖申克的救赎");
    expect(q("organize-ai-ask"), "认出了，预览也不是没识别出来了").toBeNull();
    expect(button("organize-run").disabled).toBe(false);
    expect(api.preview, "用回来的预览，不再预览一次").toHaveBeenCalledTimes(1);

    // 整理前先存一条纠正，再按填的条目整理
    const box = q("organize-ai-override")!.querySelector<HTMLInputElement>("input")!;
    expect(box.checked).toBe(true);
    media.setOverride.mockResolvedValue({ id: 1 });
    api.organize.mockResolvedValue({
      plan: chosenPlan,
      created: 1,
      done: 0,
      skipped: 0,
      failed: 0,
    });
    button("organize-run").click();
    await vi.waitFor(() => expect(api.organize).toHaveBeenCalled());
    expect(media.setOverride).toHaveBeenCalledWith({
      title: "The.Shawshank.Redemption.1971.1080p",
      subtitle: "肖申克的救赎 1994",
      media_type: "movie",
      tmdb_id: 278,
    });
    expect(media.setOverride.mock.invocationCallOrder[0]).toBeLessThan(
      api.organize.mock.invocationCallOrder[0]!,
    );
    expect(api.organize).toHaveBeenCalledWith({
      downloader_id: 1,
      hash: "abc",
      media_type: "movie",
      tmdb_id: 278,
    });
  });

  it("不勾「记成纠正」时不存纠正；改了条目也不存", async () => {
    await mountDialog({ downloader_id: 1, hash: "abc" });
    await vi.waitFor(() => expect(q("organize-ai-ask")).not.toBeNull());
    api.aiRecognize.mockResolvedValue(aiChosen);
    button("organize-ai-ask").click();
    await vi.waitFor(() => expect(q("organize-ai-override")).not.toBeNull());
    q("organize-ai-override")!.querySelector<HTMLInputElement>("input")!.click();
    await flush();
    api.organize.mockResolvedValue({ created: 1, done: 0, skipped: 0, failed: 0 });
    button("organize-run").click();
    await vi.waitFor(() => expect(api.organize).toHaveBeenCalled());
    expect(media.setOverride).not.toHaveBeenCalled();

    // 改了「识别不对」里的编号：不再是 AI 认的条目，不提记成纠正
    const id = inputOf("organize-tmdb-id");
    id.value = "5000";
    id.dispatchEvent(new Event("input"));
    id.dispatchEvent(new Event("change"));
    await flush();
    expect(q("organize-ai-override")).toBeNull();
  });

  it("勾着「记成纠正」但改了条目：按改的条目整理，不存纠正", async () => {
    await mountDialog({ downloader_id: 1, hash: "abc" });
    await vi.waitFor(() => expect(q("organize-ai-ask")).not.toBeNull());
    api.aiRecognize.mockResolvedValue(aiChosen);
    button("organize-ai-ask").click();
    await vi.waitFor(() => expect(q("organize-ai-override")).not.toBeNull());
    const id = inputOf("organize-tmdb-id");
    id.value = "5000";
    id.dispatchEvent(new Event("input"));
    id.dispatchEvent(new Event("change"));
    await flush();
    api.organize.mockResolvedValue({ created: 1, done: 0, skipped: 0, failed: 0 });
    button("organize-run").click();
    await vi.waitFor(() => expect(api.organize).toHaveBeenCalled());
    expect(api.organize.mock.calls[0]![0]).toMatchObject({ tmdb_id: 5000 });
    expect(media.setOverride).not.toHaveBeenCalled();
  });

  it("让 AI 认的是种子本身：「识别不对」里填了编号也不带", async () => {
    await mountDialog({ downloader_id: 1, hash: "abc" });
    await vi.waitFor(() => expect(q("organize-ai-ask")).not.toBeNull());
    const id = inputOf("organize-tmdb-id");
    id.value = "603";
    id.dispatchEvent(new Event("input"));
    id.dispatchEvent(new Event("change"));
    await flush();
    api.aiRecognize.mockResolvedValue({
      ...aiChosen,
      chosen: -1,
      request: undefined,
      preview: undefined,
    });
    button("organize-ai-ask").click();
    await vi.waitFor(() => expect(api.aiRecognize).toHaveBeenCalled());
    expect(api.aiRecognize).toHaveBeenCalledWith({ downloader_id: 1, hash: "abc" });
  });

  it("存纠正失败时不整理，写明原因", async () => {
    await mountDialog({ downloader_id: 1, hash: "abc" });
    await vi.waitFor(() => expect(q("organize-ai-ask")).not.toBeNull());
    api.aiRecognize.mockResolvedValue(aiChosen);
    button("organize-ai-ask").click();
    await vi.waitFor(() => expect(q("organize-ai-override")).not.toBeNull());
    media.setOverride.mockRejectedValue(new Error("TMDB 暂时不能访问"));
    button("organize-run").click();
    await vi.waitFor(() => expect(q("organize-error")).not.toBeNull());
    expect(text("organize-error")).toContain("记成纠正失败，没有整理：TMDB 暂时不能访问");
    expect(api.organize).not.toHaveBeenCalled();
  });

  it("都不是：列出候选，不填条目；没有候选时写明", async () => {
    await mountDialog({ downloader_id: 1, hash: "abc" });
    await vi.waitFor(() => expect(q("organize-ai-ask")).not.toBeNull());
    api.aiRecognize.mockResolvedValue({
      ...aiChosen,
      chosen: -1,
      reason: "年份都不对",
      request: undefined,
      preview: undefined,
    });
    button("organize-ai-ask").click();
    await vi.waitFor(() => expect(q("organize-ai-result")).not.toBeNull());
    const note = text("organize-ai-result");
    expect(note).toContain("AI 认为候选里都不是。候选：肖申克的救赎（1994） · 电影 · TMDB 278；");
    expect(note).toContain("理由：年份都不对");
    expect(inputOf("organize-tmdb-id").value).toBe("");
    expect(q("organize-ai-override")).toBeNull();
    expect(q("organize-ai-ask"), "还能再认一次").not.toBeNull();

    api.aiRecognize.mockResolvedValue({
      input: aiChosen.input,
      candidates: [],
      chosen: -1,
      reason: "TMDB 上没有搜到候选：换个名字搜，或手动填 TMDB 编号",
    });
    button("organize-ai-ask").click();
    await vi.waitFor(() => expect(text("organize-ai-result")).toContain("TMDB 上没有搜到候选"));
    expect(text("organize-ai-result")).not.toContain("理由");
  });

  it("助手没配好：先问一次能不能用，不能用时按钮置灰并写明原因，不调 AI", async () => {
    api.aiStatus.mockResolvedValue({
      ready: false,
      reason: "AI 助手没有启用：先在「AI 助手」里配好",
    });
    await mountDialog({ downloader_id: 1, hash: "abc", ai: true });
    await vi.waitFor(() => expect(q("organize-ai-unavailable")).not.toBeNull());
    expect(text("organize-ai-unavailable")).toBe("AI 助手没有启用：先在「AI 助手」里配好");
    expect(button("organize-ai-ask").disabled).toBe(true);
    await flush();
    expect(api.aiRecognize, "从整理历史打开也不自动认").not.toHaveBeenCalled();
    expect(api.aiStatus).toHaveBeenCalledTimes(1);
  });

  it("点了才发现没配好（409）时置灰并写明原因；别的错误还能再点", async () => {
    api.aiStatus.mockResolvedValue({ ready: true });
    await mountDialog({ downloader_id: 1, hash: "abc" });
    await vi.waitFor(() => expect(q("organize-ai-ask")).not.toBeNull());
    api.aiRecognize.mockRejectedValue(
      Object.assign(new Error("AI 没认成：模型服务响应超时\n"), { status: 502 }),
    );
    button("organize-ai-ask").click();
    await vi.waitFor(() => expect(q("organize-ai-error")).not.toBeNull());
    expect(text("organize-ai-error")).toBe("AI 没认成：模型服务响应超时");
    expect(button("organize-ai-ask").disabled).toBe(false);

    api.aiRecognize.mockRejectedValue(
      Object.assign(new Error("AI 助手没有启用：先在「AI 助手」里配好"), { status: 409 }),
    );
    button("organize-ai-ask").click();
    await vi.waitFor(() => expect(q("organize-ai-unavailable")).not.toBeNull());
    expect(text("organize-ai-unavailable")).toContain("AI 助手没有启用");
    expect(button("organize-ai-ask").disabled).toBe(true);
  });

  it("问不到能不能用时照样让点", async () => {
    api.aiStatus.mockRejectedValue(new Error("网络错误"));
    await mountDialog({ downloader_id: 1, hash: "abc" });
    await vi.waitFor(() => expect(q("organize-ai-ask")).not.toBeNull());
    await flush();
    expect(button("organize-ai-ask").disabled).toBe(false);
    expect(q("organize-ai-unavailable")).toBeNull();
  });

  it("别的问题（不是没识别出来）时没有按钮", async () => {
    await mountDialog(
      { downloader_id: 1, hash: "abc" },
      { ...unknownPlan, problem: "没有启用的电影媒体库", fail_kind: "library" },
    );
    await vi.waitFor(() => expect(q("organize-problem")).not.toBeNull());
    expect(q("organize-ai")).toBeNull();
  });

  it("从整理历史打开（ai）：预览没识别出来时马上认一次；监控目录的一项按目录与项预览", async () => {
    api.aiRecognize.mockResolvedValue(aiChosen);
    await mountDialog({ watch_dir_id: 3, entry: "Shawshank.1971", ai: true });
    await vi.waitFor(() => expect(api.aiRecognize).toHaveBeenCalledTimes(1));
    expect(api.preview).toHaveBeenCalledWith({ watch_dir_id: 3, entry: "Shawshank.1971" });
    expect(api.aiRecognize).toHaveBeenCalledWith({ watch_dir_id: 3, entry: "Shawshank.1971" });
    await vi.waitFor(() => expect(q("organize-ai-result")).not.toBeNull());
    expect(document.querySelector(".el-dialog__title")?.textContent).toBe("整理入库");
  });

  it("从整理历史打开（ai）、问不到能不能用时照样认一次", async () => {
    api.aiStatus.mockRejectedValue(new Error("网络错误"));
    api.aiRecognize.mockResolvedValue(aiChosen);
    await mountDialog({ downloader_id: 1, hash: "abc", ai: true });
    await vi.waitFor(() => expect(api.aiRecognize).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(q("organize-ai-result")).not.toBeNull());
  });

  it("从整理历史打开（ai）但认得出来时不调 AI", async () => {
    await mountDialog({ downloader_id: 1, hash: "abc", ai: true }, chosenPlan);
    await vi.waitFor(() => expect(q("organize-match")).not.toBeNull());
    await flush();
    expect(api.aiRecognize).not.toHaveBeenCalled();
  });
});

describe("二级分类说明", () => {
  it("写明对上第几条与对上的条件；都没对上时写明", async () => {
    await mountDialog(
      { downloader_id: 1, hash: "abc" },
      {
        ...chosenPlan,
        match: { ...shawshank, media_type: "tv" },
        category: "短剧",
        classify: {
          rule: 6,
          name: "短剧",
          rules: 10,
          runtime: 8,
          max_runtime: 15,
          keyword: "短剧",
        },
      },
    );
    await vi.waitFor(() => expect(q("organize-classify")).not.toBeNull());
    expect(text("organize-classify")).toBe(
      "二级分类：对上第 6 条「短剧」：片长 8 分钟（不超过 15 分钟），标题里有「短剧」",
    );
  });
});

describe("浏览文件", () => {
  const roots = {
    roots: [
      { path: "/data/downloads", kinds: ["map", "downloader"], labels: ["qb 默认"] },
      { path: "/media/movies", kinds: ["library"], labels: ["电影"] },
    ],
  };
  const downloads = {
    path: "/data/downloads",
    root: "/data/downloads",
    entries: [
      { name: "Movie.2023", path: "/data/downloads/Movie.2023", dir: true, mod_time: "" },
      { name: "a.mkv", path: "/data/downloads/a.mkv", dir: false, size: 1 << 30, mod_time: "" },
    ],
  };
  const movie = {
    path: "/data/downloads/Movie.2023",
    root: "/data/downloads",
    parent: "/data/downloads",
    entries: [],
    truncated: true,
  };

  async function openBrowser() {
    await mountDialog({ source_path: "" });
    api.fsRoots.mockResolvedValue(roots);
    button("organize-source-browse").click();
    await vi.waitFor(() => expect(q("organize-fs-roots")).not.toBeNull());
  }

  const rows = () => [...document.querySelectorAll<HTMLButtonElement>(".od-fs__row")];
  /** 每一行的名字与右边的说明 */
  const rowTexts = () =>
    rows().map((r) => [
      r.querySelector(".od-fs__name")?.textContent?.trim() ?? "",
      r.querySelector(".od-fs__aside")?.textContent?.trim() ?? "",
    ]);

  it("先列根目录；进文件夹、选文件后填进源路径并预览", async () => {
    await openBrowser();
    expect(rowTexts()).toEqual([
      ["/data/downloads", "路径映射、下载目录 · qb 默认"],
      ["/media/movies", "媒体库 · 电影"],
    ]);
    expect(q("organize-fs-dir"), "根目录列表里没有「选这个文件夹」").toBeNull();
    expect(button("organize-fs-file").disabled).toBe(true);

    api.fsList.mockResolvedValue(downloads);
    rows()[0]!.click();
    await vi.waitFor(() => expect(q("organize-fs-entries")).not.toBeNull());
    expect(api.fsList).toHaveBeenCalledWith("/data/downloads");
    expect(text("organize-fs-path")).toBe("/data/downloads");
    expect(rowTexts()).toEqual([
      ["Movie.2023", ""],
      ["a.mkv", "1 GB"],
    ]);
    expect(text("organize-fs-up")).toBe("根目录");

    rows()[1]!.click();
    await flush();
    expect(rows()[1]!.classList.contains("is-picked")).toBe(true);
    api.preview.mockResolvedValue(unknownPlan);
    button("organize-fs-file").click();
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenCalledWith({ source_path: "/data/downloads/a.mkv" }),
    );
    expect(
      (q("organize-source") as HTMLInputElement).value ?? inputOf("organize-source").value,
    ).toBe("/data/downloads/a.mkv");
  });

  it("选当前文件夹；上一级回到上一层，在根目录时回到根目录列表；读不了时写明原因", async () => {
    await openBrowser();
    api.fsList.mockResolvedValueOnce(downloads).mockResolvedValueOnce(movie);
    rows()[0]!.click();
    await vi.waitFor(() => expect(q("organize-fs-entries")).not.toBeNull());
    rows()[0]!.click();
    await vi.waitFor(() => expect(text("organize-fs-path")).toBe("/data/downloads/Movie.2023"));
    expect(text("organize-fs-entries")).toContain("这个文件夹是空的");
    expect(text("organize-fs-entries")).toContain("只列了前 2000 项");
    expect(text("organize-fs-up")).toBe("上一级");

    api.fsList.mockResolvedValueOnce(downloads);
    button("organize-fs-up").click();
    await vi.waitFor(() => expect(text("organize-fs-path")).toBe("/data/downloads"));
    expect(api.fsList).toHaveBeenLastCalledWith("/data/downloads");
    button("organize-fs-up").click();
    await vi.waitFor(() => expect(q("organize-fs-roots")).not.toBeNull());

    api.fsList.mockRejectedValueOnce(
      new Error("只能浏览媒体库、监控目录、路径映射与下载器的下载目录\n"),
    );
    rows()[1]!.click();
    await vi.waitFor(() => expect(q("organize-fs-error")).not.toBeNull());
    expect(text("organize-fs-error")).toBe("只能浏览媒体库、监控目录、路径映射与下载器的下载目录");

    api.fsList.mockResolvedValueOnce(movie);
    api.preview.mockResolvedValue(unknownPlan);
    button("organize-fs-up").click();
    await vi.waitFor(() => expect(q("organize-fs-roots")).not.toBeNull());
    rows()[0]!.click();
    await vi.waitFor(() => expect(q("organize-fs-dir")).not.toBeNull());
    button("organize-fs-dir").click();
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenCalledWith({ source_path: "/data/downloads/Movie.2023" }),
    );
  });

  it("还没有根目录时写明去哪里加", async () => {
    await mountDialog({ source_path: "" });
    api.fsRoots.mockResolvedValue({ roots: [] });
    button("organize-source-browse").click();
    await vi.waitFor(() => expect(text("organize-fs-roots")).toContain("还没有能浏览的目录"));
  });
});
