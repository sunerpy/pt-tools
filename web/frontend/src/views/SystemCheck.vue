<script setup lang="ts">
/*
 * 系统体检（媒体中心 M8，首页 NAS 概览 H1 改成卡片）：一次检查 TMDB、站点、下载器、媒体服务器、路径映射、媒体库、
 * 磁盘、时间、备份与密钥，每项写现在的样子与怎么修。打开页面时显示上一次的结果，「立即体检」现查一遍
 * （最多 60 秒，同时只跑一次）。不改任何设置（媒体库同「检查」，在库目录里试建一个文件再删掉）；站点只看最近一次
 * 登录探测，不发新的请求。
 *
 * 一列：总览卡 →「需要处理」（失败在前、提醒在后，带「去处理」；这次新出现的加标签）→「全部检查项」分组卡（瀑布流）。
 * 本页只管取数与状态，卡片在 components/system-check/。动效的规矩见 DESIGN.md 的 Motion 一节。
 */
import "@/styles/system-check.css";

import { type HealthGroup, type HealthReport, type HealthStatus, systemCheckApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import ScGroupCard from "@/components/system-check/ScGroupCard.vue";
import ScHero from "@/components/system-check/ScHero.vue";
import ScProblem from "@/components/system-check/ScProblem.vue";
import PtDataState from "@/components/ui/PtDataState.vue";
import PtHeadSub from "@/components/ui/PtHeadSub.vue";
import { useIsMobile } from "@/composables/useIsMobile";
import {
  changedKeys,
  checkHeadline,
  filterGroups,
  HEALTH_GROUPS,
  healthStatus,
  problemItems,
} from "@/utils/systemCheck";
import { ElMessage } from "element-plus";
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from "vue";

const isMobile = useIsMobile();

const report = ref<HealthReport | null>(null);
const loading = ref(true);
const loadError = ref("");
const running = ref(false);
const elapsed = ref(0);
const onlyProblems = ref(false);
const statusFilter = ref<HealthStatus | "">("");
/** 这次体检新出现的问题（与体检前显示的那份比） */
const fresh = ref<Set<string>>(new Set());
/** 每来一份新结果加一：问题卡与分组卡重新依次出现 */
const revealKey = ref(0);

let timer: ReturnType<typeof setInterval> | null = null;
function stopTimer() {
  if (timer) clearInterval(timer);
  timer = null;
}
onBeforeUnmount(stopTimer);

async function load() {
  loading.value = true;
  loadError.value = "";
  try {
    report.value = (await systemCheckApi.last()).report;
  } catch (e) {
    loadError.value = (e as Error)?.message || "读取体检结果失败";
  } finally {
    loading.value = false;
  }
}

async function run() {
  if (running.value) return;
  running.value = true;
  elapsed.value = 0;
  const started = Date.now();
  timer = setInterval(() => (elapsed.value = Math.floor((Date.now() - started) / 1000)), 1000);
  const before = report.value;
  try {
    const r = (await systemCheckApi.run()).report;
    fresh.value = changedKeys(before, r);
    report.value = r;
    loadError.value = "";
    revealKey.value++;
    if (r.fail) ElMessage.warning(`体检完了：${r.fail} 项失败`);
    else if (r.warn) ElMessage.info(`体检完了：${r.warn} 项提醒`);
    else ElMessage.success("体检完了：都正常");
  } catch (e) {
    ElMessage.error((e as Error)?.message || "体检失败");
  } finally {
    stopTimer();
    running.value = false;
  }
}

onMounted(() => void load());

const sub = computed(() => {
  if (loading.value) return "加载中…";
  if (running.value) return `正在体检… 已用 ${elapsed.value} 秒`;
  return checkHeadline(report.value);
});

const groupLabel = (g: HealthGroup) => HEALTH_GROUPS.find((x) => x.key === g)?.label ?? g;

const problems = computed(() => problemItems(report.value));
const shown = computed(() =>
  filterGroups(report.value, { onlyProblems: onlyProblems.value, status: statusFilter.value }),
);
const noneText = computed(() =>
  statusFilter.value ? `没有${healthStatus(statusFilter.value).label}的项` : "没有有问题的项",
);

function toggleStatus(s: HealthStatus) {
  statusFilter.value = statusFilter.value === s ? "" : s;
}

function reducedMotion(): boolean {
  return typeof window.matchMedia === "function"
    ? window.matchMedia("(prefers-reduced-motion: reduce)").matches
    : false;
}

/** 总览卡上点了一组的状态条：滚到那一组；被筛选藏起来了就先清掉筛选 */
async function jump(g: HealthGroup) {
  if (!shown.value.some((x) => x.group.key === g)) {
    onlyProblems.value = false;
    statusFilter.value = "";
    await nextTick();
  }
  const el = document.getElementById(`sc-g-${g}`);
  el?.scrollIntoView({ behavior: reducedMotion() ? "auto" : "smooth", block: "start" });
}
</script>

<template>
  <div
    class="pt-cards pt-cards--wide sc"
    :class="{ 'is-mobile': isMobile, 'is-running': running }"
    data-testid="system-check">
    <PtHeadSub>{{ sub }}</PtHeadSub>
    <Teleport to="#pt-head-acts" :disabled="isMobile">
      <el-button
        v-if="!isMobile"
        type="primary"
        :loading="running"
        data-testid="sc-run"
        @click="run">
        <PtIcon v-if="!running" name="activity" :size="15" /><span>{{
          running ? "体检中…" : "立即体检"
        }}</span>
      </el-button>
    </Teleport>

    <div v-if="loading" class="sc-skel" data-testid="sc-loading" aria-busy="true">
      <div class="sc-card sc-skel__hero"><i class="sc-skel__ring" /><i /><i /><i /></div>
      <div class="sc-skel__row">
        <div v-for="n in 3" :key="n" class="sc-card sc-skel__card"><i /><i /><i /></div>
      </div>
    </div>

    <div v-else-if="loadError && !report" class="sc-card" data-testid="sc-load-error">
      <PtDataState state="error" :sub="loadError">
        <template #action>
          <el-button @click="load">重新读取</el-button>
        </template>
      </PtDataState>
    </div>

    <div v-else-if="!report" class="sc-card sc-rv" data-testid="sc-empty">
      <PtDataState
        state="empty"
        title="还没体检过"
        sub="点「立即体检」查一遍 TMDB、站点、下载器、媒体库、磁盘与备份，不改任何设置">
        <template #action>
          <el-button type="primary" :loading="running" data-testid="sc-empty-run" @click="run">
            {{ running ? "体检中…" : "立即体检" }}
          </el-button>
        </template>
      </PtDataState>
    </div>

    <template v-else>
      <ScHero
        :report="report"
        :running="running"
        :elapsed="elapsed"
        :filter="statusFilter"
        :mobile="isMobile"
        @filter="toggleStatus"
        @jump="jump"
        @run="run" />

      <section class="sc-sec" data-testid="sc-problems-sec">
        <div class="sc-sec__head">
          <h2 class="sc-sec__title">需要处理</h2>
          <span v-if="problems.length" class="sc-sec__count">{{ problems.length }} 项</span>
          <span v-if="running" class="sc-sec__stale" data-testid="sc-stale">上一次的结果</span>
        </div>
        <!-- 两个分支的 key 不能撞：v-if 自带 key 时 Vue 不再给分支编号，同 key 同标签会把旧元素就地改成新的，静态的 class 与属性留着旧的 -->
        <div
          v-if="problems.length"
          :key="`probs-${revealKey}`"
          class="sc-probs"
          data-testid="sc-problems">
          <ScProblem
            v-for="(it, i) in problems"
            :key="it.key"
            :item="it"
            :group-label="groupLabel(it.group)"
            :is-new="fresh.has(it.key)"
            :index="i + 1" />
        </div>
        <div v-else key="allok" class="sc-card sc-rv sc-lift sc-allok" data-testid="sc-allok">
          <PtIcon name="circle-check" :size="18" class="sc-allok__icon" />
          <span>没有要处理的项</span>
        </div>
      </section>

      <section class="sc-sec" data-testid="sc-groups-sec">
        <div class="sc-sec__head">
          <h2 class="sc-sec__title">全部检查项</h2>
          <span class="sc-sec__count">{{ report.items.length }} 项</span>
          <span v-if="running" class="sc-sec__stale">上一次的结果</span>
          <button
            v-if="statusFilter"
            type="button"
            class="sc-sec__chip"
            data-testid="sc-status-chip"
            @click="statusFilter = ''">
            只看{{ healthStatus(statusFilter).label }}<PtIcon name="x" :size="12" />
          </button>
          <span class="sc-sec__gap" />
          <label class="sc-sec__switch">
            <span>只看有问题的</span>
            <el-switch v-model="onlyProblems" size="small" data-testid="sc-filter" />
          </label>
        </div>
        <div
          v-if="shown.length"
          :key="`groups-${revealKey}`"
          class="sc-groups"
          data-testid="sc-groups">
          <ScGroupCard
            v-for="(g, i) in shown"
            :key="g.group.key"
            :group="g.group"
            :items="g.items"
            :index="problems.length + i + 1" />
        </div>
        <p v-else key="none" class="sc-none" data-testid="sc-none">{{ noneText }}</p>
      </section>
    </template>
  </div>
</template>

<style scoped>
.sc {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.sc.is-mobile {
  gap: 20px;
}

.sc-sec {
  min-width: 0;
}

.sc-sec__head {
  display: flex;
  gap: 8px;
  align-items: center;
  min-height: 24px;
  margin-bottom: 12px;
}

.sc-sec__title {
  margin: 0;
  font-size: var(--pt-fz-h2);
  font-weight: 700;
  line-height: 21px;
  color: var(--pt-t1);
  white-space: nowrap;
}

.sc-sec__count {
  flex: 0 0 auto;
  height: 18px;
  padding: 0 6px;
  font-size: var(--pt-fz-label);
  font-variant-numeric: tabular-nums;
  line-height: 18px;
  color: var(--pt-t3);
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

/* 体检进行中：下面还是上一次的结果，结果到了再换（不用 opacity 压暗文字） */
.sc-sec__stale {
  flex: 0 0 auto;
  height: 18px;
  padding: 0 6px;
  font-size: var(--pt-fz-label);
  line-height: 18px;
  color: var(--pt-warn);
  white-space: nowrap;
  background: var(--pt-warn-soft);
  border-radius: var(--pt-r-sm);
}

.sc-sec__chip {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  height: 22px;
  padding: 0 8px;
  font: inherit;
  font-size: var(--pt-fz-label);
  font-weight: 600;
  color: var(--pt-p);
  cursor: pointer;
  background: var(--pt-p-soft);
  border: 0;
  border-radius: var(--pt-r-sm);
}

.sc-sec__chip:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.sc-sec__gap {
  flex: 1 1 auto;
}

.sc-sec__switch {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 8px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  cursor: pointer;
}

.sc-probs {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 340px), 1fr));
  gap: 12px;
}

