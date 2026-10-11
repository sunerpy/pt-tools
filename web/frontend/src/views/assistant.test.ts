// @vitest-environment happy-dom
/*
 * 钉子：AI 助手对话页（路线图 M19）。没启用时指去设置；打开会话以后按 last_seq 接 SSE，增量、回答、确认都按事件更新；
 * 待确认的操作有「确认执行 / 拒绝」，点了只发一次，已经处理过（409）时按回来的状态更新；新对话的第一条消息建会话；
 * Enter 发送、输入法组字时的 Enter 不发；回答中可以停止。
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

  it("打开最近的会话，按 last_seq 接事件流；增量、回答、确认都跟着事件更新", async () => {
    api.conversations.mockResolvedValue([conv]);
    api.conversation.mockResolvedValue(detail());
    await mountPage();
    await vi.waitFor(() => expect(q("as-answer")).not.toBeNull());
    expect(q("as-answer")!.innerHTML).toContain("<strong>qb</strong>");
    expect(lastES().url).toBe("/events/7?after=500");

    const es = lastES();
    es.emit({ seq: 501, type: "run_started", run_id: "r2" });
    es.emit({
      seq: 502,
      type: "message",
      run_id: "r2",
      message: { id: 3, role: "user", content: "暂停 abc", run_id: "r2", created_at: "" },
    });
    es.emit({ seq: 503, type: "delta", run_id: "r2", text: "我来暂停" });
    await flush();
    expect(q("as-live")!.textContent).toContain("我来暂停");
    expect(q("as-stop")).not.toBeNull();

    es.emit({
      seq: 504,
      type: "message",
      run_id: "r2",
      message: {
        id: 4,
        role: "assistant",
        content: "",
        run_id: "r2",
        created_at: "",
        tool_calls: [
          { id: "c1", name: "pause_torrent", arguments: '{"task_id":"abc"}', write: true },
        ],
      },
    });
    es.emit({
      seq: 505,
      type: "confirmation",
      confirmation: {
        id: 9,
        run_id: "r2",
        tool_call_id: "c1",
        tool: "pause_torrent",
        summary: "暂停种子：任务 abc",
        arguments: { task_id: "abc" },
        status: "pending",
        expires_at: "2026-10-11T08:20:00Z",
        created_at: "",
      },
    });
    await flush();
    expect(q("as-call-pause_torrent")!.textContent).toContain("暂停种子");
    expect(q("as-confirm-9")!.textContent).toContain("等你确认");
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
    await vi.waitFor(() => expect(q("as-confirm-9")!.textContent).toContain("已确认"));
    expect(q("as-approve-9")).toBeNull();
    expect(api.decide).toHaveBeenCalledTimes(1);

    // 重连时重复送来的旧事件不再处理
    es.emit({ seq: 503, type: "delta", run_id: "r2", text: "重复" });
    es.emit({
      seq: 506,
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
    await vi.waitFor(() => expect(q("as-confirm-3")!.textContent).toContain("没有在时限内确认"));
  });

  it("新对话：第一条消息建会话再打开；Enter 发送，输入法组字时不发", async () => {
    api.conversations.mockResolvedValue([]);
    api.create.mockResolvedValue({
      conversation: { ...conv, id: 8, title: "你好", running: true },
    });
    api.conversation.mockResolvedValue(
      detail({ id: 8, title: "你好", running: true, run_id: "r1" }),
    );
    await mountPage();
    // el-input 把 data-testid 放在原生的 textarea 上
    const box = q("as-input")!;
    const ta = (
      box.tagName === "TEXTAREA" ? box : box.querySelector("textarea")!
    ) as HTMLTextAreaElement;
    ta.value = "你好";
    ta.dispatchEvent(new Event("input"));
    await flush();
    ta.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", isComposing: true }));
    await flush();
    expect(api.create).not.toHaveBeenCalled();
    ta.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    await vi.waitFor(() => expect(api.create).toHaveBeenCalledWith({ message: "你好" }));
    await vi.waitFor(() => expect(lastES().url).toBe("/events/8?after=500"));
    expect(q("as-conv-8")).not.toBeNull();
    q("as-stop")!.click();
    await vi.waitFor(() => expect(api.cancel).toHaveBeenCalledWith(8));
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
});
