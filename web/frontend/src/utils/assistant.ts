/*
 * AI 助手（路线图 M19）页面用的纯函数：服务商预设、回答的 Markdown 渲染、事件流的归并与消息的分组。
 * 都不碰 DOM 以外的状态，方便单测。
 */
import type {
  AssistantAttachment,
  AssistantConfirmation,
  AssistantConfirmStatus,
  AssistantConversationDetail,
  AssistantEffort,
  AssistantEvent,
  AssistantMessage,
  AssistantToolCall,
  AssistantUsage,
} from "@/api";
import { formatBytes } from "@/utils/format";
import DOMPurify from "dompurify";
import { Marked } from "marked";

/**
 * 服务商预设：选了以后填好接口地址与常用模型；模型也可以手填，或者点「获取模型列表」从服务商读。
 * 地址、模型与申请 Key 的页面 2026-10 按各家文档核对过（Groq、Google AI Studio 2026-10-12）；免费额度会变，界面上写明以服务商页面为准。
 */
export interface AssistantProvider {
  key: string;
  label: string;
  baseURL: string;
  models: string[];
  /** 不要 API Key（本机的 Ollama） */
  noKey?: boolean;
  hint?: string;
  /** 申请 API Key 的页面 */
  consoleURL?: string;
  /** 免费额度的一句话 */
  free?: string;
  /** 隐私上要知道的，例如免费层的内容可能被拿去改进产品 */
  privacy?: string;
  /** 从中国大陆访问一般要代理 */
  needsProxy?: boolean;
}

export const ASSISTANT_PROVIDERS: readonly AssistantProvider[] = [
  {
    key: "deepseek",
    label: "DeepSeek",
    baseURL: "https://api.deepseek.com",
    models: ["deepseek-flash", "deepseek-v4-pro"],
    consoleURL: "https://platform.deepseek.com/api_keys",
  },
  {
    key: "qwen",
    label: "通义千问（阿里云百炼）",
    baseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    models: ["qwen3.7-plus", "qwen3.8-flash", "qwen3.8-max"],
    consoleURL: "https://bailian.console.aliyun.com/cn-beijing/model/settings/api-key",
    free: "新开通的账号每个模型各有一份免费额度，用完开始计费；在百炼控制台给模型打开「免费额度用完即停」就不会多花钱。",
  },
  {
    key: "kimi",
    label: "Kimi（月之暗面）",
    baseURL: "https://api.moonshot.cn/v1",
    models: ["kimi-k2.6", "kimi-k3"],
    consoleURL: "https://platform.moonshot.cn/console/api-keys",
  },
  {
    key: "siliconflow",
    label: "硅基流动",
    baseURL: "https://api.siliconflow.cn/v1",
    models: [],
    consoleURL: "https://cloud.siliconflow.cn/account/ak",
    free: "模型广场里标着「免费」的模型可以免费用，有限速。",
  },
  {
    key: "google",
    label: "Google AI Studio（Gemini）",
    baseURL: "https://generativelanguage.googleapis.com/v1beta/openai",
    // 2026-10-12 用真实 Key 核对：gemini-3.5-flash-lite 网页与通知渠道里调工具、确认推送都跑通，回得快；
    // gemini-flash-latest 跟着最新的 Flash 走，忙的时候慢、会回 503；gemini-2.5-* 对新用户已经不开放
    models: [
      "gemini-3.5-flash-lite",
      "gemini-flash-latest",
      "gemini-3.8-flash",
      "gemini-3.6-flash",
      "gemini-flash-lite-latest",
    ],
    consoleURL: "https://aistudio.google.com/apikey",
    free: "Gemini 的 Flash 等模型有免费额度，每分钟、每天限次数；有的模型每分钟只有几次，一个问题要调好几次工具时会碰到限流，等一会儿再问。",
    privacy: "免费层的对话内容可能被 Google 用来改进产品。",
    needsProxy: true,
  },
  {
    key: "groq",
    label: "Groq",
    baseURL: "https://api.groq.com/openai/v1",
    models: ["qwen/qwen3.8-27b", "openai/gpt-oss-120b", "openai/gpt-oss-20b"],
    consoleURL: "https://console.groq.com/keys",
    free: "这几个模型免费，每个每天约 1000 次请求、20 万 token；每分钟只有 8000 token，要调好几次工具的问题可能要多等一会儿。",
    needsProxy: true,
  },
  {
    key: "openrouter",
    label: "OpenRouter",
    baseURL: "https://openrouter.ai/api/v1",
    models: [],
    consoleURL: "https://openrouter.ai/settings/keys",
    free: "名字带 :free 的模型免费，每天有次数上限。",
  },
  {
    key: "ollama",
    label: "Ollama（自己部署）",
    baseURL: "http://127.0.0.1:11434/v1",
    models: [],
    noKey: true,
    hint: "pt-tools 在 Docker 里时，地址要写宿主机或 Ollama 容器的地址，不是 127.0.0.1；模型名用 ollama list 里的",
  },
  {
    key: "openai",
    label: "OpenAI",
    baseURL: "https://api.openai.com/v1",
    models: ["gpt-6.1-sol", "gpt-6-luna", "gpt-6-astra"],
    consoleURL: "https://platform.openai.com/api-keys",
    needsProxy: true,
  },
  { key: "custom", label: "其他（OpenAI 兼容接口）", baseURL: "", models: [] },
];

