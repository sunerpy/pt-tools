import { describe, expect, it } from "vitest";

import { DecryptError, decrypt, encrypt, evpBytesToKey, passphrase } from "./crypto";
import { md5Hex, toHex } from "./md5";

/*
 * 向量用 OpenSSL 生成（见 internal/cookiecloud 的 TestDecryptExtensionVectors，Go 那边解同一份）：
 *   key=$(md5(uuid-password) 的前 16 个字符)
 *   fixed:  openssl enc -aes-128-cbc -K <key 的十六进制> -iv 0…0 -base64 -A
 *   legacy: openssl enc -aes-256-cbc -md md5 -S 0102030405060708 -pass pass:<key>，前面补上 "Salted__" + salt
 */
const UUID = "test-uuid-1234";
const PASSWORD = "p@ss 中文";
const KEY = "3fb1a74ab03ea4a0";
const PLAIN =
  '{"cookie_data":{"hdsky.me":[{"name":"c_secure_uid","value":"1","domain":".hdsky.me","path":"/"}]},"local_storage_data":{}}';
const FIXED =
  "1Tm4pmaoEMIDtPcdOeKcSbNZZ/2r54DoKPlK4wl+1uQVcTEg479NKYZPN7GOo0J05ShoXe5iNP3h5RQWmXS6LjvhTRG/cSZsXyteNDn0+GTUIKrvMv2v5oSO6oJ2yjr4GzDCArorHJvD2wghOaJbZhiZi+wbR+PSZMIm3BEQ54c=";
const LEGACY =
  "U2FsdGVkX18BAgMEBQYHCCCzhyNw3DRq1A/8o3URw4n8gio0ile1cZMZoUAwxpyXNNm4yAIeJbc7ZXSBYxI1VJXIVNBjqkR+UOXi+Llol7oxiN0Rdudu1kXfW/y2HrrZTkUCpzpGGucz1MqMqu0hd8q26bCE1f22Qvv8fVp+eYktkSQeb7xpqYfAjHKMCckZ";

describe("md5", () => {
  it("和 RFC 1321 的向量一致", () => {
    expect(md5Hex("")).toBe("d41d8cd98f00b204e9800998ecf8427e");
    expect(md5Hex("abc")).toBe("900150983cd24fb0d6963f7d28e17f72");
    expect(md5Hex("The quick brown fox jumps over the lazy dog")).toBe(
      "9e107d9d372bb6826bd81d3542a419d6",
    );
    expect(md5Hex("1234567890".repeat(8))).toBe("57edf4a22be3c955ac49da2e2107b67a");
  });

  it("按 UTF-8 算中文", () => {
    expect(md5Hex("中文")).toBe("a7bac2239fcdcb3a067903d8077c4a07");
  });
});

describe("CookieCloud 加密", () => {
  it("口令是 md5(uuid-password) 的前 16 个字符，UUID 去掉首尾空白", () => {
    expect(passphrase(UUID, PASSWORD)).toBe(KEY);
    expect(passphrase(` ${UUID} `, PASSWORD)).toBe(KEY);
  });

  it("aes-128-cbc-fixed 和 OpenSSL 的结果逐字节相同，也解得开", async () => {
    expect(await encrypt(PLAIN, KEY, "aes-128-cbc-fixed")).toBe(FIXED);
    expect(await decrypt(FIXED, KEY, "aes-128-cbc-fixed")).toBe(PLAIN);
  });

  it("legacy 用给定的 salt 时和 OpenSSL 的结果相同；默认每次随机 salt，照样解得开", async () => {
    const salt = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]);
    expect(await encrypt(PLAIN, KEY, "legacy", salt)).toBe(LEGACY);
    expect(await decrypt(LEGACY, KEY, "legacy")).toBe(PLAIN);
    const a = await encrypt(PLAIN, KEY, "legacy");
    const b = await encrypt(PLAIN, KEY, "legacy");
    expect(a).not.toBe(b);
    expect(await decrypt(a, KEY, "legacy")).toBe(PLAIN);
  });

  it("没写加密方式（老服务端）时按格式判断", async () => {
    expect(await decrypt(LEGACY, KEY)).toBe(PLAIN);
    expect(await decrypt(FIXED, KEY)).toBe(PLAIN);
  });

  it("密码不对、格式不对、不认识的加密方式都抛 DecryptError", async () => {
    const wrong = passphrase(UUID, "wrong");
    await expect(decrypt(FIXED, wrong, "aes-128-cbc-fixed")).rejects.toBeInstanceOf(DecryptError);
    await expect(decrypt(LEGACY, wrong, "legacy")).rejects.toBeInstanceOf(DecryptError);
    await expect(decrypt("不是 base64", KEY)).rejects.toBeInstanceOf(DecryptError);
    await expect(decrypt(FIXED, KEY, "legacy")).rejects.toBeInstanceOf(DecryptError);
    await expect(decrypt(FIXED, KEY, "rot13")).rejects.toBeInstanceOf(DecryptError);
  });

  it("EVP_BytesToKey 和 OpenSSL 的 -md md5 一致", () => {
    const { key, iv } = evpBytesToKey(
      new TextEncoder().encode(KEY),
      new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]),
      32,
      16,
    );
    // openssl enc -aes-256-cbc -md md5 -S 0102030405060708 -pass pass:3fb1a74ab03ea4a0 -P
    expect(toHex(key)).toBe("d882bcceb2b14db20608939043f620e162e75f52731595b3aad8093f9c2a2cf8");
    expect(toHex(iv)).toBe("814a3be58594461d17cf23d426ed2b10");
  });
});
