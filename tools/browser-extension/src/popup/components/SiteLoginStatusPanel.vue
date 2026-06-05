<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";

import { t } from "../../core/i18n";
import { createMessage, sendToBackground } from "../../core/messages";
import type { SiteLoginStateRecord } from "../../modules/sync";
import { friendlyErrorMessage } from "../../modules/sync/errors";

interface MessageResponse<T> {
  ok: boolean;
  data?: T;
  error?: string;
}

const REFRESH_INTERVAL_MS = 60_000;

const sites = ref<SiteLoginStateRecord[]>([]);
const loading = ref(false);
const errorMsg = ref("");
const lastRefresh = ref<Date | null>(null);

let refreshTimer: number | null = null;

function asMessageResponse<T>(value: unknown): MessageResponse<T> {
  if (typeof value !== "object" || value === null) {
    return { ok: false, error: "invalid response" };
  }
  const raw = value as Record<string, unknown>;
  return {
    ok: raw.ok === true,
    data: raw.data as T | undefined,
    error: typeof raw.error === "string" ? raw.error : undefined,
  };
}

async function loadStatus(): Promise<void> {
  loading.value = true;
  errorMsg.value = "";
  try {
    const response = await sendToBackground(createMessage("GET_SITE_LOGIN_STATE", {}));
    const parsed = asMessageResponse<SiteLoginStateRecord[]>(response);
    if (!parsed.ok || parsed.data === undefined) {
      throw new Error(parsed.error ?? t("loginStatus.fetchFailed"));
    }
    sites.value = parsed.data;
    lastRefresh.value = new Date();
  } catch (err: unknown) {
    errorMsg.value =
      err instanceof Error ? friendlyErrorMessage(err.message) : t("loginStatus.fetchFailed");
  } finally {
    loading.value = false;
  }
}

function scheduleRefresh(): void {
  if (refreshTimer !== null) {
    window.clearTimeout(refreshTimer);
  }
  refreshTimer = window.setTimeout(() => {
    void loadStatus().finally(() => {
      scheduleRefresh();
    });
  }, REFRESH_INTERVAL_MS);
}

function formatTimeAgo(unixSeconds: number | undefined): string {
  if (!unixSeconds) {
    return t("loginStatus.never");
  }
  const now = Date.now();
  const then = unixSeconds * 1000;
  const diffSec = Math.max(0, Math.floor((now - then) / 1000));
  if (diffSec < 60) return t("loginStatus.justNow");
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return t("loginStatus.minutesAgo", diffMin);
  const diffHour = Math.floor(diffMin / 60);
  if (diffHour < 24) return t("loginStatus.hoursAgo", diffHour);
  const diffDay = Math.floor(diffHour / 24);
  return t("loginStatus.daysAgo", diffDay);
}

function tierLabel(tier: string): string {
  const key = `loginStatus.tier.${tier}` as Parameters<typeof t>[0];
  const localized = t(key);
  if (localized === key) {
    return tier;
  }
  return localized;
}

function tierTone(tier: string): "none" | "primary" | "warn" | "danger" | "muted" {
  switch (tier) {
    case "none":
      return "none";
    case "30d":
      return "primary";
    case "14d":
    case "7d":
      return "warn";
    case "3d":
    case "banned-imminent":
      return "danger";
    default:
      return "muted";
  }
}

function isImminent(site: SiteLoginStateRecord): boolean {
  return site.tier !== "unknown" && site.days_remaining < site.remind_before_days;
}

function effectiveTimestamp(site: SiteLoginStateRecord): number | undefined {
  if (site.effective_last_active_at) {
    return site.effective_last_active_at;
  }
  const candidates: number[] = [];
  if (site.last_login_at) candidates.push(site.last_login_at);
  if (site.last_access_at) candidates.push(site.last_access_at);
  if (site.last_visit_at) candidates.push(site.last_visit_at);
  if (candidates.length === 0) return undefined;
  return Math.max(...candidates);
}

function rowDisplayName(site: SiteLoginStateRecord): string {
  return site.display_name && site.display_name.trim() !== "" ? site.display_name : site.site_name;
}

function openInBrowser(site: SiteLoginStateRecord): void {
  const url = site.base_url?.trim();
  if (!url) {
    errorMsg.value = t("loginStatus.noBaseUrl", rowDisplayName(site));
    return;
  }
  chrome.tabs.create({ url, active: false }).catch((err: unknown) => {
    errorMsg.value = err instanceof Error ? err.message : t("loginStatus.openFailed");
  });
}

const lastRefreshText = computed((): string => {
  if (!lastRefresh.value) return t("loginStatus.notRefreshed");
  return t(
    "loginStatus.lastRefreshAt",
    lastRefresh.value.toLocaleTimeString(undefined, { hour12: false }),
  );
});

onMounted(() => {
  void loadStatus().finally(() => {
    scheduleRefresh();
  });
});

