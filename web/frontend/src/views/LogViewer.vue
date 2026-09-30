<script setup lang="ts">
import { type LogFilesResponse, logsApi, type LogsResponse } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { ElMessage } from "element-plus";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from "vue";

import { hasParsableTime, withinWindow } from "./logTimeWindow";
import { LEVELS, type LogLevel, levelOf } from "./logLevel";

const loading = ref(false);
const logs = shallowRef<string[]>([]);
const logPath = ref("");
const truncated = ref(false);
const logContainer = ref<HTMLElement | null>(null);
const autoScroll = ref(true);
const autoRefresh = ref(true);
const autoRefreshIntervalMs = 15000;

const lineHeight = ref(24);
const scrollTop = ref(0);
const viewportHeight = ref(0);
const overscan = 30;

const lineRenderCache = new Map<string, string>();
const maxCacheEntries = 12000;
const cacheTrimTo = 8000;
const maxHighlightLineLength = 4000;
let resizeObserver: ResizeObserver | null = null;
let scrollFrameId: number | null = null;
let refreshTimer: number | null = null;
let loadTaskId = 0;

/* 日志级别的判定在 ./logLevel.ts（有 JSON level 字段就只认它），这里只管显示 */

const LEVEL_LABEL: Record<LogLevel, string> = {
  error: "ERROR",
  warn: "WARN",
  info: "INFO",
  debug: "DEBUG",
  other: "其他",
};

const LEVEL_TONE: Record<LogLevel, "dang" | "warn" | "info" | "primary" | "mute"> = {
  error: "dang",
  warn: "warn",
  info: "info",
  debug: "mute",
  other: "mute",
};

/**
 * 日志目录清单 —— 画板 29 左栏的 p-files（文件清单）与 p-arc（归档）。
 * 日志是 lumberjack 轮转的：目录里除了当前的 all.log 还有一串带时间戳的备份。
 * 正文接口只 tail 当前文件，所以文件清单单独取一次（`/api/logs/files`）。
 */
const logFiles = ref<LogFilesResponse | null>(null);

/* `?.files?.` 而不是 `?.files.`：响应里缺 files 时也只是「没有清单」，不该整页崩掉 */
const activeFile = computed(() => logFiles.value?.files?.find((f) => f.is_active) ?? null);
const rotatedFiles = computed(() => logFiles.value?.files?.filter((f) => f.rotated) ?? []);
const rotatedBytes = computed(() => rotatedFiles.value.reduce((n, f) => n + f.size, 0));

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 ** 2) return `${(n / 1024).toFixed(1)} KB`;
  if (n < 1024 ** 3) return `${(n / 1024 ** 2).toFixed(1)} MB`;
  return `${(n / 1024 ** 3).toFixed(2)} GB`;
}

function formatWhen(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleString("zh-CN", { hour12: false });
}

/**
 * 页头摘要行 —— 画板 35（移动）的 sub 写的是「pt-tools.log · 18.4 MB · 跟随尾部」，
 * 即「在看哪个文件 · 多大 · 是不是跟着尾部」。这三样都是这一页的状态，外壳猜不出来。
 *
 * 文件清单拿不到时退回 `logPath`（正文接口自己带的路径），只是没有体积可报。
 */
const headSub = computed(() => {
  const parts: string[] = [];
  const active = activeFile.value;
  if (active) parts.push(active.name, formatBytes(active.size));
  else if (logPath.value) parts.push(logPath.value.split("/").pop() || logPath.value);
  parts.push(autoScroll.value ? "跟随尾部" : "已停止跟随");
  return parts.join(" · ");
});

/** 刷新按钮：正文与文件清单一起刷，否则清单会停在打开页面那一刻 */
async function reloadAll() {
  await Promise.all([loadLogs(), loadLogFiles()]);
}

async function loadLogFiles() {
  try {
    logFiles.value = await logsApi.files();
  } catch {
    /* 清单是附带信息，取不到不影响正文；卡里会说明「没取到」 */
    logFiles.value = null;
  }
}

/** 勾中的级别。空集合 = 不筛，全都显示（而不是全都不显示） */
const activeLevels = ref<Set<LogLevel>>(new Set());

