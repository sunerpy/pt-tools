// @vitest-environment happy-dom
/*
 * 钉子：下载器助手。扫描前给出「还没扫描」；失效种子勾选后删除，带上「同时删除数据文件」；
 * 替换 tracker 只能用预览过的内容执行，改了内容要重新预览；下载器不支持时只能预览；
 * 定时扫描开启时必须选通道。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  siteTags: vi.fn(),
  applySiteTags: vi.fn(),
  previewTrackers: vi.fn(),
  applyTrackers: vi.fn(),
  dead: vi.fn(),
  deleteDead: vi.fn(),
  getDeadScan: vi.fn(),
  saveDeadScan: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  message: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  confirm: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    downloaderAssistantApi: api,
    downloadersApi: {
      ...real.downloadersApi,
      list: vi.fn(() =>
        Promise.resolve([
          {
            id: 2,
            name: "qb-main",
            type: "qbittorrent",
            url: "",
            username: "",
            is_default: true,
            enabled: true,
          },
          {
            id: 3,
            name: "tr-off",
            type: "transmission",
            url: "",
            username: "",
            is_default: false,
            enabled: false,
          },
        ]),
      ),
    },
    chatopsApi: {
      ...real.chatopsApi,
      notifications: {
        ...real.chatopsApi.notifications,
        list: vi.fn(() =>
          Promise.resolve([{ id: 7, name: "tg", channel_type: "telegram", enabled: true }]),
        ),
      },
    },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(ui.message, {
      success: ui.success,
      error: ui.error,
      warning: ui.warning,
      info: vi.fn(),
    }),
    ElMessageBox: { ...real.ElMessageBox, confirm: ui.confirm },
  };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});

const flush = () => new Promise((r) => setTimeout(r, 0));

async function mountPage() {
  api.getDeadScan.mockResolvedValue({ enabled: false, interval_hours: 24, channel_ids: [] });
  const Page = (await import("./DownloaderAssistant.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(document.body.textContent).toContain("qb-main"));
  await flush();
}

function testid(id: string): HTMLElement {
  const el = document.querySelector<HTMLElement>(`[data-testid=${id}]`);
  if (!el) throw new Error(`找不到 ${id}`);
  return el;
}

async function chooseTab(label: string) {
  const item = [
    ...document.querySelectorAll<HTMLElement>("[data-testid=da-tabs] .el-segmented__item"),
  ].find((el) => (el.textContent ?? "").trim() === label);
  if (!item) throw new Error(`找不到标签「${label}」`);
  item.click();
  await flush();
}

function setInput(id: string, value: string) {
  // el-input 把 data-testid 透传给了里面的原生 input
  const el = testid(id);
  const input = (
    el instanceof HTMLInputElement ? el : el.querySelector("input")
  ) as HTMLInputElement;
  input.value = value;
  input.dispatchEvent(new Event("input"));
}

describe("下载器助手", () => {
  it("只列出启用的下载器；扫描前提示还没扫描", async () => {
    await mountPage();
    expect(document.body.textContent).toContain("还没扫描");
    expect(document.body.textContent).not.toContain("tr-off");
  });

  it("失效种子：扫描、勾选后删除，带上是否删除数据", async () => {
    await mountPage();
    api.dead.mockResolvedValue({
      total: 1,
      scanned: 1,
      items: [
        {
          hash: "aa",
          name: "Old Movie",
          size: 1024 ** 3,
          progress: 1,
          site: "hdsky",
          tracker_host: "tracker.hdsky.me",
          reason: "unregistered",
          message: "Unregistered torrent",
        },
      ],
    });
    testid("da-dead-scan").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("Old Movie"));
    expect(api.dead).toHaveBeenCalledWith(2);
    expect(document.body.textContent).toContain("未注册");

    const box = document.querySelector<HTMLInputElement>(".el-table__body .el-checkbox__original")!;
    box.click();
    await flush();
    testid("da-dead-remove-data").querySelector<HTMLInputElement>("input")!.click();
    await flush();
    ui.confirm.mockResolvedValue("confirm");
    api.deleteDead.mockResolvedValue({ done: 1, skipped: [], failed: [] });
    api.dead.mockResolvedValue({ items: [] });
    testid("da-dead-delete").click();
    await vi.waitFor(() => expect(api.deleteDead).toHaveBeenCalledWith(2, ["aa"], true));
    expect(ui.confirm.mock.calls[0][0]).toContain("删除它们的数据文件");
    await vi.waitFor(() =>
      expect(ui.message).toHaveBeenCalledWith(
        expect.objectContaining({ message: "删除 1 个", type: "success" }),
      ),
    );
  });

  it("替换 tracker：执行用预览过的内容，按地址勾选；不支持时只能预览", async () => {
    await mountPage();
    await chooseTab("替换 Tracker");
    setInput("da-tr-from", "old.hdsky.me");
    setInput("da-tr-to", "tracker.hdsky.me");
    await flush();
    api.previewTrackers.mockResolvedValue({
      supported: false,
      total: 1,
      scanned: 1,
      items: [
        {
          id: "f1",
          hash: "aa",
          name: "A",
          old: "https://old.hdsky.me/announce.php?passkey=***",
          new: "https://tracker.hdsky.me/announce.php?passkey=***",
        },
      ],
    });
    testid("da-tr-preview").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("只能预览"));
    expect(api.previewTrackers).toHaveBeenCalledWith(2, "old.hdsky.me", "tracker.hdsky.me");
    document.querySelector<HTMLInputElement>(".el-table__body .el-checkbox__original")!.click();
    await flush();
    expect((testid("da-tr-apply") as HTMLButtonElement).disabled).toBe(true);

    api.previewTrackers.mockResolvedValue({
      supported: true,
      total: 30000,
      scanned: 20000,
      items: [
        {
          id: "f1",
          hash: "aa",
          name: "A",
          old: "https://old.hdsky.me/a",
          new: "https://tracker.hdsky.me/a",
        },
        {
          id: "f2",
          hash: "aa",
          name: "A",
          old: "https://old.hdsky.me/b",
          new: "https://tracker.hdsky.me/b",
        },
      ],
    });
    testid("da-tr-preview").click();
    await vi.waitFor(() => expect(document.body.textContent).not.toContain("只能预览"));
    expect(document.body.textContent).toContain("这次只检查了前 20000 个");
    // 同一种子两条地址，只勾第一条
    document.querySelector<HTMLInputElement>(".el-table__body .el-checkbox__original")!.click();
    await flush();
    setInput("da-tr-to", "t2.hdsky.me");
    await flush();
    expect(document.body.textContent).toContain("请重新预览");
    expect((testid("da-tr-apply") as HTMLButtonElement).disabled).toBe(true);

    setInput("da-tr-to", "tracker.hdsky.me");
    await flush();
    ui.confirm.mockResolvedValue("confirm");
    api.applyTrackers.mockResolvedValue({ done: 1, skipped: [], failed: [] });
    testid("da-tr-apply").click();
    await vi.waitFor(() =>
      expect(api.applyTrackers).toHaveBeenCalledWith(2, "old.hdsky.me", "tracker.hdsky.me", [
        { hash: "aa", id: "f1" },
      ]),
    );
    expect(ui.confirm.mock.calls.at(-1)![0]).toContain("1 个地址（1 个种子）");
  });

  it("补站点标签：勾选后按站点提交", async () => {
    await mountPage();
    await chooseTab("补站点标签");
    api.siteTags.mockResolvedValue({
      items: [
        {
          hash: "bb",
          name: "Show",
          size: 1,
          site: "hdsky",
          site_name: "HDSky",
          tracker_host: "tracker.hdsky.me",
          tags: "",
          category: "",
        },
      ],
    });
    testid("da-tags-scan").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("Show"));
    document.querySelector<HTMLInputElement>(".el-table__body .el-checkbox__original")!.click();
    await flush();
    api.applySiteTags.mockResolvedValue({ done: 1, skipped: [], failed: [] });
    api.siteTags.mockResolvedValue({ items: [] });
    testid("da-tags-apply").click();
    await vi.waitFor(() =>
      expect(api.applySiteTags).toHaveBeenCalledWith(2, [{ hash: "bb", site: "hdsky" }]),
    );
  });

  it("定时扫描：开启时没选通道不提交", async () => {
    await mountPage();
    await chooseTab("定时扫描");
    testid("da-scan-enabled").click();
    await flush();
    testid("da-scan-save").click();
    await flush();
    expect(ui.warning).toHaveBeenCalledWith("开启定时扫描至少要选一个通知通道");
    expect(api.saveDeadScan).not.toHaveBeenCalled();
  });
});
