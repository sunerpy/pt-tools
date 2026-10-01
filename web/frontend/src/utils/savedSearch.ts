/*
 * 「保存搜索」的本地存储 —— 画板 15 的 search-head 的 ha-1。
 *
 * 为什么单独一个模块而不是写在 SFC 里：
 *
 * ① 迁移逻辑不能碰组件里的 ref。曾经写成 `const saved = ref(loadSaved())`，而
 *    `loadSaved()` 内部又去赋值 `saved.value` —— 右值阶段那个 const 还在 TDZ，
 *    凡是本地存着 v1 数据的用户一进搜索页就 ReferenceError，整页白屏。
 *    纯函数进纯函数出，就没有这个可能。
 * ② 迁移要能测。带 v1 数据的初始化、v2 写入失败时不能删 v1，这两条只有把逻辑
 *    拿出来才写得成测试。
 *
 * 版本差异：v1 的 `category` 存的是站点原始分类名（「电影/HD」这种），
 * v2 存的是桶 ID（movie / tv / anime / music）。筛选拿桶 ID 比较，所以 v1 的值
 * 必须过一遍 `bucketOf` 才能用 —— 不迁移的话旧条目点开就是零结果，且看不出原因。
 */
import { bucketOf, type CategoryBucket } from "./category";

export const SAVED_KEY = "pt-tools-search-saved-v2";
export const SAVED_KEY_V1 = "pt-tools-search-saved-v1";
export const SAVED_MAX = 12;

export interface SavedSearch {
  name: string;
  keyword: string;
  sites: string[];
  sortBy: string;
  orderDesc: boolean;
  /** v2 起是桶 ID，空串表示不限分类 */
  category: CategoryBucket;
  freeOnly: boolean;
}

/** 存储可用性：隐私模式下 localStorage 会抛，所有出口都得容忍 */
type Store = Pick<Storage, "getItem" | "setItem" | "removeItem">;

function defaultStore(): Store | null {
  try {
    return window.localStorage;
  } catch {
    return null;
  }
}

/**
 * 读一个键。**分得清「没有这个键」和「这个键是空数组」**：
 * 前者返回 null，后者返回 []。
 *
 * 混成一件事会出这样的错：用户把保存项全删了（v2 落成 `[]`），下次进来却被当成
 * 「还没有 v2」，于是去读残留的 v1 —— 删掉的东西自己回来了。
 */
function readList(store: Store, key: string): SavedSearch[] | null {
  try {
    const raw = store.getItem(key);
    if (raw === null) return null;
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return null;
    return parsed.slice(0, SAVED_MAX) as SavedSearch[];
  } catch {
    return null;
  }
}

/** 写 v2。返回是否真的写进去了 —— 调用方要靠它决定能不能删 v1 */
export function writeSavedSearches(list: SavedSearch[], store: Store | null = defaultStore()) {
  if (!store) return false;
  try {
    store.setItem(SAVED_KEY, JSON.stringify(list));
    return true;
  } catch {
    return false;
  }
}

/**
 * 读已保存的搜索，必要时把 v1 迁移成 v2。
 *
 * **只有确认 v2 写成功才删 v1**：写失败（配额满、隐私模式）时把 v1 删掉，
 * 用户的保存项就两头都没了。宁可留着 v1 下次再迁。
 */
export function loadSavedSearches(store: Store | null = defaultStore()): SavedSearch[] {
  if (!store) return [];

  /* v2 存在就用它，哪怕是空数组 —— 空数组是「用户删干净了」这个事实 */
  const v2 = readList(store, SAVED_KEY);
  if (v2 !== null) return v2;

  const legacy = readList(store, SAVED_KEY_V1);
  if (legacy === null || legacy.length === 0) return [];

  const migrated = legacy.map((item) => ({ ...item, category: bucketOf(item.category) }));
  if (writeSavedSearches(migrated, store)) {
    try {
      store.removeItem(SAVED_KEY_V1);
    } catch {
      /* 删不掉无所谓：v2 已经有值，下次不会再读 v1 */
    }
  }
  return migrated;
}
