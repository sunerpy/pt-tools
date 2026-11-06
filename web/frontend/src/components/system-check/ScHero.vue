<script setup lang="ts">
/*
 * 系统体检的总览卡（首页 NAS 概览 H1，预览 system-check.html 的 hero）：
 * 左边环形图（失败、提醒、正常、跳过按项数分段，中间是要处理的项数），右边标题与上次体检的一行、
 * 四个计数块（点了只看这个状态的项，再点一次取消）、各组的状态条（点了滚到那一组）。
 * 体检进行中环形图换成转动的弧、写已用几秒。手机上环形图 92 与标题并排，计数块 2×2，状态条横向滚动，
 * 「立即体检」是满宽按钮。
 */
import type { HealthGroup, HealthReport, HealthStatus } from "@/api";
import PtIcon from "@/components/PtIcon";
import PtStatusPill from "@/components/ui/PtStatusPill.vue";
import { useTweenedNumber } from "@/composables/useTweenedNumber";
import { checkGroups, checkMeta, healthStatus, ringSegments } from "@/utils/systemCheck";
import { computed, onMounted, ref } from "vue";

const props = defineProps<{
  report: HealthReport;
  running: boolean;
  /** 体检进行中已用的秒数 */
  elapsed: number;
  /** 选中的计数块（空串是没选） */
  filter: HealthStatus | "";
  mobile: boolean;
}>();

const emit = defineEmits<{ filter: [status: HealthStatus]; jump: [group: HealthGroup]; run: [] }>();

const TILES: readonly HealthStatus[] = ["ok", "warn", "fail", "skip"];

const segments = computed(() => ringSegments(props.report));
/* 第一次显示时各段从 0 画到终值；之后换了结果，从旧的长度过渡到新的（样式里的 transition） */
const drawn = ref(false);
onMounted(() => {
  if (typeof requestAnimationFrame === "function")
    requestAnimationFrame(() => requestAnimationFrame(() => (drawn.value = true)));
  else drawn.value = true;
});

const bad = computed(() => props.report.fail + props.report.warn);
const badShown = useTweenedNumber(() => bad.value);
const counts = {
  ok: useTweenedNumber(() => props.report.ok),
  warn: useTweenedNumber(() => props.report.warn),
  fail: useTweenedNumber(() => props.report.fail),
  skip: useTweenedNumber(() => props.report.skip),
} satisfies Record<HealthStatus, unknown>;

const groups = computed(() => checkGroups(props.report));
const meta = computed(() =>
  props.running ? `正在体检… 已用 ${props.elapsed} 秒` : checkMeta(props.report),
);
</script>

