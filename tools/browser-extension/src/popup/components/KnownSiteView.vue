<script setup lang="ts">
import { computed } from "vue";

import { t } from "../../core/i18n";
import CookieStatus from "./CookieStatus.vue";
import type { KnownSiteStatus, SiteSchema, ToggleAutoSyncPayload } from "../../core/types";

const props = defineProps<{
  status: KnownSiteStatus;
  busy: boolean;
  busyCapture: boolean;
  busyAutoCollect: boolean;
}>();

const emit = defineEmits<{
  sync: [siteId: string];
  toggleAutoSync: [payload: ToggleAutoSyncPayload];
  capture: [];
  autoCollect: [payload: { siteOrigin: string; schema: SiteSchema }];
}>();

const isCookieSyncable = computed(() => props.status.site.syncField === "cookie");

const authHint = computed(() => {
  if (props.status.site.syncField === "api_key") return t("known.authHintApiKey");
  if (props.status.site.syncField === "passkey") return t("known.authHintPasskey");
  return "";
});

const siteOrigin = computed(() => {
  const domain = props.status.site.domains[0] ?? "";
  return domain ? `https://${domain}` : "";
});

function toggleAutoSync(): void {
  emit("toggleAutoSync", {
    siteId: props.status.site.id,
    enabled: !props.status.autoSync,
  });
}

function handleAutoCollect(): void {
  if (!siteOrigin.value) return;
  emit("autoCollect", { siteOrigin: siteOrigin.value, schema: props.status.site.schema });
}
</script>

<template>
  <article class="card known-card">
    <header class="card-header">
      <h3>✅ {{ status.site.name }} ({{ status.site.schema }})</h3>
    </header>

    <template v-if="isCookieSyncable">
      <p class="line-item">
        <span>{{ t("known.cookieStatus") }}</span>
        <CookieStatus :status="status.cookieStatus" :days="status.cookieExpireDays" />
      </p>

      <button
        type="button"
        class="btn primary"
        :disabled="busy"
        @click="emit('sync', status.site.id)">
        {{ busy ? t("known.syncing") : t("known.syncCookie") }}
      </button>

      <p class="muted-line">{{ t("known.lastSync", status.lastSync ?? t("known.neverSynced")) }}</p>

      <label class="switch-row">
        <span>{{ t("known.autoSync") }}</span>
        <button
          type="button"
          class="switch"
          :class="{ on: status.autoSync }"
          :disabled="busy"
          @click="toggleAutoSync">
          <span class="knob" />
        </button>
      </label>
    </template>

    <template v-else>
      <p class="line-item">
        <span>{{ t("known.authMethod") }}</span>
        <strong class="auth-badge">{{
          status.site.syncField === "api_key" ? "API Key" : "Passkey"
        }}</strong>
      </p>
      <p class="auth-hint">⚠ {{ authHint }}</p>
    </template>

    <div class="collect-actions">
      <button
        type="button"
        class="btn primary"
        :disabled="busyAutoCollect"
        @click="handleAutoCollect">
        {{ busyAutoCollect ? t("known.autoCollecting") : t("known.autoCollect") }}
      </button>
      <button type="button" class="btn ghost" :disabled="busyCapture" @click="emit('capture')">
        {{ busyCapture ? t("known.capturing") : t("known.captureCurrentPage") }}
      </button>
    </div>
  </article>
</template>
