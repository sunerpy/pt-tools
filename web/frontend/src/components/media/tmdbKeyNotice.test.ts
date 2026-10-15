// @vitest-environment happy-dom
/*
 * 钉子：没填 TMDB API Key 时，探索、订阅、媒体库页写明要用它做什么、去哪里填；填了或读不到设置时不提示。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";
import { createMemoryHistory, createRouter } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ settings: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, mediaApi: { ...real.mediaApi, settings: api.settings } };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));

async function mountNotice() {
  const Notice = (await import("./TmdbKeyNotice.vue")).default;
  const Host = defineComponent({ render: () => h(Notice, { usage: "探索与订阅" }) });
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: Host },
      { path: "/media/recognize", component: Host },
    ],
  });
  await router.push("/");
  view = mountView(Host, { router });
  await flush();
  await flush();
}

describe("TMDB API Key 的提示", () => {
  it("没填：写明要用它做什么，带去媒体识别的链接", async () => {
    api.settings.mockResolvedValue({ has_tmdb_key: false });
    await mountNotice();
    const note = document.querySelector("[data-testid=tmdb-key-notice]")!;
    expect(note.textContent).toContain("探索与订阅都要用它");
    expect(document.querySelector("[data-testid=tmdb-key-link]")!.getAttribute("href")).toBe(
      "/media/recognize",
    );
  });

  it("填了、或者读不到设置：不提示", async () => {
    api.settings.mockResolvedValue({ has_tmdb_key: true });
    await mountNotice();
    expect(document.querySelector("[data-testid=tmdb-key-notice]")).toBeNull();
    view?.unmount();
    api.settings.mockRejectedValue(new Error("x"));
    await mountNotice();
    expect(document.querySelector("[data-testid=tmdb-key-notice]")).toBeNull();
  });
});
