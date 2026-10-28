<script setup lang="ts">
import { onBeforeUnmount, onMounted, provide, ref } from "vue";
import { BridgeError, cancelPending, nativeReady, type NativeConfig } from "./bridge";
import { Language } from "./i18n";
import SubscriptionEditor from "./SubscriptionEditor.vue";
import BrushEditor from "./BrushEditor.vue";
import PosterPage from "./PosterPage.vue";
const params = new URLSearchParams(location.search);
const english = params.get("lang")?.startsWith("en") ?? false;
provide(Language, english);
const config = ref<NativeConfig>();
const error = ref("");
const page = params.get("page");
const recordId = Number(params.get("id") || 0);
async function load() {
  error.value = "";
  try {
    config.value = await nativeReady();
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e);
  }
}
onMounted(() => void load());
onBeforeUnmount(cancelPending);
</script>
<template>
  <main class="mobile-page">
    <div v-if="error" role="alert" class="mobile-state">
      <p>{{ error }}</p>
      <el-button @click="load">{{ english ? "Retry" : "重试" }}</el-button>
    </div>
    <div v-else-if="!config" role="status" class="mobile-state">
      {{ english ? "Connecting…" : "正在连接…" }}
    </div>
    <SubscriptionEditor
      v-else-if="page === 'subscription'"
      :id="recordId"
      :can-write="config.canWrite" />
    <BrushEditor v-else-if="page === 'brush'" :id="recordId" :can-write="config.canWrite" />
    <PosterPage v-else-if="page === 'poster'" />
    <div v-else role="alert">
      {{ new BridgeError(english ? "Unknown page" : "没有这个页面").message }}
    </div>
  </main>
</template>
