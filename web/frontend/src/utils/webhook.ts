/**
 * 自定义 Webhook 请求（路线图 M21）：请求方法、请求体格式、预设，以及请求头在编辑框（JSON 文本）
 * 与接口（对象）之间的转换。预设放在 webhookPresets.json：后端测试读同一份文件，
 * 检查每个预设都能在所有事件的示例数据上通过保存前的校验。
 */
import presets from "./webhookPresets.json";

export interface WebhookPreset {
  key: string;
  label: string;
  desc: string;
  method: string;
  content_type: string;
  body_template: string;
  /** 这类服务的地址一般长什么样；空的时候不提示 */
  url_hint: string;
  /** 这类服务的成功响应怎么认（可选）：很多服务出错也回 200，错误写在响应里 */
  success_template: string;
}

export const WEBHOOK_PRESETS: WebhookPreset[] = presets;

export const WEBHOOK_METHODS = ["POST", "PUT", "PATCH", "GET"] as const;

export const WEBHOOK_FORMATS = [
  { value: "json", label: "JSON" },
  { value: "text", label: "纯文本" },
  { value: "form", label: "表单" },
  { value: "custom", label: "自定义（请求头里写 Content-Type）" },
] as const;

export const HEADERS_ERROR = "请求头要是 JSON 对象，键和值都是文字";

export type HeadersParse = { ok: true; value: Record<string, string> } | { ok: false };

/** 编辑框里的请求头 JSON 文本解析成对象；空着是空对象（保存时清掉原来的请求头）。 */
export function parseHeadersText(text: string | undefined): HeadersParse {
  const raw = (text ?? "").trim();
  if (raw === "") return { ok: true, value: {} };
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { ok: false };
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return { ok: false };
  const value: Record<string, string> = {};
  for (const [k, v] of Object.entries(parsed)) {
    if (typeof v !== "string" || k.trim() === "") return { ok: false };
    value[k] = v;
  }
  return { ok: true, value };
}

/** 接口读回来的请求头写成编辑框里的 JSON 文本：对象格式化；旧版存成的 JSON 文本原样；空的是空串。 */
export function headersToText(v: unknown): string {
  if (typeof v === "string") return v;
  if (v && typeof v === "object" && !Array.isArray(v) && Object.keys(v).length > 0) {
    return JSON.stringify(v, null, 2);
  }
  return "";
}
