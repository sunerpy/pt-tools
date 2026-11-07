// @vitest-environment happy-dom
/*
 * 钉子（追剧方案 F3，3.3.4）：手机追剧编辑页。四块布局与摘要行；新建时按媒体库带出、不覆盖自己改过的；
 * 旧主机降级；试搜索结果与每个操作改到的表单字段；保存的请求体（锁、from_torrent 只在需要时发）；
 * 复制走原生桥的 copy；「改成修改它」；取消只取消这一次试搜索。带出的规则本身在 utils/subscribeDefaults.test.ts。
 * 2026-11-07 用户：「现在手机上追剧时不够简洁……选择了媒体库 剧集 类型后，应该支持自动带出一些设置的比如报错目录、分类、标签等内容
 * 还可以试搜索列出检索后的结果 并可以比如复制标题等内容作为搜索条件进行准确追剧」。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";
import { type MountedView, mountView } from "@/test-mount";
import type { AppPreview, PreviewMember } from "./api";
import { BridgeError } from "./bridge";
import { Language } from "./i18n";

const api = vi.hoisted(() => ({
  meta: vi.fn(),
  options: vi.fn(),
  libraries: vi.fn(),
  subscription: vi.fn(),
  draft: vi.fn(),
  preview: vi.fn(),
}));
const bridge = vi.hoisted(() => ({ request: vi.fn(), notify: vi.fn(), copyText: vi.fn() }));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}));

vi.mock("./bridge", async (orig) => ({
  ...(await orig<typeof import("./bridge")>()),
  request: bridge.request,
  notify: bridge.notify,
  copyText: bridge.copyText,
}));
vi.mock("./api", async (orig) => ({
  ...(await orig<typeof import("./api")>()),
  mobileApi: api,
}));
vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return { ...real, ElMessage: Object.assign(vi.fn(), ui) };
});

const LIBS = [
  {
    id: 1,
    name: "剧集",
    kind: "tv",
    enabled: true,
    download_save_path: "/downloads/tv",
    download_category: "tv",
    download_tags: "pt-tools",
  },
  {
    id: 2,
    name: "动漫",
    kind: "tv",
    anime: true,
    enabled: true,
    download_downloader_id: 2,
    download_save_path: "/downloads/anime",
    download_category: "anime",
  },
  { id: 3, name: "电影库", kind: "movie", enabled: true, download_save_path: "/downloads/movie" },
  { id: 4, name: "旧剧集库", kind: "tv", enabled: false },
];
const OPTIONS = {
  sites: [
    { name: "hdsky", display_name: "HDSky" },
    { name: "ttg", display_name: "TTG" },
  ],
  downloaders: [
    { id: 1, name: "qB", enabled: true },
    { id: 2, name: "TR", enabled: true },
    { id: 3, name: "旧qB", enabled: false },
  ],
  directories: [
    { downloader_id: 2, path: "/data/anime" },
    { downloader_id: 1, path: "/downloads/tv", alias: "剧集目录", is_default: true },
  ],
  profiles: [{ id: 5, name: "1080p 优先" }],
  libraries: LIBS.map(({ id, name }) => ({ id, name })),
};
const DRAFT = {
  media_type: "tv",
  tmdb_id: 1001,
  season: 2,
  seasons: 2,
  total_episodes: 10,
  title: "测试剧",
  original_title: "Test Show",
  year: 2026,
  anime: false,
  auto_library_id: 1,
  subscription_id: 0,
};

function member(
  site: string,
  id: string,
  title: string,
  extra: Partial<PreviewMember> = {},
): PreviewMember {
  return {
    site,
    torrent_id: id,
    title,
    subtitle: "",
    size_bytes: 2 * 1024 ** 3,
    seeders: 12,
    leechers: 1,
    snatched: 30,
    uploaded_at: 1_780_000_000,
    discount: "free",
    free: true,
    free_left_seconds: 0,
    hr: false,
    hr_hours: 0,
    category: "tv",
    by_imdb: false,
    blocked: false,
    ...extra,
  };
}
const TITLE = "Test.Show.S02E02-E03.1080p.WEB-DL.H264-CHDWEB";
/** F2 契约（docs/reference/app-api-v1.yaml 的 AppSearchPreview）的样子：符合一组、不符合一组、对不上一条 */
function preview(): AppPreview {
  return {
    item: {
      media_type: "tv",
      tmdb_id: 1001,
      season: 2,
      title: "测试剧",
      original_title: "Test Show",
      year: 2026,
      seasons: 2,
      total_episodes: 10,
    },
    profile: { id: 0, name: "" },
    keywords: ["测试剧", "Test Show"],
    episodes: [
      { number: 1, state: "library" },
      { number: 2, state: "missing" },
      { number: 3, state: "missing" },
    ],
    groups: [
      {
        group_id: "g1",
        meta: {
          group: "CHDWEB",
          resolution: "1080p",
          source: "WEB-DL",
          season: 2,
          episode: 2,
          episode_end: 3,
        },
        verdict: { score: 80, reject: "" },
        span: { start: 2, end: 3, whole: false },
        covers: [2, 3],
        downloading: [],
        in_library: false,
        blocked: false,
        members: [
          member("hdsky", "101", TITLE, { subtitle: "测试剧 第2-3集" }),
          member("ttg", "201", "Test Show S02E02-E03 1080p WEB-DL H264-CHDWEB"),
        ],
      },
      {
        group_id: "g2",
        meta: { group: "XYZ", resolution: "720p", source: "HDTV" },
        verdict: { score: 0, reject: "标题含排除词 hdtv" },
        span: { start: 2, end: 2, whole: false },
        covers: [],
        downloading: [],
        in_library: false,
        blocked: false,
        members: [member("ttg", "202", "Test.Show.S02E02.720p.HDTV-XYZ")],
      },
    ],
    total: 3,
    blocked_count: 0,
    unmatched_count: 1,
    unmatched: [
      {
        site: "ttg",
        torrent_id: "9",
        title: "Other.Show.S01E01.1080p",
        subtitle: "",
        reason: "名字对不上",
      },
    ],
    errors: [],
    duration_ms: 2300,
  };
}

