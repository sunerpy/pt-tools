/*
 * 钉子：NAS 监控的文字与颜色（首页 NAS 概览 H2）。更新时间、过时与重试的说法按秒数分档；百分比与运行时长的写法；
 * 读不到的那类数据写原因、不当成 0；存储合计只算知道容量的池（段宽最少 5%）；80% / 90% 与 50 / 70 °C 两条线；
 * 告警按级别计数、最新的在前；主机行最多两个提醒；首页显示的选项。
 */
import { describe, expect, it } from "vitest";

import type { NasAlert, NasDisk, NasPool, NasSnapshot } from "@/api";
import {
  nasAlertCounts,
  nasAlertTone,
  nasAppsText,
  nasCpuKpi,
  nasDiskSub,
  nasDiskTempTone,
  nasFacetReason,
  nasHomeOptions,
  nasHostFacts,
  nasHottestDisk,
  nasIfaceText,
  nasKindGuide,
  nasKindIcon,
  nasKindLabel,
  nasLatestAlerts,
  nasLocalDiskSummary,
  nasMemKpi,
  nasPctText,
  nasPoolHealth,
  nasPoolsAside,
  nasPoolSub,
  nasPoolSummary,
  nasProblem,
  nasProblemNote,
  nasRetryText,
  nasSourceSummary,
  nasStaleText,
  nasStateView,
  nasTempGauge,
  nasUpdatedText,
  nasUptimeText,
  nasUsable,
  nasUsedTone,
} from "./nas";

const NOW = new Date("2026-11-06T12:00:00+08:00");
const ago = (sec: number) => new Date(NOW.getTime() - sec * 1000).toISOString();
const GB = 1024 ** 3;
const TB = 1024 ** 4;

function snap(over: Partial<NasSnapshot> = {}): NasSnapshot {
  return {
    at: ago(5),
    system: {
      hostname: "truenas",
      platform: "TrueNAS SCALE",
      version: "25.04.1",
      model: "i7-12700",
      cores: 8,
      uptime_seconds: 60,
    },
    cpu: { percent: 3.8, iowait: 12, load1: 3.71, load5: 3, load15: 2 },
    memory: { total: 64 * GB, used: 40 * GB, available: 24 * GB },
    network: { rx_bps: 0, tx_bps: 0, interfaces: ["ens161np1"] },
    pools: [],
    disks: [],
    alerts: [],
    apps: { running: 1, total: 3, upgradable: 0 },
    facets: null,
    ...over,
  };
}

function pool(name: string, size: number, used: number, over: Partial<NasPool> = {}): NasPool {
  return { name, status: "", size, used, free: size - used, disks: 0, ...over };
}

describe("来源的类型与状态", () => {
  it("类型名先用服务端的，再用内置的，都没有时原样；图标按类型", () => {
    expect(nasKindLabel("truenas", [{ name: "truenas", label: "TrueNAS Scale" } as never])).toBe(
      "TrueNAS Scale",
    );
    expect(nasKindLabel("fnos")).toBe("飞牛");
    expect(nasKindLabel("unraid")).toBe("unraid");
    expect(nasKindIcon("local")).toBe("monitor");
    expect(nasKindIcon("glances")).toBe("gauge");
    expect(nasKindIcon("synology")).toBe("hard-drive");
    expect(nasKindIcon("truenas")).toBe("server");
  });

  it("状态的文字与颜色；连不上、登录失败与证书的问题算有问题；启用了且能用的才能显示", () => {
    expect(nasStateView("cert_changed")).toEqual({ label: "证书换了", tone: "dang" });
    expect(nasStateView("weird")).toEqual({ label: "weird", tone: "neutral" });
    expect(["error", "auth_failed", "cert_untrusted", "cert_changed"].every(nasProblem)).toBe(true);
    expect(nasProblem("connecting")).toBe(false);
    expect(nasUsable({ enabled: true, state: "error" })).toBe(true);
    expect(nasUsable({ enabled: false, state: "disabled" })).toBe(false);
    expect(nasUsable({ enabled: true, state: "unsupported" })).toBe(false);
  });
});

