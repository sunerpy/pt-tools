<script setup lang="ts">
import { type AuditLog, chatopsApi } from "@/api";
import {
  ChatDotRound,
  ChatSquare,
  Check,
  CircleClose,
  Connection,
  DataLine,
  Document,
  Lock,
  Refresh,
  Search,
  Timer,
  WarningFilled,
} from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { computed, onMounted, reactive, ref } from "vue";

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

// 通道类型映射
const channelTagType: Record<string, "" | "success" | "warning" | "info" | "danger"> = {
  telegram: "info",
  qq: "success",
  qq_onebot: "success",
  wecom: "warning",
  wecom_webhook: "warning",
  webhook: "danger",
};

const channelLabelMap: Record<string, string> = {
  telegram: "Telegram",
  qq: "QQ",
  qq_onebot: "QQ",
  wecom: "WeCom",
  wecom_webhook: "WeCom",
  webhook: "Webhook",
};

const channelIconMap: Record<string, unknown> = {
  telegram: ChatDotRound,
  qq: ChatSquare,
  qq_onebot: ChatSquare,
  wecom: Connection,
  wecom_webhook: Connection,
  webhook: Document,
};

const successRateColor = computed(() => {
  const rate = stats.successRate;
  if (rate >= 0.9) return "var(--pt-color-success)";
  if (rate >= 0.7) return "var(--pt-color-warning)";
  return "var(--pt-color-danger)";
});

onMounted(() => {
  fetchAuditLogs();
});

async function fetchAuditLogs() {
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

    const res = await chatopsApi.audit.list(params);

    auditLogs.value = res.items || [];
    pagination.total = res.total || 0;
    stats.todayCount = res.today_count || 0;
    stats.successRate = res.success_rate || 0;
    stats.maxLatencyMs = res.max_latency_ms || 0;
  } catch (err: unknown) {
    ElMessage.error((err as Error).message || "获取审计日志失败");
  } finally {
    loading.value = false;
  }
}

function handleFilterChange() {
  pagination.page = 1;
  fetchAuditLogs();
}

function handleResetFilters() {
  filters.dateRange = null;
  filters.channelType = [];
  filters.result = [];
  filters.command = "";
  pagination.page = 1;
  fetchAuditLogs();
}

function handlePageChange(page: number) {
  pagination.page = page;
  fetchAuditLogs();
}

function handleSizeChange(size: number) {
  pagination.pageSize = size;
  pagination.page = 1;
  fetchAuditLogs();
}

function formatDate(dateStr: string) {
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
}

function formatTimeAgo(dateStr: string) {
  if (!dateStr) return "-";
  const ts = new Date(dateStr).getTime();
  if (!ts) return "-";
  const diffMs = Date.now() - ts;
  if (diffMs < 0) return formatDate(dateStr);
  const sec = Math.floor(diffMs / 1000);
  if (sec < 60) return "刚刚";
  const min = Math.floor(sec / 60);
  if (min < 60) return `${min} 分钟前`;
  const hour = Math.floor(min / 60);
  if (hour < 24) return `${hour} 小时前`;
  const day = Math.floor(hour / 24);
  if (day < 30) return `${day} 天前`;
  return formatDate(dateStr);
}

function formatJson(jsonStr?: string) {
  if (!jsonStr) return "无参数";
  try {
    const obj = JSON.parse(jsonStr);
    return JSON.stringify(obj, null, 2);
  } catch (_e) {
    return jsonStr;
  }
}

function formatSuccessRate(rate: number) {
  return `${(rate * 100).toFixed(1)}%`;
}

function getChannelTagType(type: string) {
  return channelTagType[type] || "info";
}

function getChannelLabel(type: string) {
  return channelLabelMap[type] || type;
}

function getChannelIcon(type: string) {
  return channelIconMap[type] || ChatDotRound;
}

function getResultTagType(result: string): "" | "success" | "warning" | "info" | "danger" {
  const map: Record<string, "" | "success" | "warning" | "info" | "danger"> = {
    success: "success",
    denied: "warning",
    error: "danger",
  };
  return map[result?.toLowerCase()] || "info";
}

function getResultIcon(result: string) {
  const r = result?.toLowerCase();
  if (r === "success") return Check;
  if (r === "denied") return WarningFilled;
  if (r === "error") return CircleClose;
  return Document;
}

function getLatencyClass(ms: number) {
  if (ms > 2000) return "latency latency--danger";
  if (ms > 500) return "latency latency--warning";
  return "latency latency--ok";
}
</script>

