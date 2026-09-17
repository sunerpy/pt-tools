<script setup lang="ts">
import { chatopsApi, type NotificationConfig, type RSSNotificationLog } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtKpiBar from "@/components/ui/PtKpiBar.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";

/*
 * 结果状态用中文 + 语义色胶囊，不再直接甩 sent/failed 这些原始值：
 * 筛选下拉里保留原值，因为它就是接口参数，改成中文会让人对不上 API 文档。
 */
const RESULT_META: Record<string, { label: string; tone: "ok" | "warn" | "dang" | "neutral" }> = {
  sent: { label: "已发送", tone: "ok" },
  failed: { label: "失败", tone: "dang" },
  throttled: { label: "被限流", tone: "warn" },
  suppressed: { label: "已取消", tone: "neutral" },
  pending: { label: "待发送", tone: "warn" },
};

const RESULT_OPTIONS = ["sent", "failed", "suppressed", "pending", "throttled"];

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

const hasFilter = computed(() =>
  Boolean(filters.rss_id || filters.kind || filters.result || filters.conf_id),
);

/* 本页统计只反映当前 30 条，标签里写清楚「本页」，免得被当成全局口径 */
const kpiItems = computed(() => [
  { label: "通知记录总数", value: pagination.total, unit: " 条", icon: "bell-ring" },
  { label: "已发送（本页）", value: sentCount.value, icon: "circle-check" },
  {
    label: "失败 / 待重试（本页）",
    value: failedCount.value,
    icon: "triangle-alert",
    delta: failedCount.value > 0 ? "需处理" : undefined,
    deltaTone: "dang" as const,
  },
]);

function confLabel(id: number): string {
  const c = confs.value.find((x) => x.id === id);
  return c ? c.name : `#${id}`;
}

