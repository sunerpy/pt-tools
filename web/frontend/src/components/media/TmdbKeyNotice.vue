<script setup lang="ts">
/*
 * 没有能用的 TMDB API Key 时的提示：探索、订阅、整理入库都要用 TMDB，没有 Key 时这些页只会拿到一句报错。
 * 发布的版本内置了 Key，没填也能用；没有能用的是两种情况：这个版本没有内置的（从源码编译）且没填，
 * 或者换了 TMDB 接口地址（内置的只发给官方地址）。这里先读一次媒体识别的设置，没有能用的 Key 时写明
 * 要用它做什么、去哪里填（带跳转）。读设置失败时不提示，免得误报。
 */
import { mediaApi } from "@/api";
import PtIcon from "@/components/PtIcon";
import { onMounted, ref } from "vue";

const props = defineProps<{
  /** 这一页拿 TMDB 做什么，例如「探索与订阅」 */
  usage: string;
}>();

const missing = ref(false);
/** 这个版本内置了 Key，是换了接口地址才用不了 */
const builtinBlocked = ref(false);

onMounted(async () => {
  try {
    const s = await mediaApi.settings();
    missing.value = Boolean(s) && s.key_source === "none";
    builtinBlocked.value = Boolean(s?.builtin_key);
  } catch {
    missing.value = false;
  }
});
</script>

<template>
  <div v-if="missing" class="pt-note pt-note--warn" data-testid="tmdb-key-notice">
    <PtIcon name="key-round" :size="14" class="pt-note__icon" />
    <span v-if="builtinBlocked"
      >换了 TMDB 接口地址，内置的 Key 只发给 TMDB 官方地址，现在没有能用的 Key：{{
        props.usage
      }}都要用它，没有时只能看到报错。
      <router-link to="/media/recognize" data-testid="tmdb-key-link"
        >去「媒体识别」填写自己的 Key 或清空接口地址</router-link
      ></span
    >
    <span v-else
      >没有填写 TMDB API Key：{{ props.usage }}都要用它，没有时只能看到报错。
      <router-link to="/media/recognize" data-testid="tmdb-key-link"
        >去「媒体识别」填写</router-link
      ></span
    >
  </div>
</template>
