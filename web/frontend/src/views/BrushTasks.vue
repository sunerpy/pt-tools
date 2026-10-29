<script setup lang="ts">
import {
  type BrushStatsResponse,
  type BrushTask,
  type BrushTaskConfig,
  type BrushTorrent,
  type BrushTorrentState,
  brushApi,
  type DownloaderSetting,
  downloadersApi,
  type SiteConfig,
  sitesApi,
} from "@/api";
import BrushRunsDrawer from "@/components/brush/BrushRunsDrawer.vue";
import BrushTrendChart, { type TrendPoint } from "@/components/brush/BrushTrendChart.vue";
import DownloaderPathInput from "@/components/downloader/DownloaderPathInput.vue";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtKpiBar from "@/components/ui/PtKpiBar.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import {
  BRUSH_DISCOUNTS,
  BRUSH_SECTIONS,
  brushSectionFor,
  configOf,
  defaultBrushConfig,
  discountLabel,
  hasBrushSection,
  limitSummary,
  nextRunAt,
  removalSummary,
  runMessage,
  taskDiscounts,
} from "@/utils/brush";
import {
  BRUSH_PERIODS,
  type BrushPeriod,
  brushPeriodLong,
  brushPeriodMeta,
  brushPeriodShort,
  loadBrushPeriod,
  saveBrushPeriod,
} from "@/utils/brushPeriods";
import { formatBytes, formatRatio, formatShortDateTime } from "@/utils/format";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onMounted, reactive, ref, watch } from "vue";

/*
 * 刷流任务页（路线图 M3）：一个任务 = 一个站点的免费列表 + 一个下载器 + 入场条件 + 删种规则 + 限额。
 * 任务默认关闭；开启后后台按间隔一轮轮运行（scheduler/brush_monitor.go），这一页只做配置与查看。
 * 每一轮的步骤在「运行记录」抽屉里（BrushRunsDrawer），「上次运行」那格点了也打开它。
 */

const isMobile = useIsMobile();

const tasks = ref<BrushTask[]>([]);
const stats = ref<BrushStatsResponse | null>(null);
const sites = ref<Record<string, SiteConfig>>({});
const downloaders = ref<DownloaderSetting[]>([]);
/** 站点、下载器这两个次要数据源没取到的个数；任务表本身取到了就算 partial */
const sideFailed = ref(0);
/** 收益统计没取到（单独记：换周期时会重新读它） */
const statsFailed = ref(false);

type StatusFilter = "all" | "on" | "off";
const STATUS_SEG: { label: string; value: StatusFilter }[] = [
  { label: "全部", value: "all" },
  { label: "开启", value: "on" },
  { label: "关闭", value: "off" },
];
const statusFilter = ref<StatusFilter>("all");
const query = ref("");
const filterOn = computed(() => statusFilter.value !== "all" || Boolean(query.value.trim()));

const { loading, state, errorText, run, isStale, hasPartialBanner } = useDataState({
  failed: () => sideFailed.value + (statsFailed.value ? 1 : 0),
  filtered: () => filterOn.value,
});

const visibleTasks = computed(() => {
  const q = query.value.trim().toLowerCase();
  return tasks.value.filter((t) => {
    if (statusFilter.value === "on" && !t.enabled) return false;
    if (statusFilter.value === "off" && t.enabled) return false;
    if (!q) return true;
    return t.name.toLowerCase().includes(q) || t.site_name.toLowerCase().includes(q);
  });
});

function clearFilters() {
  statusFilter.value = "all";
  query.value = "";
}

const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  if (state.value === "zero") return "当前筛选下没有任务，放宽筛选或清空它";
  return "建一个任务：选站点和下载器，定好入场条件与删种规则。任务默认关闭，确认之后再开启";
});
const stateAction = computed(() => {
  if (state.value === "error") return "retry";
  if (state.value === "zero") return "clear";
  if (state.value === "empty") return "create";
  return "none";
});

const enabledCount = computed(() => tasks.value.filter((t) => t.enabled).length);
const headSub = computed(() => {
  if (!tasks.value.length) return "从站点的免费列表里挑种子做种，按规则删除";
  return `${tasks.value.length} 个任务 · 开启 ${enabledCount.value} 个 · ${period.value.current}上传 ${formatBytes(curUp.value)}`;
});

/* ------------------------------------------------------------------ 统计周期 */

/*
 * 统计周期：日 / 周 / 月 / 季 / 年（2026-10-28 用户要按这几种范围看）。它管这一页所有的收益数字：
 * KPI 带（本期上传、下载与最近 7 段的柱子）、任务表里「本期 上传 / 下载」那一列和下面的收益图，
 * 所以放在页头，在它们所有人的上面。选过的记在这台浏览器里。
 */
const periodValue = ref<BrushPeriod>(loadBrushPeriod());
/** 正在读的统计周期（发请求用） */
const requested = computed(() => brushPeriodMeta(periodValue.value));
/**
 * 页面上显示的数字是哪个周期的：按统计自己的 granularity 写叫法。换周期时新的一份还在路上，
 * 图与数字保持上一份、叫法也还是上一份的（不会出现「本周上传」底下是今天的数）；没读到统计时按选的周期。
 */
const shownValue = computed<BrushPeriod>(() =>
  stats.value ? brushPeriodMeta(stats.value.granularity).value : periodValue.value,
);
const period = computed(() => brushPeriodMeta(shownValue.value));
/** 收益统计正在按新的周期重新读：页头的统计图标转起来，数字不压暗（压暗会让文字的对比度不够） */
const statsLoading = ref(false);
let statsSeq = 0;

async function loadStats() {
  const seq = ++statsSeq;
  statsLoading.value = true;
  try {
    const res = await brushApi.stats(requested.value.value, requested.value.periods);
    if (seq !== statsSeq) return;
    stats.value = res;
    statsFailed.value = false;
  } catch {
    if (seq !== statsSeq) return;
    stats.value = null;
    statsFailed.value = true;
  } finally {
    if (seq === statsSeq) statsLoading.value = false;
  }
}

watch(periodValue, (v) => {
  saveBrushPeriod(v);
  void loadStats();
});

