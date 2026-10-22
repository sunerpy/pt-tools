// @vitest-environment happy-dom
/*
 * 钉子：AI 助手对话页（路线图 M19；界面按设计稿方向 B「居中阅读」）。没启用时指去设置；打开会话以后按 last_seq 接 SSE，
 * 增量、推理内容、回答、确认都按事件更新；待确认的操作有「确认执行 / 拒绝」，点了只发一次，已经处理过（409）时按回来的状态更新；
 * 回答完过程（思考与工具调用的时间线）自动折起来；新对话的第一条消息建会话；Enter 发送、输入法组字时的 Enter 不发；回答中可以停止；
 * 附件先传再随消息发；模型与推理等级沿用会话里最后一次选的；会话可以搜索、删除前要确认。
 *
 * XSender 是 contenteditable 编辑器，happy-dom 里不好模拟输入：换成一个 textarea 的替身，接口与真的一样
 * （getModelValue / clear / setText / focus，Enter 发 submit），真的 XSender 在 assistantComposer.test.ts 里测。
 */
import { ApiError } from "@/api";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter } from "vue-router";

import { type MountedView, mountView } from "@/test-mount";

const api = vi.hoisted(() => ({
  status: vi.fn(),
  conversations: vi.fn(),
  conversation: vi.fn(),
  create: vi.fn(),
  send: vi.fn(),
  cancel: vi.fn(),
  decide: vi.fn(),
  remove: vi.fn(),
  models: vi.fn(),
  upload: vi.fn(),
  removeAttachment: vi.fn(),
}));
const ui = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
  warning: vi.fn(),
  info: vi.fn(),
}));

vi.mock("@/api", async (orig) => {
  const real = await orig<typeof import("@/api")>();
  return {
    ...real,
    assistantApi: {
      ...api,
      eventsURL: (id: number, after: number) => `/events/${id}?after=${after}`,
      attachmentURL: (id: number) => `/files/${id}`,
    },
  };
});

vi.mock("element-plus", async (orig) => {
  const real = await orig<typeof import("element-plus")>();
  return {
    ...real,
    ElMessage: Object.assign(vi.fn(), ui),
    ElMessageBox: { confirm: vi.fn().mockResolvedValue("confirm") },
  };
});

vi.mock("vue-element-plus-x", async (orig) => {
  const real = await orig<typeof import("vue-element-plus-x")>();
  const { defineComponent, h, ref } = await import("vue");
  const XSender = defineComponent({
    name: "XSenderStub",
    props: { placeholder: String, maxLength: Number, variant: String },
    emits: ["submit", "change", "cancel", "pasteFile"],
    setup(_props, { emit, slots, expose }) {
      const text = ref("");
      expose({
        getModelValue: () => ({ text: text.value, html: "" }),
        clear: () => {
          text.value = "";
        },
        setText: (t: string) => {
          text.value = t;
        },
        focus: () => undefined,
      });
      return () =>
        h("div", { class: "xsender-stub" }, [
          slots.header?.(),
          h("textarea", {
            "data-testid": "as-input",
            value: text.value,
            onInput: (e: Event) => {
              text.value = (e.target as HTMLTextAreaElement).value;
              emit("change");
            },
            onKeydown: (e: KeyboardEvent) => {
              if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
                e.preventDefault();
                emit("submit");
              }
            },
          }),
          slots.prefix?.(),
          slots["action-list"]?.(),
        ]);
    },
  });
  return { ...real, XSender };
});

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  static readonly CLOSED = 2;
  url: string;
  readyState = 1;
  onmessage: ((e: MessageEvent<string>) => void) | null = null;
  onerror: (() => void) | null = null;
  constructor(url: string) {
    this.url = url;
    FakeEventSource.instances.push(this);
  }
  close() {
    this.readyState = 2;
  }
  emit(ev: Record<string, unknown>) {
    this.onmessage?.({ data: JSON.stringify(ev) } as MessageEvent<string>);
  }
}