export function providerOf(key: string): AssistantProvider {
  return (
    ASSISTANT_PROVIDERS.find((p) => p.key === key) ??
    ASSISTANT_PROVIDERS[ASSISTANT_PROVIDERS.length - 1]!
  );
}

/*
 * 回答的 Markdown。模型可能被种子标题、站内信里的指令带偏，所以两道防线：
 *   ① marked 这一层就不产出危险的东西：原样 HTML 当文字转义，图片只留替代文字（图片地址能把数据带出去），
 *      链接只认 http、https、mailto，并在新标签页打开；
 *   ② 再经 DOMPurify，只留排版标签。
 */
function escapeHTML(s: string): string {
  return s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
}

const marked = new Marked({
  gfm: true,
  breaks: true,
  renderer: {
    html({ text }) {
      return escapeHTML(text);
    },
    image({ text }) {
      return escapeHTML(text);
    },
    link({ href, title, tokens }) {
      const inner = this.parser.parseInline(tokens);
      if (!/^(https?:|mailto:)/i.test(href.trim())) return inner;
      const t = title ? ` title="${escapeHTML(title)}"` : "";
      return `<a href="${escapeHTML(href)}"${t} target="_blank" rel="noopener noreferrer nofollow">${inner}</a>`;
    },
  },
});
const purify = DOMPurify(window);
const ALLOWED_TAGS = [
  "p",
  "br",
  "strong",
  "em",
  "del",
  "code",
  "pre",
  "ul",
  "ol",
  "li",
  "blockquote",
  "a",
  "hr",
  "table",
  "thead",
  "tbody",
  "tr",
  "th",
  "td",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
];

export function renderAnswer(text: string): string {
  if (!text) return "";
  const html = marked.parse(text, { async: false }) as string;
  return purify.sanitize(html, {
    ALLOWED_TAGS,
    ALLOWED_ATTR: ["href", "title", "target", "rel"],
  });
}

/**
 * 进行中的回答：最后一行只有 - 或 = 时先不算进去 —— 流到一半的「文字\n-」会被当成 Setext 标题，
 * 下一段一到又变回列表，画面跳一下。
 */
export function renderPartial(text: string): string {
  return renderAnswer(text.replace(/\n[ \t]*[-=]+[ \t]*$/, ""));
}

