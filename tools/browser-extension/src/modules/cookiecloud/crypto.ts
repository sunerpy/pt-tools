/*
 * CookieCloud 的端对端加密，和 CookieCloud 扩展、pt-tools 的 internal/cookiecloud 用同一种格式：
 *   - 口令是 md5(uuid + "-" + password) 十六进制的前 16 个字符；
 *   - aes-128-cbc-fixed（CookieCloud 0.3.0 起）：key 就是这 16 个字符（AES-128），iv 是 16 个 0 字节，密文是标准 base64；
 *   - legacy：CryptoJS 的口令模式，base64("Salted__" + 8 字节 salt + 密文)，key 与 iv 由 EVP_BytesToKey(MD5, 口令, salt)
 *     派生，AES-256-CBC。
 * 都是 CBC + PKCS7（WebCrypto 的 AES-CBC 自带 PKCS7 填充）。密码只在本机用来派生密钥，不发给服务端。
 */
import { md5, md5Hex } from "./md5";

export type CookieCloudCrypto = "aes-128-cbc-fixed" | "legacy";

export const CRYPTO_FIXED: CookieCloudCrypto = "aes-128-cbc-fixed";
export const CRYPTO_LEGACY: CookieCloudCrypto = "legacy";

/** DecryptError 表示解不开：多半是 UUID 或密码不对。 */
export class DecryptError extends Error {
  constructor() {
    super("decrypt failed");
    this.name = "DecryptError";
  }
}

const SALTED = new TextEncoder().encode("Salted__");

/** passphrase 是 md5(uuid + "-" + password) 十六进制的前 16 个字符。 */
export function passphrase(uuid: string, password: string): string {
  return md5Hex(`${uuid.trim()}-${password}`).slice(0, 16);
}

function toBase64(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i += 0x8000) {
    binary += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  }
  return btoa(binary);
}

function fromBase64(text: string): Uint8Array<ArrayBuffer> {
  const binary = atob(text.trim());
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) {
    out[i] = binary.charCodeAt(i);
  }
  return out;
}

/** evpBytesToKey 是 OpenSSL 的 EVP_BytesToKey（MD5，迭代 1 次）：D_i = MD5(D_{i-1} || 口令 || salt)，拼到 key + iv 的长度。 */
export function evpBytesToKey(
  password: Uint8Array,
  salt: Uint8Array,
  keyLen: number,
  ivLen: number,
): { key: Uint8Array<ArrayBuffer>; iv: Uint8Array<ArrayBuffer> } {
  const out = new Uint8Array(keyLen + ivLen);
  let filled = 0;
  let prev: Uint8Array<ArrayBuffer> = new Uint8Array(0);
  while (filled < out.length) {
    const input = new Uint8Array(prev.length + password.length + salt.length);
    input.set(prev);
    input.set(password, prev.length);
    input.set(salt, prev.length + password.length);
    prev = md5(input);
    const n = Math.min(prev.length, out.length - filled);
    out.set(prev.subarray(0, n), filled);
    filled += n;
  }
  return { key: out.slice(0, keyLen), iv: out.slice(keyLen) };
}

async function aesCbc(
  mode: "encrypt" | "decrypt",
  key: Uint8Array<ArrayBuffer>,
  iv: Uint8Array<ArrayBuffer>,
  data: Uint8Array<ArrayBuffer>,
): Promise<Uint8Array<ArrayBuffer>> {
  const cryptoKey = await crypto.subtle.importKey("raw", key, { name: "AES-CBC" }, false, [mode]);
  const params = { name: "AES-CBC", iv };
  const result =
    mode === "encrypt"
      ? await crypto.subtle.encrypt(params, cryptoKey, data)
      : await crypto.subtle.decrypt(params, cryptoKey, data);
  return new Uint8Array(result);
}

/**
 * encrypt 加密 plain（UTF-8 文本）。legacy 每次用随机的 8 字节 salt；salt 参数只给测试用。
 */
export async function encrypt(
  plain: string,
  pass: string,
  type: CookieCloudCrypto,
  salt?: Uint8Array<ArrayBuffer>,
): Promise<string> {
  const data = new TextEncoder().encode(plain);
  const passBytes = new TextEncoder().encode(pass);
  if (type === CRYPTO_FIXED) {
    return toBase64(await aesCbc("encrypt", passBytes, new Uint8Array(16), data));
  }
  const s = salt ?? crypto.getRandomValues(new Uint8Array(8));
  const { key, iv } = evpBytesToKey(passBytes, s, 32, 16);
  const body = await aesCbc("encrypt", key, iv, data);
  const out = new Uint8Array(SALTED.length + s.length + body.length);
  out.set(SALTED);
  out.set(s, SALTED.length);
  out.set(body, SALTED.length + s.length);
  return toBase64(out);
}

function isSalted(raw: Uint8Array): boolean {
  return raw.length >= 16 && SALTED.every((b, i) => raw[i] === b);
}

/**
 * decrypt 解密，返回 UTF-8 文本。type 为空（老服务端）时：是 OpenSSL 格式的按 legacy，否则按 fixed；解不开抛 DecryptError。
 */
export async function decrypt(encrypted: string, pass: string, type?: string): Promise<string> {
  let raw: Uint8Array<ArrayBuffer>;
  try {
    raw = fromBase64(encrypted);
  } catch {
    throw new DecryptError();
  }
  const passBytes = new TextEncoder().encode(pass);
  const legacy = type === CRYPTO_LEGACY || (!type && isSalted(raw));
  if (type && type !== CRYPTO_LEGACY && type !== CRYPTO_FIXED) {
    throw new DecryptError();
  }
  try {
    let plain: Uint8Array<ArrayBuffer>;
    if (legacy) {
      if (!isSalted(raw)) {
        throw new DecryptError();
      }
      const { key, iv } = evpBytesToKey(passBytes, raw.slice(8, 16), 32, 16);
      plain = await aesCbc("decrypt", key, iv, raw.slice(16));
    } else {
      plain = await aesCbc("decrypt", passBytes, new Uint8Array(16), raw);
    }
    return new TextDecoder("utf-8", { fatal: true }).decode(plain);
  } catch {
    throw new DecryptError();
  }
}
