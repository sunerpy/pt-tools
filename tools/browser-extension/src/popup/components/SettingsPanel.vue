<script setup lang="ts">
import { reactive, ref, watch } from "vue";

import { t } from "../../core/i18n";
import CookieStatus from "./CookieStatus.vue";
import type { KnownSiteStatus, PtToolsConnection } from "../../core/types";

const props = defineProps<{
  connection: PtToolsConnection;
  busy: boolean;
  sites: KnownSiteStatus[];
}>();

const emit = defineEmits<{
  test: [payload: { baseUrl: string; username?: string; password?: string }];
  save: [payload: { baseUrl: string }];
  batchSync: [siteIds: string[]];
}>();

const form = reactive({
  baseUrl: props.connection.baseUrl || "http://localhost:8080",
  username: "",
  password: "",
});

const selectedSiteIds = ref<Set<string>>(new Set());
const allSelected = ref(false);

watch(
  () => props.connection.baseUrl,
  (value) => {
    if (value) {
      form.baseUrl = value;
    }
  },
);

const syncableSites = (): KnownSiteStatus[] =>
  props.sites.filter(
    (s) =>
      s.site.syncField === "cookie" &&
      (s.cookieStatus === "valid" || s.cookieStatus === "expiring"),
  );

function toggleAll(): void {
  const sites = syncableSites();
  if (allSelected.value) {
    selectedSiteIds.value = new Set();
    allSelected.value = false;
  } else {
    selectedSiteIds.value = new Set(sites.map((s) => s.site.id));
    allSelected.value = true;
  }
}

function toggleSite(siteId: string): void {
  const next = new Set(selectedSiteIds.value);
  if (next.has(siteId)) {
    next.delete(siteId);
  } else {
    next.add(siteId);
  }
  selectedSiteIds.value = next;
  allSelected.value = next.size === syncableSites().length && next.size > 0;
}

function handleSave(): void {
  emit("save", { baseUrl: form.baseUrl.trim() });
}

function handleTest(): void {
  const payload: { baseUrl: string; username?: string; password?: string } = {
    baseUrl: form.baseUrl.trim(),
  };

  if (form.username.trim() && form.password.trim()) {
    payload.username = form.username.trim();
    payload.password = form.password.trim();
  }

  emit("test", payload);
}

function handleBatchSync(): void {
  if (selectedSiteIds.value.size === 0) return;
  emit("batchSync", [...selectedSiteIds.value]);
}
</script>

<template>
  <div class="settings-panel">
    <label class="field">
      <span>{{ t("settings.ptToolsUrl") }}</span>
      <input v-model="form.baseUrl" type="url" :placeholder="t('settings.urlPlaceholder')" />
    </label>
    <label class="field">
      <span>{{ t("settings.username") }}</span>
      <input
        v-model="form.username"
        type="text"
        autocomplete="off"
        :placeholder="t('settings.usernamePlaceholder')" />
    </label>
    <label class="field">
      <span>{{ t("settings.password") }}</span>
      <input
        v-model="form.password"
        type="password"
        autocomplete="off"
        :placeholder="t('settings.passwordPlaceholder')" />
    </label>
    <div class="row-actions">
      <button type="button" class="btn" :disabled="busy" @click="handleSave">
        {{ busy ? t("settings.saving") : t("settings.save") }}
      </button>
      <button type="button" class="btn primary" :disabled="busy" @click="handleTest">
        {{ busy ? t("settings.testing") : t("settings.testConnection") }}
      </button>
    </div>

    <div v-if="sites.length > 0" class="batch-sync-section">
      <div class="batch-header">
        <span class="batch-title">{{ t("settings.batchSync") }}</span>
        <label class="check-row select-all">
          <input type="checkbox" :checked="allSelected" @change="toggleAll" />
          <span>{{ t("settings.selectAll") }}</span>
        </label>
      </div>
      <ul class="site-check-list">
        <li v-for="s in sites" :key="s.site.id" class="site-check-item">
          <label class="check-row" :class="{ disabled: s.site.syncField !== 'cookie' }">
            <input
              type="checkbox"
              :checked="selectedSiteIds.has(s.site.id)"
              :disabled="
                s.site.syncField !== 'cookie' ||
                s.cookieStatus === 'missing' ||
                s.cookieStatus === 'expired'
              "
              @change="toggleSite(s.site.id)" />
            <span class="site-label">{{ s.site.name }}</span>
          </label>
          <span v-if="s.site.syncField !== 'cookie'" class="auth-badge">
            {{ s.site.syncField === "api_key" ? "API Key" : "Passkey" }}
          </span>
          <CookieStatus v-else :status="s.cookieStatus" :days="s.cookieExpireDays" compact />
        </li>
      </ul>
      <button
        type="button"
        class="btn primary"
        :disabled="busy || selectedSiteIds.size === 0"
        @click="handleBatchSync">
        {{ busy ? t("known.syncing") : t("settings.syncSelected", selectedSiteIds.size) }}
      </button>
    </div>
  </div>
</template>