async function loadAll() {
  const pending = run(async () => {
    const [taskRes, siteRes, dlRes] = await Promise.allSettled([
      brushApi.list(),
      sitesApi.list(),
      downloadersApi.list(),
      loadStats(),
    ]);
    if (taskRes.status === "rejected") throw taskRes.reason;
    return {
      tasks: taskRes.value,
      sites: siteRes.status === "fulfilled" ? siteRes.value : null,
      downloaders: dlRes.status === "fulfilled" ? dlRes.value : null,
      failed: [siteRes, dlRes].filter((r) => r.status === "rejected").length,
    };
  });
  const data = await pending;
  if (isStale(pending)) return;
  if (!data) {
    tasks.value = [];
    return;
  }
  tasks.value = data.tasks ?? [];
  if (data.sites) sites.value = data.sites;
  if (data.downloaders) downloaders.value = data.downloaders;
  sideFailed.value = data.failed;
}

onMounted(() => void loadAll());

/* ------------------------------------------------------------------ KPI 与收益 */

const totals = computed(() => stats.value?.totals ?? []);
/** 当前这一段与上一段（读不到统计时为空） */
const curPoint = computed(() => totals.value.at(-1) ?? null);
const prevPoint = computed(() => (totals.value.length > 1 ? totals.value.at(-2)! : null));
/** KPI 带的柱子：最近 7 段 */
const last7 = computed(() => totals.value.slice(-7));
const totalActive = computed(() => tasks.value.reduce((n, t) => n + t.active_count, 0));
const totalDownloading = computed(() => tasks.value.reduce((n, t) => n + t.downloading_count, 0));
const allUp = computed(() => tasks.value.reduce((n, t) => n + t.total.uploaded, 0));
const allDown = computed(() => tasks.value.reduce((n, t) => n + t.total.downloaded, 0));

/*
 * 当前这一段的合计。按日时统计没取到也有任务列表里的「今日」；别的周期没有统计就是不知道（null），
 * 写「—」而不是 0。
 */
const curUp = computed(() => {
  if (curPoint.value) return curPoint.value.uploaded;
  return shownValue.value === "day" ? tasks.value.reduce((n, t) => n + t.today.uploaded, 0) : 0;
});
const curKnown = computed(() => Boolean(curPoint.value) || shownValue.value === "day");
const curDown = computed(() => {
  if (curPoint.value) return curPoint.value.downloaded;
  return shownValue.value === "day" ? tasks.value.reduce((n, t) => n + t.today.downloaded, 0) : 0;
});

function prevText(v: number | undefined): string {
  return v === undefined ? "" : `${period.value.previous} ${formatBytes(v)}`;
}

/** KPI 柱子的说明：「最近 7 天每天的刷流上传」「最近 5 年每年加入的种子数」 */
const seriesHint = (what: string) =>
  `最近 ${last7.value.length || 7} ${period.value.unit}${period.value.each}${what}`;

const kpiItems = computed(() => [
  {
    label: "在做种子",
    value: String(totalActive.value),
    icon: "list-checks",
    delta: `下载中 ${totalDownloading.value}`,
    deltaTone: "neutral" as const,
    series: last7.value.map((p) => p.added),
    seriesHint: seriesHint("加入的种子数"),
    seriesBaseline: "zero" as const,
  },
  {
    label: `${period.value.current}上传`,
    value: curKnown.value ? formatBytes(curUp.value) : "—",
    icon: "upload",
    delta: prevText(prevPoint.value?.uploaded),
    deltaTone: curUp.value > 0 ? ("ok" as const) : ("neutral" as const),
    series: last7.value.map((p) => p.uploaded),
    seriesHint: seriesHint("的刷流上传"),
    seriesBaseline: "zero" as const,
  },
  {
    label: `${period.value.current}下载`,
    value: curKnown.value ? formatBytes(curDown.value) : "—",
    icon: "download",
    delta: prevText(prevPoint.value?.downloaded),
    deltaTone: "neutral" as const,
    series: last7.value.map((p) => p.downloaded),
    seriesHint: seriesHint("的刷流下载"),
    seriesBaseline: "zero" as const,
  },
  {
    label: "累计分享率",
    value: allDown.value > 0 ? formatRatio(allUp.value / allDown.value) : "—",
    icon: "trending-up",
    delta: `累计上传 ${formatBytes(allUp.value)}`,
    deltaTone: allUp.value > 0 ? ("ok" as const) : ("neutral" as const),
    series: last7.value.map((p) => p.removed),
    seriesHint: seriesHint("删掉的种子数"),
    seriesBaseline: "zero" as const,
  },
]);

/** 每个任务当前这一段的收益（任务表那一列用）；按日时统计没取到也有任务自己的「今日」 */
const taskCurrent = computed(() => {
  const out = new Map<number, { uploaded: number; downloaded: number }>();
  for (const t of stats.value?.tasks ?? []) {
    const p = t.series.at(-1);
    if (p) out.set(t.task_id, { uploaded: p.uploaded, downloaded: p.downloaded });
  }
  return out;
});

function taskPeriodText(t: BrushTask): string {
  const p = taskCurrent.value.get(t.id) ?? (shownValue.value === "day" ? t.today : null);
  return p ? `${formatBytes(p.uploaded)} / ${formatBytes(p.downloaded)}` : "—";
}

function taskPeriodShort(t: BrushTask): string {
  const p = taskCurrent.value.get(t.id) ?? (shownValue.value === "day" ? t.today : null);
  return p ? `↑${formatBytes(p.uploaded)} ↓${formatBytes(p.downloaded)}` : "—";
}

const windowUp = computed(() => totals.value.reduce((n, p) => n + p.uploaded, 0));
const windowDown = computed(() => totals.value.reduce((n, p) => n + p.downloaded, 0));

/** 收益卡下面的说明：合计与口径；周从周一开始，当前这一段只算到今天 */
const trendFoot = computed(() => {
  let text = `合计上传 ${formatBytes(windowUp.value)}、下载 ${formatBytes(windowDown.value)}。口径是刷流种子在下载器里的累计值相邻两次采样之差，按服务器时区分段`;
  if (shownValue.value === "week") text += "，周从周一开始";
  if (shownValue.value !== "day") text += `；${period.value.current}只算到今天`;
  return `${text}。`;
});

