// @vitest-environment happy-dom
/*
 * 钉子：没有能用的 TMDB API Key 时（没填、这个版本也没有内置的，或者换了接口地址用不了内置的），探索、订阅、
 * 媒体库页写明要用它做什么、去哪里填；用着自己的或内置的 Key、或读不到设置时不提示。
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
  it("没填、也没有内置的：写明要用它做什么，带去媒体识别的链接", async () => {
    api.settings.mockResolvedValue({ has_tmdb_key: false, key_source: "none", builtin_key: false });
    await mountNotice();
    const note = document.querySelector("[data-testid=tmdb-key-notice]")!;
    expect(note.textContent).toContain("没有填写 TMDB API Key");
    expect(note.textContent).toContain("探索与订阅都要用它");
    expect(document.querySelector("[data-testid=tmdb-key-link]")!.getAttribute("href")).toBe(
      "/media/recognize",
    );
  });

  it("有内置的、但换了接口地址：写明内置的 Key 只发给官方地址", async () => {
    api.settings.mockResolvedValue({ has_tmdb_key: false, key_source: "none", builtin_key: true });
    await mountNotice();
    const note = document.querySelector("[data-testid=tmdb-key-notice]")!;
    expect(note.textContent).toContain("内置的 Key 只发给 TMDB 官方地址");
    expect(note.textContent).toContain("探索与订阅都要用它");
  });

  it("用着自己的或内置的 Key、或者读不到设置：不提示", async () => {
    for (const s of [
      { has_tmdb_key: true, key_source: "user", builtin_key: false },
      { has_tmdb_key: false, key_source: "builtin", builtin_key: true },
    ]) {
      api.settings.mockResolvedValue(s);
      await mountNotice();
      expect(document.querySelector("[data-testid=tmdb-key-notice]"), s.key_source).toBeNull();
      view?.unmount();
      view = null;
    }
    api.settings.mockRejectedValue(new Error("x"));
    await mountNotice();
    expect(document.querySelector("[data-testid=tmdb-key-notice]")).toBeNull();
  });
});
