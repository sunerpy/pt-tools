/**
 * 只为读种子文件里的 tracker 地址写的 bencode 解析：整份解析成键值，取 announce 与 announce-list。
 * 种子文件本身不保存，也不往外传；格式不对时回 null。
 */

type BValue = Uint8Array | number | BValue[] | Map<string, BValue>;

const D = 0x64; // d
const L = 0x6c; // l
const I = 0x69; // i
const E = 0x65; // e
const COLON = 0x3a; // :

/** 嵌套层数上限：防止畸形文件把栈撑爆 */
const MAX_DEPTH = 64;

const decoder = new TextDecoder("utf-8", { fatal: false });

class BencodeError extends Error {}

function parseInt10(bytes: Uint8Array, start: number, end: number): number {
  if (end <= start) throw new BencodeError("empty integer");
  const text = decoder.decode(bytes.subarray(start, end));
  if (!/^-?\d+$/.test(text)) throw new BencodeError("bad integer");
  return Number(text);
}

function parseValue(bytes: Uint8Array, cursor: { pos: number }, depth: number): BValue {
  if (depth > MAX_DEPTH) throw new BencodeError("too deep");
  if (cursor.pos >= bytes.length) throw new BencodeError("unexpected end");
  const head = bytes[cursor.pos];

  if (head === I) {
    const end = bytes.indexOf(E, cursor.pos + 1);
    if (end < 0) throw new BencodeError("unterminated integer");
    const value = parseInt10(bytes, cursor.pos + 1, end);
    cursor.pos = end + 1;
    return value;
  }

  if (head === L) {
    cursor.pos += 1;
    const list: BValue[] = [];
    while (cursor.pos < bytes.length && bytes[cursor.pos] !== E) {
      list.push(parseValue(bytes, cursor, depth + 1));
    }
    if (cursor.pos >= bytes.length) throw new BencodeError("unterminated list");
    cursor.pos += 1;
    return list;
  }

  if (head === D) {
    cursor.pos += 1;
    const dict = new Map<string, BValue>();
    while (cursor.pos < bytes.length && bytes[cursor.pos] !== E) {
      const key = parseValue(bytes, cursor, depth + 1);
      if (!(key instanceof Uint8Array)) throw new BencodeError("non-string key");
      dict.set(decoder.decode(key), parseValue(bytes, cursor, depth + 1));
    }
    if (cursor.pos >= bytes.length) throw new BencodeError("unterminated dict");
    cursor.pos += 1;
    return dict;
  }

  if (head >= 0x30 && head <= 0x39) {
    const colon = bytes.indexOf(COLON, cursor.pos);
    if (colon < 0) throw new BencodeError("bad string length");
    const length = parseInt10(bytes, cursor.pos, colon);
    const start = colon + 1;
    if (length < 0 || start + length > bytes.length) throw new BencodeError("string overflow");
    cursor.pos = start + length;
    return bytes.subarray(start, start + length);
  }

  throw new BencodeError("unknown token");
}

/** 种子文件里的 tracker 地址（announce 在前，announce-list 按顺序在后）；不是种子文件时为 null */
export function parseTorrentAnnounces(bytes: Uint8Array): string[] | null {
  if (bytes.length === 0 || bytes[0] !== D) {
    return null;
  }
  let root: BValue;
  try {
    root = parseValue(bytes, { pos: 0 }, 0);
  } catch {
    return null;
  }
  if (!(root instanceof Map)) {
    return null;
  }

  const urls: string[] = [];
  const announce = root.get("announce");
  if (announce instanceof Uint8Array) {
    urls.push(decoder.decode(announce));
  }
  const tiers = root.get("announce-list");
  if (Array.isArray(tiers)) {
    for (const tier of tiers) {
      if (!Array.isArray(tier)) continue;
      for (const item of tier) {
        if (item instanceof Uint8Array) urls.push(decoder.decode(item));
      }
    }
  }
  return urls;
}

/** tracker 地址只留主机名：去重、排序，解析不了的丢掉（地址里的 passkey 不留） */
export function trackerHostsOf(urls: string[]): string[] {
  const hosts = new Set<string>();
  for (const raw of urls) {
    try {
      const host = new URL(raw.trim()).hostname.toLowerCase();
      if (host) hosts.add(host);
    } catch {
      // 不是地址
    }
  }
  return [...hosts].sort();
}
