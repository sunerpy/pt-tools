// @vitest-environment happy-dom
/*
 * 「自动签到」页：今天的签到按站点列出（默认只看已启用的站点），开关、立即签到、一键签到，
 * 最近两周的签到记录格子，点日期看那一天的签到详情（直接签到还是借助 AI、AI 用量、每次尝试）。点行进站点详情。
 */
import { createPinia, setActivePinia } from "pinia";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import type { SiteAttendance } from "@/api";
import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  signNow: vi.fn(),
  setEnabled: vi.fn(),
  runAll: vi.fn(),
  runStatus: vi.fn(),
  history: vi.fn(),
  events: vi.fn(),
  getSettings: vi.fn(),
}));

const ui = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn(), warning: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    attendanceApi: { ...real.attendanceApi, ...api, saveSettings: vi.fn(), testVision: vi.fn() },
    chatopsApi: { ...real.chatopsApi, notifications: { list: vi.fn(() => Promise.resolve([])) } },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), { ...ui, info: vi.fn() }),
  };
});

const today = "2026-10-19";

function site(name: string, extra: Partial<SiteAttendance> = {}): SiteAttendance {
  return {
    site_name: name,
    site_enabled: true,
    attendance_enabled: false,
    supported: true,
    day: today,
    status: "",
    attempts: 0,
    ...extra,
  };
}

const SITES: SiteAttendance[] = [
  site("hdsky", {
    attendance_enabled: true,
    status: "signed",
    message: "这是您的第 9 次签到",
    challenge: "hdsky_captcha",
    last_attempt_at: 1792200000,
  }),
  site("rousipro", {
    attendance_enabled: true,
    status: "failed",
    last_error: "Rousi 拒绝了签到：API Key 没有签到权限",
  }),
  site("mteam", {
    supported: false,
    unsupported_reason: "M-Team 没有签到；保号要定期用浏览器登录",
  }),
  site("pthome", { site_enabled: false }),
];

let view: MountedView | null = null;
let router: Router;

beforeEach(() => {
  setActivePinia(createPinia());
  vi.spyOn(window, "matchMedia").mockImplementation(
    (query: string) =>
      ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: () => {},
        removeEventListener: () => {},
        addListener: () => {},
        removeListener: () => {},
        dispatchEvent: () => false,
      }) as MediaQueryList,
  );
  api.list.mockResolvedValue(structuredClone(SITES));
  api.runStatus.mockResolvedValue({ running: false, done: 0, total: 0 });
  api.history.mockResolvedValue({
    days: [today, "2026-10-18"],
    rows: [
      {
        site_name: "hdsky",
        day: today,
        status: "signed",
        attempts: 1,
        message: "这是您的第 9 次签到",
      },
      {
        site_name: "hdsky",
        day: "2026-10-18",
        status: "failed",
        attempts: 3,
        last_error: "HTTP 502",
      },
    ],
  });
  api.getSettings.mockResolvedValue({ window_start: "08:00", window_end: "10:00" });
  api.events.mockImplementation((day?: string) => Promise.resolve(dayDetail(day ?? today)));
});

/** 签到详情：今天 hdsky 借助 AI 签上（第二次才成）、rousipro 失败；昨天 hdsky 失败 */
function dayDetail(day: string) {
  const ev = (over: Record<string, unknown>) => ({
    id: Math.random(),
    trigger: "auto",
    result: "signed",
    method: "direct",
    ai_calls: 0,
    prompt_tokens: 0,
    completion_tokens: 0,
    duration_ms: 800,
    started_at: 1792200000,
    finished_at: 1792200001,
    ...over,
  });
  if (day !== today) {
    return {
      day,
      today,
      summary: {
        sites: 1,
        signed: 0,
        already: 0,
        failed: 1,
        pending: 0,
        unsupported: 0,
        ai_sites: 0,
        ai_calls: 0,
        prompt_tokens: 0,
        completion_tokens: 0,
      },
      sites: [
        {
          site_name: "hdsky",
          status: "failed",
          attempts: 3,
          last_error: "HTTP 502",
          method: "ai_captcha",
          ai_calls: 0,
          prompt_tokens: 0,
          completion_tokens: 0,
          events: [],
        },
      ],
    };
  }
  return {
    day,
    today,
    summary: {
      sites: 2,
      signed: 1,
      already: 0,
      failed: 1,
      pending: 0,
      unsupported: 0,
      ai_sites: 1,
      ai_calls: 3,
      prompt_tokens: 1234,
      completion_tokens: 18,
    },
    sites: [
      {
        site_name: "hdsky",
        status: "signed",
        attempts: 2,
        message: "这是您的第 9 次签到",
        method: "ai_captcha",
        ai_model: "vision-x",
        ai_calls: 3,
        prompt_tokens: 1234,
        completion_tokens: 18,
        events: [
          ev({
            result: "retry",
            method: "ai_captcha",
            ai_model: "vision-x",
            ai_calls: 2,
            prompt_tokens: 800,
            completion_tokens: 12,
            error: "验证码没认对",
          }),
          ev({
            method: "ai_captcha",
            ai_model: "vision-x",
            ai_calls: 1,
            prompt_tokens: 434,
            completion_tokens: 6,
            message: "这是您的第 9 次签到",
            trigger: "manual",
          }),
        ],
      },
      {
        site_name: "rousipro",
        status: "failed",
        attempts: 1,
        last_error: "API Key 没有签到权限",
        method: "direct",
        ai_calls: 0,
        prompt_tokens: 0,
        completion_tokens: 0,
        events: [ev({ result: "failed", error: "API Key 没有签到权限" })],
      },
    ],
  };
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.restoreAllMocks();
  vi.clearAllMocks();
  vi.useRealTimers();
});

