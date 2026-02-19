<script setup lang="ts">
import { computed } from "vue";

import type { CollectionSession, SiteInfo } from "../../core/types";

const props = defineProps<{
  currentSite: SiteInfo | null;
  session: CollectionSession | null;
  pageCount: number;
  busy: boolean;
}>();

const emit = defineEmits<{
  capture: [];
  refresh: [];
}>();

const progress = computed(() => {
  const target = 3;
  const done = Math.min(props.pageCount, target);
  return `${done}/${target}`;
});
</script>

<template>
  <section class="panel">
    <h3>页面采集</h3>
    <p class="muted">在目标 PT 页面点击采集，内容会自动脱敏后保存。</p>
    <p class="status-line">
      检测结果: {{ currentSite ? `${currentSite.name} (${currentSite.schema})` : "未识别" }}
    </p>
    <p class="status-line">进度: {{ progress }}</p>
    <div class="actions">
      <button type="button" class="btn" :disabled="busy || !currentSite" @click="emit('capture')">
        {{ busy ? "采集中..." : "采集当前页" }}
      </button>
      <button type="button" class="btn ghost" :disabled="busy" @click="emit('refresh')">
        刷新状态
      </button>
    </div>
    <ul v-if="session?.pages.length" class="simple-list">
      <li v-for="item in session.pages" :key="item.pageType">
        <span>{{ item.pageType }}</span>
      </li>
    </ul>
  </section>
</template>
