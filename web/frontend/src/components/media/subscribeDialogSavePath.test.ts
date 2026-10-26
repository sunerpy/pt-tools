// @vitest-environment happy-dom
/*
 * 钉子：追剧 / 订阅的保存目录。点开列出推送用的那台下载器预设的目录：订阅没选下载器时是订阅设置里的默认下载器；
 * 列表里没有的路径直接输入。2026-10-26 用户：「下载器设置了下载目录，就可以直接选择而不是必须要输入」。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  profiles: vi.fn(async () => ({ items: [] })),
  settings: vi.fn(async () => ({ default_downloader_id: 2 })),
  create: vi.fn(),
  update: vi.fn(),
  previewSchedule: vi.fn(async () => ({ next: [] })),
}));

const DLS = [
  {
    id: 1,
    name: "qb",
    type: "qbittorrent",
    url: "",
    username: "",
    is_default: true,
    enabled: true,
  },
  {
    id: 2,
    name: "tr",
    type: "transmission",
    url: "",
    username: "",
    is_default: false,
    enabled: true,
  },
];

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    subscribeApi: { ...real.subscribeApi, ...api },
    downloadersApi: { list: vi.fn(async () => DLS) },
    downloaderDirectoriesApi: {
      listAll: vi.fn(async () => ({
        1: [{ id: 1, downloader_id: 1, path: "/qb/downloads", alias: "", is_default: true }],
        2: [{ id: 2, downloader_id: 2, path: "/tr/tv", alias: "剧集", is_default: true }],
      })),
    },
    sitesApi: { list: vi.fn(async () => ({})) },
    organizeApi: { ...real.organizeApi, libraries: vi.fn(async () => []) },
    mediaApi: { ...real.mediaApi, settings: vi.fn(async () => ({ image_base_url: "" })) },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

async function mountDialog() {
  const Dialog = (await import("./SubscribeDialog.vue")).default;
  const Host = defineComponent({
    setup() {
      const open = ref(true);
      return () =>
        h(Dialog, {
          modelValue: open.value,
          "onUpdate:modelValue": (v: boolean) => (open.value = v),
          target: { kind: "tv", tmdb_id: 7, title: "乘风" },
        });
    },
  });
  view = mountView(Host);
  await vi.waitFor(() =>
    expect(document.querySelector("[data-testid=sd-save-path]")).not.toBeNull(),
  );
  await vi.waitFor(() => expect(api.settings).toHaveBeenCalled());
  const el = document.querySelector<HTMLElement>("[data-testid=sd-save-path]")!;
  return (el instanceof HTMLInputElement ? el : el.querySelector("input")) as HTMLInputElement;
}

describe("追剧的保存目录", () => {
  it("订阅没选下载器：列订阅设置里默认下载器的目录", async () => {
    const input = await mountDialog();
    await new Promise((r) => setTimeout(r, 0));
    input.focus();
    input.dispatchEvent(new Event("focus"));
    let items: HTMLElement[] = [];
    await vi.waitFor(() => {
      items = [...document.querySelectorAll<HTMLElement>(".pt-path-input__popper li")];
      expect(items).toHaveLength(1);
    });
    expect(items[0]!.textContent).toContain("/tr/tv");
    items[0]!.click();
    await vi.waitFor(() => expect(input.value).toBe("/tr/tv"));
  });
});
