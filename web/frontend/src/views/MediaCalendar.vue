<script setup lang="ts">
/*
 * 订阅日历（媒体中心 M3）：订阅的剧集每一集哪天播出、电影哪天上映，入库了没有；按播出时间追剧的写播出时刻。
 * 月视图、周视图（周一开始），手机按天列出。点一项打开条目详情。画板没有这一页，沿用媒体页的样式：页头 + 面板。
 * 每天推送今天的更新在「追剧与订阅」的订阅设置里开。
 */
import { type CalendarEntry, type MediaCalendar, subscribeApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import EntryPoster from "@/components/search/EntryPoster.vue";
import TmdbKeyNotice from "@/components/media/TmdbKeyNotice.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import PtPanel from "@/components/ui/PtPanel.vue";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import { useTmdbImageBase } from "@/composables/useTmdbImageBase";
import {
  type CalendarView,
  WEEK_HEAD,
  dayKey,
  entryCode,
  entryDetail,
  entryTime,
  groupByDay,
  parseDay,
  rangeDays,
  rangeLabel,
  shiftAnchor,
  weekdayOf,
} from "@/utils/calendar";
import { episodeStatePill, itemLink } from "@/utils/mediaItem";
import { computed, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

const route = useRoute();
const router = useRouter();
const isMobile = useIsMobile();
useTmdbImageBase();

/** 月视图的一格最多列几项，多的写「还有 N 项」，点了换成那一周 */
const MONTH_CELL_MAX = 3;

const view = ref<CalendarView>(route.query.view === "week" ? "week" : "month");
const anchor = ref<Date>(parseDay(route.query.date) ?? new Date());
const today = ref(dayKey(new Date()));
const days = computed(() => rangeDays(view.value, anchor.value));
const label = computed(() => rangeLabel(view.value, anchor.value));
const month = computed(() => anchor.value.getMonth());

const data = ref<MediaCalendar | null>(null);
const loading = ref(false);
const error = ref("");
let seq = 0;

async function load() {
  const n = ++seq;
  const list = days.value;
  loading.value = true;
  error.value = "";
  try {
    const got = await subscribeApi.calendar(list[0], list[list.length - 1]);
    if (n !== seq) return;
    data.value = got;
    today.value = got.today || today.value;
  } catch (e) {
    if (n !== seq) return;
    error.value = (e as Error)?.message || "读取日历失败";
  } finally {
    if (n === seq) loading.value = false;
  }
}

watch(
  () => [view.value, days.value[0]] as const,
  () => {
    void router.replace({ query: { view: view.value, date: dayKey(anchor.value) } });
    void load();
  },
  { immediate: true },
);

const byDay = computed(() => groupByDay(data.value?.entries ?? []));
const total = computed(() => data.value?.entries.length ?? 0);
/** 手机按天列出：只列有内容的天 */
const listedDays = computed(() => days.value.filter((d) => byDay.value[d]?.length));
const headSub = computed(() => {
  if (!data.value) return "订阅的剧集哪天播出、电影哪天上映";
  const eps = data.value.entries.filter((e) => e.media_type === "tv").length;
  const movies = total.value - eps;
  const parts = [`${eps} 集`];
  if (movies) parts.push(`${movies} 部电影`);
  const inLib = data.value.entries.filter((e) => e.state === "library").length;
  parts.push(`已入库 ${inLib}`);
  return `${label.value} · ${parts.join(" · ")}`;
});

function go(n: number) {
  anchor.value = shiftAnchor(view.value, anchor.value, n);
}

function goToday() {
  anchor.value = new Date();
}

function showWeek(day: string) {
  const d = parseDay(day);
  if (!d) return;
  anchor.value = d;
  view.value = "week";
}

function open(e: CalendarEntry) {
  void router.push(itemLink(e.media_type, e.tmdb_id, "calendar", e.season));
}

function dayNumber(day: string): number {
  return Number(day.slice(8));
}

function inMonth(day: string): boolean {
  return Number(day.slice(5, 7)) - 1 === month.value;
}

/** 悬停的提示：名字、季与集、集名，按播出时间追剧的带上时刻（月视图的一项放不下时刻，写在这里） */
function entryHint(e: CalendarEntry): string {
  const time = entryTime(e);
  return `${e.title} ${entryDetail(e)}${time ? ` · ${time} 播出` : ""}`;
}

function dayTitle(day: string): string {
  const head = `${day.slice(5)} ${weekdayOf(day)}`;
  return day === today.value ? `${head} · 今天` : head;
}
</script>

<template>
  <div class="pt-cards pt-cards--wide">
    <PtHeadSub>{{ headSub }}</PtHeadSub>
    <TmdbKeyNotice class="pt-cards__full" usage="订阅日历" />

    <PtPanel title="订阅日历" icon="calendar-days" :count="total" padding="none">
      <template #actions>
        <div class="mc-bar" data-testid="mc-bar">
          <el-radio-group v-model="view" size="small" data-testid="mc-view">
            <el-radio-button value="month">月</el-radio-button>
            <el-radio-button value="week">周</el-radio-button>
          </el-radio-group>
          <template v-if="!isMobile">
            <div class="mc-nav">
              <button
                type="button"
                class="mc-icon"
                aria-label="上一段"
                data-testid="mc-prev"
                @click="go(-1)">
                <PtIcon name="chevron-left" :size="16" />
              </button>
              <span class="mc-label" data-testid="mc-label">{{ label }}</span>
              <button
                type="button"
                class="mc-icon"
                aria-label="下一段"
                data-testid="mc-next"
                @click="go(1)">
                <PtIcon name="chevron-right" :size="16" />
              </button>
            </div>
            <el-button size="small" data-testid="mc-today" @click="goToday">今天</el-button>
          </template>
        </div>
      </template>

      <!-- 手机：翻页放在面板里第一行（29 高的卡头放不下 44 的触控区） -->
      <div v-if="isMobile" class="mc-mbar" data-testid="mc-mbar">
        <button
          type="button"
          class="mc-icon"
          aria-label="上一段"
          data-testid="mc-prev"
          @click="go(-1)">
          <PtIcon name="chevron-left" :size="18" />
        </button>
        <span class="mc-label" data-testid="mc-label">{{ label }}</span>
        <button
          type="button"
          class="mc-icon"
          aria-label="下一段"
          data-testid="mc-next"
          @click="go(1)">
          <PtIcon name="chevron-right" :size="18" />
        </button>
        <el-button data-testid="mc-today" @click="goToday">今天</el-button>
      </div>

      <div v-if="data?.errors.length" class="pt-note pt-note--warn mc-note" data-testid="mc-errors">
        <PtIcon name="triangle-alert" :size="14" class="pt-note__icon" />
        <span>这几个订阅没读到 TMDB，没有列出：{{ data.errors.join("；") }}</span>
      </div>
      <PtDataState
        v-if="!data"
        :state="error ? 'error' : 'loading'"
        :title="error ? '没读到日历' : ''"
        :sub="error"
        data-testid="mc-state">
        <template v-if="error" #action>
          <el-button size="small" @click="load">
            <PtIcon name="refresh-cw" :size="14" /><span>重试</span>
          </el-button>
        </template>
      </PtDataState>

      <!-- 手机：按天列出有内容的天 -->
      <div v-else-if="isMobile" class="mc-days" data-testid="mc-days">
        <PtDataState
          v-if="!listedDays.length"
          state="empty"
          title="这段时间没有播出的集"
          sub="订阅了的剧集播出、电影上映会列在这里；暂停的、待确认的订阅不列"
          data-testid="mc-empty" />
        <section v-for="d in listedDays" :key="d" class="mc-day" :data-testid="`mc-day-${d}`">
          <h3 class="mc-day__head" :class="{ 'is-today': d === today }">{{ dayTitle(d) }}</h3>
          <button
            v-for="e in byDay[d]"
            :key="`${e.subscription_id}-${e.episode ?? 0}`"
            type="button"
            class="mc-row"
            data-testid="mc-entry"
            @click="open(e)">
            <EntryPoster :path="e.poster_path" :width="40" :height="60" :radius="4" />
            <span class="mc-row__body">
              <span class="mc-row__title">{{ e.title }}</span>
              <span class="mc-row__sub">{{ entryDetail(e) }}</span>
              <span class="mc-row__foot">
                <PtStatusPill :tone="episodeStatePill(e.state).tone" dot size="sm">
                  {{ episodeStatePill(e.state).label }}
                </PtStatusPill>
                <span v-if="entryTime(e)" class="mc-row__time">{{ entryTime(e) }} 播出</span>
              </span>
            </span>
          </button>
        </section>
      </div>

      <!-- 桌面：月、周 -->
      <div v-else class="mc-grid" :class="`is-${view}`" data-testid="mc-grid">
        <div v-for="w in WEEK_HEAD" :key="w" class="mc-grid__head">{{ w }}</div>
        <div
          v-for="d in days"
          :key="d"
          class="mc-cell"
          :class="{ 'is-today': d === today, 'is-out': view === 'month' && !inMonth(d) }"
          :data-testid="`mc-cell-${d}`">
          <div class="mc-cell__day">
            <span class="mc-cell__num">{{ dayNumber(d) }}</span>
            <span v-if="view === 'week'" class="mc-cell__date">{{ d.slice(5) }}</span>
          </div>
          <template v-if="view === 'month'">
            <button
              v-for="e in (byDay[d] ?? []).slice(0, MONTH_CELL_MAX)"
              :key="`${e.subscription_id}-${e.episode ?? 0}`"
              type="button"
              class="mc-chip"
              :title="entryHint(e)"
              data-testid="mc-entry"
              @click="open(e)">
              <i class="mc-chip__dot" :class="`is-${e.state}`" aria-hidden="true" />
              <span class="mc-chip__name">{{ e.title }}</span>
              <span class="mc-chip__code">{{ entryCode(e) }}</span>
            </button>
            <button
              v-if="(byDay[d]?.length ?? 0) > MONTH_CELL_MAX"
              type="button"
              class="mc-more"
              :data-testid="`mc-more-${d}`"
              @click="showWeek(d)">
              还有 {{ (byDay[d]?.length ?? 0) - MONTH_CELL_MAX }} 项
            </button>
          </template>
          <template v-else>
            <button
              v-for="e in byDay[d] ?? []"
              :key="`${e.subscription_id}-${e.episode ?? 0}`"
              type="button"
              class="mc-card"
              :title="entryHint(e)"
              data-testid="mc-entry"
              @click="open(e)">
              <EntryPoster :path="e.poster_path" :width="36" :height="54" :radius="4" />
              <span class="mc-card__body">
                <span class="mc-card__title">{{ e.title }}</span>
                <span class="mc-card__sub">{{ entryDetail(e) }}</span>
                <span class="mc-card__foot">
                  <PtStatusPill :tone="episodeStatePill(e.state).tone" dot size="sm">
                    {{ episodeStatePill(e.state).label }}
                  </PtStatusPill>
                  <span v-if="entryTime(e)" class="mc-card__time">{{ entryTime(e) }}</span>
                </span>
              </span>
            </button>
          </template>
        </div>
      </div>
      <p v-if="data && !isMobile && !total" class="mc-foot" data-testid="mc-empty">
        这段时间订阅里没有播出的集。暂停的、待确认的订阅不列；剧集要 TMDB 上有播出日期才排得进来。
      </p>
    </PtPanel>
  </div>
</template>

<style scoped>
.mc-bar {
  display: flex;
  gap: 12px;
  align-items: center;
}

.mc-nav {
  display: inline-flex;
  gap: 4px;
  align-items: center;
}

.mc-label {
  min-width: 96px;
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t1);
  text-align: center;
  white-space: nowrap;
}

.mc-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  color: var(--pt-t2);
  cursor: pointer;
  background: none;
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.mc-icon:hover {
  color: var(--pt-t1);
  border-color: var(--pt-border-strong);
}

.mc-note {
  margin: 12px 16px 0;
}

.mc-foot {
  margin: 0;
  padding: 12px 16px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  border-top: 1px solid var(--pt-border);
}

/* 月、周：7 列，表头一行星期 */
.mc-grid {
  display: grid;
  grid-template-columns: repeat(7, minmax(0, 1fr));
}

.mc-grid__head {
  padding: 8px 10px;
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-t3);
  border-bottom: 1px solid var(--pt-border);
}

