// @vitest-environment happy-dom
/*
 * 钉子：每日增量用 baseline="zero" 画：0 只画 2px 的底，柱高按「值 / 最大值」；
 * 默认的 range 模式照旧把最低点抬到 34%（指标图的设计）。
 */
import { afterEach, describe, expect, it } from "vitest";
import { createApp, h } from "vue";

import PtBars from "./PtBars.vue";

let app: ReturnType<typeof createApp> | null = null;
afterEach(() => {
  app?.unmount();
  app = null;
  document.body.innerHTML = "";
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

  it("range（默认）：最低点仍抬到 34%", () => {
    expect(heights({ values: [0, 10], height: 20, count: 2 })).toEqual([7, 20]);
  });
});
