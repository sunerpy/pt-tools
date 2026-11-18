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
  /** 柱图画的是什么（鼠标悬停与读屏都用它） */
  seriesHint?: string;
  /** 柱图的归一化方式，见 PtBars 的 baseline；每日增量这类 0 就是没有的序列传 zero */
  seriesBaseline?: "range" | "zero";
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
    <div class="pt-kpi__grid" :data-cols="cols" :style="{ '--pt-kpi-cols': cols }">
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
          <!--
            柱图的含义每格不同（有的是按站点的构成，有的是最近 7 天按天），
            所以必须带上说明：只给一排柱子、不说它画的是什么，读者只能猜。
            用 title + aria-label，一个给鼠标一个给读屏。
          -->
          <PtBars
            v-if="it.series && it.series.length"
            :values="it.series"
            :baseline="it.seriesBaseline ?? 'range'"
            :hue="hue(i)"
            :count="7"
            :op0="0.5"
            :foot="0.5"
            :height="22"
            class="pt-kpi__bars"
            role="img"
            :title="it.seriesHint"
            :aria-label="it.seriesHint ?? `${it.label} 走势`" />
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.pt-kpi {
  /* 换列按 KPI 带自己的宽度判，不按视口 —— 见文件末尾那条 @container */
  container-type: inline-size;
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
  /*
   * 格间竖线**上下各留 14px**（画板 kpibar 的 vh1..vh5 是 y=14、高 36，在 64 高的带里）。
   * 通栏到底的竖线会把这条带切成一格格盒子；留白的短线只起分隔作用，轻得多。
   * 用背景渐变而不是 ::before：这样仍然靠外层「往左上挪 1px + overflow 裁掉」那套规则
   * 消掉最左那条线，格数与换行都不用改规则。
   */
  background-image: linear-gradient(
    to bottom,
    transparent 0 14px,
    var(--pt-border) 14px calc(100% - 14px),
    transparent calc(100% - 14px)
  );
  background-repeat: no-repeat;
  background-position: left top;
  background-size: 1px 100%;
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

/*
 * KPI 的变化胶囊：画板 kpibar 的 d* 是 17 高、文字 10/500（比表内状态那枚小一档）。
 * 公用的 .pt-pill--sm 按画板的**表内**规格是 18/11，所以这里压回 KPI 自己的尺寸。
 */
.pt-kpi__cell :deep(.pt-pill--sm) {
  height: 17px;
  font-size: var(--pt-fz-foot);
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

/*
 * 柱图按画板 48 宽，但格子紧的时候先让柱收窄（最窄 40），不先把读数挤出格子：
 * 读数是主信息、不能截断，柱只是走势示意。1376 下主区出现 10px 滚动条时 6 格只剩 183，
 * 「412.6 TB」那格要 187，柱收到 45 就放得下，不用为这 4px 把整条带折成两行。
 */
.pt-kpi__bars {
  flex: 0 1 48px;
  min-width: 40px;
}

/* 中屏三列、手机两列（设计文档 §9：KPI 条在手机上退化成 2×N 网格） */
@media (max-width: 1180px) {
  .pt-kpi__grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

/*
 * 6 列那一档按**主区实际宽度**退 3 列，不按视口。
 *
 * 一格要放下「读数 + 8 间距 + 柱」再加左右 16 的内边距。「412.6 TB」这种最长的读数约 99 宽，
 * 柱收到最窄 40 时一格要 179，6 列就得 ≥ 1074（再留一点余量取 1070 以下才退）。
 * 上面那条 @media 按视口 ≤1180 退 3 列，是侧栏还是两列（64 + 264）时的算法；
 * 侧栏改成一块之后，钉住时 1181–1375 这一段的主区只有 917–1111，比 ≤1180 收起时（主区 1116）
 * 还窄，于是 1280 下「总上传量」那格的柱被切掉一截。发布前 1280 / 1024 扫描抓出来的。
 *
 * 阈值不能取 1110：1376 下主区 1112，但内容一长出滚动条（.pt-shell__content 的 10px），
 * KPI 带就只剩 1102 —— 取 1110 会让画板基准宽度下的 6 格一行变成 3×2（board-check 抓到带高 128）。
 * 下限 740 是为了不碰 ≤768 那条移动端 2×N（画板 §9）。
 */
@container (min-width: 740px) and (max-width: 1069px) {
  .pt-kpi__grid[data-cols="6"] {
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

  /*
   * 手机上一格只有 140 多宽，「本周 +974 GB」这类周期增量胶囊会把标签挤成「总上…」。
   * 标签不再让位，放不下时胶囊折到下一行；标签自己比一行还长时才省略。
   */
  .pt-kpi__top {
    flex-wrap: wrap;
    row-gap: 2px;
  }

  .pt-kpi__label {
    flex-shrink: 0;
    max-width: 100%;
  }

  /*
   * 2 列时一格的读数行在 375 宽下只有 148（360 宽的安卓机约 140），「412.6 TB」加 8 间距就要 107，
   * 柱最窄 40 刚好压线、窄一点的屏就溢出 —— 手机上再让柱收到 32。
   */
  .pt-kpi__bars {
    min-width: 32px;
  }
}
</style>
