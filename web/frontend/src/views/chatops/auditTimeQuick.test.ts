// @vitest-environment happy-dom
/*
 * 钉子：审计日志的快捷时间档（最近 1 小时 / 24 小时 / 7 天）只发 since，不发 until。
 *
 * 原来点选快捷档时就把 until 定死成那一刻，之后翻页、点刷新、换别的筛选都还带着这个旧 until ——
 * 新产生的审计记录永远看不到，「最近 1 小时」过一会儿就名不副实。快捷档是「从现在往回数」的窗口，
 * 每次请求按当时重算 since；只有「自定义…」选出来的区间才两头都发。
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ list: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    chatopsApi: {
      audit: {
        list: api.list,
        stats: vi.fn(() =>
          Promise.resolve({
            today_count: 0,
            total_count: 0,
            success_rate: 0,
            max_latency_ms: 0,
            avg_latency_ms: 0,
          }),
        ),
      },
    },
  };
});

let view: MountedView | null = null;

/** 第 n 次列表请求带的查询参数 */
function params(n: number): URLSearchParams {
  return api.list.mock.calls[n]![0] as URLSearchParams;
}

/*
 * 时间参数按秒级比：vi.waitFor 在假时钟下每次轮询会自己把时钟往前推 50ms，
 * 要求毫秒相等只会测到轮询次数。
 */
function expectNear(iso: string | null, want: string) {
  expect(iso).not.toBeNull();
  expect(Math.abs(Date.parse(iso!) - Date.parse(want))).toBeLessThan(2_000);
}

async function chooseTime(label: string) {
  const chip = document.querySelector("[data-testid=audit-time-chip]")!;
  (chip.querySelector<HTMLElement>(".el-select__wrapper") ?? (chip as HTMLElement)).click();
  await vi.waitFor(() => {
    const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
      (o) => (o.textContent ?? "").trim() === label,
    );
    expect(opt).toBeTruthy();
    opt!.click();
  });
}

beforeEach(() => {
  api.list.mockResolvedValue({ items: [], total: 0 });
});

afterEach(() => {
  view?.unmount();
  view = null;
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe("审计日志：快捷时间档是滑动窗口", () => {
  it("选「最近 1 小时」：只发 since（一小时前），不发 until", async () => {
    const AuditLog = (await import("./AuditLog.vue")).default;
    view = mountView(AuditLog);
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));

    const before = Date.now();
    await chooseTime("最近 1 小时");
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    const p = params(1);
    expect(p.get("until")).toBeNull();
    expectNear(p.get("since"), new Date(before - 3600_000).toISOString());
  });

  it("过一会儿点刷新：since 按当时重算，仍然不带 until —— 新记录才看得到", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-10-01T10:00:00Z"));
    const AuditLog = (await import("./AuditLog.vue")).default;
    view = mountView(AuditLog);
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
    await chooseTime("最近 1 小时");
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    expectNear(params(1).get("since"), "2026-10-01T09:00:00Z");

    vi.setSystemTime(new Date("2026-10-01T10:30:00Z"));
    /* 刷新键在请求落地前是禁用的（:disabled="loading"），等它可点再点 */
    const refresh = document.querySelector<HTMLButtonElement>("button[aria-label='刷新']")!;
    await vi.waitFor(() => expect(refresh.disabled).toBe(false));
    refresh.click();
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(3));
    expectNear(params(2).get("since"), "2026-10-01T09:30:00Z");
    expect(params(2).get("until")).toBeNull();
  });

  it("「自定义…」是两头定死的区间：since 与 until 都发（从快捷档切过去时用刚才的窗口当初值）", async () => {
    const AuditLog = (await import("./AuditLog.vue")).default;
    view = mountView(AuditLog);
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
    await chooseTime("最近 24 小时");
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));

    const now = Date.now();
    await chooseTime("自定义…");
    const refresh = document.querySelector<HTMLButtonElement>("button[aria-label='刷新']")!;
    await vi.waitFor(() => expect(refresh.disabled).toBe(false));
    refresh.click();
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(3));
    expectNear(params(2).get("since"), new Date(now - 24 * 3600_000).toISOString());
    expectNear(params(2).get("until"), new Date(now).toISOString());
  });

  it("选回「时间: 不限」：since / until 都不发", async () => {
    const AuditLog = (await import("./AuditLog.vue")).default;
    view = mountView(AuditLog);
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
    await chooseTime("最近 7 天");
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    await chooseTime("时间: 不限");
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(3));
    expect(params(2).get("since")).toBeNull();
    expect(params(2).get("until")).toBeNull();
  });
});
