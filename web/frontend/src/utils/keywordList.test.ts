import { describe, expect, it } from "vitest";

import { MAX_KEYWORDS_BYTES, addKeyword, normalizeKeyword, splitKeywords } from "./keywordList";

/* 钉子：试搜索往标题包含、排除里加一行（追剧方案 3.2.5）：写法与后端 keywords.go 相同，按规整后的写法去重，限 1024 字节。 */
describe("keywordList", () => {
  it("规整与拆分同后端：小写，空格、点、下划线算一样；换行、逗号、中文逗号分隔", () => {
    expect(normalizeKeyword("  Atmos-CHDWEB  ")).toBe("atmos-chdweb");
    expect(normalizeKeyword("3Audio.Atmos__CHDWEB")).toBe("3audio atmos chdweb");
    expect(splitKeywords("CHDWEB\nHHWEB, ADWeb，\n\n")).toEqual(["chdweb", "hhweb", "adweb"]);
  });

  it("加在最后一行；空的直接是这个词；去掉末尾多余的空行与逗号", () => {
    expect(addKeyword("", "CHDWEB")).toEqual({ ok: true, text: "CHDWEB" });
    expect(addKeyword("HHWEB", " CHDWEB ")).toEqual({ ok: true, text: "HHWEB\nCHDWEB" });
    expect(addKeyword("HHWEB,\n\n", "CHDWEB")).toEqual({ ok: true, text: "HHWEB\nCHDWEB" });
  });

  it("规整以后已经有了的不加", () => {
    const r = addKeyword("3Audio Atmos-CHDWEB", "3audio.atmos-chdweb");
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toBe("duplicate");
  });

  it("空的、带逗号的不加", () => {
    expect(addKeyword("A", "  ").ok).toBe(false);
    const r = addKeyword("A", "Show, The");
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toBe("separator");
  });

  it("加了以后超过 1024 字节时不加；正好 1024 字节可以", () => {
    const base = "a".repeat(MAX_KEYWORDS_BYTES - 2);
    expect(addKeyword(base, "b")).toEqual({ ok: true, text: `${base}\nb` });
    const r = addKeyword(base, "中");
    expect(r.ok).toBe(false);
    if (!r.ok) expect(r.reason).toBe("too_long");
  });
});
