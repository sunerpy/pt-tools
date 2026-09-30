<script setup lang="ts">
import { useRouter } from "vue-router";
import {
  type AggregatedStatsResponse,
  type SiteConfig,
  type SiteLoginState,
  sitesApi,
  tasksApi,
  type TaskStatsResponse,
  userInfoApi,
} from "@/api";
import LevelTooltip from "@/components/LevelTooltip.vue";
import PtIcon from "@/components/PtIcon";
import SiteAvatar from "@/components/SiteAvatar.vue";
import PtBreakdown, { type BreakdownRow } from "@/components/ui/PtBreakdown.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtKpiBar from "@/components/ui/PtKpiBar.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtResizeHandle from "@/components/ui/PtResizeHandle.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { type ReminderTier, useLoginState } from "@/composables/useLoginState";
import { useSiteLevelsStore } from "@/stores/siteLevels";
import {
  formatBytes,
  formatDate,
  formatJoinDuration,
  formatNumber,
  formatRatio,
  formatTime,
  formatTimeAgo,
  getRatioType,
  getSiteBonusName,
  getSiteSeedingBonusName,
} from "@/utils/format";
import { ElMessage, type TableInstance } from "element-plus";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { useResizableHeight } from "@/composables/useResizableHeight";
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";

/* 「更多」菜单里那两项要跳路由 */
const router = useRouter();

const siteLevelsStore = useSiteLevelsStore();

/**
 * 六态状态机（设计文档 §5）。partial 留给「聚合成功但有站点没同步上」，由 failedSites 决定。
 *
 * 这一页**有**筛选（状态分段 + 搜索框，画板 10 的 bar-64），筛成 0 行是 zero 不是 empty。
 * 之前这里写着「这一页没有筛选」、没传 filtered —— 于是筛空时显示「还没有数据 / 同步全部」，
 * 把「筛掉了」说成「库里没有」。siteFilter / rowQuery 在下面才声明，但 filtered 是在渲染时才调用的闭包，
 * 那时它们早已初始化，不存在 TDZ。
 */
const failedSites = ref(0);
const { loading, state, errorText, run, hasPartialBanner } = useDataState({
  failed: () => failedSites.value,
  filtered: () => siteFilter.value !== "all" || rowQuery.value.trim() !== "",
});

function clearSiteFilters() {
  siteFilter.value = "all";
  rowQuery.value = "";
}
const syncing = ref(false);
const syncingSite = ref<string | null>(null);
const aggregatedStats = ref<AggregatedStatsResponse | null>(null);

/** 账户总量卡的 7 项（KPI 带里已有的总上传、平均分享率不重复），取值与格式沿用 main 那一排汇总卡 */
const totalsRows = computed(() => {
  const s = aggregatedStats.value;
  if (!s) return [];
  return [
    { label: "总下载量", value: formatBytes(s.totalDownloaded) },
    { label: "做种数", value: String(s.totalSeeding) },
    { label: "下载中", value: String(s.totalLeeching) },
    { label: "做种体积", value: formatBytes(s.totalSeederSize ?? 0) },
    { label: "总魔力值", value: formatNumber(s.totalBonus) },
    { label: "总时魔/h", value: formatNumber(s.totalBonusPerHour ?? 0) },
    { label: "总做种积分", value: formatNumber(s.totalSeedingBonus ?? 0) },
  ];
});
/** 全库任务计数，画板 KPI 六格里有三格来自这里 */
const taskStats = ref<TaskStatsResponse | null>(null);
const sitesByName = ref<Record<string, SiteConfig>>({});
const loginStates = ref<Record<string, SiteLoginState>>({});
const isMobile = useIsMobile();
const { effectiveLastActive, daysRemaining, reminderTier, tierLabel } = useLoginState(loginStates);

/**
 * 顶部那条扩展推荐可关。原先关掉只在这一次挂载里生效（换页回来又顶在站点表上方）；
 * 现在关掉后 7 天内不再出现，过期再提醒一次 —— 既不天天挤占站点表的位置，也不一关就永远看不到。
 * 活跃时间的口径说明原来也是一条常驻黄条，现在挂到「判定活跃」列头的 popover 上了。
 */
const EXT_HINT_KEY = "pt-userinfo-ext-hint-dismissed-at";
const EXT_HINT_SNOOZE_MS = 7 * 24 * 3600_000;
function extHintSnoozed(): boolean {
  try {
    const at = Number(window.localStorage.getItem(EXT_HINT_KEY));
    return Number.isFinite(at) && at > 0 && Date.now() - at < EXT_HINT_SNOOZE_MS;
  } catch {
    return false;
  }
}
const extHintOpen = ref(!extHintSnoozed());
function closeExtHint() {
  extHintOpen.value = false;
  try {
    window.localStorage.setItem(EXT_HINT_KEY, String(Date.now()));
  } catch {
    /* 存不下就只在这一次挂载里关掉 */
  }
}

type PillTone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

/* getRatioType 返回的是 el-tag 的色名，胶囊用语义名，这里换一次 */
const RATIO_TONE: Record<ReturnType<typeof getRatioType>, PillTone> = {
  success: "ok",
  info: "info",
  warning: "warn",
  danger: "dang",
};

/**
 * 积分单元格的 title：把各站自己的积分叫法与做种积分都写清楚。
 *
 * 画板 td-*-6 只有一个数字，34 的行里也塞不下单位与第二个数（实测「204.31万 魔」被裁），
 * 但这两样不能丢 —— 各站的叫法不同（魔力 / 上传量 / 花粉…），做种积分也是真实数据。
 */
function bonusTitleOf(row: { site: string; bonus?: number; seedingBonus?: number }): string {
  const parts = [`${getSiteBonusName(row.site)} ${formatNumber(row.bonus ?? 0)}`];
  const seedName = getSiteSeedingBonusName(row.site);
  if (row.seedingBonus && row.seedingBonus > 0 && seedName) {
    parts.push(`${seedName} ${formatNumber(row.seedingBonus)}`);
  }
  return parts.join(" · ");
}

function ratioTone(ratio: number): PillTone {
  return RATIO_TONE[getRatioType(ratio)];
}

/*
 * 封禁提醒档位 → 胶囊语义色。useLoginState.tierTagType 给的是 el-tag 的 type，
 * 其中 none 档返回空串（Element 的默认灰），但「正常」应该是绿的，所以这里另立一张表。
 */
const TIER_TONE: Record<ReminderTier, PillTone> = {
  none: "ok",
  "pre-warn": "primary",
  "30d": "primary",
  "14d": "warn",
  "7d": "warn",
  "3d": "dang",
  "1d": "dang",
  "banned-imminent": "dang",
  unknown: "neutral",
};

function tierTone(site: string): PillTone {
  return TIER_TONE[reminderTier(site)];
}

// 定时刷新相关
const REFRESH_INTERVAL = 5 * 60 * 1000; // 5分钟
let refreshTimer: ReturnType<typeof setInterval> | null = null;
const autoRefreshEnabled = ref(true);

/**
 * KPI 条的数据项。结构与 PtKpiBar 的 KpiItem 一致（结构化匹配，不用导出类型）。
 * 聚合接口只给当前值、没有历史序列，所以这里不传 series —— 柱子宁缺勿造。
 */
interface KpiRow {
  label: string;
  value: string;
  icon: string;
  delta?: string;
  deltaTone?: "ok" | "warn" | "dang" | "info" | "primary" | "neutral";
  /** 画板每格右侧 48×22 的柱图 */
  series?: number[];
  /** 柱图画的是什么 —— 每格含义不同，必须说出来，否则读者只能猜 */
  seriesHint?: string;
}

/**
 * 上次查看时的快照 —— 画板每格右侧那枚「变化 pill」要一个比较对象。
 *
 * 站点统计接口只回当前值、不存历史，所以站点数 / 总上传 / 平均分享率的变化只能跟
 * **上一次打开这一页时的值**比。存 localStorage，pill 上写明「较上次」，
 * 不把它说成「较昨日」—— 那是另一回事，会误导。
 */
const SNAPSHOT_KEY = "pt-tools-userinfo-snapshot-v1";

interface Snapshot {
  siteCount: number;
  totalUploaded: number;
  averageRatio: number;
  at: number;
}

/** 本次打开时读到的上一份快照；写入新快照不改它，否则第一次渲染就没得比了 */
const prevSnapshot = ref<Snapshot | null>(loadSnapshot());

