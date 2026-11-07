/*
 * 钉子：新建订阅时按媒体库带出下载设置（追剧方案 3.1.3 的 1–6 条、D7）。2026-11-07 用户：「当我选择了媒体库 剧集 类型后，
 * 应该支持自动带出一些设置的比如保存目录、分类、标签等内容」。网页的订阅弹窗与手机的追剧编辑页共用这些规则。
 */
import type { DownloaderSetting, MediaLibrary } from "@/api";
import { describe, expect, it } from "vitest";

import {
  applyAll,
  blankValues,
  bringOut,
  differs,
  effectiveLibrary,
  libraryDefaults,
  markUser,
  newSources,
  resetField,
  sameValues,
  scopeMisses,
  sourceNote,
  userSources,
  type DefaultsForm,
} from "./subscribeDefaults";

const dl = (id: number, name: string, enabled = true): DownloaderSetting => ({
  id,
  name,
  type: "qbittorrent",
  url: "",
  username: "",
  is_default: id === 1,
  enabled,
});
const DLS = [dl(1, "qB"), dl(2, "qB-2", false)];

const lib = (id: number, name: string, extra: Partial<MediaLibrary> = {}): MediaLibrary =>
  ({
    id,
    name,
    kind: "tv",
    anime: false,
    enabled: true,
    download_downloader_id: 0,
    download_save_path: "",
    download_category: "",
    download_tags: "",
    ...extra,
  }) as MediaLibrary;

const TV = lib(1, "剧集", {
  download_downloader_id: 1,
  download_save_path: "/downloads/tv",
  download_category: "tv",
  download_tags: "pt-tools",
});
const ANIME = lib(2, "动漫", {
  anime: true,
  download_save_path: "/downloads/anime",
  download_category: "anime",
});
const TV2 = lib(3, "剧集 2", {
  download_downloader_id: 2,
  download_save_path: "/downloads/tv2",
  download_category: "tv2",
});
const LIBS = [TV, ANIME, TV2];

const fresh = (): DefaultsForm => ({ values: blankValues(), sources: newSources() });

