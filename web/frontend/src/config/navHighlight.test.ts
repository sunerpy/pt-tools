// @vitest-environment happy-dom
/*
 * 钉子：每个导航项所在的页面都要高亮它。高亮按路由 name 查表（ROUTE_NAME_TO_NAV），
 * 新加的页面忘了登记，导航上就没有一项是亮的、顶栏也不写分组 —— 媒体、qB 兼容入口、
 * 远程访问等十四页曾经都是这样。
 */
import { describe, expect, it } from "vitest";

import router from "@/router";

import { activeNavPath, NAV_ITEMS } from "./navigation";

describe("导航高亮", () => {
  const navPaths = new Set(NAV_ITEMS.map((i) => i.path));
  const routes = router.getRoutes().filter((r) => typeof r.name === "string");

  it("路径就是导航项的页面，高亮的就是这一项", () => {
    const pages = routes.filter((r) => navPaths.has(r.path));
    expect(pages.length).toBeGreaterThan(0);
    for (const r of pages) {
      expect(activeNavPath(r.name), String(r.name)).toBe(r.path);
    }
  });

  it("每个导航项都有页面会高亮它", () => {
    const lit = new Set(routes.map((r) => activeNavPath(r.name)));
    for (const path of navPaths) {
      expect(lit.has(path), path).toBe(true);
    }
  });

  it("路由没就绪时不高亮任何一项", () => {
    expect(activeNavPath(undefined)).toBe("");
    expect(activeNavPath("not-a-route")).toBe("");
  });
});

describe("打开网页先到首页", () => {
  it("/ 重定向到 /home，首页、系统体检与 NAS 监控的页面高亮自己", () => {
    expect(router.getRoutes().find((r) => r.path === "/")?.redirect).toBe("/home");
    expect(activeNavPath("home")).toBe("/home");
    expect(activeNavPath("system-check")).toBe("/system/check");
    expect(activeNavPath("nas-monitor")).toBe("/system/nas");
  });
});