/** 页面上一个会话的状态：读会话得到，之后按事件归并。 */
export interface ChatState {
  messages: AssistantMessage[];
  confirmations: AssistantConfirmation[];
  running: boolean;
  runId: string;
  /** 进行中的回答这一步到现在的文字 */
  partial: string;
  /** 进行中的回答这一步到现在的推理内容（思考过程） */
  partialReasoning: string;
  /** 正在执行的工具 */
  tool: AssistantToolCall | null;
  lastSeq: number;
  /** 最近一次运行失败的原因 */
  error: string;
  /** 上下文用量（没有时为 null） */
  usage: AssistantUsage | null;
  /** 正在压缩上下文（写摘要要一会儿；压缩完有一条提示） */
  compacting: boolean;
}

export function chatStateFrom(d: AssistantConversationDetail): ChatState {
  return {
    messages: [...d.messages],
    confirmations: [...d.confirmations],
    running: d.running,
    runId: d.run_id ?? "",
    partial: d.running ? (d.partial ?? "") : "",
    partialReasoning: d.running ? (d.partial_reasoning ?? "") : "",
    tool: null,
    lastSeq: d.last_seq,
    error: "",
    usage: d.usage ?? null,
    compacting: false,
  };
}

function upsert<T extends { id: number }>(list: T[], item: T): T[] {
  const i = list.findIndex((x) => x.id === item.id);
  if (i < 0) return [...list, item].sort((a, b) => a.id - b.id);
  const out = [...list];
  out[i] = item;
  return out;
}

/**
 * 归并一个事件；返回 null 表示要重新读会话（reset）。序号不比 lastSeq 大的事件已经处理过，原样返回。
 */
export function applyEvent(s: ChatState, ev: AssistantEvent): ChatState | null {
  if (ev.type === "reset") return null;
  if (ev.seq <= s.lastSeq) return s;
  const next: ChatState = { ...s, lastSeq: ev.seq };
  switch (ev.type) {
    case "run_started":
      return {
        ...next,
        running: true,
        runId: ev.run_id ?? "",
        partial: "",
        partialReasoning: "",
        tool: null,
        error: "",
        compacting: false,
      };
    case "delta":
      return { ...next, partial: ev.replace ? (ev.text ?? "") : next.partial + (ev.text ?? "") };
    case "reasoning":
      return {
        ...next,
        partialReasoning: ev.replace ? (ev.text ?? "") : next.partialReasoning + (ev.text ?? ""),
      };
    case "message": {
      if (!ev.message) return next;
      const m = ev.message;
      const out = { ...next, messages: upsert(next.messages, m) };
      if (m.role === "assistant" || m.role === "notice") {
        out.partial = "";
        out.partialReasoning = "";
        out.compacting = false;
      }
      if (m.role === "tool") out.tool = null;
      return out;
    }
    case "tool_started":
      return { ...next, tool: ev.tool ?? null };
    case "confirmation":
      return ev.confirmation
        ? { ...next, confirmations: upsert(next.confirmations, ev.confirmation) }
        : next;
    case "run_finished":
      return {
        ...next,
        running: false,
        partial: "",
        partialReasoning: "",
        tool: null,
        error: ev.status === "failed" ? (ev.error ?? "") : "",
        compacting: false,
      };
    case "usage":
      return ev.usage ? { ...next, usage: ev.usage } : next;
    case "compacting":
      return { ...next, compacting: true };
    default:
      // 新版本服务端多出来的事件：只记序号
      return next;
  }
}

/** 一个工具调用在页面上的样子：调用、结果、确认。 */
export interface CallView {
  call: AssistantToolCall;
  result?: AssistantMessage;
  confirmation?: AssistantConfirmation;
}

/** 一次回答里的一步：模型的一条消息（推理内容、文字与它要调的工具）。prevAt 是它前一条消息的时间（算这一步的用时）。 */
export interface TurnStep {
  message: AssistantMessage;
  calls: CallView[];
  prevAt?: string;
}

/**
 * 一次回答：同一次运行里连着的几条模型消息合成一项。最后一步没有工具调用时它的文字是回答；
 * 别的步骤（推理、工具调用与中间说的话）是「过程」，回答完以后折起来。
 */
export interface ChatTurn {
  kind: "turn";
  key: string;
  runId: string;
  steps: TurnStep[];
  /** 这次回答从哪条消息之后开始、到哪条消息结束（算总用时） */
  startedAt?: string;
  endedAt?: string;
}

