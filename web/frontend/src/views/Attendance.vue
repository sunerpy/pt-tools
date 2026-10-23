<script setup lang="ts">
/*
 * 自动签到：每个站点今天签到了没有、开关、立即签到与一键签到，最近两周的签到记录、某天的签到详情，以及签到设置。
 * 以前开关在「站点列表 → 保号配置」里、设置在「全局设置 → 每日签到」里，签到结果只能在站点详情里一个个看；
 * 这一页把它们放在一起。画板没有这一页，沿用站点组列表页的样式：页头 + 面板里的表格，手机是行卡。
 */
import {
  type AttendanceHistory,
  type AttendanceRunStatus,
  attendanceApi,
  type SiteAttendance,
} from "@/api";
import AttendanceDayDetail from "@/components/attendance/AttendanceDayDetail.vue";
import AttendanceSettingsForm from "@/components/attendance/AttendanceSettingsForm.vue";
import PtIcon from "@/components/PtIcon";
import SiteAvatar from "@/components/SiteAvatar.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtRowCard from "@/components/ui/PtRowCard.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useDataState } from "@/composables/useDataState";
import { useIsMobile } from "@/composables/useIsMobile";
import {
  attendanceChallengeText,
  type AttendanceTone,
  attendanceView,
} from "@/utils/attendanceStatus";
import { formatShortDateTime } from "@/utils/format";
import { isInteractiveClick, ROW_LINK_CLASS } from "@/utils/rowClick";
import { ElMessage } from "element-plus";
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";

type Scope = "enabled" | "auto" | "all";

/** 一键签到进行中多久刷新一次进度与列表 */
const RUN_POLL_MS = 2000;
/** 签到记录看最近几天 */
const HISTORY_DAYS = 14;

const router = useRouter();
const isMobile = useIsMobile();
const scope = ref<Scope>("enabled");
const listDS = useDataState({ filtered: () => scope.value !== "all" });

const sites = ref<SiteAttendance[]>([]);
const loaded = ref(false);
const history = ref<AttendanceHistory | null>(null);
/** 签到记录没读出来：和「还没有记录」分开说，否则读失败时看上去像从来没签过 */
const historyFailed = ref(false);
const run = ref<AttendanceRunStatus | null>(null);
const starting = ref(false);
const signing = ref<Record<string, boolean>>({});
const toggling = ref<Record<string, boolean>>({});
/** 签到详情看哪一天；空是今天 */
const detailDay = ref("");
const dayDetail = ref<InstanceType<typeof AttendanceDayDetail> | null>(null);
let poll: ReturnType<typeof setInterval> | undefined;

const SCOPES = computed(() => [
  { label: `已启用的站点 ${sites.value.filter((s) => s.site_enabled).length}`, value: "enabled" },
  { label: `开了自动签到 ${autoSites.value.length}`, value: "auto" },
  { label: `全部 ${sites.value.length}`, value: "all" },
]);

const autoSites = computed(() =>
  sites.value.filter((s) => s.attendance_enabled && s.site_enabled && s.supported),
);

/** 表格里的行：已启用、支持签到的排前面，开了自动签到的再往前 */
const rows = computed(() => {
  const list = sites.value.filter((s) => {
    if (scope.value === "enabled") return s.site_enabled;
    if (scope.value === "auto") return s.attendance_enabled && s.site_enabled;
    return true;
  });
  const rank = (s: SiteAttendance) =>
    (s.attendance_enabled ? 0 : 2) + (s.supported && s.site_enabled ? 0 : 1);
  return [...list].sort((a, b) => rank(a) - rank(b) || a.site_name.localeCompare(b.site_name));
});

/** 页头摘要：今天开了自动签到的站点里签上了几个、失败几个、还在等的几个 */
const counts = computed(() => {
  let signed = 0;
  let failed = 0;
  let waiting = 0;
  for (const s of autoSites.value) {
    if (s.status === "signed" || s.status === "already") signed++;
    else if (s.status === "failed") failed++;
    else if (s.status !== "unsupported") waiting++;
  }
  return { signed, failed, waiting };
});