<template>
  <div class="page-container chatops-audit-page">
    <div class="page-header">
      <div>
        <h1 class="page-title">操作审计</h1>
        <p class="page-subtitle">查询与追踪 ChatOps 机器人的所有命令执行记录，敏感参数已自动脱敏。</p>
      </div>
      <div class="page-actions">
        <el-button size="default" :loading="loading" @click="fetchAuditLogs">
          <el-icon><Refresh /></el-icon>
          刷新
        </el-button>
      </div>
    </div>

    <!-- 统计卡片 -->
    <div class="stats-grid">
      <div class="stat-card">
        <div class="stat-card-icon">
          <el-icon><DataLine /></el-icon>
        </div>
        <div class="stat-card-info">
          <div class="stat-card-label">今日执行命令</div>
          <div class="stat-card-value">{{ stats.todayCount }}</div>
        </div>
      </div>

      <div class="stat-card stat-card--success">
        <div class="stat-card-icon">
          <el-icon><Check /></el-icon>
        </div>
        <div class="stat-card-info">
          <div class="stat-card-label">整体成功率</div>
          <div class="stat-card-value" :style="{ color: successRateColor }">
            {{ formatSuccessRate(stats.successRate) }}
          </div>
        </div>
      </div>

      <div class="stat-card stat-card--warning">
        <div class="stat-card-icon">
          <el-icon><Timer /></el-icon>
        </div>
        <div class="stat-card-info">
          <div class="stat-card-label">最高延迟</div>
          <div class="stat-card-value">
            {{ stats.maxLatencyMs }}<span class="stat-unit">ms</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 筛选卡片 -->
    <div class="common-card filter-card">
      <div class="common-card-body">
        <el-form :inline="true" class="filter-form" @submit.prevent="handleFilterChange">
          <el-form-item label="时间范围">
            <el-date-picker
              v-model="filters.dateRange"
              type="datetimerange"
              range-separator="至"
              start-placeholder="开始时间"
              end-placeholder="结束时间"
              format="YYYY-MM-DD HH:mm:ss"
              value-format="YYYY-MM-DDTHH:mm:ssZ"
              style="width: 360px"
              @change="handleFilterChange" />
          </el-form-item>

          <el-form-item label="通道">
            <el-select
              v-model="filters.channelType"
              placeholder="全部通道"
              multiple
              collapse-tags
              collapse-tags-tooltip
              clearable
              style="width: 200px"
              @change="handleFilterChange">
              <el-option label="Telegram" value="telegram" />
              <el-option label="QQ" value="qq" />
              <el-option label="企业微信" value="wecom" />
              <el-option label="Webhook" value="webhook" />
            </el-select>
          </el-form-item>

          <el-form-item label="结果">
            <el-select
              v-model="filters.result"
              placeholder="全部结果"
              multiple
              collapse-tags
              collapse-tags-tooltip
              clearable
              style="width: 180px"
              @change="handleFilterChange">
              <el-option label="成功" value="success" />
              <el-option label="拒绝" value="denied" />
              <el-option label="失败" value="error" />
            </el-select>
          </el-form-item>

          <el-form-item label="命令">
            <el-input
              v-model="filters.command"
              placeholder="搜索命令..."
              clearable
              style="width: 200px"
              @keyup.enter="handleFilterChange"
              @clear="handleFilterChange">
              <template #prefix>
                <el-icon><Search /></el-icon>
              </template>
            </el-input>
          </el-form-item>

          <el-form-item class="filter-actions">
            <el-button type="primary" :loading="loading" @click="handleFilterChange">
              查询
            </el-button>
            <el-button @click="handleResetFilters">重置</el-button>
          </el-form-item>
        </el-form>
      </div>
    </div>

    <!-- 日志列表卡片 -->
    <div class="table-card audit-table-card" v-loading="loading">
      <div class="table-card-header">
        <div class="table-card-header-title">
          <el-icon class="header-icon"><Document /></el-icon>
          <span>审计记录</span>
          <el-tag type="info" size="small" effect="plain" round class="count-tag">
            共 {{ pagination.total }} 条
          </el-tag>
        </div>
        <div class="table-card-header-actions">
          <span class="hint-text">
            <el-icon><Lock /></el-icon>
            敏感参数已脱敏
          </span>
        </div>
      </div>

      <div class="table-wrapper">
        <el-table
          :data="auditLogs"
          class="pt-table audit-table"
          row-key="id"
          :header-cell-style="{ background: 'var(--pt-bg-secondary)', fontWeight: 600 }"
          :empty-text="loading ? '加载中...' : '暂无符合条件的审计记录'">
          <el-table-column type="expand">
            <template #default="{ row }">
              <div class="args-expand">
                <div class="args-header">
                  <h4>命令参数</h4>
                  <el-tag size="small" round type="warning" effect="plain">
                    <el-icon><Lock /></el-icon>
                    部分敏感数据已脱敏
                  </el-tag>
                </div>
                <pre class="args-json">{{ formatJson(row.args_json) }}</pre>
              </div>
            </template>
          </el-table-column>

          <el-table-column prop="created_at" label="时间" min-width="170">
            <template #default="{ row }">
              <el-tooltip :content="formatDate(row.created_at)" placement="top">
                <div class="time-cell">
                  <span class="time-relative">{{ formatTimeAgo(row.created_at) }}</span>
                  <span class="time-abs">{{ formatDate(row.created_at) }}</span>
                </div>
              </el-tooltip>
            </template>
          </el-table-column>

          <el-table-column prop="channel_type" label="通道" width="130">
            <template #default="{ row }">
              <el-tag
                :type="getChannelTagType(row.channel_type)"
                size="small"
                effect="light"
                round>
                <el-icon class="tag-icon">
                  <component :is="getChannelIcon(row.channel_type)" />
                </el-icon>
                {{ getChannelLabel(row.channel_type) }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column prop="channel_user_id" label="用户" min-width="140">
            <template #default="{ row }">
              <code class="user-id-mono">{{ row.channel_user_id }}</code>
            </template>
          </el-table-column>

          <el-table-column prop="command" label="命令" min-width="160">
            <template #default="{ row }">
              <code class="cmd-badge">{{ row.command }}</code>
            </template>
          </el-table-column>

          <el-table-column prop="result" label="结果" width="120" align="center">
            <template #default="{ row }">
              <el-tag :type="getResultTagType(row.result)" size="small" effect="light" round>
                <el-icon class="tag-icon">
                  <component :is="getResultIcon(row.result)" />
                </el-icon>
                {{ row.result?.toUpperCase() }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column prop="latency_ms" label="延迟" width="110" align="right">
            <template #default="{ row }">
              <span :class="getLatencyClass(row.latency_ms)">{{ row.latency_ms }} ms</span>
            </template>
          </el-table-column>
        </el-table>

        <div v-if="pagination.total > 0" class="pagination-container">
          <el-pagination
            v-model:current-page="pagination.page"
            v-model:page-size="pagination.pageSize"
            :page-sizes="[10, 30, 50, 100]"
            :total="pagination.total"
            layout="total, sizes, prev, pager, next, jumper"
            @size-change="handleSizeChange"
            @current-change="handlePageChange" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@import "@/styles/common-page.css";
@import "@/styles/table-page.css";

.chatops-audit-page {
  padding: var(--pt-space-4) var(--pt-space-5) var(--pt-space-8);
}

/* 统计卡片网格 */
.stats-grid {
  display: grid;
  grid-template-columns: repeat(1, 1fr);
  gap: var(--pt-space-4);
  margin-bottom: var(--pt-space-5);
}

@media (min-width: 720px) {
  .stats-grid {
    grid-template-columns: repeat(3, 1fr);
  }
}

.stat-card {
  display: flex;
  align-items: center;
  gap: var(--pt-space-4);
  padding: var(--pt-space-5);
  background: var(--pt-bg-surface-raised);
  border: 1px solid var(--pt-border-color);
  border-radius: var(--pt-radius-xl);
  box-shadow: var(--pt-shadow-sm);
  transition:
    box-shadow var(--pt-transition-normal),
    border-color var(--pt-transition-normal),
    transform var(--pt-transition-normal);
  position: relative;
  overflow: hidden;
}

.stat-card::before {
  content: "";
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: var(--pt-color-primary);
  opacity: 0.85;
}

.stat-card--success::before {
  background: var(--pt-color-success);
}

.stat-card--warning::before {
  background: var(--pt-color-warning);
}

.stat-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--pt-shadow-md);
  border-color: color-mix(in srgb, var(--pt-color-primary) 25%, var(--pt-border-color));
}

