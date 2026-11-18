// @vitest-environment happy-dom
/*
 * 刷流收益图：读数默认是当前这一段；鼠标移到柱子上、或聚焦后按方向键换成那一段，离开回到当前这一段；
 * 柱子下面的标签放不下时隔几根写一个，当前这一段总是写。
 */
import { afterEach, describe, expect, it } from "vitest";
import { createApp, h, nextTick, reactive } from "vue";

import BrushTrendChart, { type TrendPoint } from "./BrushTrendChart.vue";

function points(n: number): TrendPoint[] {
  return Array.from({ length: n }, (_, i) => ({
    short: `S${i}`,
    long: `L${i}`,
    value: i === 2 ? 0 : (i + 1) * 10,
    valueText: `上传 ${(i + 1) * 10}`,
    detail: `第 ${i} 段`,
  }));
}

let unmount: (() => void) | null = null;
afterEach(() => {
  unmount?.();
  unmount = null;
  document.body.innerHTML = "";
});

function mount(state: { points: TrendPoint[] }) {
  const el = document.createElement("div");
  document.body.appendChild(el);
  const app = createApp({
    render: () => h(BrushTrendChart, { points: state.points, label: "上传量" }),
  });
  app.mount(el);
  unmount = () => app.unmount();
  return el;
}

const readout = () => document.querySelector("[data-testid=brush-trend-readout]")?.textContent;
const plot = () => document.querySelector<HTMLElement>(".btc__plot")!;
const key = (k: string) => plot().dispatchEvent(new KeyboardEvent("keydown", { key: k }));

describe("刷流收益图", () => {
  it("读数默认是最后一段；方向键、Home/End 换段，鼠标离开回到最后一段", async () => {
    mount({ points: points(5) });
    expect(readout()).toContain("上传 50");
    expect(readout()).toContain("L4");
    key("ArrowLeft");
    await nextTick();
    expect(readout()).toContain("L3");
    key("Home");
    await nextTick();
    expect(readout()).toContain("L0");
    key("ArrowLeft");
    await nextTick();
    expect(readout()).toContain("L0");
    key("End");
    await nextTick();
    expect(readout()).toContain("L4");

    document
      .querySelectorAll<HTMLElement>(".btc__col")[1]!
      .dispatchEvent(new PointerEvent("pointerenter"));
    await nextTick();
    expect(readout()).toContain("L1");
    expect(document.querySelectorAll(".btc__col")[1]!.classList.contains("is-on")).toBe(true);
    plot().dispatchEvent(new PointerEvent("pointerleave"));
    await nextTick();
    expect(readout()).toContain("L4");
  });

  it("0 只画 2px 的底，最高的一段占满高度", () => {
    mount({ points: points(5) });
    const bars = [...document.querySelectorAll<HTMLElement>(".btc__bar")];
    expect(bars[2]!.style.height).toBe("2px");
    expect(bars[4]!.style.height).toBe("96px");
  });

  it("30 段时标签隔几根写一个，最后一段总是写，离它太近的不写", () => {
    mount({ points: points(30) });
    const ticks = [...document.querySelectorAll(".btc__tick")].map((t) =>
      (t.textContent ?? "").trim(),
    );
    const shown = ticks.filter(Boolean);
    expect(shown.length).toBeLessThanOrEqual(9);
    expect(ticks[29]).toBe("S29");
    expect(ticks[0]).toBe("S0");
    expect(ticks[28]).toBe("");
  });

  it("换了段数（换周期）读数回到新的最后一段", async () => {
    const state = reactive({ points: points(5) });
    mount(state);
    key("Home");
    await nextTick();
    expect(readout()).toContain("L0");
    state.points = points(12);
    await nextTick();
    expect(readout()).toContain("L11");
  });
});
