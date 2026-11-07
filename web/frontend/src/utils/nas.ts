/*
 * NAS 监控（首页 NAS 概览 H2）的文字与颜色：来源的状态、首页概览卡的主机行与四张数值卡、存储池与硬盘温度的颜色、
 * 告警计数。首页的概览卡与「NAS 监控」设置页共用；数据都来自 GET /api/nas（与 /api/home 的 nas 块）。
 */
import type {
  HomeDisk,
  NasAlert,
  NasDisk,
  NasFacetKey,
  NasKind,
  NasPool,
  NasSnapshot,
  NasSource,
  NasState,
} from "@/api";
import { formatShortDateTime } from "@/utils/format";
import { sizeText, type Tone } from "@/utils/home";

/** 本机来源的编号（数据库里的来源是数字编号） */
export const NAS_LOCAL_ID = "local";

/** 已用比例到这里提醒、到 DANG 危险（存储池与存储卡） */
export const NAS_USED_WARN = 80;
export const NAS_USED_DANG = 90;

/** 硬盘温度到这里提醒：机械盘 50 °C，固态与 NVMe 70 °C */
export const NAS_TEMP_HDD = 50;
export const NAS_TEMP_SSD = 70;

/** 服务端的类型表没给名字时用的（本机来源不在类型表里） */
const KIND_LABELS: Record<string, string> = {
  local: "本机",
  truenas: "TrueNAS",
  glances: "Glances",
  synology: "群晖",
  fnos: "飞牛",
};

/** 来源类型的名字：先用服务端类型表里的，没有时用内置的，都没有时原样 */
export function nasKindLabel(kind: string, kinds?: readonly NasKind[] | null): string {
  return kinds?.find((k) => k.name === kind)?.label || KIND_LABELS[kind] || kind;
}

/** 添加来源时写在类型下面的准备步骤：要在 NAS 上先做的事。NAS 网页上的按钮名照它默认的英文界面写 */
export interface NasKindGuide {
  title: string;
  steps: readonly string[];
}

const KIND_GUIDES: Record<string, NasKindGuide> = {
  truenas: {
    title: "先在 TrueNAS 上建一把只读的 API Key（要 TrueNAS 25.04 或以后的版本）",
    steps: [
      "在「Credentials → Users」点「Add」，建一个只给 pt-tools 用的用户：打开「Disable Password」，「Auxiliary Groups」选 truenas_readonly_administrators（只读管理员），不勾「SMB User」，不给 sudo",
      "在右上角的用户菜单点「My API Keys」→「Add」，「Username」选这个用户，保存后复制 Key（只显示这一次）",
      "下面写 TrueNAS 网页的 https 地址、粘贴 Key，点「测试连接」核对并信任它的证书。经 http 用过的 Key 会被 TrueNAS 吊销，所以这里只收 https",
    ],
  },
  glances: {
    title: "先在 NAS 上用 Docker 跑 Glances 4（官方镜像，例如 nicolargo/glances:4.5.8-full）",
    steps: [
      "用这个镜像建一个容器：网络用 host（network_mode: host），与主机共用进程（pid: host），环境变量 GLANCES_OPT=-w。镜像标签写 4.x 的版本号，不用 latest：latest 以后会换成 pt-tools 还不支持的 Glances 5。群晖用 Container Manager，飞牛、Unraid、OMV 用各自的 Docker",
      "要看存储就把存储目录只读挂进容器（例如 /volume1:/volume1:ro）；要看容器挂 /var/run/docker.sock:/var/run/docker.sock:ro；要显示 NAS 自己的系统名挂 /etc/os-release:/etc/os-release:ro",
      "下面写 http://NAS 的地址:61208（不写端口时按 61208）。Glances 默认不开认证；开了的话填用户名（默认 glances）和密码，经 http 发密码前要勾选明文风险",
    ],
  },
  synology: {
    title: "先在群晖上建一个只给 pt-tools 用的账号（DSM 7）",
    steps: [
      "在「Control Panel → User & Group」点「Create」建一个用户，设一个强密码；「User Groups」勾选 administrators：DSM 只让管理员组的账号读系统信息与资源监控",
      "「Permissions」勾最上面的「No Access」，这个账号就进不了共享文件夹；「Applications」先在表头勾「Deny」，再只给「DSM」这一行勾「Allow」（或「By IP」，只允许 pt-tools 所在主机的 IP）",
      "下面写 DSM 网页的地址（https 默认端口 5001，http 默认 5000）和这个账号的密码，点「测试连接」。账号开了两步验证时填一次验证码，以后 pt-tools 用这台设备的令牌登录；经 http 发密码前要勾选明文风险",
      "pt-tools 登录失败一次就停下来、等你改好再测，不会反复试密码触发 DSM 的自动封锁",
    ],
  },
  fnos: {
    title: "先在飞牛上建一个只给 pt-tools 用的管理员账号（fnOS）",
    steps: [
      "在飞牛网页的用户设置里新建一个用户，设一个强密码，给它管理员权限：飞牛只让管理员读存储空间与硬盘",
      "下面写飞牛网页的地址（https 默认端口 5667，http 默认 5666）和这个账号的用户名、密码，点「测试连接」。账号开了两步验证时填一次验证码，以后 pt-tools 用登录得到的 token 连接，不用再填；用 http 时要先勾选明文风险",
      "pt-tools 登录失败一次就停下来、等你改好再测，不会反复试密码",
    ],
  },
};

