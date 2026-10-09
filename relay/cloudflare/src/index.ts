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
  MAX_NOISE_MESSAGE,
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
/** 每日用量最多攒多少字节写一次存储（休眠时没写的部分会丢，按少算） */
const USAGE_FLUSH_BYTES = 1 << 20;

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
      const ip = request.headers.get("CF-Connecting-IP") ?? "unknown";
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

/** 每个 IP 一个：60 秒的窗口里最多 limit 次新建连接。被逐出以后计数从头开始（宽松一侧）。 */
export class IPLimiter extends DurableObject<Env> {
  private start = 0;
  private count = 0;

  async allow(limit: number): Promise<boolean> {
    const now = Date.now();
    if (now - this.start >= 60_000) {
      this.start = now;
      this.count = 0;
    }
    if (this.count >= limit) return false;
    this.count++;
    return true;
  }
}

/** 连接上的附加信息（跨休眠保留，最大 16 KiB） */
type Attachment =
  | { t: "pending"; nonce: string; at: number; origin: string; host: string }
  | { t: "host"; epoch: number; host: string }
  | { t: "client"; stream: number; epoch: number };

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
  /** 内存里没写进存储的用量（休眠时丢掉，按少算） */
  private pendingBytes = 0;
  private usage: Usage | null = null;

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
      await this.ctx.storage.setAlarm(Date.now() + AUTH_TIMEOUT_MS);
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
      safeClose(server, CLOSE.hostOffline, "host offline");
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

  private async nextStream(): Promise<number> {
    const n = ((await this.ctx.storage.get<number>("nextStream")) ?? 0) + 1;
    const stream = n > 0xffffffff ? 1 : n;
    await this.ctx.storage.put("nextStream", stream);
    return stream;
  }

  private async nextEpoch(): Promise<number> {
    const e = ((await this.ctx.storage.get<number>("epoch")) ?? 0) + 1;
    await this.ctx.storage.put("epoch", e);
    return e;
  }

  private async loadUsage(): Promise<Usage> {
    const day = utcDay();
    if (!this.usage)
      this.usage = (await this.ctx.storage.get<Usage>("usage")) ?? { day, bytes: 0, over: false };
    if (this.usage.day !== day) {
      this.usage = { day, bytes: 0, over: false };
      this.pendingBytes = 0;
      await this.ctx.storage.put("usage", this.usage);
    }
    return this.usage;
  }

  private async overQuota(lim: Limits): Promise<boolean> {
    if (lim.dailyBytes <= 0) return false;
    return (await this.loadUsage()).over;
  }

  /** 记下转发的字节；刚超额时关掉全部客户端流（4429），返回还能不能继续转发 */
  private async count(n: number): Promise<boolean> {
    const lim = limits(this.env);
    if (lim.dailyBytes <= 0) return true;
    const u = await this.loadUsage();
    if (u.over) return false;
    u.bytes += n;
    this.pendingBytes += n;
    if (u.bytes > lim.dailyBytes) {
      u.over = true;
      this.pendingBytes = 0;
      await this.ctx.storage.put("usage", u);
      for (const ws of this.clients()) safeClose(ws, CLOSE.limited, "daily quota exceeded");
      return false;
    }
    if (this.pendingBytes >= USAGE_FLUSH_BYTES) {
      this.pendingBytes = 0;
      await this.ctx.storage.put("usage", u);
    }
    return true;
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
    if (
      !f ||
      f.type !== OuterType.Auth ||
      !(await verifyRelayAuth(a.host, unb64(a.nonce), a.origin, f.payload))
    ) {
      safeClose(ws, CLOSE.authFailed, "auth failed");
      return;
    }
    // 认证通过：替换同一个 hostId 的旧连接（旧的 4409，它的客户端 4404）
    const old = this.hostSocket();
    if (old) {
      const oldEpoch = (this.attachment(old) as Attachment & { t: "host" }).epoch;
      old.serializeAttachment({
        t: "pending",
        nonce: "",
        at: 0,
        origin: "",
        host: "",
      } satisfies Attachment);
      safeClose(old, CLOSE.replaced, "replaced by a newer connection");
      for (const c of this.clients(oldEpoch)) safeClose(c, CLOSE.hostOffline, "host offline");
    }
    const epoch = await this.nextEpoch();
    ws.serializeAttachment({ t: "host", epoch, host: a.host } satisfies Attachment);
    safeSend(ws, encodeOuter({ type: OuterType.Ready, stream: 0, payload: new Uint8Array() }));
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
        if (!(await this.count(f.payload.length))) return;
        if (!safeSend(c, f.payload)) safeClose(c, 1011, "write failed");
        return;
      }
      case OuterType.Close: {
        const c = this.clientFor(f.stream);
        if (!c) return;
        const { code, reason } = parseClosePayload(f.payload);
        safeClose(c, clientCloseCode(code), reason);
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
      safeClose(ws, CLOSE.protocol, "binary only");
      return;
    }
    if (message.byteLength > MAX_NOISE_MESSAGE) {
      safeClose(ws, CLOSE.tooBig, "message too big");
      return;
    }
    const host = this.hostSocket();
    const ha = host ? (this.attachment(host) as Attachment & { t: "host" }) : null;
    if (!host || !ha || ha.epoch !== a.epoch) {
      safeClose(ws, CLOSE.hostOffline, "host offline");
      return;
    }
    if (!(await this.count(message.byteLength))) {
      safeClose(ws, CLOSE.limited, "daily quota exceeded");
      return;
    }
    if (
      !safeSend(
        host,
        encodeOuter({ type: OuterType.Data, stream: a.stream, payload: new Uint8Array(message) }),
      )
    ) {
      safeClose(ws, CLOSE.hostOffline, "host offline");
    }
  }

  /** 关掉主机连接与它的客户端流（4404） */
  private dropHost(ws: WebSocket, epoch: number, code: number, reason: string): void {
    ws.serializeAttachment({
      t: "pending",
      nonce: "",
      at: 0,
      origin: "",
      host: "",
    } satisfies Attachment);
    safeClose(ws, code, reason);
    for (const c of this.clients(epoch)) safeClose(c, CLOSE.hostOffline, "host offline");
  }

  override async webSocketClose(ws: WebSocket, code: number, reason: string): Promise<void> {
    const a = this.attachment(ws);
    if (a?.t === "host") {
      for (const c of this.clients(a.epoch)) safeClose(c, CLOSE.hostOffline, "host offline");
    } else if (a?.t === "client") {
      const host = this.hostSocket();
      const ha = host ? (this.attachment(host) as Attachment & { t: "host" }) : null;
      if (host && ha && ha.epoch === a.epoch) {
        safeSend(
          host,
          encodeOuter({
            type: OuterType.Close,
            stream: a.stream,
            payload: closePayload(clientCloseCode(code), reason),
          }),
        );
      }
    }
    safeClose(ws, 1000, "");
    if (this.usage && this.pendingBytes > 0) {
      this.pendingBytes = 0;
      await this.ctx.storage.put("usage", this.usage);
    }
  }

  override async webSocketError(ws: WebSocket): Promise<void> {
    await this.webSocketClose(ws, 1011, "error");
  }

  /** 认证超时：还没回 AUTH 的主机连接关掉 */
  override async alarm(): Promise<void> {
    const now = Date.now();
    let next = 0;
    for (const ws of this.ctx.getWebSockets()) {
      const a = this.attachment(ws);
      if (a?.t !== "pending" || a.at === 0) continue;
      if (now - a.at >= AUTH_TIMEOUT_MS) safeClose(ws, CLOSE.authFailed, "auth timeout");
      else next = next === 0 ? a.at + AUTH_TIMEOUT_MS : Math.min(next, a.at + AUTH_TIMEOUT_MS);
    }
    if (next > 0) await this.ctx.storage.setAlarm(next);
  }
}

/** 主机给的关闭码换成能发给客户端的（1000、1001、3000–4999 原样，别的换成 1000） */
function clientCloseCode(code: number): number {
  if (code === 1000 || code === 1001) return code;
  if (code >= 3000 && code <= 4999) return code;
  return 1000;
}
