// @vitest-environment happy-dom
/*
 * 钉子（2026-10-28 用户反馈）：「待绑定始终没有使用，实际上我的 tg 机器人已经和 pt-tools 绑定了，但是却没有显示」。
 * 生产上机器人在线、通知发到默认 Chat ID，账号在 10-21 发过 /unbind，之后的 /help 被拒。
 * 绑定页要把三件事分开写：机器人在不在线、通知发给谁、名单里的账号绑定了没有（最近一次往来是什么），
 * 名单里没绑定的账号可以直接绑定；没有绑定码时不再摆一块空的「待绑定」。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  listBindings: vi.fn(),
  channels: vi.fn(),
  bindDirect: vi.fn(),
  revokeCode: vi.fn(),
}));

const ui = vi.hoisted(() => ({ confirm: vi.fn(), success: vi.fn(), error: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    chatopsApi: {
      bindings: {
        list: api.listBindings,
        channels: api.channels,
        bindDirect: api.bindDirect,
        revokeCode: api.revokeCode,
        delete: vi.fn(),
        update: vi.fn(),
      },
      notifications: { list: vi.fn(() => Promise.resolve([])) },
    },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessageBox: { confirm: ui.confirm },
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: ui.error,
      warning: vi.fn(),
      info: vi.fn(),
    }),
  };
});

const CHANNEL = {
  conf_id: 1,
  name: "pt-tools-bot",
  channel_type: "telegram",
  enabled: true,
  runtime_state: "connected",
  notify_target: "1234567890",
  notify_via: "default_chat_id",
  users: [
    {
      user_id: "1234567890",
      roles: ["admin"],
      bound: false,
      last: {
        command: "help",
        result: "denied:not_bound",
        at: new Date(2026, 9, 21, 21, 41).toISOString(),
      },
    },
  ],
};

let view: MountedView | null = null;

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

async function mountPage() {
  const Bindings = (await import("./Bindings.vue")).default;
  view = mountView(Bindings);
  await vi.waitFor(() =>
    expect(document.querySelector("[data-testid=binding-channel]")).not.toBeNull(),
  );
}

describe("绑定页：通道一栏", () => {
  it("分开写机器人状态、通知发给谁、账号没绑定与最近一次往来；没有绑定码时不显示待绑定", async () => {
    api.listBindings.mockResolvedValue({ bindings: [], pending: [] });
    api.channels.mockResolvedValue({ channels: [CHANNEL] });
    await mountPage();
    const text = document.querySelector("[data-testid=binding-channels]")!.textContent ?? "";
    expect(text).toContain("pt-tools-bot");
    expect(text).toContain("已连接");
    expect(text).toContain("通知发给默认 Chat ID 12***7890，不用绑定也收得到");
    expect(text).toContain("管理员");
    expect(text).toContain("未绑定");
    expect(text).toContain("/help 被拒：没有绑定 · 10-21 21:41");
    const titles = [...document.querySelectorAll(".pt-panel__title")].map((h) =>
      (h.textContent ?? "").trim(),
    );
    expect(titles).toContain("通道");
    expect(titles).not.toContain("待绑定");
  });

  it("点「绑定」确认后直接绑定，再刷新", async () => {
    api.listBindings.mockResolvedValue({ bindings: [], pending: [] });
    api.channels.mockResolvedValue({ channels: [CHANNEL] });
    ui.confirm.mockResolvedValue("confirm");
    api.bindDirect.mockResolvedValue({ id: 3 });
    await mountPage();
    document.querySelector<HTMLButtonElement>("[aria-label='绑定 12***7890']")!.click();
    await vi.waitFor(() => expect(api.bindDirect).toHaveBeenCalledWith(1, "1234567890"));
    expect(ui.confirm.mock.calls[0]![0]).toContain("管理员命令");
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalled());
    await vi.waitFor(() => expect(api.listBindings).toHaveBeenCalledTimes(2));
  });

  it("取消确认不绑定；接口报错时把原因弹出来", async () => {
    api.listBindings.mockResolvedValue({ bindings: [], pending: [] });
    api.channels.mockResolvedValue({ channels: [CHANNEL] });
    await mountPage();
    ui.confirm.mockRejectedValueOnce("cancel");
    document.querySelector<HTMLButtonElement>("[aria-label='绑定 12***7890']")!.click();
    await vi.waitFor(() => expect(ui.confirm).toHaveBeenCalledTimes(1));
    expect(api.bindDirect).not.toHaveBeenCalled();

    ui.confirm.mockResolvedValueOnce("confirm");
    api.bindDirect.mockRejectedValueOnce(new Error("这个账号在这个通道上已经绑定了"));
    document.querySelector<HTMLButtonElement>("[aria-label='绑定 12***7890']")!.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("这个账号在这个通道上已经绑定了"));
  });

  it("没有收件人与名单是空的都说清楚；有绑定码时显示待绑定", async () => {
    api.listBindings.mockResolvedValue({
      bindings: [],
      pending: [{ id: 0, code: "K3F9Q2MX", conf_id: 2, channel_type: "", channel_user_id: "" }],
    });
    api.channels.mockResolvedValue({
      channels: [
        {
          conf_id: 2,
          name: "qq",
          channel_type: "qq_onebot",
          enabled: true,
          notify_error:
            "QQ 通道无可用收件人：在通道设置里填写管理员 QQ 或允许 QQ，或者先完成一次绑定",
          users: [],
        },
      ],
    });
    await mountPage();
    const text = document.querySelector("[data-testid=binding-channels]")!.textContent ?? "";
    expect(text).toContain("通知发不出去：QQ 通道无可用收件人");
    expect(text).toContain("名单是空的");
    await vi.waitFor(() => expect(document.body.textContent ?? "").toContain("K3F9Q2MX"));
    expect(buttonByText("生成绑定码")).toBeTruthy();
  });

  // 2026-10-29 用户：「为什么我绑定了之后」待绑定里还有 10-21 生成的永久码 —— 直接绑定不消耗绑定码，以前又没有作废的入口
  it("待绑定里没用掉的码可以作废：确认后按编号作废并刷新；取消不作废", async () => {
    api.listBindings.mockResolvedValue({
      bindings: [],
      pending: [
        {
          id: 2,
          code: "KBAZSG38",
          conf_id: 1,
          channel_type: "",
          channel_user_id: "",
          created_at: "2026-10-21T12:54:27Z",
        },
      ],
    });
    api.channels.mockResolvedValue({ channels: [CHANNEL] });
    await mountPage();
    await vi.waitFor(() => expect(document.body.textContent ?? "").toContain("KBAZSG38"));
    const voidBtn = () =>
      document.querySelector<HTMLButtonElement>("[aria-label='作废绑定码 KBAZSG38']")!;
    expect(voidBtn()).not.toBeNull();
    expect(document.body.textContent ?? "").toContain("直接绑定不用绑定码");

    ui.confirm.mockRejectedValueOnce("cancel");
    voidBtn().click();
    await vi.waitFor(() => expect(ui.confirm).toHaveBeenCalledTimes(1));
    expect(api.revokeCode).not.toHaveBeenCalled();

    ui.confirm.mockResolvedValueOnce("confirm");
    api.revokeCode.mockResolvedValueOnce({ status: "ok" });
    voidBtn().click();
    await vi.waitFor(() => expect(api.revokeCode).toHaveBeenCalledWith(2));
    expect(ui.confirm.mock.calls[1]![0]).toContain("KBAZSG38");
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("绑定码已作废"));
    await vi.waitFor(() => expect(api.listBindings).toHaveBeenCalledTimes(2));

    ui.confirm.mockResolvedValueOnce("confirm");
    api.revokeCode.mockRejectedValueOnce(new Error("绑定码不存在或已经用掉"));
    voidBtn().click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("绑定码不存在或已经用掉"));
  });
});
