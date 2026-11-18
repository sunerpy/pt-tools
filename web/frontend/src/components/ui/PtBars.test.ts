// @vitest-environment happy-dom
/*
 * 钉子：每日增量用 baseline="zero" 画：0 只画 2px 的底，柱高按「值 / 最大值」；
 * 默认的 range 模式照旧把最低点抬到 34%（指标图的设计）。
 * pushKey 变了时新的一拍从右边推进来：轨道先右移一格，两帧以后带过渡滑回；首次给值、没变、要求减少动态效果时不播。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createApp, h, nextTick, reactive } from "vue";

import PtBars from "./PtBars.vue";

let app: ReturnType<typeof createApp> | null = null;
afterEach(() => {
  app?.unmount();
  app = null;
  document.body.innerHTML = "";
  vi.unstubAllGlobals();
});

interface BarsProps {
  values: number[];
  baseline?: "range" | "zero";
  height?: number;
  count?: number;
}

function heights(props: BarsProps): number[] {
  document.body.innerHTML = '<div id="root"></div>';
  app = createApp({ render: () => h(PtBars, props) });
  app.mount("#root");
  return [...document.querySelectorAll<HTMLElement>(".pt-bars i")].map((i) =>
    Number.parseInt(i.style.height, 10),
  );
}

describe("PtBars", () => {
  it("zero：0 画成 2px 的底，其余按值占最大值的比例", () => {
    expect(heights({ values: [0, 5, 0, 10], baseline: "zero", height: 20, count: 4 })).toEqual([
      2, 10, 2, 20,
    ]);
  });

  it("zero：全是 0 时每根都是 2px，不会画成持平的走势", () => {
    expect(heights({ values: [0, 0, 0], baseline: "zero", height: 20, count: 3 })).toEqual([
      2, 2, 2,
    ]);
  });

  it("zero：一点点增量只比底高一点（3px），不被抬成最大值的五分之一", () => {
    expect(heights({ values: [1, 100], baseline: "zero", height: 20, count: 2 })).toEqual([3, 20]);
  });

  it("zero：负值按 0 画，单点画满高", () => {
    expect(heights({ values: [-5, 10], baseline: "zero", height: 20, count: 2 })).toEqual([2, 20]);
    expect(heights({ values: [5], baseline: "zero", height: 20, count: 1 })).toEqual([20]);
  });

  it("range（默认）：最低点仍抬到 34%", () => {
    expect(heights({ values: [0, 10], height: 20, count: 2 })).toEqual([7, 20]);
  });
});

describe("PtBars 推进动画", () => {
  /** 手动推进的 requestAnimationFrame：frame() 跑一帧 */
  function stubFrames() {
    let queue: FrameRequestCallback[] = [];
    vi.stubGlobal(
      "requestAnimationFrame",
      vi.fn((cb: FrameRequestCallback) => {
        queue.push(cb);
        return queue.length;
      }),
    );
    const cancel = vi.fn(() => {
      queue = [];
    });
    vi.stubGlobal("cancelAnimationFrame", cancel);
    return {
      cancel,
      frame() {
        const run = queue;
        queue = [];
        for (const cb of run) cb(0);
      },
    };
  }

  function reduced(on: boolean) {
    vi.stubGlobal(
      "matchMedia",
      vi.fn((query: string) => ({
        matches: on && query.includes("prefers-reduced-motion"),
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    );
  }

  function mount(props: { values: number[]; pushKey?: string; barWidth?: number; gap?: number }) {
    app?.unmount();
    document.body.innerHTML = '<div id="root"></div>';
    const state = reactive({ ...props });
    app = createApp({ render: () => h(PtBars, state) });
    app.mount("#root");
    const track = () => document.querySelector<HTMLElement>(".pt-bars__track")!;
    return { state, track };
  }

  it("pushKey 变了：轨道先右移一格（柱宽 + 间距），两帧以后带过渡滑回，过渡完了收起", async () => {
    reduced(false);
    const f = stubFrames();
    const { state, track } = mount({ values: [1, 2, 3], pushKey: "a", barWidth: 5, gap: 3 });
    expect(track().style.transform, "首次给值不播").toBe("");
    state.values = [2, 3, 4];
    state.pushKey = "b";
    await nextTick();
    expect(track().style.transform).toBe("translateX(8px)");
    expect(track().style.transition).toBe("none");
    f.frame();
    await nextTick();
    expect(track().style.transform, "一帧以后还没动").toBe("translateX(8px)");
    f.frame();
    await nextTick();
    expect(track().style.transform).toBe("");
    expect(track().style.transition).toContain("transform");
    track().dispatchEvent(new Event("transitionend"));
    await nextTick();
    expect(track().style.transition).toBe("none");
  });

  it("没变、没给 pushKey、要求减少动态效果时不播；卸载时取消还没跑的那一帧", async () => {
    reduced(false);
    const f = stubFrames();
    const { state, track } = mount({ values: [1, 2], pushKey: "a" });
    state.values = [1, 3];
    await nextTick();
    expect(track().style.transform, "pushKey 没变").toBe("");
    state.pushKey = "b";
    await nextTick();
    expect(track().style.transform).not.toBe("");
    app!.unmount();
    app = null;
    expect(f.cancel).toHaveBeenCalled();

    reduced(true);
    const m = mount({ values: [1, 2], pushKey: "a" });
    m.state.pushKey = "b";
    await nextTick();
    expect(m.track().style.transform, "减少动态效果").toBe("");

    reduced(false);
    const n = mount({ values: [1, 2] });
    n.state.values = [2, 3];
    await nextTick();
    expect(n.track().style.transform, "没给 pushKey").toBe("");
  });
});
