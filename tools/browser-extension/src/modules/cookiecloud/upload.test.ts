import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CookieCloudError, parseServer } from "./client";
import { decrypt, encrypt, passphrase } from "./crypto";
import { mergeCookieData, parseCookieData, siteDomains } from "./payload";
import {
  type CookieCloudConfig,
  type CookieCloudState,
  testCookieCloud,
  uploadToCookieCloud,
} from "./upload";

/** 一个内存里的 CookieCloud 服务：/get/{uuid} 与 /update，记下收到的请求。 */
interface FakeServer {
  store: Map<string, { encrypted: string; crypto_type?: string }>;
  requests: Array<{ method: string; url: string; body?: unknown }>;
  status?: number;
  redirect?: boolean;
}

let server: FakeServer;
let jar: Record<string, chrome.cookies.Cookie[]>;

const CFG: CookieCloudConfig = {
  enabled: true,
  serverUrl: "https://cc.example/cookie/",
  uuid: "uuid-1",
  password: "secret-pass",
  cryptoType: "aes-128-cbc-fixed",
};
const KEY = passphrase(CFG.uuid, CFG.password);
const NOW = new Date("2026-10-14T00:00:00Z");

function cookie(name: string, value: string, domain: string): chrome.cookies.Cookie {
  return {
    name,
    value,
    domain,
    path: "/",
    secure: true,
    httpOnly: true,
    sameSite: "lax",
    session: false,
    hostOnly: false,
    storeId: "0",
    expirationDate: 1893456000,
  } as chrome.cookies.Cookie;
}

beforeEach(() => {
  server = { store: new Map(), requests: [] };
  jar = { "hdsky.me": [cookie("c_secure_uid", "1", ".hdsky.me"), cookie("c_secure_pass", "p", ".hdsky.me")] };
  vi.stubGlobal("chrome", {
    cookies: { getAll: vi.fn(async ({ domain }: { domain: string }) => jar[domain] ?? []) },
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string, init: RequestInit) => {
      const url = new URL(input);
      const body = init.body ? (JSON.parse(String(init.body)) as unknown) : undefined;
      server.requests.push({ method: init.method ?? "GET", url: url.toString(), body });
      if (server.redirect) {
        return new Response(null, { status: 302, headers: { Location: "https://elsewhere.example/" } });
      }
      if (server.status) {
        return new Response("boom", { status: server.status });
      }
      if (init.method === "POST" && url.pathname === "/cookie/update") {
        const b = body as { uuid: string; encrypted: string; crypto_type: string };
        server.store.set(b.uuid, { encrypted: b.encrypted, crypto_type: b.crypto_type });
        return new Response(JSON.stringify({ action: "done" }), { status: 200 });
      }
      const m = /^\/cookie\/get\/(.+)$/.exec(url.pathname);
      if (m) {
        const hit = server.store.get(decodeURIComponent(m[1]));
        return hit ? new Response(JSON.stringify(hit), { status: 200 }) : new Response("Not Found", { status: 404 });
      }
      return new Response("Not Found", { status: 404 });
    }),
  );
});

afterEach(() => {
  vi.unstubAllGlobals();
});

async function stored(): Promise<ReturnType<typeof parseCookieData>> {
  const hit = server.store.get(CFG.uuid);
  if (!hit) return null;
  return parseCookieData(await decrypt(hit.encrypted, KEY, hit.crypto_type));
}

