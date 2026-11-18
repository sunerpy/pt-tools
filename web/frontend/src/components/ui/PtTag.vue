<script setup lang="ts">
/**
 * 中性标签 —— 设计文档 §3 的 `k.tag`（20 高 / 11 号 / 圆角 4 / hover 底 / t2 字）。
 *
 * 与 PtStatusPill 的分工：标签是分类，胶囊是状态。状态一律换 PtStatusPill。
 * tone 只给解析标签用（画板 06：分辨率 info、HDR kpi-3、中字 ok，各自的浅底加同色字），
 * 让一排标签里最先要看的几项一眼分得出来；它们仍是分类，不带状态圆点。
 * 需要图标时往 icon 插槽塞一个 PtIcon，尺寸由调用方决定。
 */
withDefaults(defineProps<{ tone?: "neutral" | "info" | "kpi3" | "ok" }>(), { tone: "neutral" });
</script>

<template>
  <span class="pt-tag" :class="tone !== 'neutral' ? `pt-tag--${tone}` : undefined">
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

/* 画板是 10% 叠白，落地用现有的 --pt-*-soft（12%，INDEX 偏离 ①） */
.pt-tag--info {
  color: var(--pt-info);
  background: var(--pt-info-soft);
}

.pt-tag--ok {
  color: var(--pt-ok);
  background: var(--pt-ok-soft);
}

.pt-tag--kpi3 {
  color: var(--pt-kpi-3);
  background: color-mix(in srgb, var(--pt-kpi-3) 12%, transparent);
}
</style>