/** 页面上的一项：一条用户消息、一次回答、一条提示。工具结果并进它所属的调用。 */
export type ChatItem =
  | { kind: "user"; key: string; message: AssistantMessage }
  | ChatTurn
  | { kind: "notice"; key: string; message: AssistantMessage };

// 工具调用的编号在不同的运行里可能重复（有的服务每次都从 call_0 起）：结果与确认都按「运行/调用」对上
export function chatItems(
  messages: AssistantMessage[],
  confirmations: AssistantConfirmation[],
): ChatItem[] {
  const results = new Map<string, AssistantMessage>();
  for (const m of messages) {
    if (m.role === "tool" && m.tool_call_id) results.set(`${m.run_id ?? ""}/${m.tool_call_id}`, m);
  }
  const confs = new Map<string, AssistantConfirmation>();
  for (const c of confirmations) confs.set(`${c.run_id}/${c.tool_call_id}`, c);
  const out: ChatItem[] = [];
  let turn: ChatTurn | null = null;
  let prev: AssistantMessage | null = null;
  for (const m of messages) {
    const before = prev;
    prev = m;
    if (m.role === "user" || m.role === "notice") {
      out.push({ kind: m.role, key: `${m.role}-${m.id}`, message: m });
      turn = null;
      continue;
    }
    if (m.role === "tool") {
      if (turn && turn.runId === (m.run_id ?? "")) turn.endedAt = m.created_at;
      continue;
    }
    if (m.role !== "assistant") continue;
    const runId = m.run_id ?? "";
    const calls = (m.tool_calls ?? []).map((call) => ({
      call,
      result: results.get(`${runId}/${call.id}`),
      confirmation: confs.get(`${runId}/${call.id}`),
    }));
    if (!m.content && !m.reasoning && calls.length === 0) continue;
    if (!turn || turn.runId !== runId) {
      turn = { kind: "turn", key: `turn-${m.id}`, runId, steps: [], startedAt: before?.created_at };
      out.push(turn);
    }
    turn.steps.push({ message: m, calls, prevAt: before?.created_at });
    turn.endedAt = m.created_at;
  }
  return out;
}

/** 两个时间相差的秒数；任一边没有或者算不出来时为 null。 */
export function secondsBetween(from?: string, to?: string): number | null {
  if (!from || !to) return null;
  const a = Date.parse(from);
  const b = Date.parse(to);
  if (Number.isNaN(a) || Number.isNaN(b) || b < a) return null;
  return (b - a) / 1000;
}

/** 用时写成「0.8 秒」「12 秒」「2 分 5 秒」；算不出来时为空串。 */
export function durationText(sec: number | null): string {
  // 0 是两条消息记在同一刻，量不出来，不写
  if (sec == null || sec <= 0) return "";
  if (sec < 10) return `${Math.max(0.1, Math.round(sec * 10) / 10)} 秒`;
  if (sec < 60) return `${Math.round(sec)} 秒`;
  const m = Math.floor(sec / 60);
  const rest = Math.round(sec - m * 60);
  return rest ? `${m} 分 ${rest} 秒` : `${m} 分钟`;
}

/** 回答：最后一步没有工具调用时它的文字（还在调工具、或者停在工具上时没有回答）。 */
export function turnAnswer(t: ChatTurn): string {
  const last = t.steps[t.steps.length - 1];
  return last && last.calls.length === 0 ? last.message.content : "";
}

/** 过程里的步骤：回答那一步只留推理内容（文字在下面单独显示）。 */
export function processSteps(
  t: ChatTurn,
): { reasoning: string; text: string; calls: CallView[] }[] {
  const answer = turnAnswer(t);
  return t.steps
    .map((st, i) => {
      const isAnswer = i === t.steps.length - 1 && answer !== "";
      return {
        reasoning: st.message.reasoning ?? "",
        text: isAnswer ? "" : st.message.content,
        calls: st.calls,
      };
    })
    .filter((st) => st.reasoning || st.text || st.calls.length);
}