let view: MountedView | undefined;
beforeEach(() => {
  api.meta.mockResolvedValue({ version: "1.0.0", features: ["subscription_assist"] });
  api.options.mockResolvedValue(structuredClone(OPTIONS));
  api.libraries.mockResolvedValue(structuredClone(LIBS));
  api.draft.mockImplementation(async (_type: string, _id: number, season: number) => ({
    ...DRAFT,
    season: season || 2,
    subscription_id: season === 1 ? 5 : 0,
  }));
  api.preview.mockImplementation(async () => preview());
  bridge.request.mockResolvedValue({ id: 9 });
  bridge.copyText.mockResolvedValue({ copied: true });
});
afterEach(() => {
  view?.unmount();
  view = undefined;
  document.body.innerHTML = "";
  vi.clearAllMocks();
  window.history.replaceState({}, "", "/");
});

const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);
const text = (id: string) => (q(id)?.textContent ?? "").replace(/\s+/g, " ").trim();

async function flush() {
  for (let i = 0; i < 6; i++) {
    await Promise.resolve();
    await nextTick();
  }
}
function press(id: string) {
  const el = q(id);
  if (!el) throw new Error(`找不到 ${id}`);
  (el.closest("button") ?? el).click();
}
function typeIn(id: string, v: string) {
  const el = q(id);
  if (!el) throw new Error(`找不到 ${id}`);
  const input = (el instanceof HTMLInputElement ? el : el.querySelector("input"))!;
  input.value = v;
  input.dispatchEvent(new Event("input"));
}
/** 一个下拉自己的选项（按 aria-controls 找到它的列表，不和别的下拉混） */
function optionsOf(id: string) {
  const list = q(id)?.querySelector("input")?.getAttribute("aria-controls");
  return [
    ...(document
      .getElementById(list ?? "")
      ?.querySelectorAll<HTMLElement>(".el-select-dropdown__item") ?? []),
  ];
}
async function optionLabels(id: string) {
  (q(id)!.querySelector<HTMLElement>(".el-select__wrapper") ?? q(id)!).click();
  await flush();
  return optionsOf(id).map((o) => (o.textContent ?? "").trim());
}
async function pickOption(id: string, label: string) {
  (q(id)!.querySelector<HTMLElement>(".el-select__wrapper") ?? q(id)!).click();
  const item = await vi.waitFor(() => {
    const el = optionsOf(id).find((o) => (o.textContent ?? "").trim() === label);
    expect(el, `${id} 里没有「${label}」`).toBeTruthy();
    return el!;
  });
  item.click();
  await flush();
}
const tags = (prefix: string) =>
  [...document.querySelectorAll<HTMLElement>(`[data-testid^="${prefix}-tag-"]`)].map((el) =>
    (el.textContent ?? "").trim(),
  );
