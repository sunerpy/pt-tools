import { describe, expect, it } from "vitest";

import {
  isNotifyCommand,
  notifyAuditArgs,
  notifyCommandLabel,
  notifyFailureText,
  notifySourceLabel,
} from "./notifySource";

describe("后台通知的投递在审计里的读法", () => {
  it("命令 notify:<来源>:<种类> 读成通知的叫法，别的命令原样", () => {
    expect(isNotifyCommand("notify:daily_report:daily")).toBe(true);
    expect(isNotifyCommand("help")).toBe(false);
    expect(isNotifyCommand(undefined)).toBe(false);
    expect(notifyCommandLabel("notify:daily_report:daily")).toBe("每日战报");
    expect(notifyCommandLabel("notify:attendance:daily")).toBe("签到");
    expect(notifyCommandLabel("notify:media:followshow_found")).toBe("追剧：下载了新的集");
    expect(notifyCommandLabel("notify:media:organized")).toBe("入库通知");
    expect(notifyCommandLabel("notify:login:tier")).toBe("保号提醒");
    expect(notifyCommandLabel("notify:something_new:x")).toBe("something_new");
    expect(notifyCommandLabel("pause")).toBe("pause");
    expect(notifySourceLabel("free_end")).toBe("免费到期");
    expect(notifyCommandLabel("notify:reseed:round")).toBe("辅种结果");
    expect(notifySourceLabel("reseed")).toBe("辅种");
  });

  it("失败原因带上第几次与什么时候再试；送达的没有", () => {
    const retry = new Date(2026, 9, 28, 22, 1).toISOString();
    expect(
      notifyFailureText({ error: "QQ 通道无可用收件人", attempt: 1, next_retry_at: retry }),
    ).toBe("QQ 通道无可用收件人（第 1 次，22:01 重试）");
    expect(notifyFailureText({ error: "boom", attempt: 4, final: true })).toBe(
      "boom（第 4 次，不再重试）",
    );
    expect(notifyFailureText({ error: "boom" })).toBe("boom");
    expect(notifyFailureText({ attempt: 1, title: "每日战报" })).toBe("");
  });

  it("参数读不出来时当作空", () => {
    expect(notifyAuditArgs("")).toEqual({});
    expect(notifyAuditArgs("not json")).toEqual({});
    expect(notifyAuditArgs("[1]")).toEqual({});
    expect(notifyAuditArgs('{"title":"每日战报","attempt":2}')).toEqual({
      title: "每日战报",
      attempt: 2,
    });
  });
});