/** 收益图的柱子：每一段的上传；读数里写下载、加入与删除 */
const trendPoints = computed<TrendPoint[]>(() => {
  const gran = shownValue.value;
  const n = totals.value.length;
  return totals.value.map((p, i) => {
    const end = p.end ?? p.date;
    const tag = i === n - 1 ? `（${period.value.current}）` : "";
    return {
      short: brushPeriodShort(gran, p.date),
      long: `${brushPeriodLong(gran, p.date, end)}${tag}`,
      value: p.uploaded,
      valueText: `上传 ${formatBytes(p.uploaded)}`,
      detail: `下载 ${formatBytes(p.downloaded)} · 加入 ${p.added} 个 · 删除 ${p.removed} 个`,
    };
  });
});
const hasTrend = computed(() => totals.value.some((p) => p.uploaded > 0 || p.downloaded > 0));
/** 「明细」表：新的在前 */
const showDetail = ref(false);
const detailRows = computed(() => [...trendPoints.value].reverse());
const detailData = computed(() => [...totals.value].reverse());

/* ------------------------------------------------------------------ 列表操作 */

// 站点下拉框只列启用的内置站点（2026-10-26 用户：不该展示未启用的站点）。编辑的任务的站点后来停用了时
// 仍列出它、标「未启用」，免得选择框里只剩一个站点名；保存时要开启任务，后端会要求先启用站点。
const siteOptions = computed(() => {
  const opts = Object.entries(sites.value)
    .filter(([, s]) => s.is_builtin !== false && s.enabled)
    .map(([name]) => ({ name, enabled: true }))
    .sort((a, b) => a.name.localeCompare(b.name));
  const cur = form.value.site_name;
  if (cur && !opts.some((o) => o.name === cur)) opts.push({ name: cur, enabled: false });
  return opts;
});

const running = reactive<Record<number, boolean>>({});

async function runNow(task: BrushTask) {
  if (running[task.id]) return;
  running[task.id] = true;
  try {
    // 跑完了回 200（个别种子的错误在 result.errors 里）；这一轮没跑完时接口回 502，走下面的 catch
    const res = await brushApi.run(task.id);
    const msg = runMessage(res.result);
    if (msg.tone === "error") ElMessage.error(`${task.name}：${msg.text}`);
    else if (msg.tone === "warn") ElMessage.warning(`${task.name}：${msg.text}`);
    else ElMessage.success(`${task.name}：${msg.text}`);
    await loadAll();
  } catch (e: unknown) {
    ElMessage.error(`${task.name}：运行失败：${((e as Error).message || "未知错误").trim()}`);
    // 失败原因也写进了任务的「上次运行」，刷新一下让列表里看得到
    void loadAll();
  } finally {
    running[task.id] = false;
    // 运行记录开着的正是这个任务：把刚跑完的这一轮读进来
    if (runsTask.value?.id === task.id) void runsDrawer.value?.reload();
  }
}

/* 运行记录抽屉 */
const runsTask = ref<BrushTask | null>(null);
const runsDrawer = ref<InstanceType<typeof BrushRunsDrawer> | null>(null);

function openRuns(task: BrushTask) {
  runsTask.value = task;
}

async function toggleEnabled(task: BrushTask) {
  const next = { ...configOf(task), enabled: !task.enabled };
  try {
    const saved = await brushApi.update(task.id, next);
    Object.assign(task, saved);
    ElMessage.success(
      saved.enabled ? `已开启 ${task.name}` : `已关闭 ${task.name}，已加入的种子继续按规则删除`,
    );
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  }
}

