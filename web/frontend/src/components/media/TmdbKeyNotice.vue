<script setup lang="ts">
/*
 * 没填 TMDB API Key 时的提示：探索、订阅、整理入库都要用 TMDB，没有 Key 时这些页只会拿到一句报错。
 * 这里先读一次媒体识别的设置，没有 Key 时写明要用它做什么、去哪里填（带跳转）。读设置失败时不提示，免得误报。
 */
import { mediaApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import { onMounted, ref } from "vue";

const props = defineProps<{
  /** 这一页拿 TMDB 做什么，例如「探索与订阅」 */
  usage: string;
}>();

const missing = ref(false);

onMounted(async () => {
  try {
    const s = await mediaApi.settings();
    missing.value = Boolean(s) && !s.has_tmdb_key;
  } catch {
    missing.value = false;
  }
});
</script>

<template>
  <div v-if="missing" class="pt-note pt-note--warn" data-testid="tmdb-key-notice">
    <PtIcon name="key-round" :size="14" class="pt-note__icon" />
    <span
      >没有填写 TMDB API Key：{{ props.usage }}都要用它，没有时只能看到报错。
      <router-link to="/media/recognize" data-testid="tmdb-key-link"
        >去「媒体识别」填写</router-link
      ></span
    >
  </div>
</template>
