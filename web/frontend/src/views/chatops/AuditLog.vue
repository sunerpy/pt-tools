<script setup lang="ts">
import { type AuditLog, chatopsApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtKpiBar from "@/components/ui/PtKpiBar.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
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

const isMobile = useIsMobile();
const auditLogs = ref<AuditLog[]>([]);
/** 移动端展开了参数的行 id（桌面这活儿由 el-table 的 expand 列自己管） */
const expandedIds = ref<number[]>([]);

const stats = reactive({
  todayCount: 0,
  successRate: 0,
  maxLatencyMs: 0,
});

/** 统计接口是不是没拿到：0 = 正常，1 = 失败（喂给 useDataState 的 failed） */
const statsFailed = ref(0);

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

/**
 * 六态状态机（设计文档 §5）。
 *
 * 以前这里只有一个 loading ref，失败就弹个 toast —— 两秒后 toast 没了，表格停在
 * 「还没有数据」上，用户看到的是「机器人一条命令都没执行过」，而真相是请求失败了。
 * 审计页尤其不能这样：日志为空是「没人用过机器人」，请求失败是「查不到证据」。
 *
 * failed 接的是统计接口：它只喂上面的 KPI 条，挂了不该把已经拿到的日志一起丢掉，
 * 所以记一个失败数让状态落到 partial，表格照常渲染。
 */
const { loading, state, errorText, run, hasPartialBanner } = useDataState({
  filtered: () => hasFilter.value,
  failed: () => statsFailed.value,
});

/** 状态块的副标题：失败给真实错误，partial 说清缺了哪一半，空态给下一步 */
const stateSub = computed(() => {
  if (state.value === "error" || state.value === "perm") return errorText.value;
  if (state.value === "partial") return "统计读数没拿到，日志列表本身确实是空的";
  return hasFilter.value ? "换个时间段或清掉筛选再看" : "机器人执行过的每条命令都会记录在这里";
});

/*
 * 三个读数不带趋势序列：/chatops/audit/stats 只返回当前值，
 * PtKpiBar 的柱子要真实历史才画，没有就别编（组件注释里的规矩）。
 * 统计没拿到时读数一律画「-」：摆一个 0 或上一轮的旧值等于报了个假数。
 */
const kpiItems = computed(() => {
  const ok = statsFailed.value === 0;
  return [
    {
      label: "今日执行命令",
      value: ok ? stats.todayCount : "-",
      unit: ok ? " 条" : undefined,
      icon: "activity",
    },
    {
      label: "整体成功率",
      value: ok ? stats.successRate.toFixed(2) : "-",
      unit: ok ? "%" : undefined,
      icon: "circle-check",
      delta: ok
        ? stats.successRate >= 95
          ? "健康"
          : stats.successRate >= 80
            ? "偏低"
            : "异常"
        : undefined,
      deltaTone: (stats.successRate >= 95 ? "ok" : stats.successRate >= 80 ? "warn" : "dang") as
        | "ok"
        | "warn"
        | "dang",
    },
    {
      label: "最高延迟",
      value: ok ? stats.maxLatencyMs : "-",
      unit: ok ? "ms" : undefined,
      icon: "timer",
      delta: ok && stats.maxLatencyMs > 1000 ? "偏慢" : undefined,
      deltaTone: "warn" as const,
    },
  ];
});

onMounted(() => {
  fetchAuditLogs();
});

async function fetchAuditLogs() {
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

  /*
   * 两个数据源分开判：原来是 Promise.all，统计接口一挂整页就当失败，
   * 明明拿到手的日志也被丢掉。改成 allSettled —— 列表是主数据源，它失败才算失败；
   * 统计失败只记 failed，让状态落到 partial。
   */
  const data = await run(async () => {
    const [listRes, statsRes] = await Promise.allSettled([
      chatopsApi.audit.list(params),
      chatopsApi.audit.stats(),
    ]);

    if (statsRes.status === "fulfilled") {
      statsFailed.value = 0;
      stats.todayCount = statsRes.value.today_count || 0;
      stats.successRate = statsRes.value.success_rate || 0;
      stats.maxLatencyMs = statsRes.value.max_latency_ms || 0;
    } else {
      statsFailed.value = 1;
    }

    if (listRes.status === "rejected") throw listRes.reason;
    return listRes.value;
  });

  if (!data) {
    // 失败时清空：留着上一次的日志配一个「加载失败」的状态块更让人误解
    auditLogs.value = [];
    pagination.total = 0;
    expandedIds.value = [];
    ElMessage.error(errorText.value || "获取审计日志失败");
    return;
  }

  auditLogs.value = data.items || [];
  pagination.total = data.total || 0;
  expandedIds.value = [];
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

function isArgsOpen(id: number) {
  return expandedIds.value.includes(id);
}

/** 行卡上的参数展开：桌面靠 expand 列，手机上卡片没有那一列，自己维护一份 id */
function toggleArgs(id: number) {
  const i = expandedIds.value.indexOf(id);
  if (i === -1) expandedIds.value.push(id);
  else expandedIds.value.splice(i, 1);
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

      <!-- partial：日志拿到了但统计没拿到，别用一整块状态图顶掉已经拿到的日志 -->
      <div v-if="hasPartialBanner(auditLogs.length)" class="pt-note pt-note--warn partial-note">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>
          统计读数这次没拿到，上面三个指标显示为
          <code>-</code>
          ；下面的审计日志是完整的，点右上角刷新可以再试一次。
        </span>
      </div>

      <el-table v-if="!isMobile" :data="auditLogs" class="pt-grid" row-key="id" style="width: 100%">
        <template #empty>
          <PtDataState :state="state" dense :sub="stateSub">
            <template v-if="state === 'error'" #action>
              <el-button size="small" @click="fetchAuditLogs">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>
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

      <!--
        移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
        这张表桌面有 6 列加一个展开列，手机上横着滚既看不到列头，也和页面纵向滚动打架。
        卡上留的是审计要看的四件事：谁（触发用户）在什么时候、从哪个通道、执行了什么命令，
        结果与延迟进右上角和第二行；完整参数保留成一个展开按钮，不然手机上就查不到证据了。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!auditLogs.length" :state="state" :sub="stateSub">
          <template v-if="state === 'error'" #action>
            <el-button size="small" @click="fetchAuditLogs">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>

        <PtRowCard v-for="row in auditLogs" :key="row.id">
          <template #title>
            <code class="cmd">{{ row.command }}</code>
          </template>

          <template #meta>
            <span>
              <PtIcon name="clock" :size="11" />
              {{ formatDate(row.created_at) }}
            </span>
            <PtTag>{{ channelLabel(row.channel_type) }}</PtTag>
            <span>
              <PtIcon name="user" :size="11" />
              <code class="uid">{{ row.channel_user_id || "-" }}</code>
            </span>
            <span :class="{ 'lat--slow': row.latency_ms > 1000 }">
              <PtIcon name="timer" :size="11" />
              {{ row.latency_ms }} ms
            </span>
          </template>

          <template #status>
            <PtStatusPill :tone="resultTone(row.result)" size="sm">
              {{ row.result }}
            </PtStatusPill>
          </template>

          <template #actions>
            <el-button size="small" text @click="toggleArgs(row.id)">
              <PtIcon :name="isArgsOpen(row.id) ? 'chevron-up' : 'chevron-down'" :size="14" />
              <span>{{ isArgsOpen(row.id) ? "收起参数" : "命令参数" }}</span>
            </el-button>
            <div v-if="isArgsOpen(row.id)" class="card-args">
              <span class="args__redacted">
                <PtIcon name="lock" :size="12" />
                <span>敏感字段已脱敏</span>
              </span>
              <pre class="args__json">{{ formatJson(row.args_json) }}</pre>
            </div>
          </template>
        </PtRowCard>
      </div>

      <template v-if="pagination.total > 0" #footer>
        <span class="pt-foot-note">
          {{
            isMobile
              ? "点卡片上的「命令参数」看脱敏后的完整参数"
              : "展开一行可以看脱敏后的完整命令参数"
          }}
        </span>
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

/* 面板 padding="none"，partial 提示条的留白只能自己给 */
.partial-note {
  margin: var(--pt-space-3);
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

/* 移动端行卡列表：面板 padding="none"，所以留白由这里给 */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-space-3);
}

/*
 * 展开的参数块占满 actions 那一行的整宽：actions 是 flex-wrap，
 * 不给 100% 基宽它会挤在按钮右边被压成一条。
 */
.card-args {
  display: flex;
  flex: 1 0 100%;
  flex-direction: column;
  gap: var(--pt-space-2);
  min-width: 0;
}

@media (max-width: 768px) {
  .f-date,
  .f-sel,
  .f-search {
    width: 100%;
  }
}
</style>