function loadSnapshot(): Snapshot | null {
  try {
    const raw = localStorage.getItem(SNAPSHOT_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Snapshot;
    return typeof parsed?.siteCount === "number" ? parsed : null;
  } catch {
    return null;
  }
}

function saveSnapshot(stats: AggregatedStatsResponse) {
  try {
    localStorage.setItem(
      SNAPSHOT_KEY,
      JSON.stringify({
        siteCount: stats.siteCount,
        totalUploaded: stats.totalUploaded,
        averageRatio: stats.averageRatio,
        at: Date.now(),
      } satisfies Snapshot),
    );
  } catch {
    /* 隐私模式写不进去：pill 退回「首次」，不影响别的 */
  }
}

/** 把差值写成 pill 文案；没有可比对象时说「首次」而不是编一个 0 */
function deltaPill(
  current: number,
  previous: number | undefined,
  format: (n: number) => string,
): { delta: string; deltaTone: "ok" | "warn" | "dang" | "neutral" } {
  if (previous === undefined) return { delta: "首次", deltaTone: "neutral" };
  const diff = current - previous;
  if (diff === 0) return { delta: "持平", deltaTone: "neutral" };
  const sign = diff > 0 ? "+" : "−";
  return {
    delta: `较上次 ${sign}${format(Math.abs(diff))}`,
    deltaTone: diff > 0 ? "ok" : "warn",
  };
}

/**
 * KPI 带 —— 画板 kpibar：**6 格一行、整条 64 高**，格间 1px 竖线，
 * **每格都有一枚变化 pill 和一张 48×22 柱图**。
 *
 * 六格的口径照画板 10：站点、活跃任务、今日推送、免费种子、总上传、平均分享率。
 * 后三项里的任务口径来自 `/api/tasks/stats`（站点统计接口没有这些数）。
 *
 * 柱图的含义每格不同，都是真数据，没有一格是编的：
 *   站点数量 / 总上传量 / 平均分享率 → 按站点的构成（perSiteStats）
 *   活跃任务 / 今日推送 / 免费种子   → 最近 7 天按天（tasks/stats 的 daily）
 * pill 同理：任务那三格能跟「今天 / 昨天」比，站点那三格只能跟上次查看比。
 */
const kpiItems = computed<KpiRow[]>(() => {
  const stats = aggregatedStats.value;
  if (!stats) return [];

  const perSite = stats.perSiteStats ?? [];
  const topN = (pick: (r: (typeof perSite)[number]) => number) =>
    perSite
      .map(pick)
      .filter((n) => n > 0)
      .sort((a, b) => b - a)
      .slice(0, 8);

  const tasks = taskStats.value;
  const daily = tasks?.daily ?? [];
  const today = daily.at(-1);
  const yesterday = daily.length >= 2 ? daily.at(-2) : undefined;
  const prev = prevSnapshot.value;

  const siteCountPill = deltaPill(stats.siteCount, prev?.siteCount, (n) => String(n));
  const uploadPill = deltaPill(stats.totalUploaded, prev?.totalUploaded, (n) => formatBytes(n));
  const ratioPill = deltaPill(stats.averageRatio, prev?.averageRatio, (n) => n.toFixed(2));

  const items: KpiRow[] = [
    {
      label: "站点数量",
      value: stats.siteCount.toString(),
      icon: "globe",
      series: topN((r) => r.seeding),
      seriesHint: "每根一个站点：做种数构成",
      ...siteCountPill,
    },
    {
      label: "活跃任务",
      value: tasks ? tasks.active.toString() : "—",
      icon: "list-checks",
      series: daily.map((d) => d.created),
      seriesHint: "最近 7 天每天新入库的任务数",
      delta: today ? `今日新增 ${today.created}` : "无数据",
      deltaTone: today && today.created > 0 ? "ok" : "neutral",
    },
    {
      label: "今日推送",
      value: tasks ? tasks.pushedToday.toString() : "—",
      icon: "send",
      series: daily.map((d) => d.pushed),
      seriesHint: "最近 7 天每天推送成功的条数",
      ...(today && yesterday
        ? (() => {
            const diff = today.pushed - yesterday.pushed;
            if (diff === 0) return { delta: "与昨日持平", deltaTone: "neutral" as const };
            return {
              delta: `较昨日 ${diff > 0 ? "+" : "−"}${Math.abs(diff)}`,
              deltaTone: (diff > 0 ? "ok" : "warn") as "ok" | "warn",
            };
          })()
        : { delta: "无数据", deltaTone: "neutral" as const }),
    },
    {
      label: "免费种子",
      value: tasks ? tasks.free.toString() : "—",
      icon: "zap",
      series: daily.map((d) => d.free),
      seriesHint: "最近 7 天每天新入库的免费种子数",
      delta: today ? `今日新增 ${today.free}` : "无数据",
      deltaTone: today && today.free > 0 ? "ok" : "neutral",
    },
    {
      label: "总上传量",
      value: formatBytes(stats.totalUploaded),
      icon: "upload",
      series: topN((r) => r.uploaded),
      seriesHint: "每根一个站点：上传量构成",
      ...uploadPill,
    },
    {
      label: "平均分享率",
      value: formatRatio(stats.averageRatio),
      icon: "gauge",
      series: topN((r) => r.ratio),
      seriesHint: "每根一个站点：分享率构成",
      ...ratioPill,
    },
  ];

  // 分享率低于 1 是要动手的信号，这条比「较上次」更该占那枚 pill
  if (stats.averageRatio < 1) {
    const ratio = items.find((i) => i.label === "平均分享率");
    if (ratio) {
      ratio.delta = "偏低";
      ratio.deltaTone = "warn";
    }
  }

  return items;
});

/*
 * 画板 10 的 bar-64：seg（全部站点 / 正常 / 异常 / 保号预警）+ q（筛选站点、等级…）
 * + chip-0「周期: 本周」+ chip-1「排序: 分享率」+ 右端三枚图标钮（列设置 / 导出 / 刷新）。
 *
 * 落地此前这条带上**只有动作按钮**，一个筛选都没有 —— 而这一页是十四列的表，
 * 最需要的就是筛与排。现在补上 seg、搜索、排序 chip 与列设置、导出。
 *
 * 「周期」那枚 chip 没有落地：聚合接口只回当前快照，没有历史序列，
 * 按周期筛在数据上不成立（登记在 ALLOWED_GAPS 里）。
 */
type SiteFilter = "all" | "ok" | "bad" | "warn";

const SITE_FILTERS: { label: string; value: SiteFilter }[] = [
  { label: "全部站点", value: "all" },
  { label: "正常", value: "ok" },
  { label: "异常", value: "bad" },
  { label: "保号预警", value: "warn" },
];

const siteFilter = ref<SiteFilter>("all");
const rowQuery = ref("");

const SORT_OPTIONS = [
  { label: "分享率", value: "ratio" },
  { label: "上传量", value: "uploaded" },
  { label: "做种数", value: "seeding" },
  { label: "积分", value: "bonus" },
  { label: "站点名", value: "site" },
] as const;

type RowSortKey = (typeof SORT_OPTIONS)[number]["value"];
const rowSort = ref<RowSortKey>("ratio");

/*
 * 桌面表格的排序跟着「排序」下拉走。
 *
 * 之前表格写死了 default-sort（数据量降序），各列又是 sortable —— Element 每次拿到数据都按它自己的
 * 排序重排，于是下拉在桌面上完全没效果，默认值还和下拉的「分享率」对不上；导出 CSV 却按下拉的顺序。
 * 现在下拉一变就调表格的 sort()，点表头排序时反过来把下拉同步过去（列的 prop 与下拉的值一一对应）。
 */
const SORT_ORDER: Record<RowSortKey, "ascending" | "descending"> = {
  ratio: "descending",
  uploaded: "descending",
  seeding: "descending",
  bonus: "descending",
  site: "ascending",
};
const siteTable = ref<TableInstance>();

/*
 * 站点表的高度（用户原话「站点区域面板尺寸偏小，需要扩大；支持下拉 / 上拉拖拽方式动态调整面板高度」）。
 * 默认自适应：内容少就贴合内容，多了就正好填满首屏剩下的高度（表头吸顶、表体内部滚动）——
 * 1080 高的屏上约 20 行；不按固定比例算，是为了在矮屏上不出现「表体的滚动区伸到首屏外」的套娃滚动。
 * 拖过把手就用用户定的高度，存在本地，双击把手（或 Enter）恢复。
 *
 * 只用 max-height，不在 height / max-height 之间切换：Element 的 setHeight 收到 undefined 时什么也不做，
 * 不会清掉上一次写在表格根节点上的内联样式 —— 实测从自适应切到手动后，残留的 max-height: 883px
 * 把用户拖到的 1083 封死了。始终传数字，每次都会覆盖。手动高度因此是「最多这么高」：行少时贴合内容。
 */
const viewportH = ref(window.innerHeight);
/** 内容滚动区的可视高度，与表格在滚动区里的纵向位置（不随滚动变） */
const scrollerH = ref(window.innerHeight);
const siteTableTop = ref(0);
function measureSiteTableTop() {
  const el = siteTable.value?.$el as HTMLElement | undefined;
  const scroller = el?.closest(".pt-shell__content");
  if (!el || !scroller) return;
  scrollerH.value = scroller.clientHeight;
  siteTableTop.value =
    el.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop;
}
/* 首屏里表格下面还要放：把手 14 + 页脚带 34 + 余量 16 */
const siteTableAutoMax = computed(() =>
  Math.max(320, Math.round(scrollerH.value - siteTableTop.value - 14 - 34 - 16)),
);
const siteTableBounds = () => ({ min: 200, max: Math.max(900, Math.round(viewportH.value * 1.5)) });
const {
  manual: siteTableManual,
  set: setSiteTableHeight,
  reset: resetSiteTableHeight,
} = useResizableHeight("pt-userinfo-sites-height-v1", siteTableBounds);
/** 表格此刻的实际高度：把手从这里开始算拖了多少 */
const siteTableHeight = ref(0);
let siteTableObserver: ResizeObserver | null = null;
watch(
  () => siteTable.value?.$el as HTMLElement | undefined,
  (el) => {
    siteTableObserver?.disconnect();
    if (!el || typeof ResizeObserver === "undefined") return;
    siteTableObserver = new ResizeObserver(() => {
      siteTableHeight.value = el.offsetHeight;
      measureSiteTableTop();
    });
    siteTableObserver.observe(el);
    siteTableHeight.value = el.offsetHeight;
    measureSiteTableTop();
  },
);
/* 乐观更新：先把把手读到的高度改成新值，ResizeObserver 之后按实际渲染校正（见 PtResizeHandle 的注释） */
function onSiteTableResize(h: number) {
  setSiteTableHeight(h, false);
  siteTableHeight.value = h;
}

function onViewportResize() {
  viewportH.value = window.innerHeight;
  measureSiteTableTop();
}
/* 扩展提示关掉后表格上移，首屏剩下的高度跟着变 */
watch(extHintOpen, () => void nextTick(measureSiteTableTop));
watch(rowSort, (key) => siteTable.value?.sort(key, SORT_ORDER[key]));

function onTableSortChange({ prop, order }: { prop: string; order: string | null }) {
  if (order && (SORT_OPTIONS as readonly { value: string }[]).some((o) => o.value === prop)) {
    rowSort.value = prop as RowSortKey;
  }
}

const USERINFO_COLS_KEY = "pt-tools-userinfo-cols-v1";

const OPTIONAL_USERINFO_COLS = [
  { key: "trueData", label: "真实数据" },
  { key: "seedSize", label: "做种体积" },
  { key: "bonus", label: "积分" },
  { key: "bph", label: "时魔/h" },
  { key: "inbound", label: "入站" },
  { key: "active", label: "判定活跃" },
  { key: "days", label: "剩余天数" },
  { key: "updated", label: "更新" },
] as const;

type OptionalUserinfoCol = (typeof OPTIONAL_USERINFO_COLS)[number]["key"];

/*
 * 没设过列偏好的用户，默认先收起三列低频列：真实数据（多数站点不报，整列是「-」）、入站日期、剩余天数
 * （保号预警另有「需要关注的站点」卡与「保号预警」分段）。用户原话「表格列数量过多」——
 * 14 列 min-width 合计约 1780，1920 宽也要横滚；收起后 11 列，与画板 10 的十列 + 操作一致。
 * 这三列仍在「列设置」里，勾上就回来；已经存过偏好的用户按他们自己的来。
 */
const DEFAULT_HIDDEN_USERINFO_COLS: OptionalUserinfoCol[] = ["trueData", "inbound", "days"];

function loadHiddenUserinfoCols(): Set<OptionalUserinfoCol> {
  try {
    const raw = window.localStorage.getItem(USERINFO_COLS_KEY);
    if (!raw) return new Set(DEFAULT_HIDDEN_USERINFO_COLS);
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return new Set();
    const known = new Set(OPTIONAL_USERINFO_COLS.map((c) => c.key as string));
    return new Set(parsed.filter((k): k is OptionalUserinfoCol => known.has(k)));
  } catch {
    return new Set();
  }
}

const hiddenUserinfoCols = ref<Set<OptionalUserinfoCol>>(loadHiddenUserinfoCols());

function toggleUserinfoCol(key: OptionalUserinfoCol) {
  const next = new Set(hiddenUserinfoCols.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  hiddenUserinfoCols.value = next;
  try {
    window.localStorage.setItem(USERINFO_COLS_KEY, JSON.stringify([...next]));
  } catch {
    /* 存不下就只在本次会话里生效 */
  }
}

const colShown = (key: OptionalUserinfoCol) => !hiddenUserinfoCols.value.has(key);

/** 「更多」菜单里的四个低频动作 —— 与原来那四枚按钮是同一个动作，不是另一套逻辑 */
function onMoreCommand(cmd: string) {
  if (cmd === "autoRefresh") return toggleAutoRefresh();
  if (cmd === "clearCache") return clearCache();
  if (cmd === "openAll") return openAllSites();
  if (cmd === "supported") return void router.push("/supported-sites");
}

/**
 * 导出当前筛选出的这批站点（画板 bar-64 的 bi-file-down）。
 *
 * 列固定按画板那十列的口径导，不跟着「列设置」变：列设置是**看**的偏好，
 * 而导出是拿数据走，少一列就是少一份信息。
 */
function exportCsv() {
  const head = [
    "站点",
    "等级",
    "上传",
    "下载",
    "分享率",
    "做种",
    "做种体积",
    "积分",
    "时魔/h",
    "更新时间",
  ];
  const lines = [head.join(",")];
  for (const r of siteRows.value) {
    const cells = [
      r.site,
      r.levelName || r.rank || "",
      formatBytes(r.uploaded),
      formatBytes(r.downloaded),
      formatRatio(r.ratio),
      String(r.seeding ?? 0),
      formatBytes(r.seederSize ?? 0),
      String(r.bonus ?? 0),
      String(r.bonusPerHour ?? 0),
      r.lastUpdate ? new Date(r.lastUpdate * 1000).toLocaleString("zh-CN", { hour12: false }) : "",
    ];
    lines.push(cells.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(","));
  }
  const blob = new Blob([`\uFEFF${lines.join("\n")}`], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `pt-tools-userinfo-${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
  ElMessage.success(`已导出 ${siteRows.value.length} 个站点`);
}

const allSiteRows = computed(() => aggregatedStats.value?.perSiteStats ?? []);

/**
 * 表格里这一批行 = 状态筛 + 搜索 + 排序。
 *
 * 状态的口径都用本页已有的数据：
 *   异常      站点最近一次没能取到数据（unread/ratio 之类还在，但 lastUpdate 为 0）
 *             或者保号档位已经到了 3 天内 / 即将封禁；
 *   保号预警  保号档位在 30 天 / 14 天 / 7 天这几档；
 *   正常      其余。
 */
const siteRows = computed(() => {
  let rows = allSiteRows.value;
  if (siteFilter.value !== "all") {
    rows = rows.filter((r) => {
      const tier = reminderTier(r.site);
      const warn = tier === "pre-warn" || tier === "30d" || tier === "14d" || tier === "7d";
      const bad = tier === "3d" || tier === "1d" || tier === "banned-imminent" || !r.lastUpdate;
      if (siteFilter.value === "bad") return bad;
      if (siteFilter.value === "warn") return warn && !bad;
      return !bad && !warn;
    });
  }
  const q = rowQuery.value.trim().toLowerCase();
  if (q) {
    rows = rows.filter(
      (r) =>
        r.site.toLowerCase().includes(q) ||
        (r.rank ?? "").toLowerCase().includes(q) ||
        (r.levelName ?? "").toLowerCase().includes(q),
    );
  }
  const key = rowSort.value;
  return [...rows].sort((a, b) => {
    if (key === "site") return a.site.localeCompare(b.site);
    const av =
      key === "ratio"
        ? a.ratio
        : key === "uploaded"
          ? a.uploaded
          : key === "seeding"
            ? a.seeding
            : (a.bonus ?? 0);
    const bv =
      key === "ratio"
        ? b.ratio
        : key === "uploaded"
          ? b.uploaded
          : key === "seeding"
            ? b.seeding
            : (b.bonus ?? 0);
    return bv - av;
  });
});

/**
 * 画板 10 在 gfoot 之后还有三张分析卡：p-up 548（上传构成）、p-dist 516（等级分布）、
 * p-watch 1080（需要关注的站点）。三张卡的数据全部来自已经拿到的 perSiteStats，
 * 不额外请求接口 —— 聚合接口没有历史序列，所以这里画的是「当前的构成」而不是趋势。
 */

/** p-up：上传量最大的几个站点占了多少 */
const uploadRows = computed<BreakdownRow[]>(() => {
  const rows = [...siteRows.value].sort((a, b) => b.uploaded - a.uploaded).slice(0, 6);
  return rows.map((r) => ({
    key: r.site,
    label: r.site,
    value: formatBytes(r.uploaded),
    weight: r.uploaded,
    tone: "primary" as const,
    hint: `分享率 ${formatRatio(r.ratio)} · 做种 ${r.seeding}`,
  }));
});

const uploadTotal = computed(() => siteRows.value.reduce((n, r) => n + r.uploaded, 0));

/** p-dist：等级分布。等级名是站点各自的说法，按名字归组就是画板那张分布 */
const levelRows = computed<BreakdownRow[]>(() => {
  const buckets = new Map<string, number>();
  for (const r of siteRows.value) {
    const name = r.levelName || r.rank || "未知等级";
    buckets.set(name, (buckets.get(name) ?? 0) + 1);
  }
  return [...buckets.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, n]) => ({
      key: name,
      label: name,
      value: n,
      tone: "info" as const,
    }));
});

/**
 * p-watch：需要动手的站点。
 * 判定口径写在 hint 里，不让用户猜为什么这一条被列出来。
 */
const watchRows = computed<BreakdownRow[]>(() => {
  const out: BreakdownRow[] = [];
  for (const r of siteRows.value) {
    const reasons: string[] = [];
    if (r.ratio > 0 && r.ratio < 1) reasons.push(`分享率 ${formatRatio(r.ratio)} 低于 1`);
    if ((r.unreadMessageCount ?? 0) > 0) reasons.push(`${r.unreadMessageCount} 条未读站内信`);
    if (r.seeding === 0) reasons.push("一个种都没做");
    if (reasons.length === 0) continue;
    out.push({
      key: r.site,
      label: r.site,
      value: reasons.length,
      weight: reasons.length,
      tone: r.ratio > 0 && r.ratio < 1 ? "dang" : "warn",
      hint: reasons.join(" · "),
    });
  }
  return out.sort((a, b) => Number(b.value) - Number(a.value));
});

/*
 * 本页没有摘要行 —— 画板 10 的主区顶上是 KPI 带而不是 head（App.vue 的 KPI_TOP_ROUTES），
 * 那 6 格真实数字本身就是摘要。之前这里往 #pt-head-sub 打了一个 Teleport，
 * 而那个靶子在没有 head 的页面上根本不存在，摘要落了空。
 */

/** 状态块副标题：失败时给真实错误，空态时给下一步动作 */
const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  if (state.value === "zero") return "当前筛选下没有站点，放宽状态筛选或清空搜索";
  return "同步任意站点后这里会出现统计";
});

/**
 * 加载数据。
 *
 * 聚合统计是这一页的主数据，拿不到就没有任何东西可显示 —— 它失败走 error/perm。
 * 站点配置和登录态是装饰性的补充（用来给出站点链接和保号提醒），单独失败不该
 * 把整页判死，所以它们各自 catch 掉降级为空；但要计入 failedSites，
 * 页面会挂一条「部分数据没拿到」的提示，而不是假装一切正常。
 */
async function loadData() {
  failedSites.value = 0;
  const agg = await run(() => userInfoApi.getAggregated());
  if (!agg) {
    aggregatedStats.value = null;
    return;
  }
  aggregatedStats.value = agg;
  /* 写新快照，但不动本次读到的 prevSnapshot —— 否则这次渲染就没得比了 */
  saveSnapshot(agg);

  const [siteMap, states, stats] = await Promise.all([
    sitesApi.list().catch(() => {
      failedSites.value += 1;
      return {} as Record<string, SiteConfig>;
    }),
    sitesApi.listLoginStates().catch(() => {
      failedSites.value += 1;
      return [] as SiteLoginState[];
    }),
    /*
     * 任务计数（画板 KPI 的「活跃任务 / 今日推送 / 免费种子」三格）。
     * 拿不到就让那三格显示 —— 站点统计本身还在，不该因为任务计数失败整页降级。
     */
    tasksApi.stats().catch(() => {
      failedSites.value += 1;
      return null;
    }),
  ]);
  taskStats.value = stats;

  sitesByName.value = siteMap;

  const byName: Record<string, SiteLoginState> = {};
  for (const st of states ?? []) byName[st.site_name] = st;
  loginStates.value = byName;
}

function openSite(site: string) {
  const url =
    sitesByName.value[site]?.web_url ??
    sitesByName.value[site]?.urls?.[0] ??
    loginStates.value[site]?.base_url;
  if (url) window.open(url, "_blank", "noopener");
}

function openAllSites() {
  const rows = aggregatedStats.value?.perSiteStats ?? [];
  const urls = rows
    .map(
      (row) =>
        sitesByName.value[row.site]?.web_url ??
        sitesByName.value[row.site]?.urls?.[0] ??
        loginStates.value[row.site]?.base_url,
    )
    .filter((url): url is string => Boolean(url));

  if (urls.length === 0) {
    ElMessage.info("没有可打开的站点");
    return;
  }

  if (urls.length === 1) {
    window.open(urls[0], "_blank");
    return;
  }

  let opened = 0;
  let blocked = 0;
  for (const url of urls) {
    const w = window.open(url, "_blank");
    if (w === null || typeof w === "undefined") {
      blocked++;
    } else {
      opened++;
    }
  }

  if (blocked > 0) {
    ElMessage({
      type: "warning",
      duration: 8000,
      showClose: true,
      message: `已打开 ${opened} 个站点，浏览器拦截了其余 ${blocked} 个。请在浏览器地址栏允许本站“弹出式窗口”后重试，或使用每行的“打开站点”按钮逐个打开。`,
    });
  } else {
    ElMessage.success(`已打开 ${opened} 个站点`);
  }
}

// 同步所有站点
async function syncAll() {
  syncing.value = true;
  try {
    const result = await userInfoApi.syncAll();
    if (result.failed && result.failed.length > 0) {
      ElMessage.warning(`同步完成: ${result.success.length} 成功, ${result.failed.length} 失败`);
    } else {
      ElMessage.success(`同步完成: ${result.success.length} 个站点`);
    }
    await loadData();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "同步失败");
  } finally {
    syncing.value = false;
  }
}

// 同步单个站点
async function syncSite(siteId: string) {
  syncingSite.value = siteId;
  try {
    await userInfoApi.syncSite(siteId);
    ElMessage.success(`站点 ${siteId} 同步成功`);
    await loadData();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "同步失败");
  } finally {
    syncingSite.value = null;
  }
}

// 清除缓存
async function clearCache() {
  try {
    await userInfoApi.clearCache();
    ElMessage.success("缓存已清除");
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "清除缓存失败");
  }
}

// 启动定时刷新
function startAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer);
  }
  refreshTimer = setInterval(() => {
    if (!loading.value && !syncing.value) {
      loadData();
    }
  }, REFRESH_INTERVAL);
}

// 停止定时刷新
function stopAutoRefresh() {
  if (refreshTimer) {
    clearInterval(refreshTimer);
    refreshTimer = null;
  }
}

// 切换自动刷新
function toggleAutoRefresh() {
  autoRefreshEnabled.value = !autoRefreshEnabled.value;
  if (autoRefreshEnabled.value) {
    startAutoRefresh();
    ElMessage.success("已开启自动刷新");
  } else {
    stopAutoRefresh();
    ElMessage.info("已关闭自动刷新");
  }
}

// 检查是否有 H&R 数据
function hasHnR(row: any): boolean {
  return (
    (row.hnrUnsatisfied && row.hnrUnsatisfied > 0) || (row.hnrPreWarning && row.hnrPreWarning > 0)
  );
}

onMounted(() => {
  window.addEventListener("resize", onViewportResize);
  loadData();
  // 预加载所有站点的等级信息
  siteLevelsStore.loadAll();
  if (autoRefreshEnabled.value) {
    startAutoRefresh();
  }
});

onUnmounted(() => {
  stopAutoRefresh();
  window.removeEventListener("resize", onViewportResize);
  siteTableObserver?.disconnect();
});
</script>

<template>
  <div class="dash">
    <!--
      KPI 条 —— 画板 kpibar 328,0 1112×64：主区最顶上的一条全宽白带，
      6 格之间是 1px 竖线。它替代页头出现在总览页，所以不再包在卡片里。
    -->
    <PtKpiBar v-if="kpiItems.length" :items="kpiItems" :cols="6" band />

    <!--
      提示放在顶部带**之后** —— 画板 27（系统设置）就是这个落法：head 0..64，
      提示在 y=80、左右内缩 16。放在带之前会把 KPI 带从主区顶上顶下来，
      而画板里 KPI 带是贴着 y=0 的。
    -->
    <!--
      partial（设计文档 §5）：聚合统计拿到了，但站点配置或登录态没拿到。
      既不该整页报错，也不该假装正常 —— 统计照常显示，这里说清少了什么，
      免得用户以为「打开站点」按钮和保号提醒坏了。
    -->
    <div v-if="hasPartialBanner(siteRows.length)" class="pt-note pt-note--warn dash__note">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>
        部分数据没拿到（{{ failedSites }} 项）：站点链接与保号提醒可能不完整，统计数字不受影响。
      </span>
      <button type="button" class="dash__note-x" aria-label="重新加载" @click="loadData">
        <PtIcon name="refresh-cw" :size="14" />
      </button>
    </div>

    <div v-if="extHintOpen" class="pt-note dash__note">
      <PtIcon name="info" :size="14" class="pt-note__icon" />
      <span>
        推荐安装
        <a href="https://github.com/sunerpy/pt-tools/releases" target="_blank" rel="noopener">
          PT Tools Helper 浏览器扩展
        </a>
        ：自动同步 Cookie、一键采集新站点数据。
        <a
          href="https://github.com/sunerpy/pt-tools/blob/main/docs/guide/request-new-site.md"
          target="_blank"
          rel="noopener">
          了解更多
        </a>
      </span>
      <button type="button" class="dash__note-x" aria-label="关闭提示" @click="closeExtHint">
        <PtIcon name="x" :size="14" />
      </button>
    </div>

    <!-- 站点详情表格：画板 grid 是全宽平铺的带，不是圆角描边卡片 -->
    <PtToolbar band>
      <!--
        画板 10 的 bar-64：seg（全部站点 / 正常 / 异常 / 保号预警）+ q + 排序 chip。
        这条带此前只有动作按钮、一个筛选都没有 —— 而这是一张十四列的表。
      -->
      <el-segmented
        v-model="siteFilter"
        class="pt-seg"
        :options="SITE_FILTERS"
        :props="{ label: 'label', value: 'value' }"
        data-testid="userinfo-status-seg" />
      <el-input
        v-model="rowQuery"
        class="ui-q"
        size="small"
        placeholder="筛选站点、等级…"
        clearable
        data-testid="userinfo-search">
        <template #prefix>
          <PtIcon name="search" :size="15" />
        </template>
      </el-input>
      <el-select v-model="rowSort" class="ui-chip" size="small" data-testid="userinfo-sort">
        <el-option
          v-for="o in SORT_OPTIONS"
          :key="o.value"
          :label="`排序: ${o.label}`"
          :value="o.value" />
      </el-select>

      <template #right>
        <!-- 画板 bar-64 右端的 bi-columns-3：十四列里哪几列要看，自己定 -->
        <el-popover placement="bottom-end" trigger="click" :width="180">
          <template #reference>
            <button
              type="button"
              class="pt-band__iconbtn"
              aria-label="列设置"
              data-testid="userinfo-cols-btn">
              <PtIcon name="columns-3" :size="15" />
            </button>
          </template>
          <div class="ui-cols">
            <label v-for="c in OPTIONAL_USERINFO_COLS" :key="c.key" class="ui-cols__row">
              <el-checkbox :model-value="colShown(c.key)" @change="toggleUserinfoCol(c.key)" />
              <span>{{ c.label }}</span>
            </label>
          </div>
        </el-popover>

        <!-- 画板 bar-64 右端的 bi-file-down -->
        <el-tooltip content="按当前筛选导出 CSV" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            aria-label="导出"
            data-testid="userinfo-export-btn"
            @click="exportCsv">
            <PtIcon name="file-down" :size="15" />
          </button>
        </el-tooltip>

        <!--
          画板 10 的 bar-64 右端只有三枚图标钮，这一页那六个动作按钮在画板上没有位置
          （这一页用 KPI 带替代了页头，没有页头动作区）。全都平铺的结果是它们和左边的
          筛选控件在 1112 里撞在一起 —— 实测「排序」下拉压住了「自动刷新中」。
          所以低频的四个收进一个「更多」菜单，留下「导出分享」与主操作「同步全部」。
        -->
        <el-dropdown trigger="click" @command="onMoreCommand">
          <el-button size="small" data-testid="userinfo-more-btn">
            <PtIcon name="ellipsis" :size="14" /><span>更多</span>
          </el-button>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="autoRefresh">
                <PtIcon name="timer" :size="14" class="dd-ico" />
                <span>{{ autoRefreshEnabled ? "关闭自动刷新" : "开启自动刷新（5 分钟）" }}</span>
              </el-dropdown-item>
              <el-dropdown-item command="clearCache">
                <PtIcon name="trash-2" :size="14" class="dd-ico" /><span>清除缓存</span>
              </el-dropdown-item>
              <el-dropdown-item command="openAll" :disabled="!siteRows.length">
                <PtIcon name="external-link" :size="14" class="dd-ico" /><span>一键打开站点</span>
              </el-dropdown-item>
              <el-dropdown-item command="supported">
                <PtIcon name="list" :size="14" class="dd-ico" /><span>已支持站点</span>
              </el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>

        <el-button size="small" type="info" @click="$router.push('/userinfo/export')">
          <PtIcon name="share-2" :size="14" />
          <span>导出分享</span>
        </el-button>
        <el-button type="primary" size="small" :loading="syncing" @click="syncAll">
          <PtIcon v-if="!syncing" name="refresh-cw" :size="14" />
          <span>同步全部</span>
        </el-button>
      </template>
    </PtToolbar>

    <div v-loading="loading" class="pt-band--grid">
      <!-- 桌面端表格视图 -->
      <!--
        行高回到画板 10 的 34（owner 拍板：用画板的密集网格，但保留头像、未读角标，
        以及真实数据 / 入站 / 剩余天数 / 操作四列）。
        为了在 34 里装下这些，单元格按画板的形状收成单行：
          · 头像 32 → 20，角标跟着缩一档并仍然贴头像右上角向左生长（不会被行盒裁掉）；
          · 站点名单行，用户名移进 title（画板这一列本来就是纯文本，等级另有一列）；
          · 数据量 / 真实数据 从上下两行改成画板那样的一行「↑38.4 TB ↓4.2 TB」；
          · 积分按画板只显示数字，叫法与做种积分移进 title。
        十四列在 1112 宽里放不下（min-width 合计 1764），所以这张表是**横向可滚**的。
        `scrollbar-always-on` 是必须的：Element 默认只在悬停时显出滚动条，
        实测右边那六列（积分 / 时魔 / 入站 / 判定活跃 / 剩余天数 / 更新）看不出还有内容 ——
        「能滚」和「看得出能滚」是两件事。
      -->
      <template v-if="!isMobile">
        <el-table
          ref="siteTable"
          class="pt-grid"
          :data="siteRows"
          :max-height="siteTableManual ?? siteTableAutoMax"
          scrollbar-always-on
          style="width: 100%"
          :default-sort="{ prop: rowSort, order: SORT_ORDER[rowSort] }"
          highlight-current-row
          @sort-change="onTableSortChange">
          <!-- 站点列：带消息徽章和悬停效果 -->
          <el-table-column
            prop="site"
            label="站点"
            min-width="160"
            sortable
            fixed="left"
            class-name="pt-cell-overflow">
            <template #default="{ row }">
              <div class="site">
                <el-badge
                  :value="row.unreadMessageCount"
                  :hidden="!row.unreadMessageCount || row.unreadMessageCount === 0"
                  :max="99"
                  type="danger">
                  <button
                    type="button"
                    class="site__av"
                    :aria-label="`同步 ${row.site}`"
                    @click.stop="syncSite(row.site)">
                    <SiteAvatar :site-name="row.site" :site-id="row.site" :size="20" />
                    <PtIcon
                      v-if="syncingSite === row.site"
                      name="loader-circle"
                      :size="14"
                      class="site__spin" />
                  </button>
                </el-badge>
                <!--
                画板这一列是纯文本单行。用户名不丢：挂在 title 上，鼠标停住就能看到 ——
                34 的行盒放不下第二行，而等级、积分这些本来就各有自己的列。
              -->
                <span
                  class="site__name"
                  :title="row.username ? `${row.site} · ${row.username}` : row.site">
                  {{ row.site }}
                </span>
              </div>
            </template>
          </el-table-column>

          <!-- 等级列 -->
          <!--
          画板这一列是纯文本 78 宽；落地是胶囊（带边框与内距），最长的等级名
          「Extreme User」在 124 里会被裁成「Extreme User ..」，所以放到 140。
        -->
          <el-table-column prop="rank" label="等级" min-width="140" align="center">
            <template #default="{ row }">
              <LevelTooltip
                :site-id="row.site"
                :current-level-name="row.levelName || row.rank || '-'"
                :current-level-id="row.levelId" />
            </template>
          </el-table-column>

          <!-- 上传/下载量：双行布局带图标 -->
          <el-table-column
            prop="uploaded"
            label="数据量"
            min-width="170"
            sortable
            align="right"
            class-name="pt-cell-num">
            <template #default="{ row }">
              <!--
              画板 td-*-2 的形状是一行「↑38.4 TB ↓4.2 TB」，箭头是**字符**不是图标 ——
              收成单行之后 12px 的图标加上两个数就撑不住 170，数字会折行成「52.1 / TB」。
            -->
              <div class="io">
                <span class="io__r is-up">↑{{ formatBytes(row.uploaded) }}</span>
                <span class="io__r is-dn">↓{{ formatBytes(row.downloaded) }}</span>
              </div>
            </template>
          </el-table-column>

          <!-- 真实数据（如果不同） -->
          <el-table-column
            v-if="colShown('trueData')"
            prop="trueUploaded"
            label="真实数据"
            min-width="170"
            sortable
            align="right"
            class-name="pt-cell-num">
            <template #default="{ row }">
              <div v-if="row.trueUploaded && row.trueUploaded !== row.uploaded" class="io">
                <span class="io__r is-up">↑{{ formatBytes(row.trueUploaded) }}</span>
                <span class="io__r is-dn">↓{{ formatBytes(row.trueDownloaded ?? 0) }}</span>
              </div>
              <span v-else class="nil">-</span>
            </template>
          </el-table-column>

          <!-- 分享率 -->
          <!--
          画板 10 的分享率单元是**纯文本** 13/400（td-0-3「9.14」），不是胶囊。
          十四列的表里每格都套一个胶囊，读起来是一片色块而不是一列数字 ——
          所以去掉壳、留下语义色：分享率低到危险时仍然靠字色示警，只是不再画框。
        -->
          <el-table-column prop="ratio" label="分享率" min-width="90" sortable align="center">
            <template #default="{ row }">
              <span class="num-tone" :class="`is-${ratioTone(row.ratio)}`">
                {{ formatRatio(row.ratio) }}
              </span>
            </template>
          </el-table-column>

          <!-- 做种数 + H&R -->
          <el-table-column prop="seeding" label="做种" min-width="110" sortable align="center">
            <template #default="{ row }">
              <div class="seed">
                <!-- 画板 td-0-4「286」是纯文本；做种数本身不是状态，不需要胶囊 -->
                <span class="num-tone">{{ row.seeding }}</span>
                <div v-if="hasHnR(row)" class="hnr">
                  <el-tooltip
                    v-if="row.hnrPreWarning > 0"
                    :content="`H&R 预警: ${row.hnrPreWarning}`">
                    <span class="hnr__i is-warn">
                      <PtIcon name="triangle-alert" :size="12" />{{ row.hnrPreWarning }}
                    </span>
                  </el-tooltip>
                  <el-tooltip
                    v-if="row.hnrUnsatisfied > 0"
                    :content="`H&R 未满足: ${row.hnrUnsatisfied}`">
                    <span class="hnr__i is-dang">
                      <PtIcon name="circle-x" :size="12" />{{ row.hnrUnsatisfied }}
                    </span>
                  </el-tooltip>
                </div>
              </div>
            </template>
          </el-table-column>

          <!-- 做种体积 -->
          <el-table-column
            v-if="colShown('seedSize')"
            prop="seederSize"
            label="做种体积"
            min-width="110"
            sortable
            align="right"
            class-name="pt-cell-num">
            <template #default="{ row }">
              <span class="is-ok">{{ formatBytes(row.seederSize ?? 0) }}</span>
            </template>
          </el-table-column>

          <!-- 魔力值 + 做种积分 -->
          <el-table-column
            v-if="colShown('bonus')"
            prop="bonus"
            label="积分"
            min-width="140"
            sortable
            align="right"
            class-name="pt-cell-num">
            <template #default="{ row }">
              <!--
              画板 td-*-6 只有一个数字（1,284,900）。各站的积分叫法与做种积分两个数都还在，
              但挪进 title —— 塞在 34 的行里会把数字挤掉（实测「204.31万 魔」被裁）。
            -->
              <span class="bonus" :title="bonusTitleOf(row)">
                {{ formatNumber(row.bonus ?? 0) }}
              </span>
            </template>
          </el-table-column>

          <!-- 时魔 -->
          <el-table-column
            v-if="colShown('bph')"
            prop="bonusPerHour"
            label="时魔/h"
            min-width="100"
            sortable
            align="right"
            class-name="pt-cell-num">
            <template #default="{ row }">
              <span class="is-warn">{{ formatNumber(row.bonusPerHour ?? 0) }}</span>
            </template>
          </el-table-column>

          <!-- 注册时间 -->
          <el-table-column
            v-if="colShown('inbound')"
            prop="joinDate"
            label="入站"
            min-width="110"
            sortable
            align="center"
            class-name="pt-cell-muted">
            <template #default="{ row }">
              <el-tooltip v-if="row.joinDate" :content="formatDate(row.joinDate)" placement="top">
                <span class="ts">{{ formatJoinDuration(row.joinDate) }}</span>
              </el-tooltip>
              <span v-else class="nil">-</span>
            </template>
          </el-table-column>

          <!--
          判定活跃：口径说明挂在**列头**上，不再占页面顶部一整条。
          原来那条常驻黄色提示有三行、约 90px，每次打开这一页都得先读它 ——
          而画板的页面板上没有任何常驻横幅，画板 45 给这类说明的形态正是列上的 popover
          （那张板上「落地要点 · LevelTooltip」就是同一个套路）。说明一个字没删。
        -->
          <el-table-column
            v-if="colShown('active')"
            min-width="110"
            align="center"
            class-name="pt-cell-muted">
            <template #header>
              <el-popover placement="top" :width="330" trigger="hover">
                <template #reference>
                  <span class="th-help th-help--warn">
                    判定活跃
                    <PtIcon name="triangle-alert" :size="12" />
                  </span>
                </template>
                <p class="th-help__p">
                  活跃时间通过 cookie/API 探测获取，可刷新多数站点的 last_access（最近动向）以保号；
                  但少数站点按 last_login（实际登录）或做种活跃度清理，此类站点仍需定期手动登录，
                  请勿仅依赖此处数据。
                </p>
              </el-popover>
            </template>
            <template #default="{ row }">
              <span class="ts">{{ formatTimeAgo(effectiveLastActive(row.site)) }}</span>
            </template>
          </el-table-column>

          <!-- 封禁提醒 -->
          <el-table-column v-if="colShown('days')" min-width="150" align="center">
            <template #header>
              <el-tooltip content="距离站点封禁阈值的剩余天数；负数表示已超过阈值" placement="top">
                <span class="th-help">剩余天数</span>
              </el-tooltip>
            </template>
            <template #default="{ row }">
              <div class="days">
                <span class="days__v">
                  {{ daysRemaining(row.site) === null ? "—" : `${daysRemaining(row.site)} 天` }}
                </span>
                <PtStatusPill :tone="tierTone(row.site)" size="sm">
                  {{ tierLabel(reminderTier(row.site)) }}
                </PtStatusPill>
              </div>
            </template>
          </el-table-column>

          <!-- 更新时间 -->
          <el-table-column
            v-if="colShown('updated')"
            prop="lastUpdate"
            label="更新"
            min-width="100"
            sortable
            align="center"
            class-name="pt-cell-muted">
            <template #default="{ row }">
              <el-tooltip :content="formatTime(row.lastUpdate)" placement="top">
                <span class="ts">{{ formatTimeAgo(row.lastUpdate) }}</span>
              </el-tooltip>
            </template>
          </el-table-column>

          <!-- 操作列 -->
          <!--
            操作列收成两枚图标钮（打开站点 / 同步），150 → 88 宽：列已经很多，操作列又是 fixed="right"，
            它越宽、横滚时压住的内容越多。文字挪进 tooltip 与 aria-label，键盘与读屏照旧可用。
          -->
          <el-table-column
            label="操作"
            width="88"
            align="center"
            fixed="right"
            class-name="pt-cell-act">
            <template #default="{ row }">
              <div class="acts">
                <el-tooltip
                  :content="
                    sitesByName[row.site]?.urls?.[0] || loginStates[row.site]?.base_url
                      ? '打开站点'
                      : '未配置站点地址'
                  "
                  placement="top">
                  <span>
                    <el-button
                      link
                      type="primary"
                      :aria-label="`打开 ${row.site}`"
                      :disabled="
                        !sitesByName[row.site]?.urls?.[0] && !loginStates[row.site]?.base_url
                      "
                      :data-testid="`userinfo-open-site-${row.site}`"
                      @click="openSite(row.site)">
                      <PtIcon name="external-link" :size="15" />
                    </el-button>
                  </span>
                </el-tooltip>
                <el-tooltip content="同步这个站点" placement="top">
                  <el-button
                    link
                    type="primary"
                    :aria-label="`同步 ${row.site}`"
                    :loading="syncingSite === row.site"
                    @click="syncSite(row.site)">
                    <PtIcon v-if="syncingSite !== row.site" name="refresh-cw" :size="15" />
                  </el-button>
                </el-tooltip>
              </div>
            </template>
          </el-table-column>
          <template #empty>
            <PtDataState :state="state" :sub="stateSub">
              <template v-if="state === 'error'" #action>
                <el-button size="small" @click="loadData">
                  <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
                </el-button>
              </template>
              <template v-else-if="state === 'zero'" #action>
                <el-button size="small" @click="clearSiteFilters">
                  <PtIcon name="x" :size="14" /><span>清空筛选</span>
                </el-button>
              </template>
              <template v-else-if="state === 'empty'" #action>
                <el-button type="primary" size="small" :loading="syncing" @click="syncAll">
                  <PtIcon v-if="!syncing" name="refresh-cw" :size="14" />
                  <span>同步全部</span>
                </el-button>
              </template>
            </PtDataState>
          </template>
        </el-table>
        <PtResizeHandle
          v-if="siteRows.length > 0"
          :height="siteTableHeight"
          :min="siteTableBounds().min"
          :max="siteTableBounds().max"
          label="站点表高度"
          @resize="onSiteTableResize"
          @commit="(h) => setSiteTableHeight(h)"
          @reset="resetSiteTableHeight" />
      </template>

      <!--
        移动端卡片视图：13 列的表格在手机上横向滚动没法用，所以 <768px 换成一站一卡。
        显隐由 v-if 控制而不是 CSS，两套视图不会同时挂在 DOM 上。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!siteRows.length" :state="state" :sub="stateSub">
          <template v-if="state === 'error'" #action>
            <el-button size="small" @click="loadData">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
          <template v-else-if="state === 'zero'" #action>
            <el-button size="small" @click="clearSiteFilters">
              <PtIcon name="x" :size="14" /><span>清空筛选</span>
            </el-button>
          </template>
          <template v-else-if="state === 'empty'" #action>
            <el-button type="primary" size="small" :loading="syncing" @click="syncAll">
              <PtIcon v-if="!syncing" name="refresh-cw" :size="14" />
              <span>同步全部</span>
            </el-button>
          </template>
        </PtDataState>

        <article v-for="row in siteRows" :key="row.site" class="card">
          <header class="card__head">
            <div class="site">
              <el-badge
                :value="row.unreadMessageCount"
                :hidden="!row.unreadMessageCount || row.unreadMessageCount === 0"
                :max="99"
                type="danger">
                <button
                  type="button"
                  class="site__av"
                  :aria-label="`同步 ${row.site}`"
                  @click.stop="syncSite(row.site)">
                  <SiteAvatar :site-name="row.site" :site-id="row.site" :size="40" />
                  <PtIcon
                    v-if="syncingSite === row.site"
                    name="loader-circle"
                    :size="16"
                    class="site__spin" />
                </button>
              </el-badge>
              <div class="site__txt">
                <span class="site__name">{{ row.site }}</span>
                <span class="site__user">{{ row.username }}</span>
              </div>
            </div>
            <LevelTooltip
              :site-id="row.site"
              :current-level-name="row.levelName || row.rank || '-'"
              :current-level-id="row.levelId" />
          </header>

          <div class="card__io">
            <div class="io">
              <span class="io__r is-up">
                <PtIcon name="upload" :size="13" />{{ formatBytes(row.uploaded) }}
              </span>
              <span class="io__r is-dn">
                <PtIcon name="download" :size="13" />{{ formatBytes(row.downloaded) }}
              </span>
            </div>
            <PtStatusPill :tone="ratioTone(row.ratio)">{{ formatRatio(row.ratio) }}</PtStatusPill>
          </div>

          <div class="card__stats">
            <div class="stat">
              <span class="stat__l">
                <PtIcon name="star" :size="11" />{{ getSiteBonusName(row.site) }}
              </span>
              <span class="stat__v is-warn">{{ formatNumber(row.bonus ?? 0) }}</span>
            </div>
            <div class="stat">
              <span class="stat__l"><PtIcon name="timer" :size="11" />时魔/h</span>
              <span class="stat__v is-warn">{{ formatNumber(row.bonusPerHour ?? 0) }}</span>
            </div>
            <div class="stat">
              <span class="stat__l"><PtIcon name="share-2" :size="11" />做种</span>
              <span class="stat__v">
                {{ row.seeding }}
                <PtIcon
                  v-if="(row.hnrUnsatisfied ?? 0) > 0"
                  name="circle-x"
                  :size="12"
                  class="is-dang" />
              </span>
            </div>
            <div class="stat">
              <span class="stat__l"><PtIcon name="hard-drive" :size="11" />体积</span>
              <span class="stat__v is-ok">{{ formatBytes(row.seederSize ?? 0) }}</span>
            </div>
            <div
              v-if="row.seedingBonus && row.seedingBonus > 0 && getSiteSeedingBonusName(row.site)"
              class="stat">
              <span class="stat__l"><PtIcon name="medal" :size="11" />做种积分</span>
              <span class="stat__v is-ok">{{ formatNumber(row.seedingBonus) }}</span>
            </div>
            <div class="stat">
              <span class="stat__l"><PtIcon name="calendar" :size="11" />入站</span>
              <span class="stat__v">{{ formatJoinDuration(row.joinDate ?? 0) }}</span>
            </div>
            <!--
              口径说明在移动端的入口。桌面挂在「判定活跃」列头的 popover 上，
              而移动端是行卡、没有列头 —— 一次评审查出这段说明对移动用户**完全不可达**，
              判得对。这里给一个 ⓘ，点/聚焦都能打开同一段话。
            -->
            <div class="stat">
              <span class="stat__l">
                <PtIcon name="clock" :size="11" />判定活跃
                <el-popover placement="top" :width="300" trigger="click">
                  <template #reference>
                    <button
                      type="button"
                      class="stat__why"
                      aria-label="活跃时间口径说明"
                      @click.stop>
                      <PtIcon name="info" :size="11" />
                    </button>
                  </template>
                  <p class="th-help__p">
                    活跃时间通过 cookie/API 探测获取，可刷新多数站点的
                    last_access（最近动向）以保号； 但少数站点按
                    last_login（实际登录）或做种活跃度清理，
                    此类站点仍需定期手动登录，请勿仅依赖此处数据。
                  </p>
                </el-popover>
              </span>
              <span class="stat__v">{{ formatTimeAgo(effectiveLastActive(row.site)) }}</span>
            </div>
            <div class="stat">
              <span class="stat__l"><PtIcon name="triangle-alert" :size="11" />剩余天数</span>
              <span class="stat__v">
                {{ daysRemaining(row.site) === null ? "—" : `${daysRemaining(row.site)} 天` }}
              </span>
              <PtStatusPill :tone="tierTone(row.site)" size="sm">
                {{ tierLabel(reminderTier(row.site)) }}
              </PtStatusPill>
            </div>
          </div>

          <footer class="card__foot">
            <span class="ts card__ts">
              <PtIcon name="clock" :size="12" />{{ formatTimeAgo(row.lastUpdate) }}
            </span>
            <div class="acts">
              <el-tooltip
                content="未配置站点地址"
                placement="top"
                :disabled="!!(sitesByName[row.site]?.urls?.[0] || loginStates[row.site]?.base_url)">
                <span>
                  <el-button
                    size="small"
                    :disabled="
                      !sitesByName[row.site]?.urls?.[0] && !loginStates[row.site]?.base_url
                    "
                    :data-testid="`userinfo-open-site-${row.site}`"
                    @click="openSite(row.site)">
                    <PtIcon name="external-link" :size="14" /><span>打开站点</span>
                  </el-button>
                </span>
              </el-tooltip>
              <el-button
                type="primary"
                size="small"
                :loading="syncingSite === row.site"
                @click="syncSite(row.site)">
                <PtIcon v-if="syncingSite !== row.site" name="refresh-cw" :size="14" />
                <span>同步</span>
              </el-button>
            </div>
          </footer>
        </article>
      </div>
    </div>

    <!-- 画板 gfoot 34：表格下方的说明带，左侧是统计口径与更新时间 -->
    <div v-if="aggregatedStats" class="pt-band--foot">
      <span>{{ siteRows.length }} 个站点</span>
      <span class="pt-band__spacer" />
      <span>最后更新 {{ formatTime(aggregatedStats.lastUpdate) }}</span>
    </div>

    <!--
      画板 10 的分析卡：p-up 548 / p-dist 516 两栏 + p-watch 1080 通栏。
      三张都由 perSiteStats 现算，没有额外请求。
    -->
    <div v-if="siteRows.length > 0" class="pt-cards pt-cards--2">
      <PtPanel title="上传构成" icon="upload" :count="formatBytes(uploadTotal)">
        <PtBreakdown
          :rows="uploadRows"
          :total="uploadTotal"
          foot="柱长是该站点上传量占全部站点上传量的比例，只列前 6 个。" />
      </PtPanel>

      <PtPanel title="等级分布" icon="award" :count="`${levelRows.length} 种`">
        <PtBreakdown
          :rows="levelRows"
          :total="siteRows.length"
          foot="等级名沿用各站自己的叫法，同名的归成一组。" />
      </PtPanel>

      <PtPanel
        class="pt-cards__full"
        title="需要关注的站点"
        icon="bell-ring"
        :count="watchRows.length > 0 ? `${watchRows.length} 个` : '暂无'">
        <PtBreakdown
          v-if="watchRows.length > 0"
          :rows="watchRows"
          cols
          foot="判定口径：分享率低于 1、有未读站内信、或者一个种都没做。" />
        <p v-else class="dash__ok">所有站点的分享率都在 1 以上、没有未读站内信、也都在做种。</p>
      </PtPanel>

      <!--
        账户总量。KPI 带按画板 10 换成了 6 项（站点、活跃任务、今日推送、免费种子、总上传、平均分享率），
        main 上那一排汇总里的另外 7 个数 —— 总下载、做种数、下载中、总魔力、时魔/h、做种积分、做种体积 ——
        于是在整页上都看不到了（下载中全站都没有）。按 owner 定的原则「用画板的样式，保留现有数据」，
        把它们收成一张通栏卡；KPI 带里已有的两项不重复。
      -->
      <PtPanel
        v-if="totalsRows.length > 0"
        class="pt-cards__full"
        title="账户总量"
        icon="sigma"
        :count="`${aggregatedStats?.siteCount ?? 0} 个站点合计`">
        <dl class="dash__totals" data-testid="userinfo-totals">
          <div v-for="t in totalsRows" :key="t.label" class="dash__total">
            <dt>{{ t.label }}</dt>
            <dd>{{ t.value }}</dd>
          </div>
        </dl>
      </PtPanel>
    </div>
  </div>
</template>

<style scoped>
/* 账户总量：标签 11/500 t3 在上、数值 15/600 t1 在下，等宽数字；宽度够就一行排开 */
.dash__totals {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(132px, 1fr));
  gap: var(--pt-space-3) var(--pt-space-4);
  margin: 0;
}

.dash__total dt {
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: var(--pt-t3);
}

.dash__total dd {
  margin: 2px 0 0;
  font-size: 15px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t1);
}

/* 需要关注的站点一个都没有时的正面结论，不摆一张空卡 */
.dash__ok {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

/*
 * 这一页有两套视图：≥768px 走 el-table（皮肤在全局 .pt-grid），<768px 走卡片。
 * 两边刻意复用同一批单元格类名（.site / .io / .bonus / .acts …），
 * 改一处颜色两边一起变，不用维护两份语义色。
 */
/*
 * 画板主区是一串全宽横向带（KPI 条 → 工具栏 → 表格 → 页脚带），带与带之间没有间距，
 * 所以这里不再给 gap；需要内缩的东西（提示、移动端卡片）自己带 16 的留白。
 */
.dash {
  display: flex;
  flex-direction: column;
}

/* 提示不是全宽带：画板里它们在卡片层，左右各内缩 16 */
.dash__note {
  margin: var(--pt-pad) var(--pt-pad) 0;
}

/* 两条可关提示：.pt-note 默认居中对齐，这里是多行文案，图标要贴顶 */
.dash__note {
  align-items: flex-start;
}

.dash__note a {
  font-weight: 600;
  color: var(--pt-p);
  text-decoration: none;
}

.dash__note a:hover {
  text-decoration: underline;
}

.dash__note-x {
  flex-shrink: 0;
  padding: 0;
  margin-left: auto;
  color: var(--pt-t3);
  cursor: pointer;
  background: none;
  border: 0;
}

.dash__note-x:hover {
  color: var(--pt-t2);
}

/* 画板 10 的 bar-64：q 220 宽、排序 chip、列设置面板 */
.ui-q {
  flex: 0 1 220px;
}

.ui-chip {
  flex: 0 0 auto;
  width: 150px;
}

.ui-chip :deep(.el-select__wrapper) {
  min-height: 24px;
  padding: 0 8px;
  font-size: var(--pt-fz-label);
}

.ui-cols {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.ui-cols__row {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  font-size: var(--pt-fz-sm);
  cursor: pointer;
}

/* ---- 站点单元格（表格 + 卡片头共用） ---- */
.site {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  min-width: 0;
}

/*
 * 未读消息角标压在头像右上角内侧。
 *
 * Element 默认把角标整个甩到头像外（right 偏移再 translateX(100%)），两位数就有
 * 十几像素探到间距外面，直接盖住站点名；上沿也要多出 9px，在表格里被 .cell 裁掉 ——
 * 「站点有消息角标时展示不全」就是这么来的（提交 266625b）。
 * 改成贴住头像右上角、向左生长（去掉 translateX，只留右偏移）：
 *   right -2  右沿只探出头像 2px，角标再宽也是往左长进头像里，
 *             永远吃不到和站点名之间那 8px 间距 —— 居中锚定会随位数左右扩，
 *             「99+」那种宽度足以横跨整个头像；
 *   top 2     行高回到画板的 34 之后头像收到 20，角标贴头像右上角。
 *             实测 top:0 时角标上沿比单元格上边还高 1px（会被 .cell 裁掉 1px），
 *             所以往下压 2 —— 这个数是在真实浏览器里量出来的，不是推的：
 *             改完实测 clippedTop 为负（完全在单元格内）。
 *   14/10     跟着头像收一档，否则 20 的头像上顶着 16 的角标就成了角标带头像。
 */
.site :deep(.el-badge__content.is-fixed) {
  top: 2px;
  right: -2px;
  height: 14px;
  padding: 0 4px;
  font-size: var(--pt-fz-foot);
  line-height: 14px;
  transform: translateY(-50%);
}

/* 头像本身就是「同步本站」的按钮，所以是真 button，键盘可达 */
.site__av {
  position: relative;
  display: block;
  padding: 0;
  cursor: pointer;
  background: none;
  border: 0;
  border-radius: var(--pt-radius-full);
}

.site__av:hover {
  opacity: 0.75;
}

.site__av:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

/* 同步中的转圈：不再借 el-icon.is-loading，自己转 */
.site__spin {
  position: absolute;
  top: 50%;
  left: 50%;
  color: var(--pt-p);
  transform: translate(-50%, -50%);
  animation: dash-spin 1s linear infinite;
}

@keyframes dash-spin {
  to {
    transform: translate(-50%, -50%) rotate(360deg);
  }
}

/*
 * 双行只留给**移动端行卡**的卡头（那里头像 40、空间够）。
 * 桌面表格按画板是单行，用户名挂在 title 上 —— 34 的行盒放不下第二行。
 */
.site__txt {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
}

.site__user {
  overflow: hidden;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.site__name {
  overflow: hidden;
  font-weight: 600;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* ---- 上传/下载：画板 td-*-2 是**一行**「↑38.4 TB ↓4.2 TB」 ---- */
.io {
  display: flex;
  flex-wrap: nowrap;
  gap: var(--pt-space-2);
  justify-content: flex-end;
  align-items: flex-end;
}

.io__r {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  font-variant-numeric: tabular-nums;
}

.io__r.is-up {
  color: var(--pt-ok);
}

.io__r.is-dn {
  color: var(--pt-dang);
}

/* 缺值统一用最弱的文字色，避免和真实数字抢注意力 */
.nil {
  color: var(--pt-t3);
}

/*
 * 表里的数字：画板的单元是纯文本 13/400 #4E5765（td-0-*），所以默认就是正文色。
 * 语义只落在**字色**上，不再画胶囊 —— 分享率危险时字变红，正常时和别的数字一样安静。
 * 这是「去掉壳、留下信号」：十四列每格一个色块的版本，远看是一片颜色不是一张表。
 */
.num-tone {
  font-weight: 400;
  color: var(--pt-t2);
}

.num-tone.is-ok {
  color: var(--pt-ok);
}

.num-tone.is-warn {
  font-weight: 500;
  color: var(--pt-warn);
}

.num-tone.is-dang {
  font-weight: 500;
  color: var(--pt-dang);
}

/* info 档（分享率偏低但不危险）不改色：它不是要示警，改色反而抢注意力 */
.num-tone.is-info {
  color: var(--pt-t2);
}

/* ---- 做种数 + H&R ---- */
.seed {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
  justify-content: flex-end;
}

.hnr {
  display: inline-flex;
  gap: 6px;
  align-items: center;
}

.hnr__i {
  display: inline-flex;
  gap: 2px;
  align-items: center;
  font-size: var(--pt-fz-label);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  cursor: help;
}

.is-ok {
  color: var(--pt-ok);
}

.is-warn {
  color: var(--pt-warn);
}

.is-dang {
  color: var(--pt-dang);
}

/* ---- 积分：值在上、单位名在下 ---- */
/* 积分也收成一行：「魔力 · 做种积分」两个数都留着，34 的行盒放不下第二行 */
.bonus {
  display: flex;
  flex-wrap: nowrap;
  gap: var(--pt-space-2);
  align-items: baseline;
  justify-content: flex-end;
}

.bonus__r {
  display: inline-flex;
  gap: 4px;
  align-items: baseline;
}

.bonus__v {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--pt-warn);
}

.bonus__r.is-seed .bonus__v {
  color: var(--pt-ok);
}

.bonus__l {
  font-size: var(--pt-fz-foot);
  color: var(--pt-t3);
}

/* ---- 时间戳与表头提示 ---- */
.ts {
  font-variant-numeric: tabular-nums;
}

.th-help {
  display: inline-flex;
  gap: 3px;
  align-items: center;
  cursor: help;
  border-bottom: 1px dotted var(--pt-t4);
}

/*
 * 行卡统计里的 ⓘ：触控目标按 §9 的「≥ 44×44」给足（用负外边距抵消，不撑高这一行），
 * 视觉上只有 11 的图标。它是移动端读到「活跃时间口径」的唯一入口，不能只做成 hover。
 */
.stat__why {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 44px;
  height: 44px;
  margin: -16px -14px;
  color: var(--pt-t3);
  cursor: pointer;
  background: none;
  border: 0;
  border-radius: var(--pt-r-sm);
}

.stat__why:hover,
.stat__why:focus-visible {
  color: var(--pt-p);
}

/* 带告警口径的列头：图标用 warn 色，让「这一列有前提」在表头就看得见 */
.th-help--warn :deep(svg) {
  color: var(--pt-warn);
}

/* popover 里的说明段：13/1.6，比表格正文松一档，长句才读得下去 */
.th-help__p {
  margin: 0;
  font-size: var(--pt-fz-body);
  font-weight: 400;
  line-height: 1.6;
  color: var(--pt-t2);
  text-align: left;
}

/* 「21 天」与状态胶囊一行排开：之前列被压窄时数字和「天」断成两行，胶囊还被操作列压住 */
.days {
  display: inline-flex;
  flex-wrap: nowrap;
  gap: var(--pt-space-2);
  align-items: center;
  white-space: nowrap;
}

.days__v {
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t1);
}

.acts {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
}

/*
 * 表格操作列的图标钮给足 28×28 的点击区（link 按钮默认只有图标那么大），悬停补一块底色；
 * 两枚 28 的钮之间 4 就够。只限表格：手机行卡的 .card__foot 也用 .acts，那里是
 * 「打开站点 / 同步」两枚带字按钮，套上 28 宽会被压扁（发布扫描在 768 / 375 下抓到过 26<47 的裁切）。
 */
.pt-grid .acts {
  gap: var(--pt-space-1);
}

.pt-grid .acts :deep(.el-button) {
  width: 28px;
  height: 28px;
  margin: 0;
  border-radius: var(--pt-r-sm);
}

.pt-grid .acts :deep(.el-button:not(.is-disabled):hover) {
  background: var(--pt-hover);
}

/* ---- 移动端卡片 ---- */
.cards {
  display: flex;
  flex-direction: column;
  /* 卡间距跟画板 30 / 31 一样是 8；这里原来是 12，全站十二个行卡列表里只有它一个不一样 */
  gap: var(--pt-space-2);
  padding: var(--pt-pad) 0;
}

.card {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-3);
  padding: var(--pt-space-3);
  background: var(--pt-raised);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

.card__head {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  justify-content: space-between;
}

/* 上传/下载 + 分享率一行：卡片里最想先看到的就是这三个数 */
.card__io {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  justify-content: space-between;
  padding-top: var(--pt-space-3);
  border-top: 1px solid var(--pt-border);
}

.card__io .io {
  align-items: flex-start;
  font-size: var(--pt-fz-sm);
}

.card__stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--pt-space-3) var(--pt-space-2);
}

.stat {
  display: flex;
  flex-direction: column;
  gap: 2px;
  align-items: flex-start;
  min-width: 0;
}

.stat__l {
  display: inline-flex;
  gap: 3px;
  align-items: center;
  overflow: hidden;
  font-size: var(--pt-fz-foot);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 100%;
}

.stat__v {
  display: inline-flex;
  gap: 3px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t1);
}

.card__foot {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  justify-content: space-between;
  padding-top: var(--pt-space-3);
  border-top: 1px solid var(--pt-border);
}

.card__ts {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  font-size: var(--pt-fz-foot);
  color: var(--pt-t3);
}

/* 窄屏三列会把「做种积分」这类长标签挤成两行，退成两列 */
@media (max-width: 480px) {
  .card__stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
