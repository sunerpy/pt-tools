import { PtToolsApiError } from "./errors";

const DEFAULT_TIMEOUT_MS = 10000;

interface FetchWithTimeoutOptions extends RequestInit {
  timeoutMs?: number;
}

export async function fetchWithTimeout(
  input: string,
  init: FetchWithTimeoutOptions = {},
): Promise<Response> {
  const { timeoutMs = DEFAULT_TIMEOUT_MS, signal: externalSignal, ...rest } = init;
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  const onExternalAbort = (): void => controller.abort();
  if (externalSignal) {
    if (externalSignal.aborted) controller.abort();
    else externalSignal.addEventListener("abort", onExternalAbort, { once: true });
  }
  try {
    return await fetch(input, { ...rest, signal: controller.signal });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      throw new PtToolsApiError("timeout", `request timed out after ${timeoutMs}ms`);
    }
    if (err instanceof TypeError) {
      throw new PtToolsApiError("network_unreachable", err.message);
    }
    throw err;
  } finally {
    clearTimeout(timer);
    if (externalSignal) externalSignal.removeEventListener("abort", onExternalAbort);
  }
}

export function classifyHttpResponse(response: Response, body?: string): never {
  if (response.status === 401 || response.status === 0 || response.type === "opaqueredirect") {
    throw new PtToolsApiError("auth_required", "auth required", response.status);
  }
  const detail = body?.trim() || `HTTP ${response.status}`;
  if (response.status >= 500) {
    throw new PtToolsApiError("server_error", detail, response.status);
  }
  if (response.status >= 400) {
    throw new PtToolsApiError("client_error", detail, response.status);
  }
  throw new PtToolsApiError("unknown", detail, response.status);
}