const headSub = computed(() => {
  if (!loaded.value) return "";
  const n = autoSites.value.length;
  if (n === 0) return "还没有站点开启自动签到：在下面的表格里打开站点的开关";
  const c = counts.value;
  const parts = [`${n} 个站点开了自动签到`, `今天已签 ${c.signed}`];
  if (c.failed) parts.push(`失败 ${c.failed}`);
  if (c.waiting) parts.push(`待签 ${c.waiting}`);
  return parts.join(" · ");
});

const running = computed(() => Boolean(run.value?.running));
const runText = computed(() => {
  const r = run.value;
  if (!r?.running) return "";
  return r.total > 0 ? `一键签到进行中：${r.done} / ${r.total}` : "一键签到进行中";
});

async function loadList() {
  const pending = listDS.run(() => attendanceApi.list());
  const data = await pending;
  if (listDS.isStale(pending)) return;
  if (data) {
    sites.value = data;
    loaded.value = true;
  }
}

async function loadHistory() {
  try {
    history.value = await attendanceApi.history(HISTORY_DAYS);
    historyFailed.value = false;
  } catch {
    history.value = null;
    historyFailed.value = true;
  }
  // 签到记录变了，签到详情跟着刷新（第一次由它自己读）
  void dayDetail.value?.reload();
}

/** 点签到记录里的某一天：签到详情换成那一天 */
function showDay(day: string) {
  detailDay.value = day;
}

async function loadRun() {
  try {
    run.value = await attendanceApi.runStatus();
  } catch {
    run.value = null;
  }
}

function refresh() {
  void loadList();
  void loadHistory();
  void loadRun();
}

function stopPoll() {
  if (poll) clearInterval(poll);
  poll = undefined;
}

/** 一键签到在跑时每 2 秒刷新进度和列表；跑完再刷一次记录并提示 */
function startPoll() {
  if (poll) return;
  poll = setInterval(async () => {
    await loadRun();
    await loadList();
    if (!run.value?.running) {
      stopPoll();
      await loadHistory();
      const r = run.value;
      if (r?.error) ElMessage.warning(`一键签到没有做完：${r.error}`);
      else ElMessage.success(`一键签到做完了：处理了 ${r?.total ?? 0} 个站点`);
    }
  }, RUN_POLL_MS);
}

async function runAll() {
  if (autoSites.value.length === 0) {
    ElMessage.warning("还没有站点开启自动签到：先在表格里打开要签到的站点");
    return;
  }
  starting.value = true;
  try {
    run.value = await attendanceApi.runAll();
    startPoll();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "一键签到没有开始");
  } finally {
    starting.value = false;
  }
}

function replaceRow(row: SiteAttendance) {
  sites.value = sites.value.map((s) => (s.site_name === row.site_name ? { ...s, ...row } : s));
}

async function signNow(s: SiteAttendance) {
  signing.value = { ...signing.value, [s.site_name]: true };
  try {
    const row = await attendanceApi.signNow(s.site_name);
    replaceRow(row);
    const v = attendanceView(row);
    if (v.tone === "ok") ElMessage.success(`${s.site_name}：${v.detail}`);
    else ElMessage.warning(`${s.site_name}：${v.detail}`);
    void loadHistory();
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "签到失败");
  } finally {
    signing.value = { ...signing.value, [s.site_name]: false };
  }
}

async function toggle(s: SiteAttendance, enabled: boolean) {
  toggling.value = { ...toggling.value, [s.site_name]: true };
  try {
    const row = await attendanceApi.setEnabled(s.site_name, enabled);
    // 开关接口只回开关和支持情况，当天的结果还是原来那条
    replaceRow({ ...s, attendance_enabled: row.attendance_enabled });
  } catch (e: unknown) {
    ElMessage.error((e as Error).message || "保存失败");
  } finally {
    toggling.value = { ...toggling.value, [s.site_name]: false };
  }
}

