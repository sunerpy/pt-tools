/*
 * CookieCloud 服务端的两个接口（和 CookieCloud 扩展一样）：
 *   GET  {服务地址}/get/{uuid}  → {encrypted, crypto_type}，没有这个 UUID 时 404
 *   POST {服务地址}/update      ← {uuid, encrypted, crypto_type}，成功回 {"action": "done"}
 * 只发密文，密码不出浏览器。不跟随重定向：地址里带着 UUID，不带去别处。
 */
import { t } from "../../core/i18n";
import { fetchWithTimeout } from "../sync/http";
import { PtToolsApiError } from "../sync/errors";

export type CookieCloudErrorCode =
  | "invalid_server"
  | "not_configured"
  | "network"
  | "timeout"
  | "redirect"
  | "http"
  | "bad_response"
  | "decrypt";

/** CookieCloudError 的 message 是给人看的原因（不带 UUID、密码与地址的路径）。 */
export class CookieCloudError extends Error {
  readonly code: CookieCloudErrorCode;
  readonly status?: number;

  constructor(code: CookieCloudErrorCode, message: string, status?: number) {
    super(message);
    this.name = "CookieCloudError";
    this.code = code;
    this.status = status;
  }
}

export interface CookieCloudPayload {
  encrypted: string;
  crypto_type?: string;
}

const TIMEOUT_MS = 20000;

/**
 * parseServer 解析服务地址：http(s)，不带用户名密码、查询参数或 #，末尾的 / 去掉。
 */
export function parseServer(raw: string): URL {
  let url: URL;
  try {
    url = new URL(raw.trim());
  } catch {
    throw new CookieCloudError("invalid_server", t("cc.err.invalidServer"));
  }
  if (
    (url.protocol !== "http:" && url.protocol !== "https:") ||
    !url.hostname ||
    url.username ||
    url.password ||
    url.search ||
    url.hash ||
    raw.includes("?") ||
    raw.includes("#")
  ) {
    throw new CookieCloudError("invalid_server", t("cc.err.invalidServer"));
  }
  url.pathname = url.pathname.replace(/\/+$/, "");
  return url;
}

function endpoint(server: string, ...segments: string[]): string {
  const url = parseServer(server);
  const base = url.pathname.replace(/\/+$/, "");
  url.pathname = [base, ...segments.map((s) => encodeURIComponent(s))].join("/");
  return url.toString();
}

async function request(url: string, init: RequestInit): Promise<Response> {
  let response: Response;
  try {
    response = await fetchWithTimeout(url, { ...init, redirect: "manual", timeoutMs: TIMEOUT_MS });
  } catch (error: unknown) {
    if (error instanceof PtToolsApiError && error.code === "timeout") {
      throw new CookieCloudError("timeout", t("cc.err.timeout"));
    }
    throw new CookieCloudError("network", t("cc.err.network"));
  }
  if (response.type === "opaqueredirect" || (response.status >= 300 && response.status < 400)) {
    throw new CookieCloudError("redirect", t("cc.err.redirect"));
  }
  return response;
}

/** fetchPayload 取回 uuid 的密文；服务端还没有这个 UUID 的数据时返回 null。 */
export async function fetchPayload(
  server: string,
  uuid: string,
): Promise<CookieCloudPayload | null> {
  const response = await request(endpoint(server, "get", uuid.trim()), {
    method: "GET",
    headers: { Accept: "application/json" },
    credentials: "omit",
  });
  if (response.status === 404) {
    return null;
  }
  if (!response.ok) {
    throw new CookieCloudError("http", t("cc.err.http", response.status), response.status);
  }
  let body: unknown;
  try {
    body = JSON.parse(await response.text());
  } catch {
    throw new CookieCloudError("bad_response", t("cc.err.notJson"));
  }
  const encrypted = (body as { encrypted?: unknown } | null)?.encrypted;
  if (typeof encrypted !== "string" || encrypted === "") {
    // 有的服务端没有数据时回 200 和空对象
    return null;
  }
  const cryptoType = (body as { crypto_type?: unknown }).crypto_type;
  return { encrypted, crypto_type: typeof cryptoType === "string" ? cryptoType : undefined };
}

/** pushPayload 上传密文（整份替换服务端这个 UUID 的数据）。 */
export async function pushPayload(
  server: string,
  uuid: string,
  encrypted: string,
  cryptoType: string,
): Promise<void> {
  const response = await request(endpoint(server, "update"), {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    credentials: "omit",
    body: JSON.stringify({ uuid: uuid.trim(), encrypted, crypto_type: cryptoType }),
  });
  if (!response.ok) {
    throw new CookieCloudError("http", t("cc.err.http", response.status), response.status);
  }
  const text = await response.text();
  let action: unknown;
  try {
    action = (JSON.parse(text) as { action?: unknown } | null)?.action;
  } catch {
    action = undefined;
  }
  if (action !== "done") {
    throw new CookieCloudError("bad_response", t("cc.err.notSaved"));
  }
}
