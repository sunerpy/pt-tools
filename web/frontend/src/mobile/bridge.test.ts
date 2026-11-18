// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from "vitest";
import { bridgeCall, cancelPending, copyText, request } from "./bridge";

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

// 追剧方案 F3（3.3.3）：试搜索可以单独取消，不动表单与别的请求；复制走原生的 copy 消息。
describe("mobile bridge: one request can be cancelled", () => {
  const sent = (postMessage: ReturnType<typeof vi.fn>) =>
    postMessage.mock.calls.map((c) => JSON.parse(c[0] as string) as Record<string, unknown>);

  it("cancels only the aborted request: one cancel with its id, 499, the other still completes", async () => {
    const postMessage = vi.fn();
    window.PtMobile = { postMessage };
    const abort = new AbortController();
    const preview = request(
      "/api/app/v1/subscriptions/search/preview",
      "POST",
      { tmdb_id: 1 },
      { signal: abort.signal },
    );
    const options = request<{ ok: boolean }>("/api/app/v1/mobile/options");
    const [first, second] = sent(postMessage);
    abort.abort();
    await expect(preview).rejects.toMatchObject({ status: 499, message: "Cancelled / 已取消" });
    const cancels = sent(postMessage).filter((m) => m.method === "cancel");
    expect(cancels).toEqual([expect.objectContaining({ requestId: first!.id })]);
    // the late reply of the cancelled request is ignored; the other one still resolves
    window.ptMobileReceive!({ id: first!.id as string, ok: true, data: { late: true } });
    window.ptMobileReceive!({ id: second!.id as string, ok: true, data: { ok: true } });
    await expect(options).resolves.toEqual({ ok: true });
  });

  it("aborting after the reply arrived sends nothing", async () => {
    const postMessage = vi.fn();
    window.PtMobile = { postMessage };
    const abort = new AbortController();
    const pending = request<{ id: number }>("/api/app/v1/meta", "GET", undefined, {
      signal: abort.signal,
    });
    const [first] = sent(postMessage);
    window.ptMobileReceive!({ id: first!.id as string, ok: true, data: { id: 1 } });
    await expect(pending).resolves.toEqual({ id: 1 });
    abort.abort();
    expect(sent(postMessage).filter((m) => m.method === "cancel")).toEqual([]);
  });

  it("an already aborted signal rejects without asking native", async () => {
    const postMessage = vi.fn();
    window.PtMobile = { postMessage };
    const abort = new AbortController();
    abort.abort();
    await expect(
      request("/api/app/v1/meta", "GET", undefined, { signal: abort.signal }),
    ).rejects.toMatchObject({ status: 499 });
    expect(postMessage).not.toHaveBeenCalled();
  });

  it("cancelPending still cancels every pending request as page closed", async () => {
    const postMessage = vi.fn();
    window.PtMobile = { postMessage };
    const abort = new AbortController();
    const a = request("/api/app/v1/meta", "GET", undefined, { signal: abort.signal });
    const b = request("/api/app/v1/mobile/options");
    const ids = sent(postMessage).map((m) => m.id);
    cancelPending();
    await expect(a).rejects.toThrow("Page closed");
    await expect(b).rejects.toThrow("Page closed");
    expect(
      sent(postMessage)
        .filter((m) => m.method === "cancel")
        .map((m) => m.requestId),
    ).toEqual(ids);
    // the closed request's signal no longer does anything
    abort.abort();
    expect(sent(postMessage).filter((m) => m.method === "cancel")).toHaveLength(2);
  });

  it("copies through the native copy message", async () => {
    const postMessage = vi.fn();
    window.PtMobile = { postMessage };
    const done = copyText("Dream.S01E02.1080p");
    const [msg] = sent(postMessage);
    expect(msg).toMatchObject({ method: "copy", text: "Dream.S01E02.1080p" });
    window.ptMobileReceive!({ id: msg!.id as string, ok: true, data: { copied: true } });
    await expect(done).resolves.toEqual({ copied: true });
  });
});
