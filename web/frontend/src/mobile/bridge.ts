/** Only the APK's local pages may use this channel. Native independently checks every path. */
export interface NativeConfig {
  canWrite: boolean;
  lang: string;
  dark: boolean;
}
interface NativeReply {
  id: string;
  ok: boolean;
  data?: unknown;
  error?: { status?: number; message?: string };
}
declare global {
  interface Window {
    PtMobile?: { postMessage(message: string): void };
    ptMobileConfig?: NativeConfig;
    ptMobileReceive?: (reply: NativeReply) => void;
  }
}
export class BridgeError extends Error {
  constructor(
    message: string,
    public readonly status = 0,
  ) {
    super(message);
  }
}
let sequence = 0;
const pending = new Map<
  string,
  {
    resolve: (value: unknown) => void;
    reject: (error: Error) => void;
    timer: ReturnType<typeof setTimeout>;
  }
>();
function post(message: object) {
  if (!window.PtMobile)
    throw new BridgeError("Please open this page in pt-tools App / 请在 pt-tools App 内打开");
  window.PtMobile.postMessage(JSON.stringify(message));
}
export function notify(method: string, payload: Record<string, unknown> = {}) {
  post({ id: String(++sequence), method, ...payload });
}
export function bridgeCall<T>(
  method: string,
  payload: Record<string, unknown>,
  timeout = 60_000,
): Promise<T> {
  const id = String(++sequence);
  window.ptMobileReceive = (reply) => {
    const call = pending.get(reply.id);
    if (!call) return;
    clearTimeout(call.timer);
    pending.delete(reply.id);
    if (reply.ok) call.resolve(reply.data);
    else
      call.reject(
        new BridgeError(reply.error?.message || "Request failed / 请求失败", reply.error?.status),
      );
  };
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      try {
        notify("cancel", { requestId: id });
      } catch {
        /* channel already gone */
      }
      reject(new BridgeError("Request timed out / 请求超时"));
    }, timeout);
    pending.set(id, { resolve: (value) => resolve(value as T), reject, timer });
    try {
      post({ id, method, ...payload });
    } catch (error) {
      clearTimeout(timer);
      pending.delete(id);
      reject(error);
    }
  });
}
export function cancelPending() {
  for (const [id, call] of pending) {
    clearTimeout(call.timer);
    try {
      notify("cancel", { requestId: id });
    } catch {
      /* view disposed */
    }
    call.reject(new BridgeError("Page closed / 页面已关闭"));
  }
  pending.clear();
}
export function request<T>(path: string, verb = "GET", body?: unknown): Promise<T> {
  if (
    !/^\/api\/app\/v1\/[A-Za-z0-9_/-]+(?:\?[^#]*)?$/.test(path) ||
    path.split("?")[0]!.includes("//")
  ) {
    return Promise.reject(new BridgeError("Invalid App API path / 接口路径不允许"));
  }
  return bridgeCall<T>("request", { path, verb, ...(body === undefined ? {} : { body }) });
}
export async function nativeReady(): Promise<NativeConfig> {
  if (window.ptMobileConfig) return window.ptMobileConfig;
  return new Promise((resolve, reject) => {
    const done = () => {
      if (!window.ptMobileConfig) return;
      clearTimeout(timer);
      window.removeEventListener("ptMobileReady", done);
      resolve(window.ptMobileConfig);
    };
    const timer = setTimeout(() => {
      window.removeEventListener("ptMobileReady", done);
      reject(new BridgeError("App connection unavailable / 无法连接 App"));
    }, 10_000);
    window.addEventListener("ptMobileReady", done);
    // Native injects configuration only after this local document finished loading.
    if (window.PtMobile) notify("ready");
  });
}
export async function imageAction(action: "share" | "save", blob: Blob) {
  if (blob.size > 16 * 1024 * 1024) throw new BridgeError("Image is too large / 图片过大");
  const base64 = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result).split(",")[1]!);
    reader.onerror = () => reject(new BridgeError("Cannot read image / 图片读取失败"));
    reader.readAsDataURL(blob);
  });
  return bridgeCall<{ saved?: boolean; canceled?: boolean }>("image", { action, base64 }, 120_000);
}
