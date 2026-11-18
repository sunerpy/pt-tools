// @vitest-environment happy-dom
/*
 * 钉子（追剧方案 F2，3.2.4–3.2.6）：订阅弹窗的「试搜索」。按还没保存的表单搜（草稿：档案、站点、标题包含与排除），
 * 结果在右侧抽屉里分三栏；结果上的操作只改表单：复制标题、只要或不要制作组、选标题里的一段、锁这个版本、
 * 保存时从这个种子开始追。2026-11-07 用户：「还可以试搜索列出检索后的结果 并可以比如复制标题等内容作为搜索条件进行准确追剧」。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick, ref } from "vue";

import type { MediaMeta, MediaSearchGroup, MediaSearchMember, MediaSearchResult } from "@/api";
import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  profiles: vi.fn(async () => ({ items: [] })),
  create: vi.fn(),
  update: vi.fn(),
  previewSchedule: vi.fn(async () => ({ next: [] })),
  settings: vi.fn(async () => ({})),
  episodeGroups: vi.fn(async () => ({ groups: [] })),
  draft: vi.fn(async () => null),
  searchMedia: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  warning: vi.fn(),
  error: vi.fn(),
  info: vi.fn(),
}));
const clip = vi.hoisted(() => ({ copyText: vi.fn(async () => true) }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    subscribeApi: { ...real.subscribeApi, ...api },
    downloadersApi: { list: vi.fn(async () => []) },
    downloaderDirectoriesApi: {
      ...real.downloaderDirectoriesApi,
      listAll: vi.fn(async () => ({})),
    },
    sitesApi: { list: vi.fn(async () => ({ hdsky: { enabled: true }, ttg: { enabled: true } })) },
    organizeApi: { ...real.organizeApi, libraries: vi.fn(async () => []) },
    mediaApi: { ...real.mediaApi, settings: vi.fn(async () => ({ image_base_url: "" })) },
  };
});
vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return { ...real, ElMessage: Object.assign(vi.fn(), ui) };
});
vi.mock("@/utils/clipboard", () => clip);

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  document.body.innerHTML = "";
  vi.clearAllMocks();
  clip.copyText.mockImplementation(async () => true);
});

const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);
const text = (id: string) => (q(id)?.textContent ?? "").replace(/\s+/g, " ").trim();
const area = (id: string) => {
  const el = q(id)!;
  return (
    el instanceof HTMLTextAreaElement ? el : el.querySelector("textarea")
  ) as HTMLTextAreaElement;
};
const input = (id: string) => {
  const el = q(id)!;
  return (el instanceof HTMLInputElement ? el : el.querySelector("input")) as HTMLInputElement;
};

function fill(id: string, v: string) {
  const i = area(id);
  i.value = v;
  i.dispatchEvent(new Event("input"));
}

/** 点 el-button（真正的 button 可能就是带 testid 的那个元素） */
function press(id: string) {
  const el = q(id);
  if (!el) throw new Error(`找不到 ${id}`);
  (el.closest("button") ?? el).click();
}

async function flush() {
  for (let i = 0; i < 4; i++) {
    await Promise.resolve();
    await nextTick();
  }
}

async function chooseTab(prefix: string) {
  const item = [
    ...document.querySelectorAll<HTMLElement>("[data-testid=sp-tabs] .el-segmented__item"),
  ].find((el) => (el.textContent ?? "").trim().startsWith(prefix));
  if (!item) throw new Error(`找不到标签「${prefix}」`);
  item.click();
  await flush();
}

const tabLabels = () =>
  [...document.querySelectorAll<HTMLElement>("[data-testid=sp-tabs] .el-segmented__item")].map(
    (el) => (el.textContent ?? "").trim(),
  );

const meta = (group: string, resolution: string): MediaMeta =>
  ({ group, resolution, source: "WEB-DL" }) as MediaMeta;

function member(
  site: string,
  id: string,
  title: string,
  extra: Partial<MediaSearchMember> = {},
): MediaSearchMember {
  return {
    site,
    torrent_id: id,
    title,
    size_bytes: 4 << 30,
    seeders: 12,
    leechers: 1,
    discount: "FREE",
    free: true,
    hr: false,
    meta: meta("", ""),
    verdict: { score: 80 },
    ...extra,
  };
}

