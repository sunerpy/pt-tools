// @vitest-environment happy-dom
/*
 * 钉子：「最近投递统计」读近 7 天的统计接口（RSS 与后台监控通知合计），不再只数 RSS 通知日志最近 50 条。
 * 每个通道一行「已送达 / 总条数」；小字写来源，再写没送达的是失败、在等还是按规则没发；删掉的通道写明已删除。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ list: vi.fn(), stats: vi.fn(), logs: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    chatopsApi: {
      notifications: { list: api.list, stats: api.stats },
      rssNotifications: { list: api.logs },
    },
  };
});

vi.mock("vue-router", async (orig) => {
  const real = await orig<typeof import("vue-router")>();
  return { ...real, useRouter: () => ({ push: vi.fn() }) };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const card = () => document.querySelector<HTMLElement>("[data-testid=notify-stats]");

async function mountPage() {
  api.list.mockResolvedValue([{ id: 1, channel_type: "telegram", name: "TG", enabled: true }]);
  api.logs.mockResolvedValue({ items: [] });
  const Page = (await import("./Notifications.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.stats).toHaveBeenCalled());
}

describe("最近投递统计", () => {
  it("按通道写已送达 / 总条数，小字写来源与没送达的原因", async () => {
    api.stats.mockResolvedValue({
      since: "2026-10-08T12:00:00+08:00",
      days: 7,
      channels: [
        {
          conf_id: 1,
          total: 7,
          sent: 3,
          failed: 1,
          pending: 1,
          skipped: 2,
          sources: [
            { source: "rss", total: 5, sent: 2 },
            { source: "attendance", total: 2, sent: 1 },
          ],
        },
        {
          conf_id: 9,
          total: 1,
          sent: 1,
          failed: 0,
          pending: 0,
          skipped: 0,
          sources: [{ source: "daily_report", total: 1, sent: 1 }],
        },
      ],
    });
    await mountPage();
    await vi.waitFor(() => expect(card()?.querySelectorAll(".bd__row").length).toBe(2));
    const text = card()!.textContent ?? "";
    expect(text).toContain("近 7 天");
    expect(text).toContain("通道测试和命令回复不算");
    const [first, second] = [...card()!.querySelectorAll(".bd__row")].map((r) => r.textContent);
    expect(first).toContain("TG");
    expect(first).toContain("3 / 7");
    expect(first).toContain("RSS 5 · 签到 2；1 条失败、1 条等待发送、2 条按规则没发");
    expect(second).toContain("已删除的通道 #9");
    expect(second).toContain("1 / 1");
    expect(second).toContain("每日战报 1；全部送达");
  });

  it("近 7 天没有投递、统计没取到时分别写明", async () => {
    api.stats.mockResolvedValue({ since: "", days: 7, channels: [] });
    await mountPage();
    await vi.waitFor(() => expect(card()?.textContent).toContain("近 7 天没有投递记录。"));
    view?.unmount();

    api.stats.mockRejectedValue(new Error("boom"));
    await mountPage();
    await vi.waitFor(() => expect(card()?.textContent).toContain("投递统计没取到。"));
  });
});