/*
 * 画板 29 的 bar-64：seg（全部 / INFO / WARN / ERROR / DEBUG）+ q「搜索日志内容…」
 * + chip「跟随: 开」+ chip「最近 1 小时」+ 右端两枚图标钮。
 *
 * 落地此前工具栏上只有两个勾选框，级别筛选在左栏的卡里，**正文没有任何搜索** ——
 * 一个日志查看器没有搜索，五千行只能靠眼睛找。这里补上搜索与级别分段
 * （分段与左栏那张卡共用 activeLevels，两处不会各说一套）。
 *
 * 「最近 1 小时」那枚现在也落地了。原先记的偏离理由（「时间戳格式随编码器变」）
 * 是错的：config/zap.go 里四个日志**文件**用的都是同一个 JSON 编码器，
 * console 那套只写 stdout，而这一页 tail 的正是文件。判定与筛选在 logTimeWindow.ts。
 */
const logQuery = ref("");

/** 「最近 1 小时」开关。窗口起点每次取数时重算，所以它是滚动的一小时，不是点下那一刻的 */
const lastHourOnly = ref(false);
/** 随 logs 一起更新的时间基准：让 shownLogs 保持纯计算，不在 computed 里读 Date.now() */
const nowStamp = ref(Date.now());

/** 这批日志里一行时间戳都解析不出来时，这枚 chip 停用 —— 不能把整页筛空 */
const timeFilterUsable = computed(() => hasParsableTime(logs.value));

const windowSince = computed(() =>
  lastHourOnly.value && timeFilterUsable.value ? nowStamp.value - 3600_000 : null,
);

const LEVEL_SEG = [
  { label: "全部", value: "" },
  { label: "INFO", value: "info" },
  { label: "WARN", value: "warn" },
  { label: "ERROR", value: "error" },
  { label: "DEBUG", value: "debug" },
] as const;

/** 分段是单选，左栏那张卡是多选 —— 单选时把集合收成这一档 */
const levelSeg = computed({
  get: () => {
    const set = activeLevels.value;
    if (set.size !== 1) return "";
    return [...set][0] as string;
  },
  set: (v: string) => {
    activeLevels.value = v === "" ? new Set() : new Set([v as LogLevel]);
    scrollTop.value = 0;
    if (logContainer.value) logContainer.value.scrollTop = 0;
  },
});

const levelCounts = computed(() => {
  const counts: Record<LogLevel, number> = { error: 0, warn: 0, info: 0, debug: 0, other: 0 };
  for (const line of logs.value) counts[levelOf(line)] += 1;
  return counts;
});

/** 真正渲染的行。虚拟滚动的所有换算都走这一份，不是原始的 logs */
const shownLogs = computed(() => {
  let rows = logs.value;
  if (activeLevels.value.size > 0) {
    rows = rows.filter((line) => activeLevels.value.has(levelOf(line)));
  }
  const q = logQuery.value.trim().toLowerCase();
  if (q) {
    /* 正文就在内存里（最多 5000 行），本地筛是对的：这个接口不分页 */
    rows = rows.filter((line) => line.toLowerCase().includes(q));
  }
  return withinWindow(rows, windowSince.value) as string[];
});

/* 搜索词或时间窗一变，命中集合就换了一批 —— 和切级别一样要回到结果顶部 */
watch([logQuery, lastHourOnly], () => {
  scrollTop.value = 0;
  if (logContainer.value) logContainer.value.scrollTop = 0;
});

function toggleLevel(level: LogLevel) {
  const next = new Set(activeLevels.value);
  if (next.has(level)) next.delete(level);
  else next.add(level);
  activeLevels.value = next;
  /* 换筛选后行数变了，滚动位置按旧行数算出来的偏移就不再对应任何一行 */
  scrollTop.value = 0;
  if (logContainer.value) logContainer.value.scrollTop = 0;
}

const startIndex = computed(() => {
  const rawStart = Math.floor(scrollTop.value / lineHeight.value) - overscan;
  /*
   * 钳到最后一行：有行就一定画得出行。
   *
   * 起因是一次评审判「滚到底再搜索会渲染成空白」—— 起点按旧 scrollTop 算，行数变少后
   * 可能越过表尾，slice 得到空数组，而模板因为 shownLogs 非空并不显示零态。
   * **这个空白态我没能复现**：内容变矮时浏览器会自己把 scrollTop 钳住并派发一次
   * scroll 事件，onLogScroll 随即把起点纠回来（按未钳位的构建实测过，1200 行、
   * scrollTop 24965、命中 1 行，那一行照样画得出来）。
   * 钳位仍然留着：它便宜，且不依赖浏览器一定会补派那次 scroll 事件。
   */
  const maxStart = Math.max(0, shownLogs.value.length - 1);
  return Math.min(Math.max(0, rawStart), maxStart);
});