describe("subscribeDefaults", () => {
  it("1. 生效的库：选了具体的库用它；选「自动」时用草稿里会整理进的库；都没有就不带出", () => {
    expect(effectiveLibrary(LIBS, 3, 1)?.name).toBe("剧集 2");
    expect(effectiveLibrary(LIBS, 0, 2)?.name).toBe("动漫");
    expect(effectiveLibrary(LIBS, 0, 0)).toBeNull();
    expect(effectiveLibrary(LIBS, 9, 1), "选的库不在列表里时不猜").toBeNull();
    expect(effectiveLibrary([], 0, 1)).toBeNull();
  });

  it("2. 新建：没动过的四项填成库的值并标来源；库没设的项保持空，写明推送时怎么办", () => {
    const f = bringOut(fresh(), libraryDefaults(TV, DLS));
    expect(f.values).toEqual({
      downloader_id: 1,
      save_path: "/downloads/tv",
      category: "tv",
      tags: "pt-tools",
    });
    expect(sourceNote(f, "category")).toBe("来自媒体库「剧集」");
    expect(sourceNote(f, "downloader_id")).toBe("来自媒体库「剧集」");

    const a = bringOut(fresh(), libraryDefaults(ANIME, DLS));
    expect(a.values).toEqual({
      downloader_id: 0,
      save_path: "/downloads/anime",
      category: "anime",
      tags: "",
    });
    expect(sourceNote(a, "tags")).toBe("媒体库「动漫」没设，不加标签");
    expect(sourceNote(a, "downloader_id")).toBe("媒体库「动漫」没设下载器，用设置里的默认下载器");

    const empty = bringOut(fresh(), libraryDefaults(lib(4, "空库"), DLS));
    expect(empty.values).toEqual(blankValues());
    expect(sourceNote(empty, "save_path")).toBe("媒体库「空库」没设，按下载器的默认目录");
    expect(sourceNote(empty, "category")).toBe("媒体库「空库」没设，不加分类");
    expect(sourceNote(fresh(), "category"), "还没带出过时不写").toBe("");
  });

  it("D7：库里的下载器停用了或删了时保持默认，写明原因；删了的不写名字；列表没读到时不带出", () => {
    const off = bringOut(fresh(), libraryDefaults(TV2, DLS));
    expect(off.values.downloader_id, "停用的不带出").toBe(0);
    expect(off.values.save_path).toBe("/downloads/tv2");
    expect(sourceNote(off, "downloader_id")).toBe(
      "媒体库「剧集 2」设的下载器「qB-2」已停用，用设置里的默认下载器",
    );

    const gone = bringOut(fresh(), libraryDefaults(TV, [dl(2, "qB-2")]));
    expect(gone.values.downloader_id).toBe(0);
    const note = sourceNote(gone, "downloader_id");
    expect(note).toBe("媒体库「剧集」设的下载器已删除，用设置里的默认下载器");
    expect(note, "库里只存编号，删了没有名字可查").not.toContain("qB");

    const unknown = bringOut(fresh(), libraryDefaults(TV, null));
    expect(unknown.values.downloader_id).toBe(0);
    expect(sourceNote(unknown, "downloader_id")).toBe(
      "读不到下载器列表，没有带出媒体库「剧集」设的下载器",
    );
    expect(libraryDefaults(TV, DLS).downloader).toBe("ok");
    expect(libraryDefaults(TV2, DLS)).toMatchObject({
      downloader: "disabled",
      downloaderName: "qB-2",
    });
  });

  it("3. 用户动过的项（输入、清空、选了别的）以后换库也不动；「换成媒体库的」恢复成库的值", () => {
    let f = bringOut(fresh(), libraryDefaults(TV, DLS));
    f = markUser({ ...f, values: { ...f.values, category: "mine" } }, "category");
    f = markUser({ ...f, values: { ...f.values, tags: "" } }, "tags");
    expect(sourceNote(f, "category")).toBe("");

    const d = libraryDefaults(ANIME, DLS);
    f = bringOut(f, d);
    expect(f.values.category, "自己填的不换").toBe("mine");
    expect(f.values.tags, "清空也算自己填的").toBe("");
    expect(f.values.save_path).toBe("/downloads/anime");
    expect(differs(f.values, d)).toEqual(["category"]);

    f = resetField(f, "category", d);
    expect(f.values.category).toBe("anime");
    expect(sourceNote(f, "category")).toBe("来自媒体库「动漫」");
    expect(differs(f.values, d)).toEqual([]);
    expect(resetField(f, "category", null), "没有生效的库时不动").toEqual(f);
  });

  it("4. 换库：只换来自库的项；新库没有某一项时清空；换成没有能用的库时把来自库的项清空", () => {
    let f = bringOut(fresh(), libraryDefaults(TV, DLS));
    f = markUser(f, "save_path");
    f = bringOut(f, libraryDefaults(ANIME, DLS));
    expect(f.values).toEqual({
      downloader_id: 0,
      save_path: "/downloads/tv",
      category: "anime",
      tags: "",
    });
    f = bringOut(f, null);
    expect(f.values).toEqual({
      downloader_id: 0,
      save_path: "/downloads/tv",
      category: "",
      tags: "",
    });
    expect(sourceNote(f, "category")).toBe("");
    expect(f.sources.save_path).toEqual({ kind: "user" });
  });

  it("5. 修改已有订阅：一律当作自己填的，不自动带出；换了库只在点「套用」时换", () => {
    const now = { downloader_id: 1, save_path: "/data/old", category: "old", tags: "x" };
    let f: DefaultsForm = { values: now, sources: userSources() };
    const d = libraryDefaults(TV2, DLS);
    expect(bringOut(f, d).values, "修改时不自动带出").toEqual(now);
    expect(differs(f.values, d), "停用的下载器不算要套用的").toEqual([
      "save_path",
      "category",
      "tags",
    ]);
    f = applyAll(f, d);
    expect(f.values).toEqual({
      downloader_id: 1,
      save_path: "/downloads/tv2",
      category: "tv2",
      tags: "",
    });
    expect(sourceNote(f, "save_path")).toBe("来自媒体库「剧集 2」");
    expect(f.sources.downloader_id, "库的下载器用不了时保留订阅原来的").toEqual({ kind: "user" });
    expect(differs(f.values, d)).toEqual([]);
    expect(applyAll(f, libraryDefaults(TV, DLS)).values.downloader_id).toBe(1);
  });

  it("6. 未保存的基准在带出完成以后再取：带出本身不算改动", () => {
    const f = bringOut(fresh(), libraryDefaults(TV, DLS));
    const base = { ...f.values };
    expect(sameValues(base, f.values)).toBe(true);
    expect(sameValues(base, { ...f.values, tags: "pt-tools,追剧" })).toBe(false);
    expect(sameValues(blankValues(), f.values), "和带出以前比是变了的").toBe(false);
  });

  it("自动整理设了范围时，按库的设置推送的种子不在范围里的地方都列出来", () => {
    const scope = {
      downloaders: [1],
      categories: ["TV", "anime"],
      tags: ["pt-tools"],
      save_paths: ["/downloads"],
    };
    const v = { downloader_id: 1, save_path: "/downloads/tv", category: "tv", tags: "a, PT-Tools" };
    expect(scopeMisses(scope, v), "分类与标签不分大小写").toEqual([]);
    expect(
      scopeMisses(scope, {
        downloader_id: 2,
        save_path: "/data/tv",
        category: "movies",
        tags: "x",
      }),
    ).toEqual([
      "下载器不在范围里",
      "分类「movies」不在范围里（TV、anime）",
      "标签不在范围里（pt-tools）",
      "保存目录不在范围里（/downloads）",
    ]);
    expect(scopeMisses(scope, { ...v, category: "" })).toEqual([
      "没设分类，范围限了分类（TV、anime）",
    ]);
    expect(scopeMisses(scope, { ...v, tags: "" })).toEqual(["没设标签，范围限了标签（pt-tools）"]);
    expect(
      scopeMisses(scope, { ...v, downloader_id: 0, save_path: "" }),
      "下载器、保存目录空着时推送用下载器的默认，这里不知道是哪个",
    ).toEqual([]);
    expect(scopeMisses(scope, { ...v, save_path: "/downloadsX/tv" }), "按整段目录比").toEqual([
      "保存目录不在范围里（/downloads）",
    ]);
    expect(
      scopeMisses(
        { ...scope, save_paths: ["D:\\Downloads"] },
        { ...v, save_path: "d:/downloads/TV" },
      ),
      "Windows 路径不分大小写、认两种斜杠",
    ).toEqual([]);
    expect(scopeMisses({ ...scope, save_paths: ["/"] }, v), "范围是根目录").toEqual([]);
    expect(scopeMisses(scope, blankValues()), "库没设下载设置时不提醒").toEqual([]);
    expect(scopeMisses(null, v)).toEqual([]);
    expect(
      scopeMisses({ downloaders: [], categories: [], tags: [], save_paths: [] }, v),
      "范围都不限",
    ).toEqual([]);
  });
});
