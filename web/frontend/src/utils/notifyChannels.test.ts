import { describe, expect, it } from "vitest";

import { OUTBOUND_CHANNELS, missingRequired, outboundChannel } from "./notifyChannels";

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

  it("finds the first missing required field", () => {
    expect(missingRequired("bark", {})?.key).toBe("device_key");
    expect(missingRequired("bark", { device_key: " " })?.key).toBe("device_key");
    expect(missingRequired("bark", { device_key: "k" })).toBeUndefined();
    expect(missingRequired("ntfy", { topic: "t" })).toBeUndefined();
    expect(missingRequired("telegram", {})).toBeUndefined();
  });
});