describe("时间的说法", () => {
  it("更新时间：15 秒内刚刚，再按秒、分钟、小时，一天以上写日期", () => {
    expect(nasUpdatedText(undefined, NOW)).toBe("");
    expect(nasUpdatedText("bad", NOW)).toBe("");
    expect(nasUpdatedText(ago(14), NOW)).toBe("刚刚更新");
    expect(nasUpdatedText(ago(40), NOW)).toBe("40 秒前更新");
    expect(nasUpdatedText(ago(180), NOW)).toBe("3 分钟前更新");
    expect(nasUpdatedText(ago(7200), NOW)).toBe("2 小时前更新");
    expect(nasUpdatedText(ago(3 * 86400), NOW)).toMatch(/^\d{2}-\d{2} \d{2}:\d{2} 更新$/);
  });

  it("过时的数据：不到 1 分钟、N 分钟、N 小时、日期", () => {
    expect(nasStaleText(undefined, NOW)).toBe("");
    expect(nasStaleText(ago(30), NOW)).toBe("这是不到 1 分钟前的数据");
    expect(nasStaleText(ago(600), NOW)).toBe("这是 10 分钟前的数据");
    expect(nasStaleText(ago(3 * 3600), NOW)).toBe("这是 3 小时前的数据");
    expect(nasStaleText(ago(2 * 86400), NOW)).toMatch(/^这是 \d{2}-\d{2} \d{2}:\d{2} 的数据$/);
  });

  it("下次重试：马上、N 秒后、N 分钟后；没有下次时为空", () => {
    expect(nasRetryText(undefined, NOW)).toBe("");
    expect(nasRetryText(ago(5), NOW)).toBe("马上再试");
    expect(nasRetryText(ago(-25), NOW)).toBe("25 秒后再试");
    expect(nasRetryText(ago(-125), NOW)).toBe("3 分钟后再试");
  });

  it("卡顶的说明：连不上写原因与重试；登录失败与证书写去哪里处理", () => {
    const base = {
      id: "2",
      name: "书房",
      kind: "truenas",
      enabled: true,
      experimental: false,
      has_secret: true,
      insecure_ack: false,
      snapshot: null,
      history: [],
    };
    expect(
      nasProblemNote({ ...base, state: "error", message: "超时", next_try_at: ago(-30) }, NOW),
    ).toBe("连不上 书房：超时。30 秒后再试");
    expect(nasProblemNote({ ...base, state: "error" }, NOW)).toBe("连不上 书房");
    expect(nasProblemNote({ ...base, state: "auth_failed", message: "Key 不对" }, NOW)).toBe(
      "书房 登录失败：Key 不对。改了设置或点「重新连接」以后才会再试",
    );
    expect(nasProblemNote({ ...base, state: "cert_untrusted" }, NOW)).toContain("确认证书");
    expect(nasProblemNote({ ...base, state: "cert_changed" }, NOW)).toContain("确认新证书");
    expect(nasProblemNote({ ...base, state: "ok" }, NOW)).toBe("");
  });

  it("百分比 10 以下一位小数、其余取整、越界夹住；运行时长按天、小时、分钟", () => {
    expect(nasPctText(3.84)).toBe("3.8");
    expect(nasPctText(71.4)).toBe("71");
    expect(nasPctText(140)).toBe("100");
    expect(nasPctText(Number.NaN)).toBe("0");
    expect(nasUptimeText(0)).toBe("");
    expect(nasUptimeText(18 * 86400 + 18 * 3600)).toBe("已运行 18 天 18 小时");
    expect(nasUptimeText(2 * 86400)).toBe("已运行 2 天");
    expect(nasUptimeText(3 * 3600 + 5 * 60)).toBe("已运行 3 小时 5 分");
    expect(nasUptimeText(20)).toBe("已运行 1 分钟");
  });
});

describe("数值卡", () => {
  it("读不到的那类写原因（没写原因时按状态给一句），快照里没写这一类按读到了算", () => {
    const s = snap({
      facets: {
        cpu_temp: { status: "unsupported", reason: "TrueNAS 没有给 CPU 温度" },
        pools: { status: "denied" },
        disks: { status: "ok" },
      },
    });
    expect(nasFacetReason(s, "cpu_temp")).toBe("TrueNAS 没有给 CPU 温度");
    expect(nasFacetReason(s, "pools")).toBe("账号没有读这一类的权限");
    expect(nasFacetReason(s, "disks")).toBe("");
    expect(nasFacetReason(s, "alerts")).toBe("");
    expect(nasFacetReason(null, "cpu")).toBe("");
  });

  it("CPU 卡：负载、IO 等待与温度；温度读不到时写一句、悬停看原因；整类读不到时没有数值；手机上只写负载与温度", () => {
    expect(
      nasCpuKpi(
        snap({ cpu: { percent: 3.8, iowait: 12, load1: 3.71, load5: 0, load15: 0, temp_c: 51.6 } }),
      ),
    ).toEqual({
      value: 3.8,
      aside: "8 核",
      sub: "负载 3.7 · IO 等待 12% · 52 °C",
      title: undefined,
      short: "负载 3.7 · 52 °C",
    });
    const noTemp = nasCpuKpi(
      snap({ facets: { cpu_temp: { status: "unsupported", reason: "没有温度传感器" } } }),
    );
    expect(noTemp.sub).toBe("负载 3.7 · IO 等待 12% · 读不到温度");
    expect(noTemp.title).toBe("没有温度传感器");
    expect(noTemp.short, "手机上只写负载，不写半句「读不到…」").toBe("负载 3.7");
    expect(nasCpuKpi(snap({ facets: { cpu: { status: "error", reason: "超时" } } }))).toMatchObject(
      {
        value: null,
        sub: "超时",
        missing: "超时",
      },
    );
  });

  it("内存卡：已用百分比、已用与可用；没读到总量时没有数值", () => {
    const m = nasMemKpi(snap());
    expect(m.value).toBeCloseTo(62.5);
    expect(m.aside).toBe("共 64.0 GB");
    expect(m.sub).toBe("已用 40.0 GB · 可用 24.0 GB");
    expect(m.short).toBe("已用 40.0 GB");
    expect(nasMemKpi(snap({ memory: { total: 0, used: 0, available: 0 } }))).toMatchObject({
      value: null,
      missing: "没读到内存总量",
    });
  });

  it("网卡：一块写名字，几块写块数，没有时为空", () => {
    expect(nasIfaceText(snap())).toBe("ens161np1");
    expect(nasIfaceText(snap({ network: { rx_bps: 0, tx_bps: 0, interfaces: ["a", "b"] } }))).toBe(
      "2 块网卡",
    );
    expect(nasIfaceText(snap({ network: { rx_bps: 0, tx_bps: 0, interfaces: null } }))).toBe("");
  });
});

