<script setup lang="ts">
/*
 * 辅种统计（2026-10-30 用户：「需要有一个统计面板统计辅种成功、失败、跳过等相关数据，支持按天 周 月 季度 年
 * 或者自定义时间范围等」「也应增加对 iyuu 的调用统计展示」）。
 * 分段与刷流的收益统计同一套（主机时区，周从周一开始，最后一段是当前这一段、只算到今天）；
 * 柱状图、读数与「明细」表沿用刷流任务页的收益图（BrushTrendChart）。
 */
import {
  type ReseedSettings,
  type ReseedStats,
  type ReseedStatsCounts,
  type ReseedStatsQuery,
  reseedApi,
} from "@/api";
import BrushTrendChart, { type TrendPoint } from "@/components/brush/BrushTrendChart.vue";
import PtIcon from "@/components/PtIcon";
import PtBreakdown, { type BreakdownRow } from "@/components/ui/PtBreakdown.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtKpiBar from "@/components/ui/PtKpiBar.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import {
  BRUSH_PERIODS,
  type BrushPeriod,
  brushPeriodLong,
  brushPeriodMeta,
  brushPeriodShort,
} from "@/utils/brushPeriods";
import { formatShortDateTime } from "@/utils/format";
import {
  reseedCustomGranularity,
  reseedIYUUCallsText,
  reseedIYUUEndpoint,
  reseedStatsFailed,
  reseedStatsSkipped,
} from "@/utils/reseed";
import { computed, onMounted, ref, watch } from "vue";

const props = defineProps<{ settings: ReseedSettings | null }>();

type Range = BrushPeriod | "custom";
const RANGES: { label: string; value: Range }[] = [
  ...BRUSH_PERIODS.map((p) => ({ label: p.label, value: p.value as Range })),
  { label: "自定义", value: "custom" },
];
/** 自定义范围最长 5 年（尝试行保留 5 年） */
const MAX_DAYS = 5 * 366;

const range = ref<Range>("day");
/** 自定义的两天（YYYY-MM-DD，含两头） */
const customDays = ref<[string, string] | null>(null);
const stats = ref<ReseedStats | null>(null);
const loading = ref(false);
const failed = ref("");
const showDetail = ref(false);

let seq = 0;

function query(): ReseedStatsQuery | null {
  if (range.value !== "custom") {
    const meta = brushPeriodMeta(range.value);
    return { granularity: meta.value, periods: meta.periods };
  }
  const days = customDays.value;
  if (!days) return null;
  return { granularity: reseedCustomGranularity(days[0], days[1]), from: days[0], to: days[1] };
}

async function load() {
  const q = query();
  if (!q) return;
  const token = ++seq;
  loading.value = true;
  try {
    const res = await reseedApi.stats(q);
    if (token !== seq) return;
    stats.value = res;
    failed.value = "";
  } catch (e) {
    if (token !== seq) return;
    failed.value = (e as Error)?.message || "统计没有读到";
  } finally {
    if (token === seq) loading.value = false;
  }
}

watch([range, customDays], () => void load());
onMounted(() => void load());
defineExpose({ reload: load });

/** 今天以后、5 年以前的日子不能选 */
function disabledDate(d: Date): boolean {
  const today = new Date();
  today.setHours(23, 59, 59, 999);
  return d.getTime() > today.getTime() || d.getTime() < today.getTime() - MAX_DAYS * 86400000;
}

/** 图与数字按读到的那一份的分段写叫法（换范围时新的一份还在路上，叫法不先变） */
const gran = computed<BrushPeriod>(() => (stats.value?.granularity ?? "day") as BrushPeriod);
const meta = computed(() => brushPeriodMeta(gran.value));
/** 范围的叫法：「最近 30 天」或「2026-08-01 至 2026-10-30」 */
const windowText = computed(() => {
  const s = stats.value;
  if (!s) return "";
  return s.custom ? `${s.from} 至 ${s.to}` : meta.value.window;
});

const series = computed(() => stats.value?.series ?? []);
const totals = computed<ReseedStatsCounts | null>(() => stats.value?.totals ?? null);
const last7 = computed(() => series.value.slice(-7));
const hasData = computed(() =>
  series.value.some(
    (p) => p.created || p.failed || p.gone || p.succeeded || reseedStatsSkipped(p) || p.iyuu_calls,
  ),
);

