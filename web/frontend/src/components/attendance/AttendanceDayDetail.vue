<script setup lang="ts">
/*
 * 签到详情：某天每个站点签得怎么样 —— 成功还是失败、直接签到还是借助 AI（识别验证码、答题）、
 * AI 调了几次、用掉多少 token，展开一行看这一天的每次尝试（定时还是手动、结果、用时、原因）。
 * 日期由页面给（签到记录表格里点哪天看哪天），这里也能前后翻。
 */
import {
  type AttendanceDay,
  type AttendanceDaySite,
  type AttendanceMethod,
  attendanceApi,
} from "@/api";
import PtIcon from "@/components/PtIcon";
import SiteAvatar from "@/components/SiteAvatar.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import {
  aiUsageText,
  ATTENDANCE_METHOD_LABELS,
  attendanceDayView,
  attendanceDuration,
  attendanceEventView,
  tokenText,
} from "@/utils/attendanceStatus";
import { formatShortDateTime } from "@/utils/format";
import { computed, ref, watch } from "vue";

const props = defineProps<{
  /** 能看的日期，今天在前（签到记录的那几天） */
  days: string[];
}>();

/** 看哪一天（YYYY-MM-DD）；空时是今天 */
const day = defineModel<string>("day", { default: "" });

const isMobile = useIsMobile();
const ds = useDataState();
const data = ref<AttendanceDay | null>(null);

async function load() {
  const pending = ds.run(() => attendanceApi.events(day.value || undefined));
  const got = await pending;
  if (ds.isStale(pending) || !got) return;
  data.value = got;
}

watch(day, () => void load(), { immediate: true });

defineExpose({ reload: load });

const shownDay = computed(() => data.value?.day ?? day.value);
const index = computed(() => props.days.indexOf(shownDay.value));

function dayLabel(d: string): string {
  const md = d.slice(5).replace("-", "/");
  const i = props.days.indexOf(d);
  if (i === 0) return `${md} 今天`;
  if (i === 1) return `${md} 昨天`;
  return md;
}

/** 前一天（更早）、后一天（更近） */
function step(delta: number) {
  const i = index.value;
  if (i < 0) return;
  const next = props.days[i + delta];
  if (next) day.value = next;
}

const summaryText = computed(() => {
  const s = data.value?.summary;
  if (!s || !s.sites) return "";
  const parts = [`${s.sites} 个站点`, `成功 ${s.signed + s.already}`];
  if (s.failed) parts.push(`失败 ${s.failed}`);
  if (s.pending) parts.push(`没签完 ${s.pending}`);
  if (s.unsupported) parts.push(`不支持 ${s.unsupported}`);
  return parts.join(" · ");
});

const aiText = computed(() => {
  const s = data.value?.summary;
  if (!s?.ai_calls) return "";
  const tokens =
    s.prompt_tokens || s.completion_tokens
      ? ` · 输入 ${tokenText(s.prompt_tokens)} / 输出 ${tokenText(s.completion_tokens)} token`
      : "";
  return `借助 AI ${s.ai_sites} 个站点 · 调用 ${s.ai_calls} 次${tokens}`;
});

const sites = computed(() => data.value?.sites ?? []);

/** 这一天的说明：成功写站点回的话，没成写原因，还在等的写什么时候再试 */
function detailText(s: AttendanceDaySite): string {
  if (s.status === "signed" || s.status === "already") return s.message || "签到成功";
  if (s.status === "pending" && s.next_attempt_at) {
    return `${s.last_error || "没签上"}；${formatShortDateTime(s.next_attempt_at * 1000)} 再试`;
  }
  return s.last_error || s.message || (s.status === "pending" ? "还没到签到的时间" : "");
}

function methodLabel(m: string): string {
  return ATTENDANCE_METHOD_LABELS[m as AttendanceMethod] ?? m;
}

