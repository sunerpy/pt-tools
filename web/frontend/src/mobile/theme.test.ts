import { describe, expect, it } from "vitest";
import { appThemeOf } from "./theme";

describe("appThemeOf", () => {
  it("用 App 传来的主题名", () => {
    expect(appThemeOf("warm", false)).toBe("warm");
    expect(appThemeOf("graphite", false)).toBe("graphite");
    expect(appThemeOf("light", true)).toBe("light");
  });

  it("不认识或没传时按 dark 退回明亮或暗黑", () => {
    expect(appThemeOf(null, true)).toBe("dark");
    expect(appThemeOf(null, false)).toBe("light");
    expect(appThemeOf("glass", false)).toBe("light");
  });
});