const kpiItems = computed(() => {
  const t = totals.value;
  const hint = (what: string) => `最近 ${last7.value.length || 7} 段${what}`;
  const series7 = (f: (c: ReseedStatsCounts) => number) => last7.value.map(f);
  return [
    {
      label: "成功",
      value: t ? t.succeeded : "—",
      icon: "circle-check",
      series: series7((c) => c.succeeded),
      seriesHint: hint("每段校验通过、开始做种的辅种"),
      seriesBaseline: "zero" as const,
    },
    {
      label: "失败",
      value: t ? reseedStatsFailed(t) : "—",
      icon: "circle-x",
      delta: t?.rolled_back ? `撤回 ${t.rolled_back}` : undefined,
      deltaTone: "warn" as const,
      series: series7(reseedStatsFailed),
      seriesHint: hint("每段失败的辅种（含撤回）"),
      seriesBaseline: "zero" as const,
    },
    {
      label: "站点已删除",
      value: t ? t.gone : "—",
      icon: "archive",
      series: series7((c) => c.gone),
      seriesHint: hint("每段站点上已经删了的种子"),
      seriesBaseline: "zero" as const,
    },
    {
      label: "建成任务",
      value: t ? t.created : "—",
      icon: "list-checks",
      series: series7((c) => c.created),
      seriesHint: hint("每段建成的辅种任务"),
      seriesBaseline: "zero" as const,
    },
    {
      label: "跳过",
      value: t ? reseedStatsSkipped(t) : "—",
      icon: "skip-forward",
      series: series7(reseedStatsSkipped),
      seriesHint: hint("每段跳过的候选"),
      seriesBaseline: "zero" as const,
    },
    {
      label: "IYUU 请求",
      value: t ? t.iyuu_calls : "—",
      icon: "send",
      delta: t?.iyuu_rate_limited ? `限流 ${t.iyuu_rate_limited}` : undefined,
      deltaTone: "warn" as const,
      series: series7((c) => c.iyuu_calls),
      seriesHint: hint("每段请求 IYUU 的次数"),
      seriesBaseline: "zero" as const,
    },
  ];
});

function pointLong(date: string, end: string): string {
  if (stats.value?.custom && gran.value !== "day") return date === end ? date : `${date} 至 ${end}`;
  return brushPeriodLong(gran.value, date, end);
}

const trendPoints = computed<TrendPoint[]>(() => {
  const n = series.value.length;
  return series.value.map((p, i) => {
    const tag = !stats.value?.custom && i === n - 1 ? `（${meta.value.current}）` : "";
    return {
      short: brushPeriodShort(gran.value, p.date),
      long: `${pointLong(p.date, p.end)}${tag}`,
      value: p.succeeded,
      valueText: `成功 ${p.succeeded} 个`,
      detail: `建成 ${p.created} · 失败 ${reseedStatsFailed(p)} · 站点已删除 ${p.gone} · 跳过 ${reseedStatsSkipped(p)} · IYUU 请求 ${p.iyuu_calls}`,
    };
  });
});
/** 「明细」表：新的在前 */
const detailRows = computed(() =>
  series.value.map((p, i) => ({ p, long: trendPoints.value[i]!.long })).reverse(),
);

const skipRows = computed<BreakdownRow[]>(() => {
  const t = totals.value;
  if (!t) return [];
  return [
    { key: "present", label: "下载器里已经有", value: t.skip_present, tone: "mute" },
    { key: "tried", label: "已经尝试过", value: t.skip_tried, tone: "mute" },
    { key: "limit", label: "今天这个站点已经加满", value: t.skip_daily_limit, tone: "warn" },
    { key: "scope", label: "站点不在辅种范围", value: t.skip_not_selected, tone: "mute" },
    { key: "active", label: "已有进行中的任务", value: t.active, tone: "info" },
  ];
});

const iyuuToday = computed(() => reseedIYUUCallsText(props.settings?.iyuu_today ?? []));
const iyuuRange = computed(() => reseedIYUUCallsText(stats.value?.iyuu ?? []));
</script>