/** 这类来源的准备步骤；没写的类型为 null */
export function nasKindGuide(kind: string): NasKindGuide | null {
  return KIND_GUIDES[kind] ?? null;
}

/**
 * 地址栏的示例：Glances 自己只有 http，默认端口 61208；群晖写 DSM 的 https 端口 5001，飞牛写它的 https 端口 5667；
 * 别的类型写 https
 */
export function nasUrlPlaceholder(kind: string): string {
  switch (kind) {
    case "glances":
      return "http://192.168.1.10:61208";
    case "synology":
      return "https://192.168.1.10:5001";
    case "fnos":
      return "https://192.168.1.10:5667";
    default:
      return "https://192.168.1.10";
  }
}

/** 来源类型的图标 */
export function nasKindIcon(kind: string): string {
  switch (kind) {
    case "local":
      return "monitor";
    case "glances":
      return "gauge";
    case "synology":
      return "hard-drive";
    case "fnos":
      return "warehouse";
    default:
      return "server";
  }
}

const STATES: Record<NasState, { label: string; tone: Tone }> = {
  ok: { label: "已连上", tone: "ok" },
  connecting: { label: "正在连", tone: "info" },
  error: { label: "连不上", tone: "dang" },
  auth_failed: { label: "登录失败", tone: "dang" },
  cert_untrusted: { label: "证书不受信任", tone: "warn" },
  cert_changed: { label: "证书换了", tone: "dang" },
  disabled: { label: "已停用", tone: "neutral" },
  unsupported: { label: "用不了", tone: "neutral" },
};

/** 状态的文字与颜色 */
export function nasStateView(state: NasState | string): { label: string; tone: Tone } {
  return STATES[state as NasState] ?? { label: state, tone: "neutral" };
}

/** 连不上、登录失败、证书的问题：卡里要写原因、数据按过时的画 */
export function nasProblem(state: NasState | string): boolean {
  return (
    state === "error" ||
    state === "auth_failed" ||
    state === "cert_untrusted" ||
    state === "cert_changed"
  );
}

/** 这个来源能在首页显示：启用了，而且这个版本、这个平台能用（同服务端的 nasUsable） */
export function nasUsable(s: Pick<NasSource, "enabled" | "state">): boolean {
  return s.enabled && s.state !== "unsupported";
}

function seconds(at: string | undefined, now: Date): number | null {
  if (!at) return null;
  const t = new Date(at).getTime();
  if (Number.isNaN(t)) return null;
  return (now.getTime() - t) / 1000;
}

/** 标题行右边的更新时间：「刚刚更新」「40 秒前更新」「3 分钟前更新」「2 小时前更新」 */
export function nasUpdatedText(at: string | undefined, now = new Date()): string {
  const s = seconds(at, now);
  if (s === null) return "";
  if (s < 15) return "刚刚更新";
  if (s < 60) return `${Math.floor(s)} 秒前更新`;
  if (s < 3600) return `${Math.floor(s / 60)} 分钟前更新`;
  if (s < 86400) return `${Math.floor(s / 3600)} 小时前更新`;
  return `${formatShortDateTime(at, now)} 更新`;
}

