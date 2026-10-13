import { describe, expect, it } from "vitest";

import { CookieCloudError } from "./client";
import { cookieCloudView, mergeCookieCloudInput } from "./store";
import { DEFAULT_COOKIECLOUD_CONFIG } from "./upload";

const CURRENT = {
  ...DEFAULT_COOKIECLOUD_CONFIG,
  serverUrl: "https://cc.example",
  uuid: "u",
  password: "old",
};

describe("CookieCloud 设置", () => {
  it("不传密码时保留原来的；传了就换；加密方式只认两种", () => {
    expect(
      mergeCookieCloudInput(
        { enabled: true, serverUrl: " https://cc.example ", uuid: " u " },
        CURRENT,
      ),
    ).toEqual({
      enabled: true,
      serverUrl: "https://cc.example",
      uuid: "u",
      password: "old",
      cryptoType: "aes-128-cbc-fixed",
    });
    expect(
      mergeCookieCloudInput(
        { serverUrl: "https://cc.example", uuid: "u", password: "new", cryptoType: "legacy" },
        CURRENT,
      ),
    ).toMatchObject({
      password: "new",
      cryptoType: "legacy",
      enabled: false,
    });
    expect(mergeCookieCloudInput({ cryptoType: "rot13" }, CURRENT).cryptoType).toBe(
      "aes-128-cbc-fixed",
    );
  });

  it("打开上传时三项都要有；地址不对、太长、格式不对时报错", () => {
    const bad: unknown[] = [
      { enabled: true, serverUrl: "", uuid: "u" },
      { enabled: true, serverUrl: "https://cc.example", uuid: "" },
      { serverUrl: "ftp://cc.example" },
      { serverUrl: `https://cc.example/${"a".repeat(600)}` },
      { password: 123 },
      null,
    ];
    for (const raw of bad) {
      expect(
        () => mergeCookieCloudInput(raw, raw === null ? CURRENT : { ...CURRENT, password: "" }),
        JSON.stringify(raw),
      ).toThrow(CookieCloudError);
    }
  });

  it("给弹窗看的设置不带密码", () => {
    const view = cookieCloudView(CURRENT, null);
    expect(view.hasPassword).toBe(true);
    expect(JSON.stringify(view)).not.toContain("old");
    expect(cookieCloudView({ ...CURRENT, password: "" }, null).hasPassword).toBe(false);
  });
});
