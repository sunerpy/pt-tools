<script setup lang="ts">
/*
 * 首页的「NAS 概览」（首页 NAS 概览 H2，方案 E1 = A，规格 docs/design/home-nas-spec.md）：首页最上面满宽的一张卡。
 * 主机行（主机名、平台与版本、型号与核数、内存、运行时长，右边最多两个要处理的标签）、四张数值卡（CPU、内存、网络、
 * 存储，前三张下面是最近 30 拍的柱），第二行是存储池、硬盘与告警和应用。本机来源时网络卡是下载器速度、
 * 存储是首页已有的下载盘与媒体库，第二行换成存储与一块「本机读不到」的虚线框。
 *
 * 数据来自 useNasHome（GET /api/nas 每 30 秒一次，只读内存）。来源读不到的那类按 facets 写原因，不把没有当成 0。
 * 连不上时顶上一条红色说明、柱与图标变灰、标题行写数据多旧；不用 opacity 压暗（paletteContrast.test.ts 守着）。
 */
import type { HomeBlock, HomeDisk, HomeDownloaders, HomeNas, NasView } from "@/api";
import PtIcon from "@/components/PtIcon";
import HomeSection from "@/components/home/HomeSection.vue";
import PtBars from "@/components/ui/PtBars.vue";
import PtProgress from "@/components/ui/PtProgress.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useHomeSpeed } from "@/composables/useHomeSpeed";
import { useIsMobile } from "@/composables/useIsMobile";
import { useTweenedNumber } from "@/composables/useTweenedNumber";
import { type Tone, diskBar, mobileDisks, sizeText, speedText, whenText } from "@/utils/home";
import {
  NAS_LOCAL_ID,
  NAS_USED_WARN,
  type NasStorageSegment,
  nasAlertCounts,
  nasAlertTone,
  nasAppsText,
  nasCpuKpi,
  nasDiskSub,
  nasDiskTempTone,
  nasFacetReason,
  nasHostFacts,
  nasHottestDisk,
  nasIfaceText,
  nasKindIcon,
  nasKindLabel,
  nasLatestAlerts,
  nasLocalDiskSummary,
  nasMemKpi,
  nasModelText,
  nasPctText,
  nasPlatformText,
  nasPoolHealth,
  nasPoolPercent,
  nasPoolSub,
  nasPoolSummary,
  nasPoolsAside,
  nasProblem,
  nasProblemNote,
  nasStaleText,
  nasStateView,
  nasTempGauge,
  nasUpdatedText,
  nasUptimeText,
  nasUsedTone,
} from "@/utils/nas";
import { computed } from "vue";
import { useRouter } from "vue-router";

const props = defineProps<{
  /** 首页显示的那个来源与其余来源（useNasHome） */
  home: HomeNas | null;
  /** GET /api/nas 的回应：能接入的类型、本机来源的开关（还没取到时为 null） */
  view: NasView | null;
  /** /api/home 与 GET /api/nas 都没取到时的原因 */
  failure: string;
  now: Date;
  /** 本机来源时的存储：首页接口的下载盘与媒体库 */
  storage: HomeBlock<HomeDisk[]> | null;
  /** 本机来源时的网络卡：下载器总速度 */
  downloaders: HomeBlock<HomeDownloaders> | null;
}>();

const isMobile = useIsMobile();
const router = useRouter();
const speed = useHomeSpeed(() => props.downloaders);

/** 数据超过这么久没更新时，更新时间改用提醒色（采集器 30 秒一拍，晚了三拍） */
const LATE_MS = 90_000;
/** 第二行最多列几个存储池、几块硬盘；多的写「还有 N 个」 */
const POOL_MAX = 5;
const DISK_MAX = 9;

const src = computed(() => props.home?.source ?? null);
const snap = computed(() => src.value?.snapshot ?? null);
const isLocal = computed(() => src.value?.id === NAS_LOCAL_ID);
const kinds = computed(() => props.view?.kinds ?? null);
const kindLabel = computed(() => (src.value ? nasKindLabel(src.value.kind, kinds.value) : ""));
const problem = computed(() => (src.value ? nasProblem(src.value.state) : false));
/** 连不上但有上一次的数据：照常列出来，柱与图标变灰 */
const stale = computed(() => problem.value && Boolean(snap.value));
const barCount = computed(() => (isMobile.value ? 16 : 30));

/* ---------- 标题行 ---------- */

const live = computed<{ text: string; tone: Tone } | null>(() => {
  const s = src.value;
  if (!s) return null;
  const state = nasStateView(s.state);
  if (!snap.value) {
    return s.state === "connecting"
      ? { text: "正在连", tone: "info" }
      : { text: state.label, tone: state.tone };
  }
  if (problem.value) {
    return { text: `${state.label} · ${nasStaleText(snap.value.at, props.now)}`, tone: "dang" };
  }
  const age = props.now.getTime() - new Date(snap.value.at).getTime();
  return { text: nasUpdatedText(snap.value.at, props.now), tone: age > LATE_MS ? "warn" : "ok" };
});

/** 首页没在显示的来源：有连不上的写几个有问题，否则写还有几个；悬停看各自的状态 */
const others = computed(() => {
  const list = props.home?.others ?? [];
  if (!list.length) return null;
  const bad = list.filter((o) => nasProblem(o.state)).length;
  return {
    text: bad ? `${bad} 个来源有问题` : `另有 ${list.length} 个来源`,
    bad: bad > 0,
    title: list
      .map((o) => {
        const kind = o.id === NAS_LOCAL_ID ? "" : `（${nasKindLabel(o.kind, kinds.value)}）`;
        return `${o.name}${kind}：${nasStateView(o.state).label}`;
      })
      .join("\n"),
  };
});

/* ---------- 空状态 ---------- */

const kindNames = computed(() => (kinds.value ?? []).map((k) => k.label));

const empty = computed(() => {
  const v = props.view;
  const parts: string[] = [];
  if (kindNames.value.length) {
    parts.push(
      `接入 ${kindNames.value.join("、")} 以后，这里显示 CPU、内存、网络、存储池、硬盘温度与告警。`,
    );
  }
  if (v && !v.local_supported) parts.push("本机来源只支持 Linux。");
  else if (v && !v.local_enabled) {
    parts.push("本机来源已关闭，打开以后这里显示 pt-tools 所在主机的 CPU、内存与负载。");
  }
  // 没有能接入的类型、本机来源又用不了时没有可做的，不放按钮
  const action = kindNames.value.length ? "接入 NAS" : v && !v.local_supported ? "" : "去 NAS 监控";
  return { sub: parts.join("") || "到 NAS 监控里接入来源。", action };
});

