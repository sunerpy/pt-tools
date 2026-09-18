<script setup lang="ts">
import { type AuditLog, chatopsApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtBreakdown, { type BreakdownRow } from "@/components/ui/PtBreakdown.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
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

/** 是否已经成功读过一次列表：没读过就不往页头摘要里写数字（宁缺勿造） */
const loadedOnce = ref(false);

const stats = reactive({
  todayCount: 0,
  successRate: 0,
  maxLatencyMs: 0,
});

/** 统计接口是不是没拿到：0 = 正常，1 = 失败（喂给 useDataState 的 failed） */
const statsFailed = ref(0);

/**
 * 画板 25 在 gfoot 之后有三张分析卡：p-cmd 548（命令分布）、p-ch 516（渠道分布）、
 * p-fail 1080（失败清单）。三张都由当前这页的行现算，没有额外请求；
 * 口径写在脚注里 —— 接口不回全库的分组计数，当页的分布不能当成全库的分布。
 */
const cmdRows = computed<BreakdownRow[]>(() => {
  const buckets = new Map<string, number>();
  for (const log of auditLogs.value) {
    const name = log.command || "(空命令)";
    buckets.set(name, (buckets.get(name) ?? 0) + 1);
  }
  return [...buckets.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, n]) => ({ key: name, label: name, value: n, tone: "primary" as const }));
});

const channelRows = computed<BreakdownRow[]>(() => {
  const buckets = new Map<string, number>();
  for (const log of auditLogs.value) {
    const name = log.channel_type || "未知渠道";
    buckets.set(name, (buckets.get(name) ?? 0) + 1);
  }
  return [...buckets.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, n]) => ({ key: name, label: name, value: n, tone: "info" as const }));
});

/** 失败与被拒的调用 —— 审计页真正要看的那一小撮 */
const failRows = computed<BreakdownRow[]>(() =>
  auditLogs.value
    .filter((log) => log.result !== "success")
    .slice(0, 8)
    .map((log) => ({
      key: String(log.id),
      label: log.command || "(空命令)",
      value: log.result === "denied" ? "被拒" : "出错",
      weight: 1,
      tone: log.result === "denied" ? ("warn" as const) : ("dang" as const),
      hint: `${log.channel_type} · ${log.channel_user_id} · ${log.latency_ms} ms`,
    })),
);

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
 * failed 接的是统计接口：它只喂页头摘要里的三个读数，挂了不该把已经拿到的日志
 * 一起丢掉，所以记一个失败数让状态落到 partial，表格照常渲染。
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

/** 「09-18 12:03」这种短时间：摘要行要在一行里放下两个时间点，整段日期太长 */
function shortTime(input: string | number | Date) {
  const d = input instanceof Date ? input : new Date(input);
  if (Number.isNaN(d.getTime())) return "";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

/**
 * 摘要行里的时间范围。
 *
 * 筛了时间就报筛选区间（那是用户此刻在看的范围）；没筛就报本页最早到最新那条 ——
 * 写成「全部日志的时间跨度」是编的：接口只回当前这一页，最早一条在哪不知道。
 */
const timeRangeText = computed(() => {
  if (filters.dateRange && filters.dateRange.length === 2) {
    const from = shortTime(filters.dateRange[0]);
    const to = shortTime(filters.dateRange[1]);
    if (from && to) return `${from} ~ ${to}`;
  }

  const times = auditLogs.value
    .map((row) => new Date(row.created_at).getTime())
    .filter((t) => !Number.isNaN(t));
  if (times.length === 0) return "";
  return `本页 ${shortTime(new Date(Math.min(...times)))} ~ ${shortTime(new Date(Math.max(...times)))}`;
});

/**
 * 画板 head 的 sub —— 标题下面那行实时摘要（11.5/400 t3），由本页 Teleport 进外壳页头。
 *
 * 这一页原来在主区顶上摆了一条三格 KPI 带，但画板 25 的构成是
 * head 64 → toolbar 40 → grid → gfoot 34，顶上那条带的位置就是 head 本身。
 * 所以三个读数搬进摘要行：条数、时间范围在前（本页的主角是日志条目），
 * 今日条数 / 成功率 / 最高延迟在后。统计没拿到就直说「未取到」，不摆 0 充数。
 */
const headSub = computed(() => {
  if (!loadedOnce.value) return "";

  const parts = [hasFilter.value ? `筛选后 ${pagination.total} 条` : `${pagination.total} 条记录`];
  if (timeRangeText.value) parts.push(timeRangeText.value);

  if (statsFailed.value === 0) {
    parts.push(`今日 ${stats.todayCount} 条`);
    parts.push(`成功率 ${stats.successRate.toFixed(1)}%`);
    parts.push(`最高延迟 ${stats.maxLatencyMs} ms`);
  } else {
    parts.push("统计读数未取到");
  }

  return parts.join(" · ");
});

/** gfoot 左侧的口径：本页在总数里的位置（画板 tasks 是「37 个任务 · 显示 1–10 · 每页 20」） */
const rangeFrom = computed(() => (pagination.page - 1) * pagination.pageSize + 1);
const rangeTo = computed(() => rangeFrom.value + auditLogs.value.length - 1);

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
    loadedOnce.value = false;
    ElMessage.error(errorText.value || "获取审计日志失败");
    return;
  }

  auditLogs.value = data.items || [];
  pagination.total = data.total || 0;
  expandedIds.value = [];
  loadedOnce.value = true;
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

