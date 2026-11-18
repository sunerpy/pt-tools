import { describe, expect, it } from "vitest";

import { isLoopbackListenAddr } from "./listenAddr";

describe("isLoopbackListenAddr", () => {
  it("本机地址", () => {
    for (const addr of [
      "127.0.0.1:6701",
      "127.1.2.3:80",
      "localhost:6701",
      "LOCALHOST:1",
      "[::1]:6701",
    ]) {
      expect(isLoopbackListenAddr(addr), addr).toBe(true);
    }
  });

  it("监听所有网卡或外部地址都不算本机", () => {
    for (const addr of [
      "0.0.0.0:6701",
      ":6701",
      "[::]:6701",
      "192.168.1.2:6701",
      "napcat:6701",
      "",
      "127.0.0.1",
    ]) {
      expect(isLoopbackListenAddr(addr), addr).toBe(false);
    }
  });
});