const visibleCount = computed(() => {
  const rowsInView = Math.ceil(viewportHeight.value / lineHeight.value);
  return Math.max(1, rowsInView + overscan * 2);
});

const endIndex = computed(() => {
  return Math.min(shownLogs.value.length, startIndex.value + visibleCount.value);
});

const topSpacerHeight = computed(() => startIndex.value * lineHeight.value);
const bottomSpacerHeight = computed(
  () => (shownLogs.value.length - endIndex.value) * lineHeight.value,
);

const visibleLines = computed(() => {
  const lines = shownLogs.value;
  return lines.slice(startIndex.value, endIndex.value).map((line, offset) => ({
    html: highlightLine(line ?? ""),
    index: startIndex.value + offset,
  }));
});

function trimLineRenderCache() {
  if (lineRenderCache.size <= maxCacheEntries) {
    return;
  }

  const recentEntries = Array.from(lineRenderCache.entries()).slice(-cacheTrimTo);
  lineRenderCache.clear();
  recentEntries.forEach(([key, value]) => {
    lineRenderCache.set(key, value);
  });
}

function isLikelyJSONLine(line: string): boolean {
  const trimmed = line.trim();
  return trimmed.startsWith("{") && trimmed.endsWith("}");
}

function escapeHtml(text: string): string {
  return text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function getLevelClass(level: string): string {
  switch (level?.toLowerCase()) {
    case "debug":
      return "log-debug";
    case "info":
      return "log-info";
    case "warn":
    case "warning":
      return "log-warn";
    case "error":
      return "log-error";
    case "fatal":
    case "panic":
      return "log-fatal";
    default:
      return "json-string";
  }
}

function formatValue(value: unknown, key?: string): string {
  if (value === null) {
    return '<span class="json-null">null</span>';
  }
  if (typeof value === "boolean") {
    return `<span class="json-boolean">${value}</span>`;
  }
  if (typeof value === "number") {
    return `<span class="json-number">${value}</span>`;
  }
  if (typeof value === "string") {
    const escaped = escapeHtml(value);
    if (key === "level") {
      return `"<span class="${getLevelClass(value)}">${escaped}</span>"`;
    }
    if (key === "time") {
      return `"<span class="json-time">${escaped}</span>"`;
    }
    if (key === "msg") {
      return `"<span class="json-msg">${escaped}</span>"`;
    }
    return `"<span class="json-string">${escaped}</span>"`;
  }
  if (Array.isArray(value)) {
    const items = value.map((v) => formatValue(v)).join('<span class="json-punct">,</span> ');
    return `<span class="json-punct">[</span>${items}<span class="json-punct">]</span>`;
  }
  if (typeof value === "object") {
    return formatObject(value as Record<string, unknown>);
  }
  return escapeHtml(String(value));
}

function formatObject(obj: Record<string, unknown>): string {
  const entries = Object.entries(obj);
  if (entries.length === 0) {
    return '<span class="json-punct">{}</span>';
  }
  const parts = entries.map(([k, v]) => {
    return `"<span class="json-key">${escapeHtml(k)}</span>": ${formatValue(v, k)}`;
  });
  return `<span class="json-punct">{</span>${parts.join(
    '<span class="json-punct">,</span> ',
  )}<span class="json-punct">}</span>`;
}

function highlightLine(line: string): string {
  if (!line.trim()) return "";

  const cached = lineRenderCache.get(line);
  if (cached !== undefined) {
    return cached;
  }

  let result = "";

  if (line.length > maxHighlightLineLength) {
    result = escapeHtml(line);
    lineRenderCache.set(line, result);
    trimLineRenderCache();
    return result;
  }

  if (!isLikelyJSONLine(line)) {
    result = escapeHtml(line);
    lineRenderCache.set(line, result);
    trimLineRenderCache();
    return result;
  }

  try {
    const obj = JSON.parse(line);
    result = formatObject(obj);
  } catch {
    result = escapeHtml(line);
  }

  lineRenderCache.set(line, result);
  trimLineRenderCache();
  return result;
}

function updateViewportMetrics() {
  if (!logContainer.value) {
    return;
  }

  viewportHeight.value = logContainer.value.clientHeight;
  scrollTop.value = logContainer.value.scrollTop;
}

function onLogScroll() {
  if (!logContainer.value || scrollFrameId !== null) {
    return;
  }

  scrollFrameId = window.requestAnimationFrame(() => {
    scrollFrameId = null;
    if (!logContainer.value) {
      return;
    }
    scrollTop.value = logContainer.value.scrollTop;
  });
}

function stopAutoRefresh() {
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer);
    refreshTimer = null;
  }
}