function group(
  id: string,
  m: MediaMeta,
  members: MediaSearchMember[],
  extra: Partial<MediaSearchGroup> = {},
): MediaSearchGroup {
  const verdict = extra.verdict ?? { score: 80 };
  return {
    group_id: id,
    meta: m,
    verdict,
    covers: [],
    downloading: [],
    in_library: false,
    members: members.map((x) => ({ ...x, meta: m, verdict })),
    ...extra,
  };
}

/** 一季剧：CHDWEB 2160p（两个站点）、HHWEB 1080p 整季包能补缺的集；ADWeb 被排除词挡下；一个拉黑的 */
function tvResult(over: Partial<MediaSearchResult> = {}): MediaSearchResult {
  return {
    item: { media_type: "tv", tmdb_id: 7, season: 1, title: "梦想" },
    subscription: null,
    profile: { id: 0, name: "" },
    keywords: ["梦想", "Dream"],
    episodes: [
      { episode: 1, state: "library" },
      { episode: 2, state: "missing" },
      { episode: 3, state: "missing" },
    ],
    groups: [
      group(
        "g1",
        meta("CHDWEB", "2160p"),
        [
          member("hdsky", "101", "Dream.S01E02-E03.2160p.WEB-DL.H265-CHDWEB", {
            subtitle: "梦想 第2-3集",
          }),
          member("ttg", "202", "Dream S01E02-E03 2160p WEB-DL-CHDWEB"),
        ],
        { span: { start: 2, end: 3 }, covers: [2, 3] },
      ),
      group(
        "g2",
        meta("HHWEB", "1080p"),
        [member("hdsky", "103", "Dream.S01.1080p.WEB-DL-HHWEB")],
        {
          span: { whole: true },
          covers: [2, 3],
        },
      ),
      group(
        "g3",
        meta("ADWeb", "1080p"),
        [member("ttg", "204", "Dream.S01E02.1080p.WEB-DL-ADWeb")],
        {
          span: { start: 2, end: 2 },
          verdict: { score: 0, reject: "标题含排除词 adweb" },
        },
      ),
      group(
        "g4",
        meta("XX", "720p"),
        [member("ttg", "205", "Dream.S01E03.720p-XX", { block: { id: 1, reason: "假种" } })],
        { blocked: true },
      ),
    ],
    total: 5,
    blocked_count: 1,
    unmatched_count: 1,
    unmatched: [member("hdsky", "301", "Dream.S02E01.1080p", { reason: "季对不上（第 2 季）" })],
    errors: [],
    ...over,
  };
}

const tvSub = {
  id: 4,
  media_type: "tv",
  tmdb_id: 7,
  title: "梦想",
  year: 2026,
  season: 1,
  status: "active",
  profile_id: 0,
  sites: ["ttg"],
  downloader_id: 0,
  category: "",
  tags: "",
  save_path: "",
  upgrade: false,
  include_keywords: "",
  exclude_keywords: "",
  lock_version: true,
  lock_group: "CHDWEB",
  lock_resolution: "2160p",
  schedule: { kind: "airtime" },
};

async function mountDialog(props: Record<string, unknown> = {}) {
  const Dialog = (await import("./SubscribeDialog.vue")).default;
  const Host = defineComponent({
    setup() {
      const open = ref(true);
      return () =>
        h(Dialog, {
          modelValue: open.value,
          "onUpdate:modelValue": (v: boolean) => (open.value = v),
          ...(Object.keys(props).length
            ? props
            : { target: { kind: "tv", tmdb_id: 7, title: "梦想" } }),
        });
    },
  });
  view = mountView(Host);
  await vi.waitFor(() => expect(q("sd-preview")).not.toBeNull());
  await flush();
}

async function openPreview(res: MediaSearchResult = tvResult()) {
  api.searchMedia.mockResolvedValueOnce(res);
  press("sd-preview");
  await vi.waitFor(() => expect(q("sp-facts")).not.toBeNull());
  await flush();
}

