// @vitest-environment happy-dom
/*
 * 钉子：订阅与追剧的「标题包含 / 标题排除」。新建、修改都把两项原样发给后端；修改时显示订阅现在的值。
 * 2026-10-27 用户：「追剧中无法指定比如标题包含什么吗……已经是合集了 但是没地方让我写 我怕下载错了」。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  profiles: vi.fn(async () => ({ items: [] })),
  create: vi.fn(),
  update: vi.fn(),
  previewSchedule: vi.fn(async () => ({ next: [] })),
  settings: vi.fn(async () => ({})),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    subscribeApi: { ...real.subscribeApi, ...api },
    downloadersApi: { list: vi.fn(async () => []) },
    downloaderDirectoriesApi: {
      ...real.downloaderDirectoriesApi,
      listAll: vi.fn(async () => ({})),
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

const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);
const area = (id: string) => {
  const el = q(id)!;
  return (el instanceof HTMLTextAreaElement ? el : el.querySelector("textarea")) as HTMLTextAreaElement;
};

function fill(id: string, v: string) {
  const i = area(id);
  i.value = v;
  i.dispatchEvent(new Event("input"));
}

async function mountDialog(sub?: Record<string, unknown>) {
  const Dialog = (await import("./SubscribeDialog.vue")).default;
  const Host = defineComponent({
    setup() {
      const open = ref(true);
      return () =>
        h(Dialog, {
          modelValue: open.value,
          "onUpdate:modelValue": (v: boolean) => (open.value = v),
          ...(sub ? { sub: sub as never } : { target: { kind: "tv", tmdb_id: 7, title: "梦想" } }),
        });
    },
  });
  view = mountView(Host);
  await vi.waitFor(() => expect(q("sd-include")).not.toBeNull());
}

describe("标题包含与排除", () => {
  it("新建追剧：两项随保存发出", async () => {
    api.create.mockResolvedValue({ id: 9, title: "梦想" });
    await mountDialog();
    fill("sd-include", "CHDWEB\n3Audio Atmos-CHDWEB");
    fill("sd-exclude", "DV");
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledTimes(1));
    const body = api.create.mock.calls[0]![0];
    expect(body.include_keywords).toBe("CHDWEB\n3Audio Atmos-CHDWEB");
    expect(body.exclude_keywords).toBe("DV");
  });

  it("修改：显示现在的值，改了照样发出", async () => {
    api.update.mockResolvedValue({ id: 4, title: "梦想" });
    await mountDialog({
      id: 4,
      media_type: "tv",
      tmdb_id: 7,
      title: "梦想",
      year: 2026,
      season: 1,
      status: "active",
      profile_id: 0,
      sites: [],
      downloader_id: 0,
      category: "",
      tags: "",
      save_path: "",
      upgrade: false,
      include_keywords: "CHDWEB",
      exclude_keywords: "",
      lock_version: true,
      schedule: { kind: "airtime" },
    });
    await vi.waitFor(() => expect(area("sd-include").value).toBe("CHDWEB"));
    expect(area("sd-exclude").value).toBe("");
    fill("sd-exclude", "60fps");
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    const [id, body] = api.update.mock.calls[0]!;
    expect(id).toBe(4);
    expect(body.include_keywords).toBe("CHDWEB");
    expect(body.exclude_keywords).toBe("60fps");
  });
});
