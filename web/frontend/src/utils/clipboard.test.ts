// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";

import { copyText } from "./clipboard";

/* 钉子：复制在没有 Clipboard API 的环境（局域网地址用 HTTP 打开）回退到 textarea + execCommand("copy")。 */
const setSecure = (v: boolean) =>
  Object.defineProperty(window, "isSecureContext", { value: v, configurable: true });

afterEach(() => {
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});

describe("copyText", () => {
  it("安全环境用 Clipboard API", async () => {
    setSecure(true);
    const write = vi.fn(async () => {});
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText: write },
      configurable: true,
    });
    expect(await copyText("标题")).toBe(true);
    expect(write).toHaveBeenCalledWith("标题");
  });

  it("不是安全环境：放进焦点所在的对话框里用 execCommand 复制，复制完拿掉、焦点还回去", async () => {
    setSecure(false);
    document.body.innerHTML = '<div role="dialog"><button id="b">x</button></div>';
    const button = document.getElementById("b")!;
    button.focus();
    let host: Element | null = null;
    let value = "";
    const exec = vi.fn((cmd: string) => {
      const area = document.querySelector("textarea")!;
      host = area.parentElement;
      value = area.value;
      return cmd === "copy";
    });
    Object.defineProperty(document, "execCommand", { value: exec, configurable: true });
    expect(await copyText("The.Show.S02")).toBe(true);
    expect(exec).toHaveBeenCalledWith("copy");
    expect(value).toBe("The.Show.S02");
    expect((host as Element | null)?.getAttribute("role")).toBe("dialog");
    expect(document.querySelector("textarea")).toBeNull();
    expect(document.activeElement).toBe(button);
  });

  it("Clipboard API 被拒时回退；回退也失败时返回 false", async () => {
    setSecure(true);
    Object.defineProperty(navigator, "clipboard", {
      value: { writeText: vi.fn(async () => Promise.reject(new Error("denied"))) },
      configurable: true,
    });
    Object.defineProperty(document, "execCommand", {
      value: vi.fn(() => false),
      configurable: true,
    });
    expect(await copyText("x")).toBe(false);
    Object.defineProperty(document, "execCommand", {
      value: vi.fn(() => {
        throw new Error("no");
      }),
      configurable: true,
    });
    expect(await copyText("x")).toBe(false);
    expect(document.querySelector("textarea")).toBeNull();
  });
});
