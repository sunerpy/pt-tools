<script setup lang="ts">
import { logsApi, type LogsResponse } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { ElMessage } from "element-plus";
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from "vue";

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

const startIndex = computed(() => {
  const rawStart = Math.floor(scrollTop.value / lineHeight.value) - overscan;
  return Math.max(0, rawStart);
});

const visibleCount = computed(() => {
  const rowsInView = Math.ceil(viewportHeight.value / lineHeight.value);
  return Math.max(1, rowsInView + overscan * 2);
});

const endIndex = computed(() => {
  return Math.min(logs.value.length, startIndex.value + visibleCount.value);
});

const topSpacerHeight = computed(() => startIndex.value * lineHeight.value);
const bottomSpacerHeight = computed(() => (logs.value.length - endIndex.value) * lineHeight.value);

const visibleLines = computed(() => {
  const lines = logs.value;
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

  await loadLogs();
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
    <PtPanel title="运行日志" icon="scroll-text" :count="`${logs.length} 行`" padding="none">
      <template #actions>
        <PtStatusPill v-if="truncated" tone="warn" size="sm">已截断（最近 5000 行）</PtStatusPill>
      </template>

      <PtToolbar>
        <el-checkbox v-model="autoScroll">自动滚动</el-checkbox>
        <el-checkbox v-model="autoRefresh">自动刷新（15s）</el-checkbox>

        <template v-if="logPath" #note>
          <code class="log-path">{{ logPath }}</code>
        </template>

        <template #right>
          <el-button @click="scrollToTop">
            <PtIcon name="chevron-up" :size="14" /><span>顶部</span>
          </el-button>
          <el-button @click="scrollToBottom">
            <PtIcon name="chevron-down" :size="14" /><span>底部</span>
          </el-button>
          <el-button type="primary" :loading="loading" @click="loadLogs">
            <PtIcon name="refresh-cw" :size="14" /><span>刷新</span>
          </el-button>
        </template>
      </PtToolbar>

      <div ref="logContainer" class="log-container" @scroll="onLogScroll">
        <pre
          v-if="logs.length"
          class="log-content"
          :style="{ height: `${logs.length * lineHeight}px` }">
          <div class="virtual-spacer" :style="{ height: `${topSpacerHeight}px` }"></div>
          <code
            v-for="line in visibleLines"
            :key="line.index"
            class="log-line"
            v-html="line.html || '&nbsp;'" />
          <div class="virtual-spacer" :style="{ height: `${bottomSpacerHeight}px` }"></div>
        </pre>
        <pre v-else class="log-content"><code class="log-line">暂无日志</code></pre>
      </div>
    </PtPanel>
  </div>
</template>

<style scoped>
.log-path {
  padding: 1px 6px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  background: var(--pt-hover);
  border-radius: 3px;
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
