import { computed, type ComputedRef, type Ref, ref } from "vue";
import { ApiError } from "../api";

/**
 * 列表/表格的六种数据状态（设计文档 §5）。
 *
 * 之所以要六种而不是「有数据 / 没数据」：它们要求用户做的下一步动作完全不同。
 * 最容易被糊在一起、也最要命的是 error 和 empty —— 请求失败却画成「还没有数据」，
 * 用户会以为库是空的，去添加一条重复数据，或者干脆认为功能坏了。
 *
 * | key     | 含义                     | 用户该做什么       |
 * |---------|--------------------------|--------------------|
 * | loading | 请求在飞                 | 等                 |
 * | empty   | 请求成功，库里确实没有   | 去添加第一条       |
 * | zero    | 请求成功，筛选后没有命中 | 放宽筛选条件       |
 * | error   | 请求失败                 | 重试 / 查网络      |
 * | partial | 多数据源，部分失败       | 看清哪些没拿到     |
 * | perm    | 没有权限（401/403）      | 去要权限           |
 */
export type DataStateKey = "loading" | "empty" | "zero" | "error" | "partial" | "perm";

export interface DataStateInput {
  /** 请求是否在飞 */
  loading: boolean;
  /** 最近一次失败；null/undefined 表示成功 */
  error?: unknown;
  /** 当前拿到的行数 */
  count: number;
  /**
   * 是否处于筛选/搜索态。0 行时用它区分 empty 与 zero：
   * 没筛选就是「库里没有」，筛了就是「没有命中」。
   */
  filtered?: boolean;
  /** 多数据源里失败的个数（比如 14 个站点有 2 个没同步上） */
  failed?: number;
}

/** 401/403 要单独画成「无权访问」，画成加载失败会让用户一直重试 */
export function isPermError(e: unknown): boolean {
  return e instanceof ApiError && (e.status === 401 || e.status === 403);
}

/**
 * 把请求现状解析成要展示的那一种状态。
 *
 * 优先级：loading > perm > error > 有行 > partial > zero/empty。
 *
 * 「有行」排在 partial 之前是有意的：部分失败且还有数据可看时，不该用一整块状态图
 * 顶掉表格 —— 那等于把拿到的数据也藏了。这种情形用 hasPartialBanner 在表格上方
 * 挂一条提示，表格照常渲染。只有一行都没拿到时，partial 才成为主状态。
 */
export function resolveDataState(input: DataStateInput): DataStateKey {
  if (input.loading) return "loading";
  if (isPermError(input.error)) return "perm";
  if (input.error) return "error";
  if (input.count > 0) return "empty"; // 调用方不会用到：有行就不画状态块
  if ((input.failed ?? 0) > 0) return "partial";
  return input.filtered ? "zero" : "empty";
}

/**
 * 两个 getter 都在 computed 里被调用，因此**必须读响应式来源**（ref / reactive / computed）。
 * 传一个闭在普通变量上的函数不会触发重算，状态会卡在第一次求值的结果上。
 *   对：`() => keyword.value !== ""`
 *   错：`() => filtered`（filtered 是个普通 let）
 */
export interface UseDataStateOptions {
  /** 0 行时是否算「筛选后没命中」 */
  filtered?: () => boolean;
  /** 多数据源里失败的个数 */
  failed?: () => number;
}

export interface UseDataStateReturn {
  loading: Ref<boolean>;
  error: Ref<unknown>;
  /** 当前该展示哪一种状态。有行时这个值没有意义，模板里先判断行数 */
  state: ComputedRef<DataStateKey>;
  /** 是否该在内容上方挂「部分失败」提示（有数据 + 有失败） */
  hasPartialBanner: (count: number) => boolean;
  /** 供 PtDataState 的 sub 用的错误文案 */
  errorText: ComputedRef<string>;
  /**
   * 跑一次加载：自动管 loading、清上次的错误、记这次的错误。
   * 失败时返回 null 并把错误留在 error 上，不吞掉 —— 调用方想弹 toast 照旧可以。
   */
  run: <T>(fn: () => Promise<T>) => Promise<T | null>;
  /** 手动清错误，比如用户点了「重试」之前 */
  clearError: () => void;
}

/**
 * 页面侧的状态机。
 *
 * 替代「一个 loading ref + 失败时弹个 toast」的老写法：toast 两秒就没了，
 * 而表格还停在 empty 上，用户看到的是「没有数据」。错误必须留在页面上。
 */
export function useDataState(opts: UseDataStateOptions = {}): UseDataStateReturn {
  const loading = ref(false);
  const error = ref<unknown>(null);

  const state = computed(() =>
    resolveDataState({
      loading: loading.value,
      error: error.value,
      count: 0,
      filtered: opts.filtered?.() ?? false,
      failed: opts.failed?.() ?? 0,
    }),
  );

  const errorText = computed(() => {
    const e = error.value;
    if (!e) return "";
    if (isPermError(e)) return "需要登录或管理员权限";
    const msg = e instanceof Error ? e.message : String(e);
    return msg.trim() || "请检查网络后重试";
  });

  function hasPartialBanner(count: number): boolean {
    return count > 0 && (opts.failed?.() ?? 0) > 0;
  }

  async function run<T>(fn: () => Promise<T>): Promise<T | null> {
    loading.value = true;
    error.value = null;
    try {
      return await fn();
    } catch (e) {
      error.value = e;
      return null;
    } finally {
      loading.value = false;
    }
  }

  return {
    loading,
    error,
    state,
    hasPartialBanner,
    errorText,
    run,
    clearError: () => {
      error.value = null;
    },
  };
}
