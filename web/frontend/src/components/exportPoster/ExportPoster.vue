<script setup lang="ts">
/**
 * 按选中的样式画海报。导出页拿 posterEl() 去转 PNG：预览与导出是同一个节点。
 */
import { computed, provide, ref } from "vue";
import { PosterLocale } from "./locale";

import "./poster.css";
import type { PosterModel } from "./model";
import PosterBento from "./PosterBento.vue";
import type { PosterFont } from "./posterImage";
import PosterNight from "./PosterNight.vue";
import PosterPaper from "./PosterPaper.vue";
import PosterReceipt from "./PosterReceipt.vue";
import PosterWrapped from "./PosterWrapped.vue";
import type { PosterStyleId } from "./styles";

const props = defineProps<{
  model: PosterModel;
  styleId: PosterStyleId;
  colorway: string;
  locale?: string;
}>();
provide(
  PosterLocale,
  computed(() => props.locale ?? "zh"),
);

const host = ref<HTMLElement | null>(null);

/** 各样式用到的打包字体：导出时只嵌这几种 */
const FONTS: Record<PosterStyleId, PosterFont[]> = {
  bento: [],
  paper: ["serif"],
  night: [],
  wrapped: [],
  receipt: ["mono"],
};

defineExpose({
  posterEl: (): HTMLElement | null => (host.value?.firstElementChild as HTMLElement | null) ?? null,
  fonts: (): PosterFont[] => FONTS[props.styleId],
});
</script>

<template>
  <div ref="host" class="poster-host">
    <PosterPaper v-if="styleId === 'paper'" :model="model" :colorway="colorway" />
    <PosterNight v-else-if="styleId === 'night'" :model="model" :colorway="colorway" />
    <PosterWrapped v-else-if="styleId === 'wrapped'" :model="model" :colorway="colorway" />
    <PosterReceipt v-else-if="styleId === 'receipt'" :model="model" :colorway="colorway" />
    <PosterBento v-else :model="model" :colorway="colorway" />
  </div>
</template>

<style scoped>
/* 只是挂载点：海报自己定宽，这一层不加任何尺寸 */
.poster-host {
  display: block;
}
</style>
