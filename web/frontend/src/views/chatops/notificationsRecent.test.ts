// @vitest-environment happy-dom
/*
 * 钉子：「最近的通知」读 RSS 通知与后台通知合在一起的最近记录（以前只读 RSS 通知日志，签到、战报不在里面）。
 * 每行写通道、来源、内容（发出去的消息标题，RSS 前面加站点）、结果的中文说法与尝试次数。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ list: vi.fn(), stats: vi.fn(), recent: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    chatopsApi: {
      notifications: { list: api.list, stats: api.stats, recent: api.recent },
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

const card = () => document.querySelector<HTMLElement>("[data-testid=notify-recent]");

async function mountPage() {
  api.list.mockResolvedValue([{ id: 1, channel_type: "telegram", name: "TG", enabled: true }]);
  api.stats.mockResolvedValue({ since: "", days: 7, channels: [] });
  const Page = (await import("./Notifications.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.recent).toHaveBeenCalledWith(8));
}

describe("最近的通知", () => {
  it("RSS 通知与后台通知各写来源、内容、结果", async () => {
    api.recent.mockResolvedValue({
      items: [
        {
          source: "attendance",
          log_id: 3,
          conf_id: 1,
          title: "签到汇总",
          kind: "daily",
          result: "failed",
          attempts: 4,
          created_at: "2026-10-16T08:00:00+08:00",
        },
        {
          source: "rss",
          log_id: 9,
          conf_id: 1,
          title: "Some.Movie.2026.1080p",
          site: "hdsky",
          torrent_id: "123",
          kind: "filtered",
          result: "sent",
          attempts: 1,
          created_at: "2026-10-16T07:00:00+08:00",
        },
        {
          source: "rss",
          log_id: 10,
          conf_id: 7,
          title: "",
          site: "hdsky",
          torrent_id: "124",
          kind: "all",
          result: "throttled",
          attempts: 0,
          created_at: "2026-10-16T06:00:00+08:00",
        },
      ],
    });
    await mountPage();
    await vi.waitFor(() => expect(card()?.querySelectorAll("tbody tr").length).toBe(3));
    const rows = [...card()!.querySelectorAll("tbody tr")].map((r) =>
      [...r.querySelectorAll("td")].map((td) => td.textContent?.trim()),
    );
    expect(rows[0].slice(1)).toEqual(["TG", "签到", "签到汇总", "失败", "4"]);
    expect(rows[1].slice(1)).toEqual([
      "TG",
      "RSS · 仅匹配的",
      "hdsky · Some.Movie.2026.1080p",
      "已发出",
      "1",
    ]);
    expect(rows[2].slice(1)).toEqual([
      "#7",
      "RSS · 全部新种",
      "hdsky · 种子 124",
      "超出每小时上限",
      "0",
    ]);
    const head = [...card()!.querySelectorAll("thead th")].map((th) => th.textContent?.trim());
    expect(head).toEqual(["时间", "通道", "来源", "内容", "结果", "尝试"]);
    expect(card()!.textContent).toContain("3 条");
    expect(card()!.textContent).toContain("后台通知发送失败会在 1、5、30 分钟后各重试一次");
  });

  it("没有记录、或者没取到时写明", async () => {
    api.recent.mockResolvedValue({ items: [] });
    await mountPage();
    await vi.waitFor(() => expect(card()?.textContent).toContain("还没有通知记录。"));
    view?.unmount();

    api.recent.mockRejectedValue(new Error("boom"));
    await mountPage();
    await vi.waitFor(() => expect(card()?.textContent).toContain("最近的通知没取到。"));
  });
});