async function removeTask(task: BrushTask) {
  try {
    await ElMessageBox.confirm(
      `删除任务「${task.name}」？它名下 ${task.active_count} 个在做的种子会留在下载器里，不再由刷流管理（也不会被自动清理的常规规则处理）。`,
      "删除刷流任务",
      { type: "warning", confirmButtonText: "删除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    await brushApi.remove(task.id);
    ElMessage.success("已删除");
    await loadAll();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "删除失败");
  }
}

function lastRunText(t: BrushTask): string {
  if (!t.last_run_at) return "还没运行";
  return formatShortDateTime(t.last_run_at);
}

function nextRunText(t: BrushTask): string {
  const at = nextRunAt(t);
  if (!t.enabled && t.active_count === 0) return "关闭";
  if (at === null) return "一分钟内";
  return formatShortDateTime(at);
}

/* ------------------------------------------------------------------ 编辑对话框 */

const showDialog = ref(false);
const editingId = ref<number | null>(null);
const form = ref<BrushTaskConfig>(defaultBrushConfig());
const formDiscounts = ref<string[]>(taskDiscounts(form.value.discounts));
// 分区下拉：老任务存的空值按综合区显示（下拉把空值当成没选）
const formSection = computed({
  get: () => brushSectionFor(form.value.site_name, form.value.section),
  set: (v: string) => {
    form.value.section = v;
  },
});
const saving = ref(false);

function openCreate() {
  editingId.value = null;
  form.value = defaultBrushConfig();
  const firstSite = siteOptions.value.find((s) => s.enabled);
  if (firstSite) form.value.site_name = firstSite.name;
  const dl =
    downloaders.value.find((d) => d.is_default && d.enabled) ??
    downloaders.value.find((d) => d.enabled);
  if (dl?.id) form.value.downloader_id = dl.id;
  form.value.name = firstSite ? `${firstSite.name} 刷流` : "";
  formDiscounts.value = taskDiscounts(form.value.discounts);
  showDialog.value = true;
}

function openEdit(task: BrushTask) {
  editingId.value = task.id;
  form.value = configOf(task);
  formDiscounts.value = taskDiscounts(form.value.discounts);
  showDialog.value = true;
}

async function saveTask() {
  if (saving.value) return;
  const body: BrushTaskConfig = {
    ...form.value,
    section: brushSectionFor(form.value.site_name, form.value.section),
    discounts: formDiscounts.value.join(","),
  };
  if (!body.name.trim()) {
    ElMessage.warning("填一个任务名称");
    return;
  }
  if (!body.site_name || !body.downloader_id) {
    ElMessage.warning("选择站点和下载器");
    return;
  }
  if (!formDiscounts.value.length) {
    ElMessage.warning("至少选一种优惠类型");
    return;
  }
  saving.value = true;
  try {
    if (editingId.value === null) await brushApi.create(body);
    else await brushApi.update(editingId.value, body);
    ElMessage.success(editingId.value === null ? "已创建（任务默认关闭，在列表里开启）" : "已保存");
    showDialog.value = false;
    await loadAll();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    saving.value = false;
  }
}

const formRemoval = computed(() => removalSummary({ ...form.value }));

/* ------------------------------------------------------------------ 种子抽屉 */

const drawerTask = ref<BrushTask | null>(null);
const drawerState = ref<BrushTorrentState>("active");
const drawerRows = ref<BrushTorrent[]>([]);
const drawerTotal = ref(0);
const drawerLoading = ref(false);
const drawerError = ref("");
const DRAWER_TABS: { label: string; value: BrushTorrentState }[] = [
  { label: "在做", value: "active" },
  { label: "已删除", value: "removed" },
  { label: "已不在下载器", value: "gone" },
];

async function openTorrents(task: BrushTask) {
  drawerTask.value = task;
  drawerState.value = "active";
  await loadTorrents();
}

/** 抽屉请求的序号：只有最新一次能写列表、错误与 loading（成功、失败、finally 三处都判） */
let drawerSeq = 0;

async function loadTorrents() {
  const task = drawerTask.value;
  if (!task) return;
  const token = ++drawerSeq;
  drawerLoading.value = true;
  drawerError.value = "";
  try {
    const res = await brushApi.torrents(task.id, drawerState.value, 1, 100);
    // 期间切到了别的标签或别的任务：这是旧请求，不落到页面上
    if (token !== drawerSeq) return;
    drawerRows.value = res.items ?? [];
    drawerTotal.value = res.total ?? 0;
  } catch (e: unknown) {
    if (token !== drawerSeq) return;
    drawerRows.value = [];
    drawerTotal.value = 0;
    drawerError.value = (e as Error).message || "读取失败";
  } finally {
    if (token === drawerSeq) drawerLoading.value = false;
  }
}

const drawerOpen = computed({
  get: () => drawerTask.value !== null,
  set: (v: boolean) => {
    if (!v) drawerTask.value = null;
  },
});

function progressText(t: BrushTorrent): string {
  return `${Math.round((t.progress ?? 0) * 100)}%`;
}
</script>

<template>
  <div class="brush-page">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <div class="brush-acts">
        <!-- 统计周期管下面所有的收益数字（KPI 带、任务表的「本期」列、收益图），所以放在它们上面 -->
        <div class="brush-period" role="group" aria-label="收益统计周期">
          <span class="brush-period__label" aria-hidden="true">
            <PtIcon
              :name="statsLoading ? 'loader-circle' : 'calendar-days'"
              :size="14"
              :class="{ 'brush-spin': statsLoading }" />统计
          </span>
          <el-segmented
            v-model="periodValue"
            class="pt-seg brush-period__seg"
            :block="isMobile"
            :options="BRUSH_PERIODS"
            :props="{ label: 'label', value: 'value' }"
            data-testid="brush-period" />
        </div>
        <div class="brush-acts__btns">
          <el-button :loading="loading" @click="loadAll">
            <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
          </el-button>
          <el-button type="primary" data-testid="brush-create" @click="openCreate">
            <PtIcon name="plus" :size="15" /><span>新建任务</span>
          </el-button>
        </div>
      </div>
    </Teleport>

    <PtKpiBar :items="kpiItems" band :aria-busy="statsLoading" />

    <PtToolbar band>
      <el-segmented
        v-model="statusFilter"
        class="pt-seg"
        :options="STATUS_SEG"
        :props="{ label: 'label', value: 'value' }" />
      <el-input v-model="query" class="brush-q" placeholder="筛选任务名、站点…" clearable>
        <template #prefix>
          <PtIcon name="search" :size="14" />
        </template>
      </el-input>
      <el-button v-if="filterOn" @click="clearFilters">
        <PtIcon name="x" :size="14" /><span>清空筛选</span>
      </el-button>
      <template #note>显示 {{ visibleTasks.length }} / {{ tasks.length }} 个</template>
    </PtToolbar>

    <div v-loading="loading" class="pt-band--grid">
      <div v-if="hasPartialBanner(tasks.length)" class="pt-note pt-note--warn brush-partial">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span
          >任务读到了，但站点、下载器或收益统计有没读到的，相关的列和图会是空的；点刷新可重试。</span
        >
      </div>

      <el-table v-if="!isMobile" :data="visibleTasks" class="pt-grid" style="width: 100%">
        <template #empty>
          <PtDataState :state="state" dense :sub="stateSub">
            <template v-if="stateAction !== 'none'" #action>
              <el-button v-if="stateAction === 'retry'" size="small" @click="loadAll">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
              <el-button v-else-if="stateAction === 'clear'" size="small" @click="clearFilters">
                <PtIcon name="x" :size="14" /><span>清空筛选</span>
              </el-button>
              <el-button v-else size="small" type="primary" @click="openCreate">
                <PtIcon name="plus" :size="14" /><span>新建任务</span>
              </el-button>
            </template>
          </PtDataState>
        </template>

        <el-table-column label="任务" min-width="160" class-name="pt-cell-strong">
          <template #default="{ row }">
            <div class="brush-name">{{ row.name }}</div>
            <div class="brush-sub">
              {{ row.site_name
              }}<span v-if="!row.site_enabled" class="brush-warn"> · 站点未启用</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="下载器" width="120" class-name="pt-cell-1line">
          <template #default="{ row }">{{ row.downloader_name || "已删除" }}</template>
        </el-table-column>
        <el-table-column
          label="在做"
          width="96"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">
            <span
              :title="`下载中 ${row.downloading_count} 个，共 ${formatBytes(row.active_size_bytes)}`">
              {{ row.downloading_count }} / {{ row.active_count }}
            </span>
          </template>
        </el-table-column>
        <el-table-column
          :label="`${period.current} 上传 / 下载`"
          width="170"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">{{ taskPeriodText(row) }}</template>
        </el-table-column>
        <el-table-column
          label="累计上传"
          width="110"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">{{ formatBytes(row.total.uploaded) }}</template>
        </el-table-column>
        <el-table-column label="上次运行" min-width="210">
          <template #default="{ row }">
            <div class="brush-run">
              <span>{{ lastRunText(row) }}</span>
              <span class="brush-sub">下次 {{ nextRunText(row) }}</span>
            </div>
            <button
              v-if="row.last_error || row.last_result"
              type="button"
              class="brush-lastrun"
              :title="`${row.last_error || row.last_result}（点开看每一轮的记录）`"
              @click="openRuns(row)">
              <span v-if="row.last_error" class="brush-err">{{ row.last_error }}</span>
              <span v-else class="brush-sub brush-1line">{{ row.last_result }}</span>
            </button>
          </template>
        </el-table-column>
        <el-table-column label="开启" width="70">
          <template #default="{ row }">
            <el-switch
              :model-value="row.enabled"
              size="small"
              :aria-label="`开启或关闭 ${row.name}`"
              @change="toggleEnabled(row)" />
          </template>
        </el-table-column>
        <el-table-column label="操作" width="214" fixed="right" class-name="pt-cell-act">
          <template #default="{ row }">
            <el-tooltip content="立即运行一轮" placement="top">
              <el-button
                link
                type="primary"
                size="small"
                :loading="running[row.id]"
                :aria-label="`立即运行 ${row.name}`"
                @click="runNow(row)">
                <PtIcon v-if="!running[row.id]" name="play" :size="15" />
              </el-button>
            </el-tooltip>
            <el-tooltip content="运行记录" placement="top">
              <el-button
                link
                type="primary"
                size="small"
                :aria-label="`${row.name} 的运行记录`"
                @click="openRuns(row)">
                <PtIcon name="history" :size="15" />
              </el-button>
            </el-tooltip>
            <el-tooltip content="种子" placement="top">
              <el-button
                link
                type="primary"
                size="small"
                :aria-label="`${row.name} 的种子`"
                @click="openTorrents(row)">
                <PtIcon name="list" :size="15" />
              </el-button>
            </el-tooltip>
            <el-tooltip content="编辑" placement="top">
              <el-button
                link
                type="primary"
                size="small"
                :aria-label="`编辑 ${row.name}`"
                @click="openEdit(row)">
                <PtIcon name="pencil" :size="15" />
              </el-button>
            </el-tooltip>
            <el-tooltip content="删除" placement="top">
              <el-button
                link
                type="danger"
                size="small"
                :aria-label="`删除 ${row.name}`"
                @click="removeTask(row)">
                <PtIcon name="trash-2" :size="15" />
              </el-button>
            </el-tooltip>
          </template>
        </el-table-column>
      </el-table>

      <div v-else class="brush-cards">
        <PtDataState v-if="!visibleTasks.length" :state="state" :sub="stateSub">
          <template v-if="stateAction !== 'none'" #action>
            <el-button v-if="stateAction === 'retry'" size="small" @click="loadAll">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
            <el-button v-else-if="stateAction === 'clear'" size="small" @click="clearFilters">
              <PtIcon name="x" :size="14" /><span>清空筛选</span>
            </el-button>
            <el-button v-else size="small" type="primary" @click="openCreate">
              <PtIcon name="plus" :size="14" /><span>新建任务</span>
            </el-button>
          </template>
        </PtDataState>
        <PtRowCard v-for="task in visibleTasks" :key="task.id">
          <template #title>{{ task.name }}</template>
          <template #meta>
            <PtTag>{{ task.site_name }}</PtTag>
            <span>{{ task.downloader_name || "下载器已删除" }}</span>
            <span>在做 {{ task.downloading_count }} / {{ task.active_count }}</span>
            <span>{{ period.current }} {{ taskPeriodShort(task) }}</span>
            <span>上次 {{ lastRunText(task) }}</span>
            <span v-if="task.last_error" class="brush-err brush-row-full">{{
              task.last_error
            }}</span>
          </template>
          <template #status>
            <PtStatusPill dot :tone="task.enabled ? 'ok' : 'neutral'" size="sm">
              {{ task.enabled ? "已开启" : "已关闭" }}
            </PtStatusPill>
          </template>
          <template #actions>
            <el-button size="small" :loading="running[task.id]" @click="runNow(task)">
              <PtIcon v-if="!running[task.id]" name="play" :size="14" /><span>运行</span>
            </el-button>
            <el-button size="small" @click="toggleEnabled(task)">
              <PtIcon :name="task.enabled ? 'pause' : 'play'" :size="14" />
              <span>{{ task.enabled ? "关闭" : "开启" }}</span>
            </el-button>
            <el-button size="small" @click="openRuns(task)">
              <PtIcon name="history" :size="14" /><span>记录</span>
            </el-button>
            <el-button size="small" @click="openTorrents(task)">
              <PtIcon name="list" :size="14" /><span>种子</span>
            </el-button>
            <el-button size="small" @click="openEdit(task)">
              <PtIcon name="pencil" :size="14" /><span>编辑</span>
            </el-button>
            <el-button size="small" type="danger" plain @click="removeTask(task)">
              <PtIcon name="trash-2" :size="14" /><span>删除</span>
            </el-button>
          </template>
        </PtRowCard>
      </div>
    </div>

    <div class="pt-cards pt-cards--2">
      <PtPanel
        :title="`${period.window}收益`"
        icon="chart-column"
        :count="stats && !isMobile ? `上传 ${formatBytes(windowUp)}` : undefined"
        data-testid="brush-trend-panel">
        <template #actions>
          <el-button
            v-if="hasTrend"
            link
            size="small"
            :aria-expanded="showDetail"
            data-testid="brush-trend-detail-toggle"
            @click="showDetail = !showDetail">
            <PtIcon :name="showDetail ? 'chart-column' : 'table-2'" :size="14" />
            <span>{{ showDetail ? "看图" : "明细" }}</span>
          </el-button>
        </template>
        <div class="brush-trend" :aria-busy="statsLoading" data-testid="brush-trend">
          <template v-if="hasTrend">
            <BrushTrendChart
              v-if="!showDetail"
              :points="trendPoints"
              :label="`${period.window}${period.each}全部刷流任务的上传量`" />
            <div v-else class="brush-detail" data-testid="brush-trend-table">
              <table>
                <thead>
                  <tr>
                    <th scope="col">时段</th>
                    <th scope="col">上传</th>
                    <th scope="col">下载</th>
                    <th scope="col">加入</th>
                    <th scope="col">删除</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(p, i) in detailData" :key="p.date">
                    <th scope="row">{{ detailRows[i]!.long }}</th>
                    <td>{{ formatBytes(p.uploaded) }}</td>
                    <td>{{ formatBytes(p.downloaded) }}</td>
                    <td>{{ p.added }}</td>
                    <td>{{ p.removed }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </template>
          <p v-else-if="statsFailed" class="brush-hint">收益统计没有读到，点刷新可重试。</p>
          <p v-else class="brush-hint">
            {{ period.window }}还没有刷流收益。开启任务后，每一轮采样都会记进当时那一段。
          </p>
        </div>
        <p v-if="stats" class="brush-foot">
          {{ trendFoot }}
        </p>
      </PtPanel>

      <PtPanel title="刷流怎么工作" icon="info">
        <ul class="brush-howto">
          <li>
            每轮只请求一次站点的种子列表（走站点限速），按入场条件在本地挑选，再经和 RSS
            相同的推送闸门（磁盘预留、站点做种容量）送进下载器。
          </li>
          <li>默认只收免费种、排除 H&amp;R。站点开启了整站 H&amp;R 时，整站都按 H&amp;R 处理。</li>
          <li>
            站点规则里没有 H&amp;R 做种要求时，在站点设置里填写「H&amp;R
            做种要求」，刷流和自动删种都按它； 没填时有 H&amp;R 的种子刷流不收，自动删种也不删。
          </li>
          <li>
            删种只处理带本任务标签、并且是本任务加的种子；H&amp;R 种子在做种时长达标前一律不删。
          </li>
          <li>
            刷流种子带
            <code>pt-tools-brush</code>
            标签，全局自动清理的常规规则不处理它们；磁盘告急时的紧急清理照样覆盖。
          </li>
          <li>
            关闭任务只是停止加种，已加入的种子继续按删种规则处理。刷流会增加站点请求，开启前请确认站点规则允许。
          </li>
          <li>
            每一轮做了什么记在「运行记录」里：读到的列表、不符合条件的种子与原因、加入与删除的种子、为什么停下。保留
            14 天。
          </li>
        </ul>
      </PtPanel>
    </div>

    <el-dialog
      v-model="showDialog"
      class="pt-dialog brush-dialog"
      :title="editingId === null ? '新建刷流任务' : '编辑刷流任务'"
      width="720px"
      align-center>
      <el-form :model="form" class="pt-form" label-position="top">
        <div class="field-head">基本</div>
        <div class="field-row">
          <el-form-item label="任务名称" required>
            <el-input
              v-model="form.name"
              maxlength="64"
              placeholder="例如 HDSky 刷流"
              data-testid="brush-name" />
          </el-form-item>
          <el-form-item label="检查间隔（分钟）">
            <el-input-number v-model="form.interval_min" :min="5" :max="1440" style="width: 100%" />
            <div class="field-tip">每轮采样、删种、按需加种；最短 5 分钟</div>
          </el-form-item>
        </div>
        <div class="field-row">
          <el-form-item label="站点" required>
            <el-select
              v-model="form.site_name"
              filterable
              placeholder="选择站点"
              no-data-text="没有启用的站点，先在站点列表里启用"
              style="width: 100%"
              data-testid="brush-site">
              <el-option
                v-for="s in siteOptions"
                :key="s.name"
                :label="s.enabled ? s.name : `${s.name}（未启用）`"
                :value="s.name" />
            </el-select>
          </el-form-item>
          <el-form-item label="下载器" required>
            <el-select
              v-model="form.downloader_id"
              placeholder="选择下载器"
              style="width: 100%"
              data-testid="brush-downloader">
              <el-option
                v-for="d in downloaders"
                :key="d.id"
                :label="d.enabled ? d.name : `${d.name}（未启用）`"
                :value="d.id!" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item v-if="hasBrushSection(form.site_name)" label="分区">
          <el-select v-model="formSection" style="width: 100%" data-testid="brush-section">
            <el-option
              v-for="o in BRUSH_SECTIONS"
              :key="o.value"
              :label="o.label"
              :value="o.value" />
          </el-select>
          <div class="field-tip">
            M-Team 的综合区和成人区是两张列表；每个分区每轮读第一页（100 个，置顶的在前）
          </div>
        </el-form-item>
        <div class="field-row">
          <el-form-item label="保存路径">
            <DownloaderPathInput
              v-model="form.save_path"
              :downloader-ids="form.downloader_id"
              placeholder="留空用下载器的默认路径"
              data-testid="brush-save-path" />
          </el-form-item>
          <el-form-item label="分类">
            <el-input v-model="form.category" placeholder="可选" />
          </el-form-item>
        </div>
        <el-form-item label="额外标签">
          <el-input v-model="form.tags" placeholder="逗号分隔，可选" />
          <div class="field-tip">站点名、pt-tools-brush 和任务专属标签总会带上</div>
        </el-form-item>

        <div class="field-head">入场条件</div>
        <el-form-item label="优惠类型">
          <el-checkbox-group v-model="formDiscounts" data-testid="brush-discounts">
            <el-checkbox v-for="d in BRUSH_DISCOUNTS" :key="d.value" :value="d.value">{{
              d.label
            }}</el-checkbox>
          </el-checkbox-group>
          <div class="field-tip">
            按搜索结果里的优惠类型判断。M-Team 的列表会混入普通种，同样在这里拦下；M-Team
            的单种免费、覆盖到这个种子的促销也算免费
          </div>
        </el-form-item>
        <div class="field-row field-row--3">
          <el-form-item label="优惠剩余不少于（分钟）">
            <el-input-number
              v-model="form.min_free_remain_min"
              :min="0"
              :max="10080"
              style="width: 100%" />
          </el-form-item>
          <el-form-item label="最小体积（GB）">
            <el-input-number
              v-model="form.min_size_gb"
              :min="0"
              :precision="1"
              :step="1"
              style="width: 100%" />
          </el-form-item>
          <el-form-item label="最大体积（GB）">
            <el-input-number
              v-model="form.max_size_gb"
              :min="0"
              :precision="1"
              :step="1"
              style="width: 100%" />
            <div class="field-tip">0 = 不限</div>
          </el-form-item>
        </div>
        <div class="field-row field-row--3">
          <el-form-item label="做种人数不多于">
            <el-input-number v-model="form.max_seeders" :min="0" style="width: 100%" />
          </el-form-item>
          <el-form-item label="下载人数不少于">
            <el-input-number v-model="form.min_leechers" :min="0" style="width: 100%" />
          </el-form-item>
          <el-form-item label="发布不超过（分钟）">
            <el-input-number v-model="form.max_publish_age_min" :min="0" style="width: 100%" />
            <div class="field-tip">0 = 不限</div>
          </el-form-item>
        </div>
        <div class="field-row">
          <el-form-item label="标题包含（任一）">
            <el-input
              v-model="form.include_keywords"
              type="textarea"
              :rows="2"
              placeholder="每行或逗号分隔，留空不限" />
          </el-form-item>
          <el-form-item label="标题排除">
            <el-input
              v-model="form.exclude_keywords"
              type="textarea"
              :rows="2"
              placeholder="每行或逗号分隔" />
          </el-form-item>
        </div>
        <el-form-item>
          <el-checkbox v-model="form.exclude_hr"
            >排除 H&amp;R 种子（站点开启整站 H&amp;R 时整站都排除）</el-checkbox
          >
          <div v-if="!form.exclude_hr" class="field-tip">
            不排除时只收算得出 H&amp;R 做种要求的种子：站点规则里没有的，到站点设置里填写「H&amp;R
            做种要求」
          </div>
        </el-form-item>

        <div class="field-head">限额</div>
        <div class="field-row field-row--3">
          <el-form-item label="同时下载数">
            <el-input-number
              v-model="form.max_downloading"
              :min="1"
              :max="50"
              style="width: 100%" />
          </el-form-item>
          <el-form-item label="任务总体积（GB）">
            <el-input-number
              v-model="form.max_total_size_gb"
              :min="0"
              :precision="0"
              :step="50"
              style="width: 100%" />
            <div class="field-tip">只算这个任务加入、还在做的种子；0 = 不限</div>
          </el-form-item>
          <el-form-item label="每天加入（GB）">
            <el-input-number
              v-model="form.max_daily_download_gb"
              :min="0"
              :precision="0"
              :step="10"
              style="width: 100%" />
            <div class="field-tip">0 = 不限</div>
          </el-form-item>
        </div>

        <div class="field-head">删种规则（任一条满足即删）</div>
        <div class="field-row field-row--3">
          <el-form-item label="做种满（小时）">
            <el-input-number
              v-model="form.remove_seed_time_h"
              :min="0"
              :precision="1"
              style="width: 100%" />
          </el-form-item>
          <el-form-item label="分享率达到">
            <el-input-number
              v-model="form.remove_ratio"
              :min="0"
              :precision="2"
              :step="0.5"
              style="width: 100%" />
          </el-form-item>
          <el-form-item label="没有活动（小时）">
            <el-input-number
              v-model="form.remove_inactive_h"
              :min="0"
              :precision="1"
              style="width: 100%" />
          </el-form-item>
        </div>
        <div class="field-row">
          <el-form-item label="平均上传低于（KB/s）">
            <el-input-number
              v-model="form.remove_low_speed_kbs"
              :min="0"
              :precision="0"
              :step="10"
              style="width: 100%" />
          </el-form-item>
          <el-form-item label="按最近多少分钟算">
            <el-input-number
              v-model="form.remove_low_speed_window_min"
              :min="5"
              :max="1440"
              style="width: 100%" />
            <div class="field-tip">只对已下完、做种满这段时间的种子判断</div>
          </el-form-item>
        </div>
        <el-form-item>
          <el-checkbox v-model="form.remove_free_expired_incomplete"
            >免费在下一轮检查前到期、尚未下完的删掉</el-checkbox
          >
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="form.remove_with_data"
            >删种时连数据一起删（关掉则只从下载器移除）</el-checkbox
          >
        </el-form-item>
        <div class="pt-note brush-summary">
          <PtIcon name="info" :size="14" class="pt-note__icon" />
          <span
            >删种：{{ formRemoval }}。限额：{{ limitSummary(form) }}。H&amp;R
            种子在做种时长达标前不删。</span
          >
        </div>
        <el-form-item>
          <el-checkbox v-model="form.enabled">保存后开启（默认关闭）</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showDialog = false">取消</el-button>
        <el-button type="primary" :loading="saving" data-testid="brush-save" @click="saveTask">
          {{ editingId === null ? "创建" : "保存" }}
        </el-button>
      </template>
    </el-dialog>

    <el-drawer
      v-model="drawerOpen"
      :size="isMobile ? '100%' : '760px'"
      :title="drawerTask ? `${drawerTask.name} 的种子` : ''">
      <el-segmented
        v-model="drawerState"
        class="pt-seg"
        :options="DRAWER_TABS"
        :props="{ label: 'label', value: 'value' }"
        @change="loadTorrents" />
      <div v-loading="drawerLoading" class="brush-drawer">
        <div v-if="drawerError" class="pt-note pt-note--warn">
          <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" /><span>{{
            drawerError
          }}</span>
        </div>
        <PtDataState
          v-else-if="!drawerRows.length && !drawerLoading"
          state="empty"
          dense
          sub="这里还没有种子" />
        <ul v-else class="brush-tlist">
          <li v-for="t in drawerRows" :key="t.id" class="brush-titem">
            <div class="brush-titem__title" :title="t.title">{{ t.title || t.torrent_id }}</div>
            <div class="brush-titem__meta">
              <span>{{ formatBytes(t.size_bytes) }}</span>
              <span>{{ discountLabel(t.discount) }}</span>
              <span v-if="t.has_hr">H&amp;R</span>
              <span>进度 {{ progressText(t) }}</span>
              <span>分享率 {{ formatRatio(t.ratio) }}</span>
              <span>↑{{ formatBytes(t.uploaded) }}</span>
              <span>加入 {{ formatShortDateTime(t.added_at) }}</span>
              <span v-if="t.state !== 'active'"
                >{{ formatShortDateTime(t.removed_at) }} · {{ t.remove_reason }}</span
              >
            </div>
          </li>
        </ul>
        <p v-if="drawerTotal > drawerRows.length" class="brush-foot">
          只显示最近 {{ drawerRows.length }} 个，共 {{ drawerTotal }} 个
        </p>
      </div>
    </el-drawer>

    <BrushRunsDrawer ref="runsDrawer" :task="runsTask" @close="runsTask = null" />
  </div>
