// @vitest-environment happy-dom
/*
 * 钉子：App API 的令牌做的写请求记成 channel_type = api_token（路线图 M12）。审计页要认得它：
 * 表里写「API 令牌」而不是原始串，通道筛选里也能选它，选了发 channel_type=api_token。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

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
            today_count: 1,
            total_count: 1,
            success_rate: 100,
            max_latency_ms: 12,
            avg_latency_ms: 12,
          }),
        ),
      },
    },
  };
});

let view: MountedView | null = null;

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const tokenWrite = {
  id: 7,
  notification_conf_id: 0,
  channel_type: "api_token",
  channel_user_id: "3",
  command: "POST /api/app/v1/push",
  args: { path: "/api/app/v1/push", status: 200, name: "我的手机" },
  result: "success",
  latency_ms: 12,
  created_at: "2026-10-08T10:00:00Z",
};

describe("审计日志：认得 API 令牌", () => {
  it("表里写「API 令牌」，筛选能选它", async () => {
    api.list.mockResolvedValue({ items: [tokenWrite], total: 1 });
    const AuditLog = (await import("./AuditLog.vue")).default;
    view = mountView(AuditLog);
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
    await vi.waitFor(() =>
      expect(document.querySelector(".el-table")?.textContent ?? "").toContain("API 令牌"),
    );
    // 表格与「渠道分布」都不露原始串
    expect(document.body.textContent ?? "").not.toContain("api_token");

    const chip = document.querySelector("[data-testid=audit-channel-chip]")!;
    (chip.querySelector<HTMLElement>(".el-select__wrapper") ?? (chip as HTMLElement)).click();
    await vi.waitFor(() => {
      const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
        (o) => (o.textContent ?? "").trim() === "通道: API 令牌",
      );
      expect(opt).toBeTruthy();
      opt!.click();
    });
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    const params = api.list.mock.calls[1]![0] as URLSearchParams;
    expect(params.get("channel_type")).toBe("api_token");
  });
});
