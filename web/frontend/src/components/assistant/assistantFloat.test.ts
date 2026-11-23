// @vitest-environment happy-dom
/*
 * 钉子：每一页右下角的 AI 助手（悬浮按钮 + 悬浮对话窗）。
 * 没启用不出现、对话页本身不放；点开接着最近打开的会话（没有就是新对话），新对话发第一条消息建会话；
 * 「全屏」进 /assistant?c=<编号>；收起时没在回答就断开事件流，在回答就留着、回答完再断开，按钮上转圈、标等确认的个数；
 * Esc 收起、焦点回到按钮；按住上下拖动改位置（记在浏览器里），拖完松手不算点开。
 *
 * XSender 是 contenteditable 编辑器，happy-dom 里不好模拟输入：换成一个 textarea 的替身（同 views/assistant.test.ts）。
 */
import { ApiError } from "@/api";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createMemoryHistory, createRouter, type Router } from "vue-router";

import {
  refreshAssistantStatus,
  resetAssistantStatusForTest,
} from "@/composables/useAssistantStatus";
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
  return { ...real, ElMessage: Object.assign(vi.fn(), ui) };
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
  localStorage.clear();
  resetAssistantStatusForTest();
  api.models.mockResolvedValue({ default: "deepseek-flash", models: ["deepseek-flash"] });
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
const visible = (el: HTMLElement | null) => !!el && el.style.display !== "none";

const conv = {
  id: 7,
  title: "下载器状态",
  entry: "web",
  running: false,
  created_at: "2026-11-23T08:00:00Z",
  updated_at: "2026-11-23T08:01:00Z",
};

function detail(over: Record<string, unknown> = {}) {
  return {
    ...conv,
    messages: [
      { id: 1, role: "user", content: "下载器怎么样", created_at: conv.created_at, run_id: "r1" },
      { id: 2, role: "assistant", content: "qb 在线", created_at: conv.created_at, run_id: "r1" },
    ],
    confirmations: [],
    last_seq: 500,
    ...over,
  };
}

const Stub = { template: "<div />" };

async function mountFloat(enabled = true, path = "/downloaders"): Promise<Router> {
  api.status.mockResolvedValue({
    enabled,
    model: "deepseek-flash",
    can_confirm: true,
    chatops: false,
  });
  const Float = (await import("./AssistantFloat.vue")).default;
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/downloaders", name: "downloaders", component: Stub },
      { path: "/assistant", name: "assistant", component: Stub },
    ],
  });
  await router.push(path);
  view = mountView(Float, { router });
  await vi.waitFor(() => expect(api.status).toHaveBeenCalled());
  await flush();
  await flush();
  return router;
}

async function openPanel() {
  q("as-fab")!.click();
  await vi.waitFor(() => expect(visible(q("as-float"))).toBe(true));
  await flush();
}

