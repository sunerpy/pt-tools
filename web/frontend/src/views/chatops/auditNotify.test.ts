// @vitest-environment happy-dom
/*
 * 钉子（2026-10-28 用户反馈）：签到、追剧、每日战报这些通知不在操作审计里，每日战报发送失败了也不知道为什么。
 * 后台通知的每次投递现在记成 notify:<来源>:<种类>：表里写通知的叫法、触发用户写「系统通知」，
 * 没发出去的直接在这一行写原因；「类型」chip 选「消息通知」时发 category=notify。
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
            today_count: 2,
            total_count: 2,
            success_rate: 50,
            max_latency_ms: 120,
            avg_latency_ms: 60,
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

const failedReport = {
  id: 11,
  channel_type: "qq_onebot",
  channel_user_id: "",
  command: "notify:daily_report:daily",
  args_json: JSON.stringify({
    title: "每日战报 2026-10-28",
    attempt: 2,
    error: "QQ 通道无可用收件人：在通道设置里填写管理员 QQ 或允许 QQ，或者先完成一次绑定",
    next_retry_at: new Date(2026, 9, 28, 22, 36).toISOString(),
  }),
  result: "error:send_failed",
  latency_ms: 0,
  created_at: new Date(2026, 9, 28, 22, 6).toISOString(),
};

const sentFollow = {
  id: 12,
  channel_type: "telegram",
  channel_user_id: "",
  command: "notify:media:followshow_found",
  args_json: JSON.stringify({ title: "追剧：某剧 S01E05", attempt: 1 }),
  result: "success",
  latency_ms: 120,
  created_at: new Date(2026, 9, 28, 21, 0).toISOString(),
};

describe("审计日志：后台通知的投递", () => {
  it("写通知的叫法与没发出去的原因，触发用户是系统通知", async () => {
    api.list.mockResolvedValue({ items: [failedReport, sentFollow], total: 2 });
    const AuditLog = (await import("./AuditLog.vue")).default;
    view = mountView(AuditLog);
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
    // 空态的说明里也有「每日战报」四个字：等真正的行出来
    await vi.waitFor(() =>
      expect(document.querySelector(".el-table")?.textContent ?? "").toContain(
        "追剧：下载了新的集",
      ),
    );
    const table = document.querySelector(".el-table")!.textContent ?? "";
    expect(table).toContain("每日战报");
    expect(table).toContain("系统通知");
    expect(table).toContain("QQ 通道无可用收件人");
    expect(table).toContain("第 2 次，22:36 重试");
    expect(table).not.toContain("notify:daily_report");
    // 「失败与被拒」卡里也写原因，不是只写 send_failed
    const fail = [...document.querySelectorAll(".pt-panel")].find((p) =>
      (p.textContent ?? "").includes("失败与被拒"),
    );
    expect(fail?.textContent ?? "").toContain("QQ 通道无可用收件人");
  });

  it("「类型」chip 选消息通知时只查通知", async () => {
    api.list.mockResolvedValue({ items: [sentFollow], total: 1 });
    const AuditLog = (await import("./AuditLog.vue")).default;
    view = mountView(AuditLog);
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(1));
    expect((api.list.mock.calls[0]![0] as URLSearchParams).get("category")).toBeNull();

    const chip = document.querySelector("[data-testid=audit-category-chip]")!;
    (chip.querySelector<HTMLElement>(".el-select__wrapper") ?? (chip as HTMLElement)).click();
    await vi.waitFor(() => {
      const opt = [...document.querySelectorAll<HTMLElement>(".el-select-dropdown__item")].find(
        (o) => (o.textContent ?? "").trim() === "消息通知",
      );
      expect(opt).toBeTruthy();
      opt!.click();
    });
    await vi.waitFor(() => expect(api.list).toHaveBeenCalledTimes(2));
    expect((api.list.mock.calls[1]![0] as URLSearchParams).get("category")).toBe("notify");
  });
});
