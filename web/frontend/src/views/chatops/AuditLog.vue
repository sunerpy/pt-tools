<template>
  <div class="audit-log-page">
    <div class="page-header">
      <h1 class="page-title">操作审计日志</h1>
      <p class="page-subtitle">查询与追踪 ChatOps 机器人的所有执行记录</p>
    </div>

    <!-- 顶部统计 Chips -->
    <el-row :gutter="20" class="stats-row">
      <el-col :span="8">
        <div class="stat-chip">
          <div class="stat-icon">
            <el-icon><DataLine /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">今日执行命令数</div>
            <div class="stat-value">{{ stats.todayCount }}</div>
          </div>
        </div>
      </el-col>
      <el-col :span="8">
        <div class="stat-chip">
          <div class="stat-icon success-icon">
            <el-icon><Check /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">整体成功率</div>
            <div class="stat-value">{{ formatSuccessRate(stats.successRate) }}</div>
          </div>
        </div>
      </el-col>
      <el-col :span="8">
        <div class="stat-chip">
          <div class="stat-icon warning-icon">
            <el-icon><Timer /></el-icon>
          </div>
          <div class="stat-info">
            <div class="stat-label">最高延迟 (ms)</div>
            <div class="stat-value">{{ stats.maxLatencyMs }}</div>
          </div>
        </div>
      </el-col>
    </el-row>

    <el-card class="audit-card" shadow="never">
      <!-- 筛选栏 -->
      <div class="filter-bar">
        <el-date-picker
          v-model="filters.dateRange"
          type="datetimerange"
          range-separator="至"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
          format="YYYY-MM-DD HH:mm:ss"
          value-format="YYYY-MM-DDTHH:mm:ssZ"
          class="filter-item"
          @change="handleFilterChange" />

        <el-select
          v-model="filters.channelType"
          placeholder="通道类型"
          multiple
          collapse-tags
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
          class="filter-item search-input"
          @keyup.enter="handleFilterChange"
          @clear="handleFilterChange">
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>
      </div>

      <!-- 数据表 -->
      <el-table v-loading="loading" :data="auditLogs" class="audit-table" row-key="id">
        <!-- 展开列 (Args) -->
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="args-expand">
              <div class="args-header">
                <h4>命令参数</h4>
                <el-tag size="small" type="warning" effect="plain" class="redacted-tag">
                  <el-icon><Lock /></el-icon> 部分敏感数据已脱敏 (Redacted)
                </el-tag>
              </div>
              <pre class="args-json">{{ formatJson(row.args_json) }}</pre>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="created_at" label="时间" min-width="170">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>

        <el-table-column prop="channel_type" label="通道" width="100">
          <template #default="{ row }">
            <el-tag :type="getChannelTagType(row.channel_type)" size="small">
              {{ row.channel_type }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="channel_user_id" label="触发用户" min-width="120">
          <template #default="{ row }">
            <span class="user-id">{{ row.channel_user_id }}</span>
          </template>
        </el-table-column>

        <el-table-column prop="command" label="命令" min-width="120">
          <template #default="{ row }">
            <code class="cmd-badge">{{ row.command }}</code>
          </template>
        </el-table-column>

        <el-table-column prop="result" label="结果" width="100">
          <template #default="{ row }">
            <el-tag :type="getResultTagType(row.result)" size="small" effect="light">
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

      <!-- 分页 -->
      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="pagination.page"
          :page-size="pagination.pageSize"
          :total="pagination.total"
          layout="total, prev, pager, next"
          @current-change="handlePageChange" />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from "vue";
import { DataLine, Check, Timer, Search, Lock } from "@element-plus/icons-vue";
import { chatopsApi, type AuditLog } from "@/api";
import { ElMessage } from "element-plus";

// State
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

// Lifecycle
onMounted(() => {
  fetchAuditLogs();
});

// Methods
const fetchAuditLogs = async () => {
  loading.value = true;
  try {
    const params = new URLSearchParams();

    // Pagination
    params.append("page", pagination.page.toString());
    params.append("page_size", pagination.pageSize.toString());

    // Filters
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
  } catch (err: any) {
    ElMessage.error(err.message || "获取审计日志失败");
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

// Formatters
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
  } catch (e) {
    return jsonStr;
  }
};

const formatSuccessRate = (rate: number) => {
  return `${(rate * 100).toFixed(1)}%`;
};

const getChannelTagType = (type: string) => {
  const map: Record<string, string> = {
    telegram: "",
    qq: "warning",
    wecom: "success",
    webhook: "info",
  };
  return map[type] || "info";
};

const getResultTagType = (result: string) => {
  const map: Record<string, string> = {
    success: "success",
    denied: "warning",
    error: "danger",
  };
  return map[result.toLowerCase()] || "info";
};
</script>

<style scoped>
.audit-log-page {
  padding: 24px;
  background-color: var(--pt-bg-base, #f8fafc);
  min-height: calc(100vh - 60px);
}

.page-header {
  margin-bottom: 24px;
}
.page-title {
  margin: 0 0 8px 0;
  font-size: 24px;
  font-weight: 600;
  color: var(--pt-text-primary, #1e293b);
  letter-spacing: -0.02em;
}
.page-subtitle {
  margin: 0;
  font-size: 14px;
  color: var(--pt-text-secondary, #64748b);
}

.stats-row {
  margin-bottom: 24px;
}

.stat-chip {
  display: flex;
  align-items: center;
  padding: 20px;
  background: var(--pt-bg-surface, #ffffff);
  border-radius: 16px;
  box-shadow:
    0 4px 6px -1px rgba(0, 0, 0, 0.05),
    0 2px 4px -1px rgba(0, 0, 0, 0.03);
  border: 1px solid rgba(0, 0, 0, 0.05);
  transition:
    transform 0.2s ease,
    box-shadow 0.2s ease;
}
.stat-chip:hover {
  transform: translateY(-2px);
  box-shadow:
    0 10px 15px -3px rgba(0, 0, 0, 0.05),
    0 4px 6px -2px rgba(0, 0, 0, 0.025);
}
.stat-icon {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  border-radius: 12px;
  background: rgba(196, 99, 30, 0.1);
  color: #c4631e; /* pt-tools brand warm copper */
  font-size: 24px;
  margin-right: 16px;
}
.success-icon {
  background: rgba(16, 185, 129, 0.1);
  color: #10b981;
}
.warning-icon {
  background: rgba(245, 158, 11, 0.1);
  color: #f59e0b;
}

.stat-info {
  display: flex;
  flex-direction: column;
}
.stat-label {
  font-size: 13px;
  color: var(--pt-text-secondary, #64748b);
  font-weight: 500;
  margin-bottom: 4px;
}
.stat-value {
  font-size: 24px;
  font-weight: 700;
  color: var(--pt-text-primary, #1e293b);
  line-height: 1.1;
}

.audit-card {
  border-radius: 16px;
  border: 1px solid rgba(0, 0, 0, 0.05);
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.05);
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(12px);
}

.filter-bar {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 20px;
}
.filter-item {
  min-width: 200px;
}
.search-input {
  width: 240px;
}

.audit-table {
  width: 100%;
  border-radius: 8px;
  overflow: hidden;
}
.user-id {
  font-family: monospace;
  color: var(--pt-text-secondary, #64748b);
}
.cmd-badge {
  background: rgba(196, 99, 30, 0.08);
  color: #c4631e;
  padding: 4px 8px;
  border-radius: 6px;
  font-family: monospace;
  font-size: 13px;
}
.latency {
  font-variant-numeric: tabular-nums;
  color: var(--pt-text-secondary, #64748b);
}
.high-latency {
  color: #ef4444;
  font-weight: 500;
}

.args-expand {
  padding: 16px 24px;
  background: rgba(0, 0, 0, 0.02);
  border-radius: 8px;
  margin: 8px 16px;
}
.args-header {
  display: flex;
  align-items: center;
  gap: 16px;
  margin-bottom: 12px;
}
.args-header h4 {
  margin: 0;
  font-size: 14px;
  color: var(--pt-text-primary, #1e293b);
}
.redacted-tag {
  display: flex;
  align-items: center;
  gap: 4px;
}
.args-json {
  margin: 0;
  padding: 12px;
  background: var(--pt-bg-base, #f8fafc);
  border: 1px solid rgba(0, 0, 0, 0.05);
  border-radius: 6px;
  font-family: monospace;
  font-size: 13px;
  color: var(--pt-text-primary, #1e293b);
  white-space: pre-wrap;
  word-wrap: break-word;
}

.pagination-wrapper {
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
  padding-top: 16px;
  border-top: 1px solid rgba(0, 0, 0, 0.05);
}
</style>