function submit() {
  document
    .querySelector("form")!
    .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
}
async function saved() {
  submit();
  await vi.waitFor(() => expect(bridge.request).toHaveBeenCalled());
  return bridge.request.mock.calls.at(-1) as [string, string, Record<string, unknown>];
}

async function mountEditor(
  opts: { id?: number; canWrite?: boolean; search?: string; english?: boolean } = {},
) {
  window.history.replaceState({}, "", `/${opts.search ?? "?media_type=tv&tmdb_id=1001&season=0"}`);
  const Editor = (await import("./SubscriptionEditor.vue")).default;
  view = mountView(
    defineComponent({
      provide: { [Language as symbol]: Boolean(opts.english) },
      setup: () => () => h(Editor, { id: opts.id ?? 0, canWrite: opts.canWrite ?? true }),
    }),
  );
  await vi.waitFor(() => expect(document.querySelector("form")).not.toBeNull());
  await flush();
}

describe("手机追剧编辑页：条目与带出", () => {
  it("新建：条目、季的选块、「自动」会整理进的库，摘要按媒体库带出并写明来源", async () => {
    await mountEditor();
    expect(api.draft).toHaveBeenCalledWith("tv", 1001, 0);
    for (const id of ["se-item", "se-download", "se-search", "se-when"])
      expect(q(id)).not.toBeNull();
    expect(text("se-title")).toBe("测试剧");
    expect(text("se-item-line")).toBe("2026 · 剧集 · 一共 2 季 · 第 2 季 10 集");
    expect(q("se-season-2")?.getAttribute("aria-checked")).toBe("true");
    expect(q("se-season-1")?.getAttribute("aria-checked")).toBe("false");
    expect(q("se-tmdb")).toBeNull();
    expect(text("se-library")).toContain("自动（会整理进：剧集）");
    expect(text("se-download-summary")).toBe(
      "默认下载器 · /downloads/tv · 分类 tv · 标签 pt-tools",
    );
    expect(text("se-download-source")).toBe("来自媒体库「剧集」");
    expect(text("se-schedule-line")).toBe("按播出时间");
    expect(text("se-lock-line")).toContain("第一个下载成功的版本会成为锁");
    expect(q("se-save")).not.toBeNull();
    // 只列同类型、启用着的库
    expect(await optionLabels("se-library")).toEqual(["自动（会整理进：剧集）", "剧集", "动漫"]);
    // 带出不算改动
    expect(bridge.notify).not.toHaveBeenCalledWith("dirty", { value: true });
  });

  it("换库时来自库的项跟着换；自己改过的不动，可以换回媒体库的", async () => {
    await mountEditor();
    await pickOption("se-library", "动漫");
    expect(text("se-download-summary")).toBe("TR · /downloads/anime · 分类 anime · 不加标签");
    expect(text("se-download-source")).toBe("来自媒体库「动漫」");
    expect(bridge.notify).toHaveBeenCalledWith("dirty", { value: true });

    press("se-download-toggle");
    await flush();
    typeIn("se-category", "my-cat");
    await flush();
    await pickOption("se-library", "剧集");
    expect(text("se-download-summary")).toBe(
      "默认下载器 · /downloads/tv · 分类 my-cat · 标签 pt-tools",
    );
    expect(text("se-download-source")).toBe("部分来自媒体库「剧集」，其余是自己填的");

    press("se-category-reset");
    await flush();
    expect(text("se-download-summary")).toBe(
      "默认下载器 · /downloads/tv · 分类 tv · 标签 pt-tools",
    );

    const [path, verb, body] = await saved();
    expect([path, verb]).toEqual(["/api/app/v1/subscriptions", "POST"]);
    expect(body).toMatchObject({
      media_type: "tv",
      tmdb_id: 1001,
      season: 2,
      library_id: 1,
      downloader_id: 0,
      save_path: "/downloads/tv",
      category: "tv",
      tags: "pt-tools",
    });
    expect(body).not.toHaveProperty("lock_group");
    expect(body).not.toHaveProperty("lock_resolution");
    expect(body).not.toHaveProperty("from_torrent");
    expect(bridge.notify).toHaveBeenCalledWith("close", { changed: true });
  });

  it("库设的下载器停用了：不带出它，写明按默认下载器推送", async () => {
    api.libraries.mockResolvedValue([{ ...LIBS[0], download_downloader_id: 3 }, ...LIBS.slice(1)]);
    await mountEditor();
    expect(text("se-download-summary")).toBe(
      "默认下载器 · /downloads/tv · 分类 tv · 标签 pt-tools",
    );
    expect(text("se-downloader-note")).toBe(
      "媒体库「剧集」设的下载器「旧qB」已停用，按默认下载器推送",
    );
  });

  it("保存目录的候选：所选下载器的预设目录；下载器是「默认」时按下载器排列全部", async () => {
    await mountEditor();
    press("se-download-toggle");
    await flush();
    const el = q("se-save-path")!;
    const input = el instanceof HTMLInputElement ? el : el.querySelector("input")!;
    input.dispatchEvent(new Event("focus"));
    typeIn("se-save-path", "");
    await vi.waitFor(() =>
      expect(document.querySelectorAll(".el-autocomplete-suggestion li").length).toBe(2),
    );
    const rows = [...document.querySelectorAll<HTMLElement>(".el-autocomplete-suggestion li")].map(
      (li) => (li.textContent ?? "").replace(/\s+/g, " ").trim(),
    );
    expect(rows[0]).toContain("剧集目录");
    expect(rows[0]).toContain("qB");
    expect(rows[1]).toContain("/data/anime");
    expect(rows[1]).toContain("TR");
  });

  it("选另一季重新读条目；这一季已经在追时可以改成修改它", async () => {
    api.subscription.mockResolvedValue({
      title: "测试剧",
      year: 2026,
      media_type: "tv",
      tmdb_id: 1001,
      season: 1,
      library_id: 0,
      include_keywords: "",
      exclude_keywords: "",
      sites: [],
      schedule: { kind: "weekly", at: "21:30", days: [2] },
      lock_version: true,
      lock_group: "CHDWEB",
      lock_resolution: "1080p",
    });
    await mountEditor();
    press("se-season-1");
    await vi.waitFor(() => expect(api.draft).toHaveBeenLastCalledWith("tv", 1001, 1));
    await flush();
    expect(text("se-already")).toContain("这一季已经在追");

    press("se-switch-edit");
    await vi.waitFor(() => expect(api.subscription).toHaveBeenCalledWith(5));
    await flush();
    expect(q("se-already")).toBeNull();
    expect(q("se-season-1")).toBeNull();
    expect(text("se-item-line")).toBe("2026 · 剧集 · 第 1 季");
    expect(text("se-schedule-line")).toBe("每周 周二 · 21:30");
    expect(text("se-lock-line")).toContain("锁住：CHDWEB 1080p");
    // 修改时不带出：下载设置照订阅原样
    expect(text("se-download-summary")).toBe("默认下载器 · 下载器的默认目录 · 不加分类 · 不加标签");

    const [path, verb, body] = await saved();
    expect([path, verb]).toEqual(["/api/app/v1/subscriptions/5", "PUT"]);
    expect(body).not.toHaveProperty("media_type");
    expect(body).not.toHaveProperty("tmdb_id");
    expect(body).not.toHaveProperty("library_id");
    expect(body).not.toHaveProperty("lock_group");
  });

  it("从「+」进来：填 TMDB 编号点「查找」；改了编号以后查到的条目不算了", async () => {
    await mountEditor({ search: "" });
    expect(api.draft).not.toHaveBeenCalled();
    expect(q("se-title")?.textContent).toContain("新增追剧订阅");
    typeIn("se-tmdb", "1001");
    await flush();
    press("se-lookup");
    await vi.waitFor(() => expect(api.draft).toHaveBeenCalledWith("tv", 1001, 0));
    await flush();
    expect(text("se-title")).toBe("测试剧");
    expect(q("se-season-2")?.getAttribute("aria-checked")).toBe("true");
    expect(text("se-library")).toContain("自动（会整理进：剧集）");

    typeIn("se-tmdb", "2002");
    await flush();
    expect(text("se-title")).toBe("新增追剧订阅");
    expect(q("se-season-2")).toBeNull();
    expect(q("se-season")).not.toBeNull();
    expect(text("se-library")).toContain("自动（按类型选）");
  });
});