/** 过程折起来时的一句话，例如「已思考 · 调用了 3 个工具（2 轮）· 用时 12 秒」。 */
export function processSummary(t: ChatTurn): string {
  const steps = processSteps(t);
  const parts: string[] = [];
  if (steps.some((s) => s.reasoning)) parts.push("已思考");
  const calls = steps.reduce((n, s) => n + s.calls.length, 0);
  const rounds = steps.filter((s) => s.calls.length > 0).length;
  if (calls > 0) parts.push(`调用了 ${calls} 个工具${rounds > 1 ? `（${rounds} 轮）` : ""}`);
  const took = durationText(secondsBetween(t.startedAt, t.endedAt));
  if (parts.length && took) parts.push(`用时 ${took}`);
  return parts.join(" · ");
}

/** 工具结果最多显示多少字（结果在后端已经截过一次） */
const MAX_RESULT_CHARS = 4000;

/** 工具结果：JSON 排成多行，太长的截掉。 */
export function resultText(raw: string): string {
  let text = raw;
  try {
    text = JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    /* 不是 JSON：原样 */
  }
  return text.length > MAX_RESULT_CHARS
    ? `${text.slice(0, MAX_RESULT_CHARS)}\n…（太长，后面省略了）`
    : text;
}

/** 工具结果的一行摘要：列表写几项，有说明写说明，别的取开头一段。 */
export function resultPreview(raw: string): string {
  let v: unknown;
  try {
    v = JSON.parse(raw);
  } catch {
    return oneLine(raw);
  }
  if (Array.isArray(v)) return `${v.length} 项`;
  if (v && typeof v === "object") {
    const o = v as Record<string, unknown>;
    if (Array.isArray(o.items)) {
      const total = typeof o.total === "number" ? o.total : o.items.length;
      return total === 0 ? "没有记录" : `共 ${total} 项`;
    }
    if (typeof o.message === "string" && o.message) return oneLine(o.message);
    if (typeof o.created === "number" && typeof o.failed === "number") {
      return `整理出 ${o.created} 个文件${o.failed ? `，失败 ${o.failed} 个` : ""}`;
    }
    if (o.ok === true || o.success === true) return "成功";
  }
  return oneLine(raw);
}

function oneLine(s: string): string {
  const t = s.replace(/\s+/g, " ").trim();
  return t.length > 90 ? `${t.slice(0, 90)}…` : t;
}

/** 工具调用的状态：执行中、排队、等确认、成功、失败、没有执行（被拒绝、确认过期，或者回答停了还没有结果）。 */
export type ToolStatus = "running" | "queued" | "wait" | "ok" | "error" | "skipped";

/** 过程时间线的一项：一段思考、模型中间说的话，或者一个工具调用。 */
export interface TimelineEntry {
  kind: "think" | "say" | "tool";
  key: string;
  text?: string;
  call?: AssistantToolCall;
  result?: AssistantMessage;
  confirmation?: AssistantConfirmation;
  status?: ToolStatus;
  /** 用时（秒），不知道时为 null */
  seconds: number | null;
  /** 正在生成的思考 */
  live?: boolean;
}

/** 进行中的这一步（页面在流式收到的部分）；compacting 是正在压缩上下文 */
export interface LiveStep {
  reasoning: string;
  text: string;
  tool: AssistantToolCall | null;
  compacting?: boolean;
}

function toolStatus(cv: CallView, running: boolean, current: AssistantToolCall | null): ToolStatus {
  const c = cv.confirmation;
  // 拒绝、过期是没有执行，不是执行失败：不标红（服务端给模型的那条结果是错误，也不算）
  if (c && (c.status === "denied" || c.status === "expired")) return "skipped";
  if (cv.result) return cv.result.is_error ? "error" : "ok";
  if (!running) return "skipped";
  if (c?.status === "pending") return "wait";
  if (c?.status === "approved" || current?.id === cv.call.id) return "running";
  return "queued";
}