<template>
  <section
    class="sc-card sc-rv sc-hero"
    :class="{ 'is-mobile': mobile, 'is-running': running }"
    style="--sc-i: 0"
    data-testid="sc-hero">
    <div class="sc-ring" :aria-label="`${bad} 项要处理`" role="img">
      <svg class="sc-ring__svg" viewBox="0 0 148 148" aria-hidden="true">
        <circle class="sc-ring__track" cx="74" cy="74" r="56" />
        <circle
          v-for="s in segments"
          :key="s.status"
          class="sc-ring__seg"
          :class="`is-${s.status}`"
          cx="74"
          cy="74"
          r="56"
          pathLength="100"
          :style="{
            strokeDasharray: `${drawn ? s.length : 0} 100`,
            strokeDashoffset: -s.start,
          }" />
      </svg>
      <svg v-if="running" class="sc-ring__spin" viewBox="0 0 148 148" aria-hidden="true">
        <circle cx="74" cy="74" r="56" pathLength="100" />
      </svg>
      <div class="sc-ring__center">
        <template v-if="running">
          <PtIcon name="activity" :size="mobile ? 22 : 30" class="sc-ring__run" />
          <span class="sc-ring__label">体检中</span>
        </template>
        <template v-else-if="bad">
          <span
            class="sc-ring__num"
            :class="report.fail ? 'is-dang' : 'is-warn'"
            data-testid="sc-ring-num"
            >{{ Math.round(badShown) }}</span
          >
          <span class="sc-ring__label">项要处理</span>
        </template>
        <template v-else>
          <PtIcon name="circle-check" :size="mobile ? 24 : 34" class="sc-ring__ok" />
          <span class="sc-ring__label">都正常</span>
        </template>
      </div>
    </div>

    <div class="sc-hero__main">
      <div class="sc-hero__head">
        <div class="sc-hero__title">
          <span>体检结果</span>
          <PtStatusPill v-if="report.fail" tone="dang" dot>{{ report.fail }} 项失败</PtStatusPill>
          <PtStatusPill v-else-if="report.warn" tone="warn" dot
            >{{ report.warn }} 项提醒</PtStatusPill
          >
          <PtStatusPill v-else tone="ok" dot>都正常</PtStatusPill>
        </div>
        <div class="sc-hero__meta" data-testid="sc-meta">{{ meta }}</div>
      </div>

      <div class="sc-tiles">
        <button
          v-for="s in TILES"
          :key="s"
          type="button"
          class="sc-tile"
          :class="[`is-${healthStatus(s).tone}`, { 'is-sel': filter === s }]"
          :aria-pressed="filter === s"
          :aria-label="`${healthStatus(s).label} ${report[s]}`"
          :data-testid="`sc-tile-${s}`"
          @click="emit('filter', s)">
          <span class="sc-tile__label">
            <PtIcon :name="healthStatus(s).icon" :size="14" class="sc-tile__icon" />{{
              healthStatus(s).label
            }}
          </span>
          <span class="sc-tile__num" aria-hidden="true">{{ Math.round(counts[s].value) }}</span>
        </button>
      </div>

      <div class="sc-strip" data-testid="sc-strip">
        <button
          v-for="g in groups"
          :key="g.key"
          type="button"
          class="sc-chip"
          :aria-label="`跳到「${g.label}」：${healthStatus(g.worst).label}`"
          :data-testid="`sc-chip-${g.key}`"
          @click="emit('jump', g.key)">
          <PtIcon :name="g.icon" :size="14" class="sc-chip__icon" />
          <span>{{ g.label }}</span>
          <i class="sc-chip__dot" :class="`is-${healthStatus(g.worst).tone}`" aria-hidden="true" />
        </button>
      </div>
    </div>

    <div v-if="mobile" class="sc-mbar">
      <el-button type="primary" :loading="running" data-testid="sc-run" @click="emit('run')">
        {{ running ? "体检中…" : "立即体检" }}
      </el-button>
    </div>
  </section>
</template>

<style scoped>
.sc-hero {
  display: grid;
  grid-template-columns: 148px minmax(0, 1fr);
  gap: 24px;
  align-items: center;
  padding: 20px 24px;
}

.sc-ring {
  position: relative;
  width: 148px;
  height: 148px;
}

.sc-ring__svg,
.sc-ring__spin {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
  transform: rotate(-90deg);
}

.sc-ring circle {
  fill: none;
  stroke-width: 12;
}

.sc-ring__track {
  stroke: var(--pt-hover);
}

.sc-ring__seg {
  transition:
    stroke-dasharray var(--pt-dur-slow) cubic-bezier(0.2, 0, 0, 1),
    stroke-dashoffset var(--pt-dur-slow) cubic-bezier(0.2, 0, 0, 1),
    stroke-opacity var(--pt-transition-normal);
}

.sc-ring__seg.is-fail {
  stroke: var(--pt-dang);
}

.sc-ring__seg.is-warn {
  stroke: var(--pt-warn);
}

.sc-ring__seg.is-ok {
  stroke: var(--pt-ok);
}

.sc-ring__seg.is-skip {
  stroke: var(--pt-t4);
}

.is-running .sc-ring__seg {
  stroke-opacity: 0.25;
}

/* 体检进行中的扫描：一段主色的弧转圈，表示还在查 */
.sc-ring__spin {
  animation: sc-ring-spin calc(var(--pt-dur-slow) * 3) linear infinite;
}

.sc-ring__spin circle {
  stroke: var(--pt-p);
  stroke-dasharray: 18 82;
  stroke-linecap: round;
}

@keyframes sc-ring-spin {
  from {
    transform: rotate(-90deg);
  }

  to {
    transform: rotate(270deg);
  }
}

.sc-ring__center {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
  align-items: center;
  justify-content: center;
  text-align: center;
}

.sc-ring__num {
  font-size: 34px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 1;
}

.sc-ring__num.is-dang {
  color: var(--pt-dang);
}

.sc-ring__num.is-warn {
  color: var(--pt-warn);
}

.sc-ring__run {
  color: var(--pt-p);
}

.sc-ring__ok {
  color: var(--pt-ok);
}