</template>

<style scoped>
.brush-page {
  display: flex;
  flex-direction: column;
}

.brush-q {
  width: 220px;
}

@media (max-width: 768px) {
  .brush-q {
    width: 100%;
  }
}

/*
 * 页头右端：统计周期 + 刷新 + 新建任务。宽屏送进页头一行；手机上页头不显示，就地放在页面最上面：
 * 周期占满一行（五档等宽），两个按钮在下面一行平分。
 */
.brush-acts {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2) var(--pt-space-3);
  align-items: center;
}

.brush-acts__btns {
  display: flex;
  gap: var(--pt-space-2);
}

.brush-acts__btns .el-button {
  margin: 0;
}

.brush-period {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
}

.brush-period__label {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  white-space: nowrap;
}

@media (max-width: 768px) {
  .brush-acts {
    margin-bottom: var(--pt-space-3);
  }

  .brush-period,
  .brush-acts__btns {
    flex: 1 1 100%;
  }

  .brush-period__seg {
    flex: 1 1 auto;
  }

  .brush-acts__btns .el-button {
    flex: 1 1 0;
  }
}

/* 换周期时统计在重新读：统计图标转起来；数字与图保持上一份（叫法也是上一份的），不压暗 */
.brush-spin {
  animation: brush-spin 0.9s linear infinite;
}

