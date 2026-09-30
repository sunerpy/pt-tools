<script setup lang="ts">
import { type TaskItem, type TaskListResponse, tasksApi, type TaskStatsResponse } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtBars from "@/components/ui/PtBars.vue";
import PtBreakdown, { type BreakdownRow } from "@/components/ui/PtBreakdown.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import { formatShortDateTime } from "@/utils/format";
import { ElMessage, ElMessageBox, type TableInstance } from "element-plus";
import { computed, onMounted, ref } from "vue";

type Tone = "ok" | "warn" | "dang" | "info" | "primary" | "neutral";

const isMobile = useIsMobile();
const tasks = ref<TaskItem[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const tableRef = ref<TableInstance>();

/**
 * 状态开关 —— 三个**可叠加**的筛选位，不是互斥单选。
 *
 * `apiTasks` 把 downloaded / pushed / expired 逐个 AND 进查询（web/server.go），
 * 所以「已下载 + 已推送」是「推成功了的」这个真实且常用的组合，
 * 「已下载 + 已过期」是「下过但免费期结束的」。改版中途曾把这三个收成互斥单选，
 * 那等于删掉了组合筛选能力 —— 这里恢复成独立开关。
 *
 * 落在画板 16 toolbar 的 chip 上（24 高，`chip-0` / `chip-1`）：
 * 画板那枚分段器是「全部 / 下载中 / 做种中 / 等待中 / 已暂停」的下载器状态，
 * 这一页的 RSS 任务没有那套状态，接口也不提供，所以状态位走 chip 而不是分段器。
 */
type StatusKey = "downloaded" | "pushed" | "expired";

/*
 * 画板 16 的 bar-64 上除了分段与搜索，还有两枚 chip（「站点: 全部」「优惠: Free」）
 * 和右端三枚 28×28 图标钮（列设置 / 导出 / 刷新）。站点那枚早就有了，
 * 这里补上「优惠」那枚与列设置、导出 —— 画板画了，落地一直没有。
 *
 * 画板的 seg 是五档下载器状态（全部/下载中/做种中/等待中/已暂停），
 * 而这一页的记录是 RSS 流水（已下载 / 已推送 / 已过期 三个可叠加的标记，
 * 在 apiTasks 里是逐个 AND 的），两者不是同一套词汇 —— 那五档属于下载器控制台（画板 18）。
 * 所以这里保留三枚可叠加 chip，偏离记在 ALLOWED_GAPS 里。
 */
const DISCOUNT_OPTIONS = [
  { label: "Free", value: "FREE" },
  { label: "2xFree", value: "2XFREE" },
  { label: "50%", value: "PERCENT_50" },
  { label: "30%", value: "PERCENT_30" },
  { label: "70%", value: "PERCENT_70" },
  { label: "无优惠", value: "NONE" },
] as const;

/** 空串 = 全部（画板 chip-1「优惠: Free」的默认态是选中某一档，这里默认不筛） */
const discountFilter = ref("");

const TASK_COLS_KEY = "pt-tools-tasks-cols-v1";

const OPTIONAL_TASK_COLS = [
  { key: "hash", label: "Hash" },
  { key: "size", label: "大小" },
  { key: "progress", label: "进度" },
  { key: "freeEnd", label: "免费结束" },
  { key: "checked", label: "最后检查" },
  { key: "pushed", label: "推送时间" },
] as const;

type OptionalTaskCol = (typeof OPTIONAL_TASK_COLS)[number]["key"];

/* 画板 16 的表头里没有 Hash 与推送时间，所以这两列默认收起来（要看在列设置里勾回来） */
const DEFAULT_HIDDEN_TASK_COLS: OptionalTaskCol[] = ["hash", "pushed"];

function loadHiddenTaskCols(): Set<OptionalTaskCol> {
  try {
    const raw = window.localStorage.getItem(TASK_COLS_KEY);
    if (!raw) return new Set(DEFAULT_HIDDEN_TASK_COLS);
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return new Set(DEFAULT_HIDDEN_TASK_COLS);
    const known = new Set(OPTIONAL_TASK_COLS.map((c) => c.key as string));
    return new Set(parsed.filter((k): k is OptionalTaskCol => known.has(k)));
  } catch {
    return new Set(DEFAULT_HIDDEN_TASK_COLS);
  }
}

const hiddenTaskCols = ref<Set<OptionalTaskCol>>(loadHiddenTaskCols());

function toggleTaskCol(key: OptionalTaskCol) {
  const next = new Set(hiddenTaskCols.value);
  if (next.has(key)) next.delete(key);
  else next.add(key);
  hiddenTaskCols.value = next;
  try {
    window.localStorage.setItem(TASK_COLS_KEY, JSON.stringify([...next]));
  } catch {
    /* 存不下就只在本次会话里生效 */
  }
}

const taskColShown = (key: OptionalTaskCol) => !hiddenTaskCols.value.has(key);

const STATUS_OPTIONS: { label: string; value: StatusKey }[] = [
  { label: "已下载", value: "downloaded" },
  { label: "已推送", value: "pushed" },
  { label: "已过期", value: "expired" },
];

/** 默认只开「已推送」，与改版前 filters.pushed = true 的口径一致 */
const status = ref<Record<StatusKey, boolean>>({
  downloaded: false,
  pushed: true,
  expired: false,
});

const activeStatusKeys = computed(() =>
  STATUS_OPTIONS.filter((o) => status.value[o.value]).map((o) => o.value),
);

function toggleStatus(key: StatusKey) {
  status.value[key] = !status.value[key];
  applyFilters();
}

const filters = ref({
  q: "",
  site: "",
});
const selectedIds = ref<number[]>([]);

// 站点列表（从任务中提取）
const siteOptions = computed(() => {
  const sites = new Set<string>();
  tasks.value.forEach((t) => {
    if (t.siteName) sites.add(t.siteName);
  });
  return Array.from(sites);
});

/** 空表要分「还没跑过任务」和「筛掉了」两种：前者要去建 RSS，后者要放宽条件 */
const hasFilters = computed(() => {
  const f = filters.value;
  return Boolean(f.q || f.site || activeStatusKeys.value.length > 0);
});

/**
 * 六态状态机（设计文档 §5）。
 *
 * 以前这里只有一个 loading ref，失败时弹个 toast 就完事 —— 两秒后 toast 消失，
 * 表格停在 empty 上，用户看到的是「还没有数据」。请求失败必须留在页面上，
 * 而且 401/403 要画成「无权访问」，否则用户会一直点重试。
 */
const { loading, state, errorText, run } = useDataState({ filtered: () => hasFilters.value });

const emptySub = computed(() =>
  hasFilters.value ? "换个关键词，或用工具栏右侧的重置清掉筛选" : "RSS 任务跑过之后这里会出现记录",
);

/** 状态块的副标题：失败时给真实错误，空态时给下一步动作 */
const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  return emptySub.value;
});

