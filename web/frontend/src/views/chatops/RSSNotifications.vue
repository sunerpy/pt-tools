<script setup lang="ts">
import { chatopsApi, type NotificationConfig, type RSSNotificationLog } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtBreakdown, { type BreakdownRow } from "@/components/ui/PtBreakdown.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import PtTag from "@/components/ui/PtTag.vue";
import PtToolbar from "@/components/ui/PtToolbar.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
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

const isMobile = useIsMobile();
const logs = ref<RSSNotificationLog[]>([]);
const confs = ref<NotificationConfig[]>([]);
const autoRefresh = ref(false);
let timer: ReturnType<typeof setInterval> | null = null;

/* 移动端行卡没有展开行，详情走弹窗。按 id 记而不是存整行：
   自动刷新每 10 秒换一批对象，存引用会让弹窗停在旧数据上 */
const detailVisible = ref(false);
const detailId = ref<number | null>(null);
const detailRow = computed(() => logs.value.find((l) => l.id === detailId.value) ?? null);

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

/*
 * 六态状态机（设计文档 §5）。
 *
 * 以前这里只有一个 loading ref，失败时弹个 toast 就完事 —— 两秒后 toast 消失，
 * 表格停在 empty 上，用户看到的是「还没有通知记录」，而真相是请求挂了。
 * 401/403 也要单独画成「无权访问」，否则用户会一直点重试。
 */
const { loading, state, errorText, run } = useDataState({ filtered: () => hasFilter.value });

/** 请求没拿到数据（失败或无权）：这时候所有读数都不能当真 */
const loadFailed = computed(() => state.value === "error" || state.value === "perm");

/** 状态块副标题：失败时给真实错误，空态时给下一步动作 */
const stateSub = computed(() => {
  if (loadFailed.value) return errorText.value;
  return hasFilter.value ? "换个筛选条件再看" : "RSS 上新推送的每一次投递都会记录在这里";
});

/* 本页统计只反映当前 30 条，标签里写清楚「本页」，免得被当成全局口径。
   加载失败时读数写「—」：这时候摆一排 0 会被读成「库里真的没有记录」 */
/**
 * 画板 head 的 sub（11.5/400 t3）。
 *
 * 画板 26 是带式表格页：head 64 → bar-64 → grid → gfoot，**没有 KPI 带**
 * （KPI 带只在画板 02 工作台与 10 用户统计的主区顶上）。原来这页顶着一条 3 格
 * KPI，数字挪到摘要行，信息一条没少，版面回到画板的样子。
 */
const headSub = computed(() => {
  if (loadFailed.value) return "通知日志没加载出来";
  const parts = [`${pagination.total} 条记录`];
  if (logs.value.length > 0) {
    parts.push(`本页已发送 ${sentCount.value}`);
    parts.push(`失败 / 待重试 ${failedCount.value}`);
  }
  return parts.join(" · ");
});

/**
 * 画板 26 在 gfoot 之后有分析卡：p-res 548（结果分布）、p-idem 516（幂等口径）、
 * p-retry 1080（待重试队列）。结果分布与待重试由当前这页的行现算；
 * 幂等那张是固定说明 —— 它讲的是这套日志为什么不会重复推送，没有可查的数据。
 */
const resultRows = computed<BreakdownRow[]>(() => {
  const buckets = new Map<string, number>();
  for (const row of logs.value) {
    buckets.set(row.result, (buckets.get(row.result) ?? 0) + 1);
  }
  const toneOf: Record<string, BreakdownRow["tone"]> = {
    sent: "ok",
    failed: "dang",
    pending: "warn",
    suppressed: "mute",
    throttled: "warn",
  };
  return [...buckets.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([key, n]) => ({
      key,
      label: resultMeta(key).label,
      value: n,
      tone: toneOf[key] ?? "primary",
    }));
});

/** p-site：这一页的通知按站点分布 */
const siteRows = computed<BreakdownRow[]>(() => {
  const buckets = new Map<string, number>();
  for (const row of logs.value) {
    const name = row.site_name || "未知站点";
    buckets.set(name, (buckets.get(name) ?? 0) + 1);
  }
  return [...buckets.entries()]
    .sort((a, b) => b[1] - a[1])
    .map(([name, n]) => ({ key: name, label: name, value: n, tone: "primary" as const }));
});