<template>
  <div class="rst" :aria-busy="loading" data-testid="rs-stats">
    <div class="rst-range" role="group" aria-label="统计范围">
      <!-- 不用 block：六档等宽时「自定义」在手机上放不下、被省略成「自.」 -->
      <el-segmented
        v-model="range"
        class="pt-seg"
        :options="RANGES"
        :props="{ label: 'label', value: 'value' }"
        data-testid="rs-stats-range" />
      <el-date-picker
        v-if="range === 'custom'"
        v-model="customDays"
        type="daterange"
        value-format="YYYY-MM-DD"
        unlink-panels
        range-separator="至"
        start-placeholder="开始日期"
        end-placeholder="结束日期"
        :disabled-date="disabledDate"
        class="rst-dates"
        data-testid="rs-stats-dates" />
      <span v-if="loading" class="rst-muted"
        ><PtIcon name="loader-circle" :size="14" /> 正在统计</span
      >
    </div>

    <div v-if="failed" class="pt-note pt-note--warn" data-testid="rs-stats-failed">
      <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
      <span>{{ stats ? `刷新失败：${failed}。下面是上次读到的。` : failed }}</span>
    </div>
    <p v-if="range === 'custom' && !customDays" class="rst-muted">
      选开始和结束的日期（最长 5 年）：62 天以内按天分段，一年以内按周，更长按月。
    </p>

    <template v-if="stats">
      <PtKpiBar :items="kpiItems" data-testid="rs-stats-kpi" />

      <div class="rst-grid">
        <PtPanel
          :title="`${windowText}成功辅种`"
          icon="chart-column"
          class="rst-wide"
          data-testid="rs-stats-trend">
          <template v-if="hasData" #actions>
            <el-button
              link
              size="small"
              :aria-expanded="showDetail"
              data-testid="rs-stats-detail-toggle"
              @click="showDetail = !showDetail">
              <PtIcon :name="showDetail ? 'chart-column' : 'table-2'" :size="14" />
              <span>{{ showDetail ? "看图" : "明细" }}</span>
            </el-button>
          </template>
          <template v-if="hasData">
            <BrushTrendChart
              v-if="!showDetail"
              :points="trendPoints"
              :label="`${windowText}${meta.each}成功的辅种`" />
            <div v-else class="rst-table" data-testid="rs-stats-table">
              <table>
                <thead>
                  <tr>
                    <th scope="col">时段</th>
                    <th scope="col">建成</th>
                    <th scope="col">成功</th>
                    <th scope="col">失败</th>
                    <th scope="col">撤回</th>
                    <th scope="col">站点已删除</th>
                    <th scope="col">跳过</th>
                    <th scope="col">IYUU 请求</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="row in detailRows" :key="row.p.date">
                    <th scope="row">{{ row.long }}</th>
                    <td>{{ row.p.created }}</td>
                    <td>{{ row.p.succeeded }}</td>
                    <td>{{ row.p.failed + row.p.job_failed }}</td>
                    <td>{{ row.p.rolled_back }}</td>
                    <td>{{ row.p.gone }}</td>
                    <td>{{ reseedStatsSkipped(row.p) }}</td>
                    <td>{{ row.p.iyuu_calls }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </template>
          <PtDataState
            v-else
            state="empty"
            dense
            :title="`${windowText}还没有辅种`"
            sub="开启辅种并运行一轮以后，这里按时段列出建成、成功、失败与跳过" />
          <p class="rst-foot">
            建成、失败、站点已删除与跳过按开始尝试的那一天，成功、撤回按任务完成的那一天；按服务器时区分段<template
              v-if="gran === 'week'"
              >，周从周一开始</template
            ><template v-if="stats.first_day"
              >。最早的数据是 {{ stats.first_day }}：升级前的辅种记录按记录的时间近似补进来，跳过与
              IYUU 请求从升级以后开始统计</template
            >。
          </p>
        </PtPanel>

        <PtPanel title="按站点" icon="globe" data-testid="rs-stats-sites">
          <div v-if="stats.sites.length" class="rst-table">
            <table>
              <thead>
                <tr>
                  <th scope="col">站点</th>
                  <th scope="col">建成</th>
                  <th scope="col">成功</th>
                  <th scope="col">失败</th>
                  <th scope="col">站点已删除</th>
                  <th scope="col">跳过</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="site in stats.sites" :key="site.site_name">
                  <th scope="row">{{ site.site_name || "不在辅种范围的站点" }}</th>
                  <td>{{ site.created }}</td>
                  <td>{{ site.succeeded }}</td>
                  <td>{{ reseedStatsFailed(site) }}</td>
                  <td>{{ site.gone }}</td>
                  <td>{{ reseedStatsSkipped(site) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
          <PtDataState v-else state="empty" dense sub="这个范围里还没有站点的数据" />
        </PtPanel>

        <PtPanel title="跳过的原因" icon="skip-forward" data-testid="rs-stats-skips">
          <PtBreakdown
            :rows="skipRows"
            foot="每一轮都会重新查一遍，同一个种子在每一轮都算一次跳过；「已有进行中的任务」是下载、核对以后才发现的。" />
        </PtPanel>

        <PtPanel title="IYUU 请求" icon="send" data-testid="rs-stats-iyuu">
          <ul class="rst-list">
            <li>
              <span class="rst-k">今天</span>
              <span>{{ iyuuToday || "还没有请求" }}</span>
            </li>
            <li>
              <span class="rst-k">这个范围</span>
              <span>{{ iyuuRange || "没有请求" }}</span>
            </li>
            <li>
              <span class="rst-k">最近一次</span>
              <span>{{
                stats.last_iyuu_at ? formatShortDateTime(stats.last_iyuu_at) : "还没有请求过"
              }}</span>
            </li>
            <li v-if="settings?.iyuu_cooldown_until" class="rst-warn">
              <span class="rst-k">限流冷却</span>
              <span>{{ formatShortDateTime(settings.iyuu_cooldown_until) }} 以前不请求</span>
            </li>
            <li v-if="settings?.iyuu_last_limit">
              <span class="rst-k">上次限流</span>
              <span
                >{{ formatShortDateTime(settings.iyuu_last_limit.at) }}
                {{ reseedIYUUEndpoint(settings.iyuu_last_limit.endpoint) }}：{{
                  settings.iyuu_last_limit.message || "请求太频繁"
                }}（IYUU 要求 {{ settings.iyuu_last_limit.retry_after }} 秒后再试<template
                  v-if="settings.iyuu_last_limit.reset_at"
                  >，限额
                  {{ formatShortDateTime(settings.iyuu_last_limit.reset_at) }} 重置</template
                >）</span
              >
            </li>
          </ul>
          <p class="rst-foot">
            IYUU
            推荐每天辅种一次，短时间内多次运行会被拒绝。两次请求的间隔与每天的上限在「设置」里改。
          </p>
        </PtPanel>
      </div>
    </template>
    <PtDataState v-else-if="!failed && range !== 'custom'" state="loading" sub="正在统计" />
  </div>
</template>

<style scoped>
.rst {
  display: flex;
  flex-direction: column;
  gap: var(--pt-pad);
  min-width: 0;
}

.rst-range {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2) var(--pt-space-3);
  align-items: center;
}

.rst-dates {
  max-width: 320px;
}

.rst-muted {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  margin: 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.rst-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--pt-pad);
}

.rst-wide {
  grid-column: 1 / -1;
}

@media (max-width: 1100px) {
  .rst-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}

.rst-foot {
  margin: var(--pt-space-2) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t3);
}