/* ---------- 主机行与说明 ---------- */

const hostName = computed(() =>
  isLocal.value ? "pt-tools 所在主机" : snap.value?.system.hostname || src.value?.name || "",
);
const hostTitle = computed(() => {
  const h = snap.value?.system.hostname;
  if (isLocal.value) return h ? `主机名 ${h}` : "";
  return src.value?.name ?? "";
});
const platform = computed(() => nasPlatformText(snap.value));
const meta = computed(() => {
  const s = snap.value;
  if (!s) return [];
  return [
    { icon: "cpu", text: nasModelText(s) },
    { icon: "memory-stick", text: s.memory.total > 0 ? sizeText(s.memory.total) : "" },
    { icon: "clock", text: nasUptimeText(s.system.uptime_seconds) },
  ].filter((m) => m.text);
});
const facts = computed(() => (snap.value && !isLocal.value ? nasHostFacts(snap.value) : []));
const note = computed(() =>
  src.value && problem.value ? nasProblemNote(src.value, props.now) : "",
);
const waitText = computed(() => {
  const s = src.value;
  if (!s) return "";
  return s.state === "connecting"
    ? `正在连 ${s.name}，读到数据以后显示在这里`
    : `还没有读到过 ${s.name} 的数据`;
});

/* ---------- 数值卡 ---------- */

const history = computed(() => src.value?.history ?? []);
const histAt = computed(() => history.value[history.value.length - 1]?.at);

const cpu = computed(() => (snap.value ? nasCpuKpi(snap.value) : null));
const mem = computed(() => (snap.value ? nasMemKpi(snap.value) : null));

/** 网络卡：NAS 是各网卡合计的下行与上行；本机来源读不到宿主机的网卡，换成下载器速度 */
const net = computed(() => {
  if (isLocal.value) {
    const up = `上传 ${speedText(speed.up.value)}`;
    return {
      label: "下载器速度",
      value: speed.down.value as number | null,
      aside: "本机读不到网卡",
      // 卡窄的时候放不下这句（放了只剩「本…」），整句不写；为什么不是网卡写在 NAS 监控的说明里
      asideWide: true,
      sub: `${up} · 所有下载器合计`,
      short: up,
    };
  }
  const s = snap.value;
  if (!s) return null;
  const missing = nasFacetReason(s, "network");
  const up = `上行 ${speedText(s.network.tx_bps)}`;
  return {
    label: "网络",
    value: missing ? null : s.network.rx_bps,
    aside: nasIfaceText(s),
    asideWide: false,
    sub: missing || `${up} · 所有网卡合计`,
    short: missing ? undefined : up,
  };
});
const netBars = computed(() => {
  if (isLocal.value) return speed.downBars.value;
  const v = history.value.map((h) => h.rx);
  return v.length ? v : [net.value?.value ?? 0];
});

/** 存储卡：NAS 是各池合计（可用容量）；本机来源是首页已有的下载盘与媒体库所在的盘 */
const store = computed(() => {
  if (isLocal.value) {
    const b = props.storage;
    if (!b) return { aside: "所在的盘", sum: null, missing: "加载中…" };
    if (b.error) return { aside: "所在的盘", sum: null, missing: `取不到：${b.error}` };
    const sum = nasLocalDiskSummary(b.data);
    return {
      aside: "所在的盘",
      sum,
      missing: sum.percent === null ? "不知道这些盘的总容量" : "",
    };
  }
  const s = snap.value;
  if (!s) return null;
  const pools = s.pools ?? [];
  const sum = nasPoolSummary(pools);
  const missing = nasFacetReason(s, "pools") || (sum.percent === null ? "没有读到存储池" : "");
  return { aside: pools.length ? `${pools.length} 个存储池` : "", sum, missing };
});

const cpuShown = useTweenedNumber(() => cpu.value?.value ?? 0);
const memShown = useTweenedNumber(() => mem.value?.value ?? 0);
const netShown = useTweenedNumber(() => net.value?.value ?? 0);
const storShown = useTweenedNumber(() => store.value?.sum?.percent ?? 0);

interface KpiTile {
  key: "cpu" | "mem" | "net" | "storage";
  icon: string;
  label: string;
  aside: string;
  hue: string;
  /** 大字；为 null 是这个来源读不到（sub 是原因） */
  text: string | null;
  pre?: string;
  unit?: string;
  sub: string;
  /** 卡窄的时候换成的副行（只写最要紧的一两项，不截成半句）；卡宽由 .hn-kpi 的容器查询判 */
  short?: string;
  /** 右上角那句卡窄的时候不写（放不下又截了就没意思） */
  asideWide?: boolean;
  title?: string;
  bars?: number[];
  baseline?: "range" | "zero";
  pushKey?: string;
  segments?: NasStorageSegment[];
}

/** 每张卡一个色相；连不上时全部换成灰色（数据是旧的） */
function hue(n: 1 | 2 | 3 | 4): string {
  return stale.value ? "var(--pt-t4)" : `var(--pt-kpi-${n})`;
}

function orNow(values: number[], now: number | null | undefined): number[] {
  return values.length ? values : [now ?? 0];
}

