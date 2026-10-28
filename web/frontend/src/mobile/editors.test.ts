// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";
import { mountView, type MountedView } from "@/test-mount";
const api = vi.hoisted(() => ({
  request: vi.fn(),
  notify: vi.fn(),
  options: vi.fn(),
  subscription: vi.fn(),
}));
vi.mock("./bridge", async (original) => ({
  ...(await original<typeof import("./bridge")>()),
  request: api.request,
  notify: api.notify,
}));
vi.mock("./api", () => ({ mobileApi: { options: api.options, subscription: api.subscription } }));
let view: MountedView | undefined;
afterEach(() => {
  view?.unmount();
  view = undefined;
  document.body.innerHTML = "";
  vi.clearAllMocks();
});
beforeEach(() => {
  api.options.mockResolvedValue({
    sites: [{ name: "mteam", display_name: "M-Team" }],
    downloaders: [{ id: 1, name: "qB" }],
    profiles: [],
    libraries: [],
  });
  api.subscription.mockResolvedValue({
    title: "Test series",
    media_type: "tv",
    tmdb_id: 42,
    season: 2,
    include_keywords: "WEB\nCHD",
    exclude_keywords: "DV",
    sites: ["mteam"],
    schedule: { kind: "weekly", at: "21:30", days: [2] },
    lock_version: true,
  });
});
function submit() {
  document
    .querySelector("form")!
    .dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
}
describe("mobile editors", () => {
  it("edits subscription without dropping existing site filters and schedule", async () => {
    api.request.mockResolvedValue({ id: 7 });
    const Editor = (await import("./SubscriptionEditor.vue")).default;
    view = mountView(defineComponent({ setup: () => () => h(Editor, { id: 7, canWrite: true }) }));
    await vi.waitFor(() => expect(document.querySelector("form")).not.toBeNull());
    submit();
    await vi.waitFor(() => expect(api.request).toHaveBeenCalledTimes(1));
    expect(api.request.mock.calls[0]).toMatchObject([
      "/api/app/v1/subscriptions/7",
      "PUT",
      {
        include_keywords: "WEB\nCHD",
        exclude_keywords: "DV",
        sites: ["mteam"],
        schedule: { kind: "weekly", at: "21:30", days: [2] },
      },
    ]);
    expect(api.notify).toHaveBeenCalledWith("close", { changed: true });
  });
  it("keeps edits visible when saving fails", async () => {
    api.request.mockRejectedValue(new Error("保存失败"));
    const Editor = (await import("./SubscriptionEditor.vue")).default;
    view = mountView(defineComponent({ setup: () => () => h(Editor, { id: 7, canWrite: true }) }));
    await vi.waitFor(() => expect(document.querySelector("form")).not.toBeNull());
    submit();
    await vi.waitFor(() =>
      expect(document.querySelector('[role="alert"]')?.textContent).toContain("保存失败"),
    );
    expect(document.querySelectorAll("textarea")[0]?.value).toBe("WEB\nCHD");
    expect(api.notify).not.toHaveBeenCalledWith("close", expect.anything());
  });
  it("does not submit from a read-only device", async () => {
    const Editor = (await import("./SubscriptionEditor.vue")).default;
    view = mountView(defineComponent({ setup: () => () => h(Editor, { id: 7, canWrite: false }) }));
    await vi.waitFor(() => expect(document.querySelector("form")).not.toBeNull());
    submit();
    expect(api.request).not.toHaveBeenCalled();
  });
  it("requires brush change preview before committing", async () => {
    api.request.mockImplementation(async (path, verb = "GET") => {
      if (verb === "GET") return { name: "Task", site_name: "mteam", downloader_id: 1 };
      if (path.endsWith("preview")) return { summary: "变更任务", changes: [], impact: [] };
      return { id: 9 };
    });
    const Editor = (await import("./BrushEditor.vue")).default;
    view = mountView(defineComponent({ setup: () => () => h(Editor, { id: 9, canWrite: true }) }));
    await vi.waitFor(() => expect(document.querySelector("form")).not.toBeNull());
    submit();
    await vi.waitFor(() => expect(document.body.textContent).toContain("确认保存"));
    expect(api.request).toHaveBeenCalledWith(
      "/api/app/v1/brush/tasks/preview",
      "POST",
      expect.objectContaining({ op: "update", id: "9" }),
    );
    expect(api.notify).not.toHaveBeenCalledWith("close", expect.anything());
    submit();
    await vi.waitFor(() =>
      expect(api.request).toHaveBeenCalledWith(
        "/api/app/v1/brush/tasks/9",
        "PUT",
        expect.objectContaining({ name: "Task" }),
      ),
    );
    expect(api.notify).toHaveBeenCalledWith("close", { changed: true });
  });
});
