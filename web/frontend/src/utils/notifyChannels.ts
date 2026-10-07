import type { LucideIconName } from "@/icons/lucide";

/**
 * 只出站的通知通道（路线图 M8）：类型名与后端适配器的注册名一致（internal/notify/adapter/<type>），
 * 字段名与适配器解析的 ConfigJSON 一致。新建对话框只填必填项，详情页填全部。
 */
export interface OutboundField {
  key: OutboundFieldKey;
  label: string;
  kind?: "text" | "password" | "number" | "select";
  placeholder?: string;
  tip?: string;
  required?: boolean;
  options?: { label: string; value: string }[];
  min?: number;
  max?: number;
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
  | "msg_type";

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
        tip: "自建 bark-server 时填自己的地址",
      },
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
      },
      {
        key: "server_url",
        label: "服务器地址（可选）",
        placeholder: "https://ntfy.sh",
        tip: "自建 ntfy 时填自己的地址",
      },
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

/** 必填项里没填的第一个字段（给提示用） */
export function missingRequired(
  type: string | undefined,
  values: Record<string, unknown>,
): OutboundField | undefined {
  return outboundChannel(type)?.fields.find(
    (f) => f.required && String(values[f.key] ?? "").trim() === "",
  );
}