/** 连不上时数据多旧：「这是 3 分钟前的数据」 */
export function nasStaleText(at: string | undefined, now = new Date()): string {
  const s = seconds(at, now);
  if (s === null) return "";
  if (s < 60) return "这是不到 1 分钟前的数据";
  if (s < 3600) return `这是 ${Math.floor(s / 60)} 分钟前的数据`;
  if (s < 86400) return `这是 ${Math.floor(s / 3600)} 小时前的数据`;
  return `这是 ${formatShortDateTime(at, now)} 的数据`;
}

/** 下次重试：「30 秒后再试」「2 分钟后再试」；没有下次（认证失败、证书的问题停住了）时为空 */
export function nasRetryText(at: string | undefined, now = new Date()): string {
  const s = seconds(at, now);
  if (s === null) return "";
  const left = -s;
  if (left <= 1) return "马上再试";
  if (left < 60) return `${Math.ceil(left)} 秒后再试`;
  return `${Math.ceil(left / 60)} 分钟后再试`;
}

/**
 * 卡顶的红色说明：连不上写原因与下次重试；登录失败与证书的问题不会自动重试（停住了），写去哪里处理。
 * message 是服务端去掉了凭证的错误原文。
 */
export function nasProblemNote(src: NasSource, now = new Date()): string {
  const why = src.message ? `：${src.message}` : "";
  switch (src.state) {
    case "error": {
      const retry = nasRetryText(src.next_try_at, now);
      return `连不上 ${src.name}${why}${retry ? `。${retry}` : ""}`;
    }
    case "auth_failed":
      return `${src.name} 登录失败${why}。改了设置或点「重新连接」以后才会再试`;
    case "cert_untrusted":
      return `${src.name} 的证书不受信任，没有发出凭据就断开了：到 NAS 监控里测试连接并确认证书`;
    case "cert_changed":
      return `${src.name} 的证书和信任的那张不一样，没有发出凭据就断开了：到 NAS 监控里确认新证书`;
    default:
      return "";
  }
}

/** 百分比：10 以下写一位小数（3.8），其余取整（71） */
export function nasPctText(v: number): string {
  if (!Number.isFinite(v)) return "0";
  const c = Math.min(100, Math.max(0, v));
  return c < 10 ? c.toFixed(1) : String(Math.round(c));
}

/** 运行时长：「已运行 18 天 18 小时」「已运行 3 小时 5 分」「已运行 12 分钟」 */
export function nasUptimeText(sec: number): string {
  if (!Number.isFinite(sec) || sec <= 0) return "";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  if (d > 0) return `已运行 ${d} 天${h ? ` ${h} 小时` : ""}`;
  if (h > 0) return `已运行 ${h} 小时${m ? ` ${m} 分` : ""}`;
  return `已运行 ${Math.max(1, m)} 分钟`;
}

const FACET_DEFAULT: Record<string, string> = {
  unsupported: "这个来源读不到",
  denied: "账号没有读这一类的权限",
  error: "这一次没读到",
};

/** 这一类数据读不到的原因（读到了时为空）；快照里没写这一类时按读到了算 */
export function nasFacetReason(snap: NasSnapshot | null | undefined, key: NasFacetKey): string {
  const f = snap?.facets?.[key];
  if (!f || f.status === "ok") return "";
  return f.reason || FACET_DEFAULT[f.status] || "读不到";
}

/** 主机行的平台与版本：「TrueNAS SCALE 25.04.1」「Linux 6.12.15」 */
export function nasPlatformText(snap: NasSnapshot | null | undefined): string {
  if (!snap) return "";
  return [snap.system.platform, snap.system.version].filter(Boolean).join(" ");
}

/** 主机行的型号与核数：「i7-12700 · 8 核」 */
export function nasModelText(snap: NasSnapshot | null | undefined): string {
  if (!snap) return "";
  const cores = snap.system.cores > 0 ? `${snap.system.cores} 核` : "";
  return [snap.system.model, cores].filter(Boolean).join(" · ");
}

