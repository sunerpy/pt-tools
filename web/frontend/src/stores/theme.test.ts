/**
 * @vitest-environment happy-dom
 *
 * 主题是这轮重构的招牌功能（4 配色 × 明暗），而它的正确性全落在两处副作用上：
 * 写进 localStorage 的两个键，和挂到 <html> 上的 class / data-theme-style / colorScheme。
 * 这两处一旦漂移，表现是「换主题没反应」或「登录页与主界面两种观感」，编译和 lint
 * 都看不出来，所以在这里钉住。
 *
 * 登录页（web/server.go 的 loginHTML）在 SPA 之外自带一份同样逻辑的引导脚本，
 * 它读的就是这里写的键名与取值，末尾那个用例专门比对这份契约。
 */
import { createPinia, setActivePinia } from "pinia";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { nextTick } from "vue";

import { useThemeStore } from "./theme";

/**
 * store 把 DOM 与存档都放在 watch 里做：首帧靠 immediate 同步跑一次，之后每次改动都
 * 等到微任务才落地。所以断言副作用前必须 await 一次，这也是真实页面里的行为。
 */

/** 造一个可控的 matchMedia：store 只用 matches 与 addEventListener */
function stubMatchMedia(matches: boolean) {
  const listeners = new Set<(e: MediaQueryListEvent) => void>();
  const mql = {
    matches,
    media: "(prefers-color-scheme: dark)",
    addEventListener: (_: string, fn: (e: MediaQueryListEvent) => void) => void listeners.add(fn),
    removeEventListener: (_: string, fn: (e: MediaQueryListEvent) => void) =>
      void listeners.delete(fn),
  };
  window.matchMedia = vi.fn(() => mql) as unknown as typeof window.matchMedia;
  return {
    /** 模拟系统在会话中途切换明暗 */
    emit(next: boolean) {
      mql.matches = next;
      for (const fn of listeners) fn({ matches: next } as MediaQueryListEvent);
    },
  };
}

function root() {
  return document.documentElement;
}

beforeEach(() => {
  localStorage.clear();
  root().className = "";
  root().removeAttribute("data-theme-style");
  root().style.colorScheme = "";
  // 默认给一个「系统偏好深色」的环境，需要别的用例自己再 stub 一次
  stubMatchMedia(true);
  // 每个用例一个全新 pinia，否则 store 是同一个实例，读不到新写的 localStorage
  setActivePinia(createPinia());
});

describe("默认值", () => {
  it("localStorage 为空时落到黑暗 + 驾驶舱，并且首帧就写进 DOM", () => {
    const theme = useThemeStore();

    expect(theme.mode).toBe("dark");
    expect(theme.palette).toBe("cockpit");
    expect(theme.isDark).toBe(true);
    expect(root().classList.contains("dark")).toBe(true);
    expect(root().classList.contains("light")).toBe(false);
    expect(root().getAttribute("data-theme-style")).toBe("cockpit");
    expect(root().style.colorScheme).toBe("dark");
  });

  it("非法取值当作没存过，不是当作错误抛出来", () => {
    localStorage.setItem("theme", "midnight");
    localStorage.setItem("theme-style", "rainbow");

    const theme = useThemeStore();

    expect(theme.mode).toBe("dark");
    expect(theme.palette).toBe("cockpit");
  });
});

describe("旧配色名迁移", () => {
  // 升级后如果读到旧名字又不迁移，界面上一个 token 都不会生效
  const cases: Record<string, string> = {
    default: "cockpit",
    ocean: "cockpit",
    emerald: "cockpit",
    contrast: "atlas",
    graphite: "deck",
  };

  for (const [legacy, want] of Object.entries(cases)) {
    it(`${legacy} → ${want}`, () => {
      localStorage.setItem("theme-style", legacy);

      const theme = useThemeStore();

      expect(theme.palette).toBe(want);
      expect(root().getAttribute("data-theme-style")).toBe(want);
      // 迁移结果要落盘，否则每次启动都要重算一遍
      expect(localStorage.getItem("theme-style")).toBe(want);
    });
  }
});