/* 明细与按站点的表：与刷流收益的明细表同一画法，第一列钉在左边，手机上横着滚 */
.rst-table {
  max-height: 300px;
  overflow: auto;
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-sm);
}

.rst-table table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--pt-fz-sm);
  font-variant-numeric: tabular-nums;
}

.rst-table th,
.rst-table td {
  padding: 6px 10px;
  text-align: right;
  white-space: nowrap;
  border-bottom: 1px solid var(--pt-border);
}

.rst-table thead th {
  position: sticky;
  top: 0;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: var(--pt-t3);
  background: var(--pt-surface);
}

.rst-table th:first-child {
  position: sticky;
  left: 0;
  z-index: 1;
  text-align: left;
  background: var(--pt-surface);
}

.rst-table thead th:first-child {
  z-index: 2;
}

.rst-table tbody th {
  font-weight: 500;
  color: var(--pt-t1);
}

.rst-table td {
  color: var(--pt-t2);
}

.rst-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: 0;
  list-style: none;
  font-size: 13px;
  color: var(--pt-t1);
}

.rst-list li {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.rst-k {
  flex: 0 0 72px;
  color: var(--pt-t3);
}

.rst-warn {
  color: var(--pt-warn);
}

@media (max-width: 768px) {
  .rst-table th,
  .rst-table td {
    padding: 6px 8px;
  }
}
</style>
