/**
 * pt-tools relay 的 Cloudflare 版（路线图 M16）：只做转发，看不到明文。协议见 docs/design/remote-access.md 的「relay」。
 *
 * - 每个 hostId 一个 Durable Object（HostRelay），用 WebSocket Hibernation API 持有主机与客户端的连接；
 * - 每个客户端 IP 一个 Durable Object（IPLimiter），限制每分钟新建连接数；
 * - 主机的 ping 由 setWebSocketAutoResponse 回 pong，不唤醒 Durable Object。
 */
import { DurableObject } from "cloudflare:workers";

import {
  CLOSE,
  closePayload,
  encodeOuter,
  ipKey,
  MAX_NOISE_MESSAGE,
  MAX_UNCONFIRMED,
  NONCE_LEN,
  OuterType,
  parseClosePayload,
  parseOuter,
  relayOrigin,
  validHostId,
  verifyRelayAuth,
} from "./protocol";

export interface Env {
  HOST_RELAY: DurableObjectNamespace<HostRelay>;
  IP_LIMITER: DurableObjectNamespace<IPLimiter>;
  /** relay 对外的地址（wss://relay.example.com）；主机按它签名。不填时按请求的地址推导 */
  PUBLIC_URL?: string;
  MAX_STREAMS_PER_HOST?: string;
  DAILY_BYTES_PER_HOST?: string;
  MAX_CONN_PER_IP_PER_MIN?: string;
  RELAY_DISABLED?: string;
  RELAY_VERSION?: string;
}

const AUTH_TIMEOUT_MS = 10_000;
/**
 * 每日用量先预留再用：存储里写的是「已经用的 + 一块」，用到预留的上限以前不再写。Durable Object 被逐出（休眠）时
 * 内存里的准确数没了，读回来的是预留数，只会多算不会少算；还在内存里时 USAGE_SETTLE_MS 以后把准确数写回去。
 */
const USAGE_RESERVE_BYTES = 1 << 20;
const USAGE_SETTLE_MS = 1_000;
/** 每 IP 的计数窗口 */
const IP_WINDOW_MS = 60_000;

interface Limits {
  maxStreams: number;
  dailyBytes: number;
  perIPPerMin: number;
}

function intVar(v: string | undefined, def: number): number {
  if (v === undefined || v.trim() === "") return def;
  const n = Number(v);
  return Number.isFinite(n) ? Math.trunc(n) : def;
}

function limits(env: Env): Limits {
  return {
    maxStreams: Math.max(1, intVar(env.MAX_STREAMS_PER_HOST, 16)),
    dailyBytes: Math.max(0, intVar(env.DAILY_BYTES_PER_HOST, 2 * 1024 * 1024 * 1024)),
    perIPPerMin: intVar(env.MAX_CONN_PER_IP_PER_MIN, 30),
  };
}

function disabled(env: Env): boolean {
  return (env.RELAY_DISABLED ?? "").toLowerCase() === "true";
}

