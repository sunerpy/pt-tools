// @vitest-environment happy-dom
/*
 * 钉子：消息模板页（路线图 M21）。默认打开第一个事件并预览；点变量插到聚焦输入框的光标处（列表插一段 range）；
 * 改了以后防抖预览，出错写明原因；保存与恢复默认按事件类型调接口、更新「已自定义」；有没保存的修改时切换要确认。
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  list: vi.fn(),
  save: vi.fn(),
  reset: vi.fn(),
  preview: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  confirm: vi.fn(),
  success: vi.fn(),
  error: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return { ...real, chatopsApi: { ...real.chatopsApi, templates: api } };
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

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid="${id}"]`);

function event(over: Record<string, unknown> = {}) {
  return {
    type: "rss.matched",
    group: "notify",
    name: "RSS 命中过滤规则",
    desc: "RSS 的种子命中了过滤规则",
    vars: [
      { key: "site", type: "string", desc: "站点", sample: "hdtime" },
      { key: "size", type: "string", desc: "大小", sample: "15.00 GB" },
    ],
    sample_title: "Fake.Movie",
    sample_text: "🎯 [hdtime] Fake.Movie",
    title: "",
    text: "",
    customized: false,
    ...over,
  };
}

function catalog() {
  return {
    groups: [
      { key: "notify", name: "通知", desc: "发到所有通知通道的通知" },
      { key: "reply", name: "ChatOps 命令回复", desc: "发命令时的回复，只有正文" },
      { key: "assistant", name: "AI 助手回复", desc: "" },
    ],
    common_vars: [
      { key: "title", type: "string", desc: "内置的标题" },
      { key: "text", type: "string", desc: "内置的正文" },
    ],
    events: [
      event(),
      event({
        type: "reply.status",
        group: "reply",
        name: "/status 状态概览",
        no_title: true,
        vars: [{ key: "running", type: "int", desc: "运行中的任务数", sample: 2 }],
        sample_title: "",
        sample_text: "状态概览",
      }),
      event({
        type: "rss.digest",
        name: "RSS 合并通知",
        vars: [
          { key: "count", type: "int", desc: "一共几条", sample: 2 },
          {
            key: "items",
            type: "list",
            desc: "各条通知",
            item: [
              { key: "site", type: "string", desc: "站点" },
              { key: "torrent_title", type: "string", desc: "种子标题" },
            ],
          },
        ],
      }),
    ],
  };
}

// el-input 把 data-testid 放在原生的 input / textarea 上
function field(id: string): HTMLInputElement | HTMLTextAreaElement {
  const el = q(id)!;
  if (el.tagName === "INPUT" || el.tagName === "TEXTAREA") return el as HTMLInputElement;
  return (el.querySelector("textarea") ?? el.querySelector("input"))!;
}

function typeInto(id: string, value: string) {
  const input = field(id);
  input.value = value;
  input.dispatchEvent(new Event("input"));
}

async function mountPage(c = catalog()) {
  api.list.mockResolvedValue(c);
  api.preview.mockResolvedValue({ title: "预览标题", text: "预览正文" });
  const Page = (await import("./NotifyTemplates.vue")).default;
  view = mountView(Page);
  await vi.waitFor(() => expect(q("nt-editor")).not.toBeNull());
  await flush();
}

describe("消息模板", () => {
  it("默认打开第一个事件：列出变量与内置的样子，按示例数据预览一次", async () => {
    await mountPage();
    expect(q("nt-events")!.textContent).toContain("RSS 命中过滤规则");
    expect(q("nt-events")!.textContent).toContain("RSS 合并通知");
    expect(q("nt-editor")!.textContent).toContain("RSS 命中过滤规则");
    expect(q("nt-state-pill")!.textContent).toContain("内置文字");
    expect(q("nt-builtin")!.textContent).toContain("🎯 [hdtime] Fake.Movie");
    expect(q("nt-var-title")!.textContent).toContain("{{.title}}");
    expect(q("nt-var-site")!.textContent).toContain("{{.site}}");
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenCalledWith("rss.matched", { title: "", text: "" }),
    );
    await vi.waitFor(() => expect(q("nt-preview")!.textContent).toContain("预览正文"));
    expect((q("nt-save") as HTMLButtonElement).disabled).toBe(true);
    expect((q("nt-reset") as HTMLButtonElement).disabled).toBe(true);
  });

  it("点变量插到聚焦输入框的光标处；改了以后防抖预览", async () => {
    await mountPage();
    typeInto("nt-text", "【】");
    await flush();
    const text = field("nt-text");
    text.setSelectionRange(1, 1);
    q("nt-var-site")!.click();
    await flush();
    expect(field("nt-text").value).toBe("【{{.site}}】");

    field("nt-title").dispatchEvent(new Event("focus"));
    await flush();
    q("nt-var-size")!.click();
    await flush();
    expect(field("nt-title").value).toBe("{{.size}}");
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenLastCalledWith("rss.matched", {
        title: "{{.size}}",
        text: "【{{.site}}】",
      }),
    );
    expect((q("nt-save") as HTMLButtonElement).disabled).toBe(false);
  });

  it("列表变量插一段 range，列出每一项的字段；预览出错时写明原因", async () => {
    await mountPage();
    q("nt-event-rss.digest")!.click();
    await vi.waitFor(() => expect(q("nt-editor")!.textContent).toContain("RSS 合并通知"));
    api.preview.mockRejectedValue(
      new Error(
        JSON.stringify({
          error: "invalid_template",
          detail: "模板不对（正文）：第 1 行：没有这个变量：x",
        }),
      ),
    );
    q("nt-var-items")!.click();
    await flush();
    expect(field("nt-text").value).toBe("{{range .items}}\n{{.site}} {{.torrent_title}}\n{{end}}");
    await vi.waitFor(() => expect(q("nt-preview-error")!.textContent).toContain("没有这个变量：x"));
  });

  it("保存按事件类型发表单，回来以后标成已自定义；恢复默认要确认", async () => {
    await mountPage();
    typeInto("nt-text", "{{.site}}：{{.size}}");
    await flush();
    api.save.mockResolvedValue(
      event({ text: "{{.site}}：{{.size}}", customized: true, updated_at: "2026-10-13T08:00:00Z" }),
    );
    q("nt-save")!.click();
    await vi.waitFor(() =>
      expect(api.save).toHaveBeenCalledWith("rss.matched", {
        title: "",
        text: "{{.site}}：{{.size}}",
      }),
    );
    await vi.waitFor(() => expect(q("nt-state-pill")!.textContent).toContain("已自定义"));
    expect(q("nt-event-rss.matched")!.textContent).toContain("已自定义");
    expect(ui.success).toHaveBeenCalledWith("已保存");
    // 存过以后不算有修改
    expect((q("nt-save") as HTMLButtonElement).disabled).toBe(true);

    // 恢复默认：取消就不调接口
    ui.confirm.mockRejectedValueOnce("cancel");
    q("nt-reset")!.click();
    await flush();
    expect(api.reset).not.toHaveBeenCalled();
    ui.confirm.mockResolvedValueOnce("confirm");
    api.reset.mockResolvedValue({ success: true });
    q("nt-reset")!.click();
    await vi.waitFor(() => expect(api.reset).toHaveBeenCalledWith("rss.matched"));
    await vi.waitFor(() => expect(q("nt-state-pill")!.textContent).toContain("内置文字"));
    expect(field("nt-text").value).toBe("");
  });

  it("保存失败写明原因，表单里的内容不丢", async () => {
    await mountPage();
    typeInto("nt-text", "{{.nope}}");
    await flush();
    api.save.mockRejectedValue(
      new Error(JSON.stringify({ error: "invalid_template", detail: "没有这个变量：nope" })),
    );
    q("nt-save")!.click();
    await vi.waitFor(() => expect(ui.error).toHaveBeenCalledWith("没有这个变量：nope"));
    expect(field("nt-text").value).toBe("{{.nope}}");
  });

  it("有没保存的修改时切换事件要确认：留下就不切", async () => {
    await mountPage();
    typeInto("nt-text", "改了");
    await flush();
    ui.confirm.mockRejectedValueOnce("cancel");
    q("nt-event-rss.digest")!.click();
    await flush();
    expect(q("nt-editor")!.textContent).toContain("RSS 命中过滤规则");
    expect(field("nt-text").value).toBe("改了");

    ui.confirm.mockResolvedValueOnce("confirm");
    q("nt-event-rss.digest")!.click();
    await vi.waitFor(() => expect(q("nt-editor")!.textContent).toContain("RSS 合并通知"));
    expect(field("nt-text").value).toBe("");
  });
});

describe("命令回复与助手回复", () => {
  it("事件按组列出：没有事件的组不显示", async () => {
    await mountPage();
    const groups = [...document.querySelectorAll<HTMLElement>('[data-testid^="nt-group-"]')];
    expect(groups.map((g) => g.dataset.testid)).toEqual(["nt-group-notify", "nt-group-reply"]);
    expect(q("nt-group-notify")!.textContent).toContain("RSS 命中过滤规则");
    expect(q("nt-group-notify")!.textContent).toContain("RSS 合并通知");
    expect(q("nt-group-reply")!.textContent).toContain("ChatOps 命令回复");
    expect(q("nt-group-reply")!.textContent).toContain("/status 状态概览");
  });

  it("只有正文的回复：没有标题框、标题变量与预览里的标题，保存只交正文", async () => {
    await mountPage();
    q("nt-event-reply.status")!.click();
    await flush();
    expect(q("nt-editor")!.textContent).toContain("/status 状态概览");
    expect(q("nt-title")).toBeNull();
    expect(q("nt-var-title")).toBeNull();
    expect(q("nt-var-text")).not.toBeNull();
    expect(q("nt-var-running")).not.toBeNull();
    await vi.waitFor(() =>
      expect(api.preview).toHaveBeenLastCalledWith("reply.status", { title: "", text: "" }),
    );
    await vi.waitFor(() => expect(q("nt-preview")!.textContent).toContain("预览正文"));
    expect(q("nt-preview")!.textContent).not.toContain("预览标题");
    expect(q("nt-builtin")!.textContent).toContain("状态概览");

    typeInto("nt-text", "{{.running}} 个任务在跑");
    await flush();
    api.save.mockResolvedValue({
      ...event({ type: "reply.status", group: "reply", no_title: true }),
      text: "{{.running}} 个任务在跑",
      customized: true,
    });
    q("nt-save")!.click();
    await vi.waitFor(() =>
      expect(api.save).toHaveBeenCalledWith("reply.status", {
        title: "",
        text: "{{.running}} 个任务在跑",
      }),
    );
  });
});