describe("手机追剧编辑页：旧主机、只读设备与修改", () => {
  it("旧主机没有 subscription_assist：不读条目、不试搜索、不带出，照以前填类型、编号与季", async () => {
    api.meta.mockRejectedValue(new BridgeError("Not found", 404));
    await mountEditor();
    expect(api.draft).not.toHaveBeenCalled();
    expect(q("se-type")).not.toBeNull();
    expect(q("se-tmdb")).not.toBeNull();
    expect(q("se-season")).not.toBeNull();
    expect(q("se-lookup")).toBeNull();
    expect(q("se-preview")).toBeNull();
    expect(text("se-library")).toContain("自动（按类型选）");
    expect(text("se-download-summary")).toBe("默认下载器 · 下载器的默认目录 · 不加分类 · 不加标签");
    expect(q("se-download-source")).toBeNull();
    const [, verb, body] = await saved();
    expect(verb).toBe("POST");
    expect(body).toMatchObject({
      media_type: "tv",
      tmdb_id: 1001,
      season: 0,
      save_path: "",
      category: "",
    });
  });

  it("读不到 /media/libraries 时媒体库照旧用选项里的", async () => {
    api.meta.mockResolvedValue({ features: [] });
    api.libraries.mockRejectedValue(new BridgeError("Not found", 404));
    await mountEditor();
    expect(await optionLabels("se-library")).toEqual([
      "自动（按类型选）",
      "剧集",
      "动漫",
      "电影库",
      "旧剧集库",
    ]);
  });

  it("只读设备：照旧只读，没有试搜索和保存", async () => {
    await mountEditor({ canWrite: false });
    expect(q("se-preview")).toBeNull();
    expect(q("se-save")).toBeNull();
    submit();
    await flush();
    expect(bridge.request).not.toHaveBeenCalled();
  });

  it("修改：锁住的版本可以清掉；没改锁不发锁，清掉时两项都发空串；换了库可以套用", async () => {
    api.subscription.mockResolvedValue({
      title: "测试剧",
      media_type: "tv",
      tmdb_id: 1001,
      season: 2,
      library_id: 1,
      downloader_id: 1,
      save_path: "/old",
      category: "old",
      tags: "",
      include_keywords: "WEB",
      exclude_keywords: "",
      sites: ["hdsky"],
      schedule: { kind: "airtime", at: "20:00" },
      lock_version: true,
      lock_group: "CHDWEB",
      lock_resolution: "1080p",
    });
    await mountEditor({ id: 7, search: "" });
    expect(text("se-title")).toBe("测试剧");
    expect(text("se-schedule-line")).toBe("按播出时间 · 20:00");
    expect(text("se-lock-line")).toContain("锁住：CHDWEB 1080p");
    expect(text("se-download-summary")).toBe("qB · /old · 分类 old · 不加标签");

    let [path, verb, body] = await saved();
    expect([path, verb]).toEqual(["/api/app/v1/subscriptions/7", "PUT"]);
    expect(body).not.toHaveProperty("lock_group");
    expect(body).not.toHaveProperty("library_id");
    expect(body).toMatchObject({ include_keywords: "WEB", sites: ["hdsky"], downloader_id: 1 });

    press("se-lock-clear");
    await flush();
    expect(text("se-lock-line")).toContain("第一个下载成功的版本会成为锁");
    await pickOption("se-library", "动漫");
    expect(text("se-apply")).toContain("套用「动漫」的下载设置");
    // 不点「套用」就不换
    expect(text("se-download-summary")).toBe("qB · /old · 分类 old · 不加标签");
    press("se-apply-button");
    await flush();
    expect(text("se-download-summary")).toBe("TR · /downloads/anime · 分类 anime · 不加标签");

    bridge.request.mockClear();
    [path, verb, body] = await saved();
    expect(body).toMatchObject({
      library_id: 2,
      lock_group: "",
      lock_resolution: "",
      downloader_id: 2,
      save_path: "/downloads/anime",
      category: "anime",
    });
  });
});

