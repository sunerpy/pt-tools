/*
 * 「保存搜索」迁移的钉子。
 *
 * 两条都来自真实事故：
 * ① 迁移曾经写在 `ref(loadSaved())` 的右值里，而它内部又去赋值那个还在 TDZ 的 ref ——
 *    本地存着 v1 数据的用户一进搜索页就 ReferenceError、整页白屏。全新浏览器状态下
 *    这条路径根本不执行，所以「21 条路由全绿」完全没覆盖到它。
 * ② 写 v2 失败时若照样删 v1，用户的保存项两头都没了。
 */
import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  loadSavedSearches,
  SAVED_KEY,
  SAVED_KEY_V1,
  type SavedSearch,
  writeSavedSearches,
} from "./savedSearch";

/** 可控的假存储：能造「写入抛异常」这种真实情形（配额满 / 隐私模式） */
function makeStore(seed: Record<string, string> = {}, failWrite = false) {
  const data = new Map(Object.entries(seed));
  return {
    data,
    getItem: vi.fn((k: string) => data.get(k) ?? null),
    setItem: vi.fn((k: string, v: string) => {
      if (failWrite) throw new DOMException("quota", "QuotaExceededError");
      data.set(k, v);
    }),
    removeItem: vi.fn((k: string) => {
      data.delete(k);
    }),
  };
}

const V1_ENTRY = {
  name: "Dune",
  keyword: "Dune",
  sites: ["M-Team"],
  sortBy: "seeders",
  orderDesc: true,
  // v1 存的是站点原始分类名
  category: "电影/HD",
  freeOnly: true,
};

describe("loadSavedSearches", () => {
  let store: ReturnType<typeof makeStore>;

  beforeEach(() => {
    store = makeStore();
  });

  it("两个键都空时返回空数组", () => {
    expect(loadSavedSearches(store)).toEqual([]);
  });

  it("有 v2 就直接用 v2，不去碰 v1", () => {
    const v2: SavedSearch[] = [
      {
        name: "x",
        keyword: "x",
        sites: [],
        sortBy: "seeders",
        orderDesc: false,
        category: "tv",
        freeOnly: false,
      },
    ];
    store = makeStore({ [SAVED_KEY]: JSON.stringify(v2), [SAVED_KEY_V1]: "[]" });
    expect(loadSavedSearches(store)).toEqual(v2);
    expect(store.setItem).not.toHaveBeenCalled();
    expect(store.removeItem).not.toHaveBeenCalled();
  });

  it("只有 v1 时把原始分类名归成桶 ID，写进 v2 并删掉 v1", () => {
    store = makeStore({ [SAVED_KEY_V1]: JSON.stringify([V1_ENTRY]) });
    const got = loadSavedSearches(store);

    expect(got).toHaveLength(1);
    expect(got[0].category).toBe("movie"); // 「电影/HD」→ movie
    expect(got[0].keyword).toBe("Dune"); // 其余字段原样保留
    expect(got[0].freeOnly).toBe(true);

    expect(JSON.parse(store.data.get(SAVED_KEY) as string)[0].category).toBe("movie");
    expect(store.data.has(SAVED_KEY_V1)).toBe(false);
  });

  it("v1 里归不进任何桶的分类迁成空串，而不是留着原文", () => {
    store = makeStore({
      [SAVED_KEY_V1]: JSON.stringify([{ ...V1_ENTRY, category: "纪录片" }]),
    });
    expect(loadSavedSearches(store)[0].category).toBe("");
  });

  it("写 v2 失败时**不能**删 v1 —— 否则两头都没了", () => {
    store = makeStore({ [SAVED_KEY_V1]: JSON.stringify([V1_ENTRY]) }, true);
    const got = loadSavedSearches(store);

    // 本次会话里照样能用迁移后的值
    expect(got[0].category).toBe("movie");
    // 但 v1 必须还在，下次再迁
    expect(store.data.has(SAVED_KEY_V1)).toBe(true);
    expect(store.removeItem).not.toHaveBeenCalled();
  });

  it("v2 是空数组时就用空数组，不回头去读残留的 v1", () => {
    /*
     * 用户把保存项全删干净 → v2 落成 []。若把「空」当成「没有 v2」，
     * 就会去读残留的 v1，删掉的东西自己回来了。
     */
    store = makeStore({
      [SAVED_KEY]: "[]",
      [SAVED_KEY_V1]: JSON.stringify([V1_ENTRY]),
    });
    expect(loadSavedSearches(store)).toEqual([]);
    expect(store.setItem).not.toHaveBeenCalled();
    expect(store.data.has(SAVED_KEY_V1)).toBe(true); // 也不该顺手删掉 v1
  });

  it("坏 JSON 当成没有这个键，不抛；有 v1 时照旧迁移", () => {
    store = makeStore({ [SAVED_KEY]: "{oops", [SAVED_KEY_V1]: JSON.stringify([V1_ENTRY]) });
    expect(loadSavedSearches(store)[0].category).toBe("movie");
  });

  it("没有存储（隐私模式）时返回空，不抛", () => {
    expect(loadSavedSearches(null)).toEqual([]);
  });
});

describe("writeSavedSearches", () => {
  it("写成功返回 true", () => {
    const store = makeStore();
    expect(writeSavedSearches([], store)).toBe(true);
  });

  it("写失败返回 false 而不是抛出来", () => {
    const store = makeStore({}, true);
    expect(writeSavedSearches([], store)).toBe(false);
  });

  it("没有存储时返回 false", () => {
    expect(writeSavedSearches([], null)).toBe(false);
  });
});
