import { t, type MessageKey } from "../../core/i18n";

export type PtToolsErrorCode =
  | "network_unreachable"
  | "timeout"
  | "auth_required"
  | "client_error"
  | "server_error"
  | "invalid_response"
  | "ping_failed"
  | "unknown";

const ERROR_CODE_PREFIX_RE = /^\[(?<code>[a-z_]+)]\s*/;

export class PtToolsApiError extends Error {
  readonly code: PtToolsErrorCode;
  readonly status?: number;

  constructor(code: PtToolsErrorCode, message: string, status?: number) {
    super(`[${code}] ${message}`);
    this.name = "PtToolsApiError";
    this.code = code;
    this.status = status;
  }
}

export function parseErrorString(raw: string): { code: PtToolsErrorCode; message: string } {
  const match = ERROR_CODE_PREFIX_RE.exec(raw);
  if (!match?.groups) {
    return { code: "unknown", message: raw };
  }
  return {
    code: match.groups.code as PtToolsErrorCode,
    message: raw.slice(match[0].length),
  };
}

const FRIENDLY_KEY: Record<PtToolsErrorCode, MessageKey> = {
  network_unreachable: "error.networkUnreachable",
  timeout: "error.timeout",
  auth_required: "error.authRequired",
  client_error: "error.clientError",
  server_error: "error.serverError",
  invalid_response: "error.invalidResponseFromPtTools",
  ping_failed: "error.pingFailed",
  unknown: "feedback.requestFailed",
};

export function friendlyErrorMessage(raw: string): string {
  const { code, message } = parseErrorString(raw);
  const key = FRIENDLY_KEY[code];
  if (code === "client_error" || code === "server_error") {
    return t(key, message);
  }
  if (code === "unknown" && message) {
    return message;
  }
  return t(key);
}