/** 一张数值卡要写的：value 为 null 是这个来源读不到（missing 是原因） */
export interface NasKpi {
  value: number | null;
  aside: string;
  sub: string;
  /** 副行的悬停提示（完整的原因） */
  title?: string;
  /** 手机上的副行：卡只有一半宽，只写最要紧的一两项，不截成半句 */
  short?: string;
  missing?: string;
}

/** CPU 卡：负载、IO 等待与温度；温度读不到时写一句，悬停看原因 */
export function nasCpuKpi(snap: NasSnapshot): NasKpi {
  const missing = nasFacetReason(snap, "cpu");
  const aside = snap.system.cores > 0 ? `${snap.system.cores} 核` : "";
  if (missing) return { value: null, aside, sub: missing, missing };
  const parts = [`负载 ${snap.cpu.load1.toFixed(1)}`];
  if (snap.cpu.iowait !== undefined) parts.push(`IO 等待 ${nasPctText(snap.cpu.iowait)}%`);
  let title: string | undefined;
  const load = parts[0]!;
  let short = load;
  if (snap.cpu.temp_c !== undefined) {
    const temp = `${Math.round(snap.cpu.temp_c)} °C`;
    parts.push(temp);
    short = `${load} · ${temp}`;
  } else {
    const why = nasFacetReason(snap, "cpu_temp");
    if (why) {
      parts.push("读不到温度");
      title = why;
    }
  }
  return { value: snap.cpu.percent, aside, sub: parts.join(" · "), title, short };
}

/** 内存卡：已用的百分比，已用与可用 */
export function nasMemKpi(snap: NasSnapshot): NasKpi {
  const missing = nasFacetReason(snap, "memory");
  const m = snap.memory;
  const aside = m.total > 0 ? `共 ${sizeText(m.total)}` : "";
  if (missing || m.total <= 0) {
    const why = missing || "没读到内存总量";
    return { value: null, aside, sub: why, missing: why };
  }
  return {
    value: (m.used / m.total) * 100,
    aside,
    sub: `已用 ${sizeText(m.used)} · 可用 ${sizeText(m.available)}`,
    short: `已用 ${sizeText(m.used)}`,
  };
}

/** 网络卡的右上角：一块网卡写名字，几块写块数 */
export function nasIfaceText(snap: NasSnapshot): string {
  const list = snap.network.interfaces ?? [];
  if (list.length === 1) return list[0]!;
  return list.length > 1 ? `${list.length} 块网卡` : "";
}

/** 存储卡的分段条与图例：每段一个池（或一块盘），段宽按容量，段里是已用 */
export interface NasStorageSegment {
  name: string;
  percent: number;
  /** 段宽占比（最少 5%，太小的池也看得见） */
  weight: number;
}

export interface NasStorageSummary {
  /** 合计已用的百分比；没有知道容量的池时为 null */
  percent: number | null;
  used: number;
  size: number;
  free: number;
  segments: NasStorageSegment[];
}

function summary(items: { name: string; size: number; used: number }[]): NasStorageSummary {
  const list = items.filter((it) => it.size > 0);
  const size = list.reduce((n, it) => n + it.size, 0);
  const used = list.reduce((n, it) => n + Math.min(it.used, it.size), 0);
  if (!size) return { percent: null, used: 0, size: 0, free: 0, segments: [] };
  return {
    percent: (used / size) * 100,
    used,
    size,
    free: size - used,
    segments: list.map((it) => ({
      name: it.name,
      percent: (Math.min(it.used, it.size) / it.size) * 100,
      weight: Math.max(it.size / size, 0.05),
    })),
  };
}

/** NAS 的存储：各池合计（用的是可用容量，不是裸容量） */
export function nasPoolSummary(pools: readonly NasPool[] | null | undefined): NasStorageSummary {
  return summary((pools ?? []).map((p) => ({ name: p.name, size: p.size, used: p.used })));
}

/**
 * 本机来源的存储：首页已有的下载盘与媒体库所在的盘（服务端已按设备去重）。只算知道总量的盘：
 * 下载器只报剩余的那块盘没有总量，算不进百分比，照样列在存储那一块里。
 */
export function nasLocalDiskSummary(
  disks: readonly HomeDisk[] | null | undefined,
): NasStorageSummary {
  return summary(
    (disks ?? [])
      .filter((d) => !d.error)
      .map((d) => ({ name: d.label, size: d.total, used: d.total - d.free })),
  );
}

