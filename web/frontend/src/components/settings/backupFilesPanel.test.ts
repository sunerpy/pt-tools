// @vitest-environment happy-dom
/*
 * 钉子：全局设置页的「备份与恢复」面板。列表写明时间、原因、版本、大小与带没带密钥，下载是带会话的同源链接；
 * 恢复与删除要确认，恢复只是放进待恢复；待恢复写明密钥怎么处理、能取消；启动时恢复的结果一周内提示；
 * 最近一次备份没做成时提示；读不回来时写明原因、给重试，不当成「还没有备份」。
 */
import { afterEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import type { BackupItem, BackupsView } from "@/api";
import { type MountedView, mountView } from "@/test-mount";

import BackupFilesPanel from "./BackupFilesPanel.vue";

const api = vi.hoisted(() => ({
  create: vi.fn(),
  remove: vi.fn(),
  restore: vi.fn(),
  cancelRestore: vi.fn(),
  upload: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  confirm: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, backupApi: { ...real.backupApi, ...api } };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), {
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
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);
const text = () => document.body.textContent ?? "";

const NAME = "pt-tools-backup-20261102-043000-scheduled.tar.gz";

function item(over: Partial<BackupItem> = {}): BackupItem {
  return {
    name: NAME,
    size: 2 * 1024 * 1024,
    created_at: "2026-11-02T04:30:00Z",
    reason: "scheduled",
    reason_label: "定时备份",
    app_version: "v1.0.0-rc.52",
    schema_version: 17,
    has_secret: true,
    secret_source: "file",
    ...over,
  };
}

function backupsView(over: Partial<BackupsView> = {}): BackupsView {
  return {
    settings: {
      enabled: true,
      time: "04:30",
      keep: 7,
      dir: "",
      include_secret: true,
      default_dir: "/root/.pt-tools/backups",
      min_keep: 1,
      max_keep: 60,
    },
    dir: "/root/.pt-tools/backups",
    env_key: false,
    next_at: "2026-11-03T04:30:00Z",
    last: null,
    restore: null,
    pending: null,
    backups: [item()],
    ...over,
  };
}

/** 外层模拟 GlobalSettings：接住 update 换成新的备份页，记下 reload */
const reloads = ref(0);
function mountPanel(initial: BackupsView | null, errorText = "") {
  const current = ref<BackupsView | null>(initial);
  reloads.value = 0;
  const Host = defineComponent({
    setup() {
      return () =>
        h(BackupFilesPanel, {
          view: current.value,
          loading: false,
          errorText,
          onUpdate: (v: BackupsView) => {
            current.value = v;
          },
          onReload: () => {
            reloads.value += 1;
          },
        });
    },
  });
  view = mountView(Host);
  return current;
}

function buttonIn(scope: ParentNode, label: string): HTMLButtonElement {
  const b = [...scope.querySelectorAll<HTMLButtonElement>("button")].find(
    (x) => (x.textContent ?? "").trim() === label,
  );
  if (!b) throw new Error(`找不到「${label}」`);
  return b;
}

describe("备份与恢复面板", () => {
  it("列表：时间、原因、版本、大小、带密钥；下载链接带会话", async () => {
    mountPanel(backupsView());
    await flush();
    const table = q("backup-table")!;
    expect(table.textContent).toContain("定时备份");
    expect(table.textContent).toContain("v1.0.0-rc.52 · 数据库 v17");
    expect(table.textContent).toContain("2 MB");
    expect(table.textContent).toContain("带密钥");
    expect(table.textContent).toContain(NAME);
    const link = table.querySelector<HTMLAnchorElement>("a[download]")!;
    expect(link.getAttribute("href")).toBe(`/api/backups/file/${NAME}`);
    expect(text()).toContain("备份目录 /root/.pt-tools/backups");
    expect(text()).toContain("下次定时备份");
  });

  it("坏文件只能删除，不给下载与恢复", async () => {
    mountPanel(
      backupsView({
        backups: [
          item({
            name: "pt-tools-backup-broken.tar.gz",
            error: "不是 tar.gz 文件",
            created_at: null,
            reason: "",
            reason_label: "",
          }),
        ],
      }),
    );
    await flush();
    const table = q("backup-table")!;
    expect(table.textContent).toContain("不是 tar.gz 文件");
    expect(table.querySelector("a[download]")).toBeNull();
    expect(q("backup-restore-pt-tools-backup-broken.tar.gz")).toBeNull();
    expect(q("backup-delete-pt-tools-backup-broken.tar.gz")).not.toBeNull();
  });

  it("没有备份时是空态；读不回来时写明原因、给重试", async () => {
    mountPanel(backupsView({ backups: [] }));
    await flush();
    expect(q("backup-empty")?.textContent).toContain("还没有备份");
    expect(q("backup-empty")?.textContent).toContain("每天 04:30");
    view?.unmount();

    mountPanel(null, "session 无效");
    await flush();
    expect(q("backup-state")?.textContent).toContain("session 无效");
    expect(q("backup-empty")).toBeNull();
    buttonIn(q("backup-state")!, "重试").click();
    expect(reloads.value).toBe(1);
    expect((q("backup-create") as HTMLButtonElement).disabled).toBe(true);
  });

  it("立即备份：成功换上新列表；失败提示原因并重新读取", async () => {
    const current = mountPanel(backupsView({ backups: [] }));
    await flush();
    const next = backupsView({
      last: {
        at: "2026-11-02T09:00:00Z",
        reason: "manual",
        reason_label: "手动备份",
        name: "m.tar.gz",
        last_ok_at: "2026-11-02T09:00:00Z",
      },
    });
    api.create.mockResolvedValueOnce(next);
    q("backup-create")!.click();
    await vi.waitFor(() => expect(current.value).toStrictEqual(next));
    expect(ui.success).toHaveBeenCalledWith("已备份：m.tar.gz");

    api.create.mockRejectedValueOnce(new Error("备份失败：磁盘满了"));
    q("backup-create")!.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("备份失败：磁盘满了"));
    expect(reloads.value).toBe(1);
  });

  it("恢复要确认，确认以后放进待恢复；取消确认什么都不发", async () => {
    const current = mountPanel(backupsView());
    await flush();
    ui.confirm.mockRejectedValueOnce("cancel");
    q(`backup-restore-${NAME}`)!.click();
    await flush();
    expect(api.restore).not.toHaveBeenCalled();

    const staged = backupsView({
      pending: {
        name: NAME,
        staged_at: "2026-11-02T10:00:00Z",
        created_at: "2026-11-02T04:30:00Z",
        reason: "scheduled",
        reason_label: "定时备份",
        app_version: "v1.0.0-rc.52",
        schema_version: 17,
        key_action: "use-backup",
        no_cipher: false,
      },
    });
    ui.confirm.mockResolvedValueOnce("confirm");
    api.restore.mockResolvedValueOnce(staged);
    q(`backup-restore-${NAME}`)!.click();
    await vi.waitFor(() => expect(current.value).toStrictEqual(staged));
    expect(api.restore).toHaveBeenCalledWith(NAME);
    expect(ui.confirm.mock.calls[1]![0]).toContain("重启 pt-tools 时换上");
    await flush();
    const pending = q("backup-pending")!;
    expect(pending.textContent).toContain("重启 pt-tools 以后生效");
    expect(pending.textContent).toContain("secret.key 会换成备份里的密钥");

    api.cancelRestore.mockResolvedValueOnce(backupsView());
    q("backup-cancel-restore")!.click();
    await vi.waitFor(() => expect(api.cancelRestore).toHaveBeenCalled());
    await flush();
    expect(q("backup-pending")).toBeNull();
  });

  it("恢复失败时写明原因（密钥对不上之类），列表不变", async () => {
    const current = mountPanel(backupsView());
    await flush();
    const before = current.value;
    ui.confirm.mockResolvedValueOnce("confirm");
    api.restore.mockRejectedValueOnce(new Error("密钥对不上：这份备份没带密钥"));
    q(`backup-restore-${NAME}`)!.click();
    await vi.waitFor(() =>
      expect(ui.error).toHaveBeenCalledWith("不能恢复：密钥对不上：这份备份没带密钥"),
    );
    expect(current.value).toStrictEqual(before);
  });

  it("删除要确认", async () => {
    const current = mountPanel(backupsView());
    await flush();
    ui.confirm.mockResolvedValueOnce("confirm");
    api.remove.mockResolvedValueOnce(backupsView({ backups: [] }));
    q(`backup-delete-${NAME}`)!.click();
    await vi.waitFor(() => expect(current.value?.backups).toEqual([]));
    expect(api.remove).toHaveBeenCalledWith(NAME);
  });

  it("上传恢复：选文件、确认、带进度上传", async () => {
    const current = mountPanel(backupsView());
    await flush();
    const staged = backupsView({
      pending: {
        ...backupsView().pending!,
        name: "家里的备份.tar.gz",
        staged_at: "",
        created_at: "",
        reason: "manual",
        reason_label: "手动备份",
        app_version: "",
        schema_version: 17,
        key_action: "keep-file",
        no_cipher: true,
      },
    });
    ui.confirm.mockResolvedValueOnce("confirm");
    api.upload.mockImplementationOnce(async (_f: File, onProgress?: (r: number) => void) => {
      onProgress?.(0.5);
      return staged;
    });
    const input = q("backup-file") as HTMLInputElement;
    const file = new File(["x"], "家里的备份.tar.gz", { type: "application/gzip" });
    Object.defineProperty(input, "files", { value: [file], configurable: true });
    input.dispatchEvent(new Event("change"));
    await vi.waitFor(() => expect(current.value).toStrictEqual(staged));
    expect(api.upload.mock.calls[0]![0]).toBe(file);
    expect(ui.confirm.mock.calls[0]![0]).toContain("家里的备份.tar.gz");
    await flush();
    expect(q("backup-pending")?.textContent).toContain("不需要密钥");
  });

  it("启动时恢复的结果一周内提示；最近一次备份没做成时提示", async () => {
    const now = Date.now();
    mountPanel(
      backupsView({
        restore: {
          at: new Date(now - 3600_000).toISOString(),
          name: "a.tar.gz",
          dir: "/root/.pt-tools/backups/pre-restore-20261102-050000",
        },
        last: {
          at: new Date(now).toISOString(),
          reason: "scheduled",
          reason_label: "定时备份",
          name: "",
          error: "创建备份目录失败",
          last_ok_at: null,
        },
      }),
    );
    await flush();
    expect(q("backup-restore-ok")?.textContent).toContain("pre-restore-20261102-050000");
    expect(q("backup-last-failed")?.textContent).toContain("创建备份目录失败");
    expect(q("backup-last-failed")?.textContent).toContain("还没有成功做过备份");
    view?.unmount();

    mountPanel(
      backupsView({
        restore: {
          at: new Date(now - 3600_000).toISOString(),
          name: "b.tar.gz",
          error: "当前生效的密钥解不开",
        },
      }),
    );
    await flush();
    expect(q("backup-restore-failed")?.textContent).toContain("当前生效的密钥解不开");
    view?.unmount();

    mountPanel(
      backupsView({
        restore: { at: new Date(now - 10 * 86400_000).toISOString(), name: "c.tar.gz" },
      }),
    );
    await flush();
    expect(q("backup-restore-ok")).toBeNull();
    expect(q("backup-restore-failed")).toBeNull();
  });

  it("关着定时备份时页脚写明", async () => {
    mountPanel(
      backupsView({ settings: { ...backupsView().settings, enabled: false }, next_at: null }),
    );
    await flush();
    expect(text()).toContain("定时备份已关闭");
  });
});