const kpis = computed<KpiTile[]>(() => {
  const c = cpu.value;
  const m = mem.value;
  const n = net.value;
  const st = store.value;
  return [
    {
      key: "cpu",
      icon: "cpu",
      label: "CPU",
      aside: c?.aside ?? "",
      hue: hue(1),
      text: c && c.value !== null ? nasPctText(cpuShown.value) : null,
      unit: "%",
      sub: c?.sub ?? "",
      short: c?.short,
      title: c?.title,
      bars:
        c && c.value !== null
          ? orNow(
              history.value.map((h) => h.cpu),
              c.value,
            )
          : undefined,
      baseline: "range",
      pushKey: histAt.value,
    },
    {
      key: "mem",
      icon: "memory-stick",
      label: "内存",
      aside: m?.aside ?? "",
      hue: hue(3),
      text: m && m.value !== null ? nasPctText(memShown.value) : null,
      unit: "%",
      sub: m?.sub ?? "",
      short: m?.short,
      bars:
        m && m.value !== null
          ? orNow(
              history.value.map((h) => h.mem),
              m.value,
            )
          : undefined,
      baseline: "range",
      pushKey: histAt.value,
    },
    {
      key: "net",
      icon: "arrow-up-down",
      label: n?.label ?? "网络",
      aside: n?.aside ?? "",
      asideWide: n?.asideWide,
      hue: hue(2),
      text: n && n.value !== null ? speedText(netShown.value) : null,
      pre: "↓",
      sub: n?.sub ?? "",
      short: n?.short,
      bars: n && n.value !== null ? netBars.value : undefined,
      baseline: "zero",
      pushKey: isLocal.value ? speed.lastAt.value : histAt.value,
    },
    {
      key: "storage",
      icon: "database",
      label: "存储",
      aside: st?.aside ?? "",
      hue: hue(4),
      text: st?.sum && st.sum.percent !== null ? nasPctText(storShown.value) : null,
      unit: "%",
      sub:
        st?.missing ||
        (st?.sum ? `已用 ${sizeText(st.sum.used)} · 剩余 ${sizeText(st.sum.free)}` : ""),
      short: !st?.missing && st?.sum ? `剩余 ${sizeText(st.sum.free)}` : undefined,
      segments: st?.sum && st.sum.percent !== null ? st.sum.segments : undefined,
    },
  ];
});

/* ---------- 第二行：存储池、硬盘、告警与应用 ---------- */

const pools = computed(() => snap.value?.pools ?? []);
const poolsReason = computed(() => (snap.value ? nasFacetReason(snap.value, "pools") : ""));
const poolsAside = computed(() => (poolsReason.value ? "" : nasPoolsAside(pools.value)));
const poolRows = computed(() =>
  pools.value.slice(0, POOL_MAX).map((p) => {
    const pct = nasPoolPercent(p);
    return {
      name: p.name,
      pct,
      tone: nasUsedTone(pct),
      health: nasPoolHealth(p),
      sub: nasPoolSub(p),
      full: pct >= NAS_USED_WARN,
    };
  }),
);

const disks = computed(() => snap.value?.disks ?? []);
const disksReason = computed(() => (snap.value ? nasFacetReason(snap.value, "disks") : ""));
const hottest = computed(() => nasHottestDisk(disks.value));
const hottestText = computed(() => {
  const d = hottest.value;
  return !d || d.temp_c === undefined ? "" : `${Math.round(d.temp_c)} °C`;
});
const diskRows = computed(() =>
  disks.value.slice(0, DISK_MAX).map((d) => ({
    name: d.name,
    temp: d.temp_c === undefined ? null : Math.round(d.temp_c),
    tone: nasDiskTempTone(d),
    gauge: d.temp_c === undefined ? 0 : nasTempGauge(d.temp_c),
    sub: nasDiskSub(d),
    title: [d.name, d.model, d.size > 0 ? sizeText(d.size) : "", d.pool]
      .filter(Boolean)
      .join(" · "),
  })),
);

const alerts = computed(() => snap.value?.alerts ?? []);
const alertsReason = computed(() => (snap.value ? nasFacetReason(snap.value, "alerts") : ""));
const counts = computed(() => nasAlertCounts(alerts.value));
const countBoxes = computed(() => [
  { key: "critical", label: "严重", n: counts.value.critical, tone: "dang" },
  { key: "warning", label: "警告", n: counts.value.warning, tone: "warn" },
  { key: "info", label: "信息", n: counts.value.info, tone: "info" },
]);
const latest = computed(() =>
  nasLatestAlerts(alerts.value, 3).map((a, i) => ({
    key: `${i}-${a.title}`,
    title: a.title,
    at: a.at ? whenText(a.at, props.now) : "",
    tone: nasAlertTone(a.level),
  })),
);
const appsReason = computed(() => (snap.value ? nasFacetReason(snap.value, "apps") : ""));
const appsText = computed(() => (snap.value ? nasAppsText(snap.value.apps) : ""));

/* ---------- 本机来源的第二行 ---------- */

const localDisks = computed(() => {
  const list = props.storage?.data ?? [];
  return isMobile.value ? mobileDisks(list) : list;
});
const localNote = computed(() => {
  const b = props.storage;
  if (!b) return "加载中…";
  if (b.error) return `取不到：${b.error}`;
  return localDisks.value.length ? "" : "没有下载盘与媒体库";
});
const localRows = computed(() =>
  localDisks.value.map((d, i) => {
    const bar = diskBar(d);
    let sub: string;
    if (d.error) sub = `取不到：${d.error}`;
    else if (d.total > 0) sub = `已用 ${sizeText(d.total - d.free)} · 共 ${sizeText(d.total)}`;
    else sub = `剩余 ${sizeText(d.free)}`;
    return {
      key: `${d.kind}-${d.path}-${i}`,
      label: d.label,
      path: d.path,
      pct: d.error ? null : bar.percent,
      tone: bar.tone,
      sub,
    };
  }),
);
const localMore = computed(() =>
  kindNames.value.length
    ? `本机来源读不到这些。接入 ${kindNames.value.join("、")} 以后，这里会显示每块硬盘的温度、存储池是否健康和 NAS 的告警。`
    : "本机来源读不到这些。",
);
</script>

