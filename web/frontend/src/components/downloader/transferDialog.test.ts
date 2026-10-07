// @vitest-environment happy-dom
/*
 * 钉子：转移对话框。目标下载器里不列唯一的那台源下载器；先预览才能建任务，只提交能转的那几个；
 * 换了目标要重新预览。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({ preview: vi.fn(), create: vi.fn() }));
const ui = vi.hoisted(() => ({ message: vi.fn(), error: vi.fn() }));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    transferApi: { ...real.transferApi, ...api },
    downloadersApi: {
      ...real.downloadersApi,
      list: vi.fn(() =>
        Promise.resolve([
          {
            id: 1,
            name: "qb-src",
            type: "qbittorrent",
            url: "",
            username: "",
            is_default: true,
            enabled: true,
          },
          {
            id: 2,
            name: "tr-nas",
            type: "transmission",
            url: "",
            username: "",
            is_default: false,
            enabled: true,
          },
          {
            id: 4,
            name: "qb-two",
            type: "qbittorrent",
            url: "",
            username: "",
            is_default: false,
            enabled: true,
          },
        ]),
      ),
    },
  };
});
vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return { ...real, ElMessage: Object.assign(ui.message, { error: ui.error, success: vi.fn() }) };
});

let view: MountedView | null = null;
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
});
const flush = () => new Promise((r) => setTimeout(r, 0));
const testid = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`)!;

async function mountDialog() {
  const TransferDialog = (await import("./TransferDialog.vue")).default;
  const created = vi.fn();
  const visible = ref(false);
  view = mountView(
    defineComponent({
      setup: () => () =>
        h(TransferDialog, {
          modelValue: visible.value,
          "onUpdate:modelValue": (v: boolean) => (visible.value = v),
          items: [
            { source_id: 1, hash: "aa" },
            { source_id: 1, hash: "bb" },
          ],
          onCreated: created,
        }),
    }),
  );
  visible.value = true;
  await vi.waitFor(() => expect(document.body.textContent).toContain("已选 2 个种子"));
  await flush();
  return { created, visible };
}

describe("转移对话框", () => {
  it("预览后只提交能转的种子；换目标要重新预览", async () => {
    const { created, visible } = await mountDialog();
    expect((testid("tr-create") as HTMLButtonElement).disabled).toBe(true);
    api.preview.mockResolvedValue({
      items: [
        {
          source_id: 1,
          source_name: "qb-src",
          hash: "aa",
          name: "Movie.A",
          size: 1 << 30,
          save_path: "/downloads",
          target_path: "/data",
          mapped: true,
          source: "export",
          ok: true,
        },
        {
          source_id: 1,
          source_name: "qb-src",
          hash: "bb",
          name: "Movie.B",
          size: 1,
          save_path: "/downloads",
          target_path: "/downloads",
          mapped: false,
          ok: false,
          reason: "还没下载完",
        },
      ],
    });
    testid("tr-preview").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("还没下载完"));
    // 唯一的源下载器不在目标里，默认选第一台别的
    expect(api.preview).toHaveBeenCalledWith(2, [
      { source_id: 1, hash: "aa" },
      { source_id: 1, hash: "bb" },
    ]);
    const text = document.body.textContent ?? "";
    expect(text).toContain("从下载器导出");
    expect(text).toContain("能转 1 个，跳过 1 个");
    expect(text).toContain("建 1 个转移任务");

    api.create.mockResolvedValue({ created: [{ id: 9 }], skipped: [] });
    testid("tr-create").click();
    await vi.waitFor(() =>
      expect(api.create).toHaveBeenCalledWith(2, [{ source_id: 1, hash: "aa" }]),
    );
    await vi.waitFor(() => expect(created).toHaveBeenCalledWith(1));
    expect(visible.value).toBe(false);
    expect(ui.message).toHaveBeenCalledWith(expect.objectContaining({ type: "success" }));
  });

  it("预览失败时写明原因", async () => {
    await mountDialog();
    api.preview.mockRejectedValue(new Error("目标下载器不可用: 连接被拒绝"));
    testid("tr-preview").click();
    await vi.waitFor(() => expect(document.body.textContent).toContain("目标下载器不可用"));
    expect((testid("tr-create") as HTMLButtonElement).disabled).toBe(true);
  });
});
