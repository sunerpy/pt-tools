<script setup lang="ts">
/*
 * 输入框里发送按钮左边的上下文用量：一个小圆环加百分比（离压缩阈值近了变黄、到了变红），
 * 鼠标移上去（手机上点一下）看详情：上下文多少 token、窗口从哪儿来、什么时候自动压缩、本会话一共用了多少。
 */
import type { AssistantUsage } from "@/api";
import { useIsMobile } from "@/composables/useIsMobile";
import { usageLines, usagePercent, usageTone } from "@/utils/assistant";
import { computed } from "vue";

const props = defineProps<{ usage: AssistantUsage }>();

const isMobile = useIsMobile();
const pct = computed(() => usagePercent(props.usage));
const tone = computed(() => usageTone(props.usage));
const lines = computed(() => usageLines(props.usage));

/** 圆环：半径 7，周长约 44；超过 100% 的画满 */
const R = 7;
const C = 2 * Math.PI * R;
const dash = computed(() => `${(Math.min(pct.value, 100) / 100) * C} ${C}`);
</script>

<template>
  <el-tooltip placement="top" :trigger="isMobile ? 'click' : 'hover'" :show-after="150">
    <template #content>
      <div class="as-meter__tip" data-testid="as-usage-tip">
        <div v-for="l in lines" :key="l">{{ l }}</div>
      </div>
    </template>
    <button
      type="button"
      class="as-meter"
      :class="`is-${tone}`"
      :aria-label="`上下文用了 ${pct}%`"
      data-testid="as-usage">
      <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
        <circle cx="9" cy="9" :r="R" class="as-meter__track" />
        <circle
          cx="9"
          cy="9"
          :r="R"
          class="as-meter__bar"
          :stroke-dasharray="dash"
          transform="rotate(-90 9 9)" />
      </svg>
      <span>{{ pct }}%</span>
    </button>
  </el-tooltip>
</template>

<style scoped>
.as-meter {
  display: inline-flex;
  gap: 4px;
  align-items: center;
  height: 30px;
  padding: 0 8px;
  border: 0;
  border-radius: 15px;
  background: transparent;
  color: var(--pt-t3);
  font: inherit;
  font-size: var(--pt-fz-label);
  font-variant-numeric: tabular-nums;
  cursor: default;
}

.as-meter:hover {
  background: var(--pt-hover);
}

.as-meter:focus-visible {
  outline: 2px solid var(--pt-p);
  outline-offset: 1px;
}

.as-meter__track {
  fill: none;
  stroke: var(--pt-border);
  stroke-width: 2.5;
}

.as-meter__bar {
  fill: none;
  stroke: var(--pt-ok);
  stroke-width: 2.5;
  stroke-linecap: round;
}

.as-meter.is-warn .as-meter__bar {
  stroke: var(--pt-warn);
}

.as-meter.is-warn {
  color: var(--pt-warn);
}

.as-meter.is-dang .as-meter__bar {
  stroke: var(--pt-dang);
}

.as-meter.is-dang {
  color: var(--pt-dang);
}

.as-meter__tip {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.6;
}
</style>