function startAutoRefresh() {
  stopAutoRefresh();
  refreshTimer = window.setInterval(() => {
    if (document.hidden || loading.value) {
      return;
    }
    void loadLogs();
  }, autoRefreshIntervalMs);
}

function measureLineHeight() {
  if (!logContainer.value) {
    return;
  }

  const firstLine = logContainer.value.querySelector<HTMLElement>(".log-line");
  if (!firstLine) {
    return;
  }

  const measuredHeight = firstLine.getBoundingClientRect().height;
  if (Number.isFinite(measuredHeight) && measuredHeight > 0) {
    lineHeight.value = measuredHeight;
  }
}

onMounted(async () => {
  if (logContainer.value) {
    resizeObserver = new ResizeObserver(() => {
      updateViewportMetrics();
    });
    resizeObserver.observe(logContainer.value);
    updateViewportMetrics();
  }

  await Promise.all([loadLogs(), loadLogFiles()]);
  if (autoRefresh.value) {
    startAutoRefresh();
  }
});

watch(autoRefresh, (enabled) => {
  if (enabled) {
    startAutoRefresh();
  } else {
    stopAutoRefresh();
  }
});

onBeforeUnmount(() => {
  stopAutoRefresh();
  if (scrollFrameId !== null) {
    cancelAnimationFrame(scrollFrameId);
    scrollFrameId = null;
  }
  resizeObserver?.disconnect();
  resizeObserver = null;
});

async function loadLogs() {
  const currentTaskId = ++loadTaskId;
  loading.value = true;
  try {
    const data: LogsResponse = await logsApi.get();
    if (currentTaskId !== loadTaskId) {
      return;
    }
    const lines = data.lines || [];
    logs.value = lines;
    /* 每次取数重算时间基准：「最近 1 小时」因此是滚动的一小时，不是点下那一刻起的一小时 */
    nowStamp.value = Date.now();
    logPath.value = data.path || "";
    truncated.value = data.truncated || false;

    await nextTick();
    measureLineHeight();
    updateViewportMetrics();

    if (autoScroll.value) {
      scrollToBottom();
    }
  } catch (e: unknown) {
    if (currentTaskId === loadTaskId) {
      ElMessage.error((e as Error).message || "加载失败");
    }
  } finally {
    if (currentTaskId === loadTaskId) {
      loading.value = false;
    }
  }
}

function scrollToBottom() {
  if (logContainer.value) {
    logContainer.value.scrollTop = logContainer.value.scrollHeight;
    scrollTop.value = logContainer.value.scrollTop;
  }
}

function scrollToTop() {
  if (logContainer.value) {
    logContainer.value.scrollTop = 0;
    scrollTop.value = 0;
  }
}
</script>