/** 一个池的已用百分比（不知道容量时为 0） */
export function nasPoolPercent(p: Pick<NasPool, "size" | "used">): number {
  return p.size > 0 ? Math.min(100, (p.used / p.size) * 100) : 0;
}

/** 已用比例的颜色：80% 起提醒，90% 起危险 */
export function nasUsedTone(percent: number): "primary" | "warn" | "dang" {
  if (percent >= NAS_USED_DANG) return "dang";
  if (percent >= NAS_USED_WARN) return "warn";
  return "primary";
}

/** 池的健康标签：读不到健康时不画（null） */
export function nasPoolHealth(p: Pick<NasPool, "healthy" | "status">): {
  label: string;
  tone: Tone;
} | null {
  if (p.healthy === undefined) return null;
  const label = p.status || (p.healthy ? "健康" : "不健康");
  return { label, tone: p.healthy ? "ok" : "dang" };
}

/** 池的副行：「已用 37.1 TB · 共 43.6 TB · 3 块硬盘」 */
export function nasPoolSub(p: NasPool): string {
  const parts = [`已用 ${sizeText(p.used)}`, `共 ${sizeText(p.size)}`];
  if (p.disks > 0) parts.push(`${p.disks} 块硬盘`);
  return parts.join(" · ");
}

/** 存储池一块的右上角：「都在线」「1 个不健康」；都读不到健康时为空 */
export function nasPoolsAside(pools: readonly NasPool[]): string {
  const known = pools.filter((p) => p.healthy !== undefined);
  if (!known.length) return "";
  const bad = known.filter((p) => !p.healthy).length;
  return bad ? `${bad} 个不健康` : "都健康";
}

/** 这块盘的温度提醒线（机械盘与不知道类型的按 50 °C，固态与 NVMe 按 70 °C） */
export function nasTempLimit(kind: NasDisk["kind"]): number {
  return kind === "ssd" || kind === "nvme" ? NAS_TEMP_SSD : NAS_TEMP_HDD;
}

/** 硬盘温度的颜色：读不到时 neutral，到提醒线 warn，否则 ok */
export function nasDiskTempTone(d: Pick<NasDisk, "kind" | "temp_c">): "ok" | "warn" | "neutral" {
  if (d.temp_c === undefined) return "neutral";
  return d.temp_c >= nasTempLimit(d.kind) ? "warn" : "ok";
}

/** 温度条的长度（20 °C 起到 80 °C 满，最短 4%） */
export function nasTempGauge(temp: number): number {
  return Math.min(100, Math.max(4, ((temp - 20) / 60) * 100));
}

const DISK_KINDS: Record<string, string> = { hdd: "HDD", ssd: "SSD", nvme: "NVMe" };

/** 硬盘方块的副行：「HDD · 14.6 TB」 */
export function nasDiskSub(d: Pick<NasDisk, "kind" | "size">): string {
  return [DISK_KINDS[d.kind] ?? "", d.size > 0 ? sizeText(d.size) : ""].filter(Boolean).join(" · ");
}

/** 最热的一块盘（读不到温度的不算） */
export function nasHottestDisk(disks: readonly NasDisk[]): NasDisk | null {
  let hot: NasDisk | null = null;
  for (const d of disks) {
    if (d.temp_c === undefined) continue;
    if (!hot || d.temp_c > (hot.temp_c ?? -Infinity)) hot = d;
  }
  return hot;
}

/** 告警按级别的条数 */
export function nasAlertCounts(alerts: readonly NasAlert[] | null | undefined): {
  critical: number;
  warning: number;
  info: number;
} {
  const out = { critical: 0, warning: 0, info: 0 };
  for (const a of alerts ?? []) {
    if (a.level === "critical") out.critical += 1;
    else if (a.level === "warning") out.warning += 1;
    else out.info += 1;
  }
  return out;
}

const LEVEL_RANK: Record<string, number> = { critical: 0, warning: 1, info: 2 };