/**
 * 画板 head 的 sub —— 标题下面那行实时摘要（11.5/400 t3）。
 *
 * 稿上是真实数字（「37 个任务 · 12 下载中 · ↓93.8 MB/s」这类），所以只能由页面自己算。
 * 本接口只回当页的行，拿不到全库的分状态计数，所以状态数前面写明「本页」，
 * 不把当页的 12 条冒充成全部 37 条里的 12 条。没有数据时返回空串，不摆占位符。
 */
const headSub = computed(() => {
  if (!total.value) return "";

  const rows = tasks.value;
  const parts = [`${hasFilters.value ? "筛选后 " : ""}${total.value} 条记录`];

  if (rows.length) {
    const pushed = rows.filter((t) => t.isPushed).length;
    const downloaded = rows.filter((t) => t.isDownloaded && !t.isPushed).length;
    const expired = rows.filter((t) => t.isExpired).length;

    parts.push(`本页 ${rows.length} 条`);
    if (pushed) parts.push(`已推送 ${pushed}`);
    if (downloaded) parts.push(`已下载 ${downloaded}`);
    if (expired) parts.push(`已过期 ${expired}`);
  }

  return parts.join(" · ");
});

/**
 * 吞吐 —— 画板 p-thr 548。
 * 数据是 `/api/tasks/stats` 的最近 7 天按天计数（新增 / 推送 / 免费），
 * 那是真的时间序列，不是拿当页的行凑出来的。
 */
const taskStats = ref<TaskStatsResponse | null>(null);

async function loadStats() {
  try {
    taskStats.value = await tasksApi.stats();
  } catch {
    /* 吞吐是附带信息，取不到就让卡里说明，不影响列表 */
    taskStats.value = null;
  }
}

const dailySeries = computed(() => taskStats.value?.daily ?? []);
const pushedSeries = computed(() => dailySeries.value.map((d) => d.pushed));
const createdSeries = computed(() => dailySeries.value.map((d) => d.created));
const weekPushed = computed(() => pushedSeries.value.reduce((n, v) => n + v, 0));
const weekCreated = computed(() => createdSeries.value.reduce((n, v) => n + v, 0));

/**
 * 画板 16 在 gfoot 之后还有分析卡：p-thr 548（吞吐）、p-site 516（按站点分布）、
 * p-warn 1080（需要关注）。这里落 p-site 与 p-warn —— 两张的数据都来自当前这页的行。
 * p-thr 要的是时间序列（每小时推送量一类），接口只回当页的行，没有历史，故未实现。
 *
 * 口径都写在卡片脚注里：统计的是**当前这一页**，不是全库 —— 接口不回全库的分组计数，
 * 把当页的 12 条冒充成全库 37 条的分布会误导人。
 */