<template>
  <div class="log-viewer-page">
    <PtHeadSub>{{ headSub }}</PtHeadSub>

    <!--
      画板 29 的 bar-64 是一条独立的工具栏带（40 高，贴主区两侧），不是卡片内部的一行。
      左边那一列 300 宽的卡（文件清单 / 级别筛选 / 归档）后来靠 /api/logs/files 落了 ——
      这里原来的注释还写着「后端没有对应接口，本页只落 p-tail」，是过时的。
    -->
    <PtToolbar band>
      <!-- 画板 29 的 seg：级别单选，与左栏那张多选卡共用同一个集合 -->
      <el-segmented
        v-model="levelSeg"
        class="pt-seg"
        :options="LEVEL_SEG"
        :props="{ label: 'label', value: 'value' }"
        data-testid="logs-level-seg" />
      <!-- 画板 29 的 q：五千行只能靠眼睛找是不行的 -->
      <el-input
        v-model="logQuery"
        class="log-q"
        size="small"
        placeholder="搜索日志内容…"
        clearable
        data-testid="logs-search">
        <template #prefix>
          <PtIcon name="search" :size="14" />
        </template>
      </el-input>
      <!-- 画板 chip-1「最近 1 小时」：按行里的 JSON time 字段切 -->
      <el-tooltip
        :content="
          timeFilterUsable
            ? '只看最近一小时的日志'
            : '这批日志里没有可解析的时间戳，按时间筛会把整页筛空'
        "
        placement="top">
        <el-checkbox
          v-model="lastHourOnly"
          :disabled="!timeFilterUsable"
          data-testid="logs-last-hour">
          最近 1 小时
        </el-checkbox>
      </el-tooltip>
      <!-- 画板 chip-0「跟随: 开」 -->
      <el-checkbox v-model="autoScroll">跟随尾部</el-checkbox>
      <el-checkbox v-model="autoRefresh">自动刷新（15s）</el-checkbox>

      <!--
        路径不再挂在带上：它现在在页头摘要里（文件名 · 体积 · 跟随状态），
        目录也在「日志文件」卡里写着。挂第三份的代价是它换行占掉两行宽度，
        把搜索框挤成「搜索日志…」——实测如此。
      -->

      <!--
        画板 29 的 bar 右端是纯图标钮（icons: columns-3 / refresh-cw），不是带字按钮。
        之前落成三枚带字按钮（顶部 / 底部 / 刷新，共 250 宽），左组 904 + 右组 250 超过
        1376 下有滚动条时的 1070，整条带被折成两行（81 高，画板 40）。改成图标钮后右组 100。
      -->
      <template #right>
        <el-tooltip content="回到顶部" placement="top">
          <button type="button" class="pt-band__iconbtn" aria-label="回到顶部" @click="scrollToTop">
            <PtIcon name="chevron-up" :size="15" />
          </button>
        </el-tooltip>
        <el-tooltip content="跳到底部" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            aria-label="跳到底部"
            @click="scrollToBottom">
            <PtIcon name="chevron-down" :size="15" />
          </button>
        </el-tooltip>
        <el-tooltip content="重新读取日志与文件列表" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            aria-label="刷新"
            :disabled="loading"
            data-testid="logs-refresh-btn"
            @click="reloadAll">
            <PtIcon
              :name="loading ? 'loader-circle' : 'refresh-cw'"
              :size="15"
              :class="{ 'pt-spin': loading }" />
          </button>
        </el-tooltip>
      </template>
    </PtToolbar>

    <!--
      画板 29：bar-64 之后是左列 300 / 右 764 两栏，左列三张卡
      （p-files 文件清单 / p-lv 级别筛选 / p-arc 归档），右栏是正文 p-tail。
      文件清单与归档来自 `/api/logs/files`（只读目录，不读内容）。
    -->
    <div class="pt-cards pt-cards--side">
      <!-- 左栏的三张卡要装在一个纵向容器里：不然 4 个子节点会被两栏栅格交错排下去 -->
      <div class="lv-col">
        <PtPanel
          class="lv-card"
          title="日志文件"
          icon="folder-open"
          :count="logFiles?.files ? `${logFiles.files.length} 个` : '—'">
          <ul v-if="activeFile" class="lf">
            <li class="lf__row">
              <span class="lf__k">当前文件</span>
              <span class="lf__v">{{ activeFile.name }}</span>
            </li>
            <li class="lf__row">
              <span class="lf__k">大小</span>
              <span class="lf__v">{{ formatBytes(activeFile.size) }}</span>
            </li>
            <li class="lf__row">
              <span class="lf__k">最后写入</span>
              <span class="lf__v">{{ formatWhen(activeFile.mod_time) }}</span>
            </li>
          </ul>
          <p v-else class="lf__empty">
            {{
              logFiles ? "目录里还没有 all.log —— 还没写过日志。" : "文件清单没取到，正文不受影响。"
            }}
          </p>
          <p v-if="logFiles" class="lf__foot">
            目录 <code>{{ logFiles.dir }}</code
            >。正文只 tail 当前文件，最多 5000 行。
          </p>
        </PtPanel>

        <PtPanel
          class="lv-card"
          title="级别筛选"
          icon="list-filter"
          :count="`${shownLogs.length} 行`">
          <ul class="lv">
            <li v-for="level in LEVELS" :key="level">
              <button
                type="button"
                class="lv__btn"
                :class="{ 'is-on': activeLevels.has(level) }"
                :aria-pressed="activeLevels.has(level)"
                @click="toggleLevel(level)">
                <span class="lv__dot" :class="`is-${LEVEL_TONE[level]}`" />
                <span class="lv__k">{{ LEVEL_LABEL[level] }}</span>
                <span class="lv__n">{{ levelCounts[level] }}</span>
              </button>
            </li>
          </ul>
          <p class="lv__foot">
            一个都不勾 = 全部显示。级别是从行里认的（结构化日志看 <code>level</code> 字段， console
            编码看行里的大写级别名），认不出来的归到「其他」。
          </p>
        </PtPanel>

        <!-- 画板 p-arc 300：轮转归档。保留策略来自 zap/lumberjack 的配置 -->
        <PtPanel
          class="lv-card"
          title="轮转归档"
          icon="archive"
          :count="`${rotatedFiles.length} 份`">
          <ul v-if="rotatedFiles.length > 0" class="lf">
            <li v-for="f in rotatedFiles.slice(0, 6)" :key="f.name" class="lf__row">
              <span class="lf__k lf__k--file">{{ f.name }}</span>
              <span class="lf__v">{{ formatBytes(f.size) }}</span>
            </li>
          </ul>
          <p v-else class="lf__empty">还没有轮转备份 —— 当前文件还没到切分大小。</p>
          <p v-if="logFiles" class="lf__foot">
            共 {{ formatBytes(rotatedBytes) }}；保留最近 {{ logFiles.max_backups }} 份、
            {{ logFiles.max_age }} 天。<code>pt-tools clean --category logs</code>
            清的就是这些备份，当前文件不动。
          </p>
        </PtPanel>
      </div>

      <PtPanel title="运行日志" icon="scroll-text" :count="`${logs.length} 行`" padding="none">
        <template #actions>
          <PtStatusPill v-if="truncated" tone="warn" size="sm">
            已截断（最近 5000 行）
          </PtStatusPill>
        </template>

        <div ref="logContainer" class="log-container" @scroll="onLogScroll">
          <pre
            v-if="shownLogs.length"
            class="log-content"
            :style="{ height: `${shownLogs.length * lineHeight}px` }">
          <div class="virtual-spacer" :style="{ height: `${topSpacerHeight}px` }"></div>
          <code
            v-for="line in visibleLines"
            :key="line.index"
            class="log-line"
            v-html="line.html || '&nbsp;'" />
          <div class="virtual-spacer" :style="{ height: `${bottomSpacerHeight}px` }"></div>
        </pre>
          <!--
            空态走六态组件（画板 45：全站空态统一形态），不再是一行灰字。
            两种空的含义不同，文案也不同：一行都没读到 vs 筛选把所有行筛掉了。
          -->
          <PtDataState
            v-else
            :state="logs.length === 0 ? 'empty' : 'zero'"
            :title="logs.length === 0 ? '还没有日志' : '这些级别下没有日志'"
            :sub="
              logs.length === 0
                ? '服务刚启动或日志文件刚轮转过，写入之后这里会自动刷新。'
                : '取消上面的级别筛选就能看到其余行。'
            ">
            <template v-if="logs.length > 0" #action>
              <el-button size="small" @click="activeLevels = new Set()">
                <PtIcon name="x" :size="14" /><span>清空级别筛选</span>
              </el-button>
            </template>
          </PtDataState>
        </div>
      </PtPanel>
    </div>
  </div>
