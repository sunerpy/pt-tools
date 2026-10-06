import { describe, expect, it } from "vitest";

import { createTabGenerations } from "./tab-generation";

describe("createTabGenerations", () => {
  it("同一标签页后开始的解析让先开始的作废，别的标签页不受影响", () => {
    const gens = createTabGenerations();
    const first = gens.begin(1);
    const other = gens.begin(2);
    const second = gens.begin(1);

    expect(first()).toBe(false);
    expect(second()).toBe(true);
    expect(other()).toBe(true);
  });

  it("标签页关闭后，还在路上的解析也作废", () => {
    const gens = createTabGenerations();
    const pending = gens.begin(5);
    gens.forget(5);

    expect(pending()).toBe(false);
  });
});
