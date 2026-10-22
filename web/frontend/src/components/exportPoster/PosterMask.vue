<script setup lang="ts">
/**
 * 海报上的名字：打码时只留首字，后面是圆角遮盖条（宽度随字数），原文不进 DOM ——
 * 导出的 PNG 就是这份 DOM 画出来的，DOM 里没有的字图上也没有。
 * 打码的那一支是 inline-flex：子元素之间的空白不占位，模板怎么换行都不影响间距。
 */
import { computed } from "vue";

import { maskParts } from "./model";

const props = withDefaults(defineProps<{ text: string; on: boolean; prefix?: string }>(), {
  prefix: "",
});

const parts = computed(() => maskParts(props.text));
</script>

<template>
  <span v-if="on && text" class="poster-mk">
    <span>{{ prefix }}{{ parts.first }}</span>
    <i :style="{ width: `${parts.barEm.toFixed(2)}em` }" />
  </span>
  <span v-else class="poster-mk-text">{{ prefix }}{{ text }}</span>
</template>