describe("手机追剧编辑页：试搜索", () => {
  async function openPreview() {
    press("se-preview");
    await vi.waitFor(() => expect(api.preview).toHaveBeenCalled());
    await vi.waitFor(() => expect(q("msp-ok-0")).not.toBeNull());
    await flush();
  }
  const tabLabels = () =>
    [...document.querySelectorAll<HTMLElement>("[data-testid=msp-tabs] .el-segmented__item")].map(
      (el) => (el.textContent ?? "").trim(),
    );
  async function chooseTab(prefix: string) {
    const item = [
      ...document.querySelectorAll<HTMLElement>("[data-testid=msp-tabs] .el-segmented__item"),
    ].find((el) => (el.textContent ?? "").trim().startsWith(prefix));
    if (!item) throw new Error(`找不到标签「${prefix}」`);
    item.click();
    await flush();
  }
  async function act(card: string, action: string) {
    press(card);
    await vi.waitFor(() => expect(q(action)).not.toBeNull());
    press(action);
    await flush();
  }

  it("按表单搜一次：三栏、搜的词、用时与这一季的情况", async () => {
    await mountEditor();
    await openPreview();
    expect(api.preview.mock.calls[0]![0]).toEqual({
      media_type: "tv",
      tmdb_id: 1001,
      season: 2,
      profile_id: 0,
      sites: [],
      include_keywords: "",
      exclude_keywords: "",
      include_unmatched: true,
    });
    expect(api.preview.mock.calls[0]![1]?.signal).toBeInstanceOf(AbortSignal);
    expect(tabLabels()).toEqual(["符合条件 1", "不符合 1", "对不上这一季 1"]);
    expect(text("msp-keywords")).toBe("测试剧、Test Show");
    expect(text("msp-took")).toContain("2.3 秒");
    expect(text("msp-season")).toBe("缺 E02、E03");
    expect(text("msp-ok-0-title")).toBe(TITLE);
    expect(text("msp-ok-0-note")).toBe("能补上 E02、E03");
    await chooseTab("不符合");
    expect(text("msp-reject-0-reject")).toBe("不符合：标题含排除词 hdtv");
    await chooseTab("对不上");
    expect(text("msp-unmatched-0")).toContain("名字对不上");
    // 编辑页记下上次的结果
    expect(text("se-preview-last")).toBe("上次：符合 1 · 不符合 1 · 对不上这一季 1（2.3 秒）");
  });

  it("结果上的操作只改表单：只要、不要、锁这个版本、从这个种子开始追；保存时才发出去", async () => {
    await mountEditor();
    await openPreview();
    await act("msp-ok-0", "msp-only");
    expect(tags("se-include")).toEqual(["CHDWEB"]);
    expect(ui.success).toHaveBeenCalledWith("已加进标题包含：CHDWEB");
    // 条件改过了：提示按新条件再试
    expect(q("msp-stale")).not.toBeNull();
    expect(text("msp-run")).toBe("按新条件再试");

    await chooseTab("不符合");
    await act("msp-reject-0", "msp-not");
    expect(tags("se-exclude")).toEqual(["XYZ"]);
    await chooseTab("符合");

    await act("msp-ok-0", "msp-lock-this");
    expect(text("se-lock-line")).toContain("锁住：CHDWEB 1080p");
    expect(text("msp-lock")).toBe("只收 CHDWEB 1080p");

    await act("msp-ok-0", "msp-follow");
    expect(text("se-from")).toContain(TITLE);
    expect(q("se-start")!.querySelector("input")!.value).toBe("2");

    // 同一个词不加两次
    await act("msp-ok-0", "msp-only");
    expect(tags("se-include")).toEqual(["CHDWEB"]);
    expect(ui.warning).toHaveBeenCalledWith("已经有「CHDWEB」了");

    const [, verb, body] = await saved();
    expect(verb).toBe("POST");
    expect(body).toMatchObject({
      include_keywords: "CHDWEB",
      exclude_keywords: "XYZ",
      lock_version: true,
      lock_group: "CHDWEB",
      lock_resolution: "1080p",
      start_episode: 2,
      from_torrent: { site: "hdsky", torrent_id: "101", title: TITLE },
    });
  });

  it("换了站点再从它开始追；「不从它开始」时起始集改回原来的", async () => {
    await mountEditor();
    await openPreview();
    press("msp-ok-0");
    await vi.waitFor(() => expect(q("msp-member-1")).not.toBeNull());
    (q("msp-member-1")!.querySelector("input") ?? q("msp-member-1")!).click();
    await flush();
    press("msp-follow");
    await flush();
    const body = (await saved())[2];
    expect(body.from_torrent).toEqual({
      site: "ttg",
      torrent_id: "201",
      title: "Test Show S02E02-E03 1080p WEB-DL H264-CHDWEB",
    });
    press("se-unfollow");
    await flush();
    expect(q("se-from")).toBeNull();
    expect(q("se-start")!.querySelector("input")!.value).toBe("1");
    bridge.request.mockClear();
    expect((await saved())[2]).not.toHaveProperty("from_torrent");
  });

  it("复制标题、副标题走原生桥的 copy，「已复制」由页面自己显示", async () => {
    await mountEditor();
    await openPreview();
    await act("msp-ok-0", "msp-copy-title");
    expect(bridge.copyText).toHaveBeenCalledWith(TITLE);
    expect(ui.success).toHaveBeenCalledWith("已复制标题");
    await act("msp-ok-0", "msp-copy-subtitle");
    expect(bridge.copyText).toHaveBeenLastCalledWith("测试剧 第2-3集");
    expect(ui.success).toHaveBeenCalledWith("已复制副标题");
    bridge.copyText.mockRejectedValueOnce(new BridgeError("Text is too long", 413));
    await act("msp-ok-0", "msp-copy-title");
    expect(ui.error).toHaveBeenCalledWith("复制失败：Text is too long");
  });

  it("选标题里的一段：有集号时警告，选别的一段加进排除", async () => {
    await mountEditor();
    await openPreview();
    await act("msp-ok-0", "msp-pick");
    // Test | Show | S02E02-E03 | 1080p | WEB-DL | H264-CHDWEB
    press("msp-token-2");
    press("msp-token-2");
    await flush();
    expect(text("msp-picked")).toBe("S02E02-E03");
    expect(q("msp-picked-warn")).not.toBeNull();
    press("msp-token-3");
    press("msp-token-4");
    await flush();
    expect(text("msp-picked")).toBe("1080p.WEB-DL");
    expect(q("msp-picked-warn")).toBeNull();
    press("msp-pick-exclude");
    await flush();
    expect(tags("se-exclude")).toEqual(["1080p.WEB-DL"]);
  });

  it("取消只取消这一次：不报错，马上可以再试", async () => {
    let signal: AbortSignal | undefined;
    api.preview.mockImplementationOnce(
      (_body: unknown, options: { signal?: AbortSignal }) =>
        new Promise((_resolve, reject) => {
          signal = options.signal;
          signal?.addEventListener("abort", () =>
            reject(new BridgeError("Cancelled / 已取消", 499)),
          );
        }),
    );
    await mountEditor();
    press("se-preview");
    await vi.waitFor(() => expect(q("msp-loading")).not.toBeNull());
    expect(text("msp-loading")).toContain("正在搜 2 个站点，最多 25 秒");
    press("msp-cancel");
    await flush();
    expect(signal?.aborted).toBe(true);
    expect(q("msp-loading")).toBeNull();
    expect(q("msp-error")).toBeNull();
    expect(text("msp-run")).toBe("试搜索");
    press("msp-run");
    await vi.waitFor(() => expect(q("msp-ok-0")).not.toBeNull());
    expect(api.preview).toHaveBeenCalledTimes(2);
    expect(q("msp-error")).toBeNull();
  });

  it("超时（原生桥 35 秒回 504）写「超时：少选几个站点再试」", async () => {
    api.preview.mockRejectedValueOnce(new BridgeError("timeout", 504));
    await mountEditor();
    press("se-preview");
    await vi.waitFor(() => expect(q("msp-error")).not.toBeNull());
    expect(text("msp-error")).toBe("超时：少选几个站点再试");
  });

  it("英文：摘要、结果与操作都按英文写", async () => {
    await mountEditor({ english: true });
    expect(text("se-download-summary")).toBe(
      "Default downloader · /downloads/tv · Category tv · Tags pt-tools",
    );
    expect(text("se-download-source")).toBe('From library "剧集"');
    expect(text("se-library")).toContain("Automatic (organized into: 剧集)");
    await openPreview();
    expect(tabLabels()).toEqual(["Matching 1", "Rejected 1", "Not this season 1"]);
    expect(text("msp-ok-0-note")).toBe("Fills E02, E03");
    expect(text("msp-season")).toBe("Missing E02, E03");
  });
});
