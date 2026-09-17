<script setup lang="ts">
/**
 * 状态胶囊 —— 设计文档 §3 的 `k.statusPill`。
 *
 * 只做「浅底 + 同色字」这一种形态：`k.statusPill` 的 solid 变体会把文字刷成
 * onP（白），而暗色配色里的 ok/warn 是浅色，白字压上去对比度不够；设计稿的
 * GRID 预设本身也只用浅底变体，所以这里不提供 solid。
 */
withDefaults(
  defineProps<{
    /** 语义色。neutral 走中性灰，仅在无状态含义时用 */
    tone?: "ok" | "warn" | "dang" | "info" | "primary" | "neutral";
    /** 左侧 5px 圆点，用于页头这类需要更强状态感的位置 */
    dot?: boolean;
    /** sm = 17 高（表格内），md = 20 高（页头、卡片） */
    size?: "sm" | "md";
  }>(),
  { tone: "neutral", dot: false, size: "md" },
);
</script>

<template>
  <span class="pt-pill" :class="[`pt-pill--${tone}`, `pt-pill--${size}`]">
    <i v-if="dot" class="pt-pill__dot" aria-hidden="true" />
    <slot />
  </span>
</template>

<style scoped>
.pt-pill {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  justify-content: center;
  font-variant-numeric: tabular-nums;
  font-weight: 500;
  line-height: 1;
  color: var(--pt-pill-c);
  white-space: nowrap;
  /* 14% 是设计稿的 bgOp：浅到能压住整行底色，又不至于糊掉文字 */
  background: color-mix(in srgb, var(--pt-pill-c) 14%, transparent);
  border-radius: var(--pt-r-sm);
}

.pt-pill--md {
  height: 20px;
  padding: 0 7px;
  font-size: var(--pt-fz-label);
}

.pt-pill--sm {
  height: 17px;
  padding: 0 6px;
  font-size: var(--pt-fz-foot);
}

.pt-pill__dot {
  width: 5px;
  height: 5px;
  background: var(--pt-pill-c);
  border-radius: 50%;
}

.pt-pill--ok {
  --pt-pill-c: var(--pt-ok);
}

.pt-pill--warn {
  --pt-pill-c: var(--pt-warn);
}

.pt-pill--dang {
  --pt-pill-c: var(--pt-dang);
}

.pt-pill--info {
  --pt-pill-c: var(--pt-info);
}

.pt-pill--primary {
  --pt-pill-c: var(--pt-p);
}

.pt-pill--neutral {
  --pt-pill-c: var(--pt-t2);
}
</style>