/** 接受 WebSocket 以后马上用 code 关掉（客户端能看到关闭码，不只是一个 4xx） */
function rejectSocket(code: number, reason: string): Response {
  const pair = new WebSocketPair();
  const [client, server] = Object.values(pair) as [WebSocket, WebSocket];
  server.accept();
  server.close(code, reason);
  return new Response(null, { status: 101, webSocket: client });
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);
    if (url.pathname === "/healthz" && request.method === "GET") {
      return Response.json({
        ok: true,
        version: env.RELAY_VERSION ?? "dev",
        disabled: disabled(env),
      });
    }
    // 负载均衡、监控用：接新连接时 200；暂停服务时 503（Cloudflare 自己扩容，没有连接数上限与排空）
    if (url.pathname === "/ready" && request.method === "GET") {
      if (disabled(env)) {
        return Response.json({ status: "unready", reason: "disabled" }, { status: 503 });
      }
      return Response.json({ status: "ready" });
    }
    const m = /^\/v1\/(host|client)\/([^/]+)$/.exec(url.pathname);
    if (!m || request.method !== "GET") return new Response("not found", { status: 404 });
    if ((request.headers.get("Upgrade") ?? "").toLowerCase() !== "websocket") {
      return new Response("expected a WebSocket upgrade", { status: 426 });
    }
    if (disabled(env)) return rejectSocket(CLOSE.disabled, "relay disabled");
    const [, role, hostId] = m as unknown as [string, "host" | "client", string];
    if (!validHostId(hostId)) return rejectSocket(CLOSE.protocol, "bad host id");

    const lim = limits(env);
    if (lim.perIPPerMin >= 0) {
      const ip = ipKey(request.headers.get("CF-Connecting-IP") ?? "unknown");
      const limiter = env.IP_LIMITER.get(env.IP_LIMITER.idFromName(`ip:${ip}`));
      if (!(await limiter.allow(lim.perIPPerMin)))
        return rejectSocket(CLOSE.limited, "too many connections");
    }
    let origin: string;
    try {
      origin = relayOrigin(
        env.PUBLIC_URL && env.PUBLIC_URL.trim() !== "" ? env.PUBLIC_URL : request.url,
      );
    } catch {
      return new Response("bad PUBLIC_URL", { status: 500 });
    }
    const stub = env.HOST_RELAY.get(env.HOST_RELAY.idFromName(hostId));
    const forwarded = new Request(request.url, request);
    forwarded.headers.set("X-Relay-Role", role);
    forwarded.headers.set("X-Relay-Host", hostId);
    forwarded.headers.set("X-Relay-Origin", origin);
    return stub.fetch(forwarded);
  },
} satisfies ExportedHandler<Env>;

/**
 * 每个 IP 一个：60 秒的窗口里最多 limit 次新建连接。计数写在存储里（被逐出、重新部署都不清零），
 * 同一个对象里的存储操作是串行的；窗口结束时闹钟删掉记录。
 */
export class IPLimiter extends DurableObject<Env> {
  async allow(limit: number): Promise<boolean> {
    const now = Date.now();
    let w = await this.ctx.storage.get<{ start: number; count: number }>("w");
    if (!w || now - w.start >= IP_WINDOW_MS) {
      w = { start: now, count: 0 };
      await this.ctx.storage.setAlarm(now + IP_WINDOW_MS);
    }
    if (w.count >= limit) return false;
    w.count++;
    await this.ctx.storage.put("w", w);
    return true;
  }

  override async alarm(): Promise<void> {
    const w = await this.ctx.storage.get<{ start: number }>("w");
    if (w && Date.now() - w.start < IP_WINDOW_MS) {
      await this.ctx.storage.setAlarm(w.start + IP_WINDOW_MS);
      return;
    }
    await this.ctx.storage.deleteAll();
  }
}

/**
 * 连接上的附加信息（跨休眠保留，最大 16 KiB）。relay 关掉的连接改成 closed：关闭握手完成以前 getWebSockets() 还会列出它，
 * 不能再算作主机、占着流的名额或者收转发的消息。
 */
type Attachment =
  | { t: "pending"; nonce: string; at: number; origin: string; host: string }
  | { t: "authing"; at: number }
  | { t: "host"; epoch: number; host: string }
  /**
   * accepted：主机发过 ACCEPT（握手通过）。之前每个方向只能发一条（clientSent、hostSent），字节数记在 *Pending，
   * ACCEPT 时才计入用量；没有 ACCEPT 就关掉的流不计
   */
  | {
      t: "client";
      stream: number;
      epoch: number;
      accepted?: boolean;
      clientSent?: boolean;
      hostSent?: boolean;
      clientPending?: number;
      hostPending?: number;
    }
  | { t: "closed" };

interface Usage {
  day: string;
  bytes: number;
  over: boolean;
}

function b64(b: Uint8Array): string {
  let s = "";
  for (const x of b) s += String.fromCharCode(x);
  return btoa(s);
}

function unb64(s: string): Uint8Array {
  return Uint8Array.from(atob(s), (c) => c.charCodeAt(0));
}

function utcDay(now = Date.now()): string {
  return new Date(now).toISOString().slice(0, 10);
}

function safeClose(ws: WebSocket, code: number, reason: string): void {
  try {
    ws.close(code, reason);
  } catch {
    // 已经关了
  }
}

