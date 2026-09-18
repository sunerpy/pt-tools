import { describe, expect, it } from "vitest";
import { ref } from "vue";
import { ApiError } from "../api";
import { isPermError, resolveDataState, useDataState } from "./useDataState";

describe("resolveDataState", () => {
  it("loading 压过一切", () => {
    expect(
      resolveDataState({ loading: true, error: new Error("x"), count: 0, filtered: true }),
    ).toBe("loading");
  });

  it("401/403 走 perm，而不是 error", () => {
    expect(resolveDataState({ loading: false, error: new ApiError(401, "no"), count: 0 })).toBe(
      "perm",
    );
    expect(resolveDataState({ loading: false, error: new ApiError(403, "no"), count: 0 })).toBe(
      "perm",
    );
  });

  it("其他失败走 error —— 这是以前被画成 empty 的那一种", () => {
    expect(resolveDataState({ loading: false, error: new ApiError(500, "boom"), count: 0 })).toBe(
      "error",
    );
    expect(resolveDataState({ loading: false, error: new Error("连接超时"), count: 0 })).toBe(
      "error",
    );
  });

  it("成功且 0 行：没筛选是 empty，筛过是 zero", () => {
    expect(resolveDataState({ loading: false, count: 0, filtered: false })).toBe("empty");
    expect(resolveDataState({ loading: false, count: 0, filtered: true })).toBe("zero");
  });

  it("0 行但有数据源失败时是 partial", () => {
    expect(resolveDataState({ loading: false, count: 0, failed: 2 })).toBe("partial");
    // 筛选态也一样：一行都没拿到，先说清有源失败
    expect(resolveDataState({ loading: false, count: 0, filtered: true, failed: 2 })).toBe(
      "partial",
    );
  });
});

describe("isPermError", () => {
  it("只认 401/403", () => {
    expect(isPermError(new ApiError(401, ""))).toBe(true);
    expect(isPermError(new ApiError(403, ""))).toBe(true);
    expect(isPermError(new ApiError(500, ""))).toBe(false);
    expect(isPermError(new Error("403"))).toBe(false);
    expect(isPermError(null)).toBe(false);
  });
});

describe("useDataState", () => {
  it("run 成功时清掉上一次的错误", async () => {
    const s = useDataState();
    await s.run(() => Promise.reject(new Error("first")));
    expect(s.state.value).toBe("error");

    const v = await s.run(() => Promise.resolve(42));
    expect(v).toBe(42);
    expect(s.error.value).toBeNull();
  });

  it("run 失败时返回 null 并把错误留在页面上，不抛出", async () => {
    const s = useDataState();
    const v = await s.run(() => Promise.reject(new ApiError(502, "网关错误")));
    expect(v).toBeNull();
    expect(s.state.value).toBe("error");
    expect(s.errorText.value).toBe("网关错误");
  });

  it("perm 的文案不提「重试」", async () => {
    const s = useDataState();
    await s.run(() => Promise.reject(new ApiError(403, "forbidden")));
    expect(s.state.value).toBe("perm");
    expect(s.errorText.value).toBe("需要登录或管理员权限");
  });

  it("loading 期间 state 是 loading", () => {
    const s = useDataState();
    let resolve: (v: number) => void = () => {};
    const p = s.run(() => new Promise<number>((r) => (resolve = r)));
    expect(s.state.value).toBe("loading");
    resolve(1);
    return p;
  });

  it("有数据 + 有失败才挂 partial 提示；一行都没有时不挂（那时 partial 是主状态）", () => {
    let failed = 0;
    const s = useDataState({ failed: () => failed });
    expect(s.hasPartialBanner(10)).toBe(false);
    failed = 2;
    expect(s.hasPartialBanner(10)).toBe(true);
    expect(s.hasPartialBanner(0)).toBe(false);
  });

  /*
   * 恢复语义。带静默自动刷新的页面（下载器 Web UI 每 5 秒一拍）不走 run()
   * —— run 会亮 loading 遮罩，5 秒闪一次没法用 —— 所以必须自己调 clearError，
   * 否则服务恢复后若恰好返回 0 行，页面会一直停在「加载失败」上，
   * 把一个已经恢复的真实空态显示成故障。这两个用例锁住那条语义。
   */
  it("clearError 之后 0 行回到 empty，而不是继续显示 error", async () => {
    const s = useDataState();
    await s.run(() => Promise.reject(new Error("数据库连接失败")));
    expect(s.state.value).toBe("error");

    // 静默刷新成功：只清错误，不碰 loading
    s.clearError();
    expect(s.state.value).toBe("empty");
    expect(s.errorText.value).toBe("");
  });

  it("clearError 之后仍有数据源失败时应落到 partial，而不是 error", async () => {
    const failed = ref(0);
    const s = useDataState({ failed: () => failed.value });
    await s.run(() => Promise.reject(new Error("整体失败")));
    expect(s.state.value).toBe("error");

    // 静默刷新成功了，但某台下载器仍然没回数据
    failed.value = 1;
    s.clearError();
    expect(s.state.value).toBe("partial");
  });

  it("filtered 透传给 0 行时的 empty/zero 判定（getter 必须读响应式来源）", () => {
    const filtered = ref(false);
    const s = useDataState({ filtered: () => filtered.value });
    expect(s.state.value).toBe("empty");
    filtered.value = true;
    expect(s.state.value).toBe("zero");
  });
});
