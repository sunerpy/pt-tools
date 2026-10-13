import { describe, expect, it, vi } from "vitest";

import type { KnownSite } from "../core/constants";
import type { SyncSchedule } from "../core/types";
import type { CookieCloudConfig, CookieCloudState } from "../modules/cookiecloud/upload";
import {
  AuthExpiredError,
  type ScheduledSyncDeps,
  createLock,
  normalizeSchedule,
  pickSites,
  pushFingerprint,
  runSync,
} from "./scheduled-sync";

function site(id: string, syncField: KnownSite["syncField"] = "cookie"): KnownSite {
  return {
    id,
    name: id.toUpperCase(),
    domains: [`${id}.example`],
    schema: "NexusPHP",
    authMethod: syncField === "cookie" ? "cookie" : "api_key",
    cookieNames: [],
    syncField,
  };
}

const SITES = [site("hdsky"), site("hdtime"), site("ourbits"), site("mteam", "api_key")];
const CC_OFF: CookieCloudConfig = {
  enabled: false,
  serverUrl: "",
  uuid: "",
  password: "",
  cryptoType: "aes-128-cbc-fixed",
};

interface Harness {
  deps: ScheduledSyncDeps;
  pushed: string[];
  hashes: Record<string, string>;
  ccState: CookieCloudState | null;
}

function harness(opts: {
  schedule?: Partial<SyncSchedule>;
  autoSync?: Record<string, boolean>;
  enabled?: string[] | Error;
  cookies?: Record<string, string>;
  pushError?: Record<string, Error>;
  connected?: boolean;
  baseUrl?: string;
  cc?: CookieCloudConfig;
  upload?: ScheduledSyncDeps["upload"];
}): Harness {
  const h: Harness = {
    deps: undefined as unknown as ScheduledSyncDeps,
    pushed: [],
    hashes: {},
    ccState: null,
  };
  h.deps = {
    sites: SITES,
    getSchedule: async () =>
      normalizeSchedule({ enabled: true, intervalMinutes: 60, ...opts.schedule }),
    getAutoSyncMap: async () => opts.autoSync ?? {},
    getHash: async (siteId) => h.hashes[siteId],
    withPushLock: createLock(),
    ptTools: async () => (opts.connected === false ? null : (opts.baseUrl ?? "http://pt.example")),
    enabledSites: async () => {
      if (opts.enabled instanceof Error) throw opts.enabled;
      return opts.enabled ?? [];
    },
    readCookie: async (s) => opts.cookies?.[s.id] ?? "",
    pushCookie: async (_base, s) => {
      const err = opts.pushError?.[s.id];
      if (err) throw err;
      h.pushed.push(s.id);
    },
    recordPushed: async (siteId, hash) => {
      h.hashes[siteId] = hash;
    },
    authExpired: vi.fn(async () => {}),
    getCookieCloudConfig: async () => opts.cc ?? CC_OFF,
    getCookieCloudState: async () => h.ccState,
    saveCookieCloudState: async (state) => {
      h.ccState = state;
    },
    upload:
      opts.upload ??
      (async () => {
        throw new Error("upload should not run");
      }),
    errorText: (e) => (e instanceof Error ? e.message : String(e)),
    now: () => new Date("2026-10-14T00:00:00Z"),
  };
  return h;
}

describe("normalizeSchedule", () => {
  it("间隔按分钟取整并夹在 10–1440 里，别的字段只认 true", () => {
    expect(normalizeSchedule({ enabled: true, intervalMinutes: 3, allSites: "yes" })).toEqual({
      enabled: true,
      intervalMinutes: 10,
      allSites: false,
    });
    expect(normalizeSchedule({ intervalMinutes: 99999 }).intervalMinutes).toBe(1440);
    expect(normalizeSchedule({ intervalMinutes: 61.6 }).intervalMinutes).toBe(62);
    expect(normalizeSchedule(null)).toEqual({
      enabled: false,
      intervalMinutes: 60,
      allSites: false,
    });
  });
});

