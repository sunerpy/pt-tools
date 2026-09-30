<script setup lang="ts">
/**
 * 卡内的分布列表 —— 画板里 p-* 分析卡的通用内容：一行「标签 + 计数」，
 * 下面一根 5 高的占比柱（设计令牌里进度条就是 5）。
 *
 * 为什么做成组件：画板给 7 个页面都安排了这类分析卡（p-dist / p-site / p-cmd /
 * p-res / p-order …），内容各不相同但画法完全一样。之前只有站点列表一页手写了这套
 * 标记与样式，再复制五份就会开始各自漂移 —— 柱子高度、间距、语义色迟早对不上。
 *
 * 柱长的分母默认是所有行里的最大值，也就是「组内占比」而不是绝对量；
 * 想按总数算就显式传 `total`（比如 14 个站点里 3 个异常，柱子该是 3/14）。
 */

export interface BreakdownRow {
  /** v-for 的键，也用于区分同名标签 */
  key: string;
  label: string;
  /** 计数或数值。字符串按原样显示（已经格式化过的体积、分享率之类） */
  value: number | string;
  /** 画柱子用的量。省略时用 value（必须是数字），value 是字符串时必须给 */
  weight?: number;
  /** 柱子的语义色 */
  tone?: "primary" | "ok" | "warn" | "dang" | "info" | "mute";
  /** 标签下面那行小字，用来说明这一项是什么 */
  hint?: string;
}

const props = withDefaults(
  defineProps<{
    rows: BreakdownRow[];
    /** 占比的分母。省略 = 用行里的最大 weight */
    total?: number;
    /** 宽卡片里横着铺成多列（画板 1080 通栏的 p-auth 就是这样） */
    cols?: boolean;
    /** 列表下面的口径说明（10/400 t4） */
    foot?: string;
  }>(),
  { total: 0, cols: false, foot: "" },
);

function weightOf(row: BreakdownRow): number {
  if (typeof row.weight === "number") return row.weight;
  return typeof row.value === "number" ? row.value : 0;
}

/** 分母为 0 时所有柱子都画 0 宽，而不是除出 NaN */
function barWidth(row: BreakdownRow): string {
  const denom = props.total > 0 ? props.total : Math.max(...props.rows.map(weightOf), 0);
  if (denom <= 0) return "0%";
  return `${Math.min(100, Math.round((weightOf(row) / denom) * 100))}%`;
}
</script>

<template>
  <div class="bd">
    <ul class="bd__list" :class="{ 'is-cols': cols }">
      <li v-for="row in rows" :key="row.key" class="bd__row">
        <span class="bd__k">{{ row.label }}</span>
        <span class="bd__v">{{ row.value }}</span>
        <span class="bd__bar" :class="`is-${row.tone ?? 'primary'}`" aria-hidden="true">
          <span class="bd__fill" :style="{ width: barWidth(row) }" />
        </span>
        <span v-if="row.hint" class="bd__hint">{{ row.hint }}</span>
      </li>
    </ul>
    <p v-if="foot" class="bd__foot">{{ foot }}</p>
  </div>
</template>

<style scoped>
.bd__list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 0;
  padding: 0;
  list-style: none;
}

/* 宽卡片横着铺：200 下限保证「标签 + 数字」不会挤成两行 */
.bd__list.is-cols {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 10px var(--pt-pad);
}

.bd__row {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 4px var(--pt-space-2);
  align-items: center;
  min-width: 0;
}

.bd__k {
  overflow: hidden;
  font-size: var(--pt-fz-sm);
  color: var(--pt-t2);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bd__v {
  font-size: var(--pt-fz-sm);
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t1);
}

.bd__bar {
  grid-column: 1 / -1;
  height: 5px;
  overflow: hidden;
  background: var(--pt-hover);
  border-radius: 999px;
}

.bd__fill {
  display: block;
  height: 100%;
  background: var(--bd-c, var(--pt-p));
  border-radius: inherit;
}

.bd__bar.is-ok {
  --bd-c: var(--pt-ok);
}

.bd__bar.is-warn {
  --bd-c: var(--pt-warn);
}

.bd__bar.is-dang {
  --bd-c: var(--pt-dang);
}

.bd__bar.is-info {
  --bd-c: var(--pt-info);
}

.bd__bar.is-mute {
  --bd-c: var(--pt-t4);
}

.bd__hint {
  grid-column: 1 / -1;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t3);
}

/* 卡片脚注：说明口径，10/400 t4（画板脚注字号） */
.bd__foot {
  margin: var(--pt-space-3) 0 0;
  font-size: var(--pt-fz-foot);
  line-height: 1.5;
  color: var(--pt-t3);
}
</style>