describe("右下角的 AI 助手", () => {
  it("没启用：不出现", async () => {
    await mountFloat(false);
    expect(q("as-fab")).toBeNull();
    expect(q("as-float")).toBeNull();
  });

  it("对话页本身不放；离开对话页就出现", async () => {
    const router = await mountFloat(true, "/assistant");
    expect(q("as-fab")).toBeNull();
    await router.push("/downloaders");
    await flush();
    expect(q("as-fab")).not.toBeNull();
    expect(q("as-fab")!.getAttribute("aria-label")).toBe("打开 AI 助手");
  });

  it("设置里打开以后再读一次状态：按钮跟着出现，不用刷新页面", async () => {
    await mountFloat(false);
    expect(q("as-fab")).toBeNull();
    api.status.mockResolvedValue({ enabled: true, model: "m", can_confirm: true, chatops: false });
    await refreshAssistantStatus();
    await flush();
    expect(q("as-fab")).not.toBeNull();
  });

  it("没点开以前不读会话；点开接着最近打开的会话，按 last_seq 接事件流", async () => {
    localStorage.setItem("assistant-last-conversation", "7");
    api.conversation.mockResolvedValue(detail());
    await mountFloat();
    expect(q("as-float")).toBeNull();
    expect(api.conversation).not.toHaveBeenCalled();

    await openPanel();
    await vi.waitFor(() => expect(api.conversation).toHaveBeenCalledWith(7));
    await vi.waitFor(() => expect(lastES().url).toBe("/events/7?after=500"));
    expect(q("as-float-sub")!.textContent).toBe("下载器状态");
    expect(q("as-answer")!.textContent).toContain("qb 在线");
    expect(visible(q("as-fab")), "开着对话窗时按钮藏起来").toBe(false);
  });

  it("没有最近的会话：新对话，第一条消息建会话、记成最近打开的会话", async () => {
    api.create.mockResolvedValue({ conversation: { ...conv, id: 8, title: "你好" } });
    api.conversation.mockResolvedValue(
      detail({ id: 8, title: "你好", running: true, run_id: "r1" }),
    );
    await mountFloat();
    await openPanel();
    expect(api.conversation).not.toHaveBeenCalled();
    expect(q("as-hello")).not.toBeNull();
    expect(document.querySelectorAll(".as-prompt")).toHaveLength(4);
    expect(q("as-float-sub")!.textContent).toBe("新对话");

    q("as-prompt-dl")!.click();
    await flush();
    const input = q("as-input") as HTMLTextAreaElement;
    expect(input.value).toBe("下载器现在有多少在下载");
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    await vi.waitFor(() =>
      expect(api.create).toHaveBeenCalledWith({ message: "下载器现在有多少在下载" }),
    );
    await vi.waitFor(() => expect(lastES().url).toBe("/events/8?after=500"));
    expect(q("as-float-sub")!.textContent).toBe("你好");
    expect(localStorage.getItem("assistant-last-conversation")).toBe("8");
  });

  it("最近打开的会话已经删了（404）：换成新对话，不报错", async () => {
    localStorage.setItem("assistant-last-conversation", "9");
    api.conversation.mockRejectedValue(new ApiError(404, "会话不存在"));
    await mountFloat();
    await openPanel();
    await vi.waitFor(() => expect(api.conversation).toHaveBeenCalledWith(9));
    await vi.waitFor(() => expect(q("as-hello")).not.toBeNull());
    expect(ui.error).not.toHaveBeenCalled();
    expect(localStorage.getItem("assistant-last-conversation")).toBeNull();
  });

  it("全屏：进 /assistant?c=<编号>，对话窗拆掉、事件流断开；新对话是 ?c=new", async () => {
    localStorage.setItem("assistant-last-conversation", "7");
    api.conversation.mockResolvedValue(detail());
    const router = await mountFloat();
    await openPanel();
    await vi.waitFor(() => expect(lastES().url).toBe("/events/7?after=500"));
    const es = lastES();

    q("as-float-full")!.click();
    await vi.waitFor(() => expect(router.currentRoute.value.fullPath).toBe("/assistant?c=7"));
    await flush();
    expect(q("as-float")).toBeNull();
    expect(q("as-fab")).toBeNull();
    expect(es.readyState).toBe(FakeEventSource.CLOSED);

    // 回来再打开：接着最近打开的会话（对话页里换过的话就是换过的那个）
    localStorage.setItem("assistant-last-conversation", "7");
    await router.push("/downloaders");
    await flush();
    await openPanel();
    q("as-float-new")!.click();
    await flush();
    expect(q("as-hello")).not.toBeNull();
    q("as-float-full")!.click();
    await vi.waitFor(() => expect(router.currentRoute.value.fullPath).toBe("/assistant?c=new"));
  });

  it("收起：没在回答就断开事件流，再打开从读到的最后一个事件接着读；Esc 也能收起，焦点回到按钮", async () => {
    localStorage.setItem("assistant-last-conversation", "7");
    api.conversation.mockResolvedValue(detail());
    await mountFloat();
    await openPanel();
    await vi.waitFor(() => expect(lastES().url).toBe("/events/7?after=500"));
    const first = lastES();
    first.emit({ seq: 501, type: "usage", run_id: "r1", usage: null });

    q("as-float-close")!.click();
    // 收起有一段淡出（Transition 等两帧才写 display: none）
    await vi.waitFor(() => expect(visible(q("as-float"))).toBe(false));
    expect(visible(q("as-fab"))).toBe(true);
    expect(first.readyState).toBe(FakeEventSource.CLOSED);
    expect(document.activeElement).toBe(q("as-fab"));

    await openPanel();
    expect(lastES()).not.toBe(first);
    expect(lastES().url).toBe("/events/7?after=501");
    expect(api.conversation, "收起再打开不重新读会话").toHaveBeenCalledTimes(1);

    q("as-float")!.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await vi.waitFor(() => expect(visible(q("as-float"))).toBe(false));
    expect(document.activeElement).toBe(q("as-fab"));
  });

  it("收起时正在回答：事件流留着，按钮上转圈、标等确认的个数，回答完再断开", async () => {
    localStorage.setItem("assistant-last-conversation", "7");
    api.conversation.mockResolvedValue(detail());
    await mountFloat();
    await openPanel();
    await vi.waitFor(() => expect(lastES().url).toBe("/events/7?after=500"));
    const es = lastES();
    es.emit({ seq: 501, type: "run_started", run_id: "r2" });
    await flush();

    q("as-float-close")!.click();
    await flush();
    expect(es.readyState, "在回答：不断开").toBe(1);
    const fab = q("as-fab")!;
    expect(fab.querySelector(".as-fab__ring")).not.toBeNull();
    expect(fab.getAttribute("aria-label")).toBe("AI 助手：正在回答");

    es.emit({
      seq: 502,
      type: "confirmation",
      confirmation: {
        id: 3,
        run_id: "r2",
        tool_call_id: "c1",
        tool: "pause_torrent",
        summary: "暂停种子",
        arguments: {},
        status: "pending",
        expires_at: "2026-11-23T08:20:00Z",
        created_at: "",
      },
    });
    await flush();
    expect(q("as-fab-badge")!.textContent).toBe("1");
    expect(fab.getAttribute("aria-label")).toBe("AI 助手：1 个操作等你确认");

    es.emit({ seq: 503, type: "run_finished", run_id: "r2", status: "done" });
    await flush();
    expect(es.readyState, "回答完：断开").toBe(FakeEventSource.CLOSED);
    expect(fab.querySelector(".as-fab__ring")).toBeNull();
  });

  it("开着对话窗时设置里换了默认模型：跟着新的默认走，不把旧模型当成自己选的发出去", async () => {
    localStorage.setItem("assistant-last-conversation", "7");
    api.conversation.mockResolvedValue(detail());
    await mountFloat();
    await openPanel();
    await vi.waitFor(() => expect(lastES().url).toBe("/events/7?after=500"));
    api.status.mockResolvedValue({
      enabled: true,
      model: "new-model",
      can_confirm: true,
      chatops: false,
    });
    await refreshAssistantStatus();
    await flush();
    api.send.mockResolvedValue({
      run_id: "r2",
      message: { id: 3, role: "user", content: "还好吗", created_at: "", run_id: "r2" },
    });
    const input = q("as-input") as HTMLTextAreaElement;
    input.value = "还好吗";
    input.dispatchEvent(new Event("input"));
    await flush();
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
    await vi.waitFor(() => expect(api.send).toHaveBeenCalledWith(7, "还好吗", {}));
  });

  it("按住上下拖动：按钮跟着抬高、位置记下来，松手不算点开；再点一下才打开", async () => {
    api.conversation.mockResolvedValue(detail());
    await mountFloat();
    const fab = q("as-fab")!;
    const rail = fab.parentElement!;
    Object.defineProperty(rail, "clientHeight", { value: 600, configurable: true });
    Object.defineProperty(fab, "offsetHeight", { value: 48, configurable: true });
    const pointer = (type: string, clientY: number) =>
      fab.dispatchEvent(
        new PointerEvent(type, { pointerId: 1, button: 0, clientY, bubbles: true }),
      );

    pointer("pointerdown", 500);
    pointer("pointermove", 497);
    await flush();
    expect(fab.style.getPropertyValue("--as-fab-lift"), "抖几个像素不算拖").toBe("0px");
    pointer("pointermove", 380);
    await flush();
    expect(fab.style.getPropertyValue("--as-fab-lift")).toBe("120px");
    pointer("pointermove", -500);
    await flush();
    expect(fab.style.getPropertyValue("--as-fab-lift"), "最高抬到轨道顶").toBe("552px");
    pointer("pointermove", 300);
    pointer("pointerup", 300);
    fab.click();
    await flush();
    expect(q("as-float"), "拖完松手不打开").toBeNull();
    expect(localStorage.getItem("assistant-fab-lift")).toBe("200");

    fab.click();
    await vi.waitFor(() => expect(visible(q("as-float"))).toBe(true));
  });
});