function timeOf(unix?: number): string {
  if (!unix) return "-";
  const d = new Date(unix * 1000);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

function lastTime(s: AttendanceDaySite): string {
  const last = s.events[s.events.length - 1];
  return timeOf(last?.finished_at ?? s.last_attempt_at);
}

/** 手机上展开了哪些站点的尝试 */
const openSites = ref<string[]>([]);
function toggleSite(name: string) {
  openSites.value = openSites.value.includes(name)
    ? openSites.value.filter((n) => n !== name)
    : [...openSites.value, name];
}
</script>

<template>
  <div class="att-day" data-testid="att-day">
    <div class="att-day__bar">
      <div class="att-day__nav">
        <button
          type="button"
          class="att-day__step"
          :disabled="index < 0 || index >= days.length - 1"
          aria-label="前一天"
          data-testid="att-day-prev"
          @click="step(1)">
          <PtIcon name="chevron-left" :size="15" />
        </button>
        <el-select
          :model-value="shownDay"
          class="att-day__pick"
          size="small"
          aria-label="看哪一天"
          data-testid="att-day-pick"
          @update:model-value="(v: string) => (day = v)">
          <el-option v-for="d in days" :key="d" :value="d" :label="dayLabel(d)" />
        </el-select>
        <button
          type="button"
          class="att-day__step"
          :disabled="index <= 0"
          aria-label="后一天"
          data-testid="att-day-next"
          @click="step(-1)">
          <PtIcon name="chevron-right" :size="15" />
        </button>
      </div>
      <div class="att-day__sum">
        <span v-if="summaryText" data-testid="att-day-summary">{{ summaryText }}</span>
        <span v-if="aiText" class="att-day__ai" data-testid="att-day-ai">
          <PtIcon name="sparkles" :size="13" />{{ aiText }}
        </span>
      </div>
    </div>

    <PtDataState
      v-if="!sites.length"
      dense
      :state="ds.state.value"
      :title="ds.error.value ? '' : '这一天没有签到记录'"
      :sub="ds.errorText.value || '开了自动签到的站点签过以后，每次尝试都会记在这里'">
      <template v-if="ds.error.value" #action>
        <el-button size="small" @click="load">
          <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
        </el-button>
      </template>
    </PtDataState>

    <el-table
      v-else-if="!isMobile"
      :data="sites"
      row-key="site_name"
      class="pt-grid"
      data-testid="att-day-table">
      <el-table-column type="expand" width="36">
        <template #default="{ row }">
          <ol v-if="row.events.length" class="att-ev" :data-testid="`att-ev-${row.site_name}`">
            <li v-for="e in row.events" :key="e.id" class="att-ev__item">
              <span class="att-ev__time">{{ timeOf(e.started_at) }}</span>
              <span class="att-ev__trigger">{{ e.trigger === "manual" ? "手动" : "定时" }}</span>
              <PtStatusPill :tone="attendanceEventView(e).tone" size="sm">
                {{ attendanceEventView(e).label }}
              </PtStatusPill>
              <span class="att-ev__method">{{ methodLabel(e.method) }}</span>
              <span v-if="attendanceDuration(e.duration_ms)" class="att-ev__muted">{{
                attendanceDuration(e.duration_ms)
              }}</span>
              <span v-if="aiUsageText(e)" class="att-ev__muted">{{ aiUsageText(e) }}</span>
              <span class="att-ev__text" :class="{ 'is-error': e.error }">{{
                e.error || e.message
              }}</span>
            </li>
          </ol>
          <p v-else class="att-ev__none">
            这一天没有每次尝试的记录（这个功能上线以前的签到只有当天的结果）。
          </p>
        </template>
      </el-table-column>
      <el-table-column label="站点" min-width="150" class-name="pt-cell-strong">
        <template #default="{ row }">
          <span class="att-day__site">
            <SiteAvatar
              :site-id="row.site_name"
              :site-name="row.site_name"
              :size="20"
              :no-fetch="true" />
            <span class="att-day__name">{{ row.site_name }}</span>
          </span>
        </template>
      </el-table-column>
      <el-table-column label="结果" width="104">
        <template #default="{ row }">
          <PtStatusPill :tone="attendanceDayView(row).tone" size="sm">
            {{ attendanceDayView(row).label }}
          </PtStatusPill>
        </template>
      </el-table-column>
      <el-table-column label="方式" width="118">
        <template #default="{ row }">
          <span :class="{ 'att-day__aim': row.method !== 'direct' }">{{
            methodLabel(row.method)
          }}</span>
        </template>
      </el-table-column>
      <el-table-column label="尝试" width="64" align="center" class-name="pt-cell-muted">
        <template #default="{ row }">{{ row.events.length || row.attempts || "-" }}</template>
      </el-table-column>
      <el-table-column
        label="AI 用量"
        min-width="200"
        class-name="pt-cell-1line pt-cell-muted"
        show-overflow-tooltip>
        <template #default="{ row }">{{ aiUsageText(row) || "-" }}</template>
      </el-table-column>
      <el-table-column label="时间" width="88" class-name="pt-cell-muted">
        <template #default="{ row }">{{ lastTime(row) }}</template>
      </el-table-column>
      <el-table-column
        label="说明"
        min-width="200"
        class-name="pt-cell-1line"
        show-overflow-tooltip>
        <template #default="{ row }">{{ detailText(row) }}</template>
      </el-table-column>
    </el-table>

    <div v-else class="att-day__cards">
      <PtRowCard
        v-for="s in sites"
        :key="s.site_name"
        interactive
        :data-testid="`att-day-card-${s.site_name}`"
        @click="toggleSite(s.site_name)">
        <template #lead>
          <SiteAvatar :site-id="s.site_name" :site-name="s.site_name" :size="32" :no-fetch="true" />
        </template>
        <template #title>{{ s.site_name }}</template>
        <template #meta>
          <span>{{ methodLabel(s.method) }}</span>
          <span v-if="aiUsageText(s)">{{ aiUsageText(s) }}</span>
          <span>{{ detailText(s) }}</span>
          <ol v-if="openSites.includes(s.site_name) && s.events.length" class="att-ev">
            <li v-for="e in s.events" :key="e.id" class="att-ev__item">
              <span class="att-ev__time">{{ timeOf(e.started_at) }}</span>
              <span class="att-ev__trigger">{{ e.trigger === "manual" ? "手动" : "定时" }}</span>
              <PtStatusPill :tone="attendanceEventView(e).tone" size="sm">
                {{ attendanceEventView(e).label }}
              </PtStatusPill>
              <span class="att-ev__text" :class="{ 'is-error': e.error }">{{
                e.error || e.message
              }}</span>
            </li>
          </ol>
        </template>
        <template #status>
          <PtStatusPill :tone="attendanceDayView(s).tone" size="sm">
            {{ attendanceDayView(s).label }}
          </PtStatusPill>
        </template>
      </PtRowCard>
    </div>
  </div>
</template>

<style scoped>
.att-day {
  display: flex;
  flex-direction: column;
}

.att-day__bar {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-2) var(--pt-space-4);
  align-items: center;
  padding: var(--pt-space-3) var(--pt-pad);
}