let view: MountedView | null = null;
beforeEach(() => {
  FakeEventSource.instances = [];
  vi.stubGlobal("EventSource", FakeEventSource);
  vi.stubGlobal(
    "URL",
    Object.assign(URL, { createObjectURL: () => "blob:x", revokeObjectURL: () => undefined }),
  );
  api.models.mockResolvedValue({
    default: "deepseek-flash",
    models: ["deepseek-flash", "deepseek-v4-pro"],
  });
});
afterEach(() => {
  view?.unmount();
  view = null;
  vi.clearAllMocks();
  vi.unstubAllGlobals();
});

const flush = () => new Promise((r) => setTimeout(r, 0));
const q = (id: string) => document.querySelector<HTMLElement>(`[data-testid=${id}]`);
const lastES = () => FakeEventSource.instances[FakeEventSource.instances.length - 1]!;
const input = () => q("as-input") as HTMLTextAreaElement;

function type(text: string) {
  const ta = input();
  ta.value = text;
  ta.dispatchEvent(new Event("input"));
}

const conv = {
  id: 7,
  title: "下载器状态",
  entry: "web",
  running: false,
  created_at: "2026-10-11T08:00:00Z",
  updated_at: "2026-10-11T08:01:00Z",
};

function detail(over: Record<string, unknown> = {}) {
  return {
    ...conv,
    messages: [
      { id: 1, role: "user", content: "下载器怎么样", created_at: conv.created_at, run_id: "r1" },
      {
        id: 2,
        role: "assistant",
        content: "**qb** 在线",
        created_at: conv.created_at,
        run_id: "r1",
        model: "deepseek-flash",
      },
    ],
    confirmations: [],
    last_seq: 500,
    ...over,
  };
}

async function mountPage(enabled = true) {
  api.status.mockResolvedValue({
    enabled,
    model: "deepseek-flash",
    can_confirm: true,
    chatops: false,
  });
  const Page = (await import("./Assistant.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", component: Page },
      { path: "/assistant/settings", component: { template: "<div />" } },
    ],
  });
  await router.push("/");
  view = mountView(Page, { router });
  await vi.waitFor(() => expect(api.status).toHaveBeenCalled());
  await flush();
  await flush();
}