/** 标记成 closed 再关（见 Attachment） */
function markClosed(ws: WebSocket, code: number, reason: string): void {
  try {
    ws.serializeAttachment({ t: "closed" } satisfies Attachment);
  } catch {
    // 已经关了
  }
  safeClose(ws, code, reason);
}

function safeSend(ws: WebSocket, data: ArrayBuffer | Uint8Array | string): boolean {
  try {
    ws.send(data);
    return true;
  } catch {
    return false;
  }
}

/** 每个 hostId 一个：主机连接、它的客户端流、每日用量。 */
export class HostRelay extends DurableObject<Env> {
  /** 当天准确的用量（内存里）；reserved 是存储里写的预留数（不小于 usage.bytes） */
  private usage: Usage | null = null;
  private reserved = 0;
  /** 预留数比准确数多、还没写回准确数的时候为真（等闹钟） */
  private unsettled = false;

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    ctx.setWebSocketAutoResponse(new WebSocketRequestResponsePair("ping", "pong"));
  }

  override async fetch(request: Request): Promise<Response> {
    const role = request.headers.get("X-Relay-Role");
    const host = request.headers.get("X-Relay-Host") ?? "";
    const origin = request.headers.get("X-Relay-Origin") ?? "";
    const pair = new WebSocketPair();
    const [client, server] = Object.values(pair) as [WebSocket, WebSocket];
    if (role === "host") {
      this.ctx.acceptWebSocket(server);
      const nonce = crypto.getRandomValues(new Uint8Array(NONCE_LEN));
      server.serializeAttachment({
        t: "pending",
        nonce: b64(nonce),
        at: Date.now(),
        origin,
        host,
      } satisfies Attachment);
      server.send(encodeOuter({ type: OuterType.Challenge, stream: 0, payload: nonce }));
      await this.alarmBy(Date.now() + AUTH_TIMEOUT_MS);
      return new Response(null, { status: 101, webSocket: client });
    }
    // 客户端
    const lim = limits(this.env);
    const hostWs = this.hostSocket();
    if (!hostWs) {
      server.accept();
      server.close(CLOSE.hostOffline, "host offline");
      return new Response(null, { status: 101, webSocket: client });
    }
    if ((await this.overQuota(lim)) || this.clients().length >= lim.maxStreams) {
      server.accept();
      server.close(
        CLOSE.limited,
        (await this.overQuota(lim)) ? "daily quota exceeded" : "too many streams",
      );
      return new Response(null, { status: 101, webSocket: client });
    }
    const stream = await this.nextStream();
    const epoch = (hostWs.deserializeAttachment() as Attachment & { t: "host" }).epoch;
    this.ctx.acceptWebSocket(server);
    server.serializeAttachment({ t: "client", stream, epoch } satisfies Attachment);
    if (
      !safeSend(hostWs, encodeOuter({ type: OuterType.Open, stream, payload: new Uint8Array() }))
    ) {
      this.closeClient(server, CLOSE.hostOffline, "host offline", false);
    }
    return new Response(null, { status: 101, webSocket: client });
  }

  private attachment(ws: WebSocket): Attachment | null {
    return (ws.deserializeAttachment() as Attachment | null) ?? null;
  }

  private hostSocket(): WebSocket | null {
    for (const ws of this.ctx.getWebSockets()) {
      if (this.attachment(ws)?.t === "host") return ws;
    }
    return null;
  }

  private clients(epoch?: number): WebSocket[] {
    return this.ctx.getWebSockets().filter((ws) => {
      const a = this.attachment(ws);
      return a?.t === "client" && (epoch === undefined || a.epoch === epoch);
    });
  }

  private clientFor(stream: number): WebSocket | null {
    for (const ws of this.ctx.getWebSockets()) {
      const a = this.attachment(ws);
      if (a?.t === "client" && a.stream === stream) return ws;
    }
    return null;
  }

  /** 下一个流编号：用到头以后从 1 重来，跳过还开着的 */
  private async nextStream(): Promise<number> {
    let n = (await this.ctx.storage.get<number>("nextStream")) ?? 0;
    do {
      n = n >= 0xffffffff ? 1 : n + 1;
    } while (this.clientFor(n));
    await this.ctx.storage.put("nextStream", n);
    return n;
  }

  /**
   * 关掉一个客户端流；notifyHost 为真时告诉它的主机（CLOSE），主机自己要求关的、主机已经不在的不用。
   * 已经关过的只再关一次连接。
   */
  private closeClient(ws: WebSocket, code: number, reason: string, notifyHost: boolean): void {
    const a = this.attachment(ws);
    if (a?.t === "client" && notifyHost) {
      const host = this.hostSocket();
      const ha = host ? (this.attachment(host) as Attachment & { t: "host" }) : null;
      if (host && ha && ha.epoch === a.epoch) {
        safeSend(
          host,
          encodeOuter({
            type: OuterType.Close,
            stream: a.stream,
            payload: closePayload(code, reason),
          }),
        );
      }
    }
    markClosed(ws, code, reason);
  }

  /** 新的 epoch：比所有还开着的连接上的都大（同步算出来，认证提升那一段不用等存储） */
  private nextEpoch(): number {
    let max = 0;
    for (const ws of this.ctx.getWebSockets()) {
      const a = this.attachment(ws);
      if ((a?.t === "host" || a?.t === "client") && a.epoch > max) max = a.epoch;
    }
    return max + 1;
  }

  /** 闹钟只往前挪：认证超时与用量写回共用一个闹钟 */
  private async alarmBy(at: number): Promise<void> {
    const cur = await this.ctx.storage.getAlarm();
    if (cur === null || cur > at) await this.ctx.storage.setAlarm(at);
  }

  private async loadUsage(): Promise<Usage> {
    const day = utcDay();
    if (!this.usage) {
      // 刚建的实例（或者休眠以后）：存储里的是预留数，当作已经用了这么多
      this.usage = (await this.ctx.storage.get<Usage>("usage")) ?? { day, bytes: 0, over: false };
      this.reserved = this.usage.bytes;
      this.unsettled = false;
    }
    if (this.usage.day !== day) {
      this.usage = { day, bytes: 0, over: false };
      this.reserved = 0;
      this.unsettled = false;
      await this.ctx.storage.put("usage", this.usage);
    }
    return this.usage;
  }

  private async overQuota(lim: Limits): Promise<boolean> {
    if (lim.dailyBytes <= 0) return false;
    const u = await this.loadUsage();
    return u.over || u.bytes > lim.dailyBytes;
  }

  /** 记下转发的字节；刚超额时关掉全部客户端流（4429），返回还能不能继续转发 */
  private async count(n: number): Promise<boolean> {
    const lim = limits(this.env);
    if (lim.dailyBytes <= 0) return true;
    const u = await this.loadUsage();
    if (u.over) return false;
    u.bytes += n;
    if (u.bytes > lim.dailyBytes) {
      u.over = true;
      this.reserved = u.bytes;
      this.unsettled = false;
      await this.ctx.storage.put("usage", u);
      for (const ws of this.clients())
        this.closeClient(ws, CLOSE.limited, "daily quota exceeded", true);
      return false;
    }
    if (u.bytes > this.reserved) {
      // 用到预留的上限了：先把下一块预留写进存储再继续（一块不超过上限的 1/64，小限额时也不会一休眠就超额）
      const chunk = Math.max(1, Math.min(USAGE_RESERVE_BYTES, Math.floor(lim.dailyBytes / 64)));
      this.reserved = u.bytes + chunk;
      await this.ctx.storage.put("usage", { day: u.day, bytes: this.reserved, over: false });
      if (!this.unsettled) {
        this.unsettled = true;
        await this.alarmBy(Date.now() + USAGE_SETTLE_MS);
      }
    }
    return true;
  }

  /** 还在内存里时把准确的用量写回去（替换预留数） */
  private async settleUsage(): Promise<void> {
    if (!this.usage || !this.unsettled) return;
    this.unsettled = false;
    if (this.usage.day !== utcDay()) return;
    this.reserved = this.usage.bytes;
    await this.ctx.storage.put("usage", this.usage);
  }

  override async webSocketMessage(ws: WebSocket, raw: ArrayBuffer | string): Promise<void> {
    const a = this.attachment(ws);
    if (!a) return;
    // 兼容日期较新时二进制消息可能是 Blob：统一换成 ArrayBuffer
    const message: ArrayBuffer | string =
      (raw as unknown) instanceof Blob ? await (raw as unknown as Blob).arrayBuffer() : raw;
    switch (a.t) {
      case "pending":
        return this.onAuth(ws, a, message);
      case "authing":
        markClosed(ws, CLOSE.protocol, "unexpected message during auth");
        return;
      case "host":
        return this.onHostMessage(ws, a, message);
      case "client":
        return this.onClientMessage(ws, a, message);
    }
  }

  private async onAuth(
    ws: WebSocket,
    a: Attachment & { t: "pending" },
    message: ArrayBuffer | string,
  ): Promise<void> {
    const f = typeof message === "string" ? null : parseOuter(new Uint8Array(message));
    if (!f || f.type !== OuterType.Auth) {
      markClosed(ws, CLOSE.authFailed, "auth failed");
      return;
    }
    // 验签要等（别的事件会插进来）：先改成 authing，验签期间这个连接再发来的消息按违反协议处理
    ws.serializeAttachment({ t: "authing", at: a.at } satisfies Attachment);
    const ok = await verifyRelayAuth(a.host, unb64(a.nonce), a.origin, f.payload);
    // 等验签的时候关掉了（超时、违反协议、断开）：什么也不动
    if (this.attachment(ws)?.t !== "authing" || ws.readyState !== WebSocket.OPEN) return;
    if (!ok) {
      markClosed(ws, CLOSE.authFailed, "auth failed");
      return;
    }
    // 认证通过：下面这段同步执行，不会和别的连接的认证交错。先确认 READY 发得出去，再替换同一个 hostId 的旧连接
    // （旧的 4409，它的客户端 4404）
    if (
      !safeSend(ws, encodeOuter({ type: OuterType.Ready, stream: 0, payload: new Uint8Array() }))
    ) {
      markClosed(ws, CLOSE.authFailed, "gone");
      return;
    }
    const epoch = this.nextEpoch();
    const old = this.hostSocket();
    if (old) {
      const oldEpoch = (this.attachment(old) as Attachment & { t: "host" }).epoch;
      this.dropHost(old, oldEpoch, CLOSE.replaced, "replaced by a newer connection");
    }
    ws.serializeAttachment({ t: "host", epoch, host: a.host } satisfies Attachment);
  }

  private async onHostMessage(
    ws: WebSocket,
    a: Attachment & { t: "host" },
    message: ArrayBuffer | string,
  ): Promise<void> {
    if (typeof message === "string") {
      // ping 由自动应答处理；别的文本消息忽略
      return;
    }
    const f = parseOuter(new Uint8Array(message));
    if (!f) {
      this.dropHost(ws, a.epoch, CLOSE.protocol, "bad frame");
      return;
    }
    switch (f.type) {
      case OuterType.Data: {
        const c = this.clientFor(f.stream);
        if (!c) return;
        const ca = this.attachment(c) as Attachment & { t: "client" };
        if (!ca.accepted) {
          // ACCEPT 以前只放行一条（拒绝握手的回话），不计量
          if (ca.hostSent || f.payload.length > MAX_UNCONFIRMED) {
            this.closeClient(c, CLOSE.protocol, "wait for ACCEPT", true);
            return;
          }
          c.serializeAttachment({
            ...ca,
            hostSent: true,
            hostPending: f.payload.length,
          } satisfies Attachment);
        } else if (!(await this.count(f.payload.length))) return;
        // 发不出去（运行时的发送缓冲满了、连接已经断了）：按跟不上的客户端处理
        if (!safeSend(c, f.payload)) this.closeClient(c, CLOSE.limited, "slow reader", true);
        return;
      }
      case OuterType.Close: {
        const c = this.clientFor(f.stream);
        if (!c) return;
        const { code, reason } = parseClosePayload(f.payload);
        this.closeClient(c, clientCloseCode(code), reason, false);
        return;
      }
      case OuterType.Accept: {
        // 握手通过了：之前两个方向的那一条一起计量；重复的忽略
        const c = this.clientFor(f.stream);
        if (!c) return;
        const ca = this.attachment(c) as Attachment & { t: "client" };
        if (ca.accepted) return;
        const n = (ca.clientPending ?? 0) + (ca.hostPending ?? 0);
        c.serializeAttachment({
          ...ca,
          accepted: true,
          clientPending: 0,
          hostPending: 0,
        } satisfies Attachment);
        if (n > 0) await this.count(n);
        return;
      }
      default:
        this.dropHost(ws, a.epoch, CLOSE.protocol, "unexpected frame");
    }
  }

  private async onClientMessage(
    ws: WebSocket,
    a: Attachment & { t: "client" },
    message: ArrayBuffer | string,
  ): Promise<void> {
    if (typeof message === "string" || message.byteLength === 0) {
      this.closeClient(ws, CLOSE.protocol, "binary only", true);
      return;
    }
    if (message.byteLength > MAX_NOISE_MESSAGE) {
      this.closeClient(ws, CLOSE.tooBig, "message too big", true);
      return;
    }
    const host = this.hostSocket();
    const ha = host ? (this.attachment(host) as Attachment & { t: "host" }) : null;
    if (!host || !ha || ha.epoch !== a.epoch) {
      this.closeClient(ws, CLOSE.hostOffline, "host offline", false);
      return;
    }
    if (!a.accepted) {
      // ACCEPT 以前只放行一条不大的消息（Noise 握手的第一条），不计量：未认证的客户端耗不掉主机的额度
      if (a.clientSent || message.byteLength > MAX_UNCONFIRMED) {
        this.closeClient(ws, CLOSE.protocol, "wait for the host", true);
        return;
      }
      ws.serializeAttachment({
        ...a,
        clientSent: true,
        clientPending: message.byteLength,
      } satisfies Attachment);
    } else if (!(await this.count(message.byteLength))) {
      this.closeClient(ws, CLOSE.limited, "daily quota exceeded", true);
      return;
    }
    if (
      !safeSend(
        host,
        encodeOuter({ type: OuterType.Data, stream: a.stream, payload: new Uint8Array(message) }),
      )
    ) {
      this.closeClient(ws, CLOSE.hostOffline, "host offline", false);
    }
  }

  /** 关掉主机连接与它的客户端流（4404） */
  private dropHost(ws: WebSocket, epoch: number, code: number, reason: string): void {
    markClosed(ws, code, reason);
    for (const c of this.clients(epoch))
      this.closeClient(c, CLOSE.hostOffline, "host offline", false);
  }

  override async webSocketClose(ws: WebSocket, code: number, reason: string): Promise<void> {
    const a = this.attachment(ws);
    if (a?.t === "host") {
      this.dropHost(ws, a.epoch, 1000, "");
    } else if (a?.t === "client") {
      // 客户端走了：把它的关闭码告诉主机
      this.closeClient(ws, clientCloseCode(code), reason, true);
    } else if (a?.t === "pending" || a?.t === "authing") {
      // 认证中的主机连接断了：标成 closed，正在验签的那一段看到以后不会再替换在线的主机
      markClosed(ws, 1000, "");
    }
    safeClose(ws, 1000, "");
  }

  override async webSocketError(ws: WebSocket): Promise<void> {
    await this.webSocketClose(ws, 1011, "error");
  }

  /** 闹钟：关掉认证超时的主机连接，把准确的用量写回去 */
  override async alarm(): Promise<void> {
    await this.settleUsage();
    const now = Date.now();
    let next = 0;
    for (const ws of this.ctx.getWebSockets()) {
      const a = this.attachment(ws);
      if (a?.t !== "pending" && a?.t !== "authing") continue;
      if (now - a.at >= AUTH_TIMEOUT_MS) markClosed(ws, CLOSE.authFailed, "auth timeout");
      else next = next === 0 ? a.at + AUTH_TIMEOUT_MS : Math.min(next, a.at + AUTH_TIMEOUT_MS);
    }
    if (next > 0) await this.alarmBy(next);
  }
}

/** 主机给的关闭码换成能发给客户端的（1000、1001、3000–4999 原样，别的换成 1000） */
function clientCloseCode(code: number): number {
  if (code === 1000 || code === 1001) return code;
  if (code >= 3000 && code <= 4999) return code;
  return 1000;
}