describe("存储", () => {
  it("池的合计只算知道容量的，已用不超过容量；段宽按容量、最少 5%", () => {
    const s = nasPoolSummary([
      pool("big", 30 * TB, 24 * TB),
      pool("tiny", 0.5 * TB, 1 * TB),
      pool("unknown", 0, 5),
    ]);
    expect(s.size).toBe(30.5 * TB);
    expect(s.used).toBe(24.5 * TB);
    expect(s.free).toBe(6 * TB);
    expect(s.percent).toBeCloseTo((24.5 / 30.5) * 100);
    expect(s.segments.map((x) => x.name)).toEqual(["big", "tiny"]);
    expect(s.segments[1]!.percent).toBe(100);
    expect(s.segments[1]!.weight).toBe(0.05);
    expect(nasPoolSummary(null)).toEqual({
      percent: null,
      used: 0,
      size: 0,
      free: 0,
      segments: [],
    });
  });

  it("本机的盘：出错的与不知道总量的不算", () => {
    const s = nasLocalDiskSummary([
      { kind: "download", label: "下载盘", path: "/data", total: 100, free: 40, low: false },
      { kind: "library", label: "媒体库", path: "/media", total: 0, free: 10, low: false },
      {
        kind: "library",
        label: "坏的",
        path: "/x",
        total: 50,
        free: 0,
        low: false,
        error: "读不到",
      },
    ] as never);
    expect(s.size).toBe(100);
    expect(s.used).toBe(60);
    expect(s.segments.map((x) => x.name)).toEqual(["下载盘"]);
  });

  it("80% 起提醒、90% 起危险；池的健康标签、副行与右上角", () => {
    expect(nasUsedTone(79.9)).toBe("primary");
    expect(nasUsedTone(80)).toBe("warn");
    expect(nasUsedTone(90)).toBe("dang");
    expect(nasPoolHealth(pool("a", 1, 0))).toBeNull();
    expect(nasPoolHealth(pool("a", 1, 0, { healthy: true, status: "ONLINE" }))).toEqual({
      label: "ONLINE",
      tone: "ok",
    });
    expect(nasPoolHealth(pool("a", 1, 0, { healthy: false }))).toEqual({
      label: "不健康",
      tone: "dang",
    });
    expect(nasPoolSub(pool("a", 2 * TB, 1 * TB, { disks: 3 }))).toBe(
      "已用 1.0 TB · 共 2.0 TB · 3 块硬盘",
    );
    expect(nasPoolSub(pool("a", 2 * TB, 1 * TB))).toBe("已用 1.0 TB · 共 2.0 TB");
    expect(nasPoolsAside([pool("a", 1, 0)])).toBe("");
    expect(
      nasPoolsAside([pool("a", 1, 0, { healthy: true }), pool("b", 1, 0, { healthy: false })]),
    ).toBe("1 个不健康");
    expect(nasPoolsAside([pool("a", 1, 0, { healthy: true })])).toBe("都健康");
  });

  it("硬盘温度：机械盘 50 °C、固态与 NVMe 70 °C 起提醒；温度条 20–80 °C、最短 4%；最热的一块", () => {
    expect(nasDiskTempTone({ kind: "hdd", temp_c: 50 })).toBe("warn");
    expect(nasDiskTempTone({ kind: "unknown", temp_c: 49 })).toBe("ok");
    expect(nasDiskTempTone({ kind: "nvme", temp_c: 69 })).toBe("ok");
    expect(nasDiskTempTone({ kind: "ssd", temp_c: 70 })).toBe("warn");
    expect(nasDiskTempTone({ kind: "hdd" })).toBe("neutral");
    expect(nasTempGauge(10)).toBe(4);
    expect(nasTempGauge(50)).toBe(50);
    expect(nasTempGauge(95)).toBe(100);
    expect(nasDiskSub({ kind: "hdd", size: 2 * TB })).toBe("HDD · 2.0 TB");
    expect(nasDiskSub({ kind: "unknown", size: 0 })).toBe("");
    const disks: NasDisk[] = [
      { name: "sda", model: "", kind: "ssd", size: 1 },
      { name: "sdb", model: "", kind: "hdd", size: 1, temp_c: 37 },
      { name: "nvme0n1", model: "", kind: "nvme", size: 1, temp_c: 59 },
    ];
    expect(nasHottestDisk(disks)?.name).toBe("nvme0n1");
    expect(nasHottestDisk([disks[0]!])).toBeNull();
  });
});

