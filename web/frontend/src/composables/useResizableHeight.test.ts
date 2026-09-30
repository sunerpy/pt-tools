import { describe, expect, it } from "vitest";

import { clampHeight, heightForKey, parseStoredHeight } from "./useResizableHeight";

const B = { min: 200, max: 900 };

describe("clampHeight", () => {
  it("夹在上下限之间并取整", () => {
    expect(clampHeight(150, B)).toBe(200);
    expect(clampHeight(1200, B)).toBe(900);
    expect(clampHeight(456.6, B)).toBe(457);
  });

  it("上限比下限还小时以下限为准（视口很矮的时候）", () => {
    expect(clampHeight(500, { min: 300, max: 100 })).toBe(300);
  });
});

describe("parseStoredHeight", () => {
  it("没存、存了 auto 或乱写都当自适应", () => {
    expect(parseStoredHeight(null, B)).toBeNull();
    expect(parseStoredHeight("", B)).toBeNull();
    expect(parseStoredHeight("auto", B)).toBeNull();
    expect(parseStoredHeight("abc", B)).toBeNull();
    expect(parseStoredHeight("-5", B)).toBeNull();
  });

  it("存的数字按当前上下限夹一次（换了更矮的屏也不会溢出）", () => {
    expect(parseStoredHeight("640", B)).toBe(640);
    expect(parseStoredHeight("5000", B)).toBe(900);
  });
});

describe("heightForKey", () => {
  it("↑ 变矮、↓ 变高，Shift 四倍步长", () => {
    expect(heightForKey("ArrowUp", false, 500, B)).toBe(476);
    expect(heightForKey("ArrowDown", false, 500, B)).toBe(524);
    expect(heightForKey("ArrowDown", true, 500, B)).toBe(596);
  });

  it("Home / End 到最矮 / 最高，并且不越界", () => {
    expect(heightForKey("Home", false, 500, B)).toBe(200);
    expect(heightForKey("End", false, 500, B)).toBe(900);
    expect(heightForKey("ArrowUp", true, 210, B)).toBe(200);
  });

  it("别的键不归它管", () => {
    expect(heightForKey("Tab", false, 500, B)).toBeUndefined();
    expect(heightForKey("a", false, 500, B)).toBeUndefined();
  });
});
