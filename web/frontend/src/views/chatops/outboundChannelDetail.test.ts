// @vitest-environment happy-dom
/*
 * 钉子：只出站的通道（这里用钉钉）在详情页按 utils/notifyChannels 的字段渲染：读回来的值回显，
 * 保存凭证时连同新填的一起发出；必填项为空、格式不对时不保存；开关按布尔值发出。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ get: vi.fn(), update: vi.fn(), test: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    chatopsApi: { notifications: { get: api.get, update: api.update, test: api.test } },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

async function mountDetail() {
  const NotificationDetail = (await import("./NotificationDetail.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/chatops/notifications/:id", component: NotificationDetail }],
  });
  await router.push("/chatops/notifications/7");
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
}

const input = (id: string) => {
  const el = document.querySelector<HTMLElement>(`[data-testid=${id}]`);
  if (!el) throw new Error(`找不到 ${id}`);
  return (el instanceof HTMLInputElement ? el : el.querySelector("input")) as HTMLInputElement;
};

describe("只出站通道的详情", () => {
  it("钉钉：回显并保存 Webhook 地址、加签密钥与消息格式", async () => {
    api.get.mockResolvedValue({
      id: 7,
      channel_type: "dingtalk",
      name: "钉钉群",
      enabled: true,
      webhook_url: "https://oapi.dingtalk.com/robot/send?access_token=abc",
      msg_type: "text",
    });
    api.update.mockResolvedValue({ status: "ok" });
    await mountDetail();
    await vi.waitFor(() => expect(document.body.textContent).toContain("钉钉群"));
    await vi.waitFor(() => expect(input("cred-webhook_url").value).toContain("access_token=abc"));
    expect(document.body.textContent).toContain("加签密钥");

    const secret = input("cred-secret");
    secret.value = "SECxyz";
    secret.dispatchEvent(new Event("input"));
    await vi.waitFor(() => expect(buttonByText("保存凭证").disabled).toBe(false));
    buttonByText("保存凭证").click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    const [id, payload] = api.update.mock.calls[0]!;
    expect(id).toBe(7);
    expect(payload).toMatchObject({
      webhook_url: "https://oapi.dingtalk.com/robot/send?access_token=abc",
      secret: "SECxyz",
      msg_type: "text",
    });
  });

  it("必填的 Webhook 地址为空时不保存", async () => {
    api.get.mockResolvedValue({ id: 7, channel_type: "feishu", name: "飞书群", enabled: true });
    await mountDetail();
    await vi.waitFor(() => expect(document.body.textContent).toContain("飞书群"));
    await vi.waitFor(() => expect(buttonByText("保存凭证").disabled).toBe(false));
    buttonByText("保存凭证").click();
    await new Promise((r) => setTimeout(r, 50));
    expect(api.update).not.toHaveBeenCalled();
  });

  it("飞书：Webhook 地址不是飞书的地址时不保存，并写明原因", async () => {
    api.get.mockResolvedValue({
      id: 7,
      channel_type: "feishu",
      name: "飞书群",
      enabled: true,
      webhook_url: "https://oapi.dingtalk.com/robot/send?access_token=abc",
    });
    await mountDetail();
    await vi.waitFor(() => expect(input("cred-webhook_url").value).toContain("dingtalk"));
    await vi.waitFor(() => expect(buttonByText("保存凭证").disabled).toBe(false));
    buttonByText("保存凭证").click();
    await vi.waitFor(() =>
      expect(document.body.textContent).toContain("飞书 Webhook 地址要是 https://open.feishu.cn"),
    );
    expect(api.update).not.toHaveBeenCalled();
  });

  it("Bark：「允许内网地址」回显并按布尔值保存", async () => {
    api.get.mockResolvedValue({
      id: 7,
      channel_type: "bark",
      name: "Bark",
      enabled: true,
      device_key: "dev",
      server_url: "http://192.168.1.5:8080",
      allow_private: true,
    });
    api.update.mockResolvedValue({ status: "ok" });
    await mountDetail();
    await vi.waitFor(() => expect(input("cred-server_url").value).toBe("http://192.168.1.5:8080"));
    const sw = document.querySelector<HTMLElement>("[data-testid=cred-allow_private]")!;
    expect(sw.classList.contains("is-checked")).toBe(true);
    await vi.waitFor(() => expect(buttonByText("保存凭证").disabled).toBe(false));
    buttonByText("保存凭证").click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    expect(api.update.mock.calls[0]![1]).toMatchObject({
      device_key: "dev",
      server_url: "http://192.168.1.5:8080",
      allow_private: true,
    });

    sw.click();
    await vi.waitFor(() => expect(sw.classList.contains("is-checked")).toBe(false));
    buttonByText("保存凭证").click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(2));
    expect(api.update.mock.calls[1]![1]).toMatchObject({ allow_private: false });
  });
});