/**
 * p-quiet：每个通道的安静时段。
 * 数据来自通道配置本身（NotificationConf 的 quiet_hours_start / quiet_hours_end），
 * 这一页已经为了把 conf_id 显示成通道名而拉过 confs，不额外请求。
 * start > end 表示跨午夜（models/chatops_models.go 上的注释就是这么定义的）。
 */
interface QuietRow {
  id: number;
  name: string;
  window: string;
  crossesMidnight: boolean;
}

const quietRows = computed<QuietRow[]>(() =>
  confs.value.map((c) => {
    const start = c.quiet_hours_start ?? "";
    const end = c.quiet_hours_end ?? "";
    const on = Boolean(start && end);
    return {
      id: c.id,
      name: c.name,
      window: on ? `${start} – ${end}` : "未设置",
      crossesMidnight: on && start > end,
    };
  }),
);

/** 还会再发一次的那些：failed / pending，按尝试次数排 */
const retryRows = computed<BreakdownRow[]>(() =>
  logs.value
    .filter((row) => row.result === "failed" || row.result === "pending")
    .sort((a, b) => b.attempts - a.attempts)
    .slice(0, 8)
    .map((row) => ({
      key: String(row.id),
      label: `${row.site_name || "未知站点"} · ${row.torrent_id}`,
      value: `${row.attempts} 次`,
      weight: row.attempts,
      tone: row.result === "failed" ? ("dang" as const) : ("warn" as const),
      hint:
        row.last_error ||
        (row.next_retry_at ? `下次重试 ${formatDate(row.next_retry_at)}` : "排队中"),
    })),
);

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

/** 展开行 / 详情弹窗共用的键值行，避免两处各写一遍 */
function detailItems(row: RSSNotificationLog) {
  return [
    { k: "失败原因", v: row.last_error || "（无）", err: Boolean(row.last_error) },
    { k: "下次重试", v: formatDate(row.next_retry_at), err: false },
    { k: "投递完成", v: formatDate(row.delivered_at), err: false },
  ];
}

function openDetail(row: RSSNotificationLog) {
  detailId.value = row.id;
  detailVisible.value = true;
}

