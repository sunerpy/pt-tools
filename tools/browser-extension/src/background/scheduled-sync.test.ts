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
  problemsOf,
  pushFingerprint,
  runHadProblems,
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
  /** 每个推出去的站点带没带 keep_enabled */
  keepEnabled: Record<string, boolean>;
  hashes: Record<string, string>;
  ccState: CookieCloudState | null;
}

function harness(opts: {
  schedule?: Partial<SyncSchedule>;
  autoSync?: Record<string, boolean>;
  enabled?: string[] | Error;
  /** pt-tools 列出的停用站点；默认是 SITES 里没启用的全部（pt-tools 会列出全部内置站点） */
  disabled?: string[];
  cookies?: Record<string, string>;
  pushError?: Record<string, Error>;
  connected?: boolean;
  baseUrl?: string;
  cc?: CookieCloudConfig;
  upload?: ScheduledSyncDeps["upload"];
  /** pt-tools 认不认 keep_enabled（rc.38 起认）；默认认 */
  keepEnabledSupported?: boolean;
}): Harness {
  const h: Harness = {
    deps: undefined as unknown as ScheduledSyncDeps,
    pushed: [],
    keepEnabled: {},
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
    ptToolsSites: async () => {
      if (opts.enabled instanceof Error) throw opts.enabled;
      const enabled = (opts.enabled ?? []).map((name) => name.toLowerCase());
      const disabled =
        opts.disabled ?? SITES.map((s) => s.id).filter((id) => !enabled.includes(id));
      return [
        ...enabled.map((name) => ({ name, enabled: true })),
        ...disabled.map((name) => ({ name, enabled: false })),
      ];
    },
    keepEnabledSupported: async () => opts.keepEnabledSupported !== false,
    readCookie: async (s) => opts.cookies?.[s.id] ?? "",
    pushCookie: async (_base, s, _cookie, pushOpts) => {
      const err = opts.pushError?.[s.id];
      if (err) throw err;
      h.pushed.push(s.id);
      h.keepEnabled[s.id] = pushOpts.keepEnabled;
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
        (p) => p.site.id,
      ),
    ).toEqual(["hdsky", "hdtime"]);
    expect(
      pickSites(SITES, { ...schedule, allSites: false }, { hdtime: true }, ["hdsky"]).map(
        (p) => p.site.id,
      ),
    ).toEqual(["hdtime"]);
  });

  it("给了停用列表时「全部站点」也带上 pt-tools 里停用的 Cookie 站点，标成不是必须的", () => {
    const schedule = normalizeSchedule({ enabled: true, allSites: true });
    expect(
      pickSites(SITES, schedule, { ourbits: true }, ["hdsky"], ["HDTIME", "ourbits", "mteam"]),
    ).toEqual([
      { site: SITES[0], required: true },
      { site: SITES[1], required: false },
      { site: SITES[2], required: true },
    ]);
    // pt-tools 列表里没有的站点（扩展比 pt-tools 新）不带
    expect(pickSites(SITES, schedule, {}, ["hdsky"], ["ourbits"])).toEqual([
      { site: SITES[0], required: true },
      { site: SITES[2], required: false },
    ]);
    // 读不到站点列表时不知道哪些停用：不带
    expect(pickSites(SITES, schedule, {}, null, null)).toEqual([]);
    // 没勾「全部站点」：只有打开了自动同步的
    expect(
      pickSites(SITES, { ...schedule, allSites: false }, { hdtime: true }, ["hdsky"], ["ourbits"]),
    ).toEqual([{ site: SITES[1], required: true }]);
  });
});

