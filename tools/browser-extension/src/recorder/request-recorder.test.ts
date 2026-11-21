import { describe, expect, it } from "vitest";

import { RECORDER_GLOBAL } from "./constants";
import {
  installRequestRecorder,
  type RecorderEntry,
  type RecorderStatus,
} from "./request-recorder";

/** 假的 XMLHttpRequest：send 之后下一个微任务里完成。每个假窗口一个类，免得几次安装叠在同一个原型上 */
const makeXhrClass = () =>
  class FakeXhr {
    status = 0;
    responseType = "";
    responseText = "";
    response: unknown = null;
    private listeners: Array<() => void> = [];
    private sent = false;
    open(_method: string, _url: string): void {}
    send(_body?: unknown): void {
      this.sent = true;
      queueMicrotask(() => {
        this.status = 200;
        this.responseText = `{"status":1}`;
        for (const listener of this.listeners) listener();
      });
    }
    addEventListener(type: string, listener: () => void): void {
      if (type === "loadend") this.listeners.push(listener);
    }
    getResponseHeader(name: string): string | null {
      return name.toLowerCase() === "content-type" ? "application/json" : null;
    }
    get wasSent(): boolean {
      return this.sent;
    }
  };

function fakeWindow() {
  const fetched: string[] = [];
  const win = {
    location: { href: "https://pt.example/torrents.php", origin: "https://pt.example" },
    XMLHttpRequest: makeXhrClass(),
    fetch: async (input: string) => {
      fetched.push(String(input));
      return new Response("<table>ok</table>", { headers: { "content-type": "text/html" } });
    },
    Request,
    URLSearchParams,
    FormData,
  };
  installRequestRecorder(win as unknown as Window & typeof globalThis);
  const recorder = (win as unknown as Record<string, unknown>)[RECORDER_GLOBAL] as {
    read: () => RecorderEntry[];
    status: () => RecorderStatus;
  };
  return { win: win as unknown as typeof win & { fetch: typeof fetch }, recorder, fetched };
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

describe("installRequestRecorder", () => {
  it("记同源 XHR 的方法、地址、参数、状态与回应", async () => {
    const { win, recorder } = fakeWindow();
    const xhr = new win.XMLHttpRequest();
    xhr.open("post", "/ajax_promotion.php");
    xhr.send("csrf=TOKEN&ids%5B%5D=1");
    expect(recorder.status().inflight).toBe(1);
    await settle();
    expect(recorder.read()).toEqual([
      expect.objectContaining({
        kind: "xhr",
        method: "POST",
        url: "https://pt.example/ajax_promotion.php",
        requestBody: "csrf=TOKEN&ids%5B%5D=1",
        status: 200,
        contentType: "application/json",
        responseBody: `{"status":1}`,
      }),
    ]);
    expect(recorder.status().inflight).toBe(0);
    expect(xhr.wasSent).toBe(true);
  });

  it("别的网站的请求照发、不记", async () => {
    const { win, recorder, fetched } = fakeWindow();
    const xhr = new win.XMLHttpRequest();
    xhr.open("GET", "https://cdn.other.example/x.json");
    xhr.send();
    await win.fetch("https://stats.other.example/hit");
    await settle();
    expect(recorder.read()).toEqual([]);
    expect(fetched).toEqual(["https://stats.other.example/hit"]);
  });

  it("记同源 fetch，表单参数转成字符串，回应正文另读不影响页面", async () => {
    const { win, recorder } = fakeWindow();
    const body = new URLSearchParams({ userid: "9", type: "seeding" });
    const response = await win.fetch("/ajax_getusertorrentlist.php", { method: "POST", body });
    expect(await response.text()).toBe("<table>ok</table>");
    await settle();
    await settle();
    expect(recorder.read()[0]).toMatchObject({
      kind: "fetch",
      method: "POST",
      url: "https://pt.example/ajax_getusertorrentlist.php",
      requestBody: "userid=9&type=seeding",
      status: 200,
      responseBody: "<table>ok</table>",
    });
  });

  it("装两次只装一份，页面改不了记录器", () => {
    const { win, recorder } = fakeWindow();
    installRequestRecorder(win as unknown as Window & typeof globalThis);
    const host = win as unknown as Record<string, unknown>;
    expect(host[RECORDER_GLOBAL]).toBe(recorder);
    expect(() => {
      host[RECORDER_GLOBAL] = { read: () => ["fake"] };
    }).toThrow();
  });
});