async function fetchLogs() {
  const params = new URLSearchParams();
  params.append("page", String(pagination.page));
  params.append("page_size", String(pagination.pageSize));
  if (filters.rss_id) params.append("rss_id", String(filters.rss_id));
  if (filters.kind) params.append("kind", String(filters.kind));
  if (filters.result) params.append("result", String(filters.result));
  if (filters.conf_id) params.append("conf_id", String(filters.conf_id));

  const res = await run(() => chatopsApi.rssNotifications.list(params));
  if (!res) {
    /* 失败时清空：留着上一次的数据配一个「加载失败」的状态块更让人误解。
       这里故意不弹 toast —— 错误已经常驻在表格/卡片里，而自动刷新每 10 秒
       失败一次会把 toast 刷成一片。 */
    logs.value = [];
    pagination.total = 0;
    return;
  }
  logs.value = res.items || [];
  pagination.total = res.total || 0;
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
  <!--
    画板 26 的主区构成：head 64 → bar-64（40）→ grid（表格，全宽平铺）→ gfoot（34）。
    三条带是彼此的兄弟，都不套在卡片里；表格标题与条数走页头，所以这页没有 PtPanel。
  -->
  <div class="rss-notify-page">
    <PtHeadSub>{{ headSub }}</PtHeadSub>

    <PtToolbar band>
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

    <div v-loading="loading" class="pt-band--grid">
      <el-table v-if="!isMobile" :data="logs" class="pt-grid" row-key="id" style="width: 100%">
        <template #empty>
          <PtDataState :state="state" dense :sub="stateSub">
            <template v-if="state === 'error'" #action>
              <el-button size="small" @click="fetchLogs">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
            </template>
          </PtDataState>
        </template>

        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="detail">
              <div v-for="it in detailItems(row)" :key="it.k" class="detail__kv">
                <span class="detail__k">{{ it.k }}</span>
                <span :class="{ 'detail__v--err': it.err }">{{ it.v }}</span>
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

      <!--
        移动端行卡（§9：桌面表格一律降级成行卡，不做横向滚动表格）。
        这张表桌面有 8 列 + 展开行，手机上横着滚既看不到列头，也和页面纵向滚动打架。
        卡上留真正要看的：站点+种子 ID 当标题，时间/类型/通道/尝试次数当第二行，
        结果状态挂右上角，失败原因直接摊在卡上（不用点开就能判断要不要重试）。
      -->
      <div v-else class="cards">
        <PtDataState v-if="!logs.length" :state="state" :sub="stateSub">
          <template v-if="state === 'error'" #action>
            <el-button size="small" @click="fetchLogs">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>

        <PtRowCard v-for="row in logs" :key="row.id">
          <template #title>
            {{ row.site_name || "未知站点" }} ·
            <code class="tid-title">{{ row.torrent_id }}</code>
          </template>

          <template #meta>
            <span>
              <PtIcon name="clock" :size="11" />
              {{ formatDate(row.created_at) }}
            </span>
            <PtTag>{{ row.notify_kind === "filtered" ? "仅匹配" : "全部新种" }}</PtTag>
            <span>
              <PtIcon name="send" :size="11" />
              {{ confLabel(row.notification_conf_id) }}
            </span>
            <span>尝试 {{ row.attempts }} 次</span>
            <span v-if="row.last_error" class="meta-err">{{ row.last_error }}</span>
          </template>

          <template #status>
            <PtStatusPill :tone="resultMeta(row.result).tone" size="sm">
              {{ resultMeta(row.result).label }}
            </PtStatusPill>
          </template>

          <template #actions>
            <el-button size="small" @click="openDetail(row)">
              <PtIcon name="file-text" :size="14" /><span>详情</span>
            </el-button>
            <template v-if="canAct(row.result)">
              <el-button size="small" type="primary" plain @click="handleRetry(row)">
                <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
              </el-button>
              <el-button size="small" type="danger" plain @click="handleCancel(row)">
                <PtIcon name="circle-x" :size="14" /><span>取消</span>
              </el-button>
            </template>
          </template>
        </PtRowCard>
      </div>
    </div>

    <div v-if="pagination.total > 0" class="pt-band--foot">
      <span>
        {{
          isMobile
            ? "点卡片上的详情可以看下次重试时间和推送出去的消息内容"
            : "展开一行可以看失败原因和推送出去的消息内容"
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

    <!-- 画板 26 的分析卡：p-res 548 / p-idem 516 两栏 + p-retry 1080 通栏 -->
    <div v-if="logs.length > 0" class="pt-cards pt-cards--2">
      <PtPanel title="推送结果分布" icon="chart-pie" :count="`${logs.length} 条（本页）`">
        <PtBreakdown
          :rows="resultRows"
          :total="logs.length"
          foot="统计的是当前这一页的记录；接口不回全库的分组计数。" />
      </PtPanel>

      <PtPanel title="幂等与去重口径" icon="copy-check">
        <ul class="idem">
          <li>每条「RSS + 种子 + 通道」只会留一条记录，调度器重跑同一轮不会重复推送。</li>
          <li>被安静时段或每小时配额挡下的记为 <code>suppressed</code>，不算失败。</li>
          <li>合并推送（digest）会把同一轮的多条并成一条消息，日志里仍然一条种子一条记录。</li>
          <li>手动点「重试」只会把这条重新入队，不会新建一条记录，尝试次数 +1。</li>
        </ul>
      </PtPanel>

      <PtPanel
        class="pt-cards__full"
        title="待重试与失败"
        icon="refresh-cw"
        :count="retryRows.length > 0 ? `${retryRows.length} 条` : '暂无'">
        <PtBreakdown
          v-if="retryRows.length > 0"
          :rows="retryRows"
          cols
          foot="只列当前这一页里 failed 与 pending 的记录，按尝试次数排，最多 8 条。" />
        <p v-else class="idem-ok">当前这一页没有失败或待重试的记录。</p>
      </PtPanel>
    </div>

    <!-- 画板 p-site 548（按站点分布）/ p-quiet 516（各通道的安静时段）两栏 -->
    <div v-if="logs.length > 0" class="pt-cards pt-cards--2">
      <PtPanel title="按站点分布" icon="globe" :count="`${siteRows.length} 个站点`">
        <PtBreakdown
          :rows="siteRows"
          :total="logs.length"
          foot="统计的是当前这一页的记录；接口不回全库的分组计数。" />
      </PtPanel>

      <PtPanel title="安静时段" icon="moon" :count="`${quietRows.length} 个通道`">
        <ul v-if="quietRows.length > 0" class="quiet">
          <li v-for="row in quietRows" :key="row.id" class="quiet__row">
            <span class="quiet__k">{{ row.name }}</span>
            <span class="quiet__v" :class="{ 'is-off': row.window === '未设置' }">
              {{ row.window }}
            </span>
            <span v-if="row.crossesMidnight" class="quiet__note">跨午夜</span>
          </li>
        </ul>
        <p v-else class="idem-ok">还没有配置通知通道。</p>
        <p class="quiet__foot">
          落在安静时段里的通知记为 <code>suppressed</code>，不算失败、也不会攒着补发。
          时段在「消息通知」里按通道配置。
        </p>
      </PtPanel>
    </div>

    <!-- 行卡替代了展开行，详情放弹窗；桌面走表格展开，不会用到这里 -->
    <el-dialog v-model="detailVisible" title="通知详情" width="92%" align-center>
      <div v-if="detailRow" class="detail">
        <div v-for="it in detailItems(detailRow)" :key="it.k" class="detail__kv">
          <span class="detail__k">{{ it.k }}</span>
          <span :class="{ 'detail__v--err': it.err }">{{ it.v }}</span>
        </div>
        <div class="detail__kv">
          <span class="detail__k">消息内容</span>
          <pre class="detail__json">{{ formatJson(detailRow.payload_json) }}</pre>
        </div>
      </div>
      <p v-else class="detail-gone">这条记录已经不在当前列表里了，刷新后重新打开。</p>
    </el-dialog>
  </div>
</template>

<style scoped>
/* 安静时段卡：一行一个通道 */
.quiet {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.quiet__row {
  display: flex;
  gap: var(--pt-space-2);
  align-items: baseline;
  font-size: var(--pt-fz-sm);
}

.quiet__k {
  flex: 1;
  overflow: hidden;
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quiet__v {
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: var(--pt-t1);
}

.quiet__v.is-off {
  font-weight: 400;
  color: var(--pt-t4);
}

.quiet__note {
  padding: 0 5px;
  font-size: var(--pt-fz-foot);
  color: var(--pt-warn);
  background: color-mix(in srgb, var(--pt-warn) 14%, transparent);
  border-radius: var(--pt-r-sm);
}

.quiet__foot {
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t4);
}

.quiet__foot code {
  padding: 1px 4px;
  font-family: var(--pt-font-mono);
  background: var(--pt-hover);
  border-radius: 3px;
}

/* 幂等口径卡：固定说明，条目之间留 8 */
.idem {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  margin: 0;
  padding-left: 18px;
  font-size: var(--pt-fz-sm);
  line-height: var(--pt-lh-body);
  color: var(--pt-t2);
}

.idem code {
  padding: 1px 5px;
  font-family: var(--pt-font-mono);
  font-size: var(--pt-fz-label);
  background: var(--pt-hover);
  border-radius: 3px;
}

.idem-ok {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

/* 带之间没有间隔（画板上它们是连着的），所以这里不再是带 gap 的 flex 列 */
.rss-notify-page {
  display: flex;
  flex-direction: column;
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

.detail-gone {
  margin: 0;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}

/* 移动端行卡列表：面板 padding="none"，留白由这里给 */
.cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-space-3);
}

/* 种子 ID 在标题里用等宽，字号和粗细跟着标题走 */
.tid-title {
  font-family: var(--pt-font-mono);
  font-size: inherit;
  color: inherit;
}

/* 失败原因摊在 meta 最后一行：可以很长，所以允许断词并只留两行 */
.meta-err {
  display: -webkit-box;
  overflow: hidden;
  color: var(--pt-dang);
  word-break: break-word;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

@media (max-width: 768px) {
  .f-id,
  .f-sel {
    width: 100%;
  }
}
</style>
