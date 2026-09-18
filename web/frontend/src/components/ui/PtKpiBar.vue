<script setup lang="ts">
import { computed } from "vue";
import PtIcon from "../PtIcon";
import PtBars from "./PtBars.vue";
import PtStatusPill from "./PtStatusPill.vue";

/**
 * KPI 指标条 —— 设计稿 G 的 `kpiBar()`：64 高、surface 底、每格一条竖发丝线，
 * 单格内是「图标 + 11 号标签 + 状态胶囊」压一行、「20/700 读数 + 彩虹柱」压一行。
 *
 * 色相按格子序号在 --pt-kpi-1..4 之间轮转（设计文档 §4 的 `k.A.G`）。稿上只有柱子
 * 带色，但本项目多数聚合接口没有历史序列（例如 /userinfo/aggregated 只给当前值），
 * 没有柱子色彩身份就全丢了 —— 所以图标也按同一轮转上色，不给的序列不编造。
 *
 * 稿上它是贴着主列左右边缘的通栏条；本项目内容区自带 16 内边距，所以改成带边框
 * 圆角的整块，视觉重量与稿一致，不会在留白里飘一条无框横线。
 */
interface KpiItem {
  /** 指标名，11 号标签 */
  label: string;
  /** 读数正文，已格式化好的字符串或数字 */
  value: string | number;
  /** 读数后缀，小一号弱化显示 */
  unit?: string;
  /** 状态或变化量，例如 "+12%"、"偏低"；不传则不显示胶囊 */
  delta?: string;
  deltaTone?: "ok" | "warn" | "dang" | "info" | "primary" | "neutral";
  icon?: string;
  /** 趋势序列，取最后 7 个点画柱；没有真实序列就别传 */
  series?: number[];
}

const props = withDefaults(
  defineProps<{
    items: KpiItem[];
    /** 每行格数，默认按项数铺开，最多 6（再多每格就窄到读不了数） */
    cols?: number;
    /**
     * 带形态 —— 画板 kpibar 328,0 1112×64：主区最顶上的一条全宽白带，
     * 格间 1px 竖线、下沿一条线，不包在卡片里。
     */
    band?: boolean;
  }>(),
  { cols: 0, band: false },
);

const cols = computed(() => props.cols || Math.min(props.items.length || 1, 6));

/** 四色循环：第 5 格回到第 1 色，与稿上 `k.A.G[i % 4]` 同一算法 */
function hue(i: number) {
  return `var(--pt-kpi-${(i % 4) + 1})`;
}
</script>

<template>
  <div class="pt-kpi" :class="{ 'is-band': band }">
    <div class="pt-kpi__grid" :style="{ '--pt-kpi-cols': cols }">
      <div v-for="(it, i) in props.items" :key="it.label" class="pt-kpi__cell">
        <div class="pt-kpi__top">
          <PtIcon
            v-if="it.icon"
            :name="it.icon"
            :size="13"
            class="pt-kpi__icon"
            :style="{ color: hue(i) }" />
          <span class="pt-kpi__label">{{ it.label }}</span>
          <PtStatusPill v-if="it.delta" size="sm" :tone="it.deltaTone ?? 'neutral'">
            {{ it.delta }}
          </PtStatusPill>
        </div>
        <div class="pt-kpi__bottom">
          <span class="pt-kpi__value">
            {{ it.value }}<em v-if="it.unit">{{ it.unit }}</em>
          </span>
          <PtBars
            v-if="it.series && it.series.length"
            :values="it.series"
            :hue="hue(i)"
            :count="7"
            :op0="0.5"
            :foot="0.5"
            :height="22"
            class="pt-kpi__bars" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pt-kpi {
  min-height: var(--pt-kpi-h);
  overflow: hidden;
  background: var(--pt-surface);
  border: 1px solid var(--pt-border);
  border-radius: var(--pt-r-lg);
}

/*
 * 带形态 —— 画板 kpibar：主区顶上的全宽白带，四周不描边、不圆角，
 * 只在下沿留一条线。格间竖线由下面那套「每格画左上两条边」的规则继续提供。
 */
.pt-kpi.is-band {
  border: 0;
  border-bottom: 1px solid var(--pt-border);
  border-radius: 0;
}

/*
 * 格线用「每格画左上两条边 + 整个网格往左上挪 1px」实现：外沿那一圈被
 * overflow 裁掉，剩下的正好是内部格线。这样格数、换行、末行不满都不用改规则 ——
 * 换成 nth-child 猜列数的写法，一到断点换列就会在行首多出一条竖线。
 */
.pt-kpi__grid {
  display: grid;
  grid-template-columns: repeat(var(--pt-kpi-cols, 4), minmax(0, 1fr));
  margin: -1px 0 0 -1px;
}

.pt-kpi__cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  justify-content: center;
  min-width: 0;
  min-height: var(--pt-kpi-h);
  padding: var(--pt-space-2) var(--pt-pad);
  border-top: 1px solid var(--pt-border);
  border-left: 1px solid var(--pt-border);
}

.pt-kpi__top {
  display: flex;
  gap: 6px;
  align-items: center;
  min-width: 0;
}

.pt-kpi__icon {
  flex: 0 0 auto;
}

.pt-kpi__label {
  overflow: hidden;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  color: var(--pt-t3);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.pt-kpi__bottom {
  display: flex;
  gap: var(--pt-space-2);
  align-items: flex-end;
  justify-content: space-between;
  min-width: 0;
}

/* --pt-fz-display 就是为这个读数留的 20px，不要换成区块标题的 16 */
.pt-kpi__value {
  font-size: var(--pt-fz-display);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: var(--pt-lh-tight);
  color: var(--pt-t1);
  white-space: nowrap;
}

.pt-kpi__value em {
  margin-left: 2px;
  font-size: var(--pt-fz-sm);
  font-style: normal;
  font-weight: 500;
  color: var(--pt-t3);
}

.pt-kpi__bars {
  flex: 0 0 auto;
  width: 48px;
}

/* 中屏三列、手机两列（设计文档 §9：KPI 条在手机上退化成 2×N 网格） */
@media (max-width: 1180px) {
  .pt-kpi__grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 768px) {
  .pt-kpi__grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .pt-kpi__cell {
    gap: 4px;
    padding: var(--pt-space-3);
  }
}
</style>
