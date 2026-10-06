<script setup lang="ts">
import { computed } from "vue";

/**
 * 彩虹柱指标图 —— 设计文档 §4 的 `k.bars`，是本产品指标图形的唯一形态。
 *
 * 单色平滑曲线（`k.trend`）被明确否决过两次，只在设计稿的 01 Foundations 上
 * 留作对照；任何新的指标图形默认按卡片轮转色相，不要拿单色曲线替换这里。
 *
 * 归一化与设计稿逐条对齐：取序列最后 n 个点，映射到 0.34–1.0 的高度，所以
 * 最低点也有可见高度，不会出现贴地的空柱；柱内自上而下走一道同色渐变，
 * 末柱恒为满不透明度，读作「当前值」。
 */
const props = withDefaults(
  defineProps<{
    values: number[];
    /** 柱子色相。KPI 条按卡片顺序传 --pt-kpi-1..4，其余场合默认强调色 */
    hue?: string;
    /** 取序列最后几个点 */
    count?: number;
    barWidth?: number;
    gap?: number;
    radius?: number;
    /** 首柱不透明度下限 */
    op0?: number;
    /** 柱底相对柱顶的不透明度系数 */
    foot?: number;
    height?: number;
    /** 柱子最小高度，避免最低点看不见 */
    min?: number;
    /**
     * 归一化方式。range（默认，指标图）：最低点映射到 34%；zero：从 0 起算，0 只画一道 2px 的底，
     * 用于每日增量这类「0 就是没有」的序列，否则缺快照、没上传的日子也会画成三分之一高的柱子。
     */
    baseline?: "range" | "zero";
  }>(),
  {
    hue: "var(--pt-p)",
    count: 9,
    barWidth: 5,
    gap: 3,
    radius: 2,
    op0: 0.34,
    foot: 0.32,
    height: 22,
    min: 4,
    baseline: "range",
  },
);

const bars = computed(() => {
  const src = props.values.slice(-props.count).filter((v) => Number.isFinite(v));
  if (!src.length) return [];
  const lo = Math.min(...src);
  const hi = Math.max(...src);
  const span = hi - lo;
  const n = src.length;
  return src.map((v, i) => {
    const last = i === n - 1;
    const op = last || n === 1 ? 1 : props.op0 + (1 - props.op0 - 0.16) * (i / (n - 1)) ** 1.5;
    if (props.baseline === "zero") {
      // 0 与负值只画 2px 的底；正值按占最大值的比例，最矮 3px —— 只比底高一点，能和「没有」分开，
      // 又不像 range 模式的 min=4 那样把一点点增量抬成最大值的五分之一
      const h = v > 0 && hi > 0 ? Math.max(3, Math.round((v / hi) * props.height)) : 2;
      return { h, top: op, bottom: op * props.foot };
    }
    // 序列基本持平（跨度 < 0.02）时全部取 0.62，否则会被放大成一堆噪声
    const norm = span < 0.02 ? 0.62 : 0.34 + 0.66 * ((v - lo) / span);
    return {
      h: Math.max(props.min, Math.round(norm * props.height)),
      top: op,
      bottom: op * props.foot,
    };
  });
});

function pct(v: number) {
  return `${Math.round(v * 1000) / 10}%`;
}

function barStyle(b: { h: number; top: number; bottom: number }) {
  return {
    width: `${props.barWidth}px`,
    height: `${b.h}px`,
    borderRadius: `${props.radius}px`,
    background: `linear-gradient(to bottom, color-mix(in srgb, ${props.hue} ${pct(b.top)}, transparent), color-mix(in srgb, ${props.hue} ${pct(b.bottom)}, transparent))`,
  };
}
</script>

<template>
  <div
    class="pt-bars"
    :style="{ height: `${height}px`, gap: `${gap}px` }"
    role="presentation"
    aria-hidden="true">
    <i v-for="(b, i) in bars" :key="i" :style="barStyle(b)" />
  </div>
</template>

<style scoped>
/* 右对齐：最后一根柱子是「当前值」，它该贴着盒子的右边缘 */
.pt-bars {
  display: flex;
  align-items: flex-end;
  justify-content: flex-end;
  overflow: hidden;
}

.pt-bars i {
  display: block;
  flex: 0 0 auto;
}
</style>