describe("切换明暗", () => {
  it("setMode('light') 同时改 class、colorScheme 与存档", async () => {
    const theme = useThemeStore();

    theme.setMode("light");
    await nextTick();

    expect(theme.isDark).toBe(false);
    expect(root().classList.contains("light")).toBe(true);
    expect(root().classList.contains("dark")).toBe(false);
    expect(root().style.colorScheme).toBe("light");
    expect(localStorage.getItem("theme")).toBe("light");
  });

  it("toggle 在明暗之间来回，不会停在 auto", () => {
    const theme = useThemeStore();

    theme.toggle();
    expect(theme.mode).toBe("light");

    theme.toggle();
    expect(theme.mode).toBe("dark");
  });

  it("setMode 忽略不认识的值，不把界面打成裸样式", () => {
    const theme = useThemeStore();

    theme.setMode("solarized" as never);

    expect(theme.mode).toBe("dark");
  });
});

describe("跟随系统", () => {
  it("auto 取 prefers-color-scheme 的当前值", () => {
    stubMatchMedia(false);
    localStorage.setItem("theme", "auto");

    const theme = useThemeStore();

    expect(theme.isDark).toBe(false);
    expect(root().classList.contains("light")).toBe(true);
  });

  it("auto 下系统中途切换会跟着走", async () => {
    const media = stubMatchMedia(false);
    localStorage.setItem("theme", "auto");

    const theme = useThemeStore();
    expect(theme.isDark).toBe(false);

    media.emit(true);
    await nextTick();

    expect(theme.isDark).toBe(true);
    expect(root().classList.contains("dark")).toBe(true);
    expect(root().style.colorScheme).toBe("dark");
  });

  it("显式选了明暗就不再理系统偏好", () => {
    const media = stubMatchMedia(false);

    const theme = useThemeStore();
    theme.setMode("dark");

    media.emit(false);

    expect(theme.isDark).toBe(true);
  });
});

describe("切换配色", () => {
  it("四个配色都能落到 data-theme-style 与存档", async () => {
    const theme = useThemeStore();

    for (const option of theme.palettes) {
      theme.setPalette(option.value);
      await nextTick();

      expect(theme.palette).toBe(option.value);
      expect(root().getAttribute("data-theme-style")).toBe(option.value);
      expect(localStorage.getItem("theme-style")).toBe(option.value);
    }
  });

  it("不认识的配色名被忽略", () => {
    const theme = useThemeStore();

    theme.setPalette("rainbow");

    expect(theme.palette).toBe("cockpit");
  });

  it("配色与明暗互不影响：换配色不会把 light 顶回 dark", async () => {
    const theme = useThemeStore();
    theme.setMode("light");

    theme.setPalette("halo");
    await nextTick();

    expect(theme.isDark).toBe(false);
    expect(root().classList.contains("light")).toBe(true);
    expect(root().getAttribute("data-theme-style")).toBe("halo");
  });
});

describe("与登录页引导脚本的契约", () => {
  it("只用 theme / theme-style 两个键，键名与登录页读的一致", async () => {
    const theme = useThemeStore();

    theme.setMode("light");
    theme.setPalette("atlas");
    await nextTick();

    // 登录页的内联脚本只读这两个键；多写一个键它就读不到，主题会在首帧闪一下
    expect(Object.keys(localStorage).sort()).toEqual(["theme", "theme-style"]);
    expect(localStorage.getItem("theme")).toBe("light");
    expect(localStorage.getItem("theme-style")).toBe("atlas");
  });

  it("配色清单正好是登录页脚本里的那四个，顺序也一致", () => {
    const theme = useThemeStore();

    expect(theme.palettes.map((p) => p.value)).toEqual(["cockpit", "atlas", "deck", "halo"]);
  });
});
