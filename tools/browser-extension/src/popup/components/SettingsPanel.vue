<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";

import { STORAGE_KEYS } from "../../core/constants";
import { t } from "../../core/i18n";
import { get, set } from "../../core/storage";
import CookieStatus from "./CookieStatus.vue";
import SiteLoginStatusPanel from "./SiteLoginStatusPanel.vue";
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
  batchOpen: [siteIds: string[]];
  checkUpdate: [];
}>();

const form = reactive({
  baseUrl: props.connection.baseUrl || "http://localhost:8080",
  username: "",
  password: "",
});

const selectedSiteIds = ref<Set<string>>(new Set());
const allSelected = ref(false);
const autoOpenTabsOnSync = ref(true);
const confirmVisible = ref(false);
const onlyWithCookie = ref(false);

const visibleSites = computed((): KnownSiteStatus[] =>
  onlyWithCookie.value ? props.sites.filter((s) => s.hasCookie) : props.sites,
);

watch(
  () => props.connection.baseUrl,
  (value) => {
    if (value) {
      form.baseUrl = value;
    }
  },
);

watch(autoOpenTabsOnSync, async (value) => {
  await set(STORAGE_KEYS.autoOpenTabsOnSync, value);
});

onMounted(async () => {
  const stored = await get<boolean>(STORAGE_KEYS.autoOpenTabsOnSync);
  autoOpenTabsOnSync.value = stored ?? true;
});

const syncableSites = (): KnownSiteStatus[] =>
  props.sites.filter(
    (s) =>
      s.site.syncField === "cookie" &&
      (s.cookieStatus === "valid" || s.cookieStatus === "expiring"),
  );

const openableSites = computed((): KnownSiteStatus[] =>
  props.sites.filter(
    (s) =>
      s.site.syncField !== "cookie" || s.cookieStatus === "valid" || s.cookieStatus === "expiring",
  ),
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
  form.username = "";
  form.password = "";
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
  confirmVisible.value = true;
}

const confirmText = computed((): string => {
  const n = selectedSiteIds.value.size;
  return `将打开 ${n} 个站点标签页同步 cookie，确认继续？`;
});

function confirmBatchSync(): void {
  confirmVisible.value = false;
  emit("batchSync", [...selectedSiteIds.value]);
}

function handleBatchOpen(): void {
  if (openableSites.value.length === 0) return;
  emit(
    "batchOpen",
    openableSites.value.map((s) => s.site.id),
  );
}

function cancelBatchSync(): void {
  confirmVisible.value = false;
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
      <label class="check-row auto-open-row">
        <input v-model="autoOpenTabsOnSync" type="checkbox" />
        <span>同步时自动打开标签页（全部站点）</span>
      </label>
      <label class="check-row only-cookie-row">
        <input v-model="onlyWithCookie" type="checkbox" data-testid="only-with-cookie" />
        <span>仅显示已获取 cookie 的站点</span>
      </label>
      <ul class="site-check-list">
        <li v-for="s in visibleSites" :key="s.site.id" class="site-check-item">
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
          <div class="site-status-badges">
            <span v-if="s.site.syncField !== 'cookie'" class="auth-badge">
              {{ s.site.syncField === "api_key" ? "API Key" : "Passkey" }}
            </span>
            <CookieStatus :status="s.cookieStatus" :days="s.cookieExpireDays" compact />
          </div>
        </li>
        <li v-if="visibleSites.length === 0" class="site-check-empty">暂无已获取 cookie 的站点</li>
      </ul>
      <button
        type="button"
        class="btn primary"
        :disabled="busy || selectedSiteIds.size === 0"
        @click="handleBatchSync">
        {{ busy ? t("known.syncing") : t("settings.syncSelected", selectedSiteIds.size) }}
      </button>
      <button
        type="button"
        class="btn"
        data-testid="open-all-sites-btn"
        :disabled="busy || openableSites.length === 0"
        @click="handleBatchOpen">
        一键打开站点页面 ({{ openableSites.length }})
      </button>
      <p class="open-all-hint">
        打开站点页面可保号（含 M-Team 等 API Key 站点，保号需浏览器登录）。
      </p>
    </div>

    <div class="update-section">
      <button
        type="button"
        class="btn ghost"
        data-testid="check-update-btn"
        :disabled="busy"
        @click="emit('checkUpdate')">
        检查扩展更新
      </button>
    </div>

    <SiteLoginStatusPanel />

    <div v-if="confirmVisible" class="confirm-overlay" role="dialog" aria-modal="true">
      <div class="confirm-dialog">
        <p class="confirm-text">{{ confirmText }}</p>
        <div class="row-actions">
          <button type="button" class="btn" @click="cancelBatchSync">取消</button>
          <button type="button" class="btn primary" @click="confirmBatchSync">确认继续</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.site-status-badges {
  display: flex;
  align-items: center;
  gap: 8px;
}

.auto-open-row {
  margin-top: 8px;
}

.only-cookie-row {
  margin-top: 4px;
  margin-bottom: 4px;
}

.site-check-empty {
  font-size: 12px;
  color: var(--muted, #888);
  padding: 6px 2px;
}

.confirm-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.confirm-dialog {
  background: var(--surface, #fff);
  color: var(--text, #1a1a1a);
  padding: 16px;
  border-radius: 8px;
  max-width: 320px;
  box-shadow: 0 6px 24px rgba(0, 0, 0, 0.2);
}

.confirm-text {
  margin: 0 0 12px;
  font-size: 13px;
  line-height: 1.5;
}
</style>
