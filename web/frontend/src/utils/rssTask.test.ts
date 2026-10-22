/*
 * RSS 任务（原「调度器」）的状态文案与提示条规则。
 *
 * 用户反馈：「调度器」看不出是什么，以为是总开关；重启容器后它不会自己运行，而入口只是左下角一行小字。
 * 状态栏改成按 GET /api/control/status 的真实状态显示，没运行时写明原因；
 * 没开自动启动、RSS 任务又没在跑时，页面顶上给一条带「启动」「打开自动启动」的提示。
 */
import { describe, expect, it } from "vitest";

import type { RssTaskStatus } from "@/api";
import { canStartRss, canStopRss, rssBanner, rssTaskView } from "./rssTask";

function st(over: Partial<RssTaskStatus>): RssTaskStatus {
  return { state: "manual", jobs: 0, subscriptions: 2, auto_start: false, ...over };
}

describe("rssTaskView：状态栏与浮层的文案", () => {
  const now = new Date(2026, 9, 19, 10, 30);

  it("还没取到状态时只写名字，不猜", () => {
    const v = rssTaskView(null, now);
    expect(v.label).toBe("RSS 任务");
    expect(v.pill).toBe("状态未知");
    expect(v.tone).toBe("neutral");
  });

  it("运行中写明几个订阅、什么时候启动的", () => {
    const today = rssTaskView(
      st({ state: "running", jobs: 2, started_at: new Date(2026, 9, 19, 9, 5).toISOString() }),
      now,
    );
    expect(today.label).toBe("RSS 任务运行中");
    expect(today.tone).toBe("ok");
    expect(today.detail).toBe("2 个订阅在运行，今天 09:05 启动");

    const earlier = rssTaskView(
      st({ state: "running", jobs: 1, started_at: new Date(2026, 9, 17, 23, 40).toISOString() }),
      now,
    );
    expect(earlier.detail).toBe("1 个订阅在运行，10 月 17 日 23:40 启动");
  });

  it("没开自动启动、还没启动过：是「未启动」，并说明重启后要手动启动", () => {
    const v = rssTaskView(st({ state: "manual" }), now);
    expect(v.label).toBe("RSS 任务未启动");
    expect(v.tone).toBe("warn");
    expect(v.detail).toContain("自动启动");
    expect(v.detail).toContain("重启");
  });

  it("手动停止：开着自动启动时补一句重启后会自动运行", () => {
    expect(rssTaskView(st({ state: "paused", auto_start: true }), now).detail).toContain(
      "程序重启后会自动运行",
    );
    expect(rssTaskView(st({ state: "paused" }), now).detail).not.toContain("自动运行");
    expect(rssTaskView(st({ state: "paused" }), now).label).toBe("RSS 任务已停止");
  });

  it("下载器的问题写明是哪一种", () => {
    expect(rssTaskView(st({ state: "no_downloader" }), now).detail).toContain("没有设置默认下载器");
    const down = rssTaskView(st({ state: "downloader_unhealthy" }), now);
    expect(down.label).toBe("RSS 任务未运行");
    expect(down.detail).toContain("默认下载器连不上");
  });

  it("没有订阅不算异常", () => {
    const v = rssTaskView(st({ state: "no_subscriptions", subscriptions: 0 }), now);
    expect(v.label).toBe("RSS 任务无订阅");
    expect(v.tone).toBe("neutral");
  });

  it("不认识的状态（更新的后端）按未知处理", () => {
    const v = rssTaskView(st({ state: "something_new" as RssTaskStatus["state"] }), now);
    expect(v.label).toBe("RSS 任务");
    expect(v.pill).toBe("状态未知");
  });
});

describe("rssBanner：什么时候在页面顶上提示", () => {
  it("没开自动启动、有订阅、RSS 任务没在跑：提示，并给「启动」", () => {
    const b = rssBanner(st({ state: "manual" }));
    expect(b?.action).toBe("start");
    expect(b?.text).toContain("重启");
    expect(rssBanner(st({ state: "downloader_unhealthy" }))?.action).toBe("start");
    expect(rssBanner(st({ state: "stopped" }))?.action).toBe("start");
  });

  it("没有默认下载器时引去设置下载器，而不是「启动」", () => {
    expect(rssBanner(st({ state: "no_downloader" }))?.action).toBe("downloaders");
  });

  it("这些情况不提示", () => {
    expect(rssBanner(null)).toBeNull();
    expect(rssBanner(st({ state: "manual", auto_start: true }))).toBeNull();
    expect(rssBanner(st({ state: "manual", subscriptions: 0 }))).toBeNull();
    for (const state of [
      "running",
      "starting",
      "paused",
      "no_subscriptions",
      "not_ready",
    ] as const) {
      expect(rssBanner(st({ state }))).toBeNull();
    }
  });
});

describe("启停按钮什么时候可点", () => {
  it("没有任务在跑时不能停，运行中不能再启动", () => {
    expect(canStopRss(st({ state: "manual" }))).toBe(false);
    expect(canStopRss(st({ state: "running", jobs: 1 }))).toBe(true);
    expect(canStopRss(st({ state: "starting" }))).toBe(true);
    expect(canStartRss(st({ state: "running", jobs: 1 }))).toBe(false);
    expect(canStartRss(st({ state: "paused" }))).toBe(true);
  });

  it("状态没取到时两个都能点（不因为查询失败把人挡住）", () => {
    expect(canStopRss(null)).toBe(true);
    expect(canStartRss(null)).toBe(true);
  });
});
