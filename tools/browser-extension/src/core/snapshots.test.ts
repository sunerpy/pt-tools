import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { SiteLoginStateRecord } from "../modules/sync/api-client";
import { KNOWN_SITES, STORAGE_KEYS } from "./constants";
import {
  loadLoginSnapshot,
  loadSitesSnapshot,
  saveLoginSnapshot,
  saveSitesSnapshot,
} from "./snapshots";
import type { KnownSiteStatus } from "./types";

let area: Record<string, unknown> = {};

beforeEach(() => {
  area = {};
  vi.stubGlobal("chrome", {
    storage: {
      local: {
        get: vi.fn(async (key: string) =>
          key in area ? { [key]: structuredClone(area[key]) } : {},
        ),
        set: vi.fn(async (items: Record<string, unknown>) => {
          Object.assign(area, structuredClone(items));
        }),
        remove: vi.fn(async (key: string) => {
          delete area[key];
        }),
      },
    },
  });
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function statusOf(index: number, cookieStatus: KnownSiteStatus["cookieStatus"]): KnownSiteStatus {
  return {
    site: KNOWN_SITES[index]!,
    cookieStatus,
    cookieExpireDays: cookieStatus === "valid" ? 30 : null,
    hasCookie: true,
    lastSync: null,
    autoSync: false,
  };
}

describe("站点状态快照", () => {
  it("存下以后按现在的站点定义还原，带着读到的时间", async () => {
    const at = new Date("2026-10-24T12:00:00Z");
    await saveSitesSnapshot([statusOf(0, "valid"), statusOf(1, "unverified")], at);

    const got = await loadSitesSnapshot();

    expect(got?.at).toBe(at.toISOString());
    expect(got?.data.map((s) => s.site.id)).toEqual([KNOWN_SITES[0]!.id, KNOWN_SITES[1]!.id]);
    expect(got?.data[1]).toMatchObject({ cookieStatus: "unverified", hasCookie: true });
    expect(got?.data[0]!.site, "站点定义用扩展现在的，不用存下的").toBe(KNOWN_SITES[0]);
  });

  it("扩展里没有了的站点丢掉；什么都没存时为空", async () => {
    expect(await loadSitesSnapshot()).toBeNull();
    area[STORAGE_KEYS.sitesSnapshot] = {
      at: "2026-10-24T12:00:00.000Z",
      sites: {
        "gone-site": {
          cookieStatus: "valid",
          cookieExpireDays: 1,
          hasCookie: true,
          lastSync: null,
          autoSync: false,
        },
      },
    };
    expect(await loadSitesSnapshot()).toBeNull();
  });
});

describe("登录状态快照", () => {
  const records = [{ site_name: "hdsky", enabled: true, tier: "none" }] as SiteLoginStateRecord[];

  it("只对同一个 pt-tools 地址有效（末尾斜杠不算不同）", async () => {
    await saveLoginSnapshot("http://pt.example:18080", records, new Date("2026-10-24T12:00:00Z"));

    expect((await loadLoginSnapshot("http://pt.example:18080/"))?.data).toEqual(records);
    expect(await loadLoginSnapshot("http://other.example:18080")).toBeNull();
    expect(await loadLoginSnapshot("")).toBeNull();
  });
});
