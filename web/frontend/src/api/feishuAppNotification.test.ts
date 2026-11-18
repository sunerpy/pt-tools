// @vitest-environment happy-dom
/*
 * 钉子：飞书应用机器人的配置经 chatopsApi.notifications 往返（packNotificationBody / unpackNotificationResponse，
 * 只认 NOTIFICATION_DYNAMIC_FIELDS 里的键）。别的页面测试都替换了 chatopsApi，走不到这里：表里漏了 app_secret，
 * 新建时 config_json 就没有它（后端回「App Secret 不能为空」），详情页读回来也没有，必填校验会拦住保存。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { chatopsApi, type NotificationConfig } from "@/api";

afterEach(() => {
  vi.unstubAllGlobals();
});

const CONFIG = {
  app_id: "cli_a1b2",
  app_secret: "sec-value",
  domain: "lark",
  admin_users: ["ou_admin", "ou_b"],
  allowed_users: ["ou_user"],
  default_chat_id: "oc_group",
  proxy_url: "socks5://127.0.0.1:1080",
};

function lastBody(fetchMock: ReturnType<typeof vi.fn>) {
  const init = (fetchMock.mock.calls.at(-1) as unknown as [string, RequestInit])[1];
  return JSON.parse(init.body as string);
}

describe("飞书应用机器人的配置经接口往返", () => {
  it("读回来：App ID、App Secret、平台与字符串数组的名单都在", async () => {
    const fetchMock = vi.fn(async () =>
      Response.json({
        id: 7,
        channel_type: "feishu_app",
        name: "飞书",
        enabled: true,
        config_json: CONFIG,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const conf = await chatopsApi.notifications.get(7);
    expect(conf).toMatchObject({ id: 7, channel_type: "feishu_app", ...CONFIG });
  });

  it("新建与保存：发出去的 config_json 里都在", async () => {
    const fetchMock = vi.fn(async () =>
      Response.json({
        id: 7,
        channel_type: "feishu_app",
        name: "飞书",
        enabled: true,
        config_json: CONFIG,
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    await chatopsApi.notifications.create({
      channel_type: "feishu_app",
      name: "飞书",
      enabled: true,
      ...(CONFIG as unknown as Partial<NotificationConfig>),
    });
    expect(lastBody(fetchMock)).toEqual({
      channel_type: "feishu_app",
      name: "飞书",
      enabled: true,
      config_json: CONFIG,
    });

    fetchMock.mockImplementation(async () => Response.json({ status: "ok" }));
    const patch = {
      app_id: "cli_new",
      app_secret: "s2",
      domain: "feishu",
      admin_users: ["ou_x"],
      allowed_users: [],
    };
    await chatopsApi.notifications.update(7, patch as unknown as Partial<NotificationConfig>);
    expect(lastBody(fetchMock).config_json).toEqual(patch);
  });
});
