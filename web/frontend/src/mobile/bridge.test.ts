// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { bridgeCall, cancelPending, request } from "./bridge";

afterEach(() => {
  cancelPending();
  delete window.PtMobile;
  vi.useRealTimers();
});
describe("mobile bridge", () => {
  it("returns native JSON responses and preserves POST body", async () => {
    const postMessage = vi.fn();
    window.PtMobile = { postMessage };
    const pending = request<{ id: number }>("/api/app/v1/subscriptions", "POST", { tmdb_id: 123 });
    const sent = JSON.parse(postMessage.mock.calls[0]![0]);
    expect(sent).toMatchObject({ method: "request", verb: "POST", body: { tmdb_id: 123 } });
    window.ptMobileReceive!({ id: sent.id, ok: true, data: { id: 7 } });
    await expect(pending).resolves.toEqual({ id: 7 });
  });
  it("rejects non-App API requests before native dispatch", async () => {
    window.PtMobile = { postMessage: vi.fn() };
    await expect(request("/api/sites")).rejects.toThrow();
    await expect(request("/api/app/v1/../sites")).rejects.toThrow();
    expect(window.PtMobile.postMessage).not.toHaveBeenCalled();
  });
  it("cancels native request on timeout and rejects late callbacks", async () => {
    vi.useFakeTimers();
    const postMessage = vi.fn();
    window.PtMobile = { postMessage };
    const pending = bridgeCall("request", { path: "/api/app/v1/mobile/options", verb: "GET" }, 25);
    const failed = expect(pending).rejects.toThrow("timed out");
    await vi.advanceTimersByTimeAsync(26);
    await failed;
    expect(JSON.parse(postMessage.mock.calls.at(-1)![0])).toMatchObject({ method: "cancel" });
  });
  it("propagates permission failure without treating it as empty", async () => {
    const postMessage = vi.fn();
    window.PtMobile = { postMessage };
    const pending = request("/api/app/v1/mobile/options");
    const sent = JSON.parse(postMessage.mock.calls[0]![0]);
    window.ptMobileReceive!({
      id: sent.id,
      ok: false,
      error: { status: 403, message: "forbidden" },
    });
    await expect(pending).rejects.toMatchObject({ status: 403, message: "forbidden" });
  });
});