const siteRows = computed<BreakdownRow[]>(() => {
  const buckets = new Map<string, number>();
  for (const t of tasks.value) {
    const name = t.siteName || "未知站点";
    buckets.set(name, (buckets.get(name) ?? 0) + 1);
  }
  return [...buckets.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, n]) => ({ key: name, label: name, value: n, tone: "primary" as const }));
});

/** 需要关注：已过期还没推、推失败重试过、免费期快到但还没下 */
const warnRows = computed<BreakdownRow[]>(() => {
  const out: BreakdownRow[] = [];
  for (const t of tasks.value) {
    const reasons: string[] = [];
    if (t.isExpired && !t.isPushed) reasons.push("免费期已过且没推送");
    if (t.retryCount > 0) reasons.push(`重试过 ${t.retryCount} 次`);
    if (t.lastError) reasons.push(t.lastError);
    if (!t.isDownloaded && !t.isExpired && t.freeEndTime) reasons.push("免费期内还没下载");
    if (reasons.length === 0) continue;
    out.push({
      key: String(t.id),
      label: t.title,
      value: reasons.length,
      tone: t.lastError ? "dang" : "warn",
      hint: `${t.siteName || "未知站点"} · ${reasons.join(" · ")}`,
    });
  }
  return out.sort((a, b) => Number(b.value) - Number(a.value)).slice(0, 8);
});

/** 画板 gfoot 的左侧说明：「37 个任务 · 显示 1–10」 */
const rangeText = computed(() => {
  if (!total.value) return "";
  const from = (page.value - 1) * pageSize.value + 1;
  const to = Math.min(page.value * pageSize.value, total.value);
  return `${total.value} 个任务 · 显示 ${from}–${to}`;
});

onMounted(async () => {
  await Promise.all([loadTasks(), loadStats()]);
});

async function loadTasks() {
  // 换页/换筛选后留着上一页的勾选，会让批量删除删到用户已经看不见的行
  clearSelection();

  const params = new URLSearchParams();
  params.set("page", page.value.toString());
  params.set("page_size", pageSize.value.toString());
  if (filters.value.q) params.set("q", filters.value.q);
  if (filters.value.site) params.set("site", filters.value.site);
  /* 优惠档位走服务端筛：本地筛会让页脚的 total 与表里的行数对不上（分页接口） */
  if (discountFilter.value) params.set("free_level", discountFilter.value);
  for (const key of activeStatusKeys.value) params.set(key, "1");

  const data = await run<TaskListResponse>(() => tasksApi.list(params));
  if (!data) {
    // 失败时清空列表：留着上一次的数据配一个「加载失败」的空态更让人误解
    tasks.value = [];
    total.value = 0;
    return;
  }
  tasks.value = data.items || [];
  total.value = data.total || 0;
}

function applyFilters() {
  page.value = 1;
  loadTasks();
}

function clearFilters() {
  filters.value = { q: "", site: "" };
  status.value = { downloaded: false, pushed: false, expired: false };
  discountFilter.value = "";
  page.value = 1;
  loadTasks();
}

/**
 * 导出当前这一页（画板 bar-64 的 bi-file-down）。
 *
 * 导的是**表里这一页的行**，不是整库：这个接口是分页的，一次只有这些行在手上；
 * 标题里写明页码与筛选，免得下载下来分不清是哪一批。
 */
function exportCsv() {
  const head = ["站点", "优惠", "标题", "Hash", "大小", "进度", "免费结束", "最后检查", "状态"];
  const lines = [head.join(",")];
  for (const t of tasks.value) {
    const cells = [
      t.siteName || "",
      getDiscount(t).text,
      t.title || "",
      t.torrentHash || "",
      formatSize(t.torrentSize ?? 0),
      t.progress === undefined ? "" : `${Math.round(t.progress)}%`,
      t.freeEndTime || "",
      t.lastCheckTime || "",
      getStatusText(t),
    ];
    lines.push(cells.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(","));
  }
  const blob = new Blob([`\uFEFF${lines.join("\n")}`], { type: "text/csv;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `pt-tools-tasks-p${page.value}.csv`;
  a.click();
  URL.revokeObjectURL(url);
  ElMessage.success(`已导出本页 ${tasks.value.length} 条`);
}

function handlePageChange(newPage: number) {
  page.value = newPage;
  loadTasks();
}

function handleSizeChange(newSize: number) {
  pageSize.value = newSize;
  page.value = 1;
  loadTasks();
}

function handleSelectionChange(selection: TaskItem[]) {
  selectedIds.value = selection.map((t) => t.id);
}

/** 多选带上的「取消选择」：桌面要把 el-table 自己那份勾选也一起清掉 */
function clearSelection() {
  selectedIds.value = [];
  tableRef.value?.clearSelection();
}

/** 移动端行卡上的勾选。el-table 的多选是它自己管的，卡片这边自己维护同一份 id 列表 */
function toggleSelect(task: TaskItem) {
  if (task.isPushed) return; // 与桌面 :selectable 一致：已推送的不可删，也就不可选
  const i = selectedIds.value.indexOf(task.id);
  if (i === -1) selectedIds.value.push(task.id);
  else selectedIds.value.splice(i, 1);
}

async function handleBatchDelete() {
  if (selectedIds.value.length === 0) return;

  try {
    await ElMessageBox.confirm(`确认删除 ${selectedIds.value.length} 条记录吗？`, "警告", {
      type: "warning",
      confirmButtonText: "确定",
      cancelButtonText: "取消",
    });

    const res = await tasksApi.batchDelete(selectedIds.value);
    if (res.failed > 0) {
      ElMessage.warning(`删除完成：成功 ${res.success} 条，失败 ${res.failed} 条`);
    } else {
      ElMessage.success(`成功删除 ${res.success} 条记录`);
    }
    loadTasks();
  } catch (e: unknown) {
    if ((e as string) !== "cancel") {
      ElMessage.error((e as Error).message || "批量删除失败");
    }
  }
}

function formatTime(timeStr: string): string {
  if (!timeStr || timeStr === "0001-01-01T00:00:00Z") return "-";
  try {
    return new Date(timeStr).toLocaleString("zh-CN");
  } catch {
    return timeStr;
  }
}

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return "-";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let unitIndex = 0;
  let size = bytes;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }
  return `${size.toFixed(2)} ${units[unitIndex]}`;
}