describe("pickSites", () => {
  it("只挑用 Cookie 登录的站点：打开了自动同步的，以及「全部站点」时 pt-tools 里已启用的", () => {
    const schedule = normalizeSchedule({ enabled: true, allSites: true });
    expect(
      pickSites(SITES, schedule, { hdtime: true, mteam: true }, ["HDSKY", "mteam"]).map(
        (s) => s.id,
      ),
    ).toEqual(["hdsky", "hdtime"]);
    expect(
      pickSites(SITES, { ...schedule, allSites: false }, { hdtime: true }, ["hdsky"]).map(
        (s) => s.id,
      ),
    ).toEqual(["hdtime"]);
  });
});

describe("runSync", () => {
  it("只推 Cookie 有变化的站点；没有 Cookie 的不算失败", async () => {
    const h = harness({
      schedule: { allSites: true },
      enabled: ["hdsky", "hdtime", "ourbits"],
      cookies: { hdsky: "a=1", hdtime: "b=2" },
    });
    h.hashes.hdtime = await pushFingerprint("http://pt.example", "b=2");
    const run = await runSync(h.deps, "schedule");
    expect(h.pushed).toEqual(["hdsky"]);
    expect(run.ptTools).toEqual({ synced: ["hdsky"], unchanged: 1, failed: [] });
    expect(h.hashes.hdsky).toBe(await pushFingerprint("http://pt.example/", "a=1"));
    expect(run.cookieCloud.skipped).toBeTruthy();

    // 第二轮什么都没变：不推
    const again = await runSync(h.deps, "schedule");
    expect(h.pushed).toEqual(["hdsky"]);
    expect(again.ptTools.unchanged).toBe(2);
  });

  it("没连上 pt-tools 时跳过；「全部站点」不会推 pt-tools 里没启用的站点", async () => {
    const off = harness({ connected: false, autoSync: { hdsky: true }, cookies: { hdsky: "a=1" } });
    const run = await runSync(off.deps, "schedule");
    expect(run.ptTools.skipped).toBeTruthy();
    expect(off.pushed).toEqual([]);

    const h = harness({
      schedule: { allSites: true },
      enabled: ["hdtime"],
      cookies: { hdsky: "a=1", hdtime: "b=2" },
    });
    await runSync(h.deps, "schedule");
    expect(h.pushed).toEqual(["hdtime"]);
  });

  it("读站点列表失败时只同步单独打开了自动同步的；登录过期时停下并把连接标成断开", async () => {
    const h = harness({
      schedule: { allSites: true },
      enabled: new Error("boom"),
      autoSync: { ourbits: true },
      cookies: { hdsky: "a=1", ourbits: "c=3" },
    });
    await runSync(h.deps, "schedule");
    expect(h.pushed).toEqual(["ourbits"]);

    const expired = harness({ schedule: { allSites: true }, enabled: new AuthExpiredError() });
    const run = await runSync(expired.deps, "schedule");
    expect(run.ptTools.skipped).toBeTruthy();
    expect(expired.deps.authExpired).toHaveBeenCalledWith("http://pt.example");

    const midway = harness({
      autoSync: { hdsky: true, hdtime: true },
      cookies: { hdsky: "a=1", hdtime: "b=2" },
      pushError: { hdsky: new AuthExpiredError() },
    });
    const r = await runSync(midway.deps, "schedule");
    expect(midway.pushed).toEqual([]);
    expect(r.ptTools.failed).toHaveLength(1);
    expect(midway.deps.authExpired).toHaveBeenCalled();
  });

  it("一个站点推失败不影响别的，失败的指纹不记（下一轮重试）", async () => {
    const h = harness({
      autoSync: { hdsky: true, hdtime: true },
      cookies: { hdsky: "a=1", hdtime: "b=2" },
      pushError: { hdsky: new Error("HTTP 500") },
    });
    const run = await runSync(h.deps, "schedule");
    expect(run.ptTools.failed).toEqual([{ siteId: "hdsky", error: "HTTP 500" }]);
    expect(run.ptTools.synced).toEqual(["hdtime"]);
    expect(h.hashes.hdsky).toBeUndefined();
  });

  it("CookieCloud：打开了才上传；出错时记进状态，不影响 pt-tools", async () => {
    const cc = {
      ...CC_OFF,
      enabled: true,
      serverUrl: "https://cc.example",
      uuid: "u",
      password: "p",
    };
    const ok = harness({
      cc,
      upload: async (_cfg, _state, opts) => {
        expect(opts.force).toBe(false);
        return {
          result: { uploaded: true, domains: 2, cookies: 5 },
          state: {
            lastUploadAt: "t",
            lastDigest: "d",
            lastTarget: "x",
            domains: 2,
            cookies: 5,
            lastError: null,
            lastErrorAt: null,
          },
        };
      },
    });
    const run = await runSync(ok.deps, "schedule");
    expect(run.cookieCloud).toEqual({ uploaded: true, domains: 2, cookies: 5 });
    expect(ok.ccState?.lastDigest).toBe("d");

    const bad = harness({
      cc,
      autoSync: { hdsky: true },
      cookies: { hdsky: "a=1" },
      upload: async () => {
        throw new Error("连不上");
      },
    });
    const r = await runSync(bad.deps, "schedule");
    expect(r.cookieCloud.error).toBe("连不上");
    expect(bad.ccState?.lastError).toBe("连不上");
    expect(bad.ccState?.lastErrorAt).toBe("2026-10-14T00:00:00.000Z");
    expect(bad.pushed).toEqual(["hdsky"]);
  });

  it("指纹带着 pt-tools 的地址：换了实例以后 Cookie 没变也会重新推", async () => {
    const h = harness({ autoSync: { hdsky: true }, cookies: { hdsky: "a=1" } });
    await runSync(h.deps, "schedule");
    expect(h.pushed).toEqual(["hdsky"]);
    await runSync(h.deps, "schedule");
    expect(h.pushed).toEqual(["hdsky"]);
    const other = harness({
      autoSync: { hdsky: true },
      cookies: { hdsky: "a=1" },
      baseUrl: "http://other.example",
    });
    other.hashes = { ...h.hashes };
    await runSync(other.deps, "schedule");
    expect(other.pushed).toEqual(["hdsky"]);
  });

  it("读不到 pt-tools 的站点列表时写明原因，只同步单独打开了自动同步的", async () => {
    const h = harness({
      schedule: { allSites: true },
      enabled: new Error("HTTP 500"),
      cookies: { hdsky: "a=1" },
    });
    const run = await runSync(h.deps, "schedule");
    expect(run.ptTools.error).toContain("HTTP 500");
    expect(run.ptTools.skipped).toBeUndefined();
    expect(h.pushed).toEqual([]);
  });

  it("每个站点的读 Cookie、比指纹、推送在同一把锁里：锁里有别的推送时等它做完再读", async () => {
    const lock = createLock();
    let cookie = "a=1";
    const h = harness({ autoSync: { hdsky: true }, cookies: {} });
    h.deps.withPushLock = lock;
    h.deps.readCookie = async () => cookie;
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    let entered!: () => void;
    const started = new Promise<void>((resolve) => {
      entered = resolve;
    });
    const manual = lock(async () => {
      entered();
      await gate;
      // 手动同步在锁里先把 Cookie 换成 b 并推了出去
      cookie = "b=2";
      h.hashes.hdsky = await pushFingerprint("http://pt.example", "b=2");
    });
    await started;
    const scheduled = runSync(h.deps, "schedule");
    release();
    await manual;
    const run = await scheduled;
    // 定时同步等手动的做完才读：读到的是 b，和刚推的一样，不再推旧的 a
    expect(run.ptTools.unchanged).toBe(1);
    expect(h.pushed).toEqual([]);
  });
});