.sc-ring__label {
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
}

.sc-hero__main {
  min-width: 0;
}

.sc-hero__head {
  min-width: 0;
}

.sc-hero__title {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
  font-size: var(--pt-fz-h2);
  font-weight: 700;
  color: var(--pt-t1);
}

.sc-hero__meta {
  margin-top: 4px;
  font-size: var(--pt-fz-sm);
  font-variant-numeric: tabular-nums;
  color: var(--pt-t2);
}

.sc-tiles {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-top: 16px;
}

.sc-tile {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  padding: 10px 14px 12px;
  font: inherit;
  text-align: left;
  cursor: pointer;
  border: 1px solid transparent;
  border-radius: var(--pt-r-md);
  transition:
    border-color var(--pt-transition-fast),
    transform var(--pt-transition-fast);
}

@media (hover: hover) {
  .sc-tile:hover {
    border-color: color-mix(in srgb, currentcolor 35%, transparent);
    transform: translateY(-1px);
  }
}

.sc-tile:focus-visible,
.sc-chip:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 2px;
}

.sc-tile.is-sel {
  border-color: currentcolor;
}

.sc-tile.is-ok {
  color: var(--pt-ok);
  background: var(--pt-ok-soft);
}

.sc-tile.is-warn {
  color: var(--pt-warn);
  background: var(--pt-warn-soft);
}

.sc-tile.is-dang {
  color: var(--pt-dang);
  background: var(--pt-dang-soft);
}

.sc-tile.is-neutral {
  color: var(--pt-t2);
  background: var(--pt-hover);
}

.sc-tile__label {
  display: flex;
  gap: 6px;
  align-items: center;
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-t2);
}

.sc-tile__icon {
  flex: 0 0 auto;
}

.sc-tile__num {
  font-size: 26px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 1.1;
}

.sc-strip {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 14px;
}

.sc-chip {
  display: inline-flex;
  flex: 0 0 auto;
  gap: 6px;
  align-items: center;
  height: 28px;
  padding: 0 10px 0 9px;
  font: inherit;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t1);
  white-space: nowrap;
  cursor: pointer;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: 14px;
  transition: background-color var(--pt-transition-fast);
}

@media (hover: hover) {
  .sc-chip:hover {
    background: var(--pt-hover);
  }
}

.sc-chip__icon {
  color: var(--pt-t3);
}

.sc-chip__dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.sc-chip__dot.is-ok {
  background: var(--pt-ok);
}

.sc-chip__dot.is-warn {
  background: var(--pt-warn);
}

.sc-chip__dot.is-dang {
  background: var(--pt-dang);
}

.sc-chip__dot.is-neutral {
  background: var(--pt-t4);
}

/* 手机：环形图 92 与标题并排，计数块 2×2，状态条横向滚动（右侧渐隐），「立即体检」满宽 */
.sc-hero.is-mobile {
  grid-template-columns: 92px minmax(0, 1fr);
  gap: 14px;
  padding: 14px;
}

.is-mobile .sc-ring {
  width: 92px;
  height: 92px;
}

.is-mobile .sc-ring__num {
  font-size: 24px;
}

.is-mobile .sc-ring__label {
  font-size: var(--pt-fz-label);
}

.is-mobile .sc-hero__title {
  font-size: var(--pt-fz-body-lg);
}

.is-mobile .sc-hero__main {
  display: contents;
}

.is-mobile .sc-tiles {
  grid-column: 1 / -1;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-top: 0;
}

.is-mobile .sc-tile {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
}

.is-mobile .sc-tile__num {
  font-size: 20px;
}

.is-mobile .sc-strip {
  flex-wrap: nowrap;
  grid-column: 1 / -1;
  margin: 0 -14px;
  padding: 0 14px 2px;
  overflow-x: auto;
  scrollbar-width: none;
  mask-image: linear-gradient(90deg, #000 calc(100% - 40px), transparent);
}

.sc-mbar {
  grid-column: 1 / -1;
  display: flex;
}

.sc-mbar .el-button {
  flex: 1 1 auto;
  height: 40px;
}

@media (prefers-reduced-motion: reduce) {
  .sc-ring__seg,
  .sc-tile,
  .sc-chip {
    transition: none;
  }

  .sc-ring__spin {
    animation: none;
  }

  .sc-tile:hover {
    transform: none;
  }
}
</style>
