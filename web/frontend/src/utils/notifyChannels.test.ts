import { describe, expect, it } from "vitest";

import {
  OUTBOUND_CHANNELS,
  apiErrorDetail,
  outboundChannel,
  outboundFieldKeys,
  outboundProblem,
} from "./notifyChannels";

describe("notifyChannels", () => {
  it("lists the outbound channels with one required field each", () => {
    expect(OUTBOUND_CHANNELS.map((c) => c.type)).toEqual([
      "bark",
      "serverchan",
      "ntfy",
      "dingtalk",
      "feishu",
    ]);
    for (const c of OUTBOUND_CHANNELS) {
      expect(c.fields.filter((f) => f.required).length).toBe(1);
    }
    expect(outboundChannel("telegram")).toBeUndefined();
  });

  it("自建服务器的通道有「允许内网地址」开关，其余没有", () => {
    expect(outboundFieldKeys("bark")).toContain("allow_private");
    expect(outboundFieldKeys("ntfy")).toContain("allow_private");
    expect(outboundFieldKeys("dingtalk")).not.toContain("allow_private");
    expect(outboundFieldKeys("telegram")).toEqual([]);
  });

  it("finds the first missing required field", () => {
    expect(outboundProblem("bark", {})).toBe("请填写Device Key");
    expect(outboundProblem("bark", { device_key: " " })).toBe("请填写Device Key");
    expect(outboundProblem("bark", { device_key: "k" })).toBeUndefined();
    expect(outboundProblem("ntfy", { topic: "t" })).toBeUndefined();
    expect(outboundProblem("telegram", {})).toBeUndefined();
  });

  it("按与后端相同的规则查格式", () => {
    expect(outboundProblem("ntfy", { topic: "a.b" })).toContain("Topic 只能是");
    expect(outboundProblem("ntfy", { topic: "t", server_url: "ftp://x" })).toContain("http://");
    expect(outboundProblem("ntfy", { topic: "t", server_url: "https://u:p@x" })).toContain(
      "用户名密码",
    );
    expect(outboundProblem("ntfy", { topic: "t", server_url: "http://192.168.1.2:8080/" })).toBe(
      undefined,
    );
    expect(outboundProblem("bark", { device_key: "k", server_url: "https://x/?a=1" })).toContain(
      "?",
    );
    expect(outboundProblem("serverchan", { send_key: "a/b" })).toBe("SendKey 格式不对");

    const ding = "https://oapi.dingtalk.com/robot/send?access_token=abc";
    expect(outboundProblem("dingtalk", { webhook_url: ding })).toBeUndefined();
    expect(
      outboundProblem("dingtalk", { webhook_url: "https://example.com/robot/send?access_token=a" }),
    ).toContain("oapi.dingtalk.com");
    expect(
      outboundProblem("dingtalk", { webhook_url: "https://oapi.dingtalk.com/robot/send" }),
    ).toContain("access_token");

    expect(
      outboundProblem("feishu", { webhook_url: "https://open.feishu.cn/open-apis/bot/v2/hook/x" }),
    ).toBe(undefined);
    expect(
      outboundProblem("feishu", {
        webhook_url: "https://open.larksuite.com/open-apis/bot/v2/hook/x",
      }),
    ).toBe(undefined);
    expect(outboundProblem("feishu", { webhook_url: ding })).toContain("open.feishu.cn");
  });

  it("接口错误取 detail，不是 JSON 时用原文", () => {
    const body = JSON.stringify({
      error: "invalid_argument",
      detail: "通道配置无效: ntfy topic …",
    });
    expect(apiErrorDetail(new Error(body), "添加失败")).toBe("通道配置无效: ntfy topic …");
    expect(apiErrorDetail(new Error(JSON.stringify({ error: "boom" })), "x")).toBe("boom");
    expect(apiErrorDetail(new Error("HTTP 502"), "x")).toBe("HTTP 502");
    expect(apiErrorDetail(new Error(""), "添加失败")).toBe("添加失败");
    expect(apiErrorDetail(undefined, "添加失败")).toBe("添加失败");
    expect(apiErrorDetail(new Error("null"), "x")).toBe("null");
  });
});
