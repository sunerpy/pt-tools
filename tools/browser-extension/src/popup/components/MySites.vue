<script setup lang="ts">
/*
 * 弹窗的主列表「我的站点」：只列浏览器里有 Cookie、或 pt-tools 里启用了的站点，每行是 Cookie 状态
 * 和 pt-tools 里的保号状态，行尾直接同步 / 打开。批量同步与一键打开在列表头上。
 * 以前这些都折在「全局设置」里：要先展开设置，再在六十多个站点里勾选。
 */
import { computed, ref } from "vue";

import { t } from "../../core/i18n";
import type { KnownSiteStatus } from "../../core/types";
import type { SiteLoginStateRecord } from "../../modules/sync";
import { buildMySites, countMine, loginTone, type MySiteRow } from "../mySites";
import CookieStatus from "./CookieStatus.vue";

const props = defineProps<{
  sites: KnownSiteStatus[];
  sitesLoading: boolean;
  logins: SiteLoginStateRecord[];
  loginError: string;
  busy: boolean;
  syncingId: string;
}>();

const emit = defineEmits<{
  sync: [siteId: string];
  syncAll: [siteIds: string[]];
  openAll: [siteIds: string[]];
}>();

const showAll = ref(false);

const rows = computed(() => buildMySites(props.sites, props.logins, showAll.value));
const mineCount = computed(() => countMine(props.sites, props.logins));
const syncableIds = computed(() =>
  rows.value.filter((r) => r.syncable).map((r) => r.status.site.id),
);
/** 打开站点保号：pt-tools 里启用了的，加上浏览器里登录着的 */
const openableIds = computed(() =>
  rows.value.filter((r) => r.configured || r.status.hasCookie).map((r) => r.status.site.id),
);

function loginText(row: MySiteRow): string {
  const l = row.login;
  // 定时同步只存了 Cookie、没替用户启用：写明下一步是到 pt-tools 里启用
  if (row.savedNotEnabled) return t("mySites.savedNotEnabled");
  if (!l || !row.configured) return t("mySites.notInPtTools");
  if (l.tier === "unknown" || !l.tier) return t("loginStatus.tier.unknown");
  if (l.tier === "none" || l.tier === "30d")
    return t("mySites.daysLeft", Math.max(0, l.days_remaining));
  const key = `loginStatus.tier.${l.tier}` as Parameters<typeof t>[0];
  const label = t(key);
  return label === key ? l.tier : label;
}

function syncedText(iso: string | null): string {
  if (!iso) return "";
  const diffMin = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 60000));
  if (diffMin < 1) return t("mySites.synced", t("loginStatus.justNow"));
  if (diffMin < 60) return t("mySites.synced", t("loginStatus.minutesAgo", diffMin));
  const h = Math.floor(diffMin / 60);
  if (h < 24) return t("mySites.synced", t("loginStatus.hoursAgo", h));
  return t("mySites.synced", t("loginStatus.daysAgo", Math.floor(h / 24)));
}

function openOne(row: MySiteRow): void {
  const url =
    row.login?.base_url?.trim() ||
    (row.status.site.domains[0] ? `https://${row.status.site.domains[0]}` : "");
  if (url) void chrome.tabs.create({ url, active: false });
}
</script>

<template>
  <section class="card my-sites" data-testid="my-sites">
    <header class="my-sites__head">
      <h2>{{ t("mySites.title") }}</h2>
      <span class="my-sites__count">{{ rows.length }}</span>
      <span class="my-sites__gap" />
      <button
        type="button"
        class="btn primary small"
        :disabled="busy || syncableIds.length === 0"
        data-testid="sync-all-btn"
        @click="emit('syncAll', syncableIds)">
        {{ t("mySites.syncAll", syncableIds.length) }}
      </button>
      <button
        type="button"
        class="btn small"
        :disabled="busy || openableIds.length === 0"
        :title="t('mySites.openHint')"
        data-testid="open-all-sites-btn"
        @click="emit('openAll', openableIds)">
        {{ t("mySites.openAll", openableIds.length) }}
      </button>
    </header>

    <p v-if="loginError" class="my-sites__note" role="status">
      {{ t("mySites.loginUnavailable", loginError) }}
    </p>

    <p v-if="sitesLoading && sites.length === 0" class="muted-line">{{ t("mySites.loading") }}</p>
    <p v-else-if="rows.length === 0" class="muted-line">{{ t("mySites.empty") }}</p>

    <ul v-else class="my-sites__list">
      <li
        v-for="row in rows"
        :key="row.status.site.id"
        class="my-site"
        :class="{ attention: row.attention }"
        :data-testid="`my-site-${row.status.site.id}`">
        <div class="my-site__main">
          <span class="my-site__name">{{ row.status.site.name }}</span>
          <span v-if="row.status.site.syncField !== 'cookie'" class="auth-badge">
            {{
              row.status.site.syncField === "api_key" ? t("mySites.apiKey") : t("mySites.passkey")
            }}
          </span>
          <CookieStatus
            v-else
            :status="row.status.cookieStatus"
            :days="row.status.cookieExpireDays"
            compact />
          <span
            class="my-site__login"
            :class="`tone-${loginTone(row.configured ? row.login : undefined)}`"
            :title="row.savedNotEnabled ? t('mySites.savedNotEnabledHint') : undefined">
            {{ loginText(row) }}
          </span>
        </div>
        <div class="my-site__meta">
          <span class="my-site__synced">{{ syncedText(row.status.lastSync) }}</span>
          <button type="button" class="link-btn" @click="openOne(row)">
            {{ t("mySites.open") }}
          </button>
          <button
            v-if="row.syncable"
            type="button"
            class="link-btn"
            :disabled="busy"
            :data-testid="`sync-${row.status.site.id}`"
            @click="emit('sync', row.status.site.id)">
            {{ syncingId === row.status.site.id ? t("known.syncing") : t("mySites.sync") }}
          </button>
        </div>
      </li>
    </ul>

    <button
      v-if="sites.length > 0"
      type="button"
      class="link-btn my-sites__toggle"
      data-testid="toggle-all-sites"
      @click="showAll = !showAll">
      {{ showAll ? t("mySites.showMine", mineCount) : t("mySites.showAll", sites.length) }}
    </button>
  </section>
</template>
