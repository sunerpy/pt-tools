<script setup lang="ts">
import { computed, ref, watch } from "vue";

/**
 * 刷流收益的柱状图：一根柱子是一段（一天、一周、一个月、一个季度或一年）的上传量。
 *
 * 柱子的画法沿用 PtBars（设计文档 §4 的彩虹柱：单一色相、柱内同色渐变、从 0 起算），
 * 多了这一页要的两样：柱子下面写是哪一段，柱子上面一行读数写选中那一段的上传、下载、加入与删除。
 * 读数默认是当前这一段；鼠标移到柱子上、或者聚焦图表后按左右方向键，换成那一段。
 * 读数是给所有人的（不只是鼠标），所以不做浮层提示；每一段的数字在「明细」表里也都有。
 *
 * 读数固定两行、每行不折行（放不下省略）：读数一换行，下面的图就跟着上下挪，鼠标底下的柱子换了，
 * 浏览器发 pointerleave、读数又换回来 —— 在真实 Chrome 里量到过这样来回跳。
 */
export interface TrendPoint {
  /** 柱子下面的短标签 */
  short: string;
  /** 读数里的完整写法 */
  long: string;
  value: number;
  /** 读数里的主数字（已格式化） */
  valueText: string;
  /** 读数里的次要信息 */
  detail: string;
}

const props = withDefaults(
  defineProps<{
    points: TrendPoint[];
    hue?: string;
    height?: number;
    /** 图表的无障碍名称 */
    label: string;
  }>(),
  { hue: "var(--pt-kpi-2)", height: 96 },
);

const sel = ref(Math.max(0, props.points.length - 1));
/* 换了周期（段数变了）回到当前这一段 */
watch(
  () => props.points.length,
  (n) => {
    sel.value = Math.max(0, n - 1);
  },
);

const max = computed(() => Math.max(0, ...props.points.map((p) => p.value)));

function barHeight(v: number): string {
  // 0 只画 2px 的底（「这一段没有收益」），正值按占最大值的比例、最矮 3px（与 PtBars 的 zero 基线同一套）
  if (!(v > 0) || max.value <= 0) return "2px";
  return `${Math.max(3, Math.round((v / max.value) * props.height))}px`;
}

/* 标签放不下就隔几根写一个：最多写 8 个，最后一根（当前这一段）总是写 */
const labelEvery = computed(() => Math.max(1, Math.ceil(props.points.length / 8)));

function showLabel(i: number): boolean {
  const n = props.points.length;
  if (i === n - 1) return true;
  // 离最后一根太近的不写，免得两个标签挤在一起
  if (n - 1 - i < labelEvery.value) return false;
  return i % labelEvery.value === 0;
}

const current = computed(() => props.points[sel.value]);

function onKey(e: KeyboardEvent) {
  const n = props.points.length;
  if (!n) return;
  if (e.key === "ArrowLeft") sel.value = Math.max(0, sel.value - 1);
  else if (e.key === "ArrowRight") sel.value = Math.min(n - 1, sel.value + 1);
  else if (e.key === "Home") sel.value = 0;
  else if (e.key === "End") sel.value = n - 1;
  else return;
  e.preventDefault();
}

function resetSel() {
  sel.value = Math.max(0, props.points.length - 1);
}
</script>