onBeforeUnmount(() => {
  if (refreshTimer !== null) {
    window.clearTimeout(refreshTimer);
    refreshTimer = null;
  }
});
</script>

<template>
  <div class="login-status-panel" data-testid="login-status-panel">
    <div class="panel-header">
      <h3 class="panel-title">{{ t("loginStatus.title") }}</h3>
      <button
        type="button"
        class="btn ghost refresh-btn"
        :disabled="loading"
        data-testid="login-status-refresh"
        @click="loadStatus">
        {{ loading ? t("loginStatus.refreshing") : t("loginStatus.refresh") }}
      </button>
    </div>

    <p class="muted-line">{{ lastRefreshText }}</p>

    <p v-if="errorMsg" class="error-line" role="alert">{{ errorMsg }}</p>

    <p v-if="!loading && sites.length === 0 && !errorMsg" class="muted-line">
      {{ t("loginStatus.empty") }}
    </p>

    <ul v-if="sites.length > 0" class="status-list">
      <li
        v-for="site in sites"
        :key="site.site_name"
        class="status-row"
        :class="{ imminent: isImminent(site) }"
        :data-testid="`login-status-row-${site.site_name}`">
        <div class="row-main">
          <span class="site-name">{{ rowDisplayName(site) }}</span>
          <span
            class="tier-badge"
            :class="`tone-${tierTone(site.tier)}`"
            :data-testid="`login-status-tier-${site.site_name}`">
            {{ tierLabel(site.tier) }}
          </span>
        </div>
        <div class="row-meta">
          <span class="meta-item">
            <span class="meta-label">{{ t("loginStatus.lastActive") }}:</span>
            <span class="meta-value">{{ formatTimeAgo(effectiveTimestamp(site)) }}</span>
          </span>
          <span v-if="site.tier !== 'unknown'" class="meta-item">
            <span class="meta-label">{{ t("loginStatus.daysRemaining") }}:</span>
            <span class="meta-value" :class="{ 'value-danger': isImminent(site) }">
              {{ site.days_remaining }}
            </span>
          </span>
        </div>
        <div class="row-actions">
          <button
            type="button"
            class="btn ghost open-btn"
            :disabled="!site.base_url"
            :data-testid="`login-status-open-${site.site_name}`"
            @click="openInBrowser(site)">
            {{ t("loginStatus.openInBrowser") }}
          </button>
        </div>
      </li>
    </ul>
  </div>
</template>

<style scoped>
.login-status-panel {
  margin-top: 12px;
  padding: 12px;
  border-radius: 8px;
  background: var(--surface-alt, rgba(0, 0, 0, 0.03));
  border: 1px solid var(--border, rgba(0, 0, 0, 0.08));
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.panel-title {
  margin: 0;
  font-size: 13px;
  font-weight: 600;
}

.refresh-btn {
  font-size: 11px;
  padding: 2px 8px;
}

.muted-line {
  margin: 4px 0;
  font-size: 11px;
  color: var(--muted, #888);
}

.error-line {
  margin: 6px 0;
  padding: 6px 8px;
  border-radius: 4px;
  background: rgba(203, 75, 63, 0.08);
  color: var(--danger, #cb4b3f);
  font-size: 12px;
}

.status-list {
  list-style: none;
  margin: 8px 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.status-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 10px;
  border-radius: 6px;
  background: var(--surface, #fff);
  border: 1px solid var(--border, rgba(0, 0, 0, 0.08));
  font-size: 12px;
}

.status-row.imminent {
  border-color: var(--danger, #cb4b3f);
  background: rgba(203, 75, 63, 0.06);
}

.row-main {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.site-name {
  font-weight: 600;
  font-size: 13px;
}

.tier-badge {
  display: inline-block;
  padding: 1px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 500;
  line-height: 1.4;
}

.tier-badge.tone-none {
  background: rgba(0, 0, 0, 0.06);
  color: var(--muted, #555);
}

.tier-badge.tone-primary {
  background: rgba(56, 132, 255, 0.12);
  color: var(--primary, #3884ff);
}

.tier-badge.tone-warn {
  background: rgba(225, 161, 36, 0.16);
  color: var(--warn, #c47b00);
}

.tier-badge.tone-danger {
  background: rgba(203, 75, 63, 0.14);
  color: var(--danger, #cb4b3f);
}

.tier-badge.tone-muted {
  background: rgba(0, 0, 0, 0.06);
  color: var(--muted, #888);
}

.row-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 11px;
  color: var(--muted, #666);
}

.meta-label {
  margin-right: 4px;
}

.meta-value {
  font-weight: 500;
  color: var(--text, #1a1a1a);
}

.meta-value.value-danger {
  color: var(--danger, #cb4b3f);
}

.row-actions {
  display: flex;
  justify-content: flex-end;
  margin-top: 2px;
}

.open-btn {
  font-size: 11px;
  padding: 2px 8px;
}
</style>