<template>
  <div class="hn" :class="{ 'is-mobile': isMobile, 'is-stale': stale }" data-testid="hn">
    <HomeSection
      title="NAS 概览"
      :count="kindLabel || undefined"
      :tag="src?.experimental ? '实验性' : undefined"
      action="NAS 监控"
      to="/system/nas">
      <template #right>
        <router-link
          v-if="others && !isMobile"
          to="/system/nas"
          class="hn-others"
          :class="{ 'is-dang': others.bad }"
          :title="others.title"
          data-testid="hn-others">
          {{ others.text }}
        </router-link>
        <span v-if="live" class="hn-live" :class="`is-${live.tone}`" data-testid="hn-live">
          <i :key="snap?.at ?? 'none'" class="hn-live__dot" />
          <span class="hn-live__text">{{ live.text }}</span>
        </span>
      </template>

      <!-- 首页接口与 GET /api/nas 都没取到 -->
      <div v-if="!home && failure" class="hn-card hn-msg" data-testid="hn-error">
        <PtIcon name="triangle-alert" :size="15" class="hn-msg__icon" />
        <span>取不到 NAS 监控：{{ failure }}</span>
      </div>
      <div v-else-if="!home" class="hn-card hn-msg" data-testid="hn-loading">加载中…</div>

      <!-- 一个能显示的来源都没有 -->
      <div v-else-if="!src" class="hn-card hn-empty hn-rv" data-testid="hn-empty">
        <div class="hn-empty__icon"><PtIcon name="hard-drive" :size="24" /></div>
        <div class="hn-empty__text">
          <div class="hn-empty__title">还没有 NAS 监控来源</div>
          <div class="hn-empty__sub">{{ empty.sub }}</div>
        </div>
        <el-button
          v-if="empty.action"
          type="primary"
          class="hn-empty__btn"
          data-testid="hn-connect"
          @click="router.push('/system/nas')">
          <!-- 「接入 NAS」是去接一个来源；「去 NAS 监控」只是过去开本机来源 -->
          <PtIcon :name="kindNames.length ? 'link' : 'arrow-right'" :size="15" /><span>{{
            empty.action
          }}</span>
        </el-button>
      </div>

      <div v-else class="hn-card hn-rv" data-testid="hn-card">
        <!-- 主机行 -->
        <div class="hn-host" data-testid="hn-host">
          <div class="hn-host__logo">
            <PtIcon :name="nasKindIcon(src.kind)" :size="isMobile ? 18 : 20" />
          </div>
          <div class="hn-host__id">
            <div class="hn-host__name">
              <span class="hn-host__title" :title="hostTitle || undefined">{{ hostName }}</span>
              <span v-if="platform" class="hn-host__ver" data-testid="hn-platform">{{
                platform
              }}</span>
            </div>
            <div v-if="meta.length" class="hn-host__meta">
              <span v-for="m in meta" :key="m.icon" class="hn-host__m">
                <PtIcon :name="m.icon" :size="13" class="hn-host__mi" />{{ m.text }}
              </span>
            </div>
          </div>
          <div v-if="facts.length" class="hn-host__facts" data-testid="hn-facts">
            <PtStatusPill v-for="f in facts" :key="f.key" :tone="f.tone">
              <PtIcon :name="f.icon" :size="12" />{{ f.text }}
            </PtStatusPill>
          </div>
        </div>

        <!-- 连不上、登录失败、证书的问题 -->
        <div v-if="note" class="hn-note" role="status" data-testid="hn-note">
          <PtIcon name="circle-x" :size="15" class="hn-note__icon" />
          <span class="hn-note__text">{{ note }}</span>
          <router-link to="/system/nas" class="hn-note__link">去看看</router-link>
        </div>

        <!-- 还没读到过数据 -->
        <div v-if="!snap" class="hn-wait" data-testid="hn-wait">
          <PtIcon
            :name="src.state === 'connecting' ? 'loader-circle' : 'info'"
            :size="15"
            class="hn-wait__icon"
            :class="{ 'is-spin': src.state === 'connecting' }" />
          <span>{{ waitText }}</span>
        </div>

        <div v-else class="hn-body">
          <!-- 四张数值卡 -->
          <div class="hn-kpis">
            <div
              v-for="(k, i) in kpis"
              :key="k.key"
              class="hn-kpi hn-rv"
              :style="{ '--hue': k.hue, '--hn-i': i + 1 }"
              :data-testid="`hn-kpi-${k.key}`">
              <div class="hn-kpi__top">
                <span class="hn-kpi__ic"><PtIcon :name="k.icon" :size="isMobile ? 13 : 14" /></span>
                <span class="hn-kpi__label">{{ k.label }}</span>
                <span
                  v-if="k.aside"
                  class="hn-kpi__aside"
                  :class="{ 'is-wide': k.asideWide }"
                  :title="k.aside"
                  >{{ k.aside }}</span
                >
              </div>
              <div
                class="hn-kpi__val"
                :class="{ 'is-none': k.text === null }"
                data-testid="hn-kpi-val">
                <template v-if="k.text !== null">
                  <span v-if="k.pre" class="hn-kpi__unit">{{ k.pre }}</span>
                  <span>{{ k.text }}</span>
                  <span v-if="k.unit" class="hn-kpi__unit">{{ k.unit }}</span>
                </template>
                <template v-else>—</template>
              </div>
              <div class="hn-kpi__sub" :class="{ 'has-short': k.short }" :title="k.title || k.sub">
                <span class="hn-kpi__full">{{ k.sub }}</span>
                <span v-if="k.short" class="hn-kpi__short">{{ k.short }}</span>
              </div>
              <PtBars
                v-if="k.bars"
                class="hn-kpi__bars"
                :values="k.bars"
                :hue="k.hue"
                :count="barCount"
                :bar-width="isMobile ? 4 : 5"
                :gap="3"
                :height="isMobile ? 22 : 34"
                :baseline="k.baseline"
                :push-key="k.pushKey" />
              <template v-else-if="k.segments">
                <div class="hn-seg" data-testid="hn-seg">
                  <span
                    v-for="s in k.segments"
                    :key="s.name"
                    class="hn-seg__s"
                    :style="{ flexGrow: s.weight }"
                    :title="`${s.name} 已用 ${nasPctText(s.percent)}%`">
                    <i :style="{ width: `${s.percent}%` }" />
                  </span>
                </div>
                <div v-if="!isMobile" class="hn-seg__legend">
                  <span v-for="s in k.segments" :key="s.name">
                    <b>{{ s.name }}</b> {{ nasPctText(s.percent) }}%
                  </span>
                </div>
              </template>
              <div v-else class="hn-kpi__gap" />
            </div>
          </div>

          <!-- 第二行：本机来源 -->
          <div v-if="isLocal" class="hn-row is-local">
            <section class="hn-blk hn-rv" style="--hn-i: 5" data-testid="hn-local-storage">
              <div class="hn-blk__head">
                <PtIcon name="folder-open" :size="15" class="hn-blk__ic" />
                <span>存储</span>
                <span class="hn-blk__aside">首页现有的下载盘与媒体库</span>
              </div>
              <p v-if="localNote" class="hn-blk__note">{{ localNote }}</p>
              <div
                v-for="(d, i) in localRows"
                :key="d.key"
                class="hn-pool"
                :data-testid="`hn-disk-${i}`">
                <div class="hn-pool__top">
                  <span class="hn-pool__name" :title="`${d.label} ${d.path}`">
                    <!-- 空格放在 span 外面：span 里开头的空白会被模板编译去掉，名字和路径就连在一起了 -->
                    {{ d.label }} <span v-if="d.path" class="hn-pool__path">{{ d.path }}</span>
                  </span>
                  <span v-if="d.pct !== null" class="hn-pool__pct" :class="`is-${d.tone}`">
                    {{ nasPctText(d.pct) }}%
                  </span>
                </div>
                <PtProgress v-if="d.pct !== null" :percent="d.pct" :tone="d.tone" :height="7" />
                <div class="hn-pool__sub" :title="d.sub">{{ d.sub }}</div>
              </div>
            </section>
            <section
              class="hn-blk hn-blk--none hn-rv"
              style="--hn-i: 6"
              data-testid="hn-local-more">
              <div class="hn-blk__head">
                <PtIcon name="hard-drive" :size="15" class="hn-blk__ic" />
                <span>硬盘、存储池健康与告警</span>
              </div>
              <p class="hn-blk__note">{{ localMore }}</p>
              <router-link
                v-if="kindNames.length"
                to="/system/nas"
                class="hn-blk__cta"
                data-testid="hn-local-connect">
                接入 NAS<PtIcon name="chevron-right" :size="14" />
              </router-link>
            </section>
          </div>

          <!-- 第二行：NAS -->
          <div v-else class="hn-row">
            <section class="hn-blk hn-rv" style="--hn-i: 5" data-testid="hn-pools">
              <div class="hn-blk__head">
                <PtIcon name="database" :size="15" class="hn-blk__ic" />
                <span>存储池</span>
                <span v-if="pools.length" class="hn-blk__n">{{ pools.length }}</span>
                <span v-if="poolsAside" class="hn-blk__aside">{{ poolsAside }}</span>
              </div>
              <p v-if="poolsReason" class="hn-blk__note">{{ poolsReason }}</p>
              <p v-else-if="!pools.length" class="hn-blk__note">没有存储池</p>
              <div
                v-for="(p, i) in poolRows"
                :key="p.name"
                class="hn-pool"
                :data-testid="`hn-pool-${i}`">
                <div class="hn-pool__top">
                  <span class="hn-pool__name" :title="p.name">{{ p.name }}</span>
                  <PtStatusPill v-if="p.health" :tone="p.health.tone" dot>{{
                    p.health.label
                  }}</PtStatusPill>
                  <span class="hn-pool__pct" :class="`is-${p.tone}`">{{ nasPctText(p.pct) }}%</span>
                </div>
                <PtProgress :percent="p.pct" :tone="p.tone" :height="7" />
                <div class="hn-pool__sub">
                  {{ p.sub
                  }}<template v-if="p.full">
                    · <span :class="`is-${p.tone}`">快满了</span></template
                  >
                </div>
              </div>
              <p v-if="pools.length > poolRows.length" class="hn-blk__more">
                还有 {{ pools.length - poolRows.length }} 个存储池
              </p>
            </section>

            <section v-if="!isMobile" class="hn-blk hn-rv" style="--hn-i: 6" data-testid="hn-disks">
              <div class="hn-blk__head">
                <PtIcon name="hard-drive" :size="15" class="hn-blk__ic" />
                <span>硬盘</span>
                <span v-if="disks.length" class="hn-blk__n">{{ disks.length }}</span>
                <span v-if="hottest && !disksReason" class="hn-blk__aside">
                  最高 {{ hottestText }} · {{ hottest.name }}
                </span>
              </div>
              <p v-if="disksReason" class="hn-blk__note">{{ disksReason }}</p>
              <p v-else-if="!disks.length" class="hn-blk__note">没有硬盘</p>
              <div v-else class="hn-disks">
                <div
                  v-for="(d, i) in diskRows"
                  :key="`${d.name}-${i}`"
                  class="hn-disk"
                  :class="{ 'is-warn': d.tone === 'warn' }"
                  :title="d.title"
                  :data-testid="`hn-disk-tile-${i}`">
                  <span class="hn-disk__name">{{ d.name }}</span>
                  <div v-if="d.temp !== null" class="hn-disk__temp" :class="`is-${d.tone}`">
                    <PtIcon name="thermometer" :size="14" class="hn-disk__ti" />
                    <span>{{ d.temp }}</span
                    ><small>°C</small>
                  </div>
                  <div v-else class="hn-disk__temp is-none">
                    <PtIcon name="thermometer" :size="13" class="hn-disk__ti" />没有温度
                  </div>
                  <div class="hn-disk__gauge">
                    <i
                      v-if="d.temp !== null"
                      :class="`is-${d.tone}`"
                      :style="{ width: `${d.gauge}%` }" />
                  </div>
                  <div class="hn-disk__sub">{{ d.sub }}</div>
                </div>
              </div>
              <p v-if="disks.length > diskRows.length && !disksReason" class="hn-blk__more">
                还有 {{ disks.length - diskRows.length }} 块硬盘
              </p>
            </section>

            <section
              v-if="!isMobile"
              class="hn-blk hn-blk--alerts hn-rv"
              style="--hn-i: 7"
              data-testid="hn-alerts">
              <div class="hn-blk__head">
                <PtIcon name="bell" :size="15" class="hn-blk__ic" />
                <span>告警</span>
                <span v-if="alerts.length" class="hn-blk__n">{{ alerts.length }}</span>
                <span v-if="!alertsReason" class="hn-blk__aside">{{ kindLabel }} 里没关掉的</span>
              </div>
              <p v-if="alertsReason" class="hn-blk__note">{{ alertsReason }}</p>
              <template v-else>
                <div class="hn-acounts">
                  <div
                    v-for="c in countBoxes"
                    :key="c.key"
                    class="hn-acount"
                    :class="c.n ? `is-${c.tone}` : 'is-zero'"
                    :data-testid="`hn-acount-${c.key}`">
                    <b>{{ c.n }}</b
                    ><span>{{ c.label }}</span>
                  </div>
                </div>
                <div v-if="latest.length" class="hn-alist">
                  <div v-for="a in latest" :key="a.key" class="hn-arow" :class="`is-${a.tone}`">
                    <i class="hn-arow__dot" />
                    <span class="hn-arow__t" :title="a.title">{{ a.title }}</span>
                    <span v-if="a.at" class="hn-arow__at">{{ a.at }}</span>
                  </div>
                </div>
                <p v-else class="hn-blk__note hn-alist">没有告警</p>
              </template>
              <div class="hn-blk__fill" />
              <div class="hn-apps" data-testid="hn-apps">
                <PtIcon name="package" :size="15" class="hn-apps__ic" />
                <span>{{ appsReason ? `应用：${appsReason}` : `应用 ${appsText}` }}</span>
              </div>
            </section>
          </div>

          <!-- 手机：硬盘与告警各缩成一行 -->
          <template v-if="!isLocal && isMobile">
            <div class="hn-msum" data-testid="hn-msum-alerts">
              <PtIcon name="bell" :size="15" class="hn-msum__ic" />
              <span v-if="alertsReason">告警：{{ alertsReason }}</span>
              <span v-else-if="!alerts.length">告警：没有</span>
              <span v-else>
                告警 <b :class="{ 'is-dang': counts.critical }">{{ counts.critical }} 严重</b> ·
                {{ counts.warning }} 警告 · {{ counts.info }} 信息
              </span>
            </div>
            <div class="hn-msum" data-testid="hn-msum-disks">
              <PtIcon name="hard-drive" :size="15" class="hn-msum__ic" />
              <span v-if="disksReason">硬盘：{{ disksReason }}</span>
              <span v-else-if="!disks.length">硬盘：没有</span>
              <span v-else>
                硬盘 <b>{{ disks.length }}</b> 块<template v-if="hottest">
                  · 最高 <b>{{ hottestText }}</b
                  >（{{ hottest.name }}）</template
                ><template v-else> · 读不到温度</template>
              </span>
            </div>
          </template>
        </div>
      </div>
    </HomeSection>
  </div>
