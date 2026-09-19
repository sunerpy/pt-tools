/*
 * 钉子：状态栏第三段（下载器版本）不许串台。
 *
 * 真实缺陷（评审查出）：版本只记一个字符串，而后端在问不到版本时合法地省掉该字段。
 * 于是「A 报过 v4.6.7，切到问不到版本的 B」显示成「B · 已连接 · v4.6.7」——
 * 画面上那一行在说一件不存在的事。
 */
import { describe, expect, it } from "vitest";

import { type HeldVersion, nextHeldVersion, versionFor } from "./statusBarVersion";

describe("versionFor：只认属于当前这台的版本", () => {
  const held: HeldVersion = { id: 1, version: "v4.6.7" };

  it("同一台就显示", () => {
    expect(versionFor(1, held)).toBe("v4.6.7");
  });

  it("换了台就不显示 —— 这才是那个缺陷的核心", () => {
    expect(versionFor(2, held)).toBe("");
  });

  it("还没问到、或没有默认下载器，都不显示", () => {
    expect(versionFor(1, null)).toBe("");
    expect(versionFor(undefined, held)).toBe("");
  });
});

describe("nextHeldVersion：这一轮该记什么", () => {
  it("带了版本就记下「这一台 + 这个版本」", () => {
    expect(nextHeldVersion(1, "v4.6.7", null)).toEqual({ id: 1, version: "v4.6.7" });
  });

  it("这一轮没带、但还是同一台：保留旧值（后端问不到时会省掉字段，不该闪成空）", () => {
    const held: HeldVersion = { id: 1, version: "v4.6.7" };
    expect(nextHeldVersion(1, undefined, held)).toEqual(held);
  });

  it("这一轮没带、而且换了台：丢掉旧值（它属于别人）", () => {
    const held: HeldVersion = { id: 1, version: "v4.6.7" };
    expect(nextHeldVersion(2, undefined, held)).toBeNull();
  });

  it("A 报过版本 → 切到问不到版本的 B → B 这一格不显示版本", () => {
    let held = nextHeldVersion(1, "v4.6.7", null);
    expect(versionFor(1, held)).toBe("v4.6.7");
    held = nextHeldVersion(2, undefined, held);
    expect(versionFor(2, held)).toBe("");
  });
});