describe("上传到 CookieCloud", () => {
  it("第一次上传：只放 PT 站点域名下的 Cookie，服务端解得开", async () => {
    jar["unrelated.example"] = [cookie("sid", "x", "unrelated.example")];
    const { result, state } = await uploadToCookieCloud(CFG, null, { force: false, now: () => NOW });
    expect(result).toEqual({ uploaded: true, domains: 1, cookies: 2 });
    expect(state.lastUploadAt).toBe(NOW.toISOString());
    expect(state.lastDigest).toMatch(/^[0-9a-f]{64}$/);
    const data = await stored();
    expect(Object.keys(data!.cookie_data)).toEqual(["hdsky.me"]);
    expect(data!.cookie_data["hdsky.me"].map((c) => c.name)).toEqual(["c_secure_uid", "c_secure_pass"]);
    expect(data!.update_time).toBe(NOW.toISOString());
    const update = server.requests.find((r) => r.method === "POST");
    expect(update!.url).toBe("https://cc.example/cookie/update");
    expect(JSON.stringify(update!.body)).not.toContain(CFG.password);
    expect(JSON.stringify(update!.body)).not.toContain("c_secure_pass");
    expect((update!.body as { crypto_type: string }).crypto_type).toBe("aes-128-cbc-fixed");
  });

  it("合并：别的域名与 local_storage_data 保留，自己的域名换成浏览器里现在的（没有了的删掉）", async () => {
    const before = {
      cookie_data: {
        "other.example": [cookie("keep", "1", "other.example")],
        "hdsky.me": [cookie("old", "1", ".hdsky.me")],
        "springsunday.net": [cookie("SPRINGID", "gone", "springsunday.net")],
      },
      local_storage_data: { "other.example": { k: "v" } },
    };
    server.store.set(CFG.uuid, { encrypted: await encrypt(JSON.stringify(before), KEY, "legacy"), crypto_type: "legacy" });
    await uploadToCookieCloud(CFG, null, { force: true, now: () => NOW });
    const data = await stored();
    expect(Object.keys(data!.cookie_data).sort()).toEqual(["hdsky.me", "other.example"]);
    expect(data!.cookie_data["hdsky.me"].map((c) => c.name)).toEqual(["c_secure_uid", "c_secure_pass"]);
    expect(data!.local_storage_data).toEqual({ "other.example": { k: "v" } });
    // 按设置里的加密方式重新加密
    expect(server.store.get(CFG.uuid)!.crypto_type).toBe("aes-128-cbc-fixed");
  });

  it("服务端已有的数据解不开：不上传，原来的数据不动", async () => {
    const other = await encrypt('{"cookie_data":{}}', passphrase(CFG.uuid, "别人的密码"), "aes-128-cbc-fixed");
    server.store.set(CFG.uuid, { encrypted: other, crypto_type: "aes-128-cbc-fixed" });
    await expect(uploadToCookieCloud(CFG, null, { force: true })).rejects.toMatchObject({ code: "decrypt" });
    expect(server.store.get(CFG.uuid)!.encrypted).toBe(other);
    expect(server.requests.some((r) => r.method === "POST")).toBe(false);
  });

  it("定时上传：Cookie 与目标都没变时不传；Cookie 变了、换了 UUID 或手动上传时照传", async () => {
    let state: CookieCloudState | null = null;
    ({ state } = await uploadToCookieCloud(CFG, state, { force: false }));
    const posts = (): number => server.requests.filter((r) => r.method === "POST").length;
    expect(posts()).toBe(1);

    const again = await uploadToCookieCloud(CFG, state, { force: false });
    expect(again.result.uploaded).toBe(false);
    expect(posts()).toBe(1);

    // 只有过期时间变了：不算变化
    jar["hdsky.me"] = jar["hdsky.me"].map((c) => ({ ...c, expirationDate: 1999999999 }));
    expect((await uploadToCookieCloud(CFG, state, { force: false })).result.uploaded).toBe(false);

    jar["hdsky.me"] = [cookie("c_secure_uid", "2", ".hdsky.me")];
    ({ state } = await uploadToCookieCloud(CFG, state, { force: false }));
    expect(posts()).toBe(2);

    await uploadToCookieCloud({ ...CFG, uuid: "uuid-2" }, state, { force: false });
    expect(posts()).toBe(3);

    await uploadToCookieCloud(CFG, state, { force: true });
    expect(posts()).toBe(4);
  });

  it("没填完整、地址不对、重定向、HTTP 出错时抛出能看懂的错误", async () => {
    await expect(uploadToCookieCloud({ ...CFG, password: "" }, null, { force: true })).rejects.toMatchObject({
      code: "not_configured",
    });
    await expect(uploadToCookieCloud({ ...CFG, serverUrl: "ftp://cc.example" }, null, { force: true })).rejects.toMatchObject({
      code: "invalid_server",
    });
    server.redirect = true;
    await expect(uploadToCookieCloud(CFG, null, { force: true })).rejects.toMatchObject({ code: "redirect" });
    server.redirect = false;
    server.status = 500;
    const err = await uploadToCookieCloud(CFG, null, { force: true }).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(CookieCloudError);
    expect((err as CookieCloudError).message).toContain("HTTP 500");
    expect((err as CookieCloudError).message).not.toContain(CFG.uuid);
  });
});

describe("测试 CookieCloud", () => {
  it("没有数据、解得开、解不开", async () => {
    expect(await testCookieCloud(CFG)).toEqual({ status: "empty", domains: 0, updatedAt: null });
    await uploadToCookieCloud(CFG, null, { force: true, now: () => NOW });
    expect(await testCookieCloud(CFG)).toEqual({ status: "ok", domains: 1, updatedAt: NOW.toISOString() });
    await expect(testCookieCloud({ ...CFG, password: "wrong" })).rejects.toMatchObject({ code: "decrypt" });
    // 测试不写东西：只有上面那次上传
    expect(server.requests.filter((r) => r.method === "POST")).toHaveLength(1);
  });
});

describe("工具函数", () => {
  it("服务地址：去掉末尾的 /，拒绝带查询参数、# 或用户名密码的", () => {
    expect(parseServer("https://cc.example/cookie/").toString()).toBe("https://cc.example/cookie");
    expect(parseServer("http://10.0.0.2:8088").toString()).toBe("http://10.0.0.2:8088/");
    for (const bad of ["cc.example", "https://cc.example/?a=1", "https://cc.example/#x", "https://u:p@cc.example", "javascript:alert(1)"]) {
      expect(() => parseServer(bad), bad).toThrow(CookieCloudError);
    }
  });

  it("要上传的域名来自内置站点，去重排好序", () => {
    const domains = siteDomains();
    expect(domains).toContain("hdsky.me");
    expect(new Set(domains).size).toBe(domains.length);
    expect([...domains].sort()).toEqual(domains);
  });

  it("合并不改动传进来的对象", () => {
    const existing = { cookie_data: { "a.example": [] }, local_storage_data: {} };
    const merged = mergeCookieData(existing, {}, ["a.example"], NOW);
    expect(merged.cookie_data).toEqual({});
    expect(existing.cookie_data).toEqual({ "a.example": [] });
  });

  it("解密出来不是 CookieCloud 的数据时不认", () => {
    expect(parseCookieData("[]")).toBeNull();
    expect(parseCookieData('{"cookie_data":[]}')).toBeNull();
    expect(parseCookieData("nope")).toBeNull();
    expect(parseCookieData("{}")).toEqual({ cookie_data: {} });
  });
});
