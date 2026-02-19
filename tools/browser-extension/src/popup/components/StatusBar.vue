<script setup lang="ts">
import type { PtToolsConnection, SiteInfo } from "../../core/types";

defineProps<{
  connection: PtToolsConnection;
  currentSite: SiteInfo | null;
  pageCount: number;
}>();
</script>

<template>
  <section class="status-bar">
    <div class="status-pill" :class="connection.connected ? 'ok' : 'warn'">
      {{ connection.connected ? "pt-tools 已连接" : "pt-tools 未连接" }}
    </div>
    <p class="status-line">当前站点: {{ currentSite ? currentSite.name : "未检测到 PT 站点" }}</p>
    <p class="status-line">已采集页面: {{ pageCount }}</p>
    <p v-if="connection.lastSync" class="status-line">上次同步: {{ connection.lastSync }}</p>
  </section>
</template>
