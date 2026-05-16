<template>
  <div class="audit-log-page">
    <div class="hero-block">
      <div class="hero-content">
        <span class="hero-eyebrow">CHATOPS · AUDIT</span>
        <h1 class="hero-title">操作审计日志</h1>
        <p class="hero-subtitle">查询与追踪 ChatOps 机器人的所有命令执行记录，敏感参数已脱敏。</p>
      </div>
    </div>

    <div class="stats-row">
      <div class="stat-chip stat-chip--brand">
        <div class="stat-icon">
          <el-icon><DataLine /></el-icon>
        </div>
        <div class="stat-info">
          <div class="stat-label">今日执行命令</div>
          <div class="stat-value">{{ stats.todayCount }}</div>
        </div>
      </div>
      <div class="stat-chip stat-chip--success">
        <div class="stat-icon stat-icon--success">
          <el-icon><Check /></el-icon>
        </div>
        <div class="stat-info">
          <div class="stat-label">整体成功率</div>
          <div class="stat-value">{{ formatSuccessRate(stats.successRate) }}</div>
        </div>
      </div>
      <div class="stat-chip stat-chip--warning">
        <div class="stat-icon stat-icon--warning">
          <el-icon><Timer /></el-icon>
        </div>
        <div class="stat-info">
          <div class="stat-label">最高延迟</div>
          <div class="stat-value">{{ stats.maxLatencyMs }}<span class="stat-unit">ms</span></div>
        </div>
      </div>
    </div>

    <section class="glass-card">
      <header class="card-section-header">
        <div class="title-block">
          <h2 class="section-title">日志记录</h2>
          <p class="section-desc">展开行可查看脱敏后的完整命令参数</p>
        </div>
      </header>

      <div class="filter-bar">
        <el-date-picker
          v-model="filters.dateRange"
          type="datetimerange"
          range-separator="至"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
          format="YYYY-MM-DD HH:mm:ss"
          value-format="YYYY-MM-DDTHH:mm:ssZ"
          class="filter-item filter-item--date"
          @change="handleFilterChange" />

        <el-select
          v-model="filters.channelType"
          placeholder="通道类型"
          multiple
          collapse-tags
          collapse-tags-tooltip
          clearable
          class="filter-item"
          @change="handleFilterChange">
          <el-option label="Telegram" value="telegram" />
          <el-option label="QQ" value="qq" />
          <el-option label="企业微信" value="wecom" />
          <el-option label="Webhook" value="webhook" />
        </el-select>

        <el-select
          v-model="filters.result"
          placeholder="执行结果"
          multiple
          collapse-tags
          collapse-tags-tooltip
          clearable
          class="filter-item"
          @change="handleFilterChange">
          <el-option label="Success" value="success" />
          <el-option label="Denied" value="denied" />
          <el-option label="Error" value="error" />
        </el-select>

        <el-input
          v-model="filters.command"
          placeholder="搜索命令..."
          clearable
          class="filter-item filter-item--search"
          @keyup.enter="handleFilterChange"
          @clear="handleFilterChange">
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
      </div>

      <el-table
        v-loading="loading"
        :data="auditLogs"
        class="audit-table"
        row-key="id"
        :empty-text="loading ? '加载中...' : '暂无符合条件的审计记录'">
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="args-expand">
              <div class="args-header">
                <h4>命令参数</h4>
                <el-tag size="small" round type="warning" effect="plain" class="redacted-tag">
                  <el-icon><Lock /></el-icon> 部分敏感数据已脱敏 (Redacted)
                </el-tag>
              </div>
              <pre class="args-json">{{ formatJson(row.args_json) }}</pre>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="created_at" label="时间" min-width="170">
          <template #default="{ row }">
            <span class="meta-text">{{ formatDate(row.created_at) }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="channel_type" label="通道" width="110">
          <template #default="{ row }">
            <el-tag round :type="getChannelTagType(row.channel_type)" size="small" effect="plain">
              {{ row.channel_type }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="channel_user_id" label="触发用户" min-width="140">
          <template #default="{ row }">
            <span class="user-id">{{ row.channel_user_id }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="command" label="命令" min-width="140">
          <template #default="{ row }">
            <code class="cmd-badge">{{ row.command }}</code>
          </template>
        </el-table-column>

        <el-table-column prop="result" label="结果" width="110">
          <template #default="{ row }">
            <el-tag round :type="getResultTagType(row.result)" size="small" effect="light">
              {{ row.result.toUpperCase() }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="latency_ms" label="延迟" width="100" align="right">
          <template #default="{ row }">
            <span :class="['latency', row.latency_ms > 1000 ? 'high-latency' : '']">
              {{ row.latency_ms }} ms
            </span>
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
import { ref, reactive, onMounted } from "vue";
import { DataLine, Check, Timer, Search, Lock } from "@element-plus/icons-vue";
import { chatopsApi, type AuditLog } from "@/api";
import { ElMessage } from "element-plus";

const loading = ref(false);
const auditLogs = ref<AuditLog[]>([]);

const stats = reactive({
  todayCount: 0,
  successRate: 0,
  maxLatencyMs: 0,
});

const pagination = reactive({
  page: 1,
  pageSize: 30,
  total: 0,
});

const filters = reactive({
  dateRange: null as [string, string] | null,
  channelType: [] as string[],
  result: [] as string[],
  command: "",
});

onMounted(() => {
  fetchAuditLogs();
});

const fetchAuditLogs = async () => {
  loading.value = true;
  try {
    const params = new URLSearchParams();
    params.append("page", pagination.page.toString());
    params.append("page_size", pagination.pageSize.toString());

    if (filters.dateRange && filters.dateRange.length === 2) {
      params.append("start_time", filters.dateRange[0]);
      params.append("end_time", filters.dateRange[1]);
    }
    if (filters.channelType.length > 0) {
      params.append("channel_type", filters.channelType.join(","));
    }
    if (filters.result.length > 0) {
      params.append("result", filters.result.join(","));
    }
    if (filters.command) {
      params.append("command", filters.command);
    }

    const [listRes, statsRes] = await Promise.all([
      chatopsApi.audit.list(params),
      chatopsApi.audit.stats(),
    ]);

    auditLogs.value = listRes.items || [];
    pagination.total = listRes.total || 0;
    stats.todayCount = statsRes.today_count || 0;
    stats.successRate = statsRes.success_rate || 0;
    stats.maxLatencyMs = statsRes.max_latency_ms || 0;
  } catch (err: unknown) {
    ElMessage.error((err as Error).message || "获取审计日志失败");
  } finally {
    loading.value = false;
  }
};

const handleFilterChange = () => {
  pagination.page = 1;
  fetchAuditLogs();
};

const handlePageChange = (page: number) => {
  pagination.page = page;
  fetchAuditLogs();
};

const formatDate = (dateStr: string) => {
  if (!dateStr) return "-";
  const date = new Date(dateStr);
  return date.toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
};

const formatJson = (jsonStr?: string) => {
  if (!jsonStr) return "无参数";
  try {
    const obj = JSON.parse(jsonStr);
    return JSON.stringify(obj, null, 2);
  } catch (_e) {
    return jsonStr;
  }
};

const formatSuccessRate = (rate: number) => `${rate.toFixed(2)}%`;

const getChannelTagType = (type: string) => {
  const map: Record<string, "" | "success" | "warning" | "info" | "danger"> = {
    telegram: "",
    qq: "warning",
    wecom: "success",
    webhook: "info",
  };
  return map[type] || "info";
};

const getResultTagType = (result: string) => {
  const map: Record<string, "" | "success" | "warning" | "info" | "danger"> = {
    success: "success",
    denied: "warning",
    error: "danger",
  };
  return map[result.toLowerCase()] || "info";
};
</script>

<style scoped>
/* delta-ui-origin polish layer — scoped to chatops/AuditLog */
.audit-log-page {
  --chatops-brand: oklch(0.66 0.16 50);
  --chatops-brand-hover: oklch(0.6 0.18 50);
  --chatops-brand-soft: oklch(0.95 0.04 60);
  --chatops-stone-muted: oklch(0.55 0.02 60);
  --chatops-radius-sm: 8px;
  --chatops-radius-md: 12px;
  --chatops-radius-lg: 18px;
  --chatops-shadow-sm: 0 1px 2px oklch(0 0 0 / 0.04), 0 1px 3px oklch(0 0 0 / 0.06);
  --chatops-shadow-md: 0 4px 6px -2px oklch(0 0 0 / 0.05), 0 8px 16px -4px oklch(0 0 0 / 0.08);
  --chatops-shadow-lg: 0 10px 24px -6px oklch(0 0 0 / 0.1), 0 16px 32px -8px oklch(0 0 0 / 0.12);
  --chatops-glass-bg: oklch(1 0 0 / 0.72);
  --chatops-glass-bg-dk: oklch(0.18 0.01 60 / 0.65);
  --chatops-dot-color: oklch(0.66 0.16 50 / 0.1);
  --chatops-grid-color: oklch(0.36 0.006 50 / 0.05);
  --chatops-bloom-color: oklch(0.66 0.16 50 / 0.1);
}
:global(.dark) .audit-log-page,
:global(html.dark) .audit-log-page {
  --chatops-brand: oklch(0.72 0.15 55);
  --chatops-brand-hover: oklch(0.78 0.13 55);
  --chatops-brand-soft: oklch(0.3 0.05 55 / 0.4);
  --chatops-stone-muted: oklch(0.65 0.02 70);
  --chatops-glass-bg: var(--chatops-glass-bg-dk);
  --chatops-dot-color: oklch(0.72 0.15 55 / 0.18);
  --chatops-grid-color: oklch(0.95 0.005 80 / 0.04);
  --chatops-bloom-color: oklch(0.72 0.15 55 / 0.14);
}

.audit-log-page {
  padding: 16px 24px 32px;
  background-color: var(--pt-bg-base);
  min-height: calc(100vh - 60px);
}

.hero-block {
  position: relative;
  padding: 24px 28px;
  margin-bottom: 24px;
  border-radius: 14px;
  background: var(--chatops-glass-bg);
  backdrop-filter: blur(10px) saturate(140%);
  -webkit-backdrop-filter: blur(10px) saturate(140%);
  border: 1px solid var(--pt-border-color);
  overflow: hidden;
  box-shadow: var(--chatops-shadow-md);
}

@media (min-width: 768px) {
  .hero-block {
    padding: 28px 32px;
  }
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
  z-index: 0;
}

.hero-block::after {
  content: "";
  position: absolute;
  inset: 0;
  background: radial-gradient(circle at 90% 10%, var(--chatops-bloom-color) 0%, transparent 40%);
  pointer-events: none;
  z-index: 0;
}

.hero-block > * {
  position: relative;
  z-index: 1;
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
  line-height: 1.15;
  background: linear-gradient(135deg, var(--chatops-brand), oklch(0.55 0.18 30));
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  color: transparent;
}

@media (min-width: 768px) {
  .hero-title {
    font-size: 2rem;
  }
}

.hero-subtitle {
  font-size: 0.95rem;
  color: var(--chatops-stone-muted);
  margin: 4px 0 0;
  max-width: 600px;
  line-height: 1.6;
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
  -webkit-backdrop-filter: blur(8px);
  border: 1px solid var(--pt-border-color);
  box-shadow: var(--chatops-shadow-sm);
  transition:
    transform 200ms cubic-bezier(0.16, 1, 0.3, 1),
    box-shadow 200ms cubic-bezier(0.16, 1, 0.3, 1);
  overflow: hidden;
}

/* Distinct top accent stripes per metric */
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
  background: linear-gradient(
    90deg,
    var(--chatops-brand) 0%,
    color-mix(in oklab, var(--chatops-brand) 30%, transparent) 100%
  );
}
.stat-chip--success::before {
  background: linear-gradient(
    90deg,
    oklch(0.65 0.13 145) 0%,
    color-mix(in oklab, oklch(0.65 0.13 145) 30%, transparent) 100%
  );
}
.stat-chip--warning::before {
  background: linear-gradient(
    90deg,
    oklch(0.74 0.15 70) 0%,
    color-mix(in oklab, oklch(0.74 0.15 70) 30%, transparent) 100%
  );
}

.stat-chip:hover {
  transform: translateY(-2px);
  box-shadow: var(--chatops-shadow-md);
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
  font-weight: 500;
}

.stat-value {
  font-size: 26px;
  font-weight: 700;
  color: var(--pt-text-primary);
  line-height: 1.05;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.01em;
}

.stat-unit {
  font-size: 14px;
  font-weight: 500;
  color: var(--chatops-stone-muted);
  margin-left: 4px;
}

.glass-card {
  padding: 24px;
  border-radius: var(--chatops-radius-md);
  background: color-mix(in oklab, var(--pt-bg-surface) 82%, transparent);
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  border: 1px solid var(--pt-border-color);
  box-shadow: var(--chatops-shadow-sm);
  transition: box-shadow 200ms ease;
}

.glass-card:hover {
  box-shadow: var(--chatops-shadow-md);
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
  letter-spacing: -0.01em;
}

.section-desc {
  font-size: 13px;
  color: var(--chatops-stone-muted);
  margin: 0;
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

.filter-bar :deep(.el-form-item__label),
.filter-bar :deep(.el-input__inner::placeholder),
.filter-bar :deep(.el-select__placeholder) {
  color: var(--chatops-stone-muted);
}

.filter-item {
  min-width: 200px;
}

.filter-item--date {
  min-width: 360px;
}

.filter-item--search {
  width: 240px;
}

.filter-item :deep(.el-input__wrapper),
.filter-item :deep(.el-select__wrapper) {
  border-radius: 999px;
}

.filter-item--date :deep(.el-input__wrapper) {
  border-radius: 14px;
}

.audit-table :deep(.el-table) {
  background: transparent;
  --el-table-row-hover-bg-color: color-mix(in oklab, var(--chatops-brand) 4%, transparent);
}

.audit-table :deep(.el-table tr) {
  background: transparent;
  transition: background 200ms ease;
}

.audit-table :deep(.el-table th.el-table__cell) {
  background: color-mix(in oklab, var(--pt-text-primary) 4%, transparent);
  color: var(--chatops-stone-muted);
  font-weight: 500;
  font-size: 12.5px;
  letter-spacing: 0.02em;
  text-transform: uppercase;
}

.audit-table :deep(.el-tag) {
  border-radius: var(--chatops-radius-sm);
  font-weight: 600;
}

.meta-text {
  font-size: 13px;
  color: var(--chatops-stone-muted);
  font-variant-numeric: tabular-nums;
}

.user-id {
  font-family: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  font-size: 13px;
  color: var(--chatops-stone-muted);
}

.cmd-badge {
  background: color-mix(in oklab, var(--chatops-brand) 10%, transparent);
  color: var(--chatops-brand);
  padding: 4px 10px;
  border-radius: var(--chatops-radius-sm);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  font-weight: 500;
}

.latency {
  font-variant-numeric: tabular-nums;
  color: var(--chatops-stone-muted);
  font-size: 13px;
}

.high-latency {
  color: #ef4444;
  font-weight: 600;
}

.args-expand {
  padding: 16px 20px;
  margin: 8px 16px 16px;
  border-radius: 12px;
  background: color-mix(in oklab, var(--pt-bg-base) 80%, transparent);
  border: 1px solid color-mix(in oklab, var(--pt-border-color) 70%, transparent);
}

.args-header {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.args-header h4 {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
  color: var(--pt-text-primary);
}

.redacted-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.args-json {
  margin: 0;
  padding: 14px;
  background: color-mix(in oklab, var(--pt-bg-surface) 90%, transparent);
  border: 1px solid color-mix(in oklab, var(--pt-border-color) 70%, transparent);
  border-radius: 10px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 13px;
  line-height: 1.6;
  color: var(--pt-text-primary);
  white-space: pre-wrap;
  word-wrap: break-word;
}

.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px solid color-mix(in oklab, var(--pt-border-color) 60%, transparent);
}
</style>
