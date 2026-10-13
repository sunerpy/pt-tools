/*
 * CookieCloud 服务端的两个接口（和 CookieCloud 扩展一样）：
 *   GET  {服务地址}/get/{uuid}  → {encrypted, crypto_type}，没有这个 UUID 时 404
 *   POST {服务地址}/update      ← {uuid, encrypted, crypto_type}，成功回 {"action": "done"}
 * 只发密文，密码不出浏览器。不跟随重定向：地址里带着 UUID，不带去别处。
 */
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
    throw new CookieCloudError("invalid_server", "服务地址要以 http:// 或 https:// 开头");
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
    throw new CookieCloudError(
      "invalid_server",
      "服务地址要以 http:// 或 https:// 开头，不能带用户名密码、查询参数或 #",
    );
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
      throw new CookieCloudError("timeout", "连接 CookieCloud 服务超时");
    }
    throw new CookieCloudError("network", "连不上 CookieCloud 服务");
  }
  if (response.type === "opaqueredirect" || (response.status >= 300 && response.status < 400)) {
    throw new CookieCloudError(
      "redirect",
      "CookieCloud 服务把请求重定向到了别处，没有跟随：请填写最终的服务地址",
    );
  }
  return response;
}

/** fetchPayload 取回 uuid 的密文；服务端还没有这个 UUID 的数据时返回 null。 */
export async function fetchPayload(server: string, uuid: string): Promise<CookieCloudPayload | null> {
  const response = await request(endpoint(server, "get", uuid.trim()), {
    method: "GET",
    headers: { Accept: "application/json" },
    credentials: "omit",
  });
  if (response.status === 404) {
    return null;
  }
  if (!response.ok) {
    throw new CookieCloudError("http", `CookieCloud 服务返回 HTTP ${response.status}`, response.status);
  }
  let body: unknown;
  try {
    body = JSON.parse(await response.text());
  } catch {
    throw new CookieCloudError("bad_response", "CookieCloud 的响应不是 JSON");
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
    throw new CookieCloudError("http", `CookieCloud 服务返回 HTTP ${response.status}`, response.status);
  }
  const text = await response.text();
  let action: unknown;
  try {
    action = (JSON.parse(text) as { action?: unknown } | null)?.action;
  } catch {
    action = undefined;
  }
  if (action !== "done") {
    throw new CookieCloudError("bad_response", "CookieCloud 服务没有确认保存");
  }
}
