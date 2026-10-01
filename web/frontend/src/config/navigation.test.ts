/*
 * 钉子：rail 上的导航入口不能在导航列钉住时留下重复项。
 *
 * 用户验收退回过一次，原话「有些重复了吧」—— rail 的快捷入口和导航列里的同名项
 * 同时挂着。shell.css 用 `.is-nav-docked .pt-rail__item` 解决，前提是 rail 上
 * **每一个**导航入口都带这个类，并且渲染在它该在的那一区。
 *
 * 这里守两件静态可判定的事：
 *   · rail 上的入口都是 NAV_GROUPS 里的同一个对象，不是另抄一份 path/label/icon；
 *   · 运行日志走 RAIL_FOOT_ITEM，不走 `rail: true`。
 *     给 /logs 打 `rail` 标记看着更省事，但那会把它塞进 rail 的主入口区
 *     （`.pt-rail__items`），而画板要的是 foot-rule 之下那一格。
 */
import { describe, expect, it } from "vitest";
import { NAV_ITEMS, RAIL_FOOT_ITEM, RAIL_FOOT_PATH, RAIL_ITEMS } from "./navigation";

describe("rail 的导航入口", () => {
  it("主入口区的项都是 NAV_GROUPS 里的同一个对象", () => {
    expect(RAIL_ITEMS.length).toBeGreaterThan(0);
    for (const item of RAIL_ITEMS) {
      expect(NAV_ITEMS).toContain(item);
    }
  });

  it("底部那一格指向运行日志，且同样取自 NAV_GROUPS", () => {
    expect(RAIL_FOOT_ITEM).toBeDefined();
    expect(RAIL_FOOT_ITEM?.path).toBe(RAIL_FOOT_PATH);
    expect(NAV_ITEMS).toContain(RAIL_FOOT_ITEM);
  });

  it("运行日志不带 rail 标记，否则会渲染到主入口区去", () => {
    expect(RAIL_FOOT_ITEM?.rail).toBeUndefined();
    expect(RAIL_ITEMS.map((i) => i.path)).not.toContain(RAIL_FOOT_PATH);
  });
});
