<script setup lang="ts">
import { computed } from "vue";

/**
 * 细条进度 —— 设计文档 §3 的 `k.progress`（默认 5 高、圆角 3、轨道见下）。
 *
 * 轨道颜色分明暗：设计文档 §3 明确写了「浅色 canvas 上进度轨道必须用
 * borderStrong，用 border 会看不见空轨」，暗色配色下反过来，border 就够。
 */
const props = withDefaults(
  defineProps<{
    /** 0–100，越界自动夹住 */
    percent: number;
    tone?: "primary" | "ok" | "warn" | "dang" | "info";
    height?: number;
  }>(),
  { tone: "primary", height: 5 },
);

const clamped = computed(() => Math.min(100, Math.max(0, Number(props.percent) || 0)));

/** 与设计稿一致：非零进度至少给 2px，否则 1% 会画成一条看不见的线 */
const fillWidth = computed(() => (clamped.value <= 0 ? "0" : `max(2px, ${clamped.value}%)`));
</script>

<template>
  <div
    class="pt-progress"
    :class="`pt-progress--${tone}`"
    :style="{ height: `${height}px`, borderRadius: `${height / 2}px` }"
    role="progressbar"
    :aria-valuenow="clamped"
    aria-valuemin="0"
    aria-valuemax="100">
    <i :style="{ width: fillWidth }" />
  </div>
</template>

<style scoped>
.pt-progress {
  width: 100%;
  overflow: hidden;
  background: var(--pt-border-strong);
}

html.dark .pt-progress {
  background: var(--pt-border);
}

.pt-progress i {
  display: block;
  height: 100%;
  background: var(--pt-progress-c);
  border-radius: inherit;
  transition: width var(--pt-transition-normal);
}

.pt-progress--primary {
  --pt-progress-c: var(--pt-p);
}

.pt-progress--ok {
  --pt-progress-c: var(--pt-ok);
}

.pt-progress--warn {
  --pt-progress-c: var(--pt-warn);
}

.pt-progress--dang {
  --pt-progress-c: var(--pt-dang);
}

.pt-progress--info {
  --pt-progress-c: var(--pt-info);
}

@media (prefers-reduced-motion: reduce) {
  .pt-progress i {
    transition: none;
  }
}
</style>