/* el-progress 把 color 写成内联 background-color，所以这里可以直接给 var()，
   进度条就跟着当前配色走，不再是三个写死的 Element 默认色 */
function getProgressColor(progress: number): string {
  if (progress < 30) return "var(--pt-dang)";
  if (progress < 70) return "var(--pt-warn)";
  return "var(--pt-ok)";
}

function getDownloadedSize(task: TaskItem): string {
  const downloaded = task.torrentSize * (task.progress / 100);
  return formatSize(downloaded);
}

function formatProgress(progress: number): string {
  return `${progress.toFixed(1)}%`;
}

/** 已删除走中性灰：它是终态但不是失败，用 info 会和「无需处理」撞色 */
function getStatusTone(task: TaskItem): Tone {
  if (task.lastError === "种子已从下载器中删除") return "neutral";
  if (task.isExpired) return "dang";
  if (task.isPushed) return "ok";
  if (task.isDownloaded) return "warn";
  return "info";
}

function getStatusText(task: TaskItem): string {
  if (task.lastError === "种子已从下载器中删除") return "已删除";
  if (task.isExpired) return "已过期";
  if (task.isPushed) return "已推送";
  if (task.isDownloaded) return "已下载";
  return "无需处理";
}

/**
 * 优惠等级的语义色：免费是「白拿」→ ok，打折是「还要付一部分」→ warn，
 * 上传加成不省流量 → info，普通没有优惠 → 中性灰。
 */
function getDiscount(task: TaskItem): { text: string; tone: Tone } {
  const level = (task.freeLevel || "").toUpperCase();

  switch (level) {
    case "2XFREE":
    case "_2X_FREE":
      return { text: "2xFree", tone: "ok" };
    case "FREE":
      return { text: "Free", tone: "ok" };
    case "PERCENT_50":
    case "50%":
      return { text: "50%", tone: "warn" };
    case "PERCENT_30":
    case "30%":
      return { text: "30%", tone: "warn" };
    case "PERCENT_70":
    case "70%":
      return { text: "70%", tone: "warn" };
    case "2XUP":
    case "_2X_UP":
      return { text: "2xUp", tone: "info" };
    case "2X50":
    case "_2X_PERCENT_50":
      return { text: "2x50%", tone: "warn" };
    case "NONE":
    case "":
      return { text: "普通", tone: "neutral" };
    default:
      if (task.isFree) {
        return { text: "Free", tone: "ok" };
      }
      if (level) {
        return { text: level, tone: "warn" };
      }
      return { text: "普通", tone: "neutral" };
  }
}
</script>

