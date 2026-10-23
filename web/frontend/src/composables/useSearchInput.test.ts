// @vitest-environment happy-dom
/*
 * 钉子：工具栏搜索框停下一会儿自动查；回车 / 清空立刻查；同一个词不重复查。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createApp, defineComponent, nextTick, reactive } from "vue";

import { useSearchInput } from "./useSearchInput";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

function setup() {
  const filters = reactive({ q: "" });
  const apply = vi.fn();
  let api!: ReturnType<typeof useSearchInput>;
  const app = createApp(
    defineComponent({
      setup() {
        api = useSearchInput(() => filters.q, apply, 400);
        return () => null;
      },
    }),
  );
  app.mount(document.createElement("div"));
  return { filters, apply, api: () => api, app };
}

describe("useSearchInput", () => {
  it("输入停下 400ms 才查一次，中途的每个字不各查一次", async () => {
    const { filters, apply, app } = setup();
    filters.q = "h";
    await nextTick();
    filters.q = "hd";
    await nextTick();
    vi.advanceTimersByTime(399);
    expect(apply).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(apply).toHaveBeenCalledTimes(1);
    app.unmount();
  });

  it("回车立刻查，之后不再补一次；清空同理", async () => {
    const { filters, apply, api, app } = setup();
    filters.q = "hdsky";
    await nextTick();
    api().now();
    expect(apply).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(1000);
    expect(apply).toHaveBeenCalledTimes(1);

    // el-input 的清空：先把值改成空串，再发 clear
    filters.q = "";
    api().now();
    await nextTick();
    vi.advanceTimersByTime(1000);
    expect(apply).toHaveBeenCalledTimes(2);
    app.unmount();
  });

  it("卸载以后排着的那次不再执行", async () => {
    const { filters, apply, app } = setup();
    filters.q = "x";
    await nextTick();
    app.unmount();
    vi.advanceTimersByTime(1000);
    expect(apply).not.toHaveBeenCalled();
  });
});
