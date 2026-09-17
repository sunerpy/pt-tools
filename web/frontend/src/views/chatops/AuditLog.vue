<script setup lang="ts">
import { type AuditLog, chatopsApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtKpiBar from "@/components/ui/PtKpiBar.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { ElMessage } from "element-plus";
import { computed, onMounted, reactive, ref } from "vue";

const CHANNEL_LABELS: Record<string, string> = {
  telegram: "Telegram",
  qq: "QQ",
  wecom: "企业微信",
  webhook: "Webhook",
};

const RESULT_TONES: Record<string, "ok" | "warn" | "dang" | "neutral"> = {
  success: "ok",
  denied: "warn",
  error: "dang",
};

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

const hasFilter = computed(
  () =>
    Boolean(filters.dateRange) ||
    filters.channelType.length > 0 ||
    filters.result.length > 0 ||
    Boolean(filters.command),
);

/*
 * 三个读数不带趋势序列：/chatops/audit/stats 只返回当前值，
 * PtKpiBar 的柱子要真实历史才画，没有就别编（组件注释里的规矩）。
 */
const kpiItems = computed(() => [
  {
    label: "今日执行命令",
    value: stats.todayCount,
    unit: " 条",
    icon: "activity",
  },
  {
    label: "整体成功率",
    value: stats.successRate.toFixed(2),
    unit: "%",
    icon: "circle-check",
    delta: stats.successRate >= 95 ? "健康" : stats.successRate >= 80 ? "偏低" : "异常",
    deltaTone: (stats.successRate >= 95 ? "ok" : stats.successRate >= 80 ? "warn" : "dang") as
      | "ok"
      | "warn"
      | "dang",
  },
  {
    label: "最高延迟",
    value: stats.maxLatencyMs,
    unit: "ms",
    icon: "timer",
    delta: stats.maxLatencyMs > 1000 ? "偏慢" : undefined,
    deltaTone: "warn" as const,
  },
]);

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
}

function handleFilterChange() {
  pagination.page = 1;
  fetchAuditLogs();
}

function handlePageChange(page: number) {
  pagination.page = page;
  fetchAuditLogs();
}