<template>
  <div class="tasklist-page">
    <!-- 画板 head 的 sub：标题下面那行实时摘要，由本页把真实数字送进外壳页头 -->
    <PtHeadSub v-if="headSub">{{ headSub }}</PtHeadSub>

    <!--
      画板 head 的右侧动作组（y=16 高 32）。本页只有「刷新」是真实入口：
      画板上还有「批量操作」与「添加种子」，前者在本页就是多选带里的批量删除，
      后者本页没有添加种子的入口（任务记录由 RSS 流水线写入），都不凭空造。

      窄屏 .pt-head 整条是 display:none，所以手机上这枚钮改挂在工具栏右侧，
      否则刷新入口会连着页头一起消失。
    -->
    <Teleport v-if="!isMobile" to="#pt-head-acts">
      <button
        type="button"
        class="tl-headico"
        :disabled="loading"
        aria-label="刷新任务列表"
        @click="loadTasks">
        <PtIcon
          :name="loading ? 'loader-circle' : 'refresh-cw'"
          :size="16"
          :class="loading ? 'pt-spin' : undefined" />
      </button>
    </Teleport>

    <!--
      工具栏带 —— 画板 bar-64 328,64 1112×40：只放筛选类控件，
      分段（状态）+ 搜索 220 + 站点 chip，右侧是 28×28 图标钮。
    -->
    <PtToolbar band>
      <!-- 画板 chip-0 / chip-1：24 高的筛选 chip，这里每个状态位一枚，可叠加 -->
      <el-button
        v-for="opt in STATUS_OPTIONS"
        :key="opt.value"
        class="tb__chip"
        :type="status[opt.value] ? 'primary' : 'default'"
        :plain="status[opt.value]"
        :aria-pressed="status[opt.value]"
        @click="toggleStatus(opt.value)">
        <span>{{ opt.label }}</span>
      </el-button>
      <el-input
        v-model="filters.q"
        size="small"
        placeholder="搜索标题 / Hash，回车筛选"
        clearable
        class="tl-q"
        @keyup.enter="applyFilters"
        @clear="applyFilters">
        <template #prefix>
          <PtIcon name="search" :size="14" />
        </template>
      </el-input>
      <el-select
        v-model="filters.site"
        size="small"
        placeholder="全部站点"
        clearable
        class="tl-site"
        @change="applyFilters">
        <el-option v-for="site in siteOptions" :key="site" :label="site" :value="site" />
      </el-select>

      <!-- 画板 16 的 chip-1「优惠: Free」：走服务端筛，页脚计数才跟着变 -->
      <el-select
        v-model="discountFilter"
        class="tb__chip-sel"
        size="small"
        placeholder="优惠: 全部"
        clearable
        data-testid="task-discount-filter"
        @change="applyFilters">
        <el-option label="优惠: 全部" value="" />
        <el-option
          v-for="o in DISCOUNT_OPTIONS"
          :key="o.value"
          :label="`优惠: ${o.label}`"
          :value="o.value" />
      </el-select>

      <template #right>
        <!-- 画板 bar-64 右端的 bi-columns-3 -->
        <el-popover placement="bottom-end" trigger="click" :width="180">
          <template #reference>
            <button
              type="button"
              class="pt-band__iconbtn"
              aria-label="列设置"
              data-testid="tasks-cols-btn">
              <PtIcon name="columns-3" :size="15" />
            </button>
          </template>
          <div class="tb__cols">
            <label v-for="c in OPTIONAL_TASK_COLS" :key="c.key" class="tb__cols-row">
              <el-checkbox :model-value="taskColShown(c.key)" @change="toggleTaskCol(c.key)" />
              <span>{{ c.label }}</span>
            </label>
          </div>
        </el-popover>

        <!-- 画板 bar-64 右端的 bi-file-down：导出当前这一页 -->
        <el-tooltip content="按当前筛选导出本页 CSV" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            aria-label="导出"
            data-testid="tasks-export-btn"
            @click="exportCsv">
            <PtIcon name="file-down" :size="15" />
          </button>
        </el-tooltip>

        <el-tooltip content="重置筛选" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            aria-label="重置筛选"
            @click="clearFilters">
            <PtIcon name="rotate-ccw" :size="15" />
          </button>
        </el-tooltip>
        <!-- 手机上没有页头，刷新钮落在这里 -->
        <el-tooltip v-if="isMobile" content="刷新" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            :disabled="loading"
            aria-label="刷新任务列表"
            @click="loadTasks">
            <PtIcon
              :name="loading ? 'loader-circle' : 'refresh-cw'"
              :size="15"
              :class="loading ? 'pt-spin' : undefined" />
          </button>
        </el-tooltip>
      </template>
    </PtToolbar>

    <!-- 表格带 —— 画板 grid 328,104：全宽平铺，没有圆角也没有外边距 -->
    <div v-loading="loading" class="pt-band--grid">
      <el-table
        v-if="!isMobile"
        ref="tableRef"
        :data="tasks"
        class="pt-grid"
        style="width: 100%"
        @selection-change="handleSelectionChange">
        <template #empty>
          <PtDataState :state="state" dense :sub="stateSub">
            <template v-if="state === 'error'" #action>
              <el-button size="small" @click="loadTasks">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>
        </template>

        <el-table-column
          type="selection"
          width="46"
          :selectable="(row: Record<string, any>) => !row.isPushed" />

        <!--
            站点名是纯文本：画板 15 / 16 的首列是 13/500 #10141A，画板 26 的站点列是
            13/400 #4E5765 —— 一行只有「状态 / 结果」那一列是胶囊。
            站点名包成描边胶囊之后，每行有两三枚色块在抢注意力，反而看不出哪个是状态。
          -->
        <el-table-column label="站点" prop="siteName" width="110" class-name="pt-cell-strong" />

        <!--
          画板 16 的优惠单元是**纯文本** 13/400（td-0-1「Free」），只有「状态」那一列是胶囊。
          优惠是种子的属性，不是任务状态；两列都画成色块之后，一眼扫过去分不出
          哪个才是「这条任务现在怎么了」。语义仍然留在字色上。
        -->
        <el-table-column label="优惠" width="86">
          <template #default="{ row }">
            <span class="tl-free" :class="`is-${getDiscount(row).tone}`">
              {{ getDiscount(row).text }}
            </span>
          </template>
        </el-table-column>

        <!--
          1376 下列宽合计原来 1184 > 表宽 1102，横滚 82：标题最小宽度 280 → 240，
          三列时间 150 → 124（换成「09-30 20:00」紧凑格式之后 150 是按旧的长格式留的），合计 1092。
        -->
        <el-table-column label="标题" min-width="240" class-name="pt-cell-strong">
          <template #default="{ row }">
            <div class="title-cell">
              <span class="title-text">{{ row.title || "-" }}</span>
              <span v-if="row.category || row.tag" class="title-meta">
                <PtTag v-if="row.category">{{ row.category }}</PtTag>
                <PtTag v-if="row.tag">{{ row.tag }}</PtTag>
              </span>
            </div>
          </template>
        </el-table-column>

        <el-table-column v-if="taskColShown('hash')" label="Hash" width="120">
          <template #default="{ row }">
            <el-tooltip v-if="row.torrentHash" :content="row.torrentHash" placement="top">
              <code class="hash-cell">{{ row.torrentHash.slice(0, 8) }}</code>
            </el-tooltip>
            <span v-else class="cell-dim">-</span>
          </template>
        </el-table-column>

        <el-table-column
          v-if="taskColShown('size')"
          label="大小"
          width="96"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">{{ formatSize(row.torrentSize) }}</template>
        </el-table-column>

        <el-table-column v-if="taskColShown('progress')" label="进度" width="170">
          <template #default="{ row }">
            <div v-if="row.torrentSize > 0" class="progress-cell">
              <el-progress
                :percentage="Math.round(row.progress)"
                :stroke-width="4"
                :show-text="false"
                :color="getProgressColor(row.progress)" />
              <span class="progress-info">
                <span>{{ getDownloadedSize(row) }} / {{ formatSize(row.torrentSize) }}</span>
                <span class="progress-pct">{{ formatProgress(row.progress) }}</span>
              </span>
            </div>
            <span v-else class="cell-dim">-</span>
          </template>
        </el-table-column>

        <el-table-column v-if="taskColShown('freeEnd')" label="免费结束" width="124">
          <template #default="{ row }">
            <!-- 紧凑格式单行显示（之前「2026/9/30 20:00:00」在 150 宽里折两行），完整时间在 title -->
            <span
              :class="row.isExpired ? 'cell-dang' : 'cell-mute'"
              :title="formatTime(row.freeEndTime)">
              {{ formatShortDateTime(row.freeEndTime) }}
            </span>
          </template>
        </el-table-column>

        <el-table-column
          v-if="taskColShown('checked')"
          label="最后检查"
          width="124"
          class-name="pt-cell-muted">
          <template #default="{ row }">
            <span :title="formatTime(row.lastCheckTime)">{{
              formatShortDateTime(row.lastCheckTime)
            }}</span>
          </template>
        </el-table-column>

        <el-table-column v-if="taskColShown('pushed')" label="推送时间" width="124">
          <template #default="{ row }">
            <span v-if="row.isPushed" class="cell-ok" :title="formatTime(row.pushTime)">
              {{ formatShortDateTime(row.pushTime) }}
            </span>
            <span v-else class="cell-dim">-</span>
          </template>
        </el-table-column>

        <el-table-column label="状态" width="96" fixed="right">
          <template #default="{ row }">
            <PtStatusPill :tone="getStatusTone(row)" size="sm">
              {{ getStatusText(row) }}
            </PtStatusPill>
          </template>
        </el-table-column>
      </el-table>

      <!--
        移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
        这张表桌面有 10 列，手机上横着滚既看不到列头也和页面纵向滚动打架。
        卡上留的是真正要看的：标题 + 站点/优惠/大小/时间 + 进度 + 状态。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!tasks.length" :state="state" :sub="stateSub">
          <template v-if="state === 'error'" #action>
            <el-button size="small" @click="loadTasks">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>

        <PtRowCard v-for="task in tasks" :key="task.id">
          <template #lead>
            <el-checkbox
              :model-value="selectedIds.includes(task.id)"
              :disabled="task.isPushed"
              :aria-label="`选择 ${task.title}`"
              @update:model-value="toggleSelect(task)" />
          </template>

          <template #title>{{ task.title || "-" }}</template>

          <template #meta>
            <PtTag>{{ task.siteName || "-" }}</PtTag>
            <PtStatusPill :tone="getDiscount(task).tone" size="sm">
              {{ getDiscount(task).text }}
            </PtStatusPill>
            <span>{{ formatSize(task.torrentSize) }}</span>
            <span :class="task.isExpired ? 'cell-dang' : undefined">
              <PtIcon name="clock" :size="11" />
              {{ formatTime(task.freeEndTime) }}
            </span>
          </template>

          <template #status>
            <PtStatusPill dot :tone="getStatusTone(task)" size="sm">
              {{ getStatusText(task) }}
            </PtStatusPill>
          </template>

          <template v-if="task.torrentSize > 0" #progress>
            <el-progress
              :percentage="Math.round(task.progress)"
              :stroke-width="4"
              :show-text="false"
              :color="getProgressColor(task.progress)" />
            <span class="progress-info">
              <span>{{ getDownloadedSize(task) }} / {{ formatSize(task.torrentSize) }}</span>
              <span class="progress-pct">{{ formatProgress(task.progress) }}</span>
            </span>
          </template>
        </PtRowCard>
      </div>
    </div>

    <!--
      多选带 —— 画板 selband-474 328,474 1112×44（cyan@0.06 + 顶 1px 边，按钮高 30）。
      桌面与行卡共用同一份 selectedIds，所以两边都走这一条。
    -->
    <div v-if="selectedIds.length > 0" class="pt-band--sel">
      <PtIcon name="square-check" :size="15" />
      <span>已选 {{ selectedIds.length }} 条</span>
      <span class="pt-band__note">仅未推送的记录可以删除</span>
      <span class="pt-band__spacer" />
      <el-button size="small" @click="clearSelection">
        <PtIcon name="x" :size="14" /><span>取消选择</span>
      </el-button>
      <el-button type="danger" plain size="small" @click="handleBatchDelete">
        <PtIcon name="trash-2" :size="14" /><span>删除 {{ selectedIds.length }} 条</span>
      </el-button>
    </div>

    <!-- 页脚带 —— 画板 gfoot-518 1112×34：左口径说明，右分页 -->
    <div v-if="total > 0" class="pt-band--foot tl-foot">
      <span>{{ rangeText }}</span>
      <span class="pt-band__spacer" />
      <el-pagination
        v-model:current-page="page"
        v-model:page-size="pageSize"
        class="pt-pager"
        :page-sizes="[10, 20, 50, 100]"
        :total="total"
        :pager-count="5"
        layout="sizes, prev, pager, next"
        @size-change="handleSizeChange"
        @current-change="handlePageChange" />
    </div>

    <!--
      画板 16 的分析卡：p-thr 548（吞吐）/ p-site 516（按站点）两栏 + p-warn 1080 通栏。
      吞吐要时间序列（每小时推送量一类），接口只回当页的行、没有历史，所以那张没做；
      分布卡占左栏 548，右栏留空 —— 把它挪到右栏会让左边空一张卡的位置更难看。
    -->
    <div v-if="tasks.length > 0" class="pt-cards pt-cards--2">
      <!-- 画板 p-thr 548：最近 7 天的吞吐（真时间序列，来自 /api/tasks/stats） -->
      <PtPanel title="最近 7 天吞吐" icon="activity" :count="`推送 ${weekPushed} 次`">
        <template v-if="dailySeries.length > 0">
          <div class="thr__row">
            <span class="thr__l">已推送</span>
            <span class="thr__v">{{ weekPushed }}</span>
          </div>
          <PtBars
            :values="pushedSeries"
            :count="7"
            :bar-width="26"
            :gap="8"
            :height="40"
            hue="var(--pt-p)" />
          <div class="thr__row">
            <span class="thr__l">新入库</span>
            <span class="thr__v">{{ weekCreated }}</span>
          </div>
          <PtBars
            :values="createdSeries"
            :count="7"
            :bar-width="26"
            :gap="8"
            :height="40"
            hue="var(--pt-info)" />
          <p class="thr__foot">
            一根一天，最左是 {{ dailySeries[0]?.date }}，最右是今天。口径是全库按天计数，
            不是当前这一页。
          </p>
        </template>
        <p v-else class="tl-ok">吞吐数据没取到，列表不受影响。</p>
      </PtPanel>

      <PtPanel title="按站点分布" icon="globe" :count="`${siteRows.length} 个站点`">
        <PtBreakdown
          :rows="siteRows"
          :total="tasks.length"
          foot="统计的是当前这一页的行；接口不回全库的分组计数。" />
      </PtPanel>

      <PtPanel
        class="pt-cards__full"
        title="需要关注的任务"
        icon="triangle-alert"
        :count="warnRows.length > 0 ? `${warnRows.length} 条` : '暂无'">
        <PtBreakdown
          v-if="warnRows.length > 0"
          :rows="warnRows"
          cols
          foot="判定口径：免费期已过还没推送、推送重试过、有错误、或免费期内还没下载。最多列 8 条。" />
        <p v-else class="tl-ok">当前这一页没有需要处理的任务。</p>
      </PtPanel>
    </div>
  </div>