function toggleTip(s: SiteAttendance): string {
  if (!s.supported) return s.unsupported_reason || "该站点不支持自动签到";
  if (!s.site_enabled) return "站点没有启用：先在站点列表里启用";
  return s.attendance_enabled ? "点一下关闭自动签到" : "点一下开启自动签到";
}

/** 时间列：签过的写签到时间，还在等的写计划时间 */
/** 这张表是「今天的签到」：今天的时刻只写时分，日期写出来是重复；跨了天（时区边界）才带上日期 */
function clockText(sec: number): string {
  const d = new Date(sec * 1000);
  const now = new Date();
  const sameDay =
    d.getFullYear() === now.getFullYear() &&
    d.getMonth() === now.getMonth() &&
    d.getDate() === now.getDate();
  if (!sameDay) return formatShortDateTime(sec * 1000);
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
}

function timeText(s: SiteAttendance): string {
  if (s.last_attempt_at) return clockText(s.last_attempt_at);
  if (s.status === "pending" && s.scheduled_at) return `计划 ${clockText(s.scheduled_at)}`;
  return "-";
}

function openSite(name: string, event?: Event) {
  if (isInteractiveClick(event)) return;
  router.push(`/sites/${name}`);
}

function onRowClick(row: SiteAttendance, _column: unknown, event: Event) {
  openSite(row.site_name, event);
}

// ---- 签到记录：站点 × 最近 14 天的格子，从左到右是从早到今天 ----
const historyDays = computed(() => [...(history.value?.days ?? [])].reverse());

const historyRows = computed(() => {
  const h = history.value;
  if (!h) return [];
  const byKey = new Map(h.rows.map((r) => [`${r.site_name}|${r.day}`, r]));
  const withRecords = new Set(h.rows.map((r) => r.site_name));
  // 有记录的站点，加上开了自动签到但这两周还没有记录的站点
  const names = new Set([...withRecords, ...autoSites.value.map((s) => s.site_name)]);
  return [...names].sort().map((name) => ({
    name,
    cells: historyDays.value.map((day) => {
      const r = byKey.get(`${name}|${day}`);
      return { day, row: r, ...cellView(r) };
    }),
  }));
});

function cellView(r?: AttendanceHistory["rows"][number]): {
  tone: AttendanceTone | "";
  tip: string;
} {
  if (!r) return { tone: "", tip: "没有记录" };
  switch (r.status) {
    case "signed":
      return { tone: "ok", tip: r.message || "签到成功" };
    case "already":
      return { tone: "ok", tip: r.message || "已经签到过" };
    case "failed":
      return { tone: "dang", tip: r.last_error || "签到失败" };
    case "unsupported":
      return { tone: "neutral", tip: r.last_error || "不支持自动签到" };
    default:
      return { tone: "warn", tip: r.last_error || "当天没有签完" };
  }
}

function dayLabel(day: string): string {
  return day.slice(5).replace("-", "/");
}

onMounted(async () => {
  await Promise.all([loadList(), loadHistory(), loadRun()]);
  if (run.value?.running) startPoll();
});
onBeforeUnmount(stopPoll);
</script>

