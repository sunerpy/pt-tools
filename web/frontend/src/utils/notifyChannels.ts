import type { LucideIconName } from "@/icons/lucide";

/**
 * 只出站的通知通道（路线图 M8）：类型名与后端适配器的注册名一致（internal/notify/adapter/<type>），
 * 字段名与适配器解析的 ConfigJSON 一致。新建对话框与详情页都填全部字段：自建服务器的地址与鉴权
 * 要在创建时就填好，不然通道一创建就会先连默认的公共服务器。
 *
 * pattern 是页面上的即时提示，与适配器的检查同一套规则；后端保存前还会再查一遍（CheckConfig）。
 */
export interface OutboundField {
  key: OutboundFieldKey;
  label: string;
  kind?: "text" | "password" | "number" | "select" | "switch";
  placeholder?: string;
  tip?: string;
  required?: boolean;
  options?: { label: string; value: string }[];
  min?: number;
  max?: number;
  pattern?: RegExp;
  patternMessage?: string;
}

export type OutboundFieldKey =
  | "server_url"
  | "device_key"
  | "group"
  | "sound"
  | "send_key"
  | "topic"
  | "token"
  | "username"
  | "password"
  | "priority"
  | "webhook_url"
  | "secret"
  | "msg_type"
  | "allow_private";