</template>

<style scoped>
/* 画板 29 的 q：220 宽 */
.log-q {
  flex: 0 1 240px;
}

/*
 * 左栏的三张窄卡。`.lv-col` 此前**一条样式都没有** —— 于是它是个 display:block 的 div，
 * 三张卡首尾相接（实测 120+156=276、276+289=565，一点缝都没有），
 * 而画板的卡片层卡间是 16。
 *
 * 顶对齐要放在 `.lv-col` 上（它才是 .pt-cards 栅格的子项），不能放在 `.lv-card` 上：
 * 父级一变成 flex column，子项的 `align-self: start` 就成了交叉轴上的「按内容收缩」，
 * 卡片宽度会塌成文字宽度。
 */
.lv-col {
  display: flex;
  flex-direction: column;
  gap: var(--pt-pad);
  align-self: start;
}

/*
 * 文件清单与归档列表（画板 29 的 p-files / p-arc）。
 *
 * 这几个类名之前只写在模板里，**一条样式都没有** —— 于是 <ul> 用浏览器默认的
 * 圆点列表渲染，键和值之间没有间距，读出来是「当前文件all.log」「大小2.3 MB」。
 * 移动端截图里一眼能看到，桌面验收查的是带与卡，量不到卡内部这种事。
 */
.lf {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.lf__row {
  display: flex;
  gap: var(--pt-space-3);
  align-items: baseline;
  justify-content: space-between;
  min-width: 0;
}

.lf__k {
  flex: 0 0 auto;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 文件名当键用时它才是主角：左对齐、可省略，不跟着标签色 */
.lf__k--file {
  flex: 1 1 auto;
  overflow: hidden;
  font-family: var(--pt-font-mono, monospace);
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  white-space: nowrap;
  text-overflow: ellipsis;
}

.lf__v {
  flex: 0 1 auto;
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-t1);
  text-align: right;
  overflow-wrap: anywhere;
}

.lf__foot,
.lf__empty {
  margin: var(--pt-space-2) 0 0;
  font-size: var(--pt-fz-label);
  line-height: 1.6;
  color: var(--pt-t3);
}

.lv {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin: 0;
  padding: 0;
  list-style: none;
}

/* 整行可点：级别名、计数、色点都算热区，44 的触摸目标由 32 高 + 上下留白凑够 */
.lv__btn {
  display: flex;
  gap: var(--pt-space-2);
  align-items: center;
  width: 100%;
  min-height: 32px;
  padding: 0 var(--pt-space-2);
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  background: none;
  border: 0;
  border-radius: var(--pt-r-sm);
  cursor: pointer;
}

.lv__btn:hover {
  background: var(--pt-hover);
}

.lv__btn.is-on {
  font-weight: 500;
  color: var(--pt-t1);
  background: var(--pt-p-soft);
}

.lv__dot {
  flex: 0 0 auto;
  width: 6px;
  height: 6px;
  background: var(--lv-c, var(--pt-t4));
  border-radius: 50%;
}

.lv__dot.is-dang {
  --lv-c: var(--pt-dang);
}

.lv__dot.is-warn {
  --lv-c: var(--pt-warn);
}

.lv__dot.is-info {
  --lv-c: var(--pt-info);
}

.lv__dot.is-primary {
  --lv-c: var(--pt-p);
}

.lv__dot.is-mute {
  --lv-c: var(--pt-t4);
}

.lv__k {
  flex: 1;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  text-align: left;
}

.lv__n {
  font-variant-numeric: tabular-nums;
}

.lv__foot {
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t4);
}