describe("runSync", () => {
  it("只推 Cookie 有变化的站点；浏览器里没有可用 Cookie 的站点不推，按站点名记在 missing 里", async () => {
    const h = harness({
      schedule: { allSites: true },
      enabled: ["hdsky", "hdtime", "ourbits"],
      cookies: { hdsky: "a=1", hdtime: "b=2" },
    });
    h.hashes.hdtime = await pushFingerprint("http://pt.example", "b=2");
    const run = await runSync(h.deps, "schedule");
    expect(h.pushed).toEqual(["hdsky"]);
    expect(run.ptTools).toEqual({
      synced: ["hdsky"],
      unchanged: 1,
      failed: [],
      missing: ["OURBITS"],
    });
    expect(problemsOf(run)).toContain("OURBITS");
    expect(h.hashes.hdsky).toBe(await pushFingerprint("http://pt.example/", "a=1"));
    expect(run.cookieCloud.skipped).toBeTruthy();

    // 第二轮什么都没变：不推
    const again = await runSync(h.deps, "schedule");
    expect(h.pushed).toEqual(["hdsky"]);
    expect(again.ptTools.unchanged).toBe(2);
    expect(again.ptTools.missing).toEqual(["OURBITS"]);
  });

  it("勾选了的站点都没有 Cookie 时也有结果：不是 0/0/0 的「成功」", async () => {
    const h = harness({ autoSync: { hdsky: true, hdtime: true }, cookies: {} });
    const run = await runSync(h.deps, "manual");
    expect(h.pushed).toEqual([]);
    expect(run.ptTools.synced).toEqual([]);
    expect(run.ptTools.failed).toEqual([]);
    expect(run.ptTools.missing).toEqual(["HDSKY", "HDTIME"]);
    const problems = problemsOf(run);
    expect(problems).toContain("HDSKY");
    expect(problems).toContain("HDTIME");
    // 「立即同步一次」不提示成功
    expect(runHadProblems(run)).toBe(true);
  });

  it("pt-tools 这一轮没有同步（登录过期、没连上、没有要同步的站点）也算问题：定时的会发通知", async () => {
    const expired = harness({ schedule: { allSites: true }, enabled: new AuthExpiredError() });
    const run = await runSync(expired.deps, "schedule");
    expect(run.ptTools.skipped).toBeTruthy();
    expect(problemsOf(run)).toContain(run.ptTools.skipped as string);

    const off = harness({ connected: false, autoSync: { hdsky: true } });
    expect(problemsOf(await runSync(off.deps, "schedule"))).not.toBe("");

    const none = harness({});
    expect(problemsOf(await runSync(none.deps, "schedule"))).not.toBe("");
  });

  it("runHadProblems：全部顺利时才算成功；没连上、没有要同步的站点也不算", async () => {
    const ok = harness({ autoSync: { hdsky: true }, cookies: { hdsky: "a=1" } });
    expect(runHadProblems(await runSync(ok.deps, "manual"))).toBe(false);
    // 再跑一次没有变化，也算顺利
    expect(runHadProblems(await runSync(ok.deps, "manual"))).toBe(false);

    const off = harness({ connected: false, autoSync: { hdsky: true }, cookies: { hdsky: "a=1" } });
    expect(runHadProblems(await runSync(off.deps, "manual"))).toBe(true);

    const none = harness({});
    const run = await runSync(none.deps, "manual");
    expect(run.ptTools.skipped).toBeTruthy();
    expect(runHadProblems(run)).toBe(true);

    const failed = harness({
      autoSync: { hdsky: true },
      cookies: { hdsky: "a=1" },
      pushError: { hdsky: new Error("HTTP 500") },
    });
    expect(runHadProblems(await runSync(failed.deps, "manual"))).toBe(true);
    expect(runHadProblems(null)).toBe(false);
  });

  it("没连上 pt-tools 时跳过；pt-tools 不认 keep_enabled（旧版本）时「全部站点」不推没启用的站点", async () => {
    const off = harness({ connected: false, autoSync: { hdsky: true }, cookies: { hdsky: "a=1" } });
    const run = await runSync(off.deps, "schedule");
    expect(run.ptTools.skipped).toBeTruthy();
    expect(off.pushed).toEqual([]);

    // 旧版 pt-tools 收到凭据就把站点启用：不能推没启用的站点
    const h = harness({
      schedule: { allSites: true },
      enabled: ["hdtime"],
      cookies: { hdsky: "a=1", hdtime: "b=2" },
      keepEnabledSupported: false,
    });
    await runSync(h.deps, "schedule");
    expect(h.pushed).toEqual(["hdtime"]);
    expect(h.keepEnabled.hdtime).toBe(false);
  });

  it("「全部站点」：pt-tools 里没启用、浏览器里登录着的站点也推，带 keep_enabled；没启用又没 Cookie 的不算缺", async () => {
    // 以前只推 pt-tools 里已启用的站点，而 pt-tools 启用 Cookie 站点要先有 Cookie：新站点两头都等着对方
    const h = harness({
      schedule: { allSites: true },
      enabled: ["hdtime"],
      cookies: { hdsky: "a=1", hdtime: "b=2" },
    });
    const run = await runSync(h.deps, "schedule");
    expect(h.pushed).toEqual(["hdsky", "hdtime"]);
    expect(h.keepEnabled).toEqual({ hdsky: true, hdtime: false });
    expect(run.ptTools.synced).toEqual(["hdsky", "hdtime"]);
    // ourbits 没启用、浏览器里也没有 Cookie：不推，也不列成缺失
    expect(run.ptTools.missing).toEqual([]);
    expect(runHadProblems(run)).toBe(false);

    // 推过的没启用站点下一轮 Cookie 没变：不再推
    const again = await runSync(h.deps, "schedule");
    expect(h.pushed).toEqual(["hdsky", "hdtime"]);
    expect(again.ptTools.unchanged).toBe(2);

    // 已启用的站点照旧：没有 Cookie 时列成缺失
    const missing = harness({ schedule: { allSites: true }, enabled: ["hdtime"], cookies: {} });
    expect((await runSync(missing.deps, "schedule")).ptTools.missing).toEqual(["HDTIME"]);

    // pt-tools 站点列表里没有的站点（扩展比 pt-tools 新）：有 Cookie 也不推，不然每轮都推失败
    const newer = harness({
      schedule: { allSites: true },
      enabled: [],
      disabled: ["hdsky"],
      cookies: { hdsky: "a=1", ourbits: "c=3" },
    });
    const newerRun = await runSync(newer.deps, "schedule");
    expect(newer.pushed).toEqual(["hdsky"]);
    expect(newerRun.ptTools.failed).toEqual([]);
  });

  it("只挑到没启用、浏览器里又都没有 Cookie 的站点：写明没有要同步的站点，不当成一次成功", async () => {
    const h = harness({ schedule: { allSites: true }, enabled: [], cookies: {} });
    const run = await runSync(h.deps, "manual");
    expect(h.pushed).toEqual([]);
    expect(run.ptTools.skipped).toBeTruthy();
    expect(runHadProblems(run)).toBe(true);
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

  it("problemsOf：全部顺利时为空；推失败、没有 Cookie、CookieCloud 出错各占一行；旧结果没有 missing 也能读", async () => {
    const ok = harness({ autoSync: { hdsky: true }, cookies: { hdsky: "a=1" } });
    expect(problemsOf(await runSync(ok.deps, "schedule"))).toBe("");
    expect(problemsOf(null)).toBe("");

    const problems = problemsOf({
      at: "2026-10-14T00:00:00.000Z",
      trigger: "schedule",
      ptTools: {
        synced: [],
        unchanged: 0,
        failed: [{ siteId: "hdsky", error: "HTTP 500" }],
        missing: ["AGSVPT", "XingYunGe"],
      },
      cookieCloud: { uploaded: false, domains: 0, cookies: 0, error: "连不上" },
    }).split("\n");
    expect(problems).toHaveLength(3);
    expect(problems[0]).toContain("HTTP 500");
    expect(problems[1]).toContain("AGSVPT, XingYunGe");
    expect(problems[2]).toContain("连不上");

    // 这一版以前存下的结果没有 missing
    expect(
      problemsOf({
        at: "2026-10-14T00:00:00.000Z",
        trigger: "schedule",
        ptTools: { synced: ["hdsky"], unchanged: 0, failed: [] },
        cookieCloud: { uploaded: false, domains: 0, cookies: 0 },
      }),
    ).toBe("");
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