@keyframes brush-spin {
  to {
    transform: rotate(360deg);
  }
}

@media (prefers-reduced-motion: reduce) {
  .brush-spin {
    animation: none;
  }
}

.brush-partial {
  margin: var(--pt-space-3) var(--pt-space-3) 0;
}

.brush-name {
  font-weight: 600;
  color: var(--pt-t1);
}

.brush-sub {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.brush-warn {
  color: var(--pt-warn);
}

.brush-run {
  display: flex;
  gap: var(--pt-space-2);
  align-items: baseline;
}

.brush-err {
  overflow: hidden;
  font-size: var(--pt-fz-label);
  color: var(--pt-dang);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.brush-1line {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.brush-lastrun {
  display: block;
  width: 100%;
  padding: 0;
  border: 0;
  background: transparent;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.brush-lastrun > span {
  display: block;
}

.brush-lastrun:hover span,
.brush-lastrun:focus-visible span {
  text-decoration: underline;
}

.brush-row-full {
  flex: 1 1 100%;
  white-space: normal;
}

.brush-cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-pad) 0;
}

.brush-trend {
  min-height: 56px;
}

/* 收益明细表：图的另一种看法（每一段的数字都在这里），新的在前 */
.brush-detail {
  max-height: 260px;
  overflow: auto;
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-sm);
}

.brush-detail table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--pt-fz-sm);
  font-variant-numeric: tabular-nums;
}

