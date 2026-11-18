import { describe, expect, it } from "vitest";

import { activityText, maskUserId, notifyViaText, roleTexts } from "./bindingChannels";

describe("绑定页「通道」一栏的读法", () => {
  it("通知收件人从哪来、名单里的身份", () => {
    expect(notifyViaText("default_chat_id")).toBe("默认 Chat ID");
    expect(notifyViaText("default_chat_id", "feishu_app"), "飞书的设置项叫「默认会话 ID」").toBe(
      "默认会话 ID",
    );
    expect(notifyViaText("admin", "feishu_app")).toBe("第一个管理员");
    expect(notifyViaText("binding")).toBe("第一个绑定的账号");
    expect(notifyViaText(undefined)).toBe("");
    expect(roleTexts(["admin", "allowed"])).toEqual(["管理员", "允许"]);
    expect(roleTexts(undefined)).toEqual([]);
  });

  it("最近一次往来写命令、结果与时间", () => {
    const at = new Date(2026, 9, 21, 20, 57).toISOString();
    expect(activityText({ command: "unbind", result: "success", at })).toBe(
      "/unbind 成功 · 10-21 20:57",
    );
    expect(activityText({ command: "help", result: "denied:not_bound", at })).toBe(
      "/help 被拒：没有绑定 · 10-21 20:57",
    );
    expect(activityText({ command: "pause", result: "error:handler", at })).toBe(
      "/pause 出错 · 10-21 20:57",
    );
    expect(activityText({ command: "", result: "user_message_hinted", at: "bad" })).toBe(
      "发了消息",
    );
    expect(activityText(null)).toBe("");
  });

  it("用户 ID 只露头尾", () => {
    expect(maskUserId("1234567890")).toBe("12***7890");
    expect(maskUserId("555")).toBe("555");
    expect(maskUserId("")).toBe("-");
  });
});
