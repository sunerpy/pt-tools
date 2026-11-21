import { describe, expect, it } from "vitest";

import { parseTorrentAnnounces, trackerHostsOf } from "./bencode";

const enc = new TextEncoder();

function bstr(value: string | Uint8Array): Uint8Array {
  const bytes = typeof value === "string" ? enc.encode(value) : value;
  return concat(enc.encode(`${bytes.length}:`), bytes);
}

function concat(...parts: Uint8Array[]): Uint8Array {
  const out = new Uint8Array(parts.reduce((n, p) => n + p.length, 0));
  let offset = 0;
  for (const part of parts) {
    out.set(part, offset);
    offset += part.length;
  }
  return out;
}

const d = (...parts: Uint8Array[]) => concat(enc.encode("d"), ...parts, enc.encode("e"));
const l = (...parts: Uint8Array[]) => concat(enc.encode("l"), ...parts, enc.encode("e"));
const i = (n: number) => enc.encode(`i${n}e`);

/** 一个像样的种子：announce、announce-list，info 里有二进制的 pieces */
function sampleTorrent(): Uint8Array {
  const pieces = new Uint8Array(40).map((_v, idx) => (idx * 37) % 256);
  return d(
    bstr("announce"),
    bstr("https://tracker.example.org/announce.php?passkey=PK123"),
    bstr("announce-list"),
    l(
      l(bstr("https://tracker.example.org/announce.php?passkey=PK123")),
      l(bstr("udp://backup.example.net:6969/announce"), bstr("not a url")),
    ),
    bstr("creation date"),
    i(1763700000),
    bstr("info"),
    d(
      bstr("length"),
      i(42),
      bstr("name"),
      bstr("a.mkv"),
      bstr("piece length"),
      i(16384),
      bstr("pieces"),
      bstr(pieces),
    ),
  );
}

describe("parseTorrentAnnounces", () => {
  it("读出 announce 与 announce-list 里的地址", () => {
    expect(parseTorrentAnnounces(sampleTorrent())).toEqual([
      "https://tracker.example.org/announce.php?passkey=PK123",
      "https://tracker.example.org/announce.php?passkey=PK123",
      "udp://backup.example.net:6969/announce",
      "not a url",
    ]);
  });

  it("不是种子时为 null", () => {
    expect(parseTorrentAnnounces(enc.encode("<!DOCTYPE html><html></html>"))).toBeNull();
    expect(parseTorrentAnnounces(enc.encode("d8:announce"))).toBeNull();
    expect(parseTorrentAnnounces(new Uint8Array())).toBeNull();
  });

  it("没有 tracker 的种子回空列表", () => {
    expect(parseTorrentAnnounces(d(bstr("info"), d(bstr("length"), i(1))))).toEqual([]);
  });
});

describe("trackerHostsOf", () => {
  it("只要主机名，去重排序，解析不了的丢掉", () => {
    expect(trackerHostsOf(parseTorrentAnnounces(sampleTorrent()) ?? [])).toEqual([
      "backup.example.net",
      "tracker.example.org",
    ]);
  });
});