/**
 * 一次回答的过程，按发生的顺序：每一步的思考、中间说的话（回答那一步的文字不算）、工具调用；进行中时最后接上正在生成的思考。
 * 思考的用时是这一步与前一条消息的间隔；工具的用时从调用（写操作从确认）到结果。
 */
export function timeline(t: ChatTurn, running: boolean, live?: LiveStep | null): TimelineEntry[] {
  const answer = turnAnswer(t);
  const out: TimelineEntry[] = [];
  t.steps.forEach((st, i) => {
    const m = st.message;
    const isAnswer = i === t.steps.length - 1 && answer !== "";
    if (m.reasoning) {
      out.push({
        kind: "think",
        key: `${m.id}-think`,
        text: m.reasoning,
        seconds: secondsBetween(st.prevAt, m.created_at),
      });
    }
    if (!isAnswer && m.content)
      out.push({ kind: "say", key: `${m.id}-say`, text: m.content, seconds: null });
    for (const cv of st.calls) {
      out.push({
        kind: "tool",
        key: `${m.id}-${cv.call.id}`,
        call: cv.call,
        result: cv.result,
        confirmation: cv.confirmation,
        status: toolStatus(cv, running, live?.tool ?? null),
        seconds: cv.result
          ? secondsBetween(cv.confirmation?.decided_at ?? m.created_at, cv.result.created_at)
          : null,
      });
    }
  });
  // 这一步还在生成：开始出正文以后思考就结束了，不再转圈
  if (running && live?.reasoning) {
    out.push({
      kind: "think",
      key: "live-think",
      text: live.reasoning,
      seconds: null,
      live: !live.text,
    });
  }
  return out;
}

/** 进行中的过程那一行：在做什么。 */
export function liveHeadline(t: ChatTurn, live: LiveStep | null, waiting: boolean): string {
  const calls = t.steps.reduce((n, s) => n + s.calls.length, 0);
  let doing = "正在思考…";
  if (waiting) doing = "等你确认下面的操作…";
  else if (live?.compacting) doing = "正在压缩上下文…";
  else if (live?.tool) doing = `正在${toolLabel(live.tool.name)}…`;
  else if (live?.text) doing = "正在回答…";
  return calls ? `${doing} · 已调用 ${calls} 个工具` : doing;
}

/** 这次回答在等确认的写操作（卡片放在过程下面，不折起来）。 */
export function pendingConfirmations(t: ChatTurn): AssistantConfirmation[] {
  const out: AssistantConfirmation[] = [];
  for (const st of t.steps) {
    for (const c of st.calls) if (c.confirmation?.status === "pending") out.push(c.confirmation);
  }
  return out;
}

/** 回答用的模型与推理等级（写在回答下面）。 */
export function turnMeta(t: ChatTurn): string {
  const last = t.steps[t.steps.length - 1]?.message;
  if (!last?.model) return "";
  const effort = last.effort ? EFFORT_LABELS[last.effort] : "";
  return effort ? `${last.model} · 推理${effort}` : last.model;
}

/** 会话列表的分组，按最后更新的时间。 */
export const CONV_GROUPS = ["今天", "昨天", "近 7 天", "更早"] as const;
export type ConvGroup = (typeof CONV_GROUPS)[number];

const DAY_MS = 86_400_000;

export function convGroup(iso: string, now = new Date()): ConvGroup {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t)) return "更早";
  const today = new Date(now);
  today.setHours(0, 0, 0, 0);
  const start = today.getTime();
  if (t >= start) return "今天";
  if (t >= start - DAY_MS) return "昨天";
  if (t >= start - 6 * DAY_MS) return "近 7 天";
  return "更早";
}

/** 会话的时间：今天、昨天写时分，再早写月日，不是今年的带上年份。 */
export function convTime(iso: string, now = new Date()): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const p = (n: number) => String(n).padStart(2, "0");
  const g = convGroup(iso, now);
  if (g === "今天" || g === "昨天") return `${p(d.getHours())}:${p(d.getMinutes())}`;
  const md = `${p(d.getMonth() + 1)}-${p(d.getDate())}`;
  return d.getFullYear() === now.getFullYear() ? md : `${d.getFullYear()}-${md}`;
}

