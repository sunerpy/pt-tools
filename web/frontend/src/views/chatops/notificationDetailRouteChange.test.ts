// @vitest-environment happy-dom
/*
 * 钉子：通道详情只改路由参数（/chatops/notifications/1 → /2）时要换成新通道。
 *
 * Vue Router 在只改参数时复用组件，原来只在 onMounted 加载：页面还是 1 号通道，
 * 保存按 conf.id 发给 1 号。就算重新加载，Object.assign 也只覆盖 2 号返回了的字段，
 * 2 号没配的（比如 proxy_url）会留着 1 号的值，保存凭证时一并写进 2 号。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router, RouterView } from "vue-router";
import { defineComponent, h } from "vue";

import { buttonByText, type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  get: vi.fn(),
  update: vi.fn(),
  test: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    chatopsApi: { notifications: { get: api.get, update: api.update, test: api.test } },
  };
});

/** chatopsApi.notifications.get 返回的是摊平后的对象：config_json 里的字段已经提到顶层 */
const CHANNELS: Record<number, Record<string, unknown>> = {
  1: {
    id: 1,
    channel_type: "telegram",
    name: "一号通道",
    enabled: true,
    bot_token: "token-one",
    proxy_url: "http://proxy-one:1080",
    admin_users: [1001],
    allowed_users: [],
    default_chat_id: 1001,
  },
  2: {
    id: 2,
    channel_type: "telegram",
    name: "二号通道",
    enabled: true,
    bot_token: "token-two",
    admin_users: [2002],
    allowed_users: [],
    default_chat_id: 2002,
  },
};

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

let view: MountedView | null = null;

async function mountDetail(path: string): Promise<Router> {
  const NotificationDetail = (await import("./NotificationDetail.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/chatops/notifications/:id", component: NotificationDetail },
      { path: "/chatops/notifications", component: { render: () => null } },
    ],
  });
  await router.push(path);
  await router.isReady();
  view = mountView(defineComponent({ render: () => h(RouterView) }), { router });
  return router;
}

const credBtn = () => buttonByText("保存凭证");

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

describe("通道详情：切换通道", () => {
  it("从 1 号切到 2 号：重新加载 2 号，保存凭证只发 2 号自己的配置", async () => {
    api.get.mockImplementation((id: number) => Promise.resolve(structuredClone(CHANNELS[id])));
    api.update.mockResolvedValue({ status: "ok" });
    const router = await mountDetail("/chatops/notifications/1");
    await vi.waitFor(() => expect(document.body.textContent).toContain("一号通道"));

    await router.push("/chatops/notifications/2");
    await vi.waitFor(() => expect(api.get).toHaveBeenCalledWith(2));
    await vi.waitFor(() => expect(credBtn().disabled).toBe(false));
    credBtn().click();

    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    const [id, payload] = api.update.mock.calls[0]!;
    expect(id).toBe(2);
    expect(payload.bot_token).toBe("token-two");
    expect(payload.proxy_url ?? "").toBe("");
  });

  it("2 号还在加载：保存凭证与发送测试都不放行", async () => {
    api.get.mockImplementation((id: number) =>
      id === 1 ? Promise.resolve(structuredClone(CHANNELS[1])) : new Promise(() => {}),
    );
    const router = await mountDetail("/chatops/notifications/1");
    await vi.waitFor(() => expect(document.body.textContent).toContain("一号通道"));

    await router.push("/chatops/notifications/2");
    await vi.waitFor(() => expect(api.get).toHaveBeenCalledWith(2));
    await vi.waitFor(() => expect(credBtn().disabled).toBe(true));
    credBtn().click();
    buttonByText("发送测试消息").click();
    await new Promise((r) => setTimeout(r, 0));
    expect(api.update).not.toHaveBeenCalled();
    expect(api.test).not.toHaveBeenCalled();
  });

  it("1 号的响应比 2 号晚回来：不会把页面换回 1 号", async () => {
    const late = deferred<Record<string, unknown>>();
    api.get.mockImplementation((id: number) =>
      id === 1 ? late.promise : Promise.resolve(structuredClone(CHANNELS[2])),
    );
    api.update.mockResolvedValue({ status: "ok" });
    const router = await mountDetail("/chatops/notifications/1");
    await vi.waitFor(() => expect(api.get).toHaveBeenCalledWith(1));

    await router.push("/chatops/notifications/2");
    await vi.waitFor(() => expect(credBtn().disabled).toBe(false));
    late.resolve(structuredClone(CHANNELS[1]));
    // 迟到的响应只经过微任务链：过一个宏任务边界，它该落地的都已经落地
    await new Promise((r) => setTimeout(r, 0));
    credBtn().click();

    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    const [id, payload] = api.update.mock.calls[0]!;
    expect(id).toBe(2);
    expect(payload.bot_token).toBe("token-two");
  });
});