describe("告警、应用与主机行", () => {
  const alerts: NasAlert[] = [
    { level: "info", title: "旧的", at: ago(3600) },
    { level: "warning", title: "没有时间" },
    { level: "critical", title: "同一时刻的严重", at: ago(60) },
    { level: "info", title: "同一时刻的信息", at: ago(60) },
  ];

  it("按级别计数；最新的在前，时间一样时严重的在前，没有时间的排后", () => {
    expect(nasAlertCounts(alerts)).toEqual({ critical: 1, warning: 1, info: 2 });
    expect(nasAlertCounts(null)).toEqual({ critical: 0, warning: 0, info: 0 });
    expect(nasLatestAlerts(alerts).map((a) => a.title)).toEqual([
      "同一时刻的严重",
      "同一时刻的信息",
      "旧的",
    ]);
    expect(nasLatestAlerts(alerts, 4).at(-1)?.title).toBe("没有时间");
    expect(nasAlertTone("critical")).toBe("dang");
    expect(nasAlertTone("warning")).toBe("warn");
    expect(nasAlertTone("info")).toBe("info");
  });

  it("应用那一行", () => {
    expect(nasAppsText({ running: 1, total: 3, upgradable: 0 })).toBe(
      "运行中 1 个 · 共 3 个 · 没有可更新的",
    );
    expect(nasAppsText({ running: 2, total: 2, upgradable: 1 })).toBe(
      "运行中 2 个 · 共 2 个 · 1 个可更新",
    );
    expect(nasAppsText({ running: 0, total: 0, upgradable: 0 })).toBe("没有应用");
  });

  it("主机行的提醒：严重告警条数、快满的池（一个写名字，几个写个数），读不到的那类不提醒", () => {
    expect(
      nasHostFacts(
        snap({ alerts, pools: [pool("UniData", 100, 85), pool("QbitSSD", 100, 50)] }),
      ).map((f) => [f.text, f.tone]),
    ).toEqual([
      ["1 条严重告警", "dang"],
      ["UniData 已用 85%", "warn"],
    ]);
    expect(
      nasHostFacts(snap({ pools: [pool("a", 100, 95), pool("b", 100, 81)] })).map((f) => [
        f.text,
        f.tone,
      ]),
    ).toEqual([["2 个存储池已用超过 80%", "dang"]]);
    expect(
      nasHostFacts(
        snap({
          alerts,
          pools: [pool("a", 100, 95)],
          facets: { alerts: { status: "unsupported" }, pools: { status: "error" } },
        }),
      ),
    ).toEqual([]);
  });

  it("设置页的说明行；首页显示的选项", () => {
    expect(nasSourceSummary({ snapshot: snap() })).toBe(
      "truenas · i7-12700 · 8 核 · 64.0 GB · TrueNAS SCALE 25.04.1",
    );
    expect(nasSourceSummary({ snapshot: null })).toBe("");
    expect(
      nasHomeOptions([
        { id: "local", name: "本机", kind: "local" },
        { id: "2", name: "书房", kind: "truenas" },
      ]),
    ).toEqual([
      { value: "", label: "自动" },
      { value: "local", label: "本机" },
      { value: "2", label: "书房（TrueNAS）" },
    ]);
  });
});

describe("添加来源时的准备步骤", () => {
  it("TrueNAS 写怎么建只读用户与 API Key、只收 https；别的类型没有", () => {
    const g = nasKindGuide("truenas");
    expect(g?.title).toContain("25.04");
    expect(g?.steps).toHaveLength(3);
    expect(g?.steps.join("")).toContain("truenas_readonly_administrators");
    expect(g?.steps.join("")).toContain("https");
    expect(nasKindGuide("glances")).toBeNull();
    expect(nasKindGuide("")).toBeNull();
  });
});
