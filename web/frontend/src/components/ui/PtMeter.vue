<script setup lang="ts">
import PtProgress from "./PtProgress.vue";

/**
 * 带标签的进度行 —— 设计稿 G 的 `meter()`：
 * 左标签 12/500 t1，右读数 12/500 t2，下方 5 高进度条。
 * 磁盘余量、做种容量、凭据健康度这类「一个名字 + 一个读数 + 一个占比」都用它。
 */
withDefaults(
  defineProps<{
    label: string;
    /** 右侧读数原文，例如 "1.2 TB / 4 TB"；不传就显示百分比 */
    value?: string;
    percent: number;
    tone?: "primary" | "ok" | "warn" | "dang" | "info";
  }>(),
  { value: "", tone: "primary" },
);
</script>

<template>
  <div class="pt-meter">
    <div class="pt-meter__head">
      <span class="pt-meter__label">{{ label }}</span>
      <span class="pt-meter__value">{{ value || `${Math.round(percent)}%` }}</span>
    </div>
    <PtProgress :percent="percent" :tone="tone" />
  </div>
</template>

<style scoped>
.pt-meter {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.pt-meter__head {
  display: flex;
  gap: var(--pt-space-2);
  align-items: baseline;
  justify-content: space-between;
}

.pt-meter__label {
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  color: var(--pt-t1);
}

.pt-meter__value {
  font-size: var(--pt-fz-sm);
  font-weight: 500;
  font-variant-numeric: tabular-nums;
  color: var(--pt-t2);
}
</style>