</template>

<style scoped>
.hn {
  min-width: 0;
}

/* 卡：面色、1px 发丝线、圆角 8，没有阴影（同首页别的区块卡） */
.hn-card {
  overflow: hidden;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

/* 数据到了卡片依次出现：间隔 30ms（DESIGN.md「Motion」） */
.hn-rv {
  animation: hn-rise var(--pt-dur-normal) cubic-bezier(0.2, 0, 0, 1) backwards;
  animation-delay: calc(var(--hn-i, 0) * 30ms);
}

@keyframes hn-rise {
  from {
    opacity: 0;
    transform: translateY(8px);
  }
}

/* ---------- 标题行右边 ---------- */
.hn-others {
  flex: 0 0 auto;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  text-decoration: none;
  white-space: nowrap;
}

.hn-others.is-dang {
  color: var(--pt-dang);
}

.hn-others:hover {
  text-decoration: underline;
}

.hn-live {
  display: inline-flex;
  flex: 0 1 auto;
  gap: 6px;
  align-items: center;
  min-width: 0;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
}

.hn-live__dot {
  flex: 0 0 auto;
  width: 7px;
  height: 7px;
  background: currentcolor;
  border-radius: 50%;
}

.hn-live__text {
  overflow: hidden;
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hn-live.is-ok {
  color: var(--pt-ok);
}

.hn-live.is-warn {
  color: var(--pt-warn);
}

.hn-live.is-dang {
  color: var(--pt-dang);
}

.hn-live.is-dang .hn-live__text {
  color: var(--pt-dang);
}

.hn-live.is-info {
  color: var(--pt-info);
}

.hn-live.is-neutral {
  color: var(--pt-t3);
}

/* 新的一拍到了，圆点脉冲一次（不循环） */
.hn-live.is-ok .hn-live__dot {
  animation: hn-beat var(--pt-dur-slow) ease-out 1;
}

@keyframes hn-beat {
  from {
    box-shadow: 0 0 0 0 color-mix(in srgb, currentcolor 55%, transparent);
  }

  to {
    box-shadow: 0 0 0 7px transparent;
  }
}

/* ---------- 加载、出错与空状态 ---------- */
.hn-msg {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 16px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.hn-msg__icon {
  flex: 0 0 auto;
  color: var(--pt-warn);
}

.hn-empty {
  display: flex;
  gap: 14px;
  align-items: center;
  padding: 22px 20px;
}

.hn-empty__icon {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 48px;
  height: 48px;
  color: var(--pt-p);
  background: var(--pt-p-soft);
  border-radius: 12px;
}

.hn-empty__text {
  flex: 1 1 auto;
  min-width: 0;
}

.hn-empty__title {
  font-size: var(--pt-fz-body-lg);
  font-weight: 700;
  color: var(--pt-t1);
}

.hn-empty__sub {
  margin-top: 2px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.hn-empty__btn {
  flex: 0 0 auto;
}

/* ---------- 主机行 ---------- */
.hn-host {
  display: flex;
  gap: 12px;
  align-items: center;
  padding: 14px 16px;
  border-bottom: 1px solid var(--pt-border);
}

.hn-host__logo {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 40px;
  height: 40px;
  color: var(--pt-p);
  background: var(--pt-p-soft);
  border-radius: 10px;
}

.hn-host__id {
  flex: 1 1 auto;
  min-width: 0;
}

.hn-host__name {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 8px;
  align-items: center;
  min-width: 0;
}

.hn-host__title {
  min-width: 0;
  overflow: hidden;
  font-size: var(--pt-fz-h2);
  font-weight: 700;
  line-height: 21px;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hn-host__ver {
  max-width: 100%;
  height: 20px;
  padding: 0 7px;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  font-weight: 600;
  line-height: 20px;
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

.hn-host__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 14px;
  margin-top: 3px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.hn-host__m {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  white-space: nowrap;
}

.hn-host__mi {
  color: var(--pt-t3);
}

.hn-host__facts {
  display: flex;
  flex: 0 0 auto;
  flex-wrap: wrap;
  gap: 6px;
  justify-content: flex-end;
  max-width: 45%;
}

/* ---------- 红色说明与等待 ---------- */
.hn-note {
  display: flex;
  gap: 8px;
  align-items: flex-start;
  padding: 10px 16px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t1);
  background: var(--pt-dang-soft);
  border-bottom: 1px solid color-mix(in srgb, var(--pt-dang) 25%, transparent);
}

.hn-note__icon {
  flex: 0 0 auto;
  margin-top: 1px;
  color: var(--pt-dang);
}

.hn-note__text {
  flex: 1 1 auto;
  min-width: 0;
  overflow-wrap: anywhere;
}

.hn-note__link {
  flex: 0 0 auto;
  font-weight: 500;
  color: var(--pt-p);
  text-decoration: none;
  white-space: nowrap;
}

.hn-note__link:hover {
  text-decoration: underline;
}

.hn-wait {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 18px 16px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.hn-wait__icon {
  flex: 0 0 auto;
  color: var(--pt-t3);
}

.hn-wait__icon.is-spin {
  animation: hn-spin calc(var(--pt-dur-slow) * 3) linear infinite;
}

@keyframes hn-spin {
  to {
    transform: rotate(360deg);
  }
}

/* ---------- 数值卡 ---------- */
.hn-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px 16px 16px;
}

.hn-kpis {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

/* 每张卡自己是容器：副行与右上角按卡的宽度换（手机 2×2、桌面 1024–1366 的四列都窄），不按视口 */
.hn-kpi {
  display: flex;
  container: hn-kpi / inline-size;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  padding: 12px 14px;
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
  transition: border-color var(--pt-transition-fast);
}

@media (hover: hover) {
  .hn-kpi:hover {
    border-color: color-mix(in srgb, var(--hue) 45%, var(--pt-border));
  }
}

.hn-kpi__top {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-t2);
}

.hn-kpi__ic {
  display: grid;
  flex: 0 0 auto;
  place-items: center;
  width: 24px;
  height: 24px;
  color: var(--hue);
  background: color-mix(in srgb, var(--hue) 13%, transparent);
  border-radius: 6px;
}

/* 卡名不截断（是这张卡的身份）；挤的时候截右上角那句 */
.hn-kpi__label {
  flex: none;
  white-space: nowrap;
}

.hn-kpi__aside {
  flex: 0 1 auto;
  min-width: 0;
  margin-left: auto;
  overflow: hidden;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hn-kpi__val {
  display: flex;
  gap: 4px;
  align-items: baseline;
  min-width: 0;
  margin-top: 4px;
  overflow: hidden;
  font-size: 26px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 1.1;
  color: var(--pt-t1);
  white-space: nowrap;
}

.hn-kpi__val.is-none {
  color: var(--pt-t3);
}

.hn-kpi__unit {
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t2);
}

.hn-kpi__sub {
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hn-kpi__short {
  display: none;
}

/* 卡里放不下整句（CPU 那句最长，约 210px）：副行换成短的，右上角放不下的那句不写 */
@container hn-kpi (max-width: 210px) {
  .hn-kpi__sub.has-short .hn-kpi__full {
    display: none;
  }

  .hn-kpi__short {
    display: inline;
  }

  .hn-kpi__aside.is-wide {
    display: none;
  }
}

.hn-kpi__bars {
  margin-top: 6px;
}

/* 没有柱的卡（读不到时）占住柱的高度，四张卡一样高 */
.hn-kpi__gap {
  height: 34px;
  margin-top: 6px;
}

/* 存储：各池按容量分段，段里是已用 */
.hn-seg {
  display: flex;
  gap: 3px;
  height: 12px;
  margin-top: 10px;
}

.hn-seg__s {
  position: relative;
  flex: 1 1 0;
  min-width: 6px;
  overflow: hidden;
  background: color-mix(in srgb, var(--hue) 14%, transparent);
  border-radius: 3px;
}

.hn-seg__s i {
  position: absolute;
  inset: 0 auto 0 0;
  background: linear-gradient(180deg, var(--hue), color-mix(in srgb, var(--hue) 70%, transparent));
  border-radius: 3px;
  transition: width var(--pt-transition-slow);
}

.hn-seg__legend {
  display: flex;
  flex-wrap: wrap;
  gap: 2px 10px;
  margin-top: 8px;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.hn-seg__legend b {
  font-weight: 600;
  color: var(--pt-t2);
}

/* ---------- 第二行 ---------- */
.hn-row {
  display: grid;
  grid-template-columns: minmax(0, 1.1fr) minmax(0, 1fr) minmax(0, 1fr);
  gap: 12px;
}

.hn-row.is-local {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.hn-blk {
  display: flex;
  flex-direction: column;
  min-width: 0;
  padding: 12px 14px 14px;
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.hn-blk__head {
  display: flex;
  gap: 7px;
  align-items: center;
  min-width: 0;
  margin-bottom: 8px;
  font-size: var(--pt-fz-body);
  font-weight: 700;
  color: var(--pt-t1);
}

.hn-blk__ic {
  flex: 0 0 auto;
  color: var(--pt-t3);
}

.hn-blk__n {
  flex: 0 0 auto;
  height: 18px;
  padding: 0 6px;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  line-height: 18px;
  color: var(--pt-t2);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

.hn-blk__aside {
  min-width: 0;
  margin-left: auto;
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hn-blk__note {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.hn-blk__more {
  margin: 6px 0 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 本机读不到的那一块：虚线框，canvas 底 */
.hn-blk--none {
  justify-content: center;
  background: var(--pt-canvas);
  border-style: dashed;
}

.hn-blk__cta {
  display: inline-flex;
  gap: 2px;
  align-items: center;
  align-self: flex-start;
  margin-top: 10px;
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-p);
  text-decoration: none;
}

.hn-blk__cta:hover {
  text-decoration: underline;
}

/* 存储池（本机来源的存储也用这一种行） */
.hn-pool {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 8px 0;
}

.hn-pool + .hn-pool {
  border-top: 1px dashed var(--pt-border);
}

.hn-pool__top {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
}

.hn-pool__name {
  min-width: 0;
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hn-pool__path {
  font-size: var(--pt-fz-label);
  font-weight: 400;
  color: var(--pt-t3);
}

.hn-pool__pct {
  flex: 0 0 auto;
  margin-left: auto;
  font-size: var(--pt-fz-body-lg);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t1);
}

.hn-pool__pct.is-warn,
.hn-pool__sub .is-warn {
  color: var(--pt-warn);
}

.hn-pool__pct.is-dang,
.hn-pool__sub .is-dang {
  color: var(--pt-dang);
}

.hn-pool__sub {
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 硬盘：一块盘一个方块，三列 */
.hn-disks {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.hn-disk {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  /* 左右只留 7：1440 宽时三列的方块里只有 86，「NVMe · 931 GB」才放得下 */
  padding: 8px 7px 9px;
  background: var(--pt-canvas);
  border: 1px solid transparent;
  border-radius: var(--pt-r-md);
  transition: border-color var(--pt-transition-fast);
}

@media (hover: hover) {
  .hn-disk:hover {
    border-color: var(--pt-border-strong);
  }
}

.hn-disk.is-warn {
  border-color: color-mix(in srgb, var(--pt-warn) 50%, transparent);
}

.hn-disk__name {
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  font-weight: 700;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hn-disk__temp {
  display: flex;
  gap: 3px;
  align-items: center;
  margin-top: 2px;
  font-size: var(--pt-fz-h2);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t1);
  white-space: nowrap;
}

.hn-disk__temp.is-warn {
  color: var(--pt-warn);
}

.hn-disk__temp small {
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-t2);
}

.hn-disk__temp.is-none {
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-t3);
}

.hn-disk__ti {
  flex: 0 0 auto;
  color: var(--pt-t3);
}

.hn-disk__gauge {
  height: 3px;
  margin: 2px 0 3px;
  overflow: hidden;
  background: var(--pt-border);
  border-radius: 2px;
}

.hn-disk__gauge i {
  display: block;
  height: 100%;
  background: var(--pt-ok);
  border-radius: 2px;
  transition: width var(--pt-transition-slow);
}

.hn-disk__gauge i.is-warn {
  background: var(--pt-warn);
}

.hn-disk__sub {
  overflow: hidden;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 告警：三个计数块、最新三条、应用 */
.hn-acounts {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.hn-acount {
  display: flex;
  gap: 6px;
  align-items: baseline;
  min-width: 0;
  padding: 7px 10px;
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  border-radius: var(--pt-r-md);
}

.hn-acount b {
  font-size: 20px;
  font-variant-numeric: tabular-nums;
  line-height: 1;
}

.hn-acount.is-dang {
  color: var(--pt-dang);
  background: var(--pt-dang-soft);
}

.hn-acount.is-warn {
  color: var(--pt-warn);
  background: var(--pt-warn-soft);
}

.hn-acount.is-info {
  color: var(--pt-info);
  background: var(--pt-info-soft);
}

.hn-acount.is-zero {
  color: var(--pt-t3);
  background: var(--pt-hover);
}

.hn-alist {
  display: flex;
  flex-direction: column;
  margin-top: 8px;
}

.hn-arow {
  display: flex;
  gap: 8px;
  align-items: center;
  min-height: 30px;
  font-size: var(--pt-fz-sm);
}

.hn-arow + .hn-arow {
  border-top: 1px dashed var(--pt-border);
}

.hn-arow__dot {
  flex: 0 0 auto;
  width: 6px;
  height: 6px;
  background: var(--pt-info);
  border-radius: 50%;
}

.hn-arow.is-dang .hn-arow__dot {
  background: var(--pt-dang);
}

.hn-arow.is-warn .hn-arow__dot {
  background: var(--pt-warn);
}

.hn-arow__t {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hn-arow__at {
  flex: 0 0 auto;
  font-size: var(--pt-fz-label);
  font-variant-numeric: tabular-nums;
  color: var(--pt-t3);
}

/* 应用那一行贴着这一块的底（同一行的块一样高），与上面的内容至少隔 10 */
.hn-blk__fill {
  flex: 1 1 10px;
  min-height: 10px;
}

.hn-apps {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
  padding-top: 10px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  border-top: 1px solid var(--pt-border);
}

.hn-apps__ic {
  flex: 0 0 auto;
  color: var(--pt-t3);
}

.hn-apps span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 手机的一行摘要 */
.hn-msum {
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
  padding: 10px 12px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.hn-msum span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hn-msum b {
  color: var(--pt-t1);
}

.hn-msum b.is-dang {
  color: var(--pt-dang);
}

.hn-msum__ic {
  flex: 0 0 auto;
  color: var(--pt-t3);
}

/* ---------- 内容区窄的时候 ---------- */
@container (max-width: 1080px) {
  .hn-row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .hn-blk--alerts {
    grid-column: 1 / -1;
  }
}

@container (max-width: 720px) {
  .hn-kpis {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .hn-row,
  .hn-row.is-local {
    grid-template-columns: minmax(0, 1fr);
  }

  .hn-host__facts {
    display: none;
  }
}

/* ---------- 手机 ---------- */
.hn.is-mobile .hn-host {
  padding: 12px 14px;
}

.hn.is-mobile .hn-host__logo {
  width: 34px;
  height: 34px;
  border-radius: 9px;
}

.hn.is-mobile .hn-host__meta {
  gap: 0 10px;
}

.hn.is-mobile .hn-host__facts {
  display: none;
}

.hn.is-mobile .hn-note {
  padding: 10px 14px;
}

.hn.is-mobile .hn-body {
  gap: 10px;
  padding: 12px 14px 14px;
}

.hn.is-mobile .hn-kpis {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.hn.is-mobile .hn-kpi {
  padding: 10px 12px;
}

.hn.is-mobile .hn-kpi__ic {
  width: 22px;
  height: 22px;
}

.hn.is-mobile .hn-kpi__val {
  font-size: 22px;
}

.hn.is-mobile .hn-kpi__bars {
  margin-top: 4px;
}

.hn.is-mobile .hn-kpi__gap {
  height: 22px;
  margin-top: 4px;
}

.hn.is-mobile .hn-seg {
  margin-top: 8px;
}

.hn.is-mobile .hn-row,
.hn.is-mobile .hn-row.is-local {
  grid-template-columns: minmax(0, 1fr);
}

.hn.is-mobile .hn-empty {
  flex-wrap: wrap;
  padding: 16px 14px;
}

@media (prefers-reduced-motion: reduce) {
  .hn-rv,
  .hn-live.is-ok .hn-live__dot,
  .hn-wait__icon.is-spin {
    animation: none;
  }

  .hn-kpi,
  .hn-disk,
  .hn-seg__s i,
  .hn-disk__gauge i {
    transition: none;
  }
}
</style>