</template>

<style scoped>
/*
 * 画板 16 的主区是一串全宽横向带：head 64 → bar-64 40 → grid → selband 44 → gfoot 34。
 * 带与带之间没有间距，所以这里不给 gap；需要内缩的东西（移动端行卡）自己带留白。
 */
.tasklist-page {
  display: flex;
  flex-direction: column;
}

/* 画板 toolbar：搜索框 220×28 */
.tl-q {
  flex: 0 0 auto;
  width: 220px;
}

/* 站点筛选：画板那两枚 chip 的位置，这里是真实的站点下拉 */
.tl-site {
  flex: 0 0 auto;
  width: 150px;
}

/*
 * 优惠列：纯文本 + 语义字色（画板 td-0-1 是 13/400 #4E5765）。
 * free / 2xfree 用 ok 色标出来，其余保持正文色 —— 免费是这一列唯一值得抬眼的信息。
 */
.tl-free {
  font-weight: 400;
  color: var(--pt-t2);
}

.tl-free.is-ok {
  font-weight: 500;
  color: var(--pt-ok);
}

.tl-free.is-warn {
  color: var(--pt-warn);
}

.tl-free.is-info {
  color: var(--pt-info);
}

/* 画板 chip-1「优惠」那枚下拉：与 chip 同宽档，不抢搜索框的位置 */
.tb__chip-sel {
  width: 128px;
}

