/*
 * AI 助手（路线图 M19）页面用的纯函数：服务商预设、回答的 Markdown 渲染、事件流的归并与消息的分组。
 * 都不碰 DOM 以外的状态，方便单测。
 */
import type {
  AssistantConfirmation,
  AssistantConfirmStatus,
  AssistantConversationDetail,
  AssistantEvent,
  AssistantMessage,
  AssistantToolCall,
} from "@/api";
import DOMPurify from "dompurify";
import { Marked } from "marked";

/** 服务商预设：选了以后填好接口地址与常用模型（2026-10 按各家文档核对过）。模型可以手填。 */
export interface AssistantProvider {
  key: string;
  label: string;
  baseURL: string;
  models: string[];
  /** 不要 API Key（本机的 Ollama） */
  noKey?: boolean;
  hint?: string;
}

export const ASSISTANT_PROVIDERS: readonly AssistantProvider[] = [
  {
    key: "deepseek",
    label: "DeepSeek",
    baseURL: "https://api.deepseek.com",
    models: ["deepseek-flash", "deepseek-v4-pro"],
  },
  {
    key: "qwen",
    label: "通义千问（阿里云百炼）",
    baseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
    models: ["qwen3.7-plus", "qwen3.8-flash", "qwen3.8-max"],
  },
  {
    key: "kimi",
    label: "Kimi（月之暗面）",
    baseURL: "https://api.moonshot.cn/v1",
    models: ["kimi-k2.6", "kimi-k3"],
  },
  { key: "siliconflow", label: "硅基流动", baseURL: "https://api.siliconflow.cn/v1", models: [] },
  { key: "openrouter", label: "OpenRouter", baseURL: "https://openrouter.ai/api/v1", models: [] },
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
  /** 进行中的回答到现在的文字 */
  partial: string;
  /** 正在执行的工具 */
  tool: AssistantToolCall | null;
  lastSeq: number;
  /** 最近一次运行失败的原因 */
  error: string;
}

export function chatStateFrom(d: AssistantConversationDetail): ChatState {
  return {
    messages: [...d.messages],
    confirmations: [...d.confirmations],
    running: d.running,
    runId: d.run_id ?? "",
    partial: d.running ? (d.partial ?? "") : "",
    tool: null,
    lastSeq: d.last_seq,
    error: "",
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
      return { ...next, running: true, runId: ev.run_id ?? "", partial: "", tool: null, error: "" };
    case "delta":
      return { ...next, partial: ev.replace ? (ev.text ?? "") : next.partial + (ev.text ?? "") };
    case "message": {
      if (!ev.message) return next;
      const m = ev.message;
      const out = { ...next, messages: upsert(next.messages, m) };
      if (m.role === "assistant" || m.role === "notice") out.partial = "";
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
        tool: null,
        error: ev.status === "failed" ? (ev.error ?? "") : "",
      };
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

/** 页面上的一项：一条用户消息、一条回答（含它的工具调用）、一条提示。工具结果并进它所属的调用。 */
export type ChatItem =
  | { kind: "user"; message: AssistantMessage }
  | { kind: "assistant"; message: AssistantMessage; calls: CallView[] }
  | { kind: "notice"; message: AssistantMessage };

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
  for (const m of messages) {
    if (m.role === "user") out.push({ kind: "user", message: m });
    else if (m.role === "notice") out.push({ kind: "notice", message: m });
    else if (m.role === "assistant") {
      const calls = (m.tool_calls ?? []).map((call) => ({
        call,
        result: results.get(`${m.run_id ?? ""}/${call.id}`),
        confirmation: confs.get(`${m.run_id ?? ""}/${call.id}`),
      }));
      if (!m.content && calls.length === 0) continue;
      out.push({ kind: "assistant", message: m, calls });
    }
  }
  return out;
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
