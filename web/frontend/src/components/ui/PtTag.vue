<script setup lang="ts">
/**
 * 中性标签 —— 设计文档 §3 的 `k.tag`（20 高 / 11 号 / 圆角 4 / hover 底 / t2 字）。
 *
 * 与 PtStatusPill 的分工：标签是分类，胶囊是状态。语义色只留给状态，
 * 所以这里没有 tone —— 想表达状态就换 PtStatusPill。
 * 需要图标时往 icon 插槽塞一个 PtIcon，尺寸由调用方决定。
 */
</script>

<template>
  <span class="pt-tag">
    <slot name="icon" />
    <slot />
  </span>
</template>

<style scoped>
.pt-tag {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  height: 20px;
  padding: 0 7px;
  font-size: var(--pt-fz-label);
  font-weight: 500;
  line-height: 1;
  color: var(--pt-t2);
  white-space: nowrap;
  background: var(--pt-hover);
  border-radius: var(--pt-r-sm);
}
</style>
