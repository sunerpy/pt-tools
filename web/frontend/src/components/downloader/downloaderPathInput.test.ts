// @vitest-environment happy-dom
/*
 * 钉子：下载器路径输入框。点开列出下载器预设的目录（默认的在前、有别名的另写路径），选一个填进路径；
 * 列表里没有的路径直接输入；没指定下载器时用默认下载器的目录；几台下载器一起时写明是哪台的。
 * 2026-10-26 用户：「下载器设置了下载目录，就可以直接选择而不是必须要输入」。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ listAll: vi.fn(), list: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    downloaderDirectoriesApi: { ...real.downloaderDirectoriesApi, listAll: api.listAll },
    downloadersApi: { ...real.downloadersApi, list: api.list },
  };
});

const DIRS = {
  1: [
    { id: 2, downloader_id: 1, path: "/data/movies", alias: "电影", is_default: false },
    { id: 1, downloader_id: 1, path: "/downloads", alias: "", is_default: true },
  ],
  2: [{ id: 3, downloader_id: 2, path: "/mnt/tv", alias: "剧集", is_default: true }],
};
const DLS = [
  {
    id: 1,
    name: "qb-main",
    type: "qbittorrent",
    url: "",
    username: "",
    is_default: true,
    enabled: true,
  },
  {
    id: 2,
    name: "tr-box",
    type: "transmission",
    url: "",
    username: "",
    is_default: false,
    enabled: true,
  },
];

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));

async function mountInput(downloaderIds?: number | number[], initial = "") {
  api.listAll.mockResolvedValue(DIRS);
  api.list.mockResolvedValue(DLS);
  const Input = (await import("./DownloaderPathInput.vue")).default;
  const value = ref(initial);
  const Host = defineComponent({
    setup: () => () =>
      h(Input, {
        modelValue: value.value,
        "onUpdate:modelValue": (v: string) => (value.value = v),
        downloaderIds,
        placeholder: "留空用下载器的默认路径",
        "data-testid": "path-input",
      }),
  });
  view = mountView(Host);
  await vi.waitFor(() => expect(api.listAll).toHaveBeenCalled());
  await flush();
  return value;
}

// el-autocomplete 把 data-testid 交给里面的 input：本身是 input 时直接用，否则找它下面的
function input(): HTMLInputElement {
  const el = document.querySelector<HTMLElement>("[data-testid=path-input]")!;
  return (el instanceof HTMLInputElement ? el : el.querySelector("input")) as HTMLInputElement;
}

/** 聚焦输入框，等下拉列出选项，给出每一项的文字 */
async function openOptions(): Promise<HTMLElement[]> {
  input().dispatchEvent(new Event("focus"));
  input().focus();
  let items: HTMLElement[] = [];
  await vi.waitFor(() => {
    items = [...document.querySelectorAll<HTMLElement>(".pt-path-input__popper li")];
    expect(items.length).toBeGreaterThan(0);
  });
  return items;
}

/** 一项的文字：名称、标记、路径各段用空格隔开 */
const text = (el: HTMLElement) =>
  [...el.querySelectorAll(".pt-path-opt__name > span, .pt-path-opt__path")]
    .map((s) => (s.textContent ?? "").trim())
    .join(" ");

describe("下载器路径输入框", () => {
  it("列出这台下载器的预设目录（默认的在前），选一个填进路径", async () => {
    const value = await mountInput(1);
    const items = await openOptions();
    expect(items.map(text)).toEqual(["/downloads 默认", "电影 /data/movies"]);
    items[1]!.click();
    await vi.waitFor(() => expect(value.value).toBe("/data/movies"));
  });

  it("列表里没有的路径直接输入", async () => {
    const value = await mountInput(1);
    const el = input();
    el.value = "/mnt/other";
    el.dispatchEvent(new Event("input"));
    await vi.waitFor(() => expect(value.value).toBe("/mnt/other"));
  });

  it("没指定下载器时列默认下载器的目录", async () => {
    await mountInput(undefined);
    const items = await openOptions();
    expect(items.map(text)).toEqual(["/downloads 默认", "电影 /data/movies"]);
  });

  it("几台下载器一起：每一项写明是哪台的", async () => {
    await mountInput([1, 2]);
    const items = await openOptions();
    expect(items.map(text)).toEqual([
      "/downloads 默认 qb-main",
      "电影 qb-main /data/movies",
      "剧集 默认 tr-box /mnt/tv",
    ]);
  });

  it("已经是某个预设目录时仍列出全部，方便换一个", async () => {
    await mountInput(1, "/downloads");
    const items = await openOptions();
    expect(items).toHaveLength(2);
  });
});
