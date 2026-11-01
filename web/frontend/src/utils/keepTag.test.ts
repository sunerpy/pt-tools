import { describe, expect, it } from "vitest";
import { hasKeepTag, KEEP_TAG } from "./keepTag";

describe("hasKeepTag", () => {
  it("认逗号分隔里的保留标签，大小写与空格不敏感", () => {
    expect(hasKeepTag(KEEP_TAG)).toBe(true);
    expect(hasKeepTag("hdsky, PT-Tools-Keep ,x")).toBe(true);
  });

  it("相近的名字与空值不算", () => {
    expect(hasKeepTag("pt-tools-keeper")).toBe(false);
    expect(hasKeepTag("")).toBe(false);
    expect(hasKeepTag(undefined)).toBe(false);
    expect(hasKeepTag(null)).toBe(false);
  });
});