/** CSV 单元格：一律加引号并把内部引号翻倍，参数 JSON 里的逗号与换行才不会撕开列 */
function csvCell(value: unknown) {
  const s = value === null || value === undefined ? "" : String(value);
  return `"${s.replace(/"/g, '""')}"`;
}

/**
 * 导出 —— 后端没有导出接口，所以只能导**当前这一页**已经拿到的行，
 * 按钮提示里也是这么写的。悄悄导成「全部日志」会让人拿着 30 条当完整证据。
 */
function exportCsv() {
  if (auditLogs.value.length === 0) {
    ElMessage.info("当前页没有可导出的日志");
    return;
  }

  // 两处都显式写成 string[]：混进 number 会让 [head, ...rows] 变成联合数组类型，.map 就不可调用了
  const head: string[] = ["时间", "通道", "触发用户", "命令", "结果", "延迟(ms)", "命令参数"];
  const rows: string[][] = auditLogs.value.map((row) => [
    formatDate(row.created_at),
    channelLabel(row.channel_type),
    row.channel_user_id || "",
    row.command,
    row.result,
    String(row.latency_ms),
    formatJson(row.args_json),
  ]);
  const body = [head, ...rows].map((r) => r.map(csvCell).join(",")).join("\r\n");
  // 开头的 BOM 不能省：Excel 不认没有 BOM 的 UTF-8，中文表头会变成乱码
  const csv = `\uFEFF${body}`;

  const url = URL.createObjectURL(new Blob([csv], { type: "text/csv;charset=utf-8" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = `chatops-audit-p${pagination.page}-${Date.now()}.csv`;
  a.click();
  URL.revokeObjectURL(url);
  ElMessage.success(`已导出本页 ${auditLogs.value.length} 条`);
}
</script>

<template>
  <div class="audit-page">
    <!-- 画板 head 的 sub：标题下面那行实时摘要，由本页把真实数字送进外壳页头 -->
    <Teleport v-if="headSub" to="#pt-head-sub">{{ headSub }}</Teleport>

    <!--
      工具栏带 —— 画板 bar-64：40 高全宽白带。只读页没有主操作，所以这条带上
      只有筛选控件 + 右侧 28×28 图标钮，#pt-head-acts 保持空着。
    -->
    <PtToolbar band>
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
        <el-tooltip content="刷新" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            aria-label="刷新"
            :disabled="loading"
            @click="fetchAuditLogs">
            <PtIcon name="refresh-cw" :size="15" :class="{ 'pt-spin': loading }" />
          </button>
        </el-tooltip>
        <el-tooltip content="导出本页为 CSV" placement="top">
          <button
            type="button"
            class="pt-band__iconbtn"
            aria-label="导出本页为 CSV"
            :disabled="!auditLogs.length"
            @click="exportCsv">
            <PtIcon name="file-down" :size="15" />
          </button>
        </el-tooltip>
      </template>
    </PtToolbar>

    <!--
      partial：日志拿到了但统计没拿到，别用一整块状态图顶掉已经拿到的日志。
      提示不是全宽带 —— 画板 27 的落法是在顶部带之后、左右各内缩 16。
    -->
    <div v-if="hasPartialBanner(auditLogs.length)" class="pt-note pt-note--warn partial-note">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>
        统计读数这次没拿到，标题下的摘要里少了「今日条数 / 成功率 / 最高延迟」；
        下面的审计日志是完整的，点工具栏右侧的刷新可以再试一次。
      </span>
    </div>

    <!-- 表格带 —— 画板 grid：全宽平铺，没有圆角也没有外边距，不再包在卡片里 -->
    <div v-loading="loading" class="pt-band--grid">
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
    </div>

    <!-- 画板 gfoot 34：左边本页口径 + 提示，右边分页 -->
    <div v-if="pagination.total > 0" class="pt-band--foot foot">
      <span>
        {{ pagination.total }} 条 · 显示 {{ rangeFrom }}–{{ rangeTo }} · 每页
        {{ pagination.pageSize }}
      </span>
      <span class="foot__hint">
        {{
          isMobile
            ? "点卡片上的「命令参数」看脱敏后的完整参数"
            : "展开一行可以看脱敏后的完整命令参数"
        }}
      </span>
      <span class="pt-band__spacer" />
      <el-pagination
        v-model:current-page="pagination.page"
        class="pt-pager"
        :page-size="pagination.pageSize"
        :total="pagination.total"
        :pager-count="5"
        layout="prev, pager, next"
        @current-change="handlePageChange" />
    </div>

    <!-- 画板 25 的分析卡：p-cmd 548 / p-ch 516 两栏 + p-fail 1080 通栏 -->
    <div v-if="auditLogs.length > 0" class="pt-cards pt-cards--2">
      <PtPanel title="命令分布" icon="terminal" :count="`${cmdRows.length} 种`">
        <PtBreakdown
          :rows="cmdRows"
          :total="auditLogs.length"
          foot="统计的是当前这一页的调用；接口不回全库的分组计数。" />
      </PtPanel>

      <PtPanel title="渠道分布" icon="message-square" :count="`${channelRows.length} 个`">
        <PtBreakdown :rows="channelRows" :total="auditLogs.length" />
      </PtPanel>

      <PtPanel
        class="pt-cards__full"
        title="失败与被拒"
        icon="shield-x"
        :count="failRows.length > 0 ? `${failRows.length} 条` : '暂无'">
        <PtBreakdown
          v-if="failRows.length > 0"
          :rows="failRows"
          cols
          foot="只列当前这一页里 result 不是 success 的调用，最多 8 条。" />
        <p v-else class="audit-ok">当前这一页的调用都成功了。</p>
      </PtPanel>

      <!--
        画板 p-keep 1080：保留与清理。内容按代码核过 —— 审计表是 models.ActionAudit
        （表名 action_audit），`internal/maintenance` 的 Cleaner 只管 logs / staging /
        backups 三类，**不碰审计表**，所以这里说的是「不会自动删」而不是某个保留天数。
      -->
      <PtPanel class="pt-cards__full" title="保留与清理" icon="archive">
        <ul class="audit-keep">
          <li>
            审计记录写在本机库的 <code>action_audit</code> 表里，<strong>没有自动清理</strong>：
            <code>pt-tools clean</code> 只清日志轮转备份、暂存目录与旧备份，不动这张表。
          </li>
          <li>
            每条记录只留命令、参数、渠道、发起人、结果与耗时。参数在写入前脱敏，
            页面上展开看到的就是脱敏后的那份。
          </li>
          <li>
            记录会一直攒着。要缩库就直接删表里的旧行（先备份
            <code>~/.pt-tools</code>），删掉不影响任何运行中的功能。
          </li>
        </ul>
      </PtPanel>
    </div>
  </div>
</template>

<style scoped>
/* 保留与清理说明卡 */
.audit-keep {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding-left: 18px;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.audit-keep code {
  padding: 1px 5px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  background: var(--pt-hover);
  border-radius: 3px;
}

/* 分析卡里「这一页都成功了」的正面结论 */
.audit-ok {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

/*
 * 画板主区是一串全宽横向带（toolbar → grid → gfoot），带与带之间没有间距，
 * 所以这里不给 gap；需要内缩的东西（提示、移动端行卡）自己带 16 的留白。
 */
.audit-page {
  display: flex;
  flex-direction: column;
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

/* 工具栏右侧的图标钮：不可用时只掉色，不从带上消失（位置稳定比隐藏更好认） */
.pt-band__iconbtn:disabled {
  color: var(--pt-t4);
  cursor: not-allowed;
}

.pt-band__iconbtn:disabled:hover {
  background: var(--pt-hover);
}

.pt-band__iconbtn:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 1px;
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

/* 提示不是全宽带：画板里它在卡片层，左右各内缩 16 */
.partial-note {
  align-items: flex-start;
  margin: var(--pt-pad);
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

/* 移动端行卡列表：表格带本身不留白，行卡的 16 内缩由这里给 */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-pad);
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

  /*
   * 34 高的页脚带在手机上装不下「口径 + 提示 + 分页」，允许换行；
   * 提示那句在窄屏是废话（卡片上就写着按钮名），直接收掉。
   */
  .foot {
    flex-wrap: wrap;
    padding: var(--pt-space-2) var(--pt-space-3);
  }

  .foot__hint {
    display: none;
  }
}
</style>
