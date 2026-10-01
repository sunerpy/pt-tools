// @vitest-environment happy-dom
/*
 * 钉子：撤销绑定时，「用户取消」和「接口真的报错」必须分开。
 *
 * 原来确认框和删除请求包在同一个 try 里，catch 一律当成「用户取消」吞掉 ——
 * 接口回 404 / 500 时页面一声不吭，绑定还在，用户以为已经撤销了。
 * ElMessageBox 被取消或被关掉时 reject 的是动作名（'cancel' / 'close'），这两种才是静默的。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  listBindings: vi.fn(),
  deleteBinding: vi.fn(),
}));

const ui = vi.hoisted(() => ({
  confirm: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    chatopsApi: {
      bindings: { list: api.listBindings, delete: api.deleteBinding, update: vi.fn() },
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

const BINDING = {
  id: 42,
  channel_user_id: "123456789",
  channel_type: "telegram",
  conf_id: 1,
  reply_lang: "zh",
};

let view: MountedView | null = null;

async function mountPage() {
  api.listBindings.mockResolvedValue({ bindings: [BINDING], pending: [] });
  const Bindings = (await import("./Bindings.vue")).default;
  view = mountView(Bindings);
  await vi.waitFor(() =>
    expect(document.querySelector("[aria-label='撤销 12***6789 的绑定']")).not.toBeNull(),
  );
}

function clickRevoke() {
  document.querySelector<HTMLButtonElement>("[aria-label='撤销 12***6789 的绑定']")!.click();
}

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

describe("撤销绑定：取消与失败分开", () => {
  it("确认之后接口报错：把真实错误弹出来，而不是当成用户取消吞掉", async () => {
    await mountPage();
    ui.confirm.mockResolvedValue("confirm");
    api.deleteBinding.mockRejectedValue(new Error("绑定不存在或已被撤销"));

    clickRevoke();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("绑定不存在或已被撤销"));
    expect(ui.success).not.toHaveBeenCalled();
  });

  it.each(["cancel", "close"])("确认框被%s：不发请求，也不弹错误", async (action) => {
    await mountPage();
    ui.confirm.mockRejectedValue(action);

    clickRevoke();
    await vi.waitFor(() => expect(ui.confirm).toHaveBeenCalledTimes(1));
    await new Promise((r) => setTimeout(r, 20));
    expect(api.deleteBinding).not.toHaveBeenCalled();
    expect(ui.error).not.toHaveBeenCalled();
  });

  it("确认之后撤销成功：提示成功并重新拉列表", async () => {
    await mountPage();
    ui.confirm.mockResolvedValue("confirm");
    api.deleteBinding.mockResolvedValue(undefined);

    clickRevoke();
    await vi.waitFor(() => expect(ui.success).toHaveBeenCalledWith("绑定已撤销"));
    expect(api.deleteBinding).toHaveBeenCalledWith(42);
    await vi.waitFor(() => expect(api.listBindings).toHaveBeenCalledTimes(2));
    expect(ui.error).not.toHaveBeenCalled();
  });
});
