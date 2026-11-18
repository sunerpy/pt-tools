<script setup lang="ts">
import { computed, ref, watch } from "vue";

import PtIcon from "@/components/PtIcon";
import { posterURL } from "@/utils/media";

/**
 * 条目海报（按条目搜索的输入框标记、候选、左栏、手机条目卡）：TMDB 路径用现有的 posterURL 拼地址；
 * 没有海报或加载失败时用探索页的无海报样式（hover 底加图标）。
 */
const props = withDefaults(
  defineProps<{
    path?: string;
    /** TMDB 图片尺寸 */
    size?: string;
    width: number;
    height: number;
    radius?: number;
  }>(),
  { path: "", size: "w185", radius: 4 },
);

const failed = ref(false);
watch(
  () => props.path,
  () => (failed.value = false),
);

const src = computed(() => (failed.value ? "" : posterURL(props.path, props.size)));
const box = computed(() => ({
  width: `${props.width}px`,
  height: `${props.height}px`,
  borderRadius: `${props.radius}px`,
}));
</script>

<template>
  <span class="es-poster" :style="box" aria-hidden="true">
    <img v-if="src" :src="src" alt="" loading="lazy" @error="failed = true" />
    <PtIcon v-else name="compass" :size="Math.max(10, Math.round(width * 0.4))" />
  </span>
</template>

<style scoped>
.es-poster {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  color: var(--pt-t3);
  background: var(--pt-hover);
}

.es-poster img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
</style>