.stat-card-icon {
  display: grid;
  place-items: center;
  width: 48px;
  height: 48px;
  border-radius: var(--pt-radius-lg);
  background: var(--pt-bg-accent-soft);
  color: var(--pt-color-primary);
  font-size: 22px;
  flex-shrink: 0;
}

.stat-card--success .stat-card-icon {
  background: color-mix(in srgb, var(--pt-color-success) 14%, transparent);
  color: var(--pt-color-success);
}

.stat-card--warning .stat-card-icon {
  background: color-mix(in srgb, var(--pt-color-warning) 16%, transparent);
  color: var(--pt-color-warning);
}

.stat-card-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.stat-card-label {
  font-size: var(--pt-text-sm);
  color: var(--pt-text-secondary);
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.stat-card-value {
  font-size: var(--pt-text-3xl);
  font-weight: 800;
  color: var(--pt-text-primary);
  line-height: 1.05;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.02em;
}

.stat-unit {
  font-size: var(--pt-text-base);
  font-weight: 500;
  color: var(--pt-text-secondary);
  margin-left: 4px;
}

/* 筛选卡片 */
.filter-card {
  margin-bottom: var(--pt-space-5);
}

.filter-form {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--pt-space-2);
}

.filter-actions {
  margin-left: auto;
}

