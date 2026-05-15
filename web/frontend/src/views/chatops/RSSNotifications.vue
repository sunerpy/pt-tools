<template>
  <div class="rss-notify-page">
    <div class="hero-block">
      <div class="hero-content">
        <span class="hero-eyebrow">CHATOPS · RSS NOTIFY</span>
        <h1 class="hero-title">RSS 通知日志</h1>
        <p class="hero-subtitle">
          查看 RSS 上新通知的投递结果，对失败/待发送条目执行重试或取消。
        </p>
      </div>
    </div>

    <div class="stats-row">
      <div class="stat-chip stat-chip--brand">
        <div class="stat-icon">
          <el-icon><DataLine /></el-icon>
        </div>
        <div class="stat-info">
          <div class="stat-label">总记录数</div>
          <div class="stat-value">{{ pagination.total }}</div>
        </div>
      </div>
      <div class="stat-chip stat-chip--success">
        <div class="stat-icon stat-icon--success">
          <el-icon><Check /></el-icon>
        </div>
        <div class="stat-info">
          <div class="stat-label">已发送 (本页)</div>
          <div class="stat-value">{{ sentCount }}</div>
        </div>
      </div>
      <div class="stat-chip stat-chip--warning">
        <div class="stat-icon stat-icon--warning">
          <el-icon><Warning /></el-icon>
        </div>
        <div class="stat-info">
          <div class="stat-label">失败 / 待重试 (本页)</div>
          <div class="stat-value">{{ failedCount }}</div>
        </div>
      </div>
    </div>

    <section class="glass-card">
      <header class="card-section-header">
        <div class="title-block">
          <h2 class="section-title">通知记录</h2>
          <p class="section-desc">展开行可查看 last_error 详情，失败行可手动触发重试</p>
        </div>
        <div class="header-actions">
          <el-switch
            v-model="autoRefresh"
            active-text="自动刷新 10s"
            inline-prompt
            size="small" />
          <el-button :icon="Refresh" circle @click="fetchLogs" />
        </div>
      </header>

      <div class="filter-bar">
        <el-input
          v-model="filters.rss_id"
          placeholder="RSS ID"
          clearable
          class="filter-item filter-item--search"
          @keyup.enter="handleFilterChange"
          @clear="handleFilterChange" />

        <el-select
          v-model="filters.kind"
          placeholder="通知类型"
          clearable
          class="filter-item"
          @change="handleFilterChange">
          <el-option label="全部上新 (all)" value="all" />
          <el-option label="仅匹配规则 (filtered)" value="filtered" />
        </el-select>

        <el-select
          v-model="filters.result"
          placeholder="结果"
          clearable
          class="filter-item"
          @change="handleFilterChange">
          <el-option label="sent" value="sent" />
          <el-option label="failed" value="failed" />
          <el-option label="suppressed" value="suppressed" />
          <el-option label="pending" value="pending" />
          <el-option label="throttled" value="throttled" />
        </el-select>

        <el-select
          v-model="filters.conf_id"
          placeholder="通道"
          clearable
          class="filter-item"
          @change="handleFilterChange">
          <el-option
            v-for="c in confs"
            :key="c.id"
            :label="`${c.name} (${c.channel_type})`"
            :value="c.id" />
        </el-select>
      </div>

      <el-table
        v-loading="loading"
        :data="logs"
        class="rss-notify-table"
        row-key="id"
        :empty-text="loading ? '加载中...' : '暂无符合条件的通知记录'">
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="args-expand">
              <div class="args-header">
                <h4>详细信息</h4>
              </div>
              <div class="row-detail">
                <div><strong>last_error:</strong> {{ row.last_error || "(空)" }}</div>
                <div><strong>next_retry_at:</strong> {{ row.next_retry_at || "-" }}</div>
                <div><strong>delivered_at:</strong> {{ row.delivered_at || "-" }}</div>
                <pre class="args-json">{{ formatJson(row.payload_json) }}</pre>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="created_at" label="时间" min-width="170">
          <template #default="{ row }">
            <span class="meta-text">{{ formatDate(row.created_at) }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="site_name" label="站点" width="110">
          <template #default="{ row }">
            <el-tag round size="small" effect="plain">{{ row.site_name }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="torrent_id" label="种子 ID" min-width="120">
          <template #default="{ row }">
            <code class="cmd-badge">{{ row.torrent_id }}</code>
          </template>
        </el-table-column>

        <el-table-column prop="notify_kind" label="类型" width="100">
          <template #default="{ row }">
            <el-tag
              round
              :type="row.notify_kind === 'filtered' ? 'success' : ''"
              size="small"
              effect="plain">
              {{ row.notify_kind }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="notification_conf_id" label="通道" width="140">
          <template #default="{ row }">
            <span class="meta-text">{{ confLabel(row.notification_conf_id) }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="result" label="结果" width="110">
          <template #default="{ row }">
            <el-tag round :type="resultTagType(row.result)" size="small" effect="light">
              {{ row.result.toUpperCase() }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="attempts" label="尝试" width="80" align="right">
          <template #default="{ row }">
            <span class="meta-text">{{ row.attempts }}</span>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="160" fixed="right">
          <template #default="{ row }">
            <div class="row-actions">
              <el-button
                v-if="row.result === 'failed' || row.result === 'pending'"
                size="small"
                type="primary"
                plain
                @click="handleRetry(row)">
                重试
              </el-button>
              <el-button
                v-if="row.result === 'pending' || row.result === 'failed'"
                size="small"
                type="danger"
                plain
                @click="handleCancel(row)">
                取消
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="pagination.page"
          :page-size="pagination.pageSize"
          :total="pagination.total"
          layout="total, prev, pager, next"
          @current-change="handlePageChange" />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { chatopsApi, type NotificationConfig, type RSSNotificationLog } from "@/api";
import { Check, DataLine, Refresh, Warning } from "@element-plus/icons-vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";

const loading = ref(false);
const logs = ref<RSSNotificationLog[]>([]);
const confs = ref<NotificationConfig[]>([]);
const autoRefresh = ref(false);
let timer: ReturnType<typeof setInterval> | null = null;

const pagination = reactive({ page: 1, pageSize: 30, total: 0 });
const filters = reactive({
  rss_id: "" as string,
  kind: "" as string,
  result: "" as string,
  conf_id: "" as number | string,
});

const sentCount = computed(() => logs.value.filter((l) => l.result === "sent").length);
const failedCount = computed(
  () => logs.value.filter((l) => l.result === "failed" || l.result === "pending").length,
);

function confLabel(id: number): string {
  const c = confs.value.find((x) => x.id === id);
  return c ? `${c.name}` : `#${id}`;
}

function resultTagType(r: string): "" | "success" | "warning" | "info" | "danger" {
  switch (r) {
    case "sent":
      return "success";
    case "failed":
      return "danger";
    case "throttled":
      return "warning";
    case "suppressed":
      return "info";
    case "pending":
    default:
      return "";
  }
}

function formatDate(s?: string): string {
  if (!s) return "-";
  try {
    return new Date(s).toLocaleString("zh-CN", { hour12: false });
  } catch {
    return s;
  }
}

function formatJson(s?: string): string {
  if (!s) return "(空)";
  try {
    return JSON.stringify(JSON.parse(s), null, 2);
  } catch {
    return s;
  }
}

async function fetchLogs() {
  loading.value = true;
  try {
    const params = new URLSearchParams();
    params.append("page", String(pagination.page));
    params.append("page_size", String(pagination.pageSize));
    if (filters.rss_id) params.append("rss_id", String(filters.rss_id));
    if (filters.kind) params.append("kind", String(filters.kind));
    if (filters.result) params.append("result", String(filters.result));
    if (filters.conf_id) params.append("conf_id", String(filters.conf_id));
    const res = await chatopsApi.rssNotifications.list(params);
    logs.value = res.items || [];
    pagination.total = res.total || 0;
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "加载日志失败");
  } finally {
    loading.value = false;
  }
}

async function fetchConfs() {
  try {
    confs.value = await chatopsApi.notifications.list();
  } catch {
    confs.value = [];
  }
}

async function handleRetry(row: RSSNotificationLog) {
  try {
    await chatopsApi.rssNotifications.retry(row.id);
    ElMessage.success("已加入重试队列");
    fetchLogs();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "重试失败");
  }
}

async function handleCancel(row: RSSNotificationLog) {
  try {
    await ElMessageBox.confirm(`确认取消通知 #${row.id}？`, "确认", {
      type: "warning",
      confirmButtonText: "取消通知",
      cancelButtonText: "返回",
    });
  } catch {
    return;
  }
  try {
    await chatopsApi.rssNotifications.cancel(row.id);
    ElMessage.success("已标记 suppressed");
    fetchLogs();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "取消失败");
  }
}

function handleFilterChange() {
  pagination.page = 1;
  fetchLogs();
}

function handlePageChange(p: number) {
  pagination.page = p;
  fetchLogs();
}

watch(autoRefresh, (v) => {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
  if (v) {
    timer = setInterval(fetchLogs, 10000);
  }
});

onMounted(async () => {
  await Promise.all([fetchConfs(), fetchLogs()]);
});

onBeforeUnmount(() => {
  if (timer) clearInterval(timer);
});
</script>

<style scoped>
.rss-notify-page {
  --chatops-brand: oklch(0.66 0.16 50);
  --chatops-stone-muted: oklch(0.55 0.02 60);
  --chatops-radius-md: 12px;
  --chatops-shadow-sm: 0 1px 2px oklch(0 0 0 / 0.04), 0 1px 3px oklch(0 0 0 / 0.06);
  --chatops-shadow-md:
    0 4px 6px -2px oklch(0 0 0 / 0.05), 0 8px 16px -4px oklch(0 0 0 / 0.08);
  --chatops-glass-bg: oklch(1 0 0 / 0.72);
  --chatops-glass-bg-dk: oklch(0.18 0.01 60 / 0.65);
  --chatops-grid-color: oklch(0.36 0.006 50 / 0.05);
  --chatops-bloom-color: oklch(0.66 0.16 50 / 0.1);
  padding: 16px 24px 32px;
  background-color: var(--pt-bg-base);
  min-height: calc(100vh - 60px);
}
:global(.dark) .rss-notify-page,
:global(html.dark) .rss-notify-page {
  --chatops-brand: oklch(0.72 0.15 55);
  --chatops-stone-muted: oklch(0.65 0.02 70);
  --chatops-glass-bg: var(--chatops-glass-bg-dk);
  --chatops-grid-color: oklch(0.95 0.005 80 / 0.04);
  --chatops-bloom-color: oklch(0.72 0.15 55 / 0.14);
}

.hero-block {
  position: relative;
  padding: 24px 28px;
  margin-bottom: 24px;
  border-radius: 14px;
  background: var(--chatops-glass-bg);
  backdrop-filter: blur(10px) saturate(140%);
  border: 1px solid var(--pt-border-color);
  overflow: hidden;
  box-shadow: var(--chatops-shadow-md);
}
.hero-block::before {
  content: "";
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(to right, var(--chatops-grid-color) 1px, transparent 1px),
    linear-gradient(to bottom, var(--chatops-grid-color) 1px, transparent 1px);
  background-size: 32px 32px;
  pointer-events: none;
  -webkit-mask-image: radial-gradient(ellipse at center, black 30%, transparent 75%);
  mask-image: radial-gradient(ellipse at center, black 30%, transparent 75%);
}
.hero-block::after {
  content: "";
  position: absolute;
  inset: 0;
  background: radial-gradient(circle at 90% 10%, var(--chatops-bloom-color) 0%, transparent 40%);
  pointer-events: none;
}
.hero-content {
  position: relative;
  z-index: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-width: 720px;
}
.hero-eyebrow {
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.18em;
  color: var(--chatops-brand);
  text-transform: uppercase;
}
.hero-title {
  font-family: "Playfair Display", "Noto Serif SC", Georgia, "Songti SC", serif;
  font-size: 1.625rem;
  font-weight: 700;
  margin: 0;
  letter-spacing: -0.025em;
  background: linear-gradient(135deg, var(--chatops-brand), oklch(0.55 0.18 30));
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  color: transparent;
}
.hero-subtitle {
  font-size: 0.95rem;
  color: var(--chatops-stone-muted);
  margin: 4px 0 0;
}

.stats-row {
  display: grid;
  grid-template-columns: 1fr;
  gap: 16px;
  margin-bottom: 24px;
}
@media (min-width: 720px) {
  .stats-row {
    grid-template-columns: repeat(3, 1fr);
  }
}
.stat-chip {
  position: relative;
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 18px 20px;
  border-radius: var(--chatops-radius-md);
  background: color-mix(in oklab, var(--pt-bg-surface) 82%, transparent);
  backdrop-filter: blur(8px);
  border: 1px solid var(--pt-border-color);
  box-shadow: var(--chatops-shadow-sm);
}
.stat-chip::before {
  content: "";
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  opacity: 0.9;
}
.stat-chip--brand::before {
  background: linear-gradient(90deg, var(--chatops-brand) 0%, transparent 100%);
}
.stat-chip--success::before {
  background: linear-gradient(90deg, oklch(0.65 0.13 145) 0%, transparent 100%);
}
.stat-chip--warning::before {
  background: linear-gradient(90deg, oklch(0.74 0.15 70) 0%, transparent 100%);
}
.stat-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 14px;
  font-size: 22px;
  background: color-mix(in oklab, var(--chatops-brand) 12%, transparent);
  color: var(--chatops-brand);
}
.stat-icon--success {
  background: color-mix(in oklab, #16a34a 14%, transparent);
  color: #16a34a;
}
.stat-icon--warning {
  background: color-mix(in oklab, #f59e0b 16%, transparent);
  color: #d97706;
}
.stat-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.stat-label {
  font-size: 13px;
  color: var(--chatops-stone-muted);
}
.stat-value {
  font-size: 26px;
  font-weight: 700;
  color: var(--pt-text-primary);
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.01em;
}

.glass-card {
  padding: 24px;
  border-radius: var(--chatops-radius-md);
  background: color-mix(in oklab, var(--pt-bg-surface) 82%, transparent);
  backdrop-filter: blur(8px);
  border: 1px solid var(--pt-border-color);
  box-shadow: var(--chatops-shadow-sm);
}
.card-section-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 16px;
  margin-bottom: 18px;
  padding-bottom: 14px;
  border-bottom: 1px solid color-mix(in oklab, var(--pt-border-color) 60%, transparent);
}
.title-block {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.section-title {
  font-size: 17px;
  font-weight: 600;
  margin: 0;
  color: var(--pt-text-primary);
}
.section-desc {
  font-size: 13px;
  color: var(--chatops-stone-muted);
  margin: 0;
}
.header-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 18px;
  padding: 14px;
  border-radius: 14px;
  background: color-mix(in oklab, var(--pt-bg-base) 60%, transparent);
  border: 1px solid color-mix(in oklab, var(--pt-border-color) 70%, transparent);
}
.filter-item {
  min-width: 160px;
}
.filter-item--search {
  width: 160px;
}
.filter-item :deep(.el-input__wrapper),
.filter-item :deep(.el-select__wrapper) {
  border-radius: 999px;
}

.rss-notify-table :deep(.el-table) {
  background: transparent;
  --el-table-row-hover-bg-color: color-mix(in oklab, var(--chatops-brand) 4%, transparent);
}
.rss-notify-table :deep(.el-table tr) {
  background: transparent;
}
.cmd-badge {
  font-family: "JetBrains Mono", Menlo, Consolas, monospace;
  font-size: 12px;
  background: color-mix(in oklab, var(--chatops-brand) 8%, transparent);
  padding: 2px 8px;
  border-radius: 6px;
  color: var(--pt-text-primary);
}
.meta-text {
  color: var(--chatops-stone-muted);
  font-size: 13px;
}
.row-actions {
  display: flex;
  gap: 6px;
}
.args-expand {
  padding: 12px 16px;
  background: color-mix(in oklab, var(--pt-bg-base) 80%, transparent);
  border-radius: 10px;
}
.args-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.args-header h4 {
  margin: 0;
  font-size: 14px;
}
.row-detail {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 13px;
  color: var(--pt-text-primary);
}
.args-json {
  margin: 8px 0 0;
  padding: 10px 12px;
  border-radius: 8px;
  background: color-mix(in oklab, #000 6%, transparent);
  font-family: "JetBrains Mono", Menlo, Consolas, monospace;
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}
.pagination-wrapper {
  display: flex;
  justify-content: center;
  margin-top: 18px;
}
</style>