async function mountPage() {
  const Attendance = (await import("./Attendance.vue")).default;
  router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/attendance", component: Attendance },
      { path: "/sites/:name", component: { render: () => null } },
      { path: "/:p(.*)*", component: { render: () => null } },
    ],
  });
  await router.push("/attendance");
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
  await vi.waitFor(() =>
    expect(document.querySelector("[data-testid=att-table] .el-table__row")).toBeTruthy(),
  );
}

function rowNames(): string[] {
  return [
    ...document.querySelectorAll("[data-testid=att-table] .el-table__row .att-site__name"),
  ].map((e) => e.textContent?.trim() ?? "");
}

describe("自动签到页", () => {
  it("默认只列已启用的站点：开了自动签到的排前面，不支持的写明原因", async () => {
    await mountPage();
    expect(rowNames()).toEqual(["hdsky", "rousipro", "mteam"]);
    const text = document.querySelector("[data-testid=att-table]")!.textContent ?? "";
    expect(text).toContain("已签到");
    expect(text).toContain("签到失败");
    expect(text).toContain("API Key 没有签到权限");
    expect(text).toContain("不支持");
    // 要用 AI 的站点名字后面有个小标
    expect(document.querySelector("[data-testid=att-table] .att-ai")?.textContent).toBe("AI");
    // 不支持签到的站点开关不可点
    expect(
      document.querySelector("[data-testid=att-toggle-mteam]")?.classList.contains("is-disabled"),
    ).toBe(true);
  });

  it("切到「全部」能看到没启用的站点", async () => {
    await mountPage();
    const all = [
      ...document.querySelectorAll<HTMLElement>("[data-testid=att-scope] .el-segmented__item"),
    ].find((e) => (e.textContent ?? "").includes("全部"))!;
    all.click();
    await vi.waitFor(() => expect(rowNames()).toContain("pthome"));
  });

  it("立即签到：用接口回的结果更新这一行，并刷新记录", async () => {
    await mountPage();
    api.signNow.mockResolvedValue(
      site("rousipro", { attendance_enabled: true, status: "already", message: "今日已签到" }),
    );
    (document.querySelector("[data-testid=att-sign-rousipro]") as HTMLElement).click();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("rousipro：今日已签到"));
    expect(api.signNow).toHaveBeenCalledWith("rousipro");
    expect(api.history).toHaveBeenCalledTimes(2);
  });

  it("开关：打开以后这一行记成开了自动签到", async () => {
    await mountPage();
    api.setEnabled.mockResolvedValue(site("hdsky", { attendance_enabled: false }));
    (document.querySelector("[data-testid=att-toggle-hdsky]") as HTMLElement).click();
    await vi.waitFor(() => expect(api.setEnabled).toHaveBeenCalledWith("hdsky", false));
  });

  it("一键签到：开始后显示进度，跑完刷新并提示", async () => {
    await mountPage();
    api.runAll.mockResolvedValue({ running: true, done: 0, total: 2 });
    api.runStatus
      .mockResolvedValueOnce({ running: true, done: 1, total: 2 })
      .mockResolvedValue({ running: false, done: 2, total: 2, finished_at: 1792200100 });
    buttonByText("一键签到").click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=att-running]")?.textContent).toContain(
        "一键签到进行中",
      ),
    );
    await vi.waitFor(
      () => expect(ui.success).toHaveBeenCalledWith("一键签到做完了：处理了 2 个站点"),
      {
        timeout: 6000,
      },
    );
  });

  it("签到记录：每个站点一行、每天一格，从早到今天", async () => {
    await mountPage();
    const hist = document.querySelector("[data-testid=att-history]")!;
    const heads = [...hist.querySelectorAll("thead th")].map((e) => e.textContent?.trim());
    expect(heads).toEqual(["站点", "10/18", "10/19"]);
    const names = [...hist.querySelectorAll("tbody .att-hist__site")].map((e) =>
      e.textContent?.trim(),
    );
    // hdsky 有记录；rousipro 开了自动签到，这两周还没有记录也列出来
    expect(names).toEqual(["hdsky", "rousipro"]);
    const hdskyDots = [...hist.querySelectorAll("tbody tr")][0]!.querySelectorAll(".att-dot");
    expect(
      [...hdskyDots].map((d) => [...d.classList].find((c) => c.startsWith("att-dot--"))),
    ).toEqual(["att-dot--dang", "att-dot--ok"]);
  });

  it("签到详情：默认今天，写明成功几个、借助 AI 的用量；每个站点的方式与 AI 用量，展开看每次尝试", async () => {
    await mountPage();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=att-day-table]")).toBeTruthy(),
    );
    expect(api.events).toHaveBeenCalledWith(undefined);
    const summary = document.querySelector("[data-testid=att-day-summary]")!.textContent!;
    expect(summary).toContain("2 个站点");
    expect(summary).toContain("成功 1");
    expect(summary).toContain("失败 1");
    const ai = document.querySelector("[data-testid=att-day-ai]")!.textContent!;
    expect(ai).toContain("借助 AI 1 个站点");
    expect(ai).toContain("调用 3 次");
    expect(ai).toContain("输入 1,234 / 输出 18 token");

    const rows = [...document.querySelectorAll("[data-testid=att-day-table] .el-table__row")];
    expect(rows).toHaveLength(2);
    expect(rows[0]!.textContent).toContain("AI 识别验证码");
    expect(rows[0]!.textContent).toContain("vision-x · 调用 3 次");
    expect(rows[1]!.textContent).toContain("直接签到");
    expect(rows[1]!.textContent).toContain("API Key 没有签到权限");

    // 展开 hdsky：两次尝试，第一次认错验证码之后重试，第二次手动签上
    (rows[0]!.querySelector(".el-table__expand-icon") as HTMLElement).click();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=att-ev-hdsky]")).toBeTruthy(),
    );
    const items = [...document.querySelectorAll("[data-testid=att-ev-hdsky] .att-ev__item")];
    expect(items).toHaveLength(2);
    expect(items[0]!.textContent).toContain("失败，稍后重试");
    expect(items[0]!.textContent).toContain("验证码没认对");
    expect(items[0]!.textContent).toContain("输入 800 / 输出 12 token");
    expect(items[1]!.textContent).toContain("手动");
    expect(items[1]!.textContent).toContain("签上了");
  });

  it("点签到记录里的日期：签到详情换成那一天；能前后翻", async () => {
    await mountPage();
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=att-day-table]")).toBeTruthy(),
    );
    (document.querySelector("[data-testid=att-hist-day-2026-10-18]") as HTMLElement).click();
    await vi.waitFor(() => expect(api.events).toHaveBeenCalledWith("2026-10-18"));
    await vi.waitFor(() =>
      expect(document.querySelector("[data-testid=att-day-summary]")!.textContent).toContain(
        "1 个站点",
      ),
    );
    expect(document.querySelector("[data-testid=att-day-ai]")).toBeNull();
    expect(
      document
        .querySelector("[data-testid=att-hist-day-2026-10-18]")!
        .classList.contains("is-active"),
    ).toBe(true);
    // 已经是最早的一天：不能再往前；往后回到今天
    expect(
      (document.querySelector("[data-testid=att-day-prev]") as HTMLButtonElement).disabled,
    ).toBe(true);
    (document.querySelector("[data-testid=att-day-next]") as HTMLElement).click();
    await vi.waitFor(() => expect(api.events).toHaveBeenLastCalledWith(today));
  });

  it("点行进站点详情；点行里的按钮不跳转", async () => {
    await mountPage();
    api.signNow.mockResolvedValue(site("hdsky", { attendance_enabled: true, status: "already" }));
    (document.querySelector("[data-testid=att-sign-hdsky]") as HTMLElement).click();
    await new Promise((r) => setTimeout(r, 0));
    expect(router.currentRoute.value.fullPath).toBe("/attendance");
    (
      document.querySelector(
        "[data-testid=att-table] .el-table__row .att-site__name",
      ) as HTMLElement
    ).click();
    await vi.waitFor(() => expect(router.currentRoute.value.fullPath).toBe("/sites/hdsky"));
  });
});