/* 表格卡片 */
.audit-table-card {
  margin-bottom: var(--pt-space-5);
}

.table-card-header-title {
  display: flex;
  align-items: center;
  gap: var(--pt-space-2);
}

.header-icon {
  color: var(--pt-color-primary);
  font-size: 18px;
}

.count-tag {
  margin-left: var(--pt-space-1);
}

.hint-text {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: var(--pt-text-xs);
  color: var(--pt-text-tertiary);
}

/* 表格 */
.audit-table :deep(.el-table) {
  --el-table-row-hover-bg-color: color-mix(
    in srgb,
    var(--pt-color-primary) 5%,
    var(--pt-bg-surface-muted)
  );
}

.audit-table :deep(.el-table .el-table__expand-icon) {
  color: var(--pt-text-secondary);
}

.audit-table :deep(.el-table .el-table__expand-icon.el-table__expand-icon--expanded) {
  color: var(--pt-color-primary);
}

/* 单元格 */
.time-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.time-relative {
  font-size: var(--pt-text-sm);
  color: var(--pt-text-primary);
  font-weight: 500;
}

.time-abs {
  font-size: var(--pt-text-xs);
  color: var(--pt-text-tertiary);
  font-variant-numeric: tabular-nums;
}

.user-id-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: var(--pt-text-sm);
  color: var(--pt-text-secondary);
  background: var(--pt-bg-secondary);
  padding: 2px 8px;
  border-radius: var(--pt-radius-sm);
}

.cmd-badge {
  display: inline-block;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: var(--pt-text-sm);
  font-weight: 600;
  color: var(--pt-color-primary);
  background: var(--pt-bg-accent-soft);
  padding: 4px 10px;
  border-radius: var(--pt-radius-md);
  border: 1px solid color-mix(in srgb, var(--pt-color-primary) 18%, var(--pt-border-color));
}

.tag-icon {
  margin-right: 2px;
  vertical-align: -1px;
}

.latency {
  font-variant-numeric: tabular-nums;
  font-size: var(--pt-text-sm);
  font-weight: 600;
}

.latency--ok {
  color: var(--pt-text-secondary);
}

.latency--warning {
  color: var(--pt-color-warning);
}

.latency--danger {
  color: var(--pt-color-danger);
}

/* 展开内容 */
.args-expand {
  padding: var(--pt-space-4);
  margin: var(--pt-space-2) var(--pt-space-4) var(--pt-space-3);
  border-radius: var(--pt-radius-lg);
  background: var(--pt-bg-secondary);
  border: 1px solid var(--pt-border-color);
}

.args-header {
  display: flex;
  align-items: center;
  gap: var(--pt-space-3);
  margin-bottom: var(--pt-space-3);
}

.args-header h4 {
  margin: 0;
  font-size: var(--pt-text-sm);
  font-weight: 600;
  color: var(--pt-text-primary);
}

.args-header :deep(.el-tag) {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.args-json {
  margin: 0;
  padding: var(--pt-space-3);
  background: var(--pt-bg-surface);
  border: 1px solid var(--pt-border-color);
  border-radius: var(--pt-radius-md);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: var(--pt-text-sm);
  line-height: 1.6;
  color: var(--pt-text-primary);
  white-space: pre-wrap;
  word-wrap: break-word;
  max-height: 360px;
  overflow: auto;
}

/* 分页 */
.pagination-container {
  display: flex;
  justify-content: flex-end;
  padding: var(--pt-space-4) var(--pt-space-5);
  border-top: 1px solid var(--pt-border-color);
}

/* 暗色模式 */
html.dark .stat-card,
html.dark .filter-card,
html.dark .audit-table-card {
  background: var(--pt-bg-surface);
}

html.dark .args-expand {
  background: var(--pt-bg-tertiary);
}

html.dark .cmd-badge,
html.dark .user-id-mono {
  background: color-mix(in srgb, var(--pt-color-primary) 12%, var(--pt-bg-surface));
}

/* 移动端 */
@media (max-width: 640px) {
  .chatops-audit-page {
    padding: var(--pt-space-3);
  }

  .filter-form {
    flex-direction: column;
    align-items: stretch;
  }

  .filter-form :deep(.el-form-item) {
    margin-right: 0;
    margin-bottom: var(--pt-space-2);
  }

  .filter-actions {
    margin-left: 0;
  }

  .pagination-container {
    justify-content: center;
    padding: var(--pt-space-3);
  }

  .pagination-container :deep(.el-pagination) {
    flex-wrap: wrap;
    justify-content: center;
  }
}
</style>