describe("AI 助手对话页", () => {
  it("没启用：写明到设置页打开，不列会话", async () => {
    api.conversations.mockResolvedValue([]);
    await mountPage(false);
    expect(q("as-disabled")!.textContent).toContain("AI 助手设置");
    expect(q("as-chat")).toBeNull();
  });

  it("打开最近的会话，按 last_seq 接事件流；增量、推理内容、回答、确认都跟着事件更新", async () => {
    api.conversations.mockResolvedValue([conv]);
    api.conversation.mockResolvedValue(detail());
    await mountPage();
    await vi.waitFor(() => expect(q("as-answer")).not.toBeNull());
    expect(q("as-answer")!.innerHTML).toContain("<strong>qb</strong>");
    expect(q("as-meta")!.textContent).toBe("deepseek-flash");
    expect(lastES().url).toBe("/events/7?after=500");
    expect(q("as-conv-7")!.textContent).toContain("下载器状态");

    const es = lastES();
    es.emit({ seq: 501, type: "run_started", run_id: "r2" });
    es.emit({
      seq: 502,
      type: "message",
      run_id: "r2",
      message: { id: 3, role: "user", content: "暂停 abc", run_id: "r2", created_at: "" },
    });
    es.emit({ seq: 503, type: "reasoning", run_id: "r2", text: "用户要暂停 abc" });
    await flush();
    expect(q("as-live-thinking")!.textContent).toContain("用户要暂停 abc");
    expect(q("as-process")!.textContent).toContain("正在思考");
    es.emit({ seq: 504, type: "delta", run_id: "r2", text: "我来暂停" });
    await flush();
    expect(q("as-live")!.textContent).toContain("我来暂停");
    expect(q("as-stop")).not.toBeNull();

    es.emit({
      seq: 505,
      type: "message",
      run_id: "r2",
      message: {
        id: 4,
        role: "assistant",
        content: "",
        reasoning: "用户要暂停 abc",
        run_id: "r2",
        created_at: "",
        tool_calls: [
          { id: "c1", name: "pause_torrent", arguments: '{"task_id":"abc"}', write: true },
        ],
      },
    });
    es.emit({
      seq: 506,
      type: "confirmation",
      confirmation: {
        id: 9,
        run_id: "r2",
        tool_call_id: "c1",
        tool: "pause_torrent",
        summary: "暂停种子：任务 abc",
        arguments: { task_id: "abc" },
        changes: [
          { label: "状态", before: "做种中", after: "已暂停" },
          { label: "保存目录", after: "/movies" },
          { label: "订阅", before: "沙丘" },
          { label: "Cookie", secret: true },
        ],
        impact: ["停止做种：分享率不再增长"],
        status: "pending",
        expires_at: "2026-10-11T08:20:00Z",
        created_at: "",
      },
    });
    await flush();
    // 确认卡片列出改动（改以前划掉 → 改以后；新增只写改以后；删掉的写删除；密钥不写原文）与可能的影响
    const rows = [...q("as-diff-9")!.querySelectorAll(".as-diff__row")].map((r) =>
      (r.textContent ?? "").replace(/\s+/g, ""),
    );
    expect(rows).toEqual([
      "状态做种中→已暂停",
      "保存目录/movies",
      "订阅沙丘删除",
      "Cookie已更新（不显示）",
    ]);
    expect(q("as-impact-9")!.textContent).toContain("停止做种：分享率不再增长");
    expect(q("as-process-body")!.textContent).toContain("暂停种子");
    expect(q("as-process")!.textContent).toContain("等你确认");
    expect(q("as-confirm-9")!.textContent).toContain("需要你确认");
    expect(q("as-confirm-9")!.textContent).toContain("暂停种子：任务 abc");

    api.decide.mockResolvedValue({
      id: 9,
      run_id: "r2",
      tool_call_id: "c1",
      tool: "pause_torrent",
      summary: "暂停种子：任务 abc",
      arguments: {},
      status: "approved",
      decided_by: "web:admin",
      expires_at: "",
      created_at: "",
    });
    q("as-approve-9")!.click();
    await vi.waitFor(() => expect(api.decide).toHaveBeenCalledWith(9, true));
    // 确认过的不再占一张卡片：结论写在时间线那一步上
    await vi.waitFor(() => expect(q("as-confirm-9")).toBeNull());
    expect(q("as-process-body")!.textContent).toContain("已确认");
    expect(q("as-approve-9")).toBeNull();
    expect(api.decide).toHaveBeenCalledTimes(1);

    // 重连时重复送来的旧事件不再处理
    es.emit({ seq: 504, type: "delta", run_id: "r2", text: "重复" });
    es.emit({
      seq: 507,
      type: "run_finished",
      run_id: "r2",
      status: "failed",
      error: "模型服务回 429",
    });
    await flush();
    expect(q("as-live")).toBeNull();
    expect(q("as-error")!.textContent).toContain("模型服务回 429");
    expect(document.body.textContent).not.toContain("重复");
  });

  it("回答完过程折成一行，点开能看（还是同一个按钮）；进行中展开", async () => {
    api.conversations.mockResolvedValue([conv]);
    api.conversation.mockResolvedValue(
      detail({
        messages: [
          { id: 1, role: "user", content: "站点怎么样", created_at: "", run_id: "r1" },
          {
            id: 2,
            role: "assistant",
            content: "",
            reasoning: "查一下站点",
            created_at: "",
            run_id: "r1",
            tool_calls: [{ id: "c1", name: "get_site_userinfo", arguments: "{}" }],
          },
          {
            id: 3,
            role: "tool",
            content: '{"items":[]}',
            created_at: "",
            run_id: "r1",
            tool_call_id: "c1",
          },
          { id: 4, role: "assistant", content: "都正常", created_at: "", run_id: "r1" },
        ],
      }),
    );
    await mountPage();
    await vi.waitFor(() => expect(q("as-answer")).not.toBeNull());
    expect(q("as-answer")!.textContent!.trim()).toBe("都正常");
    const head = q("as-process")!;
    expect(head.textContent).toContain("已思考 · 调用了 1 个工具");
    expect(head.getAttribute("aria-expanded")).toBe("false");
    expect(q("as-process-body")!.style.display).toBe("none");
    head.click();
    await flush();
    expect(head.getAttribute("aria-expanded")).toBe("true");
    expect(q("as-process")).toBe(head);
    expect(q("as-process-body")!.style.display).toBe("");
    expect(q("as-thinking")!.textContent).toContain("查一下站点");
    expect(q("as-process-body")!.textContent).toContain("查看站点数据");
    expect(q("as-process-body")!.textContent).toContain("没有记录");
  });

  it("确认时别人已经处理过（409）：按回来的状态更新，提示一句", async () => {
    api.conversations.mockResolvedValue([conv]);
    api.conversation.mockResolvedValue(
      detail({
        running: true,
        run_id: "r1",
        messages: [
          {
            id: 2,
            role: "assistant",
            content: "",
            created_at: "",
            run_id: "r1",
            tool_calls: [{ id: "c1", name: "delete_torrent", arguments: "{}", write: true }],
          },
        ],
        confirmations: [
          {
            id: 3,
            run_id: "r1",
            tool_call_id: "c1",
            tool: "delete_torrent",
            summary: "删除种子",
            arguments: {},
            status: "pending",
            expires_at: "2026-10-11T08:20:00Z",
            created_at: "",
          },
        ],
      }),
    );
    await mountPage();
    await vi.waitFor(() => expect(q("as-deny-3")).not.toBeNull());
    api.decide.mockRejectedValue(
      new ApiError(
        409,
        JSON.stringify({
          id: 3,
          run_id: "r1",
          tool_call_id: "c1",
          status: "expired",
          decided_by: "timeout",
        }),
      ),
    );
    q("as-deny-3")!.click();
    await vi.waitFor(() => expect(ui.info).toHaveBeenCalledWith("这个操作已经处理过了"));
    await vi.waitFor(() => expect(q("as-confirm-3")).toBeNull());
    expect(q("as-process-body")!.textContent).toContain("已过期（没有在时限内确认）");
  });

  it("新对话：第一条消息建会话再打开；Enter 发送，输入法组字时不发；推荐的问题填进输入框", async () => {
    api.conversations.mockResolvedValue([]);
    api.create.mockResolvedValue({
      conversation: { ...conv, id: 8, title: "你好", running: true },
    });
    api.conversation.mockResolvedValue(
      detail({ id: 8, title: "你好", running: true, run_id: "r1" }),
    );
    await mountPage();
    expect(q("as-hello")!.textContent).toContain("有什么可以帮你");
    const prompt = q("as-prompt-subs")!;
    expect(prompt.textContent).toContain("我的订阅还缺哪些集");
    prompt.click();
    await flush();
    expect(input().value).toBe("我的订阅还缺哪些集");

    type("你好");
    await flush();
    input().dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", isComposing: true }));
    await flush();
    expect(api.create).not.toHaveBeenCalled();
    input().dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledWith({ message: "你好" }));
    await vi.waitFor(() => expect(lastES().url).toBe("/events/8?after=500"));
    expect(q("as-conv-8")).not.toBeNull();
    expect(input().value, "发出去以后清空输入框").toBe("");
    q("as-stop")!.click();
    await vi.waitFor(() => expect(api.cancel).toHaveBeenCalledWith(8));
  });

  it("发消息的回应比事件流的 run_finished 晚到：不再卡在「回答中」", async () => {
    api.conversations.mockResolvedValue([conv]);
    api.conversation.mockResolvedValue(detail());
    await mountPage();
    await vi.waitFor(() => expect(q("as-answer")).not.toBeNull());
    const es = lastES();
    let resolveSend!: (v: unknown) => void;
    api.send.mockReturnValue(new Promise((r) => (resolveSend = r)));
    type("快问快答");
    await flush();
    q("as-send")!.click();
    await vi.waitFor(() => expect(api.send).toHaveBeenCalledWith(7, "快问快答", {}));
    const user = { id: 3, role: "user", content: "快问快答", run_id: "r9", created_at: "" };
    es.emit({ seq: 501, type: "run_started", run_id: "r9" });
    es.emit({ seq: 502, type: "message", run_id: "r9", message: user });
    es.emit({
      seq: 503,
      type: "message",
      run_id: "r9",
      message: { id: 4, role: "assistant", content: "答", run_id: "r9", created_at: "" },
    });
    es.emit({ seq: 504, type: "run_finished", run_id: "r9", status: "done" });
    await flush();
    resolveSend({ run_id: "r9", message: user });
    await flush();
    await flush();
    expect(q("as-live")).toBeNull();
    expect(q("as-send")).not.toBeNull();
    expect(q("as-stop")).toBeNull();
  });

  it("事件流说要重新读（reset）：重新读会话再接着读", async () => {
    api.conversations.mockResolvedValue([conv]);
    api.conversation.mockResolvedValue(detail());
    await mountPage();
    await vi.waitFor(() => expect(lastES()).toBeDefined());
    api.conversation.mockResolvedValue(detail({ last_seq: 900 }));
    lastES().emit({ type: "reset", last: 900, seq: 0 });
    await vi.waitFor(() => expect(lastES().url).toBe("/events/7?after=900"));
    expect(api.conversation).toHaveBeenCalledTimes(2);
  });

  it("模型与推理等级沿用会话里最后一次选的；附件先传，随消息发，发完清掉", async () => {
    api.conversations.mockResolvedValue([conv]);
    api.conversation.mockResolvedValue(
      detail({
        messages: [
          {
            id: 1,
            role: "user",
            content: "看看这张图",
            created_at: "",
            run_id: "r1",
            model: "gpt-vision",
            effort: "high",
            attachments: [
              { id: 30, name: "截图.png", kind: "image", mime: "image/png", size: 1200 },
            ],
          },
          {
            id: 2,
            role: "assistant",
            content: "是个下载器截图",
            created_at: "",
            run_id: "r1",
            model: "gpt-vision",
            effort: "high",
          },
        ],
      }),
    );
    await mountPage();
    await vi.waitFor(() => expect(q("as-answer")).not.toBeNull());
    expect(q("as-file-30")).not.toBeNull();
    expect(q("as-meta")!.textContent).toBe("gpt-vision · 推理高");

    // 不认识的文件不传
    const file = q("as-file") as HTMLInputElement;
    const pick = (files: File[]) => {
      Object.defineProperty(file, "files", { value: files, configurable: true });
      file.dispatchEvent(new Event("change"));
    };
    pick([new File(["x"], "a.pdf", { type: "application/pdf" })]);
    await flush();
    expect(ui.warning).toHaveBeenCalledWith(expect.stringContaining("不能发"));
    expect(api.upload).not.toHaveBeenCalled();

    api.upload.mockResolvedValue({
      id: 41,
      name: "qb.log",
      kind: "text",
      mime: "text/plain; charset=utf-8",
      size: 20,
    });
    pick([new File(["错误：tracker 403"], "qb.log", { type: "" })]);
    await vi.waitFor(() => expect(api.upload).toHaveBeenCalledTimes(1));
    await vi.waitFor(() => expect(q("as-attachments")!.textContent).toContain("qb.log"));

    api.send.mockResolvedValue({
      run_id: "r2",
      message: { id: 5, role: "user", content: "", created_at: "", run_id: "r2", attachments: [] },
    });
    // 只发附件、不写字也行
    await vi.waitFor(() => expect((q("as-send") as HTMLButtonElement).disabled).toBe(false));
    q("as-send")!.click();
    await vi.waitFor(() =>
      expect(api.send).toHaveBeenCalledWith(7, "", {
        model: "gpt-vision",
        effort: "high",
        attachment_ids: [41],
      }),
    );
    await vi.waitFor(() => expect(q("as-attachments")).toBeNull());
  });

  it("会话列表：按标题搜索；删除前要确认，正在回答的先停", async () => {
    const other = { ...conv, id: 9, title: "站点保号", updated_at: "2026-10-01T08:00:00Z" };
    const busy = { ...conv, id: 10, title: "正在整理", running: true };
    api.conversations.mockResolvedValue([conv, other, busy]);
    api.conversation.mockResolvedValue(detail());
    await mountPage();
    await vi.waitFor(() => expect(q("as-conv-9")).not.toBeNull());
    expect(q("as-conv-7")!.getAttribute("aria-current")).toBe("true");
    expect(q("as-conv-10")!.textContent).toContain("回答中");

    const search = q("as-conv-search") as HTMLInputElement;
    search.value = "保号";
    search.dispatchEvent(new Event("input"));
    await flush();
    expect(q("as-conv-7")).toBeNull();
    expect(q("as-conv-9")).not.toBeNull();
    search.value = "没有这个";
    search.dispatchEvent(new Event("input"));
    await flush();
    expect(q("as-convs")!.textContent).toContain("没有标题里带「没有这个」的会话");
    search.value = "";
    search.dispatchEvent(new Event("input"));
    await flush();

    q("as-del-10")!.click();
    await flush();
    expect(ui.warning).toHaveBeenCalledWith("这个会话正在回答：先停止再删");
    expect(api.remove).not.toHaveBeenCalled();

    api.remove.mockResolvedValue({ ok: true });
    q("as-del-7")!.click();
    await vi.waitFor(() => expect(api.remove).toHaveBeenCalledWith(7));
    await vi.waitFor(() => expect(q("as-conv-7")).toBeNull());
    // 删的是打开着的那个：换成新对话
    expect(q("as-hello")).not.toBeNull();
  });

  it("上下文用量：输入框旁显示百分比，跟着 usage 事件变；压缩时那一行写「正在压缩上下文」", async () => {
    api.conversations.mockResolvedValue([conv]);
    api.conversation.mockResolvedValue(
      detail({
        usage: {
          context: 12_800,
          window: 128_000,
          window_source: "provider",
          auto_compact: true,
          compact_percent: 80,
          input: 5000,
          output: 300,
        },
      }),
    );
    await mountPage();
    await vi.waitFor(() => expect(q("as-usage")).not.toBeNull());
    expect(q("as-usage")!.textContent).toContain("10%");
    expect(q("as-usage")!.classList.contains("is-ok")).toBe(true);

    const es = lastES();
    es.emit({ seq: 501, type: "run_started", run_id: "r2" });
    es.emit({ seq: 502, type: "compacting", run_id: "r2" });
    await flush();
    expect(q("as-process")!.textContent).toContain("正在压缩上下文");
    es.emit({
      seq: 503,
      type: "usage",
      run_id: "r2",
      usage: {
        context: 108_000,
        window: 128_000,
        window_source: "provider",
        auto_compact: true,
        compact_percent: 80,
        input: 9000,
        output: 400,
      },
    });
    await flush();
    expect(q("as-usage")!.textContent).toContain("84%");
    expect(q("as-usage")!.classList.contains("is-dang")).toBe(true);
  });

  it("新对话没有用量：不显示", async () => {
    api.conversations.mockResolvedValue([]);
    await mountPage();
    expect(q("as-hello")).not.toBeNull();
    expect(q("as-usage")).toBeNull();
  });

  it("删掉还没发的附件：服务端那份也删", async () => {
    api.conversations.mockResolvedValue([]);
    await mountPage();
    api.upload.mockResolvedValue({
      id: 50,
      name: "a.txt",
      kind: "text",
      mime: "text/plain",
      size: 1,
    });
    api.removeAttachment.mockResolvedValue({ ok: true });
    const file = q("as-file") as HTMLInputElement;
    Object.defineProperty(file, "files", {
      value: [new File(["x"], "a.txt", { type: "text/plain" })],
      configurable: true,
    });
    file.dispatchEvent(new Event("change"));
    await vi.waitFor(() => expect(q("as-attachments")).not.toBeNull());
    await vi.waitFor(() => expect(api.upload).toHaveBeenCalled());
    await flush();
    const del = q("as-attachments")!.querySelector<HTMLElement>(
      "[data-testid^=as-attachment-del-]",
    );
    expect(del!.getAttribute("aria-label")).toBe("删除附件 a.txt");
    del!.click();
    await vi.waitFor(() => expect(api.removeAttachment).toHaveBeenCalledWith(50));
    await vi.waitFor(() => expect(q("as-attachments")).toBeNull());
  });
});
