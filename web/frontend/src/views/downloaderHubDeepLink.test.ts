// @vitest-environment happy-dom
/*
 * 钉子：条目详情的「在下载器里打开」带着 ?downloader_id=…&search=…（种子的 hash）进来，打开时第一次就按它筛；
 * 没带或写法不对时照旧列全部。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { createMemoryHistory, createRouter } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  batchAction: vi.fn(),
}));

const ui = vi.hoisted(() => ({
  confirm: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));

function row(id: string, title: string) {
  return {
    downloader_id: 1,
    downloader_name: "qb-家里",
    downloader_type: "qbittorrent",
    task_id: id,
    title,
    info_hash: id,
    state: "seeding",
    progress: 100,
    size: 1024 ** 3,
    upload_speed: 0,
    download_speed: 0,
    seeds: 3,
    connections: 1,
    ratio: 1.2,
    added_at: 1_700_000_000,
    completed_at: -1,
    save_path: "/downloads",
    category: "",
    tags: "",
    eta: 0,
  };
}

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  const cap = {
    downloader_id: 1,
    downloader_name: "qb-家里",
    supports_categories: true,
    supports_tags: true,
    can_pause: true,
    can_resume: true,
    can_delete: true,
    can_delete_with_data: true,
    can_set_location: true,
    can_recheck: true,
    can_add_torrent: true,
    categories: [],
    tags: [],
  };
  return {
    ...real,
    downloadersApi: { list: vi.fn(() => Promise.resolve([])) },
    downloaderTorrentsApi: {
      list: api.list,
      batchAction: api.batchAction,
      capabilities: vi.fn(() => Promise.resolve({ items: [cap] })),
      meta: vi.fn(() => Promise.resolve({ downloaders: [], categories: [], tags: [] })),
      transferStats: vi.fn(() =>
        Promise.resolve({
          total_upload_speed: 0,
          total_download_speed: 0,
          total_uploaded: 0,
          total_downloaded: 0,
          total_session_uploaded: 0,
          total_session_downloaded: 0,
          total_free_space: 0,
          downloaders: [],
        }),
      ),
      detail: vi.fn(),
      add: vi.fn(),
    },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessageBox: { confirm: ui.confirm },
    ElMessage: Object.assign(vi.fn(), {
      success: ui.success,
      error: ui.error,
      warning: vi.fn(),
      info: vi.fn(),
    }),
  };
});

let view: MountedView | null = null;

afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

async function mountAt(path: string) {
  localStorage.clear();
  api.list.mockResolvedValue({ items: [row("abc", "任务甲")], total: 1, failures: [] });
  const DownloaderHub = (await import("./DownloaderHub.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:p(.*)*", component: { render: () => null } }],
  });
  await router.push(path);
  await router.isReady();
  view = mountView(DownloaderHub, { router });
  await vi.waitFor(() => expect(api.list).toHaveBeenCalled());
}

describe("下载器 Web UI 的深链接", () => {
  it("带着下载器与 hash 打开：第一次就按它筛", async () => {
    await mountAt("/downloader-hub?downloader_id=1&search=abc");
    const params = api.list.mock.calls[0]![0] as URLSearchParams;
    expect(params.get("downloader_id")).toBe("1");
    expect(params.get("search")).toBe("abc");
  });

  it("没带或写法不对：列全部", async () => {
    await mountAt("/downloader-hub?downloader_id=x");
    const params = api.list.mock.calls[0]![0] as URLSearchParams;
    expect(params.get("downloader_id")).toBeNull();
    expect(params.get("search")).toBeNull();
  });
});
