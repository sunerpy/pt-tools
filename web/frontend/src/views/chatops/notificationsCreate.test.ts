// @vitest-environment happy-dom
/*
 * 钉子：新建通知通道对话框。只出站的通道在新建时就显示全部字段（自建服务器地址、鉴权、「允许内网地址」），
 * 不用先按默认的公共服务器创建再补；换类型时清掉上一个类型填的值（钉钉与飞书都有 webhook_url）；
 * 提交时只发当前类型的字段；必填项与格式不对时不提交。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ list: vi.fn(), create: vi.fn(), logs: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    chatopsApi: {
      notifications: { list: api.list, create: api.create },
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

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);
function inputOf(id: string): HTMLInputElement {
  const el = q(id);
  if (!el) throw new Error(`找不到 ${id}`);
  return (el instanceof HTMLInputElement ? el : el.querySelector("input")) as HTMLInputElement;
}
function fill(id: string, v: string) {
  const input = inputOf(id);
  input.value = v;
  input.dispatchEvent(new Event("input"));
}
async function pickType(type: string) {
  const opt = q(`channel-type-${type}`);
  if (!opt) throw new Error(`找不到类型 ${type}`);
  opt.click();
  await flush();
  await flush();
}

async function openDialog() {
  api.list.mockResolvedValue([]);
  api.logs.mockResolvedValue({ items: [] });
  api.create.mockResolvedValue({ id: 1 });
  const Page = (await import("./Notifications.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(api.list).toHaveBeenCalled());
  q("add-channel-btn")!.click();
  await vi.waitFor(() => expect(q("name-input")).not.toBeNull());
}

describe("新建通知通道", () => {
  it("Bark：新建时就能填服务器地址与「允许内网地址」，只发 Bark 的字段", async () => {
    await openDialog();
    await pickType("bark");
    for (const id of [
      "new-device_key",
      "new-server_url",
      "new-allow_private",
      "new-group",
      "new-sound",
    ]) {
      expect(q(id), id).not.toBeNull();
    }
    fill("name-input", "家里的 Bark");
    fill("new-device_key", " dev-key ");
    fill("new-server_url", "http://192.168.1.5:8080");
    q("new-allow_private")!.click();
    await flush();
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toEqual({
      channel_type: "bark",
      name: "家里的 Bark",
      enabled: true,
      device_key: "dev-key",
      server_url: "http://192.168.1.5:8080",
      allow_private: true,
    });
  });

  it("换类型清掉上一个类型填的值：钉钉的 Webhook 地址不会带到飞书", async () => {
    await openDialog();
    fill("name-input", "群机器人");
    await pickType("dingtalk");
    fill("new-webhook_url", "https://oapi.dingtalk.com/robot/send?access_token=abc");
    await pickType("feishu");
    expect(inputOf("new-webhook_url").value).toBe("");
    expect(inputOf("name-input").value, "名称留着").toBe("群机器人");
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("请填写Webhook 地址"));
    expect(api.create).not.toHaveBeenCalled();

    // 换到 Bark 再换回钉钉：之前填的 device_key 也不会留着
    await pickType("bark");
    fill("new-device_key", "dev");
    await pickType("dingtalk");
    fill("new-webhook_url", "https://oapi.dingtalk.com/robot/send?access_token=abc");
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toEqual({
      channel_type: "dingtalk",
      name: "群机器人",
      enabled: true,
      webhook_url: "https://oapi.dingtalk.com/robot/send?access_token=abc",
    });
  });

  it("格式不对时不提交，并写明原因", async () => {
    await openDialog();
    fill("name-input", "ntfy");
    await pickType("ntfy");
    fill("new-topic", "a.b");
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("Topic 只能是字母"));
    expect(api.create).not.toHaveBeenCalled();
  });

  it("Telegram 只发 bot_token", async () => {
    await openDialog();
    fill("name-input", "tg");
    fill("bot-token-input", "123:abc");
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    expect(api.create.mock.calls[0]![0]).toEqual({
      channel_type: "telegram",
      name: "tg",
      enabled: true,
      bot_token: "123:abc",
    });
  });

  it("后端拒绝时提示里只有原因，不是整段 JSON", async () => {
    await openDialog();
    fill("name-input", "Bark");
    await pickType("bark");
    fill("new-device_key", "k");
    fill("new-server_url", "http://192.168.1.5");
    const detail = "通道配置无效: Bark 服务器地址不能用：192.168.1.5 是本机或内网地址";
    api.create.mockRejectedValue(new Error(JSON.stringify({ error: "invalid_argument", detail })));
    buttonByText("创建通道").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain(detail));
    expect(document.body.textContent).not.toContain("invalid_argument");
  });
});
