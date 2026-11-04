// @vitest-environment happy-dom
/*
 * 钉子：追剧的剧集组。TMDB 上这部剧有剧集组时列出来（名字、类型、几组几集、平台），选了以后新建与修改带上
 * episode_group_id；修改时没改不带，选「不用」带空串；没有剧集组也没选过时不显示；读不到时写明原因，选着的剧集组
 * 不在列表里时照样列出来。
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
  episodeGroups: vi.fn(),
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

const ABS = "5b11ba820e0a265a53000ce9";
const DVD = "5c8a1ef80e0a2625f5a4f1b0";
const GROUPS = [
  { id: ABS, name: "Absolute Order", type: 2, group_count: 1, episode_count: 1100 },
  { id: DVD, name: "DVD", type: 3, group_count: 20, episode_count: 1080, network: "Fuji TV" },
];

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  document.body.innerHTML = "";
  vi.clearAllMocks();
});

const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);

function options(id: string): () => HTMLElement[] {
  const sel = q(id)!;
  (sel.querySelector<HTMLElement>(".el-select__wrapper") ?? sel).click();
  const listId = sel.querySelector("input")?.getAttribute("aria-controls");
  return () => [
    ...(document
      .getElementById(listId ?? "")
      ?.querySelectorAll<HTMLElement>(".el-select-dropdown__item") ?? []),
  ];
}

async function pick(id: string, text: string) {
  const list = options(id);
  await vi.waitFor(() => {
    const opt = list().find((o) => (o.textContent ?? "").trim() === text);
    expect(opt).toBeTruthy();
    opt!.click();
  });
}

const sub = (group: string) => ({
  id: 3,
  media_type: "tv",
  tmdb_id: 37854,
  season: 1,
  title: "海贼王",
  year: 1999,
  poster_path: "",
  profile_id: 0,
  sites: [],
  downloader_id: 0,
  category: "",
  tags: "",
  save_path: "",
  upgrade: false,
  status: "active",
  schedule: { kind: "airtime" },
  lock_version: false,
  episode_group_id: group,
});

async function mountDialog(edit?: ReturnType<typeof sub>) {
  const Dialog = (await import("./SubscribeDialog.vue")).default;
  const Host = defineComponent({
    setup() {
      const open = ref(true);
      return () =>
        h(Dialog, {
          modelValue: open.value,
          "onUpdate:modelValue": (v: boolean) => (open.value = v),
          ...(edit
            ? { sub: edit as never }
            : { target: { kind: "tv", tmdb_id: 37854, title: "海贼王" } }),
        });
    },
  });
  view = mountView(Host);
  await vi.waitFor(() => expect(q("sd-schedule")).not.toBeNull());
  await vi.waitFor(() => expect(api.episodeGroups).toHaveBeenCalledWith(37854));
}

describe("追剧的剧集组", () => {
  it("新建：列出剧集组，选了以后带上编号", async () => {
    api.episodeGroups.mockResolvedValue({ groups: GROUPS });
    api.create.mockResolvedValue({ id: 9, title: "海贼王" });
    await mountDialog();
    await vi.waitFor(() => expect(q("sd-episode-group")).not.toBeNull());
    const labels = options("sd-episode-group");
    await vi.waitFor(() =>
      expect(labels().map((o) => (o.textContent ?? "").trim())).toEqual([
        "不用（按 TMDB 默认的季集）",
        "Absolute Order · 绝对集号 · 1 组 1100 集",
        "DVD · DVD · 20 组 1080 集 · Fuji TV",
      ]),
    );
    expect(q("sd-episode-group-tip")!.textContent).toContain("先按它换成 TMDB");
    await pick("sd-episode-group", "Absolute Order · 绝对集号 · 1 组 1100 集");
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalled());
    expect(api.create.mock.calls[0]![0]).toMatchObject({ tmdb_id: 37854, episode_group_id: ABS });
  });

  it("新建时没选：不带", async () => {
    api.episodeGroups.mockResolvedValue({ groups: GROUPS });
    api.create.mockResolvedValue({ id: 9, title: "海贼王" });
    await mountDialog();
    await vi.waitFor(() => expect(q("sd-episode-group")).not.toBeNull());
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.create).toHaveBeenCalled());
    expect(api.create.mock.calls[0]![0]).not.toHaveProperty("episode_group_id");
  });

  it("修改：没改不带；选「不用」带空串", async () => {
    api.episodeGroups.mockResolvedValue({ groups: GROUPS });
    api.update.mockResolvedValue(sub(""));
    await mountDialog(sub(DVD));
    await vi.waitFor(() => expect(q("sd-episode-group")).not.toBeNull());
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(1));
    expect(api.update.mock.calls[0]![1]).not.toHaveProperty("episode_group_id");

    view?.unmount();
    document.body.innerHTML = "";
    await mountDialog(sub(DVD));
    await vi.waitFor(() => expect(q("sd-episode-group")).not.toBeNull());
    await pick("sd-episode-group", "不用（按 TMDB 默认的季集）");
    q("sd-save")!.click();
    await vi.waitFor(() => expect(api.update).toHaveBeenCalledTimes(2));
    expect(api.update.mock.calls[1]![1]).toMatchObject({ episode_group_id: "" });
  });

  it("TMDB 上没有剧集组、也没选过：不显示", async () => {
    api.episodeGroups.mockResolvedValue({ groups: [] });
    await mountDialog();
    await new Promise((r) => setTimeout(r, 0));
    expect(q("sd-episode-group")).toBeNull();
  });

  it("读不到时写明原因；选着的剧集组不在列表里时照样列出来", async () => {
    const { ApiError } = await import("@/api");
    api.episodeGroups.mockRejectedValue(new ApiError(502, "TMDB 暂时不能访问"));
    await mountDialog(sub(ABS));
    await vi.waitFor(() =>
      expect(q("sd-episode-group-error")?.textContent).toContain(
        "读不到 TMDB 上的剧集组：TMDB 暂时不能访问",
      ),
    );
    const labels = options("sd-episode-group");
    await vi.waitFor(() =>
      expect(labels().map((o) => (o.textContent ?? "").trim())).toContain(
        `现在选的剧集组（${ABS}）`,
      ),
    );
  });
});