.sc-groups {
  column-gap: 12px;
  column-width: 340px;
}

.sc-groups > :deep(.sc-grp) {
  margin-bottom: 12px;
  break-inside: avoid;
}

.sc.is-mobile .sc-groups {
  column-width: auto;
  column-count: 1;
}

.sc-allok {
  display: flex;
  gap: 10px;
  align-items: center;
  padding: 14px 16px;
  font-size: var(--pt-fz-body);
  color: var(--pt-t2);
}

.sc-allok__icon {
  flex: 0 0 auto;
  color: var(--pt-ok);
}

.sc-none {
  margin: 0;
  padding: 16px;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t3);
  text-align: center;
  background: var(--pt-surface);
  border: 1px dashed var(--pt-border);
  border-radius: var(--pt-r-lg);
}

/* 加载中：静止的骨架，不加闪动 */
.sc-skel {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.sc-skel i {
  display: block;
  height: 12px;
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}

.sc-skel__hero {
  display: grid;
  grid-template-columns: 120px minmax(0, 1fr);
  grid-template-rows: repeat(3, auto);
  gap: 14px 24px;
  align-items: center;
  padding: 24px;
}

.sc-skel__hero .sc-skel__ring {
  grid-row: 1 / span 3;
  width: 120px;
  height: 120px;
  border-radius: 50%;
}

.sc-skel__hero i:nth-child(2) {
  width: 40%;
  height: 18px;
}

.sc-skel__hero i:nth-child(3) {
  width: 70%;
}

.sc-skel__hero i:nth-child(4) {
  height: 48px;
}

.sc-skel__row {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 340px), 1fr));
  gap: 12px;
}

.sc-skel__card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 16px;
}

.sc-skel__card i:first-child {
  width: 50%;
}

.sc-skel__card i:last-child {
  width: 80%;
}

.sc.is-mobile .sc-skel__hero {
  grid-template-columns: 92px minmax(0, 1fr);
  padding: 16px;
}

.sc.is-mobile .sc-skel__hero .sc-skel__ring {
  width: 92px;
  height: 92px;
}
</style>