.brush-detail th,
.brush-detail td {
  padding: 6px 10px;
  text-align: right;
  white-space: nowrap;
  border-bottom: 1px solid var(--pt-border);
}

.brush-detail thead th {
  position: sticky;
  top: 0;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: var(--pt-t3);
  background: var(--pt-surface);
}

.brush-detail th:first-child {
  text-align: left;
}

.brush-detail tbody th {
  font-weight: 500;
  color: var(--pt-t1);
}

/* 手机上表比面板宽，在框里横着滚；「时段」那一列钉在左边，滚到加入、删除时还看得见是哪一段 */
.brush-detail th:first-child {
  position: sticky;
  left: 0;
  z-index: 1;
  background: var(--pt-surface);
}

.brush-detail thead th:first-child {
  z-index: 2;
}

@media (max-width: 768px) {
  .brush-detail th,
  .brush-detail td {
    padding: 6px 8px;
  }
}

.brush-detail td {
  color: var(--pt-t2);
}

.brush-detail tbody tr:last-child > * {
  border-bottom: 0;
}

.brush-hint,
.brush-foot {
  margin: var(--pt-space-2) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t3);
}

.brush-howto {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding-left: 18px;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.brush-howto code {
  padding: 1px 5px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

.brush-drawer {
  margin-top: var(--pt-space-3);
}

.brush-tlist {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.brush-titem {
  padding: var(--pt-space-2) var(--pt-space-3);
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.brush-titem__title {
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.brush-titem__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 4px var(--pt-space-3);
  margin-top: 2px;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}
</style>

<style>
/*
 * 对话框 teleport 到 body，scoped 到不了。
 * 全局 .field-row 是 repeat(auto-fit, minmax(220px, 440px))：auto-fit 按上限 440 数列数，
 * 720 宽的对话框里只排得下一列，「站点 / 下载器」被拆成上下两行。这里固定两列、三列。
 */
.brush-dialog .pt-form .field-row {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.brush-dialog .pt-form .field-row--3 {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.brush-dialog .brush-summary {
  margin: 0 0 var(--pt-space-3);
}

@media (max-width: 768px) {
  .brush-dialog .pt-form .field-row,
  .brush-dialog .pt-form .field-row--3 {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