.lv__foot code {
  padding: 1px 4px;
  font-family: var(--pt-font-mono);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

.log-path {
  padding: 1px 6px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

/*
 * 视口高度而不是 100% ：日志区是页面里唯一需要自己滚的块，撑满剩余空间要让
 * 从 App 的滚动容器一路到这里的每一层都参与 flex 高度计算。clamp 只依赖视口，
 * 上下限保证矮窗口里还看得见几十行、超宽屏上不会拉成一条几千像素的长条。
 */
.log-container {
  height: clamp(360px, calc(100dvh - 320px), 1100px);
  overflow: auto;
  font-family: var(--pt-font-mono);
  color: var(--pt-t1);
  background: var(--pt-canvas);
}

.log-content {
  position: relative;
  margin: 0;
  padding: var(--pt-space-3) var(--pt-pad);
  font-family: inherit;
  font-size: var(--pt-fz-label);
  line-height: 1.55;
  white-space: pre;
}

/* 逐行 block ：虚拟滚动靠固定行高换算 scrollTop，行内换行会让换算漂移 */
.log-content .log-line {
  display: block;
  font-family: inherit;
  white-space: pre;
}

@media (max-width: 768px) {
  .log-container {
    height: clamp(300px, calc(100dvh - 260px), 900px);
  }
}
</style>

<style>
@import "@/styles/log-viewer-syntax.css";
</style>