/** 上下文用了窗口的百分之几（取整，0–100 以上照实写） */
export function usagePercent(u: AssistantUsage): number {
  if (!u.window) return 0;
  return Math.round((u.context / u.window) * 100);
}

/** 用量指示的颜色：到了压缩阈值是红，离阈值不到 15 个百分点是黄，其余是绿 */
export function usageTone(u: AssistantUsage): "ok" | "warn" | "dang" {
  const pct = usagePercent(u);
  const limit = u.auto_compact ? u.compact_percent : 100;
  if (pct >= limit) return "dang";
  if (pct >= limit - 15) return "warn";
  return "ok";
}

const WINDOW_SOURCES: Record<AssistantUsage["window_source"], string> = {
  setting: "AI 助手设置里填的",
  provider: "服务商的模型列表报的",
  table: "按模型名估计的",
  default: "不知道这个模型的窗口，按 128K 算",
};

/** token 数写成「1,234」 */
function tokens(n: number): string {
  return Math.max(0, Math.round(n)).toLocaleString("zh-CN");
}

/** 用量指示的提示：几行字 */
export function usageLines(u: AssistantUsage): string[] {
  const lines = [
    `上下文：${u.estimated ? "约 " : ""}${tokens(u.context)} / ${tokens(u.window)} token（${usagePercent(u)}%）`,
    `窗口：${WINDOW_SOURCES[u.window_source] ?? u.window_source}`,
    u.auto_compact
      ? `自动压缩：用到 ${u.compact_percent}% 时把前面的对话压缩成摘要${u.compacted ? "（这个会话压缩过）" : ""}`
      : "自动压缩：关闭（太长时从最早的消息丢起）",
  ];
  if (u.input || u.output) {
    lines.push(`本会话共用：输入 ${tokens(u.input)} · 输出 ${tokens(u.output)} token`);
  } else {
    lines.push("服务商没有报用量：上下文是按字数估的");
  }
  return lines;
}

/** 推理等级的选项（对话框下面的下拉框）。空串是不指定（用模型自己的默认）。 */
export const EFFORT_OPTIONS: readonly { value: AssistantEffort; label: string }[] = [
  { value: "", label: "默认" },
  { value: "none", label: "不思考" },
  { value: "low", label: "低" },
  { value: "medium", label: "中" },
  { value: "high", label: "高" },
];

const EFFORT_LABELS: Record<string, string> = {
  none: "不思考",
  minimal: "最低",
  low: "低",
  medium: "中",
  high: "高",
  xhigh: "最高",
};

/*
 * 附件：和后端（internal/assistant/attachments.go）的限制一样，先在页面上挡一道，免得传了才知道不行。
 */
export const MAX_ATTACHMENTS = 4;
export const MAX_IMAGE_BYTES = 5 << 20;
export const MAX_TEXT_BYTES = 200 << 10;
const IMAGE_TYPES = ["image/png", "image/jpeg", "image/webp", "image/gif"];
export const TEXT_EXTS = [
  ".txt",
  ".md",
  ".markdown",
  ".log",
  ".json",
  ".csv",
  ".tsv",
  ".yaml",
  ".yml",
  ".toml",
  ".ini",
  ".conf",
  ".xml",
  ".nfo",
  ".srt",
  ".ass",
];
/** 文件选择框的 accept */
export const ATTACH_ACCEPT = [...IMAGE_TYPES, ...TEXT_EXTS].join(",");

function extOf(name: string): string {
  const i = name.lastIndexOf(".");
  return i < 0 ? "" : name.slice(i).toLowerCase();
}

/** 输入框上面一个要发的附件：传完了有 id；传图片时 file 用来在本地预览 */
export interface ComposerAttachment {
  uid: string;
  name: string;
  kind: "image" | "text";
  size: number;
  status: "uploading" | "done" | "error";
  id?: number;
  file?: File;
  error?: string;
}