.att-day__nav {
  display: inline-flex;
  gap: 4px;
  align-items: center;
}

.att-day__step {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
  background: var(--pt-surface);
  color: var(--pt-t2);
  cursor: pointer;
}

.att-day__step:hover:not(:disabled) {
  background: var(--pt-hover);
  color: var(--pt-t1);
}

.att-day__step:disabled {
  color: var(--pt-t4);
  cursor: not-allowed;
}

.att-day__step:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 1px;
}

.att-day__pick {
  width: 120px;
}

.att-day__sum {
  display: flex;
  flex-wrap: wrap;
  gap: 4px var(--pt-space-4);
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.att-day__ai {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  color: var(--pt-p);
}

.att-day__site {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
  min-width: 0;
}

.att-day__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 借助 AI 的方式用主色，和「今天的签到」里站点名后面的 AI 小标一致 */
.att-day__aim {
  color: var(--pt-p);
}

.att-day__cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: 0 var(--pt-space-3) var(--pt-space-3);
}

/* 展开的每次尝试：一行一次，按时间排 */
.att-ev {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: 0;
  padding: var(--pt-space-2) var(--pt-pad) var(--pt-space-3) 52px;
  list-style: none;
}

.att-day__cards .att-ev {
  padding: var(--pt-space-2) 0 0;
}

.att-ev__item {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 10px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.att-ev__time {
  font-family: var(--pt-font-mono);
  color: var(--pt-t1);
}

.att-ev__trigger,
.att-ev__muted {
  color: var(--pt-t3);
}

.att-ev__method {
  color: var(--pt-t2);
}

.att-ev__text {
  flex: 1 1 200px;
  min-width: 0;
  color: var(--pt-t2);
  word-break: break-word;
}

.att-ev__text.is-error {
  color: var(--pt-dang);
}

.att-ev__none {
  margin: 0;
  padding: var(--pt-space-2) var(--pt-pad) var(--pt-space-3) 52px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
}
</style>