function formatDate(dateStr: string) {
  if (!dateStr) return "-";
  return new Date(dateStr).toLocaleString("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

function formatJson(jsonStr?: string) {
  if (!jsonStr) return "无参数";
  try {
    return JSON.stringify(JSON.parse(jsonStr), null, 2);
  } catch {
    return jsonStr;
  }
}

function channelLabel(type: string) {
  return CHANNEL_LABELS[type] || type;
}

function resultTone(result: string) {
  return RESULT_TONES[result?.toLowerCase()] || "neutral";
}
</script>

<template>
  <div class="audit-page">
    <PtKpiBar :items="kpiItems" />

    <PtPanel
      v-loading="loading"
      title="操作审计"
      icon="scroll-text"
      :count="`${pagination.total} 条`"
      padding="none">
      <PtToolbar>
        <el-date-picker
          v-model="filters.dateRange"
          type="datetimerange"
          range-separator="至"
          start-placeholder="开始时间"
          end-placeholder="结束时间"
          format="YYYY-MM-DD HH:mm:ss"
          value-format="YYYY-MM-DDTHH:mm:ssZ"
          class="f-date"
          @change="handleFilterChange" />

        <el-select
          v-model="filters.channelType"
          placeholder="全部通道"
          multiple
          collapse-tags
          collapse-tags-tooltip
          clearable
          class="f-sel"
          @change="handleFilterChange">
          <el-option
            v-for="(label, value) in CHANNEL_LABELS"
            :key="value"
            :label="label"
            :value="value" />
        </el-select>

        <el-select
          v-model="filters.result"
          placeholder="全部结果"
          multiple
          collapse-tags
          collapse-tags-tooltip
          clearable
          class="f-sel"
          @change="handleFilterChange">
          <el-option label="成功" value="success" />
          <el-option label="被拒绝" value="denied" />
          <el-option label="出错" value="error" />
        </el-select>

        <el-input
          v-model="filters.command"
          placeholder="搜索命令"
          clearable
          class="f-search"
          @keyup.enter="handleFilterChange"
          @clear="handleFilterChange">
          <template #prefix>
            <PtIcon name="search" :size="14" />
          </template>
        </el-input>

        <template #right>
          <el-button size="small" :loading="loading" @click="fetchAuditLogs">
            <PtIcon name="refresh-cw" :size="14" /><span>刷新</span>
          </el-button>
        </template>
      </PtToolbar>

      <el-table :data="auditLogs" class="pt-grid" row-key="id" style="width: 100%">
        <template #empty>
          <PtDataState
            :state="hasFilter ? 'zero' : 'empty'"
            dense
            :sub="
              hasFilter ? '换个时间段或清掉筛选再看' : '机器人执行过的每条命令都会记录在这里'
            " />
        </template>

        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="args">
              <div class="args__head">
                <span class="args__title">命令参数</span>
                <span class="args__redacted">
                  <PtIcon name="lock" :size="12" />
                  <span>敏感字段已脱敏</span>
                </span>
              </div>
              <pre class="args__json">{{ formatJson(row.args_json) }}</pre>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="created_at" label="时间" width="170" class-name="pt-cell-muted">
          <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
        </el-table-column>

        <el-table-column prop="channel_type" label="通道" width="110">
          <template #default="{ row }">
            <PtTag>{{ channelLabel(row.channel_type) }}</PtTag>
          </template>
        </el-table-column>

        <el-table-column prop="channel_user_id" label="触发用户" min-width="140">
          <template #default="{ row }">
            <code class="uid">{{ row.channel_user_id || "-" }}</code>
          </template>
        </el-table-column>

        <el-table-column prop="command" label="命令" min-width="140" class-name="pt-cell-strong">
          <template #default="{ row }">
            <code class="cmd">{{ row.command }}</code>
          </template>
        </el-table-column>

        <el-table-column prop="result" label="结果" width="96">
          <template #default="{ row }">
            <PtStatusPill :tone="resultTone(row.result)" size="sm">
              {{ row.result }}
            </PtStatusPill>
          </template>
        </el-table-column>

        <el-table-column
          prop="latency_ms"
          label="延迟"
          width="96"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num">
          <template #default="{ row }">
            <span :class="{ 'lat--slow': row.latency_ms > 1000 }">{{ row.latency_ms }} ms</span>
          </template>
        </el-table-column>
      </el-table>

      <template v-if="pagination.total > 0" #footer>
        <span class="pt-foot-note">展开一行可以看脱敏后的完整命令参数</span>
        <el-pagination
          v-model:current-page="pagination.page"
          class="pt-pager"
          :page-size="pagination.pageSize"
          :total="pagination.total"
          :pager-count="5"
          layout="prev, pager, next"
          @current-change="handlePageChange" />
      </template>
    </PtPanel>
  </div>
</template>

<style scoped>
.audit-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
}

.f-date {
  width: 340px;
}

.f-sel {
  width: 150px;
}

.f-search {
  width: 200px;
}

.uid {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 命令自己带 / 前缀，做成一枚 primary 底的小胶囊，扫一列就能看出执行了什么 */
.cmd {
  padding: 1px 6px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-p);
  background: var(--pt-p-soft);
  border-radius: var(--pt-r-sm);
}

.lat--slow {
  font-weight: 600;
  color: var(--pt-dang);
}

.args__head {
  display: flex;
  gap: var(--pt-space-3);
  align-items: center;
  margin-bottom: var(--pt-space-2);
}

.args__title {
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t2);
}

.args__redacted {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  font-size: var(--pt-fz-label);
  color: var(--pt-warn);
}

/* 参数体自己滚：一条 /search 的参数可以有几十行，不该把整张表撑开 */
.args__json {
  max-height: 260px;
  margin: 0;
  overflow: auto;
  padding: var(--pt-space-3);
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
  white-space: pre-wrap;
  word-break: break-word;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-sm);
}

@media (max-width: 768px) {
  .f-date,
  .f-sel,
  .f-search {
    width: 100%;
  }
}
</style>