<template>
  <div class="pt-cards pt-cards--main">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button data-testid="att-refresh" @click="refresh">
        <PtIcon name="refresh-cw" :size="15" /><span>刷新</span>
      </el-button>
      <el-button
        type="primary"
        :loading="starting || running"
        :disabled="running"
        data-testid="att-run-all"
        @click="runAll">
        <PtIcon v-if="!starting && !running" name="calendar-check" :size="15" />
        <span>{{ running ? runText : "一键签到" }}</span>
      </el-button>
    </Teleport>

    <!-- 左栏：今天的签到与最近两周的记录 -->
    <div class="pt-cards pt-cards--wide att-left">
      <PtPanel title="今天的签到" icon="calendar-check" :count="rows.length" padding="none">
        <template #actions>
          <el-segmented
            v-model="scope"
            class="pt-seg"
            :options="SCOPES"
            :props="{ label: 'label', value: 'value' }"
            size="small"
            data-testid="att-scope" />
        </template>
        <div v-if="running" class="pt-note att-run" data-testid="att-running">
          <PtIcon name="loader-circle" :size="14" class="pt-note__icon att-spin" />
          <span>{{ runText }}。要识别验证码的站点一个最多要几分钟，可以先离开这一页</span>
        </div>
        <PtDataState
          v-if="!rows.length"
          dense
          :state="listDS.state.value"
          :title="
            listDS.error.value ? '' : scope === 'auto' ? '还没有站点开启自动签到' : '没有站点'
          "
          :sub="
            listDS.errorText.value ||
            (scope === 'auto' ? '切到「已启用的站点」，打开要签到的站点的开关' : '')
          ">
          <template v-if="listDS.error.value" #action>
            <el-button size="small" @click="loadList">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>

        <el-table
          v-else-if="!isMobile"
          :data="rows"
          row-key="site_name"
          class="pt-grid"
          :row-class-name="() => ROW_LINK_CLASS"
          data-testid="att-table"
          @row-click="onRowClick">
          <!--
            列宽按左栏的实际宽度排（1440 下约 734）：以前六列加起来 884，「操作」整列挤出首屏，
            逐站的「立即签到」要横向滚动才看得到。现在说明列吃剩下的宽度，时间只写时分（今天的表），
            操作列固定在右边：170 + 80 + 88 + 176 + 100 + 104 = 718。
          -->
          <el-table-column label="站点" min-width="170" class-name="pt-cell-strong">
            <template #default="{ row }">
              <span class="att-site">
                <SiteAvatar
                  :site-id="row.site_name"
                  :site-name="row.site_name"
                  :size="20"
                  :no-fetch="true" />
                <span class="att-site__name">{{ row.site_name }}</span>
                <el-tooltip
                  v-if="row.challenge"
                  :content="attendanceChallengeText(row.challenge)"
                  placement="top">
                  <span class="att-ai">AI</span>
                </el-tooltip>
              </span>
            </template>
          </el-table-column>
          <el-table-column label="自动签到" width="80" align="center">
            <template #default="{ row }">
              <el-tooltip :content="toggleTip(row)" placement="top">
                <span class="att-sw">
                  <el-switch
                    :model-value="row.attendance_enabled"
                    size="small"
                    :loading="toggling[row.site_name]"
                    :disabled="!row.supported || !row.site_enabled"
                    :data-testid="`att-toggle-${row.site_name}`"
                    @change="(v: string | number | boolean) => toggle(row, Boolean(v))" />
                </span>
              </el-tooltip>
            </template>
          </el-table-column>
          <el-table-column label="今天" width="88">
            <template #default="{ row }">
              <PtStatusPill :tone="attendanceView(row).tone" size="sm">
                {{ attendanceView(row).label }}
              </PtStatusPill>
            </template>
          </el-table-column>
          <el-table-column
            label="说明"
            min-width="176"
            class-name="pt-cell-1line"
            show-overflow-tooltip>
            <template #default="{ row }">{{ attendanceView(row).detail }}</template>
          </el-table-column>
          <el-table-column label="时间" width="100" class-name="pt-cell-muted">
            <template #default="{ row }">{{ timeText(row) }}</template>
          </el-table-column>
          <el-table-column
            label="操作"
            width="104"
            align="center"
            fixed="right"
            class-name="pt-cell-act">
            <template #default="{ row }">
              <el-button
                size="small"
                :loading="signing[row.site_name]"
                :disabled="!attendanceView(row).canSign || running"
                :data-testid="`att-sign-${row.site_name}`"
                @click="signNow(row)">
                立即签到
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <div v-else class="att-cards">
          <PtRowCard
            v-for="s in rows"
            :key="s.site_name"
            interactive
            :data-testid="`att-card-${s.site_name}`"
            @click="openSite(s.site_name, $event)">
            <template #lead>
              <SiteAvatar
                :site-id="s.site_name"
                :site-name="s.site_name"
                :size="32"
                :no-fetch="true" />
            </template>
            <template #title>
              {{ s.site_name }}<span v-if="s.challenge" class="att-ai">AI</span>
            </template>
            <template #meta>
              <span>{{ attendanceView(s).detail }}</span>
            </template>
            <template #status>
              <PtStatusPill :tone="attendanceView(s).tone" size="sm">
                {{ attendanceView(s).label }}
              </PtStatusPill>
            </template>
            <template #actions>
              <el-switch
                :model-value="s.attendance_enabled"
                :loading="toggling[s.site_name]"
                :disabled="!s.supported || !s.site_enabled"
                inline-prompt
                active-text="自动"
                inactive-text="手动"
                @change="(v: string | number | boolean) => toggle(s, Boolean(v))" />
              <el-button
                size="small"
                :loading="signing[s.site_name]"
                :disabled="!attendanceView(s).canSign || running"
                @click="signNow(s)">
                立即签到
              </el-button>
            </template>
          </PtRowCard>
        </div>
      </PtPanel>

      <PtPanel title="签到记录" icon="history" :count="`最近 ${HISTORY_DAYS} 天`" padding="none">
        <PtDataState v-if="historyFailed" dense state="error" sub="签到记录没读出来，可以重试">
          <template #action>
            <el-button size="small" @click="loadHistory">
              <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
            </el-button>
          </template>
        </PtDataState>
        <PtDataState
          v-else-if="!historyRows.length"
          dense
          state="empty"
          title="还没有签到记录"
          sub="开了自动签到的站点每天签完以后，结果会记在这里" />
        <div v-else class="att-hist" data-testid="att-history">
          <table class="att-hist__grid">
            <thead>
              <tr>
                <th class="att-hist__site">站点</th>
                <th v-for="d in historyDays" :key="d">
                  <button
                    type="button"
                    class="att-hist__day"
                    :class="{ 'is-active': d === (detailDay || history?.days[0]) }"
                    :aria-label="`看 ${d} 的签到详情`"
                    :data-testid="`att-hist-day-${d}`"
                    @click="showDay(d)">
                    {{ dayLabel(d) }}
                  </button>
                </th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="r in historyRows" :key="r.name">
                <td class="att-hist__site">{{ r.name }}</td>
                <td
                  v-for="c in r.cells"
                  :key="c.day"
                  class="att-hist__cell"
                  :class="{ 'is-active': c.day === (detailDay || history?.days[0]) }"
                  @click="showDay(c.day)">
                  <el-tooltip :content="`${c.day} · ${c.tip}`" placement="top" :show-after="200">
                    <span
                      class="att-dot"
                      :class="c.tone ? `att-dot--${c.tone}` : 'att-dot--none'" />
                  </el-tooltip>
                </td>
              </tr>
            </tbody>
          </table>
          <p class="att-hist__legend">
            <span><i class="att-dot att-dot--ok" />签上了</span>
            <span><i class="att-dot att-dot--dang" />失败</span>
            <span><i class="att-dot att-dot--warn" />当天没签完</span>
            <span><i class="att-dot att-dot--none" />没有记录</span>
            <span class="att-hist__hint">点日期看那一天的签到详情</span>
          </p>
        </div>
      </PtPanel>

      <!--
        没有任何签到记录时不画「签到详情」：它和上面那张卡是同一个前提，
        两块一样的空态叠在一起只是重复（第一次有记录以后它就出来）。
      -->
      <PtPanel
        v-if="historyRows.length"
        title="签到详情"
        icon="list-checks"
        padding="none"
        data-testid="att-detail">
        <AttendanceDayDetail ref="dayDetail" v-model:day="detailDay" :days="history?.days ?? []" />
      </PtPanel>
    </div>

    <!-- 右栏：签到设置 -->
    <PtPanel title="签到设置" icon="settings">
      <AttendanceSettingsForm />
    </PtPanel>
  </div>