/** 选的文件能不能发：不能时回原因。 */
export function attachProblem(file: File): string {
  if (IMAGE_TYPES.includes(file.type)) {
    return file.size > MAX_IMAGE_BYTES ? `「${file.name}」超过 5 MB` : "";
  }
  if (!TEXT_EXTS.includes(extOf(file.name))) {
    return `「${file.name}」不能发：只收 PNG、JPEG、WebP、GIF 图片与文本文件`;
  }
  return file.size > MAX_TEXT_BYTES ? `「${file.name}」超过 200 KB` : "";
}

/** FilesCard 的文件种类（决定图标）。 */
export function fileTypeOf(
  a: Pick<AssistantAttachment, "name" | "kind">,
): "image" | "mark" | "code" | "excel" | "txt" {
  if (a.kind === "image") return "image";
  const ext = extOf(a.name);
  if (ext === ".md" || ext === ".markdown") return "mark";
  if ([".json", ".yaml", ".yml", ".toml", ".ini", ".conf", ".xml"].includes(ext)) return "code";
  if (ext === ".csv" || ext === ".tsv") return "excel";
  return "txt";
}

/** 附件卡片下面那行字：「文本 · 2.3 KB」（FilesCard 默认写的是英文扩展名）。 */
export function attachmentDesc(a: Pick<AssistantAttachment, "kind" | "size">): string {
  return `${a.kind === "image" ? "图片" : "文本"} · ${formatBytes(a.size)}`;
}

/** 工具在页面上的名字（和 MCP 接入页、确认卡片一致）。 */
const TOOL_LABELS: Record<string, string> = {
  list_tasks: "查看抓到的种子",
  list_downloader_torrents: "查看下载器里的种子",
  get_downloader_stats: "查看下载器状态",
  search_torrents: "搜索种子",
  pause_torrent: "暂停种子",
  resume_torrent: "继续种子",
  delete_torrent: "删除种子",
  push_torrent: "推送种子到下载器",
  get_site_userinfo: "查看站点数据",
  check_updates: "检查更新",
  explore_media: "查找影视",
  list_subscriptions: "查看订阅",
  add_subscription: "添加订阅",
  get_subscription: "查看订阅详情",
  set_subscription_status: "暂停或恢复订阅",
  search_subscription: "立即搜索订阅",
  delete_subscription: "删除订阅",
  recognize_media: "识别种子标题",
  list_media_libraries: "查看媒体库",
  list_media_history: "查看整理历史",
  preview_media_organize: "预览整理",
  organize_media: "整理进媒体库",
  retry_media_organize: "重新整理",
  get_config: "查看配置",
  change_config: "修改配置",
  get_skill: "读取内置技能",
};

export function toolLabel(name: string): string {
  return TOOL_LABELS[name] ?? name;
}

/** 参数在卡片上的一行文字：去掉花括号，空参数写「无参数」。 */
export function argsText(raw: string): string {
  try {
    const v = JSON.parse(raw) as Record<string, unknown>;
    const parts = Object.entries(v).map(
      ([k, x]) => `${k}: ${typeof x === "string" ? x : JSON.stringify(x)}`,
    );
    return parts.length ? parts.join("，") : "无参数";
  } catch {
    return raw;
  }
}

export const CONFIRM_LABELS: Record<AssistantConfirmStatus, string> = {
  pending: "等你确认",
  approved: "已确认",
  denied: "已拒绝",
  expired: "已过期",
};

export const CONFIRM_TONES: Record<AssistantConfirmStatus, "warn" | "ok" | "neutral" | "dang"> = {
  pending: "warn",
  approved: "ok",
  denied: "neutral",
  expired: "neutral",
};

/** 过期的原因写给人看。 */
export function decidedByText(c: AssistantConfirmation): string {
  switch (c.decided_by) {
    case "timeout":
      return "没有在时限内确认";
    case "cancel":
      return "回答停止了";
    case "restart":
    case "shutdown":
      return "pt-tools 重启过";
    default:
      return "";
  }
}