function resultMeta(r: string) {
  return RESULT_META[r] || { label: r, tone: "neutral" as const };
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

function canAct(result: string) {
  return result === "failed" || result === "pending";
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

<template>
  <div class="rss-notify-page">
    <PtKpiBar :items="kpiItems" />

    <PtPanel
      v-loading="loading"
      title="RSS 通知日志"
      icon="rss"
      :count="`${pagination.total} 条`"
      padding="none">
      <PtToolbar>
        <el-input
          v-model="filters.rss_id"
          placeholder="RSS ID"
          clearable
          class="f-id"
          @keyup.enter="handleFilterChange"
          @clear="handleFilterChange">
          <template #prefix>
            <PtIcon name="hash" :size="14" />
          </template>
        </el-input>

        <el-select
          v-model="filters.kind"
          placeholder="全部类型"
          clearable
          class="f-sel"
          @change="handleFilterChange">
          <el-option label="全部新种（简略）" value="all" />
          <el-option label="仅匹配的（详细）" value="filtered" />
        </el-select>

        <el-select
          v-model="filters.result"
          placeholder="全部结果"
          clearable
          class="f-sel"
          @change="handleFilterChange">
          <el-option
            v-for="r in RESULT_OPTIONS"
            :key="r"
            :label="`${resultMeta(r).label}（${r}）`"
            :value="r" />
        </el-select>

        <el-select
          v-model="filters.conf_id"
          placeholder="全部通道"
          clearable
          class="f-sel"
          @change="handleFilterChange">
          <el-option
            v-for="c in confs"
            :key="c.id"
            :label="`${c.name}（${c.channel_type}）`"
            :value="c.id" />
        </el-select>

        <template #right>
          <el-tooltip content="每 10 秒重新拉一次当前列表" placement="bottom">
            <label class="ctl">
              <el-switch v-model="autoRefresh" size="small" />
              <span>自动刷新</span>
            </label>
          </el-tooltip>
          <el-button size="small" :loading="loading" @click="fetchLogs">
            <PtIcon name="refresh-cw" :size="14" /><span>刷新</span>
          </el-button>
        </template>
      </PtToolbar>

      <el-table :data="logs" class="pt-grid" row-key="id" style="width: 100%">
        <template #empty>
          <PtDataState
            :state="hasFilter ? 'zero' : 'empty'"
            dense
            :sub="hasFilter ? '换个筛选条件再看' : 'RSS 上新推送的每一次投递都会记录在这里'" />
        </template>

        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="detail">
              <div class="detail__kv">
                <span class="detail__k">失败原因</span>
                <span :class="{ 'detail__v--err': row.last_error }">
                  {{ row.last_error || "（无）" }}
                </span>
              </div>
              <div class="detail__kv">
                <span class="detail__k">下次重试</span>
                <span>{{ formatDate(row.next_retry_at) }}</span>
              </div>
              <div class="detail__kv">
                <span class="detail__k">投递完成</span>
                <span>{{ formatDate(row.delivered_at) }}</span>
              </div>
              <div class="detail__kv">
                <span class="detail__k">消息内容</span>
                <pre class="detail__json">{{ formatJson(row.payload_json) }}</pre>
              </div>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="created_at" label="时间" width="170" class-name="pt-cell-muted">
          <template #default="{ row }">{{ formatDate(row.created_at) }}</template>
        </el-table-column>

        <el-table-column prop="site_name" label="站点" width="110">
          <template #default="{ row }">
            <PtTag>{{ row.site_name || "-" }}</PtTag>
          </template>
        </el-table-column>

        <el-table-column
          prop="torrent_id"
          label="种子 ID"
          min-width="120"
          class-name="pt-cell-strong">
          <template #default="{ row }">
            <code class="tid">{{ row.torrent_id }}</code>
          </template>
        </el-table-column>

        <el-table-column prop="notify_kind" label="类型" width="100">
          <template #default="{ row }">
            <PtTag>{{ row.notify_kind === "filtered" ? "仅匹配" : "全部新种" }}</PtTag>
          </template>
        </el-table-column>

        <el-table-column
          prop="notification_conf_id"
          label="通道"
          width="140"
          class-name="pt-cell-muted">
          <template #default="{ row }">{{ confLabel(row.notification_conf_id) }}</template>
        </el-table-column>

        <el-table-column prop="result" label="结果" width="96">
          <template #default="{ row }">
            <PtStatusPill :tone="resultMeta(row.result).tone" size="sm">
              {{ resultMeta(row.result).label }}
            </PtStatusPill>
          </template>
        </el-table-column>

        <el-table-column
          prop="attempts"
          label="尝试"
          width="72"
          class-name="pt-cell-num"
          label-class-name="pt-cell-num" />

        <el-table-column label="操作" width="130" fixed="right" class-name="pt-cell-act">
          <template #default="{ row }">
            <template v-if="canAct(row.result)">
              <el-button link type="primary" size="small" @click="handleRetry(row)">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
              <el-button link type="danger" size="small" @click="handleCancel(row)">
                <PtIcon name="circle-x" :size="14" /><span>取消</span>
              </el-button>
            </template>
            <span v-else class="no-act">—</span>
          </template>
        </el-table-column>
      </el-table>

      <template v-if="pagination.total > 0" #footer>
        <span class="pt-foot-note">展开一行可以看失败原因和推送出去的消息内容</span>
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
.rss-notify-page {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-4);
}

.f-id {
  width: 130px;
}

.f-sel {
  width: 160px;
}

/* 开关 + 文字算一个整体控件，点文字也能切；label 天然带这个行为 */
.ctl {
  display: inline-flex;
  gap: 6px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  cursor: pointer;
  user-select: none;
}

.tid {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  color: var(--pt-t2);
}

.no-act {
  color: var(--pt-t4);
}

.detail {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

/* 键宽固定 72：四行标签宽度差不多，对齐后值列才成一条直线 */
.detail__kv {
  display: grid;
  grid-template-columns: 72px 1fr;
  gap: var(--pt-space-3);
  align-items: start;
}

.detail__k {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.detail__v--err {
  color: var(--pt-dang);
  word-break: break-word;
}

/* 消息体自己滚：一条详细通知的 payload 可以有几十行 */
.detail__json {
  max-height: 220px;
  margin: 0;
  overflow: auto;
  padding: var(--pt-space-3);
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  line-height: var(--pt-lh-body);
  white-space: pre-wrap;
  word-break: break-word;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-sm);
}

@media (max-width: 768px) {
  .f-id,
  .f-sel {
    width: 100%;
  }
}
</style>
