<script setup lang="ts">
import { computed } from "vue";

import { t } from "../../core/i18n";
import type { KnownSiteStatus, PtToolsConnection } from "../../core/types";

const props = defineProps<{
  sites: KnownSiteStatus[];
  connection: PtToolsConnection;
}>();

const healthyCount = computed(
  () =>
    props.sites.filter((site) => site.cookieStatus === "valid" || site.cookieStatus === "expiring")
      .length,
);
</script>

<template>
  <article class="card default-card">
    <p class="muted-line hint-line">{{ t("default.hint") }}</p>
    <p class="line-item">
      <span>{{ t("default.builtinSites") }}</span>
      <strong>{{ sites.length }}</strong>
    </p>
    <p class="line-item">
      <span>{{ t("default.cookieAvailable") }}</span>
      <strong>{{ healthyCount }} / {{ sites.length }}</strong>
    </p>
    <p class="line-item">
      <span>{{ t("default.pttools") }}</span>
      <strong>{{
        connection.connected ? t("default.connected") : t("default.disconnected")
      }}</strong>
    </p>
  </article>
</template>
