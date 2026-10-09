/**
 * relay 协议 v1 的线格式（docs/design/remote-access.md 的「relay」一节），与 Go 版 internal/remote 的 outer.go 一致。
 * relay 只解外层帧，不碰里面的 Noise 消息。
 */

export const OUTER_HEADER_LEN = 5;
export const MAX_NOISE_MESSAGE = 65535;
export const MAX_OUTER_FRAME = OUTER_HEADER_LEN + MAX_NOISE_MESSAGE;
export const NONCE_LEN = 32;
/**
 * 主机发 ACCEPT 以前，一个流上每个方向只能发一条消息（客户端的 Noise 握手第一条、主机拒绝握手的回话），不超过这么多字节，
 * 也不计量；没有 ACCEPT 就关掉的流不计入每天的转发量
 */
export const MAX_UNCONFIRMED = 4096;
export const AUTH_PAYLOAD_LEN = 32 + 64;
const MAX_CLOSE_REASON = 123;
const RELAY_AUTH_CONTEXT = "pt-tools-relay-v1";
export const HOST_ID_LEN = 26;

export const enum OuterType {
  Challenge = 0x01,
  Auth = 0x02,
  Ready = 0x03,
  Open = 0x10,
  Data = 0x11,
  Close = 0x12,
  /** 主机 → relay：这个流的握手通过了，从这里起计量 */
  Accept = 0x13,
}

/** relay 用的 WebSocket 关闭码 */
export const CLOSE = {
  protocol: 4400,
  authFailed: 4401,
  hostOffline: 4404,
  replaced: 4409,
  limited: 4429,
  disabled: 4503,
  tooBig: 1009,
} as const;

export interface OuterFrame {
  type: number;
  stream: number;
  payload: Uint8Array;
}

/** 检查外层帧的规则（与 Go 版 checkOuter 一致），不对时返回原因 */
function checkOuter(f: OuterFrame): string | null {
  const n = f.payload.length;
  switch (f.type) {
    case OuterType.Challenge:
      return f.stream === 0 && n === NONCE_LEN ? null : "CHALLENGE";
    case OuterType.Auth:
      return f.stream === 0 && n === AUTH_PAYLOAD_LEN ? null : "AUTH";
    case OuterType.Ready:
      return f.stream === 0 && n === 0 ? null : "READY";
    case OuterType.Open:
      return f.stream !== 0 && n === 0 ? null : "OPEN";
    case OuterType.Data:
      return f.stream !== 0 && n > 0 && n <= MAX_NOISE_MESSAGE ? null : "DATA";
    case OuterType.Close: {
      if (f.stream === 0 || n === 1 || n > 2 + MAX_CLOSE_REASON) return "CLOSE";
      if (n > 2) {
        try {
          new TextDecoder("utf-8", { fatal: true, ignoreBOM: false }).decode(f.payload.subarray(2));
        } catch {
          return "CLOSE";
        }
      }
      return null;
    }
    case OuterType.Accept:
      return f.stream !== 0 && n === 0 ? null : "ACCEPT";
    default:
      return "type";
  }
}

export function encodeOuter(f: OuterFrame): Uint8Array {
  const bad = checkOuter(f);
  if (bad) throw new Error(`bad outer frame: ${bad}`);
  const out = new Uint8Array(OUTER_HEADER_LEN + f.payload.length);
  out[0] = f.type;
  new DataView(out.buffer).setUint32(1, f.stream >>> 0, false);
  out.set(f.payload, OUTER_HEADER_LEN);
  return out;
}

export function parseOuter(b: Uint8Array): OuterFrame | null {
  if (b.length < OUTER_HEADER_LEN || b.length > MAX_OUTER_FRAME) return null;
  const f: OuterFrame = {
    type: b[0]!,
    stream: new DataView(b.buffer, b.byteOffset, b.byteLength).getUint32(1, false),
    payload: b.subarray(OUTER_HEADER_LEN),
  };
  return checkOuter(f) ? null : f;
}

/** CLOSE 的内容：u16 关闭码（大端）加原因，原因超长时截在字符边界上 */
export function closePayload(code: number, reason: string): Uint8Array {
  let r = new TextEncoder().encode(reason);
  if (r.length > MAX_CLOSE_REASON) {
    let s = reason;
    while (new TextEncoder().encode(s).length > MAX_CLOSE_REASON) s = [...s].slice(0, -1).join("");
    r = new TextEncoder().encode(s);
  }
  const out = new Uint8Array(2 + r.length);
  new DataView(out.buffer).setUint16(0, code, false);
  out.set(r, 2);
  return out;
}