describe("试搜索：按还没保存的表单搜", () => {
  it("请求带草稿、季与站点，要对不上的；结果分三栏，写出搜的词、这一季缺的集与每组能补什么", async () => {
    await mountDialog();
    fill("sd-include", "CHDWEB");
    fill("sd-exclude", "HDTV");
    await flush();
    await openPreview();

    expect(api.searchMedia).toHaveBeenCalledTimes(1);
    expect(api.searchMedia.mock.calls[0]![0]).toEqual({
      media_type: "tv",
      tmdb_id: 7,
      season: 0,
      sites: [],
      include_unmatched: true,
      draft: { profile_id: 0, include_keywords: "CHDWEB", exclude_keywords: "HDTV" },
    });
    expect(text("sp-keywords")).toBe("梦想、Dream");
    expect(text("sp-took")).toContain("没有质量档案：按分辨率与来源比");
    expect(text("sp-season")).toBe("缺 E02、E03");
    expect(text("sp-lock-line")).toBe("开着、还没定版本：第一个下载成功的版本会成为锁");
    expect(q("sp-cache")).not.toBeNull();
    expect(tabLabels()).toEqual(["符合条件 2", "不符合 2", "对不上这一季 1"]);
    expect(text("sp-ok-0-title")).toBe("Dream.S01E02-E03.2160p.WEB-DL.H265-CHDWEB");
    expect(text("sp-ok-0-note")).toBe("能补上 E02、E03");
    expect(q("sp-stale")).toBeNull();
  });

  it("不符合写出全部原因（拉黑的写明拉黑了），对不上的写原因、能复制标题", async () => {
    await mountDialog();
    await openPreview();

    await chooseTab("不符合");
    expect(text("sp-reject-0-reject")).toBe("不符合：标题含排除词 adweb");
    expect(text("sp-reject-1-reject")).toBe("不符合：拉黑了（假种）");
    expect(q("sp-reject-1-m-0")?.textContent).toContain("已拉黑");

    await chooseTab("对不上这一季");
    expect(q("sp-unmatched-0")?.textContent).toContain("季对不上（第 2 季）");
    press("sp-unmatched-0-copy-title");
    await flush();
    expect(clip.copyText).toHaveBeenCalledWith("Dream.S02E01.1080p");
  });

  it("只要、不要制作组：加进表单、重复的不加；表单改了提示再试，再试按新表单搜", async () => {
    await mountDialog();
    await openPreview();

    press("sp-ok-0-only");
    await flush();
    expect(area("sd-include").value).toBe("CHDWEB");
    expect(ui.success).toHaveBeenCalledWith("已加进标题包含：CHDWEB");
    press("sp-ok-0-only");
    await flush();
    expect(area("sd-include").value).toBe("CHDWEB");
    expect(ui.warning).toHaveBeenCalledWith("已经有「CHDWEB」了");
    press("sp-ok-1-not");
    await flush();
    expect(area("sd-exclude").value).toBe("HHWEB");

    expect(q("sp-stale")).not.toBeNull();
    api.searchMedia.mockResolvedValueOnce(
      tvResult({ errors: [{ site: "ttg", error: "60 秒内没有回应", timed_out: true }] }),
    );
    press("sp-run");
    await vi.waitFor(() => expect(api.searchMedia).toHaveBeenCalledTimes(2));
    await flush();
    expect(api.searchMedia.mock.calls[1]![0].draft).toEqual({
      profile_id: 0,
      include_keywords: "CHDWEB",
      exclude_keywords: "HHWEB",
    });
    expect(q("sp-stale")).toBeNull();
    expect(text("sp-errors")).toContain("ttg：60 秒内没有回应");
    expect(text("sp-errors")).toContain("结果没有进搜索缓存");
    expect(q("sp-cache")).toBeNull();
  });

  it("选标题里的一段：点起止两块取原标题的这一段；有集号时提醒；加进包含或排除", async () => {
    await mountDialog();
    await openPreview();

    press("sp-ok-0-pick");
    await flush();
    // Dream | S01E02-E03 | 2160p | WEB-DL | H265-CHDWEB
    q("sp-token-2")!.click();
    q("sp-token-3")!.click();
    await flush();
    expect(text("sp-pick-text")).toBe("2160p.WEB-DL");
    expect(q("sp-pick-warn")).toBeNull();
    press("sp-pick-include");
    await flush();
    expect(area("sd-include").value).toBe("2160p.WEB-DL");
    expect(q("sp-picker")).toBeNull();

    press("sp-ok-0-pick");
    await flush();
    q("sp-token-1")!.click();
    q("sp-token-1")!.click();
    await flush();
    expect(text("sp-pick-text")).toBe("S01E02-E03");
    expect(q("sp-pick-warn")).not.toBeNull();
    press("sp-pick-exclude");
    await flush();
    expect(area("sd-exclude").value).toBe("S01E02-E03");
  });

  it("一组里选另一个站点的种子：复制它的标题；浏览器不让复制时提示手动复制", async () => {
    await mountDialog();
    await openPreview();

    press("sp-ok-0-copy-subtitle");
    await flush();
    expect(clip.copyText).toHaveBeenLastCalledWith("梦想 第2-3集");
    expect(ui.success).toHaveBeenCalledWith("已复制副标题");

    q("sp-ok-0-m-1")!.click();
    await flush();
    expect(text("sp-ok-0-title")).toBe("Dream S01E02-E03 2160p WEB-DL-CHDWEB");
    expect(q("sp-ok-0-copy-subtitle")).toBeNull();
    clip.copyText.mockImplementation(async () => false);
    press("sp-ok-0-copy-title");
    await flush();
    expect(clip.copyText).toHaveBeenLastCalledWith("Dream S01E02-E03 2160p WEB-DL-CHDWEB");
    expect(ui.warning).toHaveBeenCalledWith("浏览器不让复制：选中文字手动复制");
  });

  it("锁这个版本：新建时一开始就锁住，别的组标「版本锁不收」，保存时发出；清空以后不发", async () => {
    api.create.mockResolvedValue({ id: 9, title: "梦想" });
    await mountDialog();
    await openPreview();

    press("sp-ok-1-lock");
    await flush();
    expect(text("sd-new-lock")).toContain("一开始就锁住：HHWEB 1080p");
    expect(text("sp-lock-line")).toBe("只收 HHWEB 1080p");
    expect(text("sp-ok-0-lock-miss")).toBe("版本锁不收（HHWEB 1080p）");
    expect(q("sp-ok-1-lock-miss")).toBeNull();
    expect(text("sp-ok-1-lock")).toBe("已锁这个版本");
    expect(q("sp-ok-1-lock")!.closest("button")!.disabled).toBe(true);

    press("sd-save");
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    let body = api.create.mock.calls[0]![0];
    expect(body).toMatchObject({
      lock_version: true,
      lock_group: "HHWEB",
      lock_resolution: "1080p",
    });

    api.create.mockClear();
    view?.unmount();
    await mountDialog();
    await openPreview();
    press("sp-ok-1-lock");
    await flush();
    press("sd-new-lock-clear");
    await flush();
    expect(q("sd-new-lock")).toBeNull();
    press("sd-save");
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    body = api.create.mock.calls[0]![0];
    expect(body.lock_version).toBe(true);
    expect(body).not.toHaveProperty("lock_group");
    expect(body).not.toHaveProperty("lock_resolution");
  });

  it("保存时从这个种子开始追：起始集设成它的第一集，整季包不改；不从它开始时改回去；保存发出种子", async () => {
    api.create.mockResolvedValue({ id: 9, title: "梦想", message: "已推送" });
    await mountDialog();
    await openPreview();

    press("sp-ok-0-follow");
    await flush();
    expect(text("sd-from")).toContain("Dream.S01E02-E03.2160p.WEB-DL.H265-CHDWEB");
    expect(input("sd-start").value).toBe("2");
    expect(text("sp-ok-0-follow")).toBe("保存时从它开始追");
    expect(ui.success).toHaveBeenCalledWith("保存以后从这个种子开始追（从第 2 集起）");

    press("sp-ok-1-follow");
    await flush();
    expect(text("sd-from")).toContain("Dream.S01.1080p.WEB-DL-HHWEB");
    expect(input("sd-start").value).toBe("2");
    expect(ui.success).toHaveBeenCalledWith("保存以后从这个种子开始追（整季包，起始集不变）");

    press("sd-unfollow");
    await flush();
    expect(q("sd-from")).toBeNull();
    expect(input("sd-start").value).toBe("");

    press("sp-ok-0-follow");
    await flush();
    press("sd-save");
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    const body = api.create.mock.calls[0]![0];
    expect(body.from_torrent).toEqual({
      site: "hdsky",
      torrent_id: "101",
      title: "Dream.S01E02-E03.2160p.WEB-DL.H265-CHDWEB",
    });
    expect(body.start_episode).toBe(2);
    expect(ui.info).toHaveBeenCalledWith("已推送");
  });

  it("从搜索结果的种子打开时：带进来的种子照旧保存，不能「不从它开始」", async () => {
    api.create.mockResolvedValue({ id: 9, title: "梦想" });
    await mountDialog({
      target: { kind: "tv", tmdb_id: 7, title: "梦想" },
      fromTorrent: {
        site: "ttg",
        torrent_id: "202",
        title: "Dream S01E02-E03 2160p WEB-DL-CHDWEB",
      },
    });
    expect(text("sd-from")).toContain("Dream S01E02-E03 2160p WEB-DL-CHDWEB");
    expect(q("sd-unfollow")).toBeNull();
    await openPreview();
    q("sp-ok-0-m-1")!.click();
    await flush();
    expect(text("sp-ok-0-follow")).toBe("保存时从它开始追");
    press("sd-save");
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0].from_torrent).toEqual({
      site: "ttg",
      torrent_id: "202",
      title: "Dream S01E02-E03 2160p WEB-DL-CHDWEB",
    });
  });

  it("修改追剧：按订阅的季与站点搜，标出锁住的版本；没有从种子开始追；改锁走修改的锁", async () => {
    api.update.mockResolvedValue({ ...tvSub });
    await mountDialog({ sub: tvSub });
    await openPreview(tvResult({ subscription: { id: 4, status: "active", lock: null } }));

    expect(api.searchMedia.mock.calls[0]![0]).toMatchObject({ season: 1, sites: ["ttg"] });
    expect(text("sp-lock-line")).toBe("只收 CHDWEB 2160p");
    expect(text("sp-ok-1-lock-miss")).toBe("版本锁不收（CHDWEB 2160p）");
    expect(q("sp-ok-0-follow")).toBeNull();
    press("sp-ok-1-lock");
    await flush();
    expect(input("sd-lock-group").value).toBe("HHWEB");
    press("sd-save");
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    expect(api.update.mock.calls[0]![1]).toMatchObject({
      lock_group: "HHWEB",
      lock_resolution: "1080p",
    });
  });

  it("电影：没有锁与从种子开始追，第三栏是对不上这部电影；接口报错时写出原因", async () => {
    await mountDialog({ target: { kind: "movie", tmdb_id: 278, title: "肖申克的救赎" } });
    const movie: MediaSearchResult = {
      ...tvResult(),
      item: { media_type: "movie", tmdb_id: 278, title: "肖申克的救赎" },
      episodes: [],
      movie: { state: "missing" },
      groups: [
        group("m1", meta("WiKi", "1080p"), [
          member("hdsky", "1", "The.Shawshank.Redemption.1994.1080p.BluRay.x264-WiKi"),
        ]),
      ],
      unmatched: [],
      unmatched_count: 0,
    };
    await openPreview(movie);
    expect(api.searchMedia.mock.calls[0]![0]).toMatchObject({ media_type: "movie", season: 0 });
    expect(tabLabels()).toEqual(["符合条件 1", "不符合 0", "对不上这部电影 0"]);
    expect(text("sp-ok-0-note")).toBe("库里还没有");
    expect(q("sp-ok-0-lock")).toBeNull();
    expect(q("sp-ok-0-follow")).toBeNull();
    expect(q("sp-lock-line")).toBeNull();

    api.searchMedia.mockRejectedValueOnce(new Error("参数不对: 指定的站点都没有启用"));
    press("sp-run");
    await vi.waitFor(() => expect(text("sp-error")).toBe("指定的站点都没有启用"));
  });
});
