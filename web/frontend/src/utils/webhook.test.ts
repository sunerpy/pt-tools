// @vitest-environment happy-dom
/*
 * 钉子：Webhook 的请求头在编辑框里是 JSON 文本、接口里是对象（路线图 M21）。原来前端把它当文字存取：
 * 打包时原样发字符串（后端要的是对象，通道起不来），解包时对象被丢掉（再保存就把原来的请求头覆盖掉）。
 * 现在读回来的对象写成格式化的 JSON 文本，保存时解析成对象；预设的结构也在这里核对。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { chatopsApi } from "@/api";

import {
  WEBHOOK_FORMATS,
  WEBHOOK_METHODS,
  WEBHOOK_PRESETS,
  headersToText,
  parseHeadersText,
} from "./webhook";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("请求头", () => {
  it("编辑框里的 JSON 文本解析成对象；空着是空对象；不是「文字对文字」的对象就不行", () => {
    expect(parseHeadersText("")).toEqual({ ok: true, value: {} });
    expect(parseHeadersText(undefined)).toEqual({ ok: true, value: {} });
    expect(parseHeadersText(' {"Authorization": "Bearer x"} ')).toEqual({
      ok: true,
      value: { Authorization: "Bearer x" },
    });
    for (const bad of ["nope", "[1]", '"x"', '{"A": 1}', '{" ": "x"}', "null"]) {
      expect(parseHeadersText(bad).ok, bad).toBe(false);
    }
  });

  it("读回来的对象写成格式化的 JSON 文本；旧版存的 JSON 文本原样；空的是空串", () => {
    expect(headersToText({ "X-A": "1" })).toBe('{\n  "X-A": "1"\n}');
    expect(headersToText('{"X-A":"1"}')).toBe('{"X-A":"1"}');
    expect(headersToText({})).toBe("");
    expect(headersToText(undefined)).toBe("");
  });

  it("经接口往返：读回来是文本，保存时发对象", async () => {
    const fetchMock = vi.fn(async () =>
      Response.json({
        id: 7,
        channel_type: "webhook",
        name: "钩子",
        enabled: true,
        config_json: {
          endpoint_url: "https://example.com/hook",
          headers: { "X-A": "1" },
          method: "PUT",
          content_type: "text",
          body_template: "{{.title}}",
        },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    const conf = await chatopsApi.notifications.get(7);
    expect(conf.headers).toBe('{\n  "X-A": "1"\n}');
    expect(conf).toMatchObject({
      method: "PUT",
      content_type: "text",
      body_template: "{{.title}}",
    });

    fetchMock.mockImplementation(async () => Response.json({ status: "ok" }));
    await chatopsApi.notifications.update(7, { headers: '{"X-A": "2"}', body_template: "x" });
    const init = (fetchMock.mock.calls.at(-1) as unknown as [string, RequestInit])[1];
    expect(JSON.parse(init.body as string).config_json).toEqual({
      headers: { "X-A": "2" },
      body_template: "x",
    });
    // 清空等于清掉原来的请求头
    await chatopsApi.notifications.update(7, { headers: "" });
    const cleared = (fetchMock.mock.calls.at(-1) as unknown as [string, RequestInit])[1];
    expect(JSON.parse(cleared.body as string).config_json).toEqual({ headers: {} });
  });
});

describe("预设", () => {
  it("每个预设的方法与格式都是界面上能选的，键不重复", () => {
    const methods: readonly string[] = WEBHOOK_METHODS;
    const formats = WEBHOOK_FORMATS.map((f) => f.value as string);
    const keys = new Set<string>();
    for (const p of WEBHOOK_PRESETS) {
      expect(methods, p.key).toContain(p.method);
      expect(formats, p.key).toContain(p.content_type);
      expect(p.label, p.key).not.toBe("");
      expect(keys.has(p.key), p.key).toBe(false);
      keys.add(p.key);
    }
    // 只有「原样 JSON」不写请求体：留空发固定 JSON
    expect(WEBHOOK_PRESETS.filter((p) => p.body_template === "").map((p) => p.key)).toEqual([
      "raw",
    ]);
  });
});