/** 最新的几条告警：先按时间（新的在前，没有时间的排后），时间一样时严重的在前 */
export function nasLatestAlerts(alerts: readonly NasAlert[] | null | undefined, n = 3): NasAlert[] {
  const t = (a: NasAlert) => {
    const v = a.at ? new Date(a.at).getTime() : Number.NaN;
    return Number.isNaN(v) ? -Infinity : v;
  };
  return [...(alerts ?? [])]
    .sort((a, b) => t(b) - t(a) || (LEVEL_RANK[a.level] ?? 3) - (LEVEL_RANK[b.level] ?? 3))
    .slice(0, n);
}

/** 告警级别的颜色 */
export function nasAlertTone(level: NasAlert["level"]): "dang" | "warn" | "info" {
  if (level === "critical") return "dang";
  return level === "warning" ? "warn" : "info";
}

/**
 * 告警一块的右上角：告警从哪来。Glances 不给 NAS 自己的告警，那里的告警是 pt-tools 按软 RAID 阵列的状态算的；
 * 其余来源是 NAS 里没关掉的。还不知道类型名时为空。
 */
export function nasAlertsAside(kind: string, label: string): string {
  if (!label) return "";
  if (kind === "glances") return "按软 RAID 阵列的状态";
  // 拉丁字母结尾的类型名（TrueNAS）与后面的中文之间留空格，中文的（群晖）不留
  return /[A-Za-z0-9]$/.test(label) ? `${label} 里没关掉的` : `${label}里没关掉的`;
}

/** 应用那一行：「运行中 1 个 · 共 3 个 · 没有可更新的」 */
export function nasAppsText(apps: NasSnapshot["apps"]): string {
  if (apps.total <= 0) return "没有应用";
  const head = `运行中 ${apps.running} 个 · 共 ${apps.total} 个`;
  // 来源不知道哪些能更新（Glances 只看得到容器）时不写可更新几个
  if (apps.updates_unknown) return head;
  const up = apps.upgradable > 0 ? `${apps.upgradable} 个可更新` : "没有可更新的";
  return `${head} · ${up}`;
}

/** 主机行右边的提醒（最多两个）：严重告警的条数、已用超过 80% 的存储池 */
export interface NasHostFact {
  key: "alerts" | "pools";
  text: string;
  tone: "dang" | "warn";
  icon: string;
}

export function nasHostFacts(snap: NasSnapshot): NasHostFact[] {
  const out: NasHostFact[] = [];
  if (!nasFacetReason(snap, "alerts")) {
    const n = nasAlertCounts(snap.alerts).critical;
    if (n > 0) out.push({ key: "alerts", text: `${n} 条严重告警`, tone: "dang", icon: "bell" });
  }
  if (!nasFacetReason(snap, "pools")) {
    const full = (snap.pools ?? []).filter((p) => nasPoolPercent(p) >= NAS_USED_WARN);
    if (full.length === 1) {
      const p = full[0]!;
      out.push({
        key: "pools",
        text: `${p.name} 已用 ${nasPctText(nasPoolPercent(p))}%`,
        tone: nasPoolPercent(p) >= NAS_USED_DANG ? "dang" : "warn",
        icon: "database",
      });
    } else if (full.length > 1) {
      out.push({
        key: "pools",
        text: `${full.length} 个存储池已用超过 ${NAS_USED_WARN}%`,
        tone: full.some((p) => nasPoolPercent(p) >= NAS_USED_DANG) ? "dang" : "warn",
        icon: "database",
      });
    }
  }
  return out.slice(0, 2);
}

/** 设置页一行的说明：读到的主机、型号与内存（还没读到时为空） */
export function nasSourceSummary(src: Pick<NasSource, "snapshot">): string {
  const snap = src.snapshot;
  if (!snap) return "";
  const host = snap.system.hostname;
  const mem = snap.memory.total > 0 ? sizeText(snap.memory.total) : "";
  return [host, nasModelText(snap), mem, nasPlatformText(snap)].filter(Boolean).join(" · ");
}

/** 首页显示哪个来源的选项：自动、本机与各个来源 */
export function nasHomeOptions(
  sources: readonly Pick<NasSource, "id" | "name" | "kind">[],
  kinds?: readonly NasKind[] | null,
): { value: string; label: string }[] {
  return [
    { value: "", label: "自动" },
    ...sources.map((s) => ({
      value: s.id,
      label: s.id === NAS_LOCAL_ID ? "本机" : `${s.name}（${nasKindLabel(s.kind, kinds)}）`,
    })),
  ];
}