</template>

<style scoped>
/*
 * 左栏两张卡按自己的内容高。外层两栏的行高跟着右边那张长的「签到设置」走，
 * 左栏这层栅格不顶对齐的话，多出来的高度会摊进两张卡里，表格底下空出一大块。
 */
.att-left {
  align-self: start;
}

.att-site {
  display: inline-flex;
  gap: var(--pt-space-2);
  align-items: center;
  min-width: 0;
}

.att-site__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 要用 AI 过验证码或答题的站点：名字后面一枚小标 */
.att-ai {
  flex: 0 0 auto;
  padding: 0 5px;
  margin-left: 4px;
  font-size: 10px;
  font-weight: 600;
  line-height: 16px;
  color: var(--pt-p);
  background: var(--pt-p-soft);
  border-radius: var(--pt-r-sm);
}

.att-sw {
  display: inline-flex;
}

.att-run {
  margin: var(--pt-space-3) var(--pt-pad) 0;
}

.att-spin {
  animation: att-spin 1s linear infinite;
}

@keyframes att-spin {
  to {
    transform: rotate(360deg);
  }
}

.att-cards {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  padding: var(--pt-space-3);
}

/* 签到记录：站点 × 天的格子，横向放不下时在面板里横滚 */
.att-hist {
  padding: var(--pt-space-3) var(--pt-pad) var(--pt-pad);
  overflow-x: auto;
}

