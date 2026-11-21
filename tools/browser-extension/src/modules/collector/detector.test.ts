import { describe, expect, it } from "vitest";

import { detectPageType } from "./detector";

/** 地址规则先于页面结构判断：这里的地址都由 PAGE_PATTERNS 认出，用不到 document */
const noDocument = {} as Document;

describe("detectPageType", () => {
  it.each([
    ["https://pt.example/rules.php", "rules"],
    ["https://pt.example/faq.php#id5", "faq"],
    ["https://pt.example/mybonus.php", "bonus"],
    ["https://pt.example/torrents.php?spstate=2", "search_free"],
  ])("%s → %s", (url, expected) => {
    expect(detectPageType(url, noDocument)).toBe(expected);
  });
});
