// @vitest-environment happy-dom
/*
 * 钉子：媒体识别页。设置里 API Key 只在填了时才发出，清除要确认；识别结果显示解析出的各项、匹配条目与来源、
 * 候选；选候选或填编号纠正后重新识别；TMDB 出错时写明原因；识别词与纠正的增删改。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  settings: vi.fn(),
  saveSettings: vi.fn(),
  testTMDB: vi.fn(),
  recognize: vi.fn(),
  overrides: vi.fn(),
  setOverride: vi.fn(),
  deleteOverride: vi.fn(),
  words: vi.fn(),
  createWord: vi.fn(),
  updateWord: vi.fn(),
  deleteWord: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  confirm: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, mediaApi: api };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: ui.error,
      warning: ui.warning,
      info: vi.fn(),
    }),
    ElMessageBox: { ...real.ElMessageBox, confirm: ui.confirm },
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

const match = {
  id: 278,
  media_type: "movie" as const,
  title: "肖申克的救赎",
  original_title: "The Shawshank Redemption",
  year: 1994,
  overview: "希望让人自由。",
  poster_path: "/p.jpg",
  imdb_id: "tt0111161",
};

const recognized = {
  meta: {
    name_en: "The Shawshank Redemption",
    year: 1994,
    type: "movie",
    resolution: "1080p",
    source: "BluRay",
    group: "WiKi",
  },
  summary: "The Shawshank Redemption 1994",
  match,
  source: "search",
  score: 1.4,
  candidates: [
    { ...match, score: 1.4 },
    {
      id: 5000,
      media_type: "movie",
      title: "Shawshank: The Redeeming Feature",
      original_title: "Shawshank: The Redeeming Feature",
      year: 2001,
      score: 0.4,
    },
  ],
};

async function mountPage(opts: { hasKey?: boolean } = {}) {
  api.settings.mockResolvedValue({
    has_tmdb_key: opts.hasKey ?? false,
    language: "zh-CN",
    proxy_url: "",
  });
  api.overrides.mockResolvedValue([]);
  api.words.mockResolvedValue([]);
  const Page = (await import("./MediaRecognize.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.settings).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("媒体识别", () => {
  it("设置：只在填了 Key 时发出；清除要确认；没有 Key 时不能测试连接", async () => {
    await mountPage();
    expect(document.body.textContent).toContain("未经 TMDB 认可或认证");
    expect((q("media-test") as HTMLButtonElement).disabled).toBe(true);

    api.saveSettings.mockResolvedValue({
      has_tmdb_key: true,
      language: "en-US",
      proxy_url: "http://127.0.0.1:7890",
    });
    fill("media-proxy", " http://127.0.0.1:7890 ");
    q("media-save")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(1));
    expect(api.saveSettings.mock.calls[0]![0]).toEqual({
      language: "zh-CN",
      proxy_url: "http://127.0.0.1:7890",
    });
    await flush();

    fill("media-key", " k-123456789012345 ");
    q("media-save")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(2));
    expect(api.saveSettings.mock.calls[1]![0]).toMatchObject({ tmdb_api_key: "k-123456789012345" });
    await flush();
    expect(inputOf("media-key").value, "保存后清空输入框，不回显").toBe("");
    expect((q("media-test") as HTMLButtonElement).disabled).toBe(false);

    ui.confirm.mockResolvedValue("confirm");
    api.saveSettings.mockResolvedValue({ has_tmdb_key: false, language: "en-US", proxy_url: "" });
    q("media-clear-key")!.click();
    await vi.waitFor(() => expect(api.saveSettings).toHaveBeenCalledTimes(3));
    expect(api.saveSettings.mock.calls[2]![0]).toMatchObject({ tmdb_api_key: "" });

    api.testTMDB.mockRejectedValue(new Error("TMDB API Key 无效"));
    await mountPage({ hasKey: true });
    q("media-test")!.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("TMDB API Key 无效"));
  });

  it("识别：显示解析结果、匹配条目与来源、候选；选候选后纠正并重新识别", async () => {
    await mountPage({ hasKey: true });
    q("media-recognize")!.click();
    await flush();
    expect(ui.warning).toHaveBeenCalledWith("先填写标题");
    expect(api.recognize).not.toHaveBeenCalled();

    api.recognize.mockResolvedValue(recognized);
    fill("media-title", " The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi ");
    fill("media-imdb", "tt0111161");
    q("media-recognize")!.click();
    await vi.waitFor(() => expect(q("media-match")).not.toBeNull());
    expect(api.recognize).toHaveBeenCalledWith({
      title: "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi",
      subtitle: "",
      imdb_id: "tt0111161",
    });
    expect(q("media-summary")!.textContent).toContain("The Shawshank Redemption 1994");
    const tags = q("media-tags")!.textContent ?? "";
    for (const t of ["电影", "1994", "1080p", "BluRay", "制作组 WiKi"]) expect(tags).toContain(t);
    const matchText = q("media-match")!.textContent ?? "";
    expect(matchText).toContain("肖申克的救赎（The Shawshank Redemption）");
    expect(matchText).toContain("按名字搜索");
    expect(matchText).toContain("tt0111161");
    expect(q("media-match")!.querySelector("a")!.getAttribute("href")).toBe(
      "https://www.themoviedb.org/movie/278",
    );
    expect(q("media-match")!.querySelector("img")!.getAttribute("src")).toBe(
      "https://image.tmdb.org/t/p/w185/p.jpg",
    );
    expect((q("media-pick-movie-278") as HTMLButtonElement).disabled, "已经是这一条").toBe(true);

    api.setOverride.mockResolvedValue({ id: 1 });
    api.recognize.mockResolvedValue({
      ...recognized,
      source: "override",
      match: { ...match, id: 5000 },
    });
    api.overrides.mockResolvedValue([
      {
        id: 1,
        key: "k",
        label: "The Shawshank Redemption 1994",
        tmdb_id: 5000,
        media_type: "movie",
        title: "Shawshank: The Redeeming Feature",
      },
    ]);
    q("media-pick-movie-5000")!.click();
    await vi.waitFor(() => expect(api.setOverride).toHaveBeenCalledTimes(1));
    expect(api.setOverride.mock.calls[0]![0]).toEqual({
      title: "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi",
      subtitle: "",
      tmdb_id: 5000,
      media_type: "movie",
    });
    await vi.waitFor(() => expect(api.recognize).toHaveBeenCalledTimes(2));
    await vi.waitFor(() =>
      expect(q("media-overrides")?.textContent).toContain("Shawshank: The Redeeming Feature"),
    );
  });

  it("识别：TMDB 出错时写明原因；没有匹配时给出提示", async () => {
    await mountPage({ hasKey: true });
    api.recognize.mockResolvedValue({
      meta: { name_en: "X" },
      summary: "X",
      source: "none",
      error: "TMDB API Key 无效",
    });
    fill("media-title", "X.1080p");
    q("media-recognize")!.click();
    await vi.waitFor(() => expect(q("media-error")?.textContent).toContain("TMDB API Key 无效"));

    api.recognize.mockResolvedValue({
      meta: {},
      summary: "",
      source: "none",
      message: "没有填写 TMDB API Key，只做了标题解析",
    });
    q("media-recognize")!.click();
    await vi.waitFor(() => expect(q("media-message")?.textContent).toContain("只做了标题解析"));
    expect(q("media-summary")!.textContent).toContain("没解析出名字");

    // 新一次识别失败：旧结果清掉，页面里写明原因（不只是一闪而过的提示）
    api.recognize.mockRejectedValue(new Error("TMDB 暂时不能访问: HTTP 502"));
    q("media-recognize")!.click();
    await vi.waitFor(() =>
      expect(q("media-recognize-error")?.textContent).toContain("TMDB 暂时不能访问"),
    );
    expect(q("media-result")).toBeNull();

    api.recognize.mockResolvedValue({ meta: {}, summary: "Y", source: "none" });
    q("media-recognize")!.click();
    await vi.waitFor(() => expect(q("media-result")).not.toBeNull());
    expect(q("media-recognize-error")).toBeNull();
  });

  it("按 TMDB 编号纠正：没填编号时提示", async () => {
    await mountPage({ hasKey: true });
    api.recognize.mockResolvedValue({ ...recognized, candidates: [] });
    fill("media-title", "Some.Movie.2020");
    q("media-recognize")!.click();
    await vi.waitFor(() => expect(q("media-manual-save")).not.toBeNull());
    q("media-manual-save")!.click();
    await flush();
    expect(ui.warning).toHaveBeenCalledWith("填写 TMDB 编号");
    expect(api.setOverride).not.toHaveBeenCalled();
  });

  it("识别词：添加、开关、删除；纠正：删除要确认", async () => {
    await mountPage();
    api.createWord.mockResolvedValue({});
    buttonByText("添加").click();
    await vi.waitFor(() => expect(q("media-word-pattern")).not.toBeNull());
    q("media-word-save")!.click();
    await flush();
    expect(ui.warning).toHaveBeenCalledWith("填写匹配的文字");
    fill("media-word-pattern", "TSR");
    fill("media-word-replacement", "The.Shawshank.Redemption");
    api.words.mockResolvedValue([
      {
        id: 3,
        kind: "replace",
        pattern: "TSR",
        replacement: "The.Shawshank.Redemption",
        offset: 0,
        is_regex: false,
        enabled: true,
        note: "",
      },
    ]);
    q("media-word-save")!.click();
    await vi.waitFor(() => expect(api.createWord).toHaveBeenCalledTimes(1));
    expect(api.createWord.mock.calls[0]![0]).toEqual({
      kind: "replace",
      pattern: "TSR",
      replacement: "The.Shawshank.Redemption",
      offset: 0,
      is_regex: false,
      enabled: true,
      note: "",
    });
    await vi.waitFor(() =>
      expect(q("media-words")?.textContent).toContain("换成「The.Shawshank.Redemption」"),
    );

    api.updateWord.mockResolvedValue({});
    q("media-word-enabled-3")!.click();
    await vi.waitFor(() => expect(api.updateWord).toHaveBeenCalledTimes(1));
    expect(api.updateWord.mock.calls[0]![0]).toBe(3);
    expect(api.updateWord.mock.calls[0]![1], "只发接口收的字段，不带 id").toEqual({
      kind: "replace",
      pattern: "TSR",
      replacement: "The.Shawshank.Redemption",
      offset: 0,
      is_regex: false,
      enabled: false,
      note: "",
    });

    ui.confirm.mockRejectedValueOnce("cancel");
    q("media-word-del-3")!.click();
    await flush();
    expect(api.deleteWord).not.toHaveBeenCalled();
    ui.confirm.mockResolvedValue("confirm");
    api.deleteWord.mockResolvedValue({});
    q("media-word-del-3")!.click();
    await vi.waitFor(() => expect(api.deleteWord).toHaveBeenCalledWith(3));
  });

  it("集数偏移不能为 0", async () => {
    await mountPage();
    buttonByText("添加").click();
    await vi.waitFor(() => expect(q("media-word-kind")).not.toBeNull());
    const offsetRadio = [
      ...document.querySelectorAll<HTMLElement>("[data-testid=media-word-kind] label"),
    ].find((l) => l.textContent?.includes("集数偏移"))!;
    offsetRadio.click();
    await flush();
    fill("media-word-pattern", "Some.Show");
    q("media-word-save")!.click();
    await flush();
    expect(ui.warning).toHaveBeenCalledWith("集数偏移不能为 0");
    expect(api.createWord).not.toHaveBeenCalled();
  });
});