.mc-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  min-height: 112px;
  padding: 6px 8px 8px;
  border-right: 1px solid var(--pt-border);
  border-bottom: 1px solid var(--pt-border);
}

.mc-grid.is-week .mc-cell {
  min-height: 360px;
  gap: 8px;
}

.mc-cell:nth-child(7n + 7) {
  border-right: 0;
}

.mc-cell.is-out {
  background: var(--pt-canvas);
}

.mc-cell.is-out .mc-cell__num {
  font-weight: 400;
  color: var(--pt-t3);
}

.mc-cell__day {
  display: flex;
  gap: 6px;
  align-items: center;
  height: 22px;
}

.mc-cell__num {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 22px;
  height: 22px;
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-t2);
  border-radius: 11px;
}

.mc-cell.is-today .mc-cell__num {
  color: var(--pt-on-p);
  background: var(--pt-p);
}

.mc-cell__date {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

.mc-chip {
  display: flex;
  gap: 5px;
  align-items: center;
  width: 100%;
  height: 22px;
  padding: 0 6px;
  font: inherit;
  font-size: var(--pt-fz-label);
  color: var(--pt-t1);
  text-align: left;
  cursor: pointer;
  background: var(--pt-hover);
  border: 0;
  border-radius: 4px;
}

.mc-chip:hover {
  background: var(--pt-p-soft);
}

.mc-chip__dot {
  flex: 0 0 auto;
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.mc-chip__dot.is-library {
  background: var(--pt-ok);
}

.mc-chip__dot.is-downloading {
  background: var(--pt-info);
}

.mc-chip__dot.is-waiting_version {
  background: var(--pt-p);
}

.mc-chip__dot.is-missing {
  background: var(--pt-warn);
}

.mc-chip__dot.is-upcoming,
.mc-chip__dot.is-skipped {
  background: var(--pt-t4);
}

.mc-chip__name {
  min-width: 0;
  overflow: hidden;
  font-weight: 500;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mc-chip__code {
  flex: 0 0 auto;
  margin-left: auto;
  color: var(--pt-t3);
}

.mc-more {
  align-self: flex-start;
  padding: 0 6px;
  font: inherit;
  font-size: var(--pt-fz-label);
  color: var(--pt-p);
  cursor: pointer;
  background: none;
  border: 0;
}

.mc-more:hover {
  text-decoration: underline;
}

.mc-card {
  display: flex;
  gap: 8px;
  width: 100%;
  padding: 6px;
  font: inherit;
  text-align: left;
  cursor: pointer;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-md);
}

.mc-card:hover {
  border-color: var(--pt-border-strong);
}

.mc-card__body,
.mc-row__body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.mc-card__title,
.mc-row__title {
  overflow: hidden;
  font-size: var(--pt-fz-body);
  font-weight: 600;
  color: var(--pt-t1);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mc-card__sub,
.mc-row__sub {
  overflow: hidden;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mc-card__foot,
.mc-row__foot {
  display: flex;
  gap: 6px;
  align-items: center;
  margin-top: 2px;
}

.mc-card__time,
.mc-row__time {
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
}

/* 手机：按天的行卡 */
.mc-days {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 12px 14px 14px;
}

.mc-day {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.mc-day__head {
  margin: 0;
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  color: var(--pt-t2);
}

.mc-day__head.is-today {
  color: var(--pt-p);
}

.mc-row {
  display: flex;
  gap: 10px;
  align-items: center;
  width: 100%;
  min-height: var(--pt-m-touch);
  padding: 8px;
  font: inherit;
  text-align: left;
  cursor: pointer;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

/* 手机的翻页一行：箭头与「今天」都是 44 的触控高度（§9），箭头的图形对齐到内容区左边 14 */
.mc-mbar {
  display: flex;
  gap: 4px;
  align-items: center;
  padding: 4px 14px 0 2px;
}

.mc-mbar .mc-label {
  flex: 1 1 auto;
  min-width: 0;
}

.mc-mbar .el-button {
  height: var(--pt-m-touch);
  margin-left: 8px;
}

@media (max-width: 768px) {
  .mc-icon {
    width: var(--pt-m-touch);
    height: var(--pt-m-touch);
  }

  /* 卡头只有 29 高，月、周切换本体 24：命中区用 ::before 的负 inset 撑到 44 */
  .mc-bar :deep(.el-radio-button__inner) {
    position: relative;
  }

  .mc-bar :deep(.el-radio-button__inner)::before {
    position: absolute;
    inset: -10px 0;
    content: "";
  }
}
</style>
