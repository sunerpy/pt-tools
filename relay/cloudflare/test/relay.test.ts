// relay（Cloudflare 版）的单测：跑在 workerd 里（vitest-pool-workers），经 Worker 的 fetch 建 WebSocket。
// 限额在 vitest.config.ts 里调小：每主机 2 个流、每天 4096 字节、每 IP 不限。
// 线格式与 Go 版的一致性用 internal/remote/testdata/vectors.json 核对（同一份向量，App 也用它）。
import { env, runInDurableObject, SELF } from "cloudflare:test";
import { describe, expect, it } from "vitest";

import vectors from "../../../internal/remote/testdata/vectors.json";
import type { Env } from "../src/index";
import {
  CLOSE,
  closePayload,
  encodeOuter,
  hostIdOf,
  ipKey,
  OuterType,
  parseOuter,
  relayAuthMessage,
  relayOrigin,
  validHostId,
  verifyRelayAuth,
} from "../src/protocol";

const BASE = "https://relay.test";
const ORIGIN = "wss://relay.test";

function hex(s: string): Uint8Array<ArrayBuffer> {
  const out = new Uint8Array(s.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(s.slice(i * 2, i * 2 + 2), 16);
  return out;
}

function toHex(b: Uint8Array): string {
  return [...b].map((x) => x.toString(16).padStart(2, "0")).join("");
}

/** 收消息的队列：next() 等下一条，closed 是关闭码 */
class Conn {
  private queue: (ArrayBuffer | string)[] = [];
  private waiters: ((m: ArrayBuffer | string | null) => void)[] = [];
  closed: Promise<number>;

  constructor(readonly ws: WebSocket) {
    let resolveClosed!: (code: number) => void;
    this.closed = new Promise((r) => (resolveClosed = r));
    ws.addEventListener("message", (e) => {
      const w = this.waiters.shift();
      if (w) w(e.data as ArrayBuffer | string);
      else this.queue.push(e.data as ArrayBuffer | string);
    });
    ws.addEventListener("close", (e) => {
      resolveClosed(e.code);
      for (const w of this.waiters.splice(0)) w(null);
    });
  }

  next(): Promise<ArrayBuffer | string | null> {
    const m = this.queue.shift();
    if (m !== undefined) return Promise.resolve(m);
    return new Promise((r) => this.waiters.push(r));
  }

  async outer() {
    const m = await this.next();
    expect(m).not.toBeNull();
    expect(typeof m).not.toBe("string");
    const f = parseOuter(new Uint8Array(m as ArrayBuffer));
    expect(f).not.toBeNull();
    return f!;
  }

  send(b: Uint8Array | string) {
    this.ws.send(b);
  }
}

async function open(path: string): Promise<Conn> {
  const res = await SELF.fetch(`${BASE}${path}`, { headers: { Upgrade: "websocket" } });
  expect(res.status).toBe(101);
  const ws = res.webSocket!;
  // 新的兼容日期下二进制消息默认是 Blob（和浏览器一样），这里按 ArrayBuffer 收
  ws.binaryType = "arraybuffer";
  ws.accept();
  return new Conn(ws);
}

interface Host {
  conn: Conn;
  id: string;
}

async function newKey() {
  const kp = (await crypto.subtle.generateKey({ name: "Ed25519" }, true, [
    "sign",
    "verify",
  ])) as CryptoKeyPair;
  const pub = new Uint8Array((await crypto.subtle.exportKey("raw", kp.publicKey)) as ArrayBuffer);
  return { kp, pub, id: await hostIdOf(pub) };
}

async function connectHost(
  key?: Awaited<ReturnType<typeof newKey>>,
  origin = ORIGIN,
): Promise<Host & { ready: boolean }> {
  const k = key ?? (await newKey());
  const conn = await open(`/v1/host/${k.id}`);
  const ch = await conn.outer();
  expect(ch.type).toBe(OuterType.Challenge);
  const sig = new Uint8Array(
    await crypto.subtle.sign(
      { name: "Ed25519" },
      k.kp.privateKey,
      relayAuthMessage(k.id, ch.payload, origin),
    ),
  );
  const auth = new Uint8Array(96);
  auth.set(k.pub, 0);
  auth.set(sig, 32);
  conn.send(encodeOuter({ type: OuterType.Auth, stream: 0, payload: auth }));
  const m = await conn.next();
  if (m === null) return { conn, id: k.id, ready: false };
  const f = parseOuter(new Uint8Array(m as ArrayBuffer));
  return { conn, id: k.id, ready: f?.type === OuterType.Ready };
}

describe("线格式与 Go 版一致（vectors.json）", () => {
  it("hostId", async () => {
    for (const v of vectors.host_ids) expect(await hostIdOf(hex(v.ed25519_public))).toBe(v.host_id);
    expect(validHostId(vectors.host_ids[0]!.host_id)).toBe(true);
    expect(validHostId("NOT-A-HOST")).toBe(false);
  });

  it("外层帧：编出来一字不差，不合规的解不开", () => {
    for (const f of vectors.outer_frames) {
      const enc = encodeOuter({ type: f.type, stream: f.stream, payload: hex(f.payload) });
      expect(toHex(enc)).toBe(f.encoded);
      const back = parseOuter(hex(f.encoded));
      expect(back?.type).toBe(f.type);
      expect(back?.stream).toBe(f.stream);
    }
    for (const b of vectors.invalid_outer_frames) expect(parseOuter(hex(b))).toBeNull();
  });

  it("relay 认证：Go 签的 AUTH 在这里校验得过，换 origin 就不行", async () => {
    const v = vectors.relay_auth;
    expect(toHex(relayAuthMessage(v.host_id, hex(v.nonce), v.origin))).toBe(v.message);
    expect(await verifyRelayAuth(v.host_id, hex(v.nonce), v.origin, hex(v.auth))).toBe(true);
    expect(
      await verifyRelayAuth(v.host_id, hex(v.nonce), "wss://evil.example.com", hex(v.auth)),
    ).toBe(false);
  });

  it("origin 规整", () => {
    expect(relayOrigin("https://Relay.Example.com:443/v1/host/x")).toBe("wss://relay.example.com");
    expect(relayOrigin("ws://127.0.0.1:8787")).toBe("ws://127.0.0.1:8787");
    expect(relayOrigin("http://[::1]:80/")).toBe("ws://[::1]");
  });

  it("限流的键：IPv4 按地址，IPv6 按 /64，映射的 IPv4 当 IPv4", () => {
    expect(ipKey("1.2.3.4")).toBe("1.2.3.4");
    expect(ipKey("::ffff:1.2.3.4")).toBe("1.2.3.4");
    expect(ipKey("2001:db8:1:2:aaaa::1")).toBe("2001:db8:1:2::/64");
    expect(ipKey("2001:DB8:1:2:bbbb:cccc:0:9")).toBe("2001:db8:1:2::/64");
    expect(ipKey("2001:db8::1")).toBe("2001:db8:0:0::/64");
    expect(ipKey("::1")).toBe("0:0:0:0::/64");
    expect(ipKey("fe80::1%eth0")).toBe("fe80:0:0:0::/64");
    expect(ipKey("unknown")).toBe("unknown");
    expect(ipKey("1:2:3")).toBe("1:2:3");
  });

  it("CLOSE 的原因截在字符边界上", () => {
    const p = closePayload(4404, "长".repeat(60));
    expect(p.length).toBeLessThanOrEqual(2 + 123);
    expect(new TextDecoder().decode(p.subarray(2))).toMatch(/^长+$/);
  });
});

describe("relay", () => {
  it("healthz", async () => {
    const res = await SELF.fetch(`${BASE}/healthz`);
    expect(res.status).toBe(200);
    expect(await res.json()).toEqual({ ok: true, version: "test", disabled: false });
    expect((await SELF.fetch(`${BASE}/v1/other/x`)).status).toBe(404);
  });

  it("不是 WebSocket 升级回 426；hostId 不对 4400", async () => {
    expect((await SELF.fetch(`${BASE}/v1/host/${"a".repeat(26)}`)).status).toBe(426);
    const c = await open("/v1/client/NOT-A-HOST");
    expect(await c.closed).toBe(CLOSE.protocol);
  });

  it("认证通过以后转发：客户端的消息装进 DATA，主机的 DATA 原样给客户端，两边都能关", async () => {
    const h = await connectHost();
    expect(h.ready).toBe(true);
    const c = await open(`/v1/client/${h.id}`);
    const o = await h.conn.outer();
    expect(o.type).toBe(OuterType.Open);
    c.send(new TextEncoder().encode("hello"));
    const d = await h.conn.outer();
    expect(d.type).toBe(OuterType.Data);
    expect(d.stream).toBe(o.stream);
    expect(new TextDecoder().decode(d.payload)).toBe("hello");
    h.conn.send(
      encodeOuter({
        type: OuterType.Data,
        stream: o.stream,
        payload: new TextEncoder().encode("world"),
      }),
    );
    expect(new TextDecoder().decode((await c.next()) as ArrayBuffer)).toBe("world");
    h.conn.send(
      encodeOuter({ type: OuterType.Close, stream: o.stream, payload: closePayload(4321, "bye") }),
    );
    expect(await c.closed).toBe(4321);
  });

  it("认证超时的闹钟只往前挪：不停有新的未认证连接时，早来的照样按时关掉", async () => {
    const k = await newKey();
    const stub = (env as unknown as Env).HOST_RELAY.get(
      (env as unknown as Env).HOST_RELAY.idFromName(k.id),
    );
    const a = await open(`/v1/host/${k.id}`);
    expect((await a.outer()).type).toBe(OuterType.Challenge);
    const first = await runInDurableObject(stub, (_, state) => state.storage.getAlarm());
    expect(first).not.toBeNull();
    await new Promise((r) => setTimeout(r, 30));
    const b = await open(`/v1/host/${k.id}`);
    expect((await b.outer()).type).toBe(OuterType.Challenge);
    expect(await runInDurableObject(stub, (_, state) => state.storage.getAlarm())).toBe(first);
    a.ws.close(1000, "");
    b.ws.close(1000, "");
  });

  it("主机 ping 回 pong", async () => {
    const h = await connectHost();
    h.conn.send("ping");
    expect(await h.conn.next()).toBe("pong");
  });

  it("签名的 origin 不对：4401", async () => {
    const h = await connectHost(undefined, "wss://another-relay.example.com");
    expect(h.ready).toBe(false);
    expect(await h.conn.closed).toBe(CLOSE.authFailed);
  });

  it("主机不在线 4404；主机断开以后它的客户端 4404", async () => {
    const k = await newKey();
    const lonely = await open(`/v1/client/${k.id}`);
    expect(await lonely.closed).toBe(CLOSE.hostOffline);
    const h = await connectHost(k);
    const c = await open(`/v1/client/${h.id}`);
    await h.conn.outer();
    h.conn.ws.close(1000, "");
    expect(await c.closed).toBe(CLOSE.hostOffline);
  });

  it("同一个 hostId 的新连接替换旧的：旧的 4409，旧连接的客户端 4404", async () => {
    const k = await newKey();
    const old = await connectHost(k);
    const c = await open(`/v1/client/${k.id}`);
    await old.conn.outer();
    const fresh = await connectHost(k);
    expect(fresh.ready).toBe(true);
    expect(await old.conn.closed).toBe(CLOSE.replaced);
    expect(await c.closed).toBe(CLOSE.hostOffline);
    const c2 = await open(`/v1/client/${k.id}`);
    expect((await fresh.conn.outer()).type).toBe(OuterType.Open);
    c2.ws.close(1000, "");
  });

  it("每主机流上限（这里是 2）：第 3 个 4429", async () => {
    const h = await connectHost();
    const a = await open(`/v1/client/${h.id}`);
    await h.conn.outer();
    const b = await open(`/v1/client/${h.id}`);
    await h.conn.outer();
    const extra = await open(`/v1/client/${h.id}`);
    expect(await extra.closed).toBe(CLOSE.limited);
    a.ws.close(1000, "");
    b.ws.close(1000, "");
  });

  it("客户端发文本 4400，发超过 65535 字节 1009", async () => {
    const h = await connectHost();
    const t = await open(`/v1/client/${h.id}`);
    await h.conn.outer();
    t.send("hello");
    expect(await t.closed).toBe(CLOSE.protocol);
    const big = await open(`/v1/client/${h.id}`);
    await h.conn.outer();
    big.send(new Uint8Array(65536));
    expect(await big.closed).toBe(CLOSE.tooBig);
  });

  it("主机发不合规的帧：主机 4400，它的客户端 4404", async () => {
    const h = await connectHost();
    const c = await open(`/v1/client/${h.id}`);
    await h.conn.outer();
    h.conn.send(new Uint8Array([0x55, 0, 0, 0, 1]));
    expect(await h.conn.closed).toBe(CLOSE.protocol);
    expect(await c.closed).toBe(CLOSE.hostOffline);
  });

  it("每天的转发量（这里是 4096 字节）：超额以后客户端流 4429，新的客户端也 4429，主机连接还在", async () => {
    const h = await connectHost();
    const c = await open(`/v1/client/${h.id}`);
    await h.conn.outer();
    c.send(new Uint8Array(3000));
    expect((await h.conn.outer()).type).toBe(OuterType.Data);
    c.send(new Uint8Array(3000));
    expect(await c.closed).toBe(CLOSE.limited);
    const c2 = await open(`/v1/client/${h.id}`);
    expect(await c2.closed).toBe(CLOSE.limited);
    // 主机连接还在：前面可能排着关掉那个流的 CLOSE，跳过二进制帧等 pong
    h.conn.send("ping");
    let m = await h.conn.next();
    while (m !== null && typeof m !== "string") m = await h.conn.next();
    expect(m).toBe("pong");
  });
});
