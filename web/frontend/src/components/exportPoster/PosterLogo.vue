<script setup lang="ts">
/**
 * 海报上的站点图标：打码时是 4×4 色块（从图标缩出来的，看得出颜色看不出是哪个站）；
 * 不打码时是站点的 favicon（data URL，导出时不用再取），取不到时是首字母块。
 * alt 一律留空：图标是装饰，站点名另有文字，打码时 alt 里也不能有站点名。
 */
import { computed } from "vue";

import type { PosterLogoData } from "./model";

const props = withDefaults(
  defineProps<{
    logo: PosterLogoData;
    name: string;
    color: string;
    masked: boolean;
    size?: number;
    radius?: number;
  }>(),
  { size: 22, radius: 6 },
);

const box = computed(() => ({
  width: `${props.size}px`,
  height: `${props.size}px`,
  borderRadius: `${props.radius}px`,
}));

const letter = computed(() => (Array.from(props.name)[0] ?? "?").toUpperCase());
</script>

<template>
  <span v-if="masked" class="poster-logo poster-logo--mask" :style="box">
    <b v-for="(c, i) in logo.cells" :key="i" :style="{ background: c }" />
  </span>
  <span v-else-if="logo.src" class="poster-logo poster-logo--img" :style="box">
    <img :src="logo.src" alt="" />
  </span>
  <span
    v-else
    class="poster-logo"
    :style="{ ...box, background: color, fontSize: `${Math.round(size * 0.5)}px` }">
    {{ letter }}
  </span>
</template>
