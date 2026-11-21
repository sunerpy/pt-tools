import {
  RECORDER_GLOBAL,
  RECORDER_MAX_BODY,
  RECORDER_MAX_ENTRIES,
  RECORDER_MAX_PARAMS,
} from "./constants";

/*
 * 在页面自己的脚本之前（document_start、页面的 MAIN world）装上的请求记录器：
 * 只记同源的 XHR 与 fetch——方法、地址、状态、内容类型、请求参数、回应正文（截断）。
 * 记录只放在页面的内存里，由扩展在关标签页前读走（读走以后再脱敏）；不发任何请求，不改请求和回应。
 */

export interface RecorderEntry {
  kind: "xhr" | "fetch";
  method: string;
  url: string;
  status?: number;
  contentType?: string | null;
  requestBody?: string;
  responseBody?: string;
  error?: string;
  startedAt: number;
  endedAt?: number;
}

export interface RecorderStatus {
  inflight: number;
  lastActivity: number;
  count: number;
}

interface XhrInfo {
  method: string;
  url: string;
}

type XhrLike = XMLHttpRequest & { __pttRecorderInfo?: XhrInfo };

/** 回应正文读太久（长连接、流）也不算一直在忙 */
const BODY_READ_TIMEOUT_MS = 8000;

export function installRequestRecorder(win: Window & typeof globalThis): void {
  const host = win as unknown as Record<string, unknown>;
  if (host[RECORDER_GLOBAL]) {
    return;
  }

  const entries: RecorderEntry[] = [];
  let inflight = 0;
  let lastActivity = Date.now();
  const touch = (): void => {
    lastActivity = Date.now();
  };

  Object.defineProperty(win, RECORDER_GLOBAL, {
    value: Object.freeze({
      read: (): RecorderEntry[] => entries.map((entry) => ({ ...entry })),
      status: (): RecorderStatus => ({ inflight, lastActivity, count: entries.length }),
    }),
    enumerable: false,
    configurable: false,
    writable: false,
  });

  const resolve = (url: string): URL | null => {
    try {
      return new URL(url, win.location.href);
    } catch {
      return null;
    }
  };
  const sameOrigin = (url: URL | null): url is URL =>
    url !== null && url.origin === win.location.origin;

  const bodyText = (body: unknown): string | undefined => {
    if (body === null || body === undefined) return undefined;
    if (typeof body === "string") return body.slice(0, RECORDER_MAX_PARAMS);
    if (typeof win.URLSearchParams === "function" && body instanceof win.URLSearchParams) {
      return body.toString().slice(0, RECORDER_MAX_PARAMS);
    }
    if (typeof win.FormData === "function" && body instanceof win.FormData) {
      const parts: string[] = [];
      body.forEach((value, key) => {
        parts.push(
          `${encodeURIComponent(key)}=${typeof value === "string" ? encodeURIComponent(value) : "[file]"}`,
        );
      });
      return parts.join("&").slice(0, RECORDER_MAX_PARAMS);
    }
    return `[${Object.prototype.toString.call(body).slice(8, -1)}]`;
  };

  const Xhr = win.XMLHttpRequest;
  if (typeof Xhr === "function" && Xhr.prototype) {
    const originalOpen = Xhr.prototype.open;
    const originalSend = Xhr.prototype.send;

    Xhr.prototype.open = function open(this: XhrLike, ...args: unknown[]): void {
      try {
        this.__pttRecorderInfo = {
          method: String(args[0] ?? "GET").toUpperCase(),
          url: String(args[1] ?? ""),
        };
      } catch {
        // 记不下也不影响请求
      }
      return (originalOpen as (...a: unknown[]) => void).apply(this, args);
    } as typeof Xhr.prototype.open;

    Xhr.prototype.send = function send(
      this: XhrLike,
      body?: Document | XMLHttpRequestBodyInit | null,
    ): void {
      const info = this.__pttRecorderInfo;
      const url = info ? resolve(info.url) : null;
      if (info && sameOrigin(url) && entries.length < RECORDER_MAX_ENTRIES) {
        const entry: RecorderEntry = {
          kind: "xhr",
          method: info.method,
          url: url.href,
          requestBody: bodyText(body),
          startedAt: Date.now(),
        };
        entries.push(entry);
        inflight += 1;
        touch();
        let finished = false;
        const finish = (): void => {
          if (finished) return;
          finished = true;
          try {
            entry.status = this.status;
            entry.contentType = this.getResponseHeader("content-type");
            const type = this.responseType;
            if (type === "" || type === "text") {
              entry.responseBody = String(this.responseText).slice(0, RECORDER_MAX_BODY);
            } else if (type === "json") {
              entry.responseBody = JSON.stringify(this.response).slice(0, RECORDER_MAX_BODY);
            } else {
              entry.responseBody = `[${type}]`;
            }
          } catch (error) {
            entry.error = String(error);
          }
          entry.endedAt = Date.now();
          inflight -= 1;
          touch();
        };
        this.addEventListener("loadend", finish);
      }
      return (originalSend as (b?: unknown) => void).call(this, body);
    };
  }

  const originalFetch = win.fetch;
  if (typeof originalFetch === "function") {
    win.fetch = function fetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
      const promise = originalFetch.call(win, input, init);
      let url: URL | null = null;
      let method = "GET";
      let requestBody: string | undefined;
      try {
        const request =
          typeof win.Request === "function" && input instanceof win.Request ? input : null;
        url = resolve(request ? request.url : input instanceof URL ? input.href : String(input));
        method = String(init?.method ?? request?.method ?? "GET").toUpperCase();
        if (init && "body" in init) {
          requestBody = bodyText(init.body);
        } else if (request && method !== "GET" && method !== "HEAD") {
          requestBody = "[Request body]";
        }
      } catch {
        return promise;
      }
      if (!sameOrigin(url) || entries.length >= RECORDER_MAX_ENTRIES) {
        return promise;
      }

      const entry: RecorderEntry = {
        kind: "fetch",
        method,
        url: url.href,
        requestBody,
        startedAt: Date.now(),
      };
      entries.push(entry);
      inflight += 1;
      touch();

      const readBody = (response: Response): Promise<void> =>
        Promise.race([
          response
            .clone()
            .text()
            .then((text) => {
              entry.responseBody = text.slice(0, RECORDER_MAX_BODY);
            }),
          new Promise<void>((done) => {
            setTimeout(done, BODY_READ_TIMEOUT_MS);
          }),
        ]);

      promise
        .then((response) => {
          entry.status = response.status;
          entry.contentType = response.headers.get("content-type");
          return readBody(response);
        })
        .catch((error: unknown) => {
          entry.error = String(error);
        })
        .finally(() => {
          entry.endedAt = Date.now();
          inflight -= 1;
          touch();
        });
      return promise;
    } as typeof win.fetch;
  }
}
