// @vitest-environment happy-dom
/*
 * 「自动签到」页的签到设置：时间窗、AI 签到（开关与看图模型）、签到结果推送一起保存；
 * AI 助手没配好时提醒；「测试看图」写明认没认对。以前这些在「全局设置 → 每日签到」里。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  getAttendance: vi.fn(),
  saveAttendance: vi.fn(),
  testVision: vi.fn(),
  channels: vi.fn(),
}));

const ui = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    attendanceApi: {
      getSettings: api.getAttendance,
      saveSettings: api.saveAttendance,
      testVision: api.testVision,
    },
    chatopsApi: { ...real.chatopsApi, notifications: { list: api.channels } },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: ui.error,
      warning: ui.warning,
      info: vi.fn(),
    }),
  };
});

let view: MountedView | null = null;

async function mountWith(attendance: Record<string, unknown>) {
  api.getAttendance.mockResolvedValue(attendance);
  api.channels.mockResolvedValue([
    { id: 3, channel_type: "telegram", name: "TG", enabled: true },
    { id: 4, channel_type: "webhook", name: "停用的", enabled: false },
  ]);
  const Form = (await import("./AttendanceSettingsForm.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:p(.*)*", component: { template: "<div />" } }],
  });
  await router.push("/attendance");
  view = mountView(Form, { router });
  await vi.waitFor(() => expect(api.getAttendance).toHaveBeenCalled());
  await vi.waitFor(() =>
    expect(
      document.querySelector("[data-testid=attendance-settings]")?.getAttribute("data-state"),
    ).toBe("ready"),
  );
}

function saveButton(): HTMLButtonElement {
  const btn = document.querySelector<HTMLButtonElement>("[data-testid=attendance-settings-save]");
  if (!btn || btn.disabled) throw new Error("找不到可点的「保存签到设置」");
  return btn;
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

describe("签到设置：AI 签到与推送", () => {
  it("打开 AI 签到后和时间窗、推送一起保存；没改过时保存键不可点", async () => {
    await mountWith({
      window_start: "08:00",
      window_end: "10:00",
      ai_enabled: false,
      vision_model: "",
      assistant_ready: true,
      assistant_model: "deepseek-chat",
    });
    api.saveAttendance.mockImplementation((v: Record<string, unknown>) =>
      Promise.resolve({ ...v, assistant_ready: true }),
    );
    expect(
      document.querySelector<HTMLButtonElement>("[data-testid=attendance-settings-save]")?.disabled,
    ).toBe(true);
    (document.querySelector("[data-testid=attendance-ai-enabled]") as HTMLElement).click();
    await vi.waitFor(() => saveButton());
    saveButton().click();

    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("签到设置已保存"));
    expect(api.saveAttendance).toHaveBeenCalledWith({
      window_start: "08:00",
      window_end: "10:00",
      ai_enabled: true,
      vision_model: "",
      notify_enabled: true,
      notify_channel_ids: [],
    });
    // 没选看图模型时写明用 AI 助手的哪个模型
    const input = document.querySelector<HTMLInputElement>(
      "[data-testid=attendance-vision-model] input",
    );
    expect(document.body.textContent).toContain("DeepSeek 不能看图");
    expect(input?.getAttribute("placeholder") ?? document.body.innerHTML).toContain(
      "deepseek-chat",
    );
  });

  it("AI 助手没配好时提醒，「测试看图」不可点", async () => {
    await mountWith({
      window_start: "08:00",
      window_end: "10:00",
      ai_enabled: true,
      assistant_ready: false,
    });
    expect(document.querySelector("[data-testid=attendance-ai-not-ready]")?.textContent).toContain(
      "AI 助手设置",
    );
    expect(buttonByText("测试看图").disabled).toBe(true);
  });

  it("「测试看图」写明认没认对", async () => {
    await mountWith({
      window_start: "08:00",
      window_end: "10:00",
      ai_enabled: true,
      vision_model: "gemini-3.5-flash-lite",
      assistant_ready: true,
    });
    api.testVision.mockResolvedValueOnce({
      model: "gemini-3.5-flash-lite",
      expected: "HK7PN3",
      got: "HK7PN3",
      ok: true,
      mismatch: 0,
      latency_ms: 1200,
    });
    buttonByText("测试看图").click();
    await vi.waitFor(() =>
      expect(
        document.querySelector("[data-testid=attendance-vision-result]")?.textContent,
      ).toContain("认对了"),
    );
    expect(api.testVision).toHaveBeenCalledWith("gemini-3.5-flash-lite");

    api.testVision.mockResolvedValueOnce({
      model: "gemini-3.5-flash-lite",
      expected: "HK7PN3",
      got: "HK1PN3",
      ok: false,
      mismatch: 1,
      latency_ms: 900,
    });
    buttonByText("测试看图").click();
    await vi.waitFor(() =>
      expect(
        document.querySelector("[data-testid=attendance-vision-result]")?.textContent,
      ).toContain("差 1 个字"),
    );

    api.testVision.mockResolvedValueOnce({
      model: "deepseek-chat",
      expected: "HK7PN3",
      got: "",
      ok: false,
      mismatch: 6,
      latency_ms: 900,
    });
    buttonByText("测试看图").click();
    await vi.waitFor(() =>
      expect(
        document.querySelector("[data-testid=attendance-vision-result]")?.textContent,
      ).toContain("可能不能看图"),
    );
  });

  it("推送：通道下拉只列启用的通道；关掉推送时通道不可选；保存带上推送两项", async () => {
    await mountWith({
      window_start: "08:00",
      window_end: "10:00",
      ai_enabled: false,
      assistant_ready: true,
      notify_enabled: true,
      notify_channel_ids: [3],
    });
    api.saveAttendance.mockImplementation((v: Record<string, unknown>) =>
      Promise.resolve({ ...v }),
    );
    await vi.waitFor(() =>
      expect(
        document.querySelector("[data-testid=attendance-notify-channels]")?.textContent,
      ).toContain("TG"),
    );
    (document.querySelector("[data-testid=attendance-notify-enabled]") as HTMLElement).click();
    await vi.waitFor(() =>
      expect(
        document.querySelector("[data-testid=attendance-notify-channels] .is-disabled"),
      ).toBeTruthy(),
    );
    saveButton().click();
    await vi.waitFor(() => expect(api.saveAttendance).toHaveBeenCalled());
    expect(api.saveAttendance.mock.calls[0]![0]).toMatchObject({
      notify_enabled: false,
      notify_channel_ids: [3],
    });
  });
});
