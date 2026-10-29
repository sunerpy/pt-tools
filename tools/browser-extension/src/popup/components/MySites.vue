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
import {
  buildMySites,
  countMine,
  loginLabel,
  cookieHint,
  loginTone,
  type MySiteRow,
} from "../mySites";
import CookieStatus from "./CookieStatus.vue";

const props = defineProps<{
  sites: KnownSiteStatus[];
  sitesLoading: boolean;
  /** 显示的是上次的站点状态、这次的还没读完 */
  refreshing: boolean;
  logins: SiteLoginStateRecord[];
  /** 有 pt-tools 的登录状态可用（这次读到的，或上次存下的）；否则每行写「状态未知」，不写「pt-tools 未启用」 */
  loginsKnown: boolean;
  /** 登录状态是上次存下的那份时，读到它的时间（ISO）；这次读到的为空 */
  loginsAt: string;
  loginError: string;
  busy: boolean;
  syncingId: string;
}>();

const emit = defineEmits<{
  sync: [siteId: string];
  syncAll: [siteIds: string[]];
  openAll: [siteIds: string[]];
  login: [];
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

/** 读不到 pt-tools 时的说明：有上次的登录状态就写明是什么时候的 */
const loginNote = computed(() =>
  props.loginsAt
    ? t("mySites.loginStale", ago(props.loginsAt), props.loginError)
    : t("mySites.loginUnavailable", props.loginError),
);

function loginText(row: MySiteRow): string {
  const label = loginLabel(row, props.loginsKnown);
  if (label.kind === "unknown") return t("mySites.loginUnknown");
  if (label.kind === "savedNotEnabled") return t("mySites.savedNotEnabled");
  if (label.kind === "notInPtTools") return t("mySites.notInPtTools");
  const l = label.login;
  if (l.tier === "unknown" || !l.tier) return t("loginStatus.tier.unknown");
  if (l.tier === "none" || l.tier === "30d")
    return t("mySites.daysLeft", Math.max(0, l.days_remaining));
  const key = `loginStatus.tier.${l.tier}` as Parameters<typeof t>[0];
  const text = t(key);
  return text === key ? l.tier : text;
}

function ago(iso: string): string {
  const diffMin = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 60000));
  if (diffMin < 1) return t("loginStatus.justNow");
  if (diffMin < 60) return t("loginStatus.minutesAgo", diffMin);
  const h = Math.floor(diffMin / 60);
  if (h < 24) return t("loginStatus.hoursAgo", h);
  return t("loginStatus.daysAgo", Math.floor(h / 24));
}

function syncedText(iso: string | null): string {
  return iso ? t("mySites.synced", ago(iso)) : "";
}

/** 没登录 Cookie 时总写出来；格式认不出时没同步过才写（同步过一次就说明 pt-tools 验证通过了，写同步时间） */
function hintText(row: MySiteRow): string {
  const hint = cookieHint(row.status);
  if (hint === "noLogin") return t("mySites.cookieNoLogin");
  if (hint === "unverified" && !row.status.lastSync) return t("mySites.cookieUnverified");
  return "";
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
      <span v-if="refreshing" class="my-sites__count" data-testid="my-sites-refreshing">{{
        t("mySites.updating")
      }}</span>
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

    <p v-if="loginError" class="my-sites__note" role="status" data-testid="my-sites-login-note">
      {{ loginNote }}
      <button type="button" class="link-btn" data-testid="my-sites-login" @click="emit('login')">
        {{ t("mySites.goLogin") }}
      </button>
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
          <span
            class="my-site__synced"
            :class="{ 'my-site__missing': cookieHint(row.status) === 'noLogin' }"
            :data-testid="`synced-${row.status.site.id}`"
            >{{ hintText(row) || syncedText(row.status.lastSync) }}</span
          >
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