const SERVER_URL = /^https?:\/\/[^\s/?#@]+(\/[^\s?#]*)?$/i;
const SERVER_URL_MESSAGE = "服务器地址要以 http:// 或 https:// 开头，不能带用户名密码、? 或 #";

/** 自建服务器在本机或内网时要打开的开关（后端连接前按解析出的 IP 检查） */
const ALLOW_PRIVATE: OutboundField = {
  key: "allow_private",
  label: "允许内网地址",
  kind: "switch",
  tip: "服务器在本机或内网（如 192.168.x.x、NAS 上的 Docker）时打开；用官方或公网服务器时保持关闭",
};

export interface OutboundChannel {
  type: string;
  label: string;
  icon: LucideIconName;
  color: string;
  fields: OutboundField[];
}

export const OUTBOUND_CHANNELS: readonly OutboundChannel[] = [
  {
    type: "bark",
    label: "Bark",
    icon: "smartphone",
    color: "var(--pt-info)",
    fields: [
      {
        key: "device_key",
        label: "Device Key",
        kind: "password",
        required: true,
        placeholder: "Bark App 里推送地址中的那一段",
        tip: "Bark App 首页的推送地址形如 https://api.day.app/<Device Key>/…",
      },
      {
        key: "server_url",
        label: "服务器地址（可选）",
        placeholder: "https://api.day.app",
        tip: "自建 bark-server 时填自己的地址；留空用官方服务器",
        pattern: SERVER_URL,
        patternMessage: SERVER_URL_MESSAGE,
      },
      ALLOW_PRIVATE,
      { key: "group", label: "分组（可选）", placeholder: "pt-tools" },
      { key: "sound", label: "铃声（可选）", placeholder: "minuet" },
    ],
  },
  {
    type: "serverchan",
    label: "Server 酱",
    icon: "bell-ring",
    color: "var(--pt-ok)",
    fields: [
      {
        key: "send_key",
        label: "SendKey",
        kind: "password",
        required: true,
        placeholder: "SCT… 或 sctp…",
        tip: "Turbo 版的 SCT… 与 Server 酱³ 的 sctp… 都可以",
        pattern: /^\s*[^\s/?#%]+\s*$/,
        patternMessage: "SendKey 格式不对",
      },
    ],
  },
  {
    type: "ntfy",
    label: "ntfy",
    icon: "bell-dot",
    color: "var(--pt-p)",
    fields: [
      {
        key: "topic",
        label: "Topic",
        required: true,
        placeholder: "pt-tools-xxxx",
        tip: "字母、数字、- 和 _，最多 64 个字符；公共服务器上的 topic 谁知道名字都能订阅，起一个不好猜的",
        pattern: /^\s*[-_A-Za-z0-9]{1,64}\s*$/,
        patternMessage: "Topic 只能是字母、数字、- 和 _，最多 64 个字符",
      },
      {
        key: "server_url",
        label: "服务器地址（可选）",
        placeholder: "https://ntfy.sh",
        tip: "自建 ntfy 时填自己的地址；留空用公共服务器",
        pattern: SERVER_URL,
        patternMessage: SERVER_URL_MESSAGE,
      },
      ALLOW_PRIVATE,
      { key: "token", label: "Access Token（可选）", kind: "password", placeholder: "tk_…" },
      { key: "username", label: "用户名（可选）" },
      { key: "password", label: "密码（可选）", kind: "password" },
      {
        key: "priority",
        label: "优先级（可选）",
        kind: "number",
        min: 0,
        max: 5,
        tip: "1 最低、5 最高，0 用服务器的默认值",
      },
    ],
  },
  {
    type: "dingtalk",
    label: "钉钉群机器人",
    icon: "message-square-text",
    color: "var(--pt-info)",
    fields: [
      {
        key: "webhook_url",
        label: "Webhook 地址",
        kind: "password",
        required: true,
        placeholder: "https://oapi.dingtalk.com/robot/send?access_token=…",
        pattern: /^\s*https:\/\/oapi\.dingtalk\.com\/robot\/send\?(.*&)?access_token=[^&#\s]+/i,
        patternMessage:
          "钉钉 Webhook 地址要是 https://oapi.dingtalk.com/robot/send?access_token=… 的形式",
      },
      {
        key: "secret",
        label: "加签密钥（可选）",
        kind: "password",
        placeholder: "SEC…",
        tip: "机器人安全设置选了「加签」时填 SEC 开头的密钥；选「自定义关键词」时消息里要含有关键词",
      },
      {
        key: "msg_type",
        label: "消息格式",
        kind: "select",
        options: [
          { label: "Markdown", value: "markdown" },
          { label: "纯文本", value: "text" },
        ],
      },
    ],
  },
  {
    type: "feishu",
    label: "飞书群机器人",
    icon: "send-horizontal",
    color: "var(--pt-warn)",
    fields: [
      {
        key: "webhook_url",
        label: "Webhook 地址",
        kind: "password",
        required: true,
        placeholder: "https://open.feishu.cn/open-apis/bot/v2/hook/…",
        tip: "Lark 国际版的地址是 https://open.larksuite.com/open-apis/bot/v2/hook/…",
        pattern:
          /^\s*https:\/\/open\.(feishu\.cn|larksuite\.com)\/open-apis\/bot\/v2\/hook\/[^/?#\s]+\s*$/i,
        patternMessage:
          "飞书 Webhook 地址要是 https://open.feishu.cn/open-apis/bot/v2/hook/… 的形式",
      },
      {
        key: "secret",
        label: "签名密钥（可选）",
        kind: "password",
        tip: "机器人安全设置开了「签名校验」时填",
      },
    ],
  },
];

/** 按类型取只出站通道的定义；不是这几种时返回 undefined */
export function outboundChannel(type: string | undefined): OutboundChannel | undefined {
  return OUTBOUND_CHANNELS.find((c) => c.type === type);
}

/** 该类型的字段名：新建与保存时只发这些字段，换类型后上一个类型填的值不会跟着发出去 */
export function outboundFieldKeys(type: string | undefined): OutboundFieldKey[] {
  return outboundChannel(type)?.fields.map((f) => f.key) ?? [];
}

/** 第一个没填的必填项或格式不对的字段的提示；都没问题时返回 undefined */
export function outboundProblem(
  type: string | undefined,
  values: Record<string, unknown>,
): string | undefined {
  for (const f of outboundChannel(type)?.fields ?? []) {
    const v = values[f.key];
    if (f.kind === "switch" || f.kind === "number") continue;
    const text = String(v ?? "").trim();
    if (f.required && text === "") return `请填写${f.label}`;
    if (text !== "" && f.pattern && !f.pattern.test(text)) return f.patternMessage;
  }
  return undefined;
}

/** 接口错误的原因：ChatOps 接口回 {"error": "…", "detail": "…"}，取 detail；不是这个格式时用原文 */
export function apiErrorDetail(e: unknown, fallback: string): string {
  const raw = (e as Error | undefined)?.message || "";
  if (!raw) return fallback;
  try {
    const parsed = JSON.parse(raw) as { detail?: unknown; error?: unknown } | null;
    if (parsed && typeof parsed === "object") {
      if (typeof parsed.detail === "string" && parsed.detail) return parsed.detail;
      if (typeof parsed.error === "string" && parsed.error) return parsed.error;
    }
  } catch {
    /* 不是 JSON：用原文 */
  }
  return raw;
}