export function parseClosePayload(p: Uint8Array): { code: number; reason: string } {
  if (p.length < 2) return { code: 0, reason: "" };
  return {
    code: new DataView(p.buffer, p.byteOffset, p.byteLength).getUint16(0, false),
    reason: new TextDecoder().decode(p.subarray(2)),
  };
}

const B32 = "abcdefghijklmnopqrstuvwxyz234567";

/** hostId = lower(base32(sha256(Ed25519 公钥)))[:26]，标准字母表、无填充 */
export async function hostIdOf(pub: Uint8Array): Promise<string> {
  const sum = new Uint8Array(await crypto.subtle.digest("SHA-256", pub));
  let bits = 0;
  let value = 0;
  let out = "";
  for (const byte of sum) {
    value = (value << 8) | byte;
    bits += 8;
    while (bits >= 5) {
      out += B32[(value >>> (bits - 5)) & 31];
      bits -= 5;
    }
  }
  if (bits > 0) out += B32[(value << (5 - bits)) & 31];
  return out.slice(0, HOST_ID_LEN);
}

export function validHostId(s: string): boolean {
  return /^[a-z2-7]{26}$/.test(s);
}

/** 主机签名的内容："pt-tools-relay-v1" || hostId || nonce || relay origin */
export function relayAuthMessage(hostId: string, nonce: Uint8Array, origin: string): Uint8Array {
  const enc = new TextEncoder();
  const parts = [enc.encode(RELAY_AUTH_CONTEXT), enc.encode(hostId), nonce, enc.encode(origin)];
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let off = 0;
  for (const p of parts) {
    out.set(p, off);
    off += p.length;
  }
  return out;
}

/** 校验主机的 AUTH：公钥推导出的 hostId 要对得上，签名要对 */
export async function verifyRelayAuth(
  hostId: string,
  nonce: Uint8Array,
  origin: string,
  payload: Uint8Array,
): Promise<boolean> {
  if (payload.length !== AUTH_PAYLOAD_LEN || nonce.length !== NONCE_LEN) return false;
  const pub = payload.slice(0, 32);
  const sig = payload.slice(32);
  if ((await hostIdOf(pub)) !== hostId) return false;
  try {
    const key = await crypto.subtle.importKey("raw", pub, { name: "Ed25519" }, false, ["verify"]);
    return await crypto.subtle.verify(
      { name: "Ed25519" },
      key,
      sig,
      relayAuthMessage(hostId, nonce, origin),
    );
  } catch {
    return false;
  }
}

/** relay 地址的 origin：scheme://主机[:端口]，小写，去掉默认端口（ws 的 80、wss 的 443） */
export function relayOrigin(raw: string): string {
  const u = new URL(raw);
  let scheme = u.protocol.replace(/:$/, "").toLowerCase();
  if (scheme === "http") scheme = "ws";
  if (scheme === "https") scheme = "wss";
  if (scheme !== "ws" && scheme !== "wss") throw new Error("relay URL must be ws:// or wss://");
  const host = u.hostname.toLowerCase();
  const port = u.port;
  const isDefault =
    (scheme === "ws" && port === "80") || (scheme === "wss" && port === "443") || port === "";
  return `${scheme}://${host}${isDefault ? "" : `:${port}`}`;
}

/**
 * 限流按什么计：IPv4 按地址，IPv6 按 /64（一台机器通常拿得到整个 /64，按地址计等于不限），
 * 映射的 IPv4（::ffff:a.b.c.d）当 IPv4。认不出来的原样返回。
 */
export function ipKey(raw: string): string {
  const s = raw.trim().toLowerCase().split("%")[0]!;
  if (!s.includes(":")) return s;
  const mapped = /^::ffff:(\d{1,3}(?:\.\d{1,3}){3})$/.exec(s);
  if (mapped) return mapped[1]!;
  const halves = s.split("::");
  if (halves.length > 2) return raw;
  const head = halves[0] ? halves[0].split(":") : [];
  const tail = halves.length === 2 && halves[1] ? halves[1].split(":") : [];
  const fill = 8 - head.length - tail.length;
  if (halves.length === 1 ? fill !== 0 : fill < 1) return raw;
  const groups = [...head, ...Array<string>(halves.length === 2 ? fill : 0).fill("0"), ...tail];
  if (groups.some((g) => !/^[0-9a-f]{1,4}$/.test(g))) return raw;
  return `${groups
    .slice(0, 4)
    .map((g) => parseInt(g, 16).toString(16))
    .join(":")}::/64`;
}