/* 列设置面板：一行一个勾选 */
.tb__cols {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tb__cols-row {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  font-size: var(--pt-fz-sm);
  cursor: pointer;
}

/* 画板 head 的图标钮：32×32 fill hover r=4 icon16（外壳只给 el-button 定了 32 高） */
.tl-headico {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  color: var(--pt-t2);
  cursor: pointer;
  background: var(--pt-hover);
  border: 0;
  border-radius: var(--pt-r-sm);
  transition:
    color var(--pt-transition-fast),
    background var(--pt-transition-fast);
}

.tl-headico:hover:not(:disabled) {
  color: var(--pt-t1);
  background: var(--pt-border);
}

.tl-headico:disabled {
  cursor: default;
  opacity: 0.6;
}

/* 分页里的「每页条数」下拉默认 32 高，会把 34 的页脚带顶开，收到 26 */
.tl-foot :deep(.el-pagination__sizes .el-input__wrapper) {
  height: 26px;
}

/* 多选带整条是 12.5/600，带里那句说明应当退回常规字重 */
.pt-band--sel .pt-band__note {
  font-weight: 400;
}

/* 共享的 28×28 图标钮没给禁用态，刷新进行中需要看出来点不动 */
.pt-band__iconbtn:disabled {
  cursor: default;
  opacity: 0.6;
}

/* 标题与分类标签同一行，标题占满剩余宽度后再省略号 */
.title-cell {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  min-width: 0;
}

.title-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.title-meta {
  display: flex;
  flex: 0 0 auto;
  gap: 4px;
}

.hash-cell {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  cursor: help;
}

/* 进度条 + 一行说明压在 34 高的行里：4px 细条 + 11 号说明刚好 */
.progress-cell {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.progress-info {
  display: flex;
  gap: var(--pt-space-2);
  justify-content: space-between;
  font-size: var(--pt-fz-foot);
  color: var(--pt-t3);
}

.progress-pct {
  flex: 0 0 auto;
  font-variant-numeric: tabular-nums;
}

.cell-mute {
  color: var(--pt-t2);
}

.cell-dim {
  color: var(--pt-t3);
}

.cell-ok {
  color: var(--pt-ok);
}

.cell-dang {
  color: var(--pt-dang);
}

/*
 * 状态筛选 chip —— 画板 chip 是 24 高（工具栏里其余控件 28）。
 * 用 el-button 而不是 el-check-tag：后者在四套配色下自带一套自己的选中色，
 * 和 primary/plain 这对已经在用的表达对不上。
 */
.tb__chip {
  height: 24px;
  padding: 0 10px;
  margin: 0;
  font-size: var(--pt-fz-label);
}

/* 吞吐卡：一行标签 + 数字，下面一排柱 */
.thr__row {
  display: flex;
  gap: var(--pt-space-2);
  align-items: baseline;
  justify-content: space-between;
  margin-bottom: 4px;
  font-size: var(--pt-fz-sm);
}

.thr__row:not(:first-child) {
  margin-top: var(--pt-space-3);
}

.thr__l {
  color: var(--pt-t3);
}

.thr__v {
  font-variant-numeric: tabular-nums;
  font-weight: 600;
  color: var(--pt-t1);
}

.thr__foot {
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t3);
}

/* 分析卡里「一条都不用管」的正面结论 */
.tl-ok {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

/* 移动端行卡列表：表格带自己不留白，所以 16 的内缩由这里给 */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-pad) 0;
}

@media (max-width: 768px) {
  .tl-q,
  .tl-site {
    flex: 1 1 100%;
    width: 100%;
  }
}
</style>