.att-hist__grid {
  border-collapse: collapse;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.att-hist__grid th {
  padding: 0 4px 6px;
  font-weight: 500;
  white-space: nowrap;
}

.att-hist__grid td {
  height: 26px;
  padding: 0 4px;
  text-align: center;
}

.att-hist__site {
  max-width: 160px;
  padding-right: var(--pt-space-3) !important;
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t1);
  text-align: left !important;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.att-dot {
  display: inline-block;
  width: 12px;
  height: 12px;
  vertical-align: middle;
  border-radius: 3px;
}

.att-dot--ok {
  background: var(--pt-ok);
}

.att-dot--dang {
  background: var(--pt-dang);
}

.att-dot--warn {
  background: var(--pt-warn);
}

.att-dot--info,
.att-dot--neutral {
  background: var(--pt-t4);
}

.att-dot--none {
  background: var(--pt-hover);
  box-shadow: inset 0 0 0 1px var(--pt-border);
}

.att-hist__legend {
  display: flex;
  flex-wrap: wrap;
  gap: var(--pt-space-4);
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.att-hist__legend span {
  display: inline-flex;
  gap: 6px;
  align-items: center;
}

.att-hist__hint {
  margin-left: auto;
}

/* 日期是按钮：点了看那一天的签到详情；看着的那一天整列标出来 */
.att-hist__day {
  padding: 2px 4px;
  border: 0;
  border-radius: var(--pt-r-sm);
  background: transparent;
  color: inherit;
  font: inherit;
  white-space: nowrap;
  cursor: pointer;
}

.att-hist__day:hover {
  color: var(--pt-t1);
}

.att-hist__day.is-active {
  background: var(--pt-p-soft);
  color: var(--pt-p);
}

.att-hist__day:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 1px;
}

.att-hist__cell {
  cursor: pointer;
}

.att-hist__cell.is-active {
  background: color-mix(in srgb, var(--pt-p-soft) 60%, transparent);
}
</style>