<template>
  <div class="btc">
    <div v-if="current" class="btc__readout" aria-live="polite" data-testid="brush-trend-readout">
      <p class="btc__line">
        <strong class="btc__value">{{ current.valueText }}</strong>
        <span class="btc__when">{{ current.long }}</span>
      </p>
      <p class="btc__line btc__detail">{{ current.detail }}</p>
    </div>
    <div
      class="btc__plot"
      role="group"
      tabindex="0"
      :aria-label="`${label}。按左右方向键看每一段`"
      :style="{ height: `${height}px`, '--btc-hue': hue }"
      @keydown="onKey"
      @pointerleave="resetSel">
      <div
        v-for="(p, i) in points"
        :key="i"
        class="btc__col"
        :class="{ 'is-on': i === sel }"
        :title="`${p.long}：${p.valueText}`"
        @pointerenter="sel = i"
        @click="sel = i">
        <span class="btc__bar" :style="{ height: barHeight(p.value) }" />
      </div>
    </div>
    <div class="btc__axis" aria-hidden="true">
      <span
        v-for="(p, i) in points"
        :key="i"
        class="btc__tick"
        :class="{ 'is-on': i === sel, 'is-first': i === 0, 'is-last': i === points.length - 1 }">
        <template v-if="showLabel(i)">{{ p.short }}</template>
      </span>
    </div>
  </div>
</template>

<style scoped>
.btc {
  display: flex;
  flex-direction: column;
  gap: var(--pt-space-2);
  min-width: 0;
}

/* 读数：数字在前、是哪一段在后（读者已经知道是上传，要的是数）；两行各自不折行，高度不随内容变 */
.btc__readout {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.btc__line {
  display: flex;
  gap: 10px;
  align-items: baseline;
  min-width: 0;
  margin: 0;
  overflow: hidden;
  white-space: nowrap;
}

.btc__value {
  flex: 0 0 auto;
  font-size: var(--pt-fz-h2);
  font-weight: 650;
  line-height: 1.3;
  color: var(--pt-t1);
  font-variant-numeric: tabular-nums;
}

.btc__when {
  min-width: 0;
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  text-overflow: ellipsis;
}

.btc__detail {
  display: block;
  font-size: var(--pt-fz-label);
  line-height: 1.4;
  color: var(--pt-t3);
  text-overflow: ellipsis;
}

.btc__plot {
  display: flex;
  gap: 2px;
  align-items: flex-end;
  padding-bottom: 1px;
  border-bottom: 1px solid var(--pt-border);
  outline: none;
  border-radius: var(--pt-r-sm);
}

.btc__plot:focus-visible {
  box-shadow: 0 0 0 2px var(--pt-p-soft);
}

/* 一段一列：命中范围是整列（比柱子大），柱子贴着底 */
.btc__col {
  display: flex;
  flex: 1 1 0;
  align-items: flex-end;
  justify-content: center;
  min-width: 0;
  height: 100%;
  cursor: pointer;
}

/*
 * 没选中的柱子用带透明度的颜色（color-mix）画浅，不用 opacity：项目的门禁只许 opacity 落在禁用态、箭头、头像与图标上
 * （styles/paletteContrast.test.ts），半透明一律写成颜色。
 */
.btc__bar {
  width: 100%;
  max-width: 28px;
  border-radius: 3px 3px 0 0;
  background: linear-gradient(
    to bottom,
    color-mix(in srgb, var(--btc-hue) 55%, transparent),
    color-mix(in srgb, var(--btc-hue) 25%, transparent)
  );
}

.btc__col.is-on .btc__bar {
  background: linear-gradient(
    to bottom,
    var(--btc-hue),
    color-mix(in srgb, var(--btc-hue) 45%, transparent)
  );
}

.btc__axis {
  display: flex;
  gap: 2px;
}

/*
 * 标签比一列宽（30 天时一列才十来 px）：居中、两边溢出；第一个与最后一个往图里溢出，
 * 不伸出图的左右边（手机上最后一个「10/29」被面板裁掉过）。用 flex 对齐而不是 text-align：
 * 一行字比盒子宽时 text-align 不管用，总是往右溢出。
 */
.btc__tick {
  display: flex;
  flex: 1 1 0;
  justify-content: center;
  min-width: 0;
  font-size: var(--pt-fz-label);
  color: var(--pt-t3);
  white-space: nowrap;
}

.btc__tick.is-first {
  justify-content: flex-start;
}

.btc__tick.is-last {
  justify-content: flex-end;
}

.btc__tick.is-on {
  color: var(--pt-t1);
  font-weight: 600;
}
</style>
